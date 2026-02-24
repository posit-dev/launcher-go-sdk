// Package conformance provides automated behavioral tests for Launcher plugins.
//
// The package verifies that a [launcher.Plugin] implementation conforms to the
// behavioral contracts expected by Posit products (Workbench, Connect) that
// communicate through the Launcher.
//
// Three tiers of tests are available:
//
//   - [Run]: universal invariants that hold for all correct plugins
//   - [RunWorkflows]: product workflow tests that replay real request sequences
//   - Individual Run* functions: parameterized scenarios for isolated testing
//
// Cross-plugin behavioral deltas (e.g., Stop yields Finished on Local/K8s but
// Killed on Slurm) are handled via the [Profile] struct, which parameterizes
// expected outcomes.
//
// # Quick Start
//
//	func TestConformance(t *testing.T) {
//	    plugin := setupMyPlugin()
//	    profile := conformance.Profile{
//	        JobFactory:     func(u string) *api.Job { return &api.Job{User: u, Command: "echo hi"} },
//	        LongRunningJob: func(u string) *api.Job { return &api.Job{User: u, Command: "sleep 300"} },
//	        StopStatus:     api.StatusFinished,
//	        StopExitCodes:  []int{143},
//	        KillStatus:     api.StatusKilled,
//	        KillExitCodes:  []int{137},
//	    }
//	    conformance.Run(t, plugin, "testuser", profile)
//	    conformance.RunWorkflows(t, plugin, "testuser", profile)
//	}
package conformance

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/launcher"
	"github.com/posit-dev/launcher-go-sdk/plugintest"
)

// defaultPollInterval is used by WaitForStatus and WaitForTerminalStatus
// when polling GetJob for status changes.
const defaultPollInterval = 50 * time.Millisecond

// SubmitJob calls p.SubmitJob and returns the ID of the submitted job.
// It fails the test immediately if the plugin returns an error or does
// not return exactly one job, since tests cannot continue without a job ID.
func SubmitJob(t *testing.T, p launcher.Plugin, user string, job *api.Job) string {
	t.Helper()
	w := plugintest.NewMockResponseWriter()
	p.SubmitJob(w, user, job)
	if w.HasError() {
		t.Fatalf("SubmitJob returned error: %v", w.LastError())
	}
	jobs := w.LastJobs()
	if len(jobs) == 0 {
		t.Fatal("SubmitJob did not return a job")
	}
	if jobs[0].ID == "" {
		t.Fatal("SubmitJob returned a job with empty ID")
	}
	return jobs[0].ID
}

// GetJob calls p.GetJob and returns the job, or nil and the error if the
// plugin returned an error. Unlike [SubmitJob], this does not fail the test
// on error, allowing callers to assert on error conditions.
func GetJob(p launcher.Plugin, user string, id string, fields []string) (*api.Job, *api.Error) {
	w := plugintest.NewMockResponseWriter()
	p.GetJob(w, user, api.JobID(id), fields)
	if w.HasError() {
		return nil, w.LastError()
	}
	jobs := w.LastJobs()
	if len(jobs) == 0 {
		return nil, nil
	}
	return jobs[0], nil
}

// GetJobs calls p.GetJobs with the given filter and returns all matching jobs.
func GetJobs(p launcher.Plugin, user string, filter *api.JobFilter) []*api.Job {
	w := plugintest.NewMockResponseWriter()
	p.GetJobs(w, user, filter, nil)
	return w.AllJobs()
}

// ControlJob calls p.ControlJob and returns the result and any error.
func ControlJob(p launcher.Plugin, user string, id string, op api.JobOperation) (*plugintest.ControlResult, *api.Error) {
	w := plugintest.NewMockResponseWriter()
	p.ControlJob(w, user, api.JobID(id), op)
	if w.HasError() {
		return nil, w.LastError()
	}
	if len(w.ControlResults) == 0 {
		return nil, nil
	}
	return &w.ControlResults[0], nil
}

// WaitForStatus polls p.GetJob until the job reaches the expected status
// or the context expires. Returns the job in the expected status, or an
// error if the timeout is reached or the job enters a terminal status
// that is not the expected one.
func WaitForStatus(ctx context.Context, p launcher.Plugin, user, id, status string) (*api.Job, error) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for job %s to reach status %q: %w",
				id, status, ctx.Err())
		case <-ticker.C:
			job, apiErr := GetJob(p, user, id, nil)
			if apiErr != nil {
				return nil, fmt.Errorf("GetJob returned error: %v", apiErr)
			}
			if job == nil {
				return nil, fmt.Errorf("GetJob returned no job for ID %s", id)
			}
			if job.Status == status {
				return job, nil
			}
			// If job reached a different terminal status, it will never
			// reach the expected one.
			if api.TerminalStatus(job.Status) && !api.TerminalStatus(status) {
				return nil, fmt.Errorf("job %s reached terminal status %q while waiting for %q",
					id, job.Status, status)
			}
		}
	}
}

// WaitForTerminalStatus polls p.GetJob until the job reaches any terminal
// status (Finished, Failed, Killed, Canceled) or the context expires.
func WaitForTerminalStatus(ctx context.Context, p launcher.Plugin, user, id string) (*api.Job, error) {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for job %s to reach terminal status: %w",
				id, ctx.Err())
		case <-ticker.C:
			job, apiErr := GetJob(p, user, id, nil)
			if apiErr != nil {
				return nil, fmt.Errorf("GetJob returned error: %v", apiErr)
			}
			if job == nil {
				return nil, fmt.Errorf("GetJob returned no job for ID %s", id)
			}
			if api.TerminalStatus(job.Status) {
				return job, nil
			}
		}
	}
}

// CollectStatusStream starts a GetJobStatuses stream in a background
// goroutine and returns the mock writer capturing updates, plus a channel
// that closes when the goroutine exits.
//
// The caller must cancel ctx to stop the stream and then receive from
// the done channel to avoid goroutine leaks:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	sw, done := conformance.CollectStatusStream(ctx, plugin, user)
//	// ... use sw ...
//	cancel()
//	<-done
func CollectStatusStream(ctx context.Context, p launcher.Plugin, user string) (*plugintest.MockStreamResponseWriter, <-chan struct{}) {
	sw := plugintest.NewMockStreamResponseWriter()
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.GetJobStatuses(ctx, sw, user)
	}()
	return sw, done
}

// CollectOutputStream starts a GetJobOutput stream in a background
// goroutine and returns the mock writer capturing output chunks, plus a
// channel that closes when the goroutine exits.
//
// The same lifecycle contract as [CollectStatusStream] applies: the caller
// must cancel ctx and wait on done.
func CollectOutputStream(ctx context.Context, p launcher.Plugin, user, id string, outputType api.JobOutput) (*plugintest.MockStreamResponseWriter, <-chan struct{}) {
	sw := plugintest.NewMockStreamResponseWriter()
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.GetJobOutput(ctx, sw, user, api.JobID(id), outputType)
	}()
	return sw, done
}
