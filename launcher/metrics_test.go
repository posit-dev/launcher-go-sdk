package launcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/posit-dev/launcher-go-sdk/internal/protocol"
)

func TestHistogram_ObserveAndDrain(t *testing.T) {
	h := NewHistogram(ClusterInteractionLatencyBuckets)

	// Observe some values.
	h.Observe(0.005) // bucket 0: <=0.01
	h.Observe(0.03)  // bucket 2: <=0.05
	h.Observe(0.6)   // bucket 6: <=1.0
	h.Observe(2.0)   // bucket 7: <=5.0

	sample := h.Drain()
	if sample == nil {
		t.Fatal("Drain() returned nil, expected histogram sample")
	}

	if len(sample.Buckets) != len(ClusterInteractionLatencyBuckets) {
		t.Fatalf("len(Buckets) = %d, want %d",
			len(sample.Buckets), len(ClusterInteractionLatencyBuckets))
	}

	// Verify total count across all buckets.
	var total float64
	for _, c := range sample.Buckets {
		total += c
	}
	if total != 4 {
		t.Errorf("total bucket count = %v, want 4", total)
	}

	// Verify sum.
	expectedSum := 0.005 + 0.03 + 0.6 + 2.0
	if sample.Sum != expectedSum {
		t.Errorf("Sum = %v, want %v", sample.Sum, expectedSum)
	}
}

func TestHistogram_DrainEmpty(t *testing.T) {
	h := NewHistogram(ClusterInteractionLatencyBuckets)

	sample := h.Drain()
	if sample != nil {
		t.Errorf("Drain() on empty histogram = %v, want nil", sample)
	}
}

func TestHistogram_DrainResetsAccumulator(t *testing.T) {
	h := NewHistogram(ClusterInteractionLatencyBuckets)

	h.Observe(1.0)
	sample1 := h.Drain()
	if sample1 == nil {
		t.Fatal("first Drain() returned nil")
	}

	// Second drain should be empty since we reset.
	sample2 := h.Drain()
	if sample2 != nil {
		t.Errorf("second Drain() = %v, want nil (should be reset)", sample2)
	}

	// New observations should appear in the next drain.
	h.Observe(0.5)
	sample3 := h.Drain()
	if sample3 == nil {
		t.Fatal("third Drain() returned nil after new observation")
	}
	if sample3.Sum != 0.5 {
		t.Errorf("Sum after reset = %v, want 0.5", sample3.Sum)
	}
}

func TestHistogram_ConcurrentObserve(t *testing.T) {
	h := NewHistogram(ClusterInteractionLatencyBuckets)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				h.Observe(0.1)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	sample := h.Drain()
	if sample == nil {
		t.Fatal("Drain() returned nil after concurrent observations")
	}

	var total float64
	for _, c := range sample.Buckets {
		total += c
	}
	if total != 1000 {
		t.Errorf("total count = %v, want 1000", total)
	}
}

// metricsPlugin implements MetricsPlugin for testing.
type metricsPlugin struct {
	stubPlugin
	latency *Histogram
}

func (p *metricsPlugin) Metrics(_ context.Context) PluginMetrics {
	return PluginMetrics{
		ClusterInteractionLatency: p.latency.Drain(),
	}
}

func TestMetricsLoop_SendsResponses(t *testing.T) {
	ch := make(chan interface{}, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &metricsPlugin{latency: NewHistogram(ClusterInteractionLatencyBuckets)}
	p.latency.Observe(0.5)

	startTime := time.Now()
	go metricsLoop(ctx, slog.Default(), ch, p, 50*time.Millisecond, startTime)

	// Wait for at least two metrics messages.
	var responses []map[string]interface{}
	timeout := time.After(2 * time.Second)
	for len(responses) < 2 {
		select {
		case resp := <-ch:
			data, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			var m map[string]interface{}
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			responses = append(responses, m)
		case <-timeout:
			t.Fatalf("timed out waiting for metrics responses, got %d", len(responses))
		}
	}
	cancel()

	// Verify first response.
	first := responses[0]
	if mt := int(first["messageType"].(float64)); mt != 203 {
		t.Errorf("messageType = %d, want 203", mt)
	}
	if rid := uint64(first["requestId"].(float64)); rid != 0 {
		t.Errorf("requestId = %d, want 0", rid)
	}

	// First response should include latency (we observed 0.5 before starting).
	if _, ok := first["clusterInteractionLatencySample"]; !ok {
		t.Error("first response should include clusterInteractionLatencySample")
	}

	// Second response should have no latency (drained on first tick).
	if _, ok := responses[1]["clusterInteractionLatencySample"]; ok {
		t.Error("second response should not include clusterInteractionLatencySample (already drained)")
	}

	// Uptime should increase between messages.
	uptime1 := first["uptimeSeconds"].(float64)
	uptime2 := responses[1]["uptimeSeconds"].(float64)
	if uptime2 < uptime1 {
		t.Errorf("uptime should not decrease: %v -> %v", uptime1, uptime2)
	}
}

func TestMetricsLoop_NoMetricsPlugin(t *testing.T) {
	ch := make(chan interface{}, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a plain stubPlugin (no MetricsPlugin interface).
	p := &stubPlugin{}
	go metricsLoop(ctx, slog.Default(), ch, p, 50*time.Millisecond, time.Now())

	// Should still get a response with uptimeSeconds only.
	select {
	case resp := <-ch:
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if mt := int(m["messageType"].(float64)); mt != 203 {
			t.Errorf("messageType = %d, want 203", mt)
		}
		if _, ok := m["clusterInteractionLatencySample"]; ok {
			t.Error("plain plugin should not include clusterInteractionLatencySample")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for metrics response")
	}
}

func TestMetricsLoop_StopsOnContextCancel(t *testing.T) {
	ch := make(chan interface{}, 10)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		metricsLoop(ctx, slog.Default(), ch, &stubPlugin{}, 50*time.Millisecond, time.Now())
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Good, loop exited.
	case <-time.After(2 * time.Second):
		t.Fatal("metricsLoop did not exit after context cancellation")
	}
}

func TestMetricsBootstrapTrigger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &stubPlugin{}
	handler := createHandler(ctx, slog.Default(), p, 50*time.Millisecond, time.Now())

	ch := make(chan interface{}, 10)

	// Send a bootstrap request.
	req, err := protocol.RequestFromJSON([]byte(
		fmt.Sprintf(`{"messageType":1,"requestId":1,"version":{"major":%d,"minor":%d,"patch":%d}}`,
			3, 7, 0)))
	if err != nil {
		t.Fatalf("RequestFromJSON() error = %v", err)
	}
	handler(req, ch)

	// Drain the bootstrap response.
	<-ch

	// Wait for a metrics response to appear.
	select {
	case resp := <-ch:
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if mt := int(m["messageType"].(float64)); mt != 203 {
			t.Errorf("messageType = %d, want 203", mt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for metrics response after bootstrap")
	}
}

func TestMetricsDisabledWhenIntervalZero(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &stubPlugin{}
	handler := createHandler(ctx, slog.Default(), p, 0, time.Now())

	ch := make(chan interface{}, 10)

	// Send a bootstrap request.
	req, err := protocol.RequestFromJSON([]byte(
		fmt.Sprintf(`{"messageType":1,"requestId":1,"version":{"major":%d,"minor":%d,"patch":%d}}`,
			3, 7, 0)))
	if err != nil {
		t.Fatalf("RequestFromJSON() error = %v", err)
	}
	handler(req, ch)

	// Drain the bootstrap response.
	<-ch

	// Wait briefly — no metrics should appear.
	time.Sleep(200 * time.Millisecond)
	if len(ch) != 0 {
		t.Errorf("expected no metrics messages with interval=0, got %d", len(ch))
	}
}

func TestNewHistogram_Panics(t *testing.T) {
	tests := []struct {
		name    string
		buckets []float64
		wantMsg string
	}{
		{"Empty", []float64{}, "must not be empty"},
		{"Negative", []float64{0.1, -0.5, 1.0}, "finite positive number"},
		{"Zero", []float64{0, 0.5, 1.0}, "finite positive number"},
		{"NaN", []float64{0.1, math.NaN(), 1.0}, "finite positive number"},
		{"Inf", []float64{0.1, math.Inf(1), 1.0}, "finite positive number"},
		{"Equal", []float64{0.1, 0.5, 0.5, 1.0}, "strictly increasing"},
		{"Decreasing", []float64{0.1, 1.0, 0.5}, "strictly increasing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("NewHistogram should have panicked")
				}
				if !strings.Contains(fmt.Sprint(r), tt.wantMsg) {
					t.Errorf("panic = %v, want substring %q", r, tt.wantMsg)
				}
			}()
			NewHistogram(tt.buckets)
		})
	}
}

func TestHistogram_ObserveIgnoresInvalidValues(t *testing.T) {
	h := NewHistogram(ClusterInteractionLatencyBuckets)

	// All of these should be silently ignored.
	h.Observe(math.NaN())
	h.Observe(math.Inf(1))
	h.Observe(math.Inf(-1))
	h.Observe(-1.0)
	h.Observe(-0.001)

	if sample := h.Drain(); sample != nil {
		t.Errorf("Drain() after invalid observations = %+v, want nil", sample)
	}

	// A valid observation after the invalid ones should work normally.
	h.Observe(0.5)
	sample := h.Drain()
	if sample == nil {
		t.Fatal("Drain() after valid observation returned nil")
	}
	var total float64
	for _, c := range sample.Buckets {
		total += c
	}
	if total != 1 {
		t.Errorf("total count = %v, want 1", total)
	}
	if sample.Sum != 0.5 {
		t.Errorf("Sum = %v, want 0.5", sample.Sum)
	}
}

// panicPlugin panics when Metrics() is called.
type panicPlugin struct {
	stubPlugin
}

func (p *panicPlugin) Metrics(_ context.Context) PluginMetrics {
	panic("intentional panic for testing")
}

func TestMetricsLoop_RecoverFromPanic(t *testing.T) {
	ch := make(chan interface{}, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &panicPlugin{}
	lgr := slog.New(slog.NewTextHandler(io.Discard, nil))

	go metricsLoop(ctx, lgr, ch, p, 50*time.Millisecond, time.Now())

	// Wait for at least two ticks — proves the loop continues after a panic.
	var count int
	timeout := time.After(2 * time.Second)
	for count < 2 {
		select {
		case <-ch:
			count++
		case <-timeout:
			t.Fatalf("expected 2 metrics responses despite panic, got %d", count)
		}
	}
}
