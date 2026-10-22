package main

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/cache"
	"github.com/posit-dev/launcher-go-sdk/conformance"
)

// testUser is the username that the conformance tests submit jobs as.
const testUser = "testuser"

func newTestPlugin(t *testing.T) *InMemoryPlugin {
	t.Helper()
	lgr := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	c, err := cache.NewJobCache(lgr)
	if err != nil {
		t.Fatalf("failed to create job cache: %v", err)
	}
	p := &InMemoryPlugin{cache: c}
	t.Cleanup(func() {
		p.wg.Wait()
		if err := c.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return p
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
	conformance.Run(t, p, testUser, testProfile())
}

// Tier 2: Product workflows.
func TestRunWorkflows(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunWorkflows(t, p, testUser, testProfile())
}

// Tier 3: Individual scenarios.

func TestRunStopJob(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunStopJob(t, p, testUser, conformance.StopOpts{
		Job:             testProfile().LongRunningJob(testUser),
		ExpectStatus:    api.StatusFinished,
		ExpectExitCodes: []int{143},
		Timeout:         5 * time.Second,
	})
}

func TestRunKillJob(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunKillJob(t, p, testUser, conformance.KillOpts{
		Job:             testProfile().LongRunningJob(testUser),
		ExpectStatus:    api.StatusKilled,
		ExpectExitCodes: []int{137},
		Timeout:         5 * time.Second,
	})
}

func TestRunCancelJob(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunCancelJob(t, p, testUser, conformance.CancelOpts{
		Job:     testProfile().LongRunningJob(testUser),
		Timeout: 5 * time.Second,
	})
}

func TestRunOutputStream(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunOutputStream(t, p, testUser, conformance.OutputStreamOpts{
		Job:            testProfile().JobFactory(testUser),
		OutputType:     api.OutputStdout,
		ExpectNonEmpty: true,
		Timeout:        5 * time.Second,
	})
}

func TestRunStatusStream(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunStatusStream(t, p, testUser, conformance.StatusStreamOpts{
		Job:     testProfile().JobFactory(testUser),
		Timeout: 5 * time.Second,
	})
}

func TestRunStreamCancellation(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunStreamCancellation(t, p, testUser, conformance.StreamCancelOpts{
		Job:     testProfile().JobFactory(testUser),
		Timeout: 5 * time.Second,
	})
}

func TestRunFieldFiltering(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunFieldFiltering(t, p, testUser, conformance.FieldFilterOpts{
		Job:    testProfile().JobFactory(testUser),
		Fields: []string{"status", "name"},
	})
}

func TestRunControlInvalidState(t *testing.T) {
	p := newTestPlugin(t)
	conformance.RunControlInvalidState(t, p, testUser, conformance.InvalidStateOpts{
		Job:       testProfile().JobFactory(testUser),
		Operation: api.OperationStop,
		Timeout:   10 * time.Second,
	})
}
