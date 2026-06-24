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
		// Close() blocks until the background goroutine sends on r.done, so
		// by the time Close() returns the goroutine has already exited — no
		// sleep needed.
	}

	after := runtime.NumGoroutine()
	if after > before {
		buf := make([]byte, 1<<20)
		// runtime.Stack fills buf and returns bytes written; slice to that length.
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
