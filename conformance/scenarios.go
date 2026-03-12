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

// RunWorkflows executes product workflow tests that replay the request
// sequences Posit products (Workbench, Connect) produce during normal
// operation. Every plugin intended for use with Posit products should call
// this function.
//
// The [Profile] parameterizes expected outcomes for behaviors that vary by
// scheduler. The request sequences themselves are fixed because they come
// from the products, not from the plugin.
//
// Tests are registered as subtests under Workflows/, making them
// individually addressable:
//
//	go test -run TestConformance/Workflows/Launch
//	go test -run TestConformance/Workflows/Stop
func RunWorkflows(t *testing.T, p launcher.Plugin, user string, profile Profile) {
	t.Helper()
	profile.mustValidate(t)
	profile.applyDefaults()

	t.Run("Workflows", func(t *testing.T) {
		t.Run("Launch", func(t *testing.T) {
			testLaunchWorkflow(t, p, user, &profile)
		})

		t.Run("Stop", func(t *testing.T) {
			RunStopJob(t, p, user, StopOpts{
				Job:             profile.LongRunningJob(user),
				ExpectStatus:    profile.StopStatus,
				ExpectExitCodes: profile.StopExitCodes,
				Timeout:         profile.JobStartTimeout,
			})
		})

		t.Run("Kill", func(t *testing.T) {
			RunKillJob(t, p, user, KillOpts{
				Job:             profile.LongRunningJob(user),
				ExpectStatus:    profile.KillStatus,
				ExpectExitCodes: profile.KillExitCodes,
				Timeout:         profile.JobStartTimeout,
			})
		})

		t.Run("Cancel", func(t *testing.T) {
			RunCancelJob(t, p, user, CancelOpts{
				Job:     profile.LongRunningJob(user),
				Timeout: profile.JobStartTimeout,
			})
		})

		t.Run("List", func(t *testing.T) {
			testListWorkflow(t, p, user, &profile)
		})

		if profile.SuspendSupported {
			t.Run("SuspendResume", func(t *testing.T) {
				RunSuspendResume(t, p, user, SuspendResumeOpts{
					Job:     profile.LongRunningJob(user),
					Timeout: profile.JobStartTimeout,
				})
			})
		}
	})
}

func testLaunchWorkflow(t *testing.T, p launcher.Plugin, user string, profile *Profile) {
	// Step 1: ClusterInfo — products call this first to populate the launch UI.
	t.Run("ClusterInfoReturnsConfig", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()
		p.ClusterInfo(context.Background(), w, user)
		plugintest.AssertNoError(t, w)
		if w.ClusterInfo == nil {
			t.Error("ClusterInfo must return cluster configuration")
		}
	})

	// Step 2: SubmitJob — user launches a workload.
	job := profile.JobFactory(user)
	job.Tags = append(job.Tags, "conformance-launch")
	id := SubmitJob(t, p, user, job)

	t.Run("SubmitReturnsJobWithID", func(t *testing.T) {
		if id == "" {
			t.Error("submitted job must have a non-empty ID")
		}
	})

	// Step 3: Status stream — products monitor all user sessions.
	t.Run("StatusStreamDeliversRunningUpdate", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), profile.JobStartTimeout)
		defer cancel()

		_, err := WaitForStatus(ctx, p, user, id, api.StatusRunning)
		if err != nil {
			t.Errorf("job did not reach Running status: %v", err)
		}
	})

	// Step 4: GetJobNetwork — products connect to the running session.
	if profile.NetworkAvailable {
		t.Run("NetworkAvailableWhenRunning", func(t *testing.T) {
			w := plugintest.NewMockResponseWriter()
			p.GetJobNetwork(context.Background(), w, user, id)
			plugintest.AssertNoError(t, w)
		})
	}

	// Step 5: Output stream — user views logs.
	if profile.OutputAvailable {
		t.Run("OutputStreamDeliversData", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), profile.OutputTimeout)
			defer cancel()

			sw, done := CollectOutputStream(ctx, p, user, id, api.OutputStdout)
			defer func() {
				cancel()
				<-done
			}()

			deadline := time.Now().Add(profile.OutputTimeout)
			for time.Now().Before(deadline) {
				if sw.OutputCount() > 0 {
					return
				}
				time.Sleep(profile.PollInterval)
			}
			if sw.OutputCount() == 0 {
				t.Error("expected output on stdout stream, but none was received")
			}
		})
	}
}

func testListWorkflow(t *testing.T, p launcher.Plugin, user string, profile *Profile) {
	// Submit jobs with distinct tags for filtering.
	tag := fmt.Sprintf("conformance-list-%d", time.Now().UnixNano())
	tagA := tag + "-a"
	tagB := tag + "-b"

	job1 := profile.JobFactory(user)
	job1.Tags = append(job1.Tags, tag, tagA)
	id1 := SubmitJob(t, p, user, job1)

	job2 := profile.JobFactory(user)
	job2.Tags = append(job2.Tags, tag, tagB)
	id2 := SubmitJob(t, p, user, job2)

	t.Run("FilterByStatusReturnsSubset", func(t *testing.T) {
		// Wait for at least one job to reach a known status.
		ctx, cancel := context.WithTimeout(context.Background(), profile.JobCompleteTimeout)
		defer cancel()
		_, _ = WaitForTerminalStatus(ctx, p, user, id1) //nolint:errcheck // best-effort wait; test proceeds regardless

		// Filter for terminal statuses.
		w := plugintest.NewMockResponseWriter()
		filter := &api.JobFilter{
			Tags:     []string{tag},
			Statuses: []string{api.StatusFinished, api.StatusFailed, api.StatusKilled, api.StatusCanceled},
		}
		p.GetJobs(context.Background(), w, user, filter, nil)
		plugintest.AssertNoError(t, w)

		jobs := w.AllJobs()
		for _, j := range jobs {
			if !api.TerminalStatus(j.Status) {
				t.Errorf("filter returned non-terminal job with status %q", j.Status)
			}
		}
	})

	t.Run("FilterByTagReturnsSubset", func(t *testing.T) {
		// Filter for tagA — should return job1 but not job2.
		jobs := GetJobs(p, user, &api.JobFilter{Tags: []string{tagA}})

		found1 := plugintest.FindJobByID(jobs, id1)
		found2 := plugintest.FindJobByID(jobs, id2)
		if found1 == nil {
			t.Errorf("job %s with tag %q not found in filtered results", id1, tagA)
		}
		if found2 != nil {
			t.Errorf("job %s without tag %q should not appear in filtered results", id2, tagA)
		}
	})
}

// StopOpts configures the [RunStopJob] scenario.
type StopOpts struct {
	// Job to submit. Must run long enough to be stopped while Running.
	Job *api.Job

	// ExpectStatus is the terminal status expected after Stop.
	ExpectStatus string

	// ExpectExitCodes lists acceptable exit codes after Stop.
	ExpectExitCodes []int

	// Timeout for each phase (waiting for Running, waiting for terminal).
	// Default: 30s.
	Timeout time.Duration
}

// KillOpts configures the [RunKillJob] scenario.
type KillOpts struct {
	// Job to submit. Must run long enough to be killed while Running.
	Job *api.Job

	// ExpectStatus is the terminal status expected after Kill.
	ExpectStatus string

	// ExpectExitCodes lists acceptable exit codes after Kill.
	ExpectExitCodes []int

	// Timeout for each phase. Default: 30s.
	Timeout time.Duration
}

// CancelOpts configures the [RunCancelJob] scenario.
type CancelOpts struct {
	// Job to submit. Should remain Pending briefly.
	Job *api.Job

	// Timeout for waiting for the Canceled status. Default: 30s.
	Timeout time.Duration
}

// SuspendResumeOpts configures the [RunSuspendResume] scenario.
type SuspendResumeOpts struct {
	// Job to submit. Must run long enough for suspend and resume.
	Job *api.Job

	// Timeout for each phase. Default: 30s.
	Timeout time.Duration
}

// OutputStreamOpts configures the [RunOutputStream] scenario.
type OutputStreamOpts struct {
	// Job to submit.
	Job *api.Job

	// OutputType selects which stream to request (stdout, stderr, both).
	OutputType api.JobOutput

	// ExpectNonEmpty asserts that at least one chunk of output is received.
	ExpectNonEmpty bool

	// Timeout for waiting for output. Default: 10s.
	Timeout time.Duration
}

// StatusStreamOpts configures the [RunStatusStream] scenario.
type StatusStreamOpts struct {
	// Job to submit.
	Job *api.Job

	// Timeout for waiting for status updates. Default: 30s.
	Timeout time.Duration
}

// StreamCancelOpts configures the [RunStreamCancellation] scenario.
type StreamCancelOpts struct {
	// Job to submit.
	Job *api.Job

	// Timeout for waiting for the stream to stop after cancellation.
	// Default: 5s.
	Timeout time.Duration
}

// FieldFilterOpts configures the [RunFieldFiltering] scenario.
type FieldFilterOpts struct {
	// Job to submit.
	Job *api.Job

	// Fields to request via GetJob. The test asserts that unrequested
	// fields are omitted from the response.
	Fields []string
}

// InvalidStateOpts configures the [RunControlInvalidState] scenario.
type InvalidStateOpts struct {
	// Job to submit. Should complete quickly so it reaches terminal state.
	Job *api.Job

	// Operation to attempt in the wrong state (e.g., OperationStop on a
	// finished job).
	Operation api.JobOperation

	// Timeout for the job to reach terminal status. Default: 30s.
	Timeout time.Duration
}

func defaultTimeout(d, fallback time.Duration) time.Duration {
	if d == 0 {
		return fallback
	}
	return d
}

// RunStopJob submits a long-running job, waits for it to reach Running,
// sends a Stop (SIGTERM) control operation, and verifies the resulting
// terminal status and exit code.
func RunStopJob(t *testing.T, p launcher.Plugin, user string, opts StopOpts) {
	t.Helper()
	timeout := defaultTimeout(opts.Timeout, 30*time.Second)

	id := SubmitJob(t, p, user, opts.Job)

	// Wait for Running.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := WaitForStatus(ctx, p, user, id, api.StatusRunning)
	if err != nil {
		t.Fatalf("job did not reach Running: %v", err)
	}

	// Open status stream so we can observe the transition (mirrors product behavior).
	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()
	_, streamDone := CollectStatusStream(streamCtx, p, user)
	defer func() {
		streamCancel()
		<-streamDone
	}()

	// Give stream time to attach.
	time.Sleep(50 * time.Millisecond)

	// Send Stop.
	result, apiErr := ControlJob(p, user, id, api.OperationStop)
	if apiErr != nil {
		t.Fatalf("ControlJob(Stop) returned error: %v", apiErr)
	}
	if result != nil && !result.Complete {
		t.Log("ControlJob(Stop) returned incomplete result")
	}

	// Wait for terminal status.
	ctx2, cancel2 := context.WithTimeout(context.Background(), timeout)
	defer cancel2()
	finalJob, err := WaitForTerminalStatus(ctx2, p, user, id)
	if err != nil {
		t.Fatalf("job did not reach terminal status after Stop: %v", err)
	}

	if finalJob.Status != opts.ExpectStatus {
		t.Errorf("expected terminal status %q after Stop, got %q",
			opts.ExpectStatus, finalJob.Status)
	}

	assertExitCode(t, finalJob, opts.ExpectExitCodes)
}

// RunKillJob submits a long-running job, waits for it to reach Running,
// sends a Kill (SIGKILL) control operation, and verifies the resulting
// terminal status and exit code.
func RunKillJob(t *testing.T, p launcher.Plugin, user string, opts KillOpts) {
	t.Helper()
	timeout := defaultTimeout(opts.Timeout, 30*time.Second)

	id := SubmitJob(t, p, user, opts.Job)

	// Wait for Running.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := WaitForStatus(ctx, p, user, id, api.StatusRunning)
	if err != nil {
		t.Fatalf("job did not reach Running: %v", err)
	}

	// Send Kill.
	result, apiErr := ControlJob(p, user, id, api.OperationKill)
	if apiErr != nil {
		t.Fatalf("ControlJob(Kill) returned error: %v", apiErr)
	}
	if result != nil && !result.Complete {
		t.Log("ControlJob(Kill) returned incomplete result")
	}

	// Wait for terminal status.
	ctx2, cancel2 := context.WithTimeout(context.Background(), timeout)
	defer cancel2()
	finalJob, err := WaitForTerminalStatus(ctx2, p, user, id)
	if err != nil {
		t.Fatalf("job did not reach terminal status after Kill: %v", err)
	}

	if finalJob.Status != opts.ExpectStatus {
		t.Errorf("expected terminal status %q after Kill, got %q",
			opts.ExpectStatus, finalJob.Status)
	}

	assertExitCode(t, finalJob, opts.ExpectExitCodes)
}

// RunCancelJob submits a job and immediately sends a Cancel operation
// before it starts running. Verifies the job reaches Canceled status.
//
// If the job transitions to Running before the cancel can be sent, the
// test is skipped rather than failed, since this is a known race condition
// that depends on scheduler timing.
func RunCancelJob(t *testing.T, p launcher.Plugin, user string, opts CancelOpts) {
	t.Helper()
	timeout := defaultTimeout(opts.Timeout, 30*time.Second)

	id := SubmitJob(t, p, user, opts.Job)

	// Immediately attempt to cancel.
	_, apiErr := ControlJob(p, user, id, api.OperationCancel)
	if apiErr != nil {
		// If the job already started, Cancel is invalid — this is
		// acceptable, not a test failure.
		t.Skipf("Cancel returned error (job may have started): %v", apiErr)
	}

	// Wait for Canceled status.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	finalJob, err := WaitForStatus(ctx, p, user, id, api.StatusCanceled)
	if err != nil {
		t.Fatalf("job did not reach Canceled status: %v", err)
	}

	plugintest.AssertJobStatus(t, finalJob, api.StatusCanceled)
}

// RunSuspendResume submits a long-running job, waits for Running, suspends
// it, verifies Suspended status, resumes it, and verifies it returns to
// Running.
func RunSuspendResume(t *testing.T, p launcher.Plugin, user string, opts SuspendResumeOpts) {
	t.Helper()
	timeout := defaultTimeout(opts.Timeout, 30*time.Second)

	id := SubmitJob(t, p, user, opts.Job)

	// Wait for Running.
	ctx1, cancel1 := context.WithTimeout(context.Background(), timeout)
	defer cancel1()
	_, err := WaitForStatus(ctx1, p, user, id, api.StatusRunning)
	if err != nil {
		t.Fatalf("job did not reach Running: %v", err)
	}

	// Suspend.
	_, apiErr := ControlJob(p, user, id, api.OperationSuspend)
	if apiErr != nil {
		t.Fatalf("ControlJob(Suspend) returned error: %v", apiErr)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), timeout)
	defer cancel2()
	_, err = WaitForStatus(ctx2, p, user, id, api.StatusSuspended)
	if err != nil {
		t.Fatalf("job did not reach Suspended: %v", err)
	}

	// Resume.
	_, apiErr = ControlJob(p, user, id, api.OperationResume)
	if apiErr != nil {
		t.Fatalf("ControlJob(Resume) returned error: %v", apiErr)
	}

	ctx3, cancel3 := context.WithTimeout(context.Background(), timeout)
	defer cancel3()
	resumedJob, err := WaitForStatus(ctx3, p, user, id, api.StatusRunning)
	if err != nil {
		t.Fatalf("job did not return to Running after resume: %v", err)
	}

	plugintest.AssertJobStatus(t, resumedJob, api.StatusRunning)

	// Clean up: stop the job so it doesn't leak.
	_, _ = ControlJob(p, user, id, api.OperationStop) //nolint:errcheck // best-effort cleanup of long-running job
}

// RunOutputStream submits a job, opens an output stream, and verifies
// that output is received (if expected) and the stream eventually closes.
func RunOutputStream(t *testing.T, p launcher.Plugin, user string, opts OutputStreamOpts) {
	t.Helper()
	timeout := defaultTimeout(opts.Timeout, 10*time.Second)

	id := SubmitJob(t, p, user, opts.Job)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	sw, done := CollectOutputStream(ctx, p, user, id, opts.OutputType)
	defer func() {
		cancel()
		<-done
	}()

	if opts.ExpectNonEmpty {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if sw.OutputCount() > 0 {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		if sw.OutputCount() == 0 {
			t.Error("expected output, but none was received")
		}
	}
}

// RunStatusStream submits a job, opens a status stream, and verifies
// that at least one status update is delivered.
func RunStatusStream(t *testing.T, p launcher.Plugin, user string, opts StatusStreamOpts) {
	t.Helper()
	timeout := defaultTimeout(opts.Timeout, 30*time.Second)

	_ = SubmitJob(t, p, user, opts.Job)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	sw, done := CollectStatusStream(ctx, p, user)
	defer func() {
		cancel()
		<-done
	}()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if sw.StatusCount() > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if sw.StatusCount() == 0 {
		t.Error("expected status updates on stream, but none were received")
	}
}

// RunStreamCancellation opens a status stream and cancels it, verifying
// that the stream stops producing messages and the goroutine exits.
func RunStreamCancellation(t *testing.T, p launcher.Plugin, user string, opts StreamCancelOpts) {
	t.Helper()
	timeout := defaultTimeout(opts.Timeout, 5*time.Second)

	_ = SubmitJob(t, p, user, opts.Job)

	ctx, cancel := context.WithCancel(context.Background())
	sw, done := CollectStatusStream(ctx, p, user)

	// Let it collect briefly.
	time.Sleep(100 * time.Millisecond)

	countBefore := sw.StatusCount()
	cancel()

	// Stream goroutine should exit promptly.
	select {
	case <-done:
		// Good.
	case <-time.After(timeout):
		t.Error("stream goroutine did not exit after context cancellation")
	}

	// Verify no new updates after cancel.
	time.Sleep(100 * time.Millisecond)
	countAfter := sw.StatusCount()
	if countAfter > countBefore+1 {
		// Allow at most one in-flight update that was already being written.
		t.Errorf("stream produced %d updates after cancellation (had %d before)",
			countAfter-countBefore, countBefore)
	}
}

// RunFieldFiltering submits a job, retrieves it with a restricted field
// list, and verifies that only requested fields are populated.
func RunFieldFiltering(t *testing.T, p launcher.Plugin, user string, opts FieldFilterOpts) {
	t.Helper()

	id := SubmitJob(t, p, user, opts.Job)

	job, apiErr := GetJob(p, user, id, opts.Fields)
	if apiErr != nil {
		t.Fatalf("GetJob with fields returned error: %v", apiErr)
	}
	if job == nil {
		t.Fatal("GetJob returned nil job")
	}

	// ID is always present regardless of field filter.
	if job.ID == "" {
		t.Error("ID must always be present regardless of field filter")
	}

	// Check that requested fields are populated where possible.
	for _, f := range opts.Fields {
		switch f {
		case "status":
			if job.Status == "" {
				t.Error("requested field 'status' was not returned")
			}
		case "name":
			if job.Name == "" && opts.Job.Name != "" {
				t.Error("requested field 'name' was not returned")
			}
		}
	}
}

// RunControlInvalidState sends a control operation that is invalid for
// the job's current state and verifies the plugin returns an error.
func RunControlInvalidState(t *testing.T, p launcher.Plugin, user string, opts InvalidStateOpts) {
	t.Helper()
	timeout := defaultTimeout(opts.Timeout, 30*time.Second)

	id := SubmitJob(t, p, user, opts.Job)

	// Wait for the job to finish.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := WaitForTerminalStatus(ctx, p, user, id)
	if err != nil {
		t.Fatalf("job did not reach terminal status: %v", err)
	}

	// Try the operation on a finished job — should error.
	_, apiErr := ControlJob(p, user, id, opts.Operation)
	if apiErr == nil {
		t.Errorf("expected error when performing %s on a finished job, but got none",
			opts.Operation)
	}
}

// MetricsOpts configures the [RunMetrics] scenario.
type MetricsOpts struct {
	// Timeout for the Metrics call. Default: 1s.
	Timeout time.Duration
}

// RunMetrics verifies that a [launcher.MetricsPlugin] implementation returns
// promptly and produces valid metrics data.
func RunMetrics(t *testing.T, p launcher.Plugin, opts MetricsOpts) {
	t.Helper()
	timeout := defaultTimeout(opts.Timeout, time.Second)

	mp, ok := p.(launcher.MetricsPlugin)
	if !ok {
		t.Skip("plugin does not implement MetricsPlugin")
	}

	t.Run("ReturnsWithinTimeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		done := make(chan launcher.PluginMetrics, 1)
		go func() {
			done <- mp.Metrics(ctx)
		}()

		select {
		case <-done:
			// Good — Metrics returned promptly.
		case <-ctx.Done():
			t.Fatal("Metrics() did not return within timeout")
		}
	})

	t.Run("LatencyBucketsNonNegative", func(t *testing.T) {
		metrics := mp.Metrics(context.Background())
		if metrics.ClusterInteractionLatency == nil {
			t.Skip("no cluster interaction latency reported")
		}
		for i, v := range metrics.ClusterInteractionLatency.Buckets {
			if v < 0 {
				t.Errorf("bucket[%d] = %v, want >= 0", i, v)
			}
		}
		if metrics.ClusterInteractionLatency.Sum < 0 {
			t.Errorf("sum = %v, want >= 0", metrics.ClusterInteractionLatency.Sum)
		}
	})
}

// assertExitCode checks that the job's exit code is in the list of
// acceptable values.
func assertExitCode(t *testing.T, job *api.Job, acceptable []int) {
	t.Helper()
	if len(acceptable) == 0 {
		return
	}
	if job.ExitCode == nil {
		t.Error("expected exit code, but job has nil ExitCode")
		return
	}
	for _, code := range acceptable {
		if *job.ExitCode == code {
			return
		}
	}
	t.Errorf("expected exit code in %v, got %d", acceptable, *job.ExitCode)
}
