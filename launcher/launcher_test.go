package launcher

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/internal/protocol"
)

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
	handler := createHandler(ctx, p)
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
