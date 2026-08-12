package launcher

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/internal/protocol"
)

// TestDefaultOptions_EnableDebugLogging verifies that --enable-debug-logging
// consumes its value argument so that subsequent flags are not silently dropped.
// The launcher passes "--enable-debug-logging 0" (space-separated), which
// flag.BoolVar does not handle: the bare flag name sets Debug to true without
// consuming the next token, so "0" becomes a stray non-flag argument that halts
// parsing of all later flags.
func TestDefaultOptions_EnableDebugLogging(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantDebug bool
		wantErr   bool
	}{
		{"space-sep-0", []string{"--enable-debug-logging", "0", "--plugin-name", "myplugin"}, false, false},
		{"space-sep-1", []string{"--enable-debug-logging", "1", "--plugin-name", "myplugin"}, true, false},
		{"equals-false", []string{"--enable-debug-logging=false", "--plugin-name", "myplugin"}, false, false},
		{"equals-true", []string{"--enable-debug-logging=true", "--plugin-name", "myplugin"}, true, false},
		{"absent", []string{"--plugin-name", "myplugin"}, false, false},
		{"invalid-value", []string{"--enable-debug-logging", "maybe", "--plugin-name", "myplugin"}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts DefaultOptions
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			opts.AddFlags(fs, "default")
			err := fs.Parse(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Parse() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if fs.NArg() != 0 {
				t.Errorf("unparsed args = %v, want none", fs.Args())
			}
			if opts.Debug != tt.wantDebug {
				t.Errorf("Debug = %v, want %v", opts.Debug, tt.wantDebug)
			}
			if opts.PluginName != "myplugin" {
				t.Errorf("PluginName = %q, want %q", opts.PluginName, "myplugin")
			}
		})
	}
}

// TestDefaultOptions_JobExpiryHours verifies that --job-expiry-hours accepts
// fractional and integer values and is converted into a time.Duration.
// The C++ Launcher treats job-expiry-hours as a float end-to-end (including
// fractional and scientific-notation values cascaded to plugins at spawn), so
// the Go SDK must parse it as a float64 rather than a uint to avoid dying at
// startup (flag.Parse on flag.CommandLine is ExitOnError) on values like 0.5.
func TestDefaultOptions_JobExpiryHours(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want time.Duration
	}{
		{"fractional", []string{"--job-expiry-hours", "0.5"}, 30 * time.Minute},
		{"integer", []string{"--job-expiry-hours", "24"}, 24 * time.Hour},
		{"default", nil, 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts DefaultOptions
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			opts.AddFlags(fs, "default")
			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if err := opts.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if opts.JobExpiry != tt.want {
				t.Errorf("JobExpiry = %v, want %v", opts.JobExpiry, tt.want)
			}
		})
	}
}

// stubPlugin implements Plugin with no-op methods.
type stubPlugin struct{}

func (stubPlugin) SubmitJob(context.Context, ResponseWriter, string, *api.Job)               {}
func (stubPlugin) GetJob(context.Context, ResponseWriter, string, api.JobID, []string)       {}
func (stubPlugin) GetJobs(context.Context, ResponseWriter, string, *api.JobFilter, []string) {}
func (stubPlugin) ControlJob(context.Context, ResponseWriter, string, api.JobID, api.JobOperation) {
}
func (stubPlugin) GetJobStatus(context.Context, StreamResponseWriter, string, api.JobID) {}
func (stubPlugin) GetJobStatuses(context.Context, StreamResponseWriter, string)          {}
func (stubPlugin) GetJobOutput(context.Context, StreamResponseWriter, string, api.JobID, api.JobOutput) {
}
func (stubPlugin) GetJobResourceUtil(context.Context, StreamResponseWriter, string, api.JobID) {}
func (stubPlugin) GetJobNetwork(context.Context, ResponseWriter, string, api.JobID)            {}
func (stubPlugin) ClusterInfo(context.Context, ResponseWriter, string)                         {}

// reloadablePlugin implements ConfigReloadablePlugin.
type reloadablePlugin struct {
	stubPlugin
	err error
}

func (p *reloadablePlugin) ReloadConfig(context.Context) error {
	return p.err
}

func newConfigReloadRequest(t *testing.T, requestID uint64) protocol.Request {
	t.Helper()
	data := fmt.Sprintf(`{"messageType":202,"requestId":%d,"requestUsername":"admin","username":"testuser"}`, requestID)
	req, err := protocol.RequestFromJSON([]byte(data))
	if err != nil {
		t.Fatalf("RequestFromJSON() error = %v", err)
	}
	return req
}

// configReloadResult unmarshals a config reload response from the channel.
type configReloadResult struct {
	MessageType  int    `json:"messageType"`
	RequestID    uint64 `json:"requestId"`
	ResponseID   uint64 `json:"responseId"`
	ErrorType    int    `json:"errorType"`
	ErrorMessage string `json:"errorMessage"`
}

func runConfigReloadHandler(t *testing.T, p Plugin, requestID uint64) configReloadResult {
	t.Helper()
	ctx := context.Background()
	handler := createHandler(ctx, slog.Default(), p, 0, time.Now())
	ch := make(chan interface{}, 1)
	req := newConfigReloadRequest(t, requestID)
	handler(req, ch)

	if len(ch) == 0 {
		t.Fatal("expected a response on the channel")
	}

	resp := <-ch
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var result configReloadResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return result
}

func TestConfigReload_NotImplemented(t *testing.T) {
	result := runConfigReloadHandler(t, &stubPlugin{}, 42)

	if result.MessageType != 202 {
		t.Errorf("messageType = %d, want 202", result.MessageType)
	}
	if result.RequestID != 42 {
		t.Errorf("requestId = %d, want 42", result.RequestID)
	}
	if result.ErrorType != 0 {
		t.Errorf("errorType = %d, want 0 (None)", result.ErrorType)
	}
	if result.ErrorMessage != "" {
		t.Errorf("errorMessage = %q, want empty", result.ErrorMessage)
	}
}

func TestConfigReload_Success(t *testing.T) {
	p := &reloadablePlugin{err: nil}
	result := runConfigReloadHandler(t, p, 10)

	if result.ErrorType != 0 {
		t.Errorf("errorType = %d, want 0 (None)", result.ErrorType)
	}
	if result.ErrorMessage != "" {
		t.Errorf("errorMessage = %q, want empty", result.ErrorMessage)
	}
}

func TestConfigReload_ConfigReloadError(t *testing.T) {
	p := &reloadablePlugin{
		err: &ConfigReloadError{
			Type:    api.ReloadErrorValidate,
			Message: "invalid profiles",
		},
	}
	result := runConfigReloadHandler(t, p, 10)

	if result.ErrorType != 2 {
		t.Errorf("errorType = %d, want 2 (Validate)", result.ErrorType)
	}
	if result.ErrorMessage != "invalid profiles" {
		t.Errorf("errorMessage = %q, want %q", result.ErrorMessage, "invalid profiles")
	}
}

func TestConfigReload_PlainError(t *testing.T) {
	p := &reloadablePlugin{err: fmt.Errorf("something broke")}
	result := runConfigReloadHandler(t, p, 10)

	if result.ErrorType != -1 {
		t.Errorf("errorType = %d, want -1 (Unknown)", result.ErrorType)
	}
	if result.ErrorMessage != "something broke" {
		t.Errorf("errorMessage = %q, want %q", result.ErrorMessage, "something broke")
	}
}

func TestConfigReload_WrappedConfigReloadError(t *testing.T) {
	inner := &ConfigReloadError{
		Type:    api.ReloadErrorLoad,
		Message: "config file syntax error",
	}
	p := &reloadablePlugin{err: fmt.Errorf("reload failed: %w", inner)}
	result := runConfigReloadHandler(t, p, 10)

	if result.ErrorType != 1 {
		t.Errorf("errorType = %d, want 1 (Load)", result.ErrorType)
	}
	if result.ErrorMessage != "config file syntax error" {
		t.Errorf("errorMessage = %q, want %q", result.ErrorMessage, "config file syntax error")
	}
}
