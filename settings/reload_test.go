package settings

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestReloader_SeededJobExpiryAtConstruction(t *testing.T) {
	// Mirrors TestSnapshot_SeededNonNilAtConstruction one level up: a
	// non-default startup --job-expiry-hours must be visible immediately,
	// not only after the first reload.
	r := NewReloader(Registry, StaticOwnConfSource{}, testInherited(), 48, nil)

	hours, dur := r.JobExpiry()
	if hours != 48 {
		t.Errorf("JobExpiry() hours = %v, want 48", hours)
	}
	if want := time.Duration(48 * float64(time.Hour)); dur != want {
		t.Errorf("JobExpiry() duration = %v, want %v", dur, want)
	}
}

func TestReloader_AppliesJobExpiryChange(t *testing.T) {
	inherited := testInherited() // starts at 24 hours
	r := NewReloader(Registry, StaticOwnConfSource{}, inherited, 24, nil)

	pushed := inherited
	pushed.JobExpiryHours = 48
	report, err := r.Reload(context.Background(), &pushed, 1)
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	if hours, _ := r.JobExpiry(); hours != 48 {
		t.Errorf("JobExpiry() hours after reload = %v, want 48", hours)
	}
	if !containsString(report.Applied, "job-expiry-hours") {
		t.Errorf("Applied = %v, want to contain job-expiry-hours", report.Applied)
	}
}

func TestReloader_NoChangeMeansNotApplied(t *testing.T) {
	inherited := testInherited() // starts at 24 hours
	r := NewReloader(Registry, StaticOwnConfSource{}, inherited, 24, nil)

	report, err := r.Reload(context.Background(), &inherited, 1)
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if containsString(report.Applied, "job-expiry-hours") {
		t.Errorf("Applied = %v, want NOT to contain job-expiry-hours (value unchanged)", report.Applied)
	}
}

func TestReloader_InvalidJobExpiry_NoPartialApply(t *testing.T) {
	inherited := testInherited() // starts at 24 hours
	r := NewReloader(Registry, StaticOwnConfSource{}, inherited, 24, nil)

	pushed := inherited
	pushed.JobExpiryHours = -5
	_, err := r.Reload(context.Background(), &pushed, 1)
	if err == nil {
		t.Fatal("Reload() error = nil, want validation error for negative job-expiry-hours")
	}

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Errorf("error type = %T, want *ValidationError", err)
	}

	// Nothing must have been applied: the snapshot is untouched.
	if hours, _ := r.JobExpiry(); hours != 24 {
		t.Errorf("JobExpiry() hours after failed reload = %v, want unchanged 24", hours)
	}
}

func TestReloader_PresenceAwareCacheUpdate_AbsentPushDoesNotClobber(t *testing.T) {
	inherited := testInherited()
	inherited.ServerUser = "custom-user"
	r := NewReloader(Registry, StaticOwnConfSource{}, inherited, 24, nil)

	// An absent push (nil) must not reset the cached inherited settings to
	// a zero-valued struct - it must be treated as "the launcher had
	// nothing new to push this time", not "reset everything".
	_, err := r.Reload(context.Background(), nil, 2)
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	report, err := r.Reload(context.Background(), nil, 3)
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	// server-user is RestartRequired; if the cache had been clobbered to a
	// zero-valued InheritedSettings, ServerUser would resolve to "" here,
	// which differs from the startup baseline ("custom-user") and would
	// incorrectly show up as pendingRestart.
	if containsString(report.PendingRestart, "server-user") {
		t.Errorf("PendingRestart = %v, want NOT to contain server-user after an absent push", report.PendingRestart)
	}
}

func TestReloader_PendingRestart_BaselineIsStartupNotCached(t *testing.T) {
	startup := testInherited()
	startup.ServerUser = "original-user"
	r := NewReloader(Registry, StaticOwnConfSource{}, startup, 24, nil)

	// First push changes server-user away from the startup baseline.
	pushed1 := startup
	pushed1.ServerUser = "pushed-user"
	report1, err := r.Reload(context.Background(), &pushed1, 1)
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if !containsString(report1.PendingRestart, "server-user") {
		t.Errorf("PendingRestart after push 1 = %v, want to contain server-user", report1.PendingRestart)
	}

	// Second push restores the ORIGINAL startup value. Because the
	// baseline is the immutable startup layer (not the mutable cache from
	// push 1), this must be measured as "back to baseline", i.e. no longer
	// pending restart.
	pushed2 := startup
	pushed2.ServerUser = "original-user"
	report2, err := r.Reload(context.Background(), &pushed2, 2)
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if containsString(report2.PendingRestart, "server-user") {
		t.Errorf("PendingRestart after push 2 = %v, want NOT to contain server-user (restored to startup baseline)", report2.PendingRestart)
	}
}

func TestReloader_PendingRestart_OwnConfSourcedKeyComparedAgainstInheritedBaseline(t *testing.T) {
	// Mirrors a documented C++ nuance (SettingsReloadRoutine.cpp): the
	// pendingRestart baseline is built purely from startupInherited_ (the
	// cascade), never from a full own-conf-aware startup resolve. So an
	// own-conf-sourced RestartRequired setting is compared against the
	// plain inherited value, not against "what own-conf said at startup" -
	// own-conf's raw value ("own-conf-user") differs from the inherited
	// baseline ("rstudio-server" per testInherited()), so this reports
	// pendingRestart even though own-conf hasn't changed since startup.
	startup := testInherited()
	own := StaticOwnConfSource{"server-user": "own-conf-user"}
	r := NewReloader(Registry, own, startup, 24, nil)

	report, err := r.Reload(context.Background(), nil, 1)
	if err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if !containsString(report.PendingRestart, "server-user") {
		t.Errorf("PendingRestart = %v, want to contain server-user", report.PendingRestart)
	}
}

func TestReloader_ApplyExtra_CalledEveryReload(t *testing.T) {
	inherited := testInherited()
	r := NewReloader(Registry, StaticOwnConfSource{}, inherited, 24, nil)

	var calls int
	var mu sync.Mutex
	r.ApplyExtra = func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return nil
	}

	if _, err := r.Reload(context.Background(), nil, 1); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if _, err := r.Reload(context.Background(), nil, 2); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("ApplyExtra called %d times, want 2", calls)
	}
}

func TestReloader_ApplyExtra_ErrorIsNonFatal(t *testing.T) {
	inherited := testInherited()
	r := NewReloader(Registry, StaticOwnConfSource{}, inherited, 24, nil)
	r.ApplyExtra = func(context.Context) error {
		return errors.New("plugin-specific apply failed")
	}

	report, err := r.Reload(context.Background(), nil, 1)
	if err != nil {
		t.Errorf("Reload() error = %v, want nil (ApplyExtra failures are non-fatal)", err)
	}
	_ = report
}

func TestReloader_ConcurrentReloadsAreSerialized(_ *testing.T) {
	inherited := testInherited()
	r := NewReloader(Registry, StaticOwnConfSource{}, inherited, 24, nil)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			pushed := inherited
			pushed.JobExpiryHours = float64(n)
			_, _ = r.Reload(context.Background(), &pushed, uint(n)) //nolint:gosec // test loop index is small
		}(i)
	}
	wg.Wait()
	// No assertion beyond "the race detector didn't fire" - that's the
	// point of this test (run with -race).
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
