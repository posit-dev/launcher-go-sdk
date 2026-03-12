package conformance_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/cache"
	"github.com/posit-dev/launcher-go-sdk/conformance"
	"github.com/posit-dev/launcher-go-sdk/launcher"
)

// testPlugin is a minimal in-memory plugin for testing the conformance suite
// itself. It uses the cache package to handle storage and streaming.
type testPlugin struct {
	cache   *cache.JobCache
	nextID  int32
	wg      sync.WaitGroup
	latency *launcher.Histogram
}

func newTestPlugin(t *testing.T) *testPlugin {
	t.Helper()
	lgr := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	c, err := cache.NewJobCache(context.Background(), lgr)
	if err != nil {
		t.Fatalf("failed to create job cache: %v", err)
	}
	tp := &testPlugin{
		cache:   c,
		latency: launcher.NewHistogram(launcher.ClusterInteractionLatencyBuckets),
	}
	t.Cleanup(func() {
		tp.wg.Wait()
		_ = c.Close()
	})
	return tp
}

func (p *testPlugin) SubmitJob(_ context.Context, w launcher.ResponseWriter, user string, job *api.Job) {
	id := api.JobID(fmt.Sprintf("job-%d", atomic.AddInt32(&p.nextID, 1)))
	now := time.Now().UTC()
	job.ID = id
	job.User = user
	job.Submitted = &now
	job.LastUpdated = &now
	job.Status = api.StatusPending

	if err := p.cache.AddOrUpdate(job); err != nil {
		w.WriteError(err)
		return
	}
	p.cache.WriteJob(w, user, id)

	longRunning := false
	for _, tag := range job.Tags {
		if tag == "long-running" {
			longRunning = true
			break
		}
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.simulateLifecycle(user, id, longRunning)
	}()
}

func (p *testPlugin) simulateLifecycle(user string, id api.JobID, longRunning bool) {
	time.Sleep(50 * time.Millisecond)
	p.cache.Update(user, id, func(j *api.Job) *api.Job {
		if j.Status == api.StatusPending {
			j.Status = api.StatusRunning
		}
		return j
	})

	if longRunning {
		return
	}

	time.Sleep(500 * time.Millisecond)
	p.cache.Update(user, id, func(j *api.Job) *api.Job {
		if j.Status == api.StatusRunning {
			exitCode := 0
			j.Status = api.StatusFinished
			j.ExitCode = &exitCode
		}
		return j
	})
}

func (p *testPlugin) GetJob(_ context.Context, w launcher.ResponseWriter, user string, id api.JobID, _ []string) {
	p.cache.WriteJob(w, user, id)
}

func (p *testPlugin) GetJobs(_ context.Context, w launcher.ResponseWriter, user string, filter *api.JobFilter, _ []string) {
	p.cache.WriteJobs(w, user, filter)
}

func (p *testPlugin) ControlJob(_ context.Context, w launcher.ResponseWriter, user string, id api.JobID, op api.JobOperation) {
	err := p.cache.Update(user, id, func(job *api.Job) *api.Job {
		if op.ValidForStatus() != job.Status {
			w.WriteErrorf(api.CodeInvalidJobState,
				"Job must be %s to %s it (current status: %s)",
				op.ValidForStatus(), op, job.Status)
			return job
		}

		switch op {
		case api.OperationStop:
			job.Status = api.StatusFinished
			exitCode := 143
			job.ExitCode = &exitCode
		case api.OperationKill:
			job.Status = api.StatusKilled
			exitCode := 137
			job.ExitCode = &exitCode
		case api.OperationCancel:
			job.Status = api.StatusCanceled
		case api.OperationSuspend, api.OperationResume:
			w.WriteErrorf(api.CodeRequestNotSupported,
				"Operation %s is not supported", op)
			return job
		}

		w.WriteControlJob(true, fmt.Sprintf("Job %s successful", op))
		return job
	})
	if err != nil {
		w.WriteError(err)
	}
}

func (p *testPlugin) GetJobStatus(ctx context.Context, w launcher.StreamResponseWriter, user string, id api.JobID) {
	p.cache.StreamJobStatus(ctx, w, user, id)
}

func (p *testPlugin) GetJobStatuses(ctx context.Context, w launcher.StreamResponseWriter, user string) {
	p.cache.StreamJobStatuses(ctx, w, user)
}

func (p *testPlugin) GetJobOutput(ctx context.Context, w launcher.StreamResponseWriter, user string, id api.JobID, outputType api.JobOutput) {
	err := p.cache.Lookup(user, id, func(_ *api.Job) {})
	if err != nil {
		w.WriteError(err)
		return
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer w.Close()
		for i := range 3 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				w.WriteJobOutput(fmt.Sprintf("line %d\n", i+1), outputType)
			}
		}
	}()
}

func (p *testPlugin) GetJobResourceUtil(ctx context.Context, _ launcher.StreamResponseWriter, user string, id api.JobID) {
	err := p.cache.Lookup(user, id, func(_ *api.Job) {})
	if err != nil {
		return
	}
	<-ctx.Done()
}

func (p *testPlugin) GetJobNetwork(_ context.Context, w launcher.ResponseWriter, user string, id api.JobID) {
	err := p.cache.Lookup(user, id, func(_ *api.Job) {
		hostname, _ := os.Hostname()
		w.WriteJobNetwork(hostname, []string{"127.0.0.1"})
	})
	if err != nil {
		w.WriteError(err)
	}
}

func (p *testPlugin) Metrics(_ context.Context) launcher.PluginMetrics {
	return launcher.PluginMetrics{
		ClusterInteractionLatency: p.latency.Drain(),
	}
}

func (p *testPlugin) ClusterInfo(_ context.Context, w launcher.ResponseWriter, _ string) {
	w.WriteClusterInfo(launcher.ClusterOptions{
		Queues:       []string{"default"},
		DefaultQueue: "default",
		Limits: []api.ResourceLimit{
			{Type: "cpuCount", Max: "8"},
			{Type: "memory", Max: "16GB"},
		},
	})
}

func testProfile() conformance.Profile {
	return conformance.Profile{
		JobFactory: func(user string) *api.Job {
			return &api.Job{
				User:    user,
				Name:    "conformance-test",
				Command: "echo hello",
			}
		},
		LongRunningJob: func(user string) *api.Job {
			return &api.Job{
				User:    user,
				Name:    "conformance-long",
				Command: "sleep 300",
				Tags:    []string{"long-running"},
			}
		},
		StopStatus:         api.StatusFinished,
		StopExitCodes:      []int{143},
		KillStatus:         api.StatusKilled,
		KillExitCodes:      []int{137},
		OutputAvailable:    true,
		NetworkAvailable:   true,
		JobStartTimeout:    5 * time.Second,
		JobCompleteTimeout: 10 * time.Second,
		OutputTimeout:      5 * time.Second,
	}
}

// Tier 1: Universal invariants.
func TestRun(t *testing.T) {
	p := newTestPlugin(t)
	conformance.Run(t, p, "testuser", testProfile())
}

// Tier 2: Product workflows.
func TestRunWorkflows(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunWorkflows(t, p, "testuser", testProfile())
}

// Tier 3: Individual scenarios.

func TestRunStopJob(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunStopJob(t, p, "testuser", conformance.StopOpts{
		Job: &api.Job{
			User:    "testuser",
			Command: "sleep 300",
			Tags:    []string{"long-running"},
		},
		ExpectStatus:    api.StatusFinished,
		ExpectExitCodes: []int{143},
		Timeout:         5 * time.Second,
	})
}

func TestRunKillJob(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunKillJob(t, p, "testuser", conformance.KillOpts{
		Job: &api.Job{
			User:    "testuser",
			Command: "sleep 300",
			Tags:    []string{"long-running"},
		},
		ExpectStatus:    api.StatusKilled,
		ExpectExitCodes: []int{137},
		Timeout:         5 * time.Second,
	})
}

func TestRunCancelJob(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunCancelJob(t, p, "testuser", conformance.CancelOpts{
		Job: &api.Job{
			User:    "testuser",
			Command: "sleep 300",
			Tags:    []string{"long-running"},
		},
		Timeout: 5 * time.Second,
	})
}

func TestRunOutputStream(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunOutputStream(t, p, "testuser", conformance.OutputStreamOpts{
		Job: &api.Job{
			User:    "testuser",
			Command: "echo hello",
		},
		OutputType:     api.OutputStdout,
		ExpectNonEmpty: true,
		Timeout:        5 * time.Second,
	})
}

func TestRunStatusStream(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunStatusStream(t, p, "testuser", conformance.StatusStreamOpts{
		Job: &api.Job{
			User:    "testuser",
			Command: "echo hello",
		},
		Timeout: 5 * time.Second,
	})
}

func TestRunStreamCancellation(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunStreamCancellation(t, p, "testuser", conformance.StreamCancelOpts{
		Job: &api.Job{
			User:    "testuser",
			Command: "echo hello",
		},
		Timeout: 5 * time.Second,
	})
}

func TestRunFieldFiltering(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunFieldFiltering(t, p, "testuser", conformance.FieldFilterOpts{
		Job: &api.Job{
			User:    "testuser",
			Name:    "field-filter-test",
			Command: "echo hello",
		},
		Fields: []string{"status", "name"},
	})
}

func TestRunMetrics(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunMetrics(t, p, conformance.MetricsOpts{
		Timeout: 2 * time.Second,
	})
}

func TestRunControlInvalidState(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunControlInvalidState(t, p, "testuser", conformance.InvalidStateOpts{
		Job: &api.Job{
			User:    "testuser",
			Command: "echo hello",
		},
		Operation: api.OperationStop,
		Timeout:   10 * time.Second,
	})
}
