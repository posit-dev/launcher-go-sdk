package launcher

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"runtime/debug"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/posit-dev/launcher-go-sdk/internal/protocol"
)

// ClusterInteractionLatencyBuckets are the histogram bucket upper bounds (in
// seconds) for the cluster interaction latency metric. These must match the
// bucket boundaries used by the Launcher so that histogram data can be
// replayed correctly on the receiving side.
var ClusterInteractionLatencyBuckets = []float64{
	0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 5.0, 10.0, 30.0,
}

// PluginMetrics contains metrics data collected by a plugin. The framework
// always reports uptimeSeconds automatically; this struct carries additional
// plugin-specific metrics.
type PluginMetrics struct {
	// ClusterInteractionLatency is a histogram snapshot of cluster
	// interaction latency in seconds. A cluster interaction is any
	// individual call to the external scheduler — a CLI command, an API
	// request, or an SDK call. Record the wall-clock duration of each
	// external call using [Histogram.Observe] and return the accumulated
	// snapshot here via [Histogram.Drain]. When nil, no latency data is
	// reported.
	ClusterInteractionLatency *protocol.HistogramSample

	// MemoryUsageBytes is the current memory usage of the plugin process in
	// bytes. This field is always serialized on the wire (no omitempty).
	// Return zero if usage is unavailable or cannot be determined; the
	// Launcher treats zero as unknown rather than as a measurement of
	// zero bytes.
	MemoryUsageBytes uint64
}

// Histogram is a thread-safe histogram that accumulates observations locally
// and can be drained into a portable snapshot for sending to the Launcher.
//
// The plugin calls [Histogram.Observe] on the hot path (e.g., after each
// scheduler command) and the framework calls [Histogram.Drain] on each
// metrics tick to collect and reset the accumulated data.
//
// Internally this uses [prometheus.Histogram] with a swap-on-drain pattern.
// The prometheus client does not expose a Reset() method on histograms, so
// [Histogram.Drain] atomically swaps the current histogram for a fresh one
// before collecting the old instance's data.
type Histogram struct {
	mu      sync.Mutex
	buckets []float64
	current prometheus.Histogram
}

// NewHistogram creates a new Histogram with the given bucket boundaries.
// Buckets must be non-empty, positive, and strictly increasing; invalid
// boundaries cause a panic (this is a programming error, not a runtime
// condition). Use [ClusterInteractionLatencyBuckets] for cluster interaction
// latency.
func NewHistogram(buckets []float64) *Histogram {
	if len(buckets) == 0 {
		panic("NewHistogram: buckets must not be empty")
	}
	for i, b := range buckets {
		if b <= 0 || math.IsNaN(b) || math.IsInf(b, 0) {
			panic(fmt.Sprintf("NewHistogram: bucket[%d] = %v, must be a finite positive number", i, b))
		}
		if i > 0 && buckets[i-1] >= b {
			panic(fmt.Sprintf("NewHistogram: buckets must be strictly increasing, but bucket[%d] (%v) >= bucket[%d] (%v)", i-1, buckets[i-1], i, b))
		}
	}
	return &Histogram{
		buckets: buckets,
		current: newPrometheusHistogram(buckets),
	}
}

func newPrometheusHistogram(buckets []float64) prometheus.Histogram {
	return prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "sdk_internal",
		Help:    "SDK internal histogram accumulator",
		Buckets: buckets,
	})
}

// Observe records a single observation (e.g., a latency measurement in
// seconds). It is safe to call from multiple goroutines. Values that are
// NaN, infinite, or negative are silently ignored.
func (h *Histogram) Observe(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.current.Observe(v)
}

// Drain collects all accumulated observations since the last drain, resets
// the histogram, and returns a portable snapshot. Returns nil if no
// observations have been recorded.
func (h *Histogram) Drain() *protocol.HistogramSample {
	h.mu.Lock()
	old := h.current
	h.current = newPrometheusHistogram(h.buckets)
	h.mu.Unlock()

	m := &dto.Metric{}
	if err := old.Write(m); err != nil {
		// Write should never fail for prometheus.NewHistogram, but guard
		// against future implementations or invalid metric configurations.
		// Returning nil here is treated as "no observations" by callers.
		slog.Debug("Histogram.Drain: failed to write metric", "error", err)
		return nil
	}
	hist := m.GetHistogram()
	if hist == nil || hist.GetSampleCount() == 0 {
		return nil
	}

	// Convert cumulative bucket counts to per-bucket (non-cumulative)
	// counts, matching the portable format used by the C++ SDK.
	buckets := make([]float64, len(hist.GetBucket()))
	var prev uint64
	for i, b := range hist.GetBucket() {
		count := b.GetCumulativeCount()
		buckets[i] = float64(count - prev)
		prev = count
	}

	return &protocol.HistogramSample{
		Buckets: buckets,
		Sum:     hist.GetSampleSum(),
	}
}

// metricsLoop runs in a goroutine (started after bootstrap completes) and
// periodically sends MetricsResponse messages to the Launcher via the
// response channel. It stops when ctx is canceled.
func metricsLoop(ctx context.Context, lgr *slog.Logger, ch chan<- interface{}, p Plugin, interval time.Duration, startTime time.Time) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	mp, ok := p.(MetricsPlugin)
	if !ok {
		mp = nil
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metricsOnce(ctx, lgr, ch, mp, startTime)
		}
	}
}

// metricsOnce collects and sends a single metrics response. Plugin
// metrics collection is wrapped in a recovery so that a panicking
// Metrics() implementation does not prevent uptime from being reported.
func metricsOnce(ctx context.Context, lgr *slog.Logger, ch chan<- interface{}, mp MetricsPlugin, startTime time.Time) {
	uptime := uint64(time.Since(startTime).Seconds())
	pm := collectPluginMetrics(ctx, lgr, mp)

	resp := protocol.NewMetricsResponse(uptime, pm.MemoryUsageBytes, pm.ClusterInteractionLatency)
	select {
	case ch <- resp:
	default:
		lgr.Warn("Metrics channel full, dropping metrics response")
	}
}

// collectPluginMetrics calls the plugin's Metrics method, recovering from
// panics. Returns a zero PluginMetrics if the plugin is nil or panics.
func collectPluginMetrics(ctx context.Context, lgr *slog.Logger, mp MetricsPlugin) (pm PluginMetrics) {
	if mp == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			lgr.Error("Panic in plugin Metrics() call",
				"plugin", fmt.Sprintf("%T", mp),
				"panic", r,
				"stack", string(debug.Stack()))
		}
	}()
	return mp.Metrics(ctx)
}
