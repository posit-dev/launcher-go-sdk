package conformance

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/launcher"
	"github.com/posit-dev/launcher-go-sdk/plugintest"
)

// Profile describes the behavioral expectations for a plugin that will be
// used with Posit products (Workbench, Connect) through the Launcher.
//
// Posit products manage computational workloads (interactive sessions,
// content deployments, batch jobs) by submitting them as Launcher jobs.
// The typical workload lifecycle as seen by a plugin is:
//
//	ClusterInfo → SubmitJob → stream statuses → wait Running →
//	GetJobNetwork → stream output → ControlJob(Stop) → observe terminal
//
// Every field in this struct corresponds to a behavioral expectation that
// products have of the plugin. Fields with documented per-plugin deltas
// indicate where the three first-party plugins (Local, Kubernetes, Slurm)
// differ in behavior.
type Profile struct {
	// JobFactory returns a fresh, submittable job for the given user.
	// The returned job should complete on its own within JobCompleteTimeout
	// and produce output on stdout (unless OutputAvailable is false).
	//
	// This function is called multiple times; it must return a fresh job
	// each time. For Kubernetes plugins, include a container spec. For
	// Local plugins, a simple command suffices.
	//
	// Required.
	JobFactory func(user string) *api.Job

	// LongRunningJob returns a job that stays running indefinitely (e.g.,
	// sleep 300) for testing control operations like stop, kill, and
	// suspend. Interactive sessions are long-running by nature — they run
	// until the user stops them or they hit a timeout.
	//
	// Required.
	LongRunningJob func(user string) *api.Job

	// StopStatus is the terminal status after ControlJob(Stop).
	//
	// Products send Stop when the user ends a workload gracefully. The UI
	// waits for a terminal status on the status stream and displays it.
	//
	// Known plugin behaviors (from Launcher e2e test_general.py):
	//   - Local:      api.StatusFinished
	//   - Kubernetes: api.StatusFinished
	//   - Slurm:      api.StatusKilled  (scancel reports KILLED)
	//
	// Required.
	StopStatus string

	// StopExitCodes lists acceptable exit codes after a Stop operation.
	//
	// Known plugin behaviors:
	//   - Local:      []int{143}  (128 + SIGTERM)
	//   - Kubernetes: []int{143}  (128 + SIGTERM)
	//   - Slurm:      []int{143}  (128 + SIGTERM)
	//
	// Required.
	StopExitCodes []int

	// KillStatus is the terminal status after ControlJob(Kill).
	//
	// Products send Kill when the user force-terminates a workload.
	//
	// Known plugin behaviors:
	//   - All plugins: api.StatusKilled
	//
	// Required.
	KillStatus string

	// KillExitCodes lists acceptable exit codes after a Kill operation.
	//
	// Known plugin behaviors:
	//   - Local:      []int{137}        (128 + SIGKILL)
	//   - Kubernetes: []int{137}        (128 + SIGKILL)
	//   - Slurm:      []int{137, 143}   (SIGTERM sent before SIGKILL;
	//                                     wrapper script non-determinism)
	//
	// Required.
	KillExitCodes []int

	// OutputAvailable indicates whether GetJobOutput returns data.
	//
	// Products open an output stream to show workload logs. If output is
	// unavailable, the UI shows a placeholder message.
	//
	// Known plugin behaviors:
	//   - Local:      true
	//   - Kubernetes: true
	//   - Slurm:      false  (ParallelCluster does not return output)
	OutputAvailable bool

	// SuspendSupported indicates whether ControlJob(Suspend) and
	// ControlJob(Resume) are supported.
	//
	// Products show suspend/resume controls only when the cluster reports
	// suspend support.
	//
	// Known plugin behaviors:
	//   - Local:      depends on configuration
	//   - Kubernetes: false
	//   - Slurm:      depends on configuration
	SuspendSupported bool

	// NetworkAvailable indicates whether GetJobNetwork returns meaningful
	// host/IP data for running jobs.
	//
	// Products use this to route user connections to running workloads
	// (e.g., proxying to an RStudio or Jupyter session).
	//
	// All known plugins support this. Default: true.
	NetworkAvailable bool

	// PollInterval controls how frequently helpers poll GetJob when
	// waiting for status transitions. Default: 50ms.
	PollInterval time.Duration

	// JobStartTimeout is the maximum time to wait for a job to transition
	// from Pending to Running. Default: 30s.
	JobStartTimeout time.Duration

	// JobCompleteTimeout is the maximum time to wait for a job to reach
	// a terminal status on its own. Default: 60s.
	JobCompleteTimeout time.Duration

	// OutputTimeout is the maximum time to wait for output to appear on
	// an output stream. Default: 10s.
	OutputTimeout time.Duration
}

func (p *Profile) mustValidate(t *testing.T) {
	t.Helper()
	var missing []string
	if p.JobFactory == nil {
		missing = append(missing, "JobFactory")
	}
	if p.LongRunningJob == nil {
		missing = append(missing, "LongRunningJob")
	}
	if p.StopStatus == "" {
		missing = append(missing, "StopStatus")
	}
	if len(p.StopExitCodes) == 0 {
		missing = append(missing, "StopExitCodes")
	}
	if p.KillStatus == "" {
		missing = append(missing, "KillStatus")
	}
	if len(p.KillExitCodes) == 0 {
		missing = append(missing, "KillExitCodes")
	}
	if len(missing) > 0 {
		t.Fatalf("conformance: Profile is missing required fields: %s",
			strings.Join(missing, ", "))
	}
}

func (p *Profile) applyDefaults() {
	if p.PollInterval == 0 {
		p.PollInterval = 50 * time.Millisecond
	}
	if p.JobStartTimeout == 0 {
		p.JobStartTimeout = 30 * time.Second
	}
	if p.JobCompleteTimeout == 0 {
		p.JobCompleteTimeout = 60 * time.Second
	}
	if p.OutputTimeout == 0 {
		p.OutputTimeout = 10 * time.Second
	}
}

// Run executes universal invariant tests that hold for all correct plugins.
// These verify fundamental Launcher protocol requirements that every plugin
// must satisfy regardless of scheduler type.
//
// Tests are registered as subtests under Invariants/, making them individually
// addressable via go test -run:
//
//	go test -run TestConformance/Invariants/Submit/ReturnsNonEmptyID
func Run(t *testing.T, p launcher.Plugin, user string, profile Profile) {
	t.Helper()
	profile.mustValidate(t)
	profile.applyDefaults()

	t.Run("Invariants", func(t *testing.T) {
		t.Run("Submit", func(t *testing.T) {
			testSubmitInvariants(t, p, user, &profile)
		})
		t.Run("GetJob", func(t *testing.T) {
			testGetJobInvariants(t, p, user, &profile)
		})
		t.Run("GetJobs", func(t *testing.T) {
			testGetJobsInvariants(t, p, user, &profile)
		})
		t.Run("ClusterInfo", func(t *testing.T) {
			testClusterInfoInvariants(t, p, user)
		})
		t.Run("Lifecycle", func(t *testing.T) {
			testLifecycleInvariants(t, p, user, &profile)
		})
		t.Run("Errors", func(t *testing.T) {
			testErrorInvariants(t, p, user, &profile)
		})
	})
}

func testSubmitInvariants(t *testing.T, p launcher.Plugin, user string, profile *Profile) {
	t.Run("ReturnsNonEmptyID", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()
		job := profile.JobFactory(user)
		p.SubmitJob(context.Background(), w, user, job)

		plugintest.AssertNoError(t, w)
		plugintest.AssertJobCount(t, w, 1)

		submitted := w.LastJobs()[0]
		if submitted.ID == "" {
			t.Error("submitted job must have a non-empty ID")
		}
	})

	t.Run("SetsSubmittedTimestamp", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()
		job := profile.JobFactory(user)
		p.SubmitJob(context.Background(), w, user, job)

		plugintest.AssertNoError(t, w)
		submitted := w.LastJobs()[0]
		if submitted.Submitted == nil {
			t.Error("submitted job must have a non-nil Submitted timestamp")
		}
	})

	t.Run("TwoJobsGetDifferentIDs", func(t *testing.T) {
		w1 := plugintest.NewMockResponseWriter()
		p.SubmitJob(context.Background(), w1, user, profile.JobFactory(user))
		plugintest.AssertNoError(t, w1)

		w2 := plugintest.NewMockResponseWriter()
		p.SubmitJob(context.Background(), w2, user, profile.JobFactory(user))
		plugintest.AssertNoError(t, w2)

		id1 := w1.LastJobs()[0].ID
		id2 := w2.LastJobs()[0].ID
		if id1 == id2 {
			t.Errorf("two submitted jobs must have different IDs, both got %q", id1)
		}
	})

	t.Run("PreservesName", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()
		job := profile.JobFactory(user)
		job.Name = "conformance-name-test"
		p.SubmitJob(context.Background(), w, user, job)

		plugintest.AssertNoError(t, w)
		submitted := w.LastJobs()[0]
		if submitted.Name != "conformance-name-test" {
			t.Errorf("expected name %q, got %q", "conformance-name-test", submitted.Name)
		}
	})

	t.Run("PreservesTags", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()
		job := profile.JobFactory(user)
		job.Tags = []string{"conformance-tag-a", "conformance-tag-b"}
		p.SubmitJob(context.Background(), w, user, job)

		plugintest.AssertNoError(t, w)
		submitted := w.LastJobs()[0]
		plugintest.AssertJobHasTag(t, submitted, "conformance-tag-a")
		plugintest.AssertJobHasTag(t, submitted, "conformance-tag-b")
	})
}

func testGetJobInvariants(t *testing.T, p launcher.Plugin, user string, profile *Profile) {
	t.Run("ReturnsByID", func(t *testing.T) {
		id := SubmitJob(t, p, user, profile.JobFactory(user))

		job, apiErr := GetJob(p, user, id, nil)
		if apiErr != nil {
			t.Fatalf("GetJob returned error: %v", apiErr)
		}
		if job == nil {
			t.Fatal("GetJob returned nil job")
		}
		plugintest.AssertJobID(t, job, id)
	})

	t.Run("NotFoundForBogusID", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()
		p.GetJob(context.Background(), w, user, "nonexistent-conformance-id-99999", nil)
		plugintest.AssertErrorCode(t, w, api.CodeJobNotFound)
	})
}

func testGetJobsInvariants(t *testing.T, p launcher.Plugin, user string, profile *Profile) {
	t.Run("ReturnsSubmittedJobs", func(t *testing.T) {
		// Submit two jobs with known tags so we can find them.
		tag := fmt.Sprintf("conformance-get-jobs-%d", time.Now().UnixNano())

		job1 := profile.JobFactory(user)
		job1.Tags = append(job1.Tags, tag)
		id1 := SubmitJob(t, p, user, job1)

		job2 := profile.JobFactory(user)
		job2.Tags = append(job2.Tags, tag)
		id2 := SubmitJob(t, p, user, job2)

		jobs := GetJobs(p, user, &api.JobFilter{Tags: []string{tag}})
		if len(jobs) < 2 {
			t.Fatalf("expected at least 2 jobs with tag %q, got %d", tag, len(jobs))
		}

		found1 := plugintest.FindJobByID(jobs, id1)
		found2 := plugintest.FindJobByID(jobs, id2)
		if found1 == nil {
			t.Errorf("job %s not found in filtered results", id1)
		}
		if found2 == nil {
			t.Errorf("job %s not found in filtered results", id2)
		}
	})

	t.Run("FilterByTagANDLogic", func(t *testing.T) {
		tagA := fmt.Sprintf("conformance-and-a-%d", time.Now().UnixNano())
		tagB := fmt.Sprintf("conformance-and-b-%d", time.Now().UnixNano())

		// Job 1 has both tags.
		job1 := profile.JobFactory(user)
		job1.Tags = append(job1.Tags, tagA, tagB)
		id1 := SubmitJob(t, p, user, job1)

		// Job 2 has only tagA.
		job2 := profile.JobFactory(user)
		job2.Tags = append(job2.Tags, tagA)
		_ = SubmitJob(t, p, user, job2)

		// Filter for both tags — AND logic means only job1 should match.
		jobs := GetJobs(p, user, &api.JobFilter{Tags: []string{tagA, tagB}})
		for _, j := range jobs {
			plugintest.AssertJobHasTag(t, j, tagA)
			plugintest.AssertJobHasTag(t, j, tagB)
		}

		found1 := plugintest.FindJobByID(jobs, id1)
		if found1 == nil {
			t.Error("job with both tags was not found in AND-filtered results")
		}
	})

	t.Run("FilterByStatus", func(t *testing.T) {
		// This is a structural test: verify filtering by status doesn't
		// produce an error. Detailed status filtering is tested in the
		// workflow tests via WaitForStatus.
		w := plugintest.NewMockResponseWriter()
		filter := &api.JobFilter{Statuses: []string{api.StatusRunning}}
		p.GetJobs(context.Background(), w, user, filter, nil)
		plugintest.AssertNoError(t, w)
	})

	t.Run("UnmatchedFilterReturnsEmpty", func(t *testing.T) {
		tag := fmt.Sprintf("conformance-nomatch-%d", time.Now().UnixNano())
		jobs := GetJobs(p, user, &api.JobFilter{Tags: []string{tag}})
		if len(jobs) != 0 {
			t.Errorf("expected 0 jobs for unmatched tag filter, got %d", len(jobs))
		}
	})
}

func testClusterInfoInvariants(t *testing.T, p launcher.Plugin, user string) {
	t.Run("ReturnsWithoutError", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()
		p.ClusterInfo(context.Background(), w, user)
		plugintest.AssertNoError(t, w)
		if w.ClusterInfo == nil {
			t.Error("ClusterInfo must write cluster info")
		}
	})

	t.Run("LimitsHaveType", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()
		p.ClusterInfo(context.Background(), w, user)
		if w.ClusterInfo == nil {
			t.Skip("ClusterInfo not available")
		}
		for i, limit := range w.ClusterInfo.Limits {
			if limit.Type == "" {
				t.Errorf("resource limit at index %d has empty Type", i)
			}
		}
	})
}

func testLifecycleInvariants(t *testing.T, p launcher.Plugin, user string, profile *Profile) {
	t.Run("ReachesTerminalStatus", func(t *testing.T) {
		id := SubmitJob(t, p, user, profile.JobFactory(user))

		ctx, cancel := context.WithTimeout(context.Background(), profile.JobCompleteTimeout)
		defer cancel()

		job, err := WaitForTerminalStatus(ctx, p, user, id)
		if err != nil {
			t.Fatalf("job did not reach terminal status: %v", err)
		}
		if !api.TerminalStatus(job.Status) {
			t.Errorf("expected terminal status, got %q", job.Status)
		}
	})

	t.Run("TerminalStatusNeverRegresses", func(t *testing.T) {
		id := SubmitJob(t, p, user, profile.JobFactory(user))

		ctx, cancel := context.WithTimeout(context.Background(), profile.JobCompleteTimeout)
		defer cancel()

		job, err := WaitForTerminalStatus(ctx, p, user, id)
		if err != nil {
			t.Fatalf("job did not reach terminal status: %v", err)
		}

		terminalStatus := job.Status

		// Wait briefly and verify status hasn't changed.
		time.Sleep(200 * time.Millisecond)

		job2, apiErr := GetJob(p, user, id, nil)
		if apiErr != nil {
			t.Fatalf("GetJob returned error: %v", apiErr)
		}
		if job2.Status != terminalStatus {
			t.Errorf("terminal status regressed from %q to %q",
				terminalStatus, job2.Status)
		}
	})
}

func testErrorInvariants(t *testing.T, p launcher.Plugin, user string, _ *Profile) {
	t.Run("NotFoundUsesCodeJobNotFound", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()
		p.GetJob(context.Background(), w, user, "nonexistent-conformance-err-99999", nil)
		plugintest.AssertErrorCode(t, w, api.CodeJobNotFound)
	})

	t.Run("ControlNonexistentReturnsError", func(t *testing.T) {
		w := plugintest.NewMockResponseWriter()
		p.ControlJob(context.Background(), w, user, "nonexistent-conformance-ctrl-99999", api.OperationStop)
		plugintest.AssertError(t, w)
	})
}
