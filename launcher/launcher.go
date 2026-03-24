// Package launcher provides an interface and runtime for Launcher plugins.
package launcher

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/internal/protocol"
)

// Options represents configuration options for a plugin.
type Options interface {
	// AddFlags allows a PluginOptions implementation to specify
	// command-line flags for configuration. It is primarily for use with
	// the default options, which the Launcher may expect to partially
	// configure via these flags.
	AddFlags(f *flag.FlagSet, pluginName string)

	Validate() error
}

// DefaultOptions contains the default options common to all Launcher plugins.
type DefaultOptions struct {
	// Whether debug logging should be enabled.
	Debug bool

	// The time after which failed, finished or killed jobs should disappear
	// from view. If possible, plugins should clean up jobs they launched
	// after this period.
	JobExpiry time.Duration

	// Expected time between heartbeat requests sent by the Launcher. When
	// zero, a plugin should expect no heartbeats at all. In theory, missed
	// heartbeats can be used to detect when the Launcher itself has become
	// unresponsive, but it is not obvious how a plugin could meaningfully
	// recover from that scenario.
	HeartbeatInterval time.Duration

	// Path to the Launcher configuration file. Useful in rare cases where a
	// plugin needs to know about Launcher's own settings to behave
	// correctly.
	LauncherConfig string

	// The human-friendly name of this instance of the plugin. This name
	// will also appear in the "cluster" field for job submission requests.
	PluginName string

	// Directory for temporary plugin files, if any.
	ScratchPath string

	// The unprivileged user Launcher expects this plugin to run as.
	ServerUser string

	// The plugin is running as an unprivileged user and may not have access
	// to elevated system privileges (for example, to launch processes as
	// other users). This is most often the case when Launcher is run in
	// single-user mode itself.
	Unprivileged bool

	// Directory for log files, if any.
	LoggingDir string

	// Path to the plugin's configuration file, if it has one.
	ConfigFile string

	// MetricsInterval is the interval between periodic metrics reports sent
	// to the Launcher. When zero, metrics collection is disabled.
	MetricsInterval time.Duration

	jobExpiryHours         uint
	heartbeatSeconds       uint
	metricsIntervalSeconds uint
	threadPoolSize         uint64
}

// AddFlags implements Options and exposes the default plugin options as
// command-line flags.
func (o *DefaultOptions) AddFlags(f *flag.FlagSet, pluginName string) {
	f.BoolVar(&o.Debug, "enable-debug-logging", false,
		"whether to enable debug logging or not - if true, enforces a log-level of at least DEBUG")
	f.UintVar(&o.jobExpiryHours, "job-expiry-hours", uint(24),
		"amount of hours before completed jobs are removed from the system")
	// Set by Launcher but not used by any (known) plugin. If the upstream
	// Launcher service became unresponsive and stopped sending heartbeats,
	// it's not obvious what the plugin could do to recover, anyway.
	f.UintVar(&o.heartbeatSeconds, "heartbeat-interval-seconds", uint(5),
		"the amount of seconds between heartbeats - 0 to disable")
	f.StringVar(&o.LauncherConfig, "launcher-config-file", "",
		"path to launcher config file")
	f.StringVar(&o.PluginName, "plugin-name", pluginName,
		"the name of this plugin")
	f.StringVar(&o.ScratchPath, "scratch-path",
		fmt.Sprintf("/var/lib/rstudio-launcher/%s", pluginName),
		"scratch path where temporary plugin files are stored")
	f.StringVar(&o.ServerUser, "server-user", "rstudio-server",
		"user to run the plugin as")
	f.UintVar(&o.metricsIntervalSeconds, "plugin-metrics-interval-seconds", uint(60),
		"plugin metrics collection interval in seconds - 0 to disable")
	f.Uint64Var(&o.threadPoolSize, "thread-pool-size", 0,
		"the number of threads in the thread pool (ignored)")
	// This is passed by the smoke-test tool but not by Launcher itself.
	f.BoolVar(&o.Unprivileged, "unprivileged", os.Getuid() != 0,
		"special unprivileged mode - does not change user, runs without root, no impersonation, single user")
	f.StringVar(&o.LoggingDir, "logging-dir", "/var/log/rstudio/launcher",
		"specifies path where debug logs should be written")
	f.StringVar(&o.ConfigFile, "config-file",
		ConfigPathf("launcher.%s.conf", pluginName),
		"path to main plugin configuration file")
}

// Validate implements Options.
func (o *DefaultOptions) Validate() error {
	o.JobExpiry = time.Hour * time.Duration(o.jobExpiryHours)                 //nolint:gosec // CLI flag values are small integers
	o.HeartbeatInterval = time.Second * time.Duration(o.heartbeatSeconds)     //nolint:gosec // CLI flag values are small integers
	o.MetricsInterval = time.Second * time.Duration(o.metricsIntervalSeconds) //nolint:gosec // CLI flag values are small integers
	return nil
}

// LoadOptions loads options from command-line flags.
func LoadOptions(o Options, pluginName string) error {
	o.AddFlags(flag.CommandLine, strings.ToLower(pluginName))
	flag.Parse()
	return o.Validate()
}

// MustLoadOptions is like LoadOptions, but prints a message to standard error
// on failure and then aborts. This is recommended.
func MustLoadOptions(o Options, pluginName string) {
	if err := LoadOptions(o, pluginName); err != nil {
		// Use a basic logger since we may not have initialized one yet
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}
}

// ConfigPathf returns the path to a configuration file, checking for the usual
// XDG and product-specific configuration directories (which may be influenced
// by environment variables) before falling back to a path in /etc.
func ConfigPathf(format string, a ...interface{}) string {
	leafPath := fmt.Sprintf(format, a...)
	configDir := os.ExpandEnv(os.Getenv("RSTUDIO_CONFIG_DIR"))
	if configDir != "" {
		return path.Join(configDir, leafPath)
	}
	configDir = os.ExpandEnv(os.Getenv("XDG_CONFIG_DIRS"))
	for _, dir := range strings.Split(configDir, ":") {
		dir = path.Join(dir, "rstudio")
		if _, err := os.Stat(dir); err == nil {
			return path.Join(dir, leafPath)
		}
	}
	return path.Join("/etc/rstudio", leafPath)
}

// Plugin is the main interface that all launcher plugins must implement.
//
// Every method receives a context.Context. For streaming methods (those that
// accept a [StreamResponseWriter]), the context is request-scoped and canceled
// when the Launcher sends a cancel request or the plugin shuts down. For all
// other methods the same server-wide context is passed to every call; it is
// canceled only on plugin shutdown (e.g. SIGTERM). The Launcher protocol does
// not support per-request cancellation for non-streaming operations. If a
// non-streaming method needs a per-operation deadline, create a child context:
//
//	func (p *MyPlugin) SubmitJob(ctx context.Context, w ResponseWriter, ...) {
//	    opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
//	    defer cancel()
//	    // use opCtx for the operation
//	}
type Plugin interface {
	SubmitJob(ctx context.Context, w ResponseWriter, user string, job *api.Job)

	GetJob(ctx context.Context, w ResponseWriter, user string, id api.JobID, fields []string)

	GetJobs(ctx context.Context, w ResponseWriter, user string, filter *api.JobFilter, fields []string)

	ControlJob(ctx context.Context, w ResponseWriter, user string, id api.JobID, op api.JobOperation)

	GetJobStatus(ctx context.Context, w StreamResponseWriter, user string, id api.JobID)

	GetJobStatuses(ctx context.Context, w StreamResponseWriter, user string)

	GetJobOutput(ctx context.Context, w StreamResponseWriter, user string, id api.JobID, outputType api.JobOutput)

	GetJobResourceUtil(ctx context.Context, w StreamResponseWriter, user string, id api.JobID)

	GetJobNetwork(ctx context.Context, w ResponseWriter, user string, id api.JobID)

	ClusterInfo(ctx context.Context, w ResponseWriter, user string)
}

// BootstrappedPlugin can be implemented by plugins that want an explicit
// bootstrap phase.
type BootstrappedPlugin interface {
	Plugin

	// This method will be called when Launcher first begins communicating
	// with the plugin. No other methods will be called before it returns. A
	// response writer is provided to send (unrecoverable) errors back to
	// Launcher.
	Bootstrap(ctx context.Context, w ResponseWriter)
}

// MultiClusterPlugin can be implemented by plugins that allow job submission to
// more than one cluster. Note that this is an extension mechanism not supported
// by all Launcher implementations.
type MultiClusterPlugin interface {
	Plugin
	GetClusters(ctx context.Context, w MultiClusterResponseWriter, user string)
}

// LoadBalancedPlugin can be implemented by plugins that must be aware of other
// nodes.
type LoadBalancedPlugin interface {
	Plugin

	// This method will be called when the nodes in a load-balanced Launcher
	// cluster change. It is purely advisory.
	SyncNodes(nodes []api.Node)
}

// ConfigReloadablePlugin can be implemented by plugins that support runtime
// configuration reloading. When the Launcher sends a config reload request,
// ReloadConfig will be called and the SDK writes the response automatically
// based on the returned error. Plugins that do not implement this interface
// will send an empty success response automatically.
//
// At minimum, plugins should reload user profiles and resource profiles.
// Reloading additional configuration (e.g., the main plugin configuration
// file) is permitted but not required.
type ConfigReloadablePlugin interface {
	Plugin

	// ReloadConfig is called when the Launcher requests a configuration
	// reload. Return nil on success. Return a [*ConfigReloadError] to
	// provide a classified error type, or any other error for an
	// unclassified failure.
	ReloadConfig(ctx context.Context) error
}

// MetricsPlugin can be implemented by plugins that want to report custom
// metrics to the Launcher. The Metrics method is called periodically
// (controlled by the --plugin-metrics-interval-seconds flag). The returned
// PluginMetrics will be sent alongside the framework-managed uptimeSeconds.
//
// All plugins automatically report uptimeSeconds. Implement this interface
// only if you need to report additional plugin-specific metrics such as
// cluster interaction latency.
type MetricsPlugin interface {
	Plugin

	// Metrics is called periodically to collect plugin-specific metrics.
	// Implementations should return quickly and avoid blocking I/O.
	Metrics(ctx context.Context) PluginMetrics
}

// ConfigReloadError represents a configuration reload failure with a
// classified error type. Plugins should return this from
// [ConfigReloadablePlugin.ReloadConfig] to provide both an error type and
// message. If a plain error is returned, the error type defaults to
// [api.ReloadErrorUnknown].
type ConfigReloadError struct {
	Type    api.ConfigReloadErrorType
	Message string
}

func (e *ConfigReloadError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("config reload failed: %s", e.Type)
}

// ResponseWriter is the interface for writing responses back to the Launcher.
// Methods return error to allow implementations flexibility in error reporting
// (e.g., mock writers can simulate failures). Most plugin code treats writes as
// fire-and-forget operations since encoding errors are handled by the protocol
// layer and will cause the plugin to terminate.
type ResponseWriter interface {
	// WriteErrorf sends a formatted error with the given code to the
	// Launcher.
	WriteErrorf(code api.ErrCode, format string, a ...interface{}) error

	// WriteError sends an error to the Launcher. Use api.Errorf() to create
	// standalone errors with an embedded code.
	WriteError(error) error
	WriteJobs(jobs []*api.Job) error
	WriteControlJob(complete bool, msg string) error
	WriteJobNetwork(host string, addr []string) error
	WriteClusterInfo(ClusterOptions) error
}

// StreamResponseWriter is the interface for writing streaming responses.
type StreamResponseWriter interface {
	ResponseWriter
	WriteJobStatus(id api.JobID, status, msg string) error
	WriteJobOutput(output string, outputType api.JobOutput) error
	WriteJobResourceUtil(cpuPercent float64, cpuTime float64,
		residentMem float64, virtualMem float64) error
	Close() error
}

// MultiClusterResponseWriter is the writer for multicluster responses.
type MultiClusterResponseWriter interface {
	ResponseWriter
	// WriteClusters sends Launcher information about available clusters.
	WriteClusters([]ClusterOptions) error
}

// ClusterOptions describes the capabilities and configuration of a cluster.
type ClusterOptions struct {
	Constraints  []api.PlacementConstraint
	Queues       []string
	DefaultQueue string
	Limits       []api.ResourceLimit
	ImageOpt     ImageOptions
	Configs      []api.JobConfig
	Profiles     []api.ResourceProfile
	Name         string
}

func (o *ClusterOptions) toProtocol() protocol.ClusterInfo {
	return protocol.ClusterInfo{
		Containers:   len(o.ImageOpt.Images) != 0,
		Constraints:  o.Constraints,
		Queues:       o.Queues,
		DefaultQueue: o.DefaultQueue,
		Limits:       o.Limits,
		Images:       o.ImageOpt.Images,
		DefaultImage: o.ImageOpt.Default,
		AllowUnknown: o.ImageOpt.AllowUnknown,
		Configs:      o.Configs,
		Profiles:     o.Profiles,
		HostNetwork:  o.ImageOpt.HostNetwork,
		Name:         o.Name,
	}
}

// ImageOptions describes container image configuration for a cluster.
type ImageOptions struct {
	Default      string
	Images       []string
	AllowUnknown bool

	// When true, containers use the host network namespace and jobs cannot
	// specify exposed ports. This is common when using Singularity or other
	// HPC container solutions.
	HostNetwork bool
}

// Errorf creates an error with the corresponding plugin API code.
var Errorf = api.Errorf

// Runtime manages the plugin lifecycle and communication with the Launcher.
type Runtime struct {
	// MaxMessageSize is the upper limit on message size for requests and
	// responses.
	MaxMessageSize int

	// MetricsInterval is the interval between periodic metrics reports sent
	// to the Launcher. When zero (the default), metrics collection is
	// disabled. Set this from [DefaultOptions.MetricsInterval] after
	// calling [NewRuntime]:
	//
	//	rt := launcher.NewRuntime(lgr, plugin)
	//	rt.MetricsInterval = options.MetricsInterval
	MetricsInterval time.Duration

	lgr *slog.Logger
	p   Plugin
}

// NewRuntime creates a new Runtime for the given plugin.
func NewRuntime(lgr *slog.Logger, p Plugin) *Runtime {
	return &Runtime{
		MaxMessageSize: protocol.DefaultMaxMsgSize,
		lgr:            lgr,
		p:              p,
	}
}

// Run starts the plugin runtime, handling requests from Launcher.
func (r *Runtime) Run(ctx context.Context) error {
	comm := protocol.NewCommunicator(r.lgr, os.Stdin, os.Stdout, r.MaxMessageSize)
	revision, date := buildInfo()
	r.lgr.Info("Starting plugin", "revision", revision, "released", date,
		"api_version", fmt.Sprintf("%d.%d.%d", api.APIVersion.Major,
			api.APIVersion.Minor, api.APIVersion.Patch))
	defer func() {
		r.lgr.Info("Plugin stopped")
	}()
	if r.MetricsInterval > 0 {
		r.lgr.Info("Metrics collection enabled", "interval", r.MetricsInterval)
	} else {
		r.lgr.Info("Metrics collection disabled (interval=0)")
	}
	startTime := time.Now()
	return comm.Serve(ctx, createHandler(ctx, r.lgr, r.p, r.MetricsInterval, startTime))
}

func createHandler(ctx context.Context, lgr *slog.Logger, p Plugin, metricsInterval time.Duration, startTime time.Time) func(req protocol.Request, ch chan<- interface{}) {
	s := newStreamStore(ctx)
	var metricsOnce sync.Once
	return func(req protocol.Request, ch chan<- interface{}) {
		var w *defaultResponseWriter
		switch r := req.(type) {
		case *protocol.HeartbeatRequest:
			w = newResponseWriter(req, ch)
			//nolint:errcheck // sendResponse currently always returns nil
			w.WriteHeartbeat()
		case *protocol.BootstrapRequest:
			w = newResponseWriter(req, ch)
			v := r.Version
			if v.Major != api.APIVersion.Major {
				//nolint:errcheck // sendResponse currently always returns nil
				w.WriteErrorf(api.CodeUnsupportedVersion,
					"The plugin supports API version %d.X.XXXX. "+
						"The Launcher's API version is %d.%d.%d",
					api.APIVersion.Major, v.Major, v.Minor, v.Patch)
				return
			}
			bsPlugin, ok := p.(BootstrappedPlugin)
			if ok {
				bsPlugin.Bootstrap(ctx, w)
			}
			//nolint:errcheck // sendResponse currently always returns nil
			w.WriteBootstrap()
			// Start metrics collection after bootstrap completes.
			if metricsInterval > 0 {
				metricsOnce.Do(func() {
					go metricsLoop(ctx, lgr, ch, p, metricsInterval, startTime)
				})
			}
		case *protocol.SubmitJobRequest:
			w = newResponseWriter(req, ch)
			if r.Username != "*" && r.Job.User == "" {
				r.Job.User = r.Username
			}
			if r.Job.User == "" {
				//nolint:errcheck // sendResponse currently always returns nil
				w.WriteErrorf(api.CodeInvalidRequest,
					"User must not be empty")
				return
			}
			// Treat a missing resource profile as the custom profile, which
			// allows setting manual limits.
			if r.Job.Profile == "" {
				r.Job.Profile = "custom"
			}
			p.SubmitJob(ctx, w, r.Username, r.Job)
		case *protocol.JobStateRequest:
			w = newResponseWriter(req, ch)
			if !r.JobID.IsWildcard() {
				p.GetJob(ctx, w, r.Username, r.JobID, r.Fields)
				return
			}
			p.GetJobs(ctx, w, r.Username, &r.JobFilter, r.Fields)
		case *protocol.JobStatusStreamRequest:
			if r.Cancel {
				s.Cancel(r.ID())
				return
			}
			ctx := s.Start(r.ID())
			w = newStreamWriter(req, ch, s)
			if !r.JobID.IsWildcard() {
				p.GetJobStatus(ctx, w, r.Username, r.JobID)
				return
			}
			p.GetJobStatuses(ctx, w, r.Username)
		case *protocol.ControlJobRequest:
			w = newResponseWriter(req, ch)
			if r.JobID.IsWildcard() {
				//nolint:errcheck // sendResponse currently always returns nil
				w.WriteErrorf(api.CodeInvalidRequest,
					"Cannot control all jobs simultaneously. Please specify a single Job ID.")
				return
			}
			// Validate that the operation field doesn't have some random
			// integer.
			switch r.Operation {
			case api.OperationSuspend, api.OperationResume,
				api.OperationStop, api.OperationKill,
				api.OperationCancel:
			default:
				//nolint:errcheck // sendResponse currently always returns nil
				w.WriteErrorf(api.CodeInvalidRequest,
					"Unknown control job operation (%d) for job %s",
					r.Operation, r.JobID)
				return
			}
			p.ControlJob(ctx, w, r.Username, r.JobID, r.Operation)
		case *protocol.JobOutputRequest:
			if r.Cancel {
				s.Cancel(r.ID())
				return
			}
			ctx := s.Start(r.ID())
			w = newStreamWriter(req, ch, s)
			p.GetJobOutput(ctx, w, r.Username, r.JobID, r.Output)
		case *protocol.JobResourceUtilRequest:
			if r.Cancel {
				s.Cancel(r.ID())
				return
			}
			ctx := s.Start(r.ID())
			w = newStreamWriter(req, ch, s)
			p.GetJobResourceUtil(ctx, w, r.Username, r.JobID)
		case *protocol.JobNetworkRequest:
			w = newResponseWriter(req, ch)
			p.GetJobNetwork(ctx, w, r.Username, r.JobID)
		case *protocol.ClusterInfoRequest:
			w = newResponseWriter(req, ch)
			p.ClusterInfo(ctx, w, r.Username)
		case *protocol.MultiClusterInfoRequest:
			w = newResponseWriter(req, ch)
			mcPlugin, ok := p.(MultiClusterPlugin)
			if !ok {
				// Servers must allow multicluster requests to return a
				// single-cluster response.
				p.ClusterInfo(ctx, w, r.Username)
				return
			}
			mcPlugin.GetClusters(ctx, w, r.Username)
		case *protocol.SetLoadBalancerNodesRequest:
			w = newResponseWriter(req, ch)
			lbPlugin, ok := p.(LoadBalancedPlugin)
			if ok {
				lbPlugin.SyncNodes(r.Nodes)
			}
			//nolint:errcheck // sendResponse currently always returns nil
			w.WriteSetLoadBalancerNodes()
		case *protocol.ConfigReloadRequest:
			w = newResponseWriter(req, ch)
			crPlugin, ok := p.(ConfigReloadablePlugin)
			if !ok {
				//nolint:errcheck // sendResponse currently always returns nil
				w.WriteConfigReload(api.ReloadErrorNone, "")
				return
			}
			if err := crPlugin.ReloadConfig(ctx); err != nil {
				var crErr *ConfigReloadError
				if errors.As(err, &crErr) {
					//nolint:errcheck // sendResponse currently always returns nil
					w.WriteConfigReload(crErr.Type, crErr.Message)
				} else {
					//nolint:errcheck // sendResponse currently always returns nil
					w.WriteConfigReload(api.ReloadErrorUnknown, err.Error())
				}
				return
			}
			//nolint:errcheck // sendResponse currently always returns nil
			w.WriteConfigReload(api.ReloadErrorNone, "")
		default:
			w = newResponseWriter(req, ch)
			//nolint:errcheck // sendResponse currently always returns nil
			w.WriteErrorf(api.CodeRequestNotSupported,
				"Request not supported")
		}
	}
}

func buildInfo() (string, string) {
	revision, date := "unknown", "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				revision = s.Value[:7] // Short hash.
			}
			if s.Key == "vcs.time" {
				date = s.Value
			}
		}
	}
	return revision, date
}

var responseCounter uint64

// nextResponseID generates the next response ID and increments the underlying
// counter. It can be called safely from multiple goroutines.
func nextResponseID() uint64 {
	// Emulate C++'s fetch_add(1).
	return atomic.AddUint64(&responseCounter, 1) - 1
}

type defaultResponseWriter struct {
	ch    chan<- interface{}
	req   protocol.Request
	store *streamStore
}

func newResponseWriter(req protocol.Request, ch chan<- interface{}) *defaultResponseWriter {
	return &defaultResponseWriter{ch: ch, req: req}
}

func newStreamWriter(req protocol.Request, ch chan<- interface{}, store *streamStore) *defaultResponseWriter {
	return &defaultResponseWriter{ch, req, store}
}

func (w *defaultResponseWriter) WriteErrorf(code api.ErrCode, format string, a ...interface{}) error {
	resp := protocol.NewErrorResponse(w.req.ID(), code,
		fmt.Sprintf(format, a...))
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) WriteError(err error) error {
	resp := protocol.NewErrorResponse(w.req.ID(), api.CodeUnknown, err.Error())
	var werr *api.Error
	if errors.As(err, &werr) {
		resp.Code = werr.Code
	}
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) WriteJobs(jobs []*api.Job) error {
	resp := protocol.NewJobStateResponse(w.req.ID(), nextResponseID(), jobs)
	return w.sendResponse(resp)
}

// errNotStreamWriter is returned when a streaming method is called on a
// non-stream response writer.
var errNotStreamWriter = fmt.Errorf("method called on non-stream response writer")

func (w *defaultResponseWriter) WriteJobStatus(id api.JobID, status, msg string) error {
	if w.store == nil {
		return errNotStreamWriter
	}
	rid := w.req.ID()
	resp := protocol.NewJobStatusStreamResponse(nextResponseID(), string(id),
		status, msg)
	resp.Sequences = []protocol.StreamSequence{
		{RequestID: rid, SequenceID: w.store.SequenceID(rid)},
	}
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) WriteJobOutput(output string, outputType api.JobOutput) error {
	if w.store == nil {
		return errNotStreamWriter
	}
	rid := w.req.ID()
	resp := protocol.NewJobOutputStreamResponse(rid, nextResponseID())
	resp.SequenceID = w.store.SequenceID(rid)
	resp.Output = output
	resp.OutputType = outputType.String()
	resp.Complete = false
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) WriteJobResourceUtil(cpuPercent, cpuTime, residentMem, virtualMem float64) error {
	if w.store == nil {
		return errNotStreamWriter
	}
	rid := w.req.ID()
	resp := protocol.NewJobResourceResponse(nextResponseID(), false)
	resp.Sequences = []protocol.StreamSequence{
		{RequestID: rid, SequenceID: w.store.SequenceID(rid)},
	}
	resp.CPUPercent = cpuPercent
	resp.CPUTime = cpuTime
	resp.ResidentMem = residentMem
	resp.VirtualMem = virtualMem
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) Close() error {
	if w.store == nil {
		return nil
	}
	// Only some types of request have a completion message.
	switch w.req.(type) {
	case *protocol.JobOutputRequest:
		rid := w.req.ID()
		resp := protocol.NewJobOutputStreamResponse(rid, nextResponseID())
		resp.SequenceID = w.store.SequenceID(rid)
		resp.Complete = true
		return w.sendResponse(resp)
	case *protocol.JobResourceUtilRequest:
		rid := w.req.ID()
		resp := protocol.NewJobResourceResponse(nextResponseID(), true)
		resp.Sequences = []protocol.StreamSequence{
			{RequestID: rid, SequenceID: w.store.SequenceID(rid)},
		}
		return w.sendResponse(resp)
	}
	return nil
}

func (w *defaultResponseWriter) WriteControlJob(complete bool, msg string) error {
	resp := protocol.NewControlJobResponse(w.req.ID(), nextResponseID(),
		complete, msg)
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) WriteJobNetwork(host string, addr []string) error {
	resp := protocol.NewJobNetworkResponse(w.req.ID(), nextResponseID(), host, addr)
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) WriteClusterInfo(o ClusterOptions) error {
	resp := protocol.NewClusterInfoResponse(w.req.ID(), nextResponseID(),
		o.toProtocol())
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) WriteClusters(o []ClusterOptions) error {
	clusters := make([]protocol.ClusterInfo, len(o))
	for i := range o {
		clusters[i] = o[i].toProtocol()
	}
	resp := protocol.NewMultiClusterInfoResponse(w.req.ID(), nextResponseID(),
		clusters)
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) WriteHeartbeat() error {
	return w.sendResponse(protocol.NewHeartbeatResponse())
}

func (w *defaultResponseWriter) WriteBootstrap() error {
	resp := protocol.NewBootstrapResponse(w.req.ID(), nextResponseID())
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) WriteSetLoadBalancerNodes() error {
	resp := protocol.NewSetLoadBalancerNodesResponse(w.req.ID(), nextResponseID())
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) WriteConfigReload(errorType api.ConfigReloadErrorType, errorMessage string) error {
	resp := protocol.NewConfigReloadResponse(w.req.ID(), nextResponseID(), errorType, errorMessage)
	return w.sendResponse(resp)
}

func (w *defaultResponseWriter) sendResponse(resp interface{}) error {
	w.ch <- resp
	return nil
}

// streamStore is a concurrency-safe store for stream state, which maps a
// request ID to a cancellable context and sequence ID.
type streamStore struct {
	sync.Mutex
	ctx     context.Context
	streams map[uint64]*streamRecord
}

// newStreamStore creates a store with the given parent context.
func newStreamStore(ctx context.Context) *streamStore {
	return &streamStore{
		ctx:     ctx,
		streams: make(map[uint64]*streamRecord),
	}
}

// Start starts a stream for the given request ID and returns a context. This context
// will be canceled when Cancel() is called.
func (s *streamStore) Start(requestID uint64) context.Context {
	s.Lock()
	defer s.Unlock()
	rec, ok := s.streams[requestID]
	if ok {
		return rec.Context
	}
	rec = &streamRecord{}
	rec.Context, rec.Cancel = context.WithCancel(s.ctx) //nolint:gosec // G118: cancel is stored and called in Cancel()
	s.streams[requestID] = rec
	return rec.Context
}

// Cancel ends the stream for the given request ID and cancels the context that
// was returned from Start().
func (s *streamStore) Cancel(requestID uint64) {
	s.Lock()
	defer s.Unlock()
	if rec, ok := s.streams[requestID]; ok {
		rec.Cancel()
		delete(s.streams, requestID)
	}
}

// SequenceID returns the current sequence ID for a given request ID, if any.
func (s *streamStore) SequenceID(requestID uint64) uint64 {
	s.Lock()
	defer s.Unlock()
	if rec, ok := s.streams[requestID]; ok {
		rec.SeqID++
		return rec.SeqID
	}
	return 0
}

type streamRecord struct {
	Context context.Context
	Cancel  context.CancelFunc
	SeqID   uint64
}
