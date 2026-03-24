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
