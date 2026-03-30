package plugintest_test

import (
	"testing"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/plugintest"
)

// Example of using JobBuilder to create test jobs.
func ExampleJobBuilder() {
	job := plugintest.NewJob().
		WithID("job-123").
		WithUser("alice").
		WithCommand("python train.py").
		WithEnv("MODEL_PATH", "/models/v1").
		WithTag("ml-training").
		Running().
		Build()

	_ = job // Job is now ready for use in tests
}

// Example of using MockResponseWriter to capture responses.
func ExampleMockResponseWriter() {
	w := plugintest.NewMockResponseWriter()

	// Simulate a plugin writing a job
	job := plugintest.NewJob().WithID("job-1").Build()
	_ = w.WriteJobs([]*api.Job{job})

	// Make assertions
	_ = len(w.AllJobs()) // Returns the number of jobs written
}

// TestExampleAssertions shows how to use assertion helpers in tests.
func TestExampleAssertions(t *testing.T) {
	job := plugintest.NewJob().Running().Build()
	plugintest.AssertJobStatus(t, job, api.StatusRunning)
}

func TestJobBuilder(t *testing.T) {
	t.Run("creates job with default values", func(t *testing.T) {
		job := plugintest.NewJob().Build()

		if job.ID == "" {
			t.Error("expected job to have an ID")
		}
		if job.Status == "" {
			t.Error("expected job to have a status")
		}
		if job.Submitted == nil {
			t.Error("expected job to have a submission time")
		}
	})

	t.Run("sets all fields correctly", func(t *testing.T) {
		job := plugintest.NewJob().
			WithID("test-123").
			WithUser("alice").
			WithCommand("echo hello").
			WithEnv("FOO", "bar").
			WithTag("test").
			Running().
			Build()

		plugintest.AssertJobID(t, job, "test-123")
		plugintest.AssertJobUser(t, job, "alice")
		plugintest.AssertJobStatus(t, job, api.StatusRunning)
		plugintest.AssertJobHasTag(t, job, "test")
		plugintest.AssertJobHasEnv(t, job, "FOO")
		plugintest.AssertJobEnvValue(t, job, "FOO", "bar")
	})

	t.Run("supports status shortcuts", func(t *testing.T) {
		pending := plugintest.NewJob().Pending().Build()
		plugintest.AssertJobStatus(t, pending, api.StatusPending)

		running := plugintest.NewJob().Running().Build()
		plugintest.AssertJobStatus(t, running, api.StatusRunning)

		finished := plugintest.NewJob().Finished(0).Build()
		plugintest.AssertJobStatus(t, finished, api.StatusFinished)
		plugintest.AssertJobExitCode(t, finished, 0)

		failed := plugintest.NewJob().Failed("error occurred").Build()
		plugintest.AssertJobStatus(t, failed, api.StatusFailed)
	})

	t.Run("clones jobs correctly", func(t *testing.T) {
		original := plugintest.NewJob().
			WithID("original").
			WithTag("tag1").
			Build()

		clonedBuilder := plugintest.NewJob().
			WithID("original").
			WithTag("tag1").
			Clone()

		cloned := clonedBuilder.WithID("cloned").Build()

		if original.ID == cloned.ID {
			t.Error("clone should have different ID after modification")
		}
		if len(original.Tags) != len(cloned.Tags) {
			t.Error("clone should preserve tags")
		}
	})
}

func TestMockResponseWriter(t *testing.T) {
	t.Run("captures errors", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()

		w.WriteErrorf(api.CodeJobNotFound, "Job %s not found", "job-1")

		plugintest.AssertError(t, w)
		plugintest.AssertErrorCode(t, w, api.CodeJobNotFound)
		plugintest.AssertErrorMessage(t, w, "job-1")
	})

	t.Run("captures jobs", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()

		job1 := plugintest.NewJob().WithID("job-1").Build()
		job2 := plugintest.NewJob().WithID("job-2").Build()

		w.WriteJobs([]*api.Job{job1})
		w.WriteJobs([]*api.Job{job2})

		plugintest.AssertJobCount(t, w, 2)

		foundJob1 := plugintest.FindJobByID(w.AllJobs(), "job-1")
		if foundJob1 == nil {
			t.Error("expected to find job-1")
		}
	})

	t.Run("captures control results", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()

		w.WriteControlJob(true, "Job stopped")

		plugintest.AssertControlComplete(t, w, "Job stopped")
	})

	t.Run("captures cluster info", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()

		opts := plugintest.NewClusterOptions().
			WithQueue("default").
			Build()

		w.WriteClusterInfo(opts)

		if w.ClusterInfo == nil {
			t.Error("expected cluster info to be captured")
		}
	})

	t.Run("resets correctly", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()

		w.WriteErrorf(api.CodeUnknown, "error")
		w.WriteJobs([]*api.Job{plugintest.NewJob().Build()})

		w.Reset()

		plugintest.AssertNoError(t, w)
		plugintest.AssertJobCount(t, w, 0)
	})
}

func TestMockStreamResponseWriter(t *testing.T) {
	t.Run("captures status updates", func(t *testing.T) {
		w := plugintest.NewMockStreamResponseWriter()

		w.WriteJobStatus("job-1", "My Job", api.StatusRunning, "", "Job started")
		w.WriteJobStatus("job-1", "My Job", api.StatusFinished, "", "Job completed")

		plugintest.AssertStatusCount(t, w, 2)

		lastStatus := w.LastStatus()
		if lastStatus == nil {
			t.Fatal("expected last status")
		}
		if lastStatus.Status != api.StatusFinished {
			t.Errorf("expected last status to be %s, got %s", api.StatusFinished, lastStatus.Status)
		}
	})

	t.Run("captures output", func(t *testing.T) {
		w := plugintest.NewMockStreamResponseWriter()

		w.WriteJobOutput("line 1\n", api.OutputStdout)
		w.WriteJobOutput("line 2\n", api.OutputStdout)

		plugintest.AssertOutputCount(t, w, 2)

		combined := w.CombinedOutput()
		expected := "line 1\nline 2\n"
		if combined != expected {
			t.Errorf("expected combined output %q, got %q", expected, combined)
		}
	})

	t.Run("captures resource utilization", func(t *testing.T) {
		w := plugintest.NewMockStreamResponseWriter()

		w.WriteJobResourceUtil(50.0, 100.0, 1024.0, 2048.0)

		if len(w.ResourceUtils) != 1 {
			t.Errorf("expected 1 resource util entry, got %d", len(w.ResourceUtils))
		}
	})

	t.Run("tracks close", func(t *testing.T) {
		w := plugintest.NewMockStreamResponseWriter()

		plugintest.AssertStreamNotClosed(t, w)

		w.Close()

		plugintest.AssertStreamClosed(t, w)
	})
}

func TestJobFilterBuilder(t *testing.T) {
	t.Run("builds filter with all fields", func(t *testing.T) {
		filter := plugintest.NewJobFilter().
			WithStatus(api.StatusRunning).
			WithStatus(api.StatusPending).
			WithTag("test").
			Build()

		if len(filter.Statuses) != 2 {
			t.Errorf("expected 2 statuses, got %d", len(filter.Statuses))
		}
		if len(filter.Tags) != 1 {
			t.Errorf("expected 1 tag, got %d", len(filter.Tags))
		}
	})

	t.Run("filter includes matching jobs", func(t *testing.T) {
		filter := plugintest.NewJobFilter().
			WithStatus(api.StatusRunning).
			Build()

		runningJob := plugintest.NewJob().Running().Build()
		pendingJob := plugintest.NewJob().Pending().Build()

		plugintest.AssertJobMatchesFilter(t, runningJob, filter)
		plugintest.AssertJobDoesNotMatchFilter(t, pendingJob, filter)
	})
}

func TestHelperFunctions(t *testing.T) {
	t.Run("FindJobByID finds correct job", func(t *testing.T) {
		jobs := []*api.Job{
			plugintest.NewJob().WithID("job-1").Build(),
			plugintest.NewJob().WithID("job-2").Build(),
			plugintest.NewJob().WithID("job-3").Build(),
		}

		found := plugintest.FindJobByID(jobs, "job-2")
		if found == nil {
			t.Fatal("expected to find job-2")
		}
		plugintest.AssertJobID(t, found, "job-2")
	})

	t.Run("FindJobsByStatus finds all matching", func(t *testing.T) {
		jobs := []*api.Job{
			plugintest.NewJob().WithID("job-1").Running().Build(),
			plugintest.NewJob().WithID("job-2").Pending().Build(),
			plugintest.NewJob().WithID("job-3").Running().Build(),
		}

		running := plugintest.FindJobsByStatus(jobs, api.StatusRunning)
		if len(running) != 2 {
			t.Errorf("expected 2 running jobs, got %d", len(running))
		}
	})

	t.Run("FindJobsByUser finds all matching", func(t *testing.T) {
		jobs := []*api.Job{
			plugintest.NewJob().WithID("job-1").WithUser("alice").Build(),
			plugintest.NewJob().WithID("job-2").WithUser("bob").Build(),
			plugintest.NewJob().WithID("job-3").WithUser("alice").Build(),
		}

		aliceJobs := plugintest.FindJobsByUser(jobs, "alice")
		if len(aliceJobs) != 2 {
			t.Errorf("expected 2 jobs for alice, got %d", len(aliceJobs))
		}
	})
}
