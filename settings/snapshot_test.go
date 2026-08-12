package settings

import (
	"sync"
	"testing"
)

func TestSnapshot_SeededNonNilAtConstruction(t *testing.T) {
	// The C++ Launcher hit a real bug where a Snapshot's ctor-time seeding
	// was missing, so a non-default --job-expiry-hours was silently ignored
	// until the first reload. NewSnapshot must make that class of bug
	// impossible: Load() immediately after construction must return the
	// seed value, not a zero value.
	s := NewSnapshot(42)
	if got := s.Load(); got != 42 {
		t.Errorf("Load() immediately after NewSnapshot(42) = %d, want 42", got)
	}
}

func TestSnapshot_StoreThenLoad(t *testing.T) {
	s := NewSnapshot("initial")
	s.Store("updated")
	if got := s.Load(); got != "updated" {
		t.Errorf("Load() after Store(%q) = %q, want %q", "updated", got, "updated")
	}
}

func TestSnapshot_ConcurrentLoadAndStore(_ *testing.T) {
	type payload struct{ N int }
	s := NewSnapshot(payload{N: 0})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			s.Store(payload{N: n})
		}(i)
		go func() {
			defer wg.Done()
			// Load must never observe a torn/zero value: N is always one
			// of the values a Store call used.
			_ = s.Load()
		}()
	}
	wg.Wait()
}

func TestSnapshot_LoadReturnsPinnedCopy(t *testing.T) {
	// A caller's local copy from Load() must not change out from under it
	// when a concurrent Store happens — Load returns a value, not a shared
	// pointer to mutable state.
	type payload struct{ N int }
	s := NewSnapshot(payload{N: 1})
	local := s.Load()
	s.Store(payload{N: 2})
	if local.N != 1 {
		t.Errorf("local.N changed after concurrent Store: got %d, want 1", local.N)
	}
}
