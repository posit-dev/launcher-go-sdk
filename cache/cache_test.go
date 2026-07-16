package cache

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
)

// TestCloseStopsGoroutine verifies that Close() fully stops the cache's
// internal goroutine without leaking it. Regression test for #18, where the
// goroutine could block forever sending on an unbuffered done channel.
func TestCloseStopsGoroutine(t *testing.T) {
	const n = 5
	lgr := slog.New(slog.DiscardHandler)
	before := runtime.NumGoroutine()

	for range n {
		c, err := NewJobCache(lgr)
		if err != nil {
			t.Fatalf("NewJobCache: %v", err)
		}
		if err := c.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	// Close() unblocks once the goroutine calls close(r.done), but the goroutine
	// may still be counted by runtime.NumGoroutine() for a brief window while the
	// scheduler finalizes it. Poll until the count settles or 100ms elapses.
	var after int
	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		after = runtime.NumGoroutine()
		if after <= before || time.Now().After(deadline) {
			break
		}
		runtime.Gosched()
	}
	if after > before {
		buf := make([]byte, 1<<20)
		t.Errorf("goroutine leak: count went from %d to %d\n%s",
			before, after, buf[:runtime.Stack(buf, true)])
	}
}

// TestNotifyCloseConcurrent exercises the TOCTOU window in notify(): goroutines
// call AddOrUpdate continuously while Close() runs, verifying no panic occurs.
// This tests the <-r.stop guard in notify().
func TestNotifyCloseConcurrent(t *testing.T) {
	lgr := slog.New(slog.DiscardHandler)

	for range 3 {
		c, err := NewJobCache(lgr)
		if err != nil {
			t.Fatalf("NewJobCache: %v", err)
		}
		job := &api.Job{ID: "job-1", User: "alice", Status: api.StatusRunning}
		if err := c.AddOrUpdate(job); err != nil {
			t.Fatalf("AddOrUpdate: %v", err)
		}

		var wg sync.WaitGroup
		stop := make(chan struct{})
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
						_ = c.AddOrUpdate(job)
					}
				}
			}()
		}

		time.Sleep(5 * time.Millisecond)
		close(stop)
		// Call Close() while writers may still be running so the <-r.stop
		// guard in notify() is exercised concurrently.
		if err := c.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		wg.Wait()
	}
}

// TestSubscribeAfterClose verifies that subscribeToID and subscribeToUser
// close the caller's channel immediately rather than leaving it stranded.
func TestSubscribeAfterClose(t *testing.T) {
	lgr := slog.New(slog.DiscardHandler)
	c, err := NewJobCache(lgr)
	if err != nil {
		t.Fatalf("NewJobCache: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ctx := context.Background()

	chID := make(chan *statusUpdate, 1)
	c.subscribeToID(ctx, "job-1", chID)
	select {
	case _, ok := <-chID:
		if ok {
			t.Error("subscribeToID: expected channel to be closed, got value")
		}
	default:
		t.Error("subscribeToID: channel not immediately closed after shutdown")
	}

	chUser := make(chan *statusUpdate, 1)
	c.subscribeToUser(ctx, "alice", chUser)
	select {
	case _, ok := <-chUser:
		if ok {
			t.Error("subscribeToUser: expected channel to be closed, got value")
		}
	default:
		t.Error("subscribeToUser: channel not immediately closed after shutdown")
	}
}

// TestSubscriberCanceledContextCloseGuard verifies that subManager.Close() does
// not double-close a subscriber channel that Notify() already closed when it
// detected a canceled context.
func TestSubscriberCanceledContextCloseGuard(t *testing.T) {
	lgr := slog.New(slog.DiscardHandler)
	c, err := NewJobCache(lgr)
	if err != nil {
		t.Fatalf("NewJobCache: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *statusUpdate, 1)
	c.subscribeToID(ctx, "job-1", ch)

	// Cancel the subscriber's context so Notify() will close the channel.
	cancel()

	// Trigger a Notify() pass — Notify sees the canceled context, sets
	// sub.closed=true, and closes ch.
	job := &api.Job{ID: "job-1", User: "alice", Status: api.StatusRunning}
	if err := c.AddOrUpdate(job); err != nil {
		t.Fatalf("AddOrUpdate: %v", err)
	}

	// Give the background goroutine time to process the update.
	time.Sleep(20 * time.Millisecond)

	// Close() must not panic with "close of closed channel".
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestJobsForUserPreservesSessionTags verifies that a session-association
// query returns the job with its tags intact. rserver's getSessionJob filters
// by tags + "Running" status with an empty field projection; the Launcher
// protocol guarantees only "id" under a projection, but an empty projection
// must return the full job (tags included) so rserver can correlate the job to
// its session. Regression guard for session startup failing with
// "noJobForSession".
func TestJobsForUserPreservesSessionTags(t *testing.T) {
	sessionID := "s12345678904321"
	job := &api.Job{
		ID:     "job-1",
		User:   "alice",
		Status: api.StatusRunning,
		Tags:   []string{sessionID, "rstudio-r-session-id:" + sessionID},
	}

	tests := []struct {
		name   string
		filter *api.JobFilter
	}{
		{
			// Mirrors LauncherClient::getSessionJob: tags + Running, no fields.
			name: "empty fields (getSessionJob)",
			filter: &api.JobFilter{
				Tags:     []string{sessionID},
				Statuses: []string{api.StatusRunning},
			},
		},
		{
			// Mirrors getActiveLauncherSessions: an explicit projection that
			// includes tags.
			name: "projection including tags",
			filter: &api.JobFilter{
				Tags:     []string{sessionID},
				Statuses: []string{api.StatusRunning},
				Fields:   []string{"user", "submissionTime", "pid", "tags"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newInMemoryStore()
			if _, err := store.Update(string(job.ID), func(*api.Job) *api.Job {
				return job
			}); err != nil {
				t.Fatalf("Update: %v", err)
			}

			var got []*api.Job
			if err := store.JobsForUser("alice", tt.filter, func(jobs []*api.Job) {
				got = jobs
			}); err != nil {
				t.Fatalf("JobsForUser: %v", err)
			}

			if len(got) != 1 {
				t.Fatalf("JobsForUser returned %d jobs, want 1", len(got))
			}
			if len(got[0].Tags) != len(job.Tags) {
				t.Fatalf("returned tags = %v, want %v", got[0].Tags, job.Tags)
			}
			for i, tag := range job.Tags {
				if got[0].Tags[i] != tag {
					t.Errorf("tags[%d] = %q, want %q", i, got[0].Tags[i], tag)
				}
			}
		})
	}
}

// TestDoubleCloseIsNoOp verifies that a second call to Close() is safe.
func TestDoubleCloseIsNoOp(t *testing.T) {
	lgr := slog.New(slog.DiscardHandler)
	c, err := NewJobCache(lgr)
	if err != nil {
		t.Fatalf("NewJobCache: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
