package main

import (
	"context"
	"testing"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/conformance"
	"github.com/posit-dev/launcher-go-sdk/plugintest"
)

func TestControlJob_KillRecordsReason(t *testing.T) {
	p := newTestPlugin(t)
	id := conformance.SubmitJob(t, p, testUser, testProfile().LongRunningJob(testUser))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := conformance.WaitForStatus(ctx, p, testUser, id, api.StatusRunning); err != nil {
		t.Fatalf("WaitForStatus(Running): %v", err)
	}

	w := plugintest.NewMockResponseWriter()
	p.ControlJob(context.Background(), w, testUser, id, api.OperationKill,
		api.ControlJobOptions{StatusMessage: "Out of memory", StatusCode: "OOM"})
	if w.HasError() {
		t.Fatalf("ControlJob(Kill) returned error: %v", w.LastError())
	}

	job, apiErr := conformance.GetJob(p, testUser, id, nil)
	if apiErr != nil {
		t.Fatalf("GetJob: %v", apiErr)
	}
	if job.Status != api.StatusKilled {
		t.Errorf("Status = %q, want %q", job.Status, api.StatusKilled)
	}
	if job.StatusMsg != "Out of memory" {
		t.Errorf("StatusMsg = %q, want %q", job.StatusMsg, "Out of memory")
	}
	if job.StatusCode != "OOM" {
		t.Errorf("StatusCode = %q, want %q", job.StatusCode, "OOM")
	}
}
