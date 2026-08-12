package protocol

import (
	"encoding/json"
	"testing"

	"github.com/posit-dev/launcher-go-sdk/api"
)

func TestRequestFromJSON_ConfigReload(t *testing.T) {
	input := `{"messageType": 202, "requestId": 42, "requestUsername": "admin", "username": "testuser"}`

	req, err := RequestFromJSON([]byte(input))
	if err != nil {
		t.Fatalf("RequestFromJSON() error = %v", err)
	}

	cr, ok := req.(*ConfigReloadRequest)
	if !ok {
		t.Fatalf("expected *ConfigReloadRequest, got %T", req)
	}

	if cr.ID() != 42 {
		t.Errorf("ID() = %d, want 42", cr.ID())
	}
	if cr.Username != "testuser" {
		t.Errorf("Username = %q, want %q", cr.Username, "testuser")
	}
	if cr.ReqUsername != "admin" {
		t.Errorf("ReqUsername = %q, want %q", cr.ReqUsername, "admin")
	}
}

func TestNewConfigReloadResponse_Success(t *testing.T) {
	resp := NewConfigReloadResponse(42, 7, api.ReloadErrorNone, "")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if mt := int(got["messageType"].(float64)); mt != 202 {
		t.Errorf("messageType = %d, want 202", mt)
	}
	if rid := uint64(got["requestId"].(float64)); rid != 42 {
		t.Errorf("requestId = %d, want 42", rid)
	}
	if resID := uint64(got["responseId"].(float64)); resID != 7 {
		t.Errorf("responseId = %d, want 7", resID)
	}
	if et := int(got["errorType"].(float64)); et != 0 {
		t.Errorf("errorType = %d, want 0", et)
	}
	if em := got["errorMessage"].(string); em != "" {
		t.Errorf("errorMessage = %q, want empty", em)
	}
}

func TestNewMetricsResponse_Basic(t *testing.T) {
	resp := NewMetricsResponse(3600, 0, nil)

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if mt := int(got["messageType"].(float64)); mt != 203 {
		t.Errorf("messageType = %d, want 203", mt)
	}
	if rid := uint64(got["requestId"].(float64)); rid != 0 {
		t.Errorf("requestId = %d, want 0", rid)
	}
	if resID := uint64(got["responseId"].(float64)); resID != 0 {
		t.Errorf("responseId = %d, want 0", resID)
	}
	if uptime := uint64(got["uptimeSeconds"].(float64)); uptime != 3600 {
		t.Errorf("uptimeSeconds = %d, want 3600", uptime)
	}
	if mem := uint64(got["memoryUsageBytes"].(float64)); mem != 0 {
		t.Errorf("memoryUsageBytes = %d, want 0", mem)
	}
	if _, ok := got["clusterInteractionLatencySample"]; ok {
		t.Error("clusterInteractionLatencySample should be omitted when nil")
	}
}

func TestNewMetricsResponse_WithLatency(t *testing.T) {
	latency := &HistogramSample{
		Buckets: []float64{0, 2, 3, 0, 0, 0, 0, 0, 0, 0},
		Sum:     1.52,
	}
	resp := NewMetricsResponse(120, 1024*1024, latency)

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if mt := int(got["messageType"].(float64)); mt != 203 {
		t.Errorf("messageType = %d, want 203", mt)
	}
	if uptime := uint64(got["uptimeSeconds"].(float64)); uptime != 120 {
		t.Errorf("uptimeSeconds = %d, want 120", uptime)
	}
	if mem := uint64(got["memoryUsageBytes"].(float64)); mem != 1024*1024 {
		t.Errorf("memoryUsageBytes = %d, want %d", mem, 1024*1024)
	}

	sample, ok := got["clusterInteractionLatencySample"].(map[string]interface{})
	if !ok {
		t.Fatal("clusterInteractionLatencySample missing or wrong type")
	}

	buckets, ok := sample["buckets"].([]interface{})
	if !ok {
		t.Fatal("buckets missing or wrong type")
	}
	if len(buckets) != 10 {
		t.Errorf("len(buckets) = %d, want 10", len(buckets))
	}
	if buckets[1].(float64) != 2 {
		t.Errorf("buckets[1] = %v, want 2", buckets[1])
	}

	if sum := sample["sum"].(float64); sum != 1.52 {
		t.Errorf("sum = %v, want 1.52", sum)
	}
}

func TestNewJobStatusStreamResponse(t *testing.T) {
	t.Run("all fields present", func(t *testing.T) {
		resp := NewJobStatusStreamResponse(5, "job-1", "My Job", "Running", "PodRunning", "all good")

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		var got map[string]interface{}
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		if mt := int(got["messageType"].(float64)); mt != 3 {
			t.Errorf("messageType = %d, want 3", mt)
		}
		if id := got["id"].(string); id != "job-1" {
			t.Errorf("id = %q, want %q", id, "job-1")
		}
		if name := got["name"].(string); name != "My Job" {
			t.Errorf("name = %q, want %q", name, "My Job")
		}
		if status := got["status"].(string); status != "Running" {
			t.Errorf("status = %q, want %q", status, "Running")
		}
		if code := got["statusCode"].(string); code != "PodRunning" {
			t.Errorf("statusCode = %q, want %q", code, "PodRunning")
		}
		if msg := got["statusMessage"].(string); msg != "all good" {
			t.Errorf("statusMessage = %q, want %q", msg, "all good")
		}
	})

	t.Run("statusCode omitted when empty", func(t *testing.T) {
		resp := NewJobStatusStreamResponse(5, "job-1", "My Job", "Running", "", "")

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		var got map[string]interface{}
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		if _, ok := got["statusCode"]; ok {
			t.Error("statusCode should be omitted when empty")
		}
		if _, ok := got["statusMessage"]; ok {
			t.Error("statusMessage should be omitted when empty")
		}
	})
}

func TestNewConfigReloadResponse_ErrorTypes(t *testing.T) {
	tests := []struct {
		name      string
		errorType api.ConfigReloadErrorType
		wantCode  int
	}{
		{"Unknown", api.ReloadErrorUnknown, -1},
		{"Load", api.ReloadErrorLoad, 1},
		{"Validate", api.ReloadErrorValidate, 2},
		{"Save", api.ReloadErrorSave, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := NewConfigReloadResponse(10, 3, tt.errorType, "error msg")

			data, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			var got map[string]interface{}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			if et := int(got["errorType"].(float64)); et != tt.wantCode {
				t.Errorf("errorType = %d, want %d", et, tt.wantCode)
			}
			if em := got["errorMessage"].(string); em != "error msg" {
				t.Errorf("errorMessage = %q, want %q", em, "error msg")
			}
		})
	}
}

func TestConfigReloadRequest_RoundTrip_WithInheritedSettings(t *testing.T) {
	req := &ConfigReloadRequest{
		BaseUserRequest: BaseUserRequest{
			BaseRequest: BaseRequest{
				MessageType: requestTypePtr(requestConfigReload),
				RequestID:   uint64Ptr(42),
			},
		},
		Generation: 7,
		InheritedSettings: &api.InheritedSettings{
			ServerUser:                          "rstudio-server",
			EnableDebugLogging:                  true,
			ScratchPath:                         "/var/scratch",
			LoggingDir:                          "/var/log/rstudio/launcher",
			HeartbeatIntervalSeconds:            30,
			JobExpiryHours:                      24.5,
			PluginMetricsIntervalSeconds:        60,
			IncludePluginMetricsIntervalSeconds: true,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	parsed, err := RequestFromJSON(data)
	if err != nil {
		t.Fatalf("RequestFromJSON() error = %v", err)
	}

	got, ok := parsed.(*ConfigReloadRequest)
	if !ok {
		t.Fatalf("expected *ConfigReloadRequest, got %T", parsed)
	}

	if got.Generation != 7 {
		t.Errorf("Generation = %d, want 7", got.Generation)
	}
	if got.InheritedSettings == nil {
		t.Fatal("InheritedSettings = nil, want non-nil")
	}
	want := req.InheritedSettings
	is := got.InheritedSettings
	if is.ServerUser != want.ServerUser ||
		is.EnableDebugLogging != want.EnableDebugLogging ||
		is.ScratchPath != want.ScratchPath ||
		is.LoggingDir != want.LoggingDir ||
		is.HeartbeatIntervalSeconds != want.HeartbeatIntervalSeconds ||
		is.JobExpiryHours != want.JobExpiryHours ||
		is.PluginMetricsIntervalSeconds != want.PluginMetricsIntervalSeconds ||
		is.IncludePluginMetricsIntervalSeconds != want.IncludePluginMetricsIntervalSeconds {
		t.Errorf("InheritedSettings round-trip mismatch: got %+v, want %+v", is, want)
	}
}

func TestConfigReloadRequest_AbsentInheritedSettings_IsNil(t *testing.T) {
	// This is the key presence-aware test: when the Launcher omits
	// inheritedSettings entirely (as opposed to sending an empty object),
	// the parsed request must have a NIL InheritedSettings pointer, not a
	// zero-valued struct. Callers rely on nil to mean "nothing pushed down
	// this time", not "reset everything to zero/default".
	input := `{"messageType": 202, "requestId": 42, "requestUsername": "admin", "username": "testuser", "generation": 3}`

	req, err := RequestFromJSON([]byte(input))
	if err != nil {
		t.Fatalf("RequestFromJSON() error = %v", err)
	}

	cr, ok := req.(*ConfigReloadRequest)
	if !ok {
		t.Fatalf("expected *ConfigReloadRequest, got %T", req)
	}

	if cr.InheritedSettings != nil {
		t.Errorf("InheritedSettings = %+v, want nil", cr.InheritedSettings)
	}
	if cr.Generation != 3 {
		t.Errorf("Generation = %d, want 3", cr.Generation)
	}
}

func TestConfigReloadRequest_NoGenerationOrInheritedSettings_Defaults(t *testing.T) {
	// A request with none of the new fields at all (mimicking a pre-3.10.0
	// Launcher, or a reload that carries nothing new) must still parse.
	input := `{"messageType": 202, "requestId": 1, "requestUsername": "admin", "username": "testuser"}`

	req, err := RequestFromJSON([]byte(input))
	if err != nil {
		t.Fatalf("RequestFromJSON() error = %v", err)
	}

	cr, ok := req.(*ConfigReloadRequest)
	if !ok {
		t.Fatalf("expected *ConfigReloadRequest, got %T", req)
	}

	if cr.InheritedSettings != nil {
		t.Errorf("InheritedSettings = %+v, want nil", cr.InheritedSettings)
	}
	if cr.Generation != 0 {
		t.Errorf("Generation = %d, want 0", cr.Generation)
	}
}

func TestConfigReloadRequest_UnknownFields_StillParses(t *testing.T) {
	// Additive-safety: unknown extra fields (e.g. from a newer Launcher
	// version) must not break parsing.
	input := `{
		"messageType": 202, "requestId": 5, "requestUsername": "admin", "username": "testuser",
		"generation": 9,
		"inheritedSettings": {
			"serverUser": "rstudio-server",
			"enableDebugLogging": false,
			"scratchPath": "/scratch",
			"loggingDir": "/log",
			"heartbeatIntervalSeconds": 15,
			"jobExpiryHours": 12,
			"pluginMetricsIntervalSeconds": 30,
			"includePluginMetricsIntervalSeconds": false,
			"somethingBrandNewFromTheFuture": "ignored"
		},
		"somethingElseEntirely": {"a": 1}
	}`

	req, err := RequestFromJSON([]byte(input))
	if err != nil {
		t.Fatalf("RequestFromJSON() error = %v", err)
	}

	cr, ok := req.(*ConfigReloadRequest)
	if !ok {
		t.Fatalf("expected *ConfigReloadRequest, got %T", req)
	}
	if cr.Generation != 9 {
		t.Errorf("Generation = %d, want 9", cr.Generation)
	}
	if cr.InheritedSettings == nil {
		t.Fatal("InheritedSettings = nil, want non-nil")
	}
	if cr.InheritedSettings.ServerUser != "rstudio-server" {
		t.Errorf("ServerUser = %q, want %q", cr.InheritedSettings.ServerUser, "rstudio-server")
	}
}

func TestNewConfigReloadResponse_WithAppliedAndPendingRestart(t *testing.T) {
	resp := NewConfigReloadResponse(42, 7, api.ReloadErrorNone, "")
	resp.Applied = []string{"loggingDir", "scratchPath"}
	resp.PendingRestart = []string{"enableDebugLogging"}
	resp.Generation = 11

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got ConfigReloadResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if len(got.Applied) != 2 || got.Applied[0] != "loggingDir" || got.Applied[1] != "scratchPath" {
		t.Errorf("Applied = %v, want [loggingDir scratchPath]", got.Applied)
	}
	if len(got.PendingRestart) != 1 || got.PendingRestart[0] != "enableDebugLogging" {
		t.Errorf("PendingRestart = %v, want [enableDebugLogging]", got.PendingRestart)
	}
	if got.Generation != 11 {
		t.Errorf("Generation = %d, want 11", got.Generation)
	}
}

func TestNewConfigReloadResponse_EmptyAppliedAndPendingRestart(t *testing.T) {
	resp := NewConfigReloadResponse(42, 7, api.ReloadErrorNone, "")
	resp.Applied = []string{}
	resp.PendingRestart = []string{}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// The C++ Launcher's ConfigReloadResponse::toJson() always emits
	// "applied"/"pendingRestart" as arrays, even when empty, rather than
	// omitting the key - "nothing applied" must round-trip distinctly from
	// "this plugin doesn't report applied/pendingRestart at all". Match
	// that: present, empty array, never omitted and never null.
	appliedVal, ok := got["applied"].([]interface{})
	if !ok {
		t.Fatalf("applied = %v (%T), want present empty array", got["applied"], got["applied"])
	}
	if len(appliedVal) != 0 {
		t.Errorf("applied = %v, want empty array", appliedVal)
	}
	pendingVal, ok := got["pendingRestart"].([]interface{})
	if !ok {
		t.Fatalf("pendingRestart = %v (%T), want present empty array", got["pendingRestart"], got["pendingRestart"])
	}
	if len(pendingVal) != 0 {
		t.Errorf("pendingRestart = %v, want empty array", pendingVal)
	}
}

func TestNewConfigReloadResponse_NoAppliedOrPendingRestart_Defaults(t *testing.T) {
	// A response that never explicitly sets the new fields still emits them
	// as present, empty arrays (see TestNewConfigReloadResponse_EmptyAppliedAndPendingRestart),
	// and round-trips back to empty (non-nil) slices, not nil.
	resp := NewConfigReloadResponse(1, 2, api.ReloadErrorNone, "")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got ConfigReloadResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got.Applied) != 0 {
		t.Errorf("Applied = %v, want empty", got.Applied)
	}
	if len(got.PendingRestart) != 0 {
		t.Errorf("PendingRestart = %v, want empty", got.PendingRestart)
	}
	if got.Generation != 0 {
		t.Errorf("Generation = %d, want 0", got.Generation)
	}
}

func requestTypePtr(rt requestType) *requestType {
	return &rt
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}
