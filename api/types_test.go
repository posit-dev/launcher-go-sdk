package api //nolint:revive // short package name is intentional for this API package

import (
	"errors"
	"testing"
)

func TestTerminalStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"pending is not terminal", StatusPending, false},
		{"running is not terminal", StatusRunning, false},
		{"suspended is not terminal", StatusSuspended, false},
		{"finished is terminal", StatusFinished, true},
		{"failed is terminal", StatusFailed, true},
		{"killed is terminal", StatusKilled, true},
		{"canceled is terminal", StatusCanceled, true},
		{"unknown status is not terminal", "Unknown", false},
		{"empty status is not terminal", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TerminalStatus(tt.status)
			if got != tt.want {
				t.Errorf("TerminalStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestJobOperation_ValidForStatus(t *testing.T) {
	tests := []struct {
		name      string
		operation JobOperation
		want      string
	}{
		{"cancel valid for pending", OperationCancel, StatusPending},
		{"kill valid for running", OperationKill, StatusRunning},
		{"stop valid for running", OperationStop, StatusRunning},
		{"suspend valid for running", OperationSuspend, StatusRunning},
		{"resume valid for suspended", OperationResume, StatusSuspended},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.operation.ValidForStatus()
			if got != tt.want {
				t.Errorf("%s.ValidForStatus() = %q, want %q", tt.operation, got, tt.want)
			}
		})
	}
}

func TestErrorf(t *testing.T) {
	tests := []struct {
		name    string
		code    ErrCode
		format  string
		args    []interface{}
		wantMsg string
	}{
		{
			name:    "simple error",
			code:    CodeJobNotFound,
			format:  "job not found",
			args:    nil,
			wantMsg: "job not found",
		},
		{
			name:    "formatted error",
			code:    CodeJobNotFound,
			format:  "job %s not found",
			args:    []interface{}{"job-123"},
			wantMsg: "job job-123 not found",
		},
		{
			name:    "multiple args",
			code:    CodeUnknown,
			format:  "failed to submit job %s: %v",
			args:    []interface{}{"job-456", "connection timeout"},
			wantMsg: "failed to submit job job-456: connection timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Errorf(tt.code, tt.format, tt.args...)

			// Check code
			if err.Code != tt.code {
				t.Errorf("Error.Code = %v, want %v", err.Code, tt.code)
			}

			// Check message
			if err.Msg != tt.wantMsg {
				t.Errorf("Error.Msg = %q, want %q", err.Msg, tt.wantMsg)
			}

			// Check Error() method
			if err.Error() != tt.wantMsg {
				t.Errorf("Error.Error() = %q, want %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	err := &Error{
		Code: CodeJobNotFound,
		Msg:  "job not found",
	}

	got := err.Error()
	want := "job not found"

	if got != want {
		t.Errorf("Error.Error() = %q, want %q", got, want)
	}
}

func TestError_Is(t *testing.T) {
	apiErr := &Error{
		Code: CodeJobNotFound,
		Msg:  "job not found",
	}

	// Test that errors.Is works with *Error
	if !errors.Is(apiErr, apiErr) {
		t.Error("errors.Is(apiErr, apiErr) = false, want true")
	}

	otherErr := &Error{
		Code: CodeJobNotFound,
		Msg:  "different message",
	}

	// Different instances with same code are not equal
	if errors.Is(apiErr, otherErr) {
		t.Error("errors.Is(apiErr, otherErr) = true, want false")
	}
}

func TestErrCode_String(t *testing.T) {
	tests := []struct {
		name string
		code ErrCode
		want string
	}{
		{"unknown error", CodeUnknown, "an error occurred"},
		{"not supported", CodeRequestNotSupported, "not supported"},
		{"invalid request", CodeInvalidRequest, "invalid request"},
		{"job not found", CodeJobNotFound, "job not found"},
		{"plugin restarted", CodePluginRestarted, "plugin is restarting"},
		{"timeout", CodeTimeout, "timeout"},
		{"job not running", CodeJobNotRunning, "job not running"},
		{"job output not found", CodeJobOutputNotFound, "job output not found"},
		{"invalid job state", CodeInvalidJobState, "invalid job state"},
		{"job control failure", CodeJobControlFailure, "job control failure"},
		{"unsupported version", CodeUnsupportedVersion, "unsupported version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.code.String()
			if got != tt.want {
				t.Errorf("%s.String() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestJobID(t *testing.T) {
	// Test that JobID is a distinct type
	var id JobID = "test-123"

	if string(id) != "test-123" {
		t.Errorf("JobID conversion failed: got %q, want %q", string(id), "test-123")
	}

	if id.IsWildcard() {
		t.Error("regular JobID should not be wildcard")
	}
	if !JobIDWildcard.IsWildcard() {
		t.Error("JobIDWildcard should be wildcard")
	}
	if string(JobIDWildcard) != "*" {
		t.Errorf("JobIDWildcard = %q, want %q", string(JobIDWildcard), "*")
	}
}

func TestJobOutput(t *testing.T) {
	// Test that JobOutput constants are defined
	tests := []struct {
		name  string
		value JobOutput
		want  string
	}{
		{"stdout constant", OutputStdout, "stdout"},
		{"stderr constant", OutputStderr, "stderr"},
		{"both constant", OutputBoth, "mixed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value.String() != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.value.String(), tt.want)
			}
		})
	}
}

func TestJobOperation(t *testing.T) {
	// Test that JobOperation constants are defined
	tests := []struct {
		name  string
		value JobOperation
		want  string
	}{
		{"cancel operation", OperationCancel, "Cancel"},
		{"kill operation", OperationKill, "Kill"},
		{"stop operation", OperationStop, "Stop"},
		{"suspend operation", OperationSuspend, "Suspend"},
		{"resume operation", OperationResume, "Resume"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value.String() != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.value.String(), tt.want)
			}
		})
	}
}

func TestJobStatusConstants(t *testing.T) {
	// Test that all job status constants are defined correctly
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"pending status", StatusPending, "Pending"},
		{"running status", StatusRunning, "Running"},
		{"suspended status", StatusSuspended, "Suspended"},
		{"finished status", StatusFinished, "Finished"},
		{"failed status", StatusFailed, "Failed"},
		{"killed status", StatusKilled, "Killed"},
		{"canceled status", StatusCanceled, "Canceled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.value, tt.want)
			}
		})
	}
}

func TestNode_Online(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"online node", "Online", true},
		{"offline node", "Offline", false},
		{"unknown status", "unknown", false},
		{"empty status", "", false},
		{"lowercase online", "online", false}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &Node{Status: tt.status}
			got := node.Online()
			if got != tt.want {
				t.Errorf("Node{Status: %q}.Online() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestJob_StructFields(t *testing.T) {
	// Test that Job struct has expected fields by creating an instance
	job := &Job{
		ID:     "test-123",
		User:   "alice",
		Status: StatusPending,
		Name:   "test-job",
	}

	if job.ID != "test-123" {
		t.Errorf("Job.ID = %q, want %q", job.ID, "test-123")
	}

	if job.User != "alice" {
		t.Errorf("Job.User = %q, want %q", job.User, "alice")
	}

	if job.Status != StatusPending {
		t.Errorf("Job.Status = %q, want %q", job.Status, StatusPending)
	}

	if job.Name != "test-job" {
		t.Errorf("Job.Name = %q, want %q", job.Name, "test-job")
	}
}

func TestJobFilter_StructFields(t *testing.T) {
	// Test that JobFilter struct has expected fields
	filter := &JobFilter{
		Statuses: []string{StatusRunning, StatusPending},
		Tags:     []string{"ml-training"},
	}

	if len(filter.Statuses) != 2 {
		t.Errorf("len(JobFilter.Statuses) = %d, want 2", len(filter.Statuses))
	}

	if filter.Statuses[0] != StatusRunning {
		t.Errorf("JobFilter.Statuses[0] = %q, want %q", filter.Statuses[0], StatusRunning)
	}

	if len(filter.Tags) != 1 {
		t.Errorf("len(JobFilter.Tags) = %d, want 1", len(filter.Tags))
	}

	if filter.Tags[0] != "ml-training" {
		t.Errorf("JobFilter.Tags[0] = %q, want %q", filter.Tags[0], "ml-training")
	}
}

func TestContainer_StructFields(t *testing.T) {
	// Test that Container struct has expected fields
	uid := 1000
	gid := 1000
	container := &Container{
		Image:      "ubuntu:22.04",
		RunAsUser:  &uid,
		RunAsGroup: &gid,
	}

	if container.Image != "ubuntu:22.04" {
		t.Errorf("Container.Image = %q, want %q", container.Image, "ubuntu:22.04")
	}

	if *container.RunAsUser != 1000 {
		t.Errorf("*Container.RunAsUser = %d, want 1000", *container.RunAsUser)
	}

	if *container.RunAsGroup != 1000 {
		t.Errorf("*Container.RunAsGroup = %d, want 1000", *container.RunAsGroup)
	}
}

func TestResourceLimit_StructFields(t *testing.T) {
	// Test that ResourceLimit struct has expected fields
	limit := &ResourceLimit{
		Type:    "cpuCount",
		Value:   "4",
		Max:     "8",
		Default: "2",
	}

	if limit.Type != "cpuCount" {
		t.Errorf("ResourceLimit.Type = %q, want %q", limit.Type, "cpuCount")
	}

	if limit.Value != "4" {
		t.Errorf("ResourceLimit.Value = %q, want %q", limit.Value, "4")
	}

	if limit.Max != "8" {
		t.Errorf("ResourceLimit.Max = %q, want %q", limit.Max, "8")
	}

	if limit.Default != "2" {
		t.Errorf("ResourceLimit.Default = %q, want %q", limit.Default, "2")
	}
}

func TestPlacementConstraint_StructFields(t *testing.T) {
	// Test that PlacementConstraint struct has expected fields
	constraint := &PlacementConstraint{
		Name:  "node-type",
		Value: "gpu",
	}

	if constraint.Name != "node-type" {
		t.Errorf("PlacementConstraint.Name = %q, want %q", constraint.Name, "node-type")
	}

	if constraint.Value != "gpu" {
		t.Errorf("PlacementConstraint.Value = %q, want %q", constraint.Value, "gpu")
	}
}

func TestResourceProfile_StructFields(t *testing.T) {
	// Test that ResourceProfile struct has expected fields
	profile := &ResourceProfile{
		Name:        "small",
		DisplayName: "Small Instance",
		Limits: []ResourceLimit{
			{Type: "cpuCount", Value: "2"},
			{Type: "memory", Value: "8GB"},
		},
	}

	if profile.Name != "small" {
		t.Errorf("ResourceProfile.Name = %q, want %q", profile.Name, "small")
	}

	if profile.DisplayName != "Small Instance" {
		t.Errorf("ResourceProfile.DisplayName = %q, want %q", profile.DisplayName, "Small Instance")
	}

	if len(profile.Limits) != 2 {
		t.Errorf("len(ResourceProfile.Limits) = %d, want 2", len(profile.Limits))
	}
}

func TestJobConfig_StructFields(t *testing.T) {
	// Test that JobConfig struct has expected fields
	config := &JobConfig{
		Name:  "email",
		Type:  "string",
		Value: "user@example.com",
	}

	if config.Name != "email" {
		t.Errorf("JobConfig.Name = %q, want %q", config.Name, "email")
	}

	if config.Type != "string" {
		t.Errorf("JobConfig.Type = %q, want %q", config.Type, "string")
	}

	if config.Value != "user@example.com" {
		t.Errorf("JobConfig.Value = %q, want %q", config.Value, "user@example.com")
	}
}
