package conformance

import (
	"context"
	"strconv"
	"testing"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/internal/reloaddispatch"
	"github.com/posit-dev/launcher-go-sdk/launcher"
	"github.com/posit-dev/launcher-go-sdk/settings"
)

// wantReloadable is the exact set of dual-homed setting keys the Launcher
// and Go SDK agree are Reloadable (can be applied to an already-running
// plugin without a restart). This mirrors the C++ launcher's registry
// tables (launcher/SettingsRegistry.hpp) and is fixed today: see
// [settings.Registry].
var wantReloadable = []string{"job-expiry-hours", "logging-dir", "enable-debug-logging"}

// wantRestartRequired is the exact set of dual-homed setting keys that can
// only take effect on the plugin's next start. See wantReloadable.
var wantRestartRequired = []string{"server-user", "scratch-path", "heartbeat-interval-seconds", "plugin-metrics-interval-seconds"}

// RunSettingsRegistry pins the [settings.Registry] every Go SDK plugin
// links against to the exact key strings and Reloadable/RestartRequired
// classification the Launcher expects, so that a future accidental change
// to the SDK's registry (a renamed key, a flipped classification) fails a
// test here instead of silently producing a plugin that reports the wrong
// thing to the Launcher on reload.
//
// Unlike the other Run* functions in this package, RunSettingsRegistry does
// not take a plugin: [settings.Registry] is a package-level table shared by
// every Go SDK plugin, not something an individual plugin customizes.
func RunSettingsRegistry(t *testing.T) {
	t.Helper()

	got := make(map[string]settings.ReloadClass, len(settings.Registry))
	for _, d := range settings.Registry {
		got[d.Key] = d.ReloadClass
		if !d.DualHomed {
			t.Errorf("registry entry %q has DualHomed = false, want true - every entry in settings.Registry is dual-homed today", d.Key)
		}
	}

	if len(got) != len(wantReloadable)+len(wantRestartRequired) {
		t.Fatalf("settings.Registry has %d entries, want %d (%d Reloadable + %d RestartRequired)",
			len(got), len(wantReloadable)+len(wantRestartRequired), len(wantReloadable), len(wantRestartRequired))
	}

	for _, key := range wantReloadable {
		class, ok := got[key]
		if !ok {
			t.Errorf("settings.Registry is missing key %q (want Reloadable)", key)
			continue
		}
		if class != settings.Reloadable {
			t.Errorf("settings.Registry[%q].ReloadClass = %v, want Reloadable", key, class)
		}
	}
	for _, key := range wantRestartRequired {
		class, ok := got[key]
		if !ok {
			t.Errorf("settings.Registry is missing key %q (want RestartRequired)", key)
			continue
		}
		if class != settings.RestartRequired {
			t.Errorf("settings.Registry[%q].ReloadClass = %v, want RestartRequired", key, class)
		}
	}
}

// ReloadOpts configures [RunReload].
type ReloadOpts struct {
	// StartupInherited must be the exact api.InheritedSettings value the
	// plugin under test's *settings.Reloader was constructed with (the
	// seedInherited argument to [settings.NewReloader]). RunReload diffs
	// mutated copies of this value against it to drive reload scenarios; a
	// value that does not match what the plugin's Reloader was actually
	// seeded with will produce false failures in
	// AppliedAndPendingRestartClassification and
	// AbsentInheritedSettingsDoesNotClobberCache, since both rely on the
	// Reloader's own immutable startup baseline (see the settings package
	// doc) equaling this value.
	//
	// Both of those subtests are own-conf-aware: they recompute their
	// expectations from the plugin's actual resolved values
	// (reloader.LastResolved(), read after the reload) rather than assuming
	// every RestartRequired key is controlled by the pushed
	// InheritedSettings. A key the plugin's own conf legitimately sets is
	// own-conf's to win regardless of what is pushed - that is correct
	// dual-homed-settings behavior, not a plugin defect, so RunReload does
	// not fail a plugin for it.
	//
	// Required for every subtest that needs [launcher.SettingsReloadablePlugin];
	// those subtests are skipped, not failed, when p does not implement it.
	StartupInherited api.InheritedSettings
}

// RunReload verifies a plugin's config-reload wiring: that it declares
// reload support the way it intends to (or correctly reports
// [api.ReloadErrorRequestNotSupported] if it does not support reload at
// all), and — for a plugin implementing [launcher.SettingsReloadablePlugin]
// — that resolving a reload against its real *settings.Reloader classifies
// Reloadable and RestartRequired settings correctly, rejects an invalid
// push without partially applying it, and does not let an absent
// InheritedSettings push clobber previously-cached values.
//
// Every check drives the plugin through the same internal dispatch logic
// (internal/reloaddispatch) a real Launcher's ConfigReloadRequest triggers
// via [launcher.Runtime], so
// this exercises the plugin's actual reload behavior end to end rather than
// re-testing package settings' own internals (see that package's tests for
// Reloader-level coverage). RunReload does not verify runtime application
// of logging-dir or enable-debug-logging: the Go SDK's logger package has
// no runtime reconfiguration surface, so the SDK never reports those two
// Reloadable keys as applied even though the registry classifies them
// Reloadable — that gap is a known, currently-unclosed SDK limitation, not
// something a plugin author's own code can fix.
//
// Each subtest that needs a SettingsReloadablePlugin issues real reload
// calls against the plugin's actual Reloader and so mutates its persistent
// state (cached inherited settings, job-expiry snapshot); pass a plugin
// instance dedicated to this call, not one a caller also depends on holding
// a particular reload state afterward.
func RunReload(t *testing.T, p launcher.Plugin, opts ReloadOpts) {
	t.Helper()

	_, hasSettings := p.(launcher.SettingsReloadablePlugin)
	_, hasConfig := p.(launcher.ConfigReloadablePlugin)

	t.Run("Reload", func(t *testing.T) {
		t.Run("RequestNotSupportedWhenNoReloadInterfaceImplemented", func(t *testing.T) {
			if hasSettings || hasConfig {
				t.Skip("plugin implements a reload interface; this check only applies to a plugin implementing neither")
			}

			errType, errMsg, applied, pendingRestart, generation := reloaddispatch.Handle(context.Background(), p, nil, 42)

			if errType != api.ReloadErrorRequestNotSupported {
				t.Errorf("errorType = %v, want RequestNotSupported - a plugin implementing neither ConfigReloadablePlugin nor SettingsReloadablePlugin must never report a reload as successful", errType)
			}
			if errMsg == "" {
				t.Error("errorMessage is empty, want an explanation that reload is unsupported")
			}
			if applied != nil {
				t.Errorf("applied = %v, want nil", applied)
			}
			if pendingRestart != nil {
				t.Errorf("pendingRestart = %v, want nil", pendingRestart)
			}
			if generation != 42 {
				t.Errorf("echoed generation = %d, want 42 - the Launcher correlates a reload response to its request by generation even on this error path", generation)
			}
		})

		t.Run("AppliedAndPendingRestartClassification", func(t *testing.T) {
			srPlugin, ok := p.(launcher.SettingsReloadablePlugin)
			if !ok {
				t.Skip("plugin does not implement SettingsReloadablePlugin; applied/pendingRestart classification only applies to it")
			}
			reloader := srPlugin.SettingsReloader()
			if reloader == nil {
				t.Fatal("SettingsReloader() returned nil; construct a *settings.Reloader before calling RunReload")
			}

			// Compute the pushed job-expiry-hours relative to whatever the
			// Reloader currently holds, not opts.StartupInherited's
			// original value, so this subtest's result does not depend on
			// whether an earlier reload in this same test process already
			// changed it.
			beforeHours, _ := reloader.JobExpiry()

			pushed := opts.StartupInherited
			pushed.ServerUser += "-conformance-changed"
			pushed.ScratchPath += "-conformance-changed"
			pushed.LoggingDir += "-conformance-changed"
			pushed.HeartbeatIntervalSeconds++
			pushed.PluginMetricsIntervalSeconds++
			pushed.EnableDebugLogging = !opts.StartupInherited.EnableDebugLogging
			pushed.JobExpiryHours = beforeHours + 1

			errType, errMsg, applied, pendingRestart, generation := reloaddispatch.Handle(context.Background(), p, &pushed, 7)
			if errType != api.ReloadErrorNone {
				t.Fatalf("errorType = %v (%s), want None - construct ReloadOpts.StartupInherited from values the Reloader accepts", errType, errMsg)
			}
			if generation != 7 {
				t.Errorf("echoed generation = %d, want 7", generation)
			}

			// Recompute expectations from what the plugin's Reloader
			// actually resolved, rather than assuming every RestartRequired
			// key is controlled by the pushed InheritedSettings: if the
			// plugin's own conf legitimately sets one of these keys,
			// own-conf wins regardless of what was just pushed, and that
			// is correct behavior, not a defect to fail on.
			resolved := reloader.LastResolved()
			if resolved == nil {
				t.Fatal("reloader.LastResolved() is nil immediately after a successful reload")
			}

			if rv, ok := resolved["job-expiry-hours"]; ok {
				parsed, err := strconv.ParseFloat(rv.Raw, 64)
				wantApplied := err == nil && parsed != beforeHours
				gotApplied := containsKey(applied, "job-expiry-hours")
				if wantApplied != gotApplied {
					t.Errorf("applied contains job-expiry-hours = %v, want %v (resolved raw %q, provenance %v, value before this reload %v)",
						gotApplied, wantApplied, rv.Raw, rv.Provenance, beforeHours)
				}
			}

			baseline := settings.RawByKey(opts.StartupInherited)
			for _, key := range wantRestartRequired {
				rv, ok := resolved[key]
				if !ok {
					continue
				}
				wantPending := rv.Raw != baseline[key]
				gotPending := containsKey(pendingRestart, key)
				if wantPending != gotPending {
					t.Errorf("pendingRestart contains %q = %v, want %v (resolved raw %q vs startup baseline raw %q, provenance %v) - if this plugin's own conf legitimately sets %[1]q, own-conf wins over the pushed value, which is correct; verify ReloadOpts.StartupInherited still matches what settings.NewReloader was actually seeded with",
						key, gotPending, wantPending, rv.Raw, baseline[key], rv.Provenance)
				}
			}
			for _, key := range pendingRestart {
				if !containsKey(wantRestartRequired, key) {
					t.Errorf("pendingRestart contains %q, want only keys from %v", key, wantRestartRequired)
				}
			}
		})

		t.Run("NoPartialApplyOnValidationFailure", func(t *testing.T) {
			srPlugin, ok := p.(launcher.SettingsReloadablePlugin)
			if !ok {
				t.Skip("plugin does not implement SettingsReloadablePlugin; validation-failure semantics only apply to it")
			}
			reloader := srPlugin.SettingsReloader()
			if reloader == nil {
				t.Fatal("SettingsReloader() returned nil; construct a *settings.Reloader before calling RunReload")
			}

			// NOTE: like AppliedAndPendingRestartClassification above, this
			// subtest assumes job-expiry-hours is not itself own-conf-owned
			// for this plugin - if it is, own-conf's value wins regardless
			// of the -1 pushed here, validation never runs against it, and
			// the Fatalf below will misleadingly read as a broken plugin
			// rather than "this key is legitimately own-conf-controlled."
			beforeHours, beforeDuration := reloader.JobExpiry()

			invalid := opts.StartupInherited
			invalid.JobExpiryHours = -1

			errType, errMsg, applied, pendingRestart, _ := reloaddispatch.Handle(context.Background(), p, &invalid, 1)
			if errType != api.ReloadErrorValidate {
				t.Fatalf("errorType = %v (%s), want Validate for a negative job-expiry-hours", errType, errMsg)
			}
			if errMsg == "" {
				t.Error("errorMessage is empty, want a validation message")
			}
			if applied != nil || pendingRestart != nil {
				t.Errorf("applied = %v, pendingRestart = %v, want both nil - a rejected reload must not report a partial apply", applied, pendingRestart)
			}

			afterHours, afterDuration := reloader.JobExpiry()
			if afterHours != beforeHours || afterDuration != beforeDuration {
				t.Errorf("JobExpiry() changed after a rejected reload: before (%v, %v), after (%v, %v) - a validation failure must leave the Reloader's state exactly as it was", beforeHours, beforeDuration, afterHours, afterDuration)
			}
		})

		t.Run("AbsentInheritedSettingsDoesNotClobberCache", func(t *testing.T) {
			srPlugin, ok := p.(launcher.SettingsReloadablePlugin)
			if !ok {
				t.Skip("plugin does not implement SettingsReloadablePlugin; presence-aware caching only applies to it")
			}
			reloader := srPlugin.SettingsReloader()
			if reloader == nil {
				t.Fatal("SettingsReloader() returned nil; construct a *settings.Reloader before calling RunReload")
			}

			pushed := opts.StartupInherited
			pushed.ServerUser += "-conformance-cached-user"
			pushed.ScratchPath += "-conformance-cached-path"
			pushed.HeartbeatIntervalSeconds++
			pushed.PluginMetricsIntervalSeconds++

			errType, errMsg, _, pendingRestart1, _ := reloaddispatch.Handle(context.Background(), p, &pushed, 1)
			if errType != api.ReloadErrorNone {
				t.Fatalf("setup reload errorType = %v (%s), want None", errType, errMsg)
			}

			// Find a RestartRequired key this plugin's own conf does not
			// own (provenance Inherited) and whose resolved value actually
			// moved away from the startup baseline, so we have something
			// genuinely pending to probe cache persistence with. A plugin
			// whose own conf sets every RestartRequired key cannot be
			// probed this way - own-conf always wins regardless of what is
			// pushed, so nothing here would ever be cache-dependent for
			// such a plugin.
			resolved := reloader.LastResolved()
			baseline := settings.RawByKey(opts.StartupInherited)
			var probeKey string
			for _, key := range wantRestartRequired {
				if rv, ok := resolved[key]; ok && rv.Provenance == settings.ProvenanceInherited && rv.Raw != baseline[key] {
					probeKey = key
					break
				}
			}
			if probeKey == "" {
				t.Skip("no RestartRequired key resolved via inherited provenance with a value different from the startup baseline; cannot probe cache persistence (this plugin's own conf may set all of them)")
			}

			if !containsKey(pendingRestart1, probeKey) {
				t.Fatalf("setup reload pendingRestart = %v, want to contain %q (this subtest's precondition failed)", pendingRestart1, probeKey)
			}

			// A reload with no InheritedSettings at all (e.g. a
			// plugin-internal or signal-triggered reload, or an older
			// Launcher that never populates the field) must not clobber
			// the cached value with a zero-valued struct - doing so would
			// make it disagree with the startup baseline again for the
			// wrong reason and silently drop probeKey from pendingRestart.
			_, _, _, pendingRestart2, _ := reloaddispatch.Handle(context.Background(), p, nil, 2)
			if !containsKey(pendingRestart2, probeKey) {
				t.Errorf("reload with nil InheritedSettings: pendingRestart = %v, want to still contain %q - an absent push must not reset the cached value", pendingRestart2, probeKey)
			}
		})
	})
}

func containsKey(haystack []string, key string) bool {
	for _, k := range haystack {
		if k == key {
			return true
		}
	}
	return false
}
