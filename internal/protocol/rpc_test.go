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
