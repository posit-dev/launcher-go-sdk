package settings

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
)

// ValidationError is returned by [Reloader.Reload] when a resolved setting
// fails validation (e.g. a negative job-expiry-hours). Mirrors the C++
// reload routine's guarantee: on a ValidationError, nothing was applied and
// the Reloader's state (including its job-expiry snapshot) is unchanged.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// Report describes the outcome of one call to [Reloader.Reload]: which
// Reloadable settings were actually applied, and which RestartRequired
// settings changed but can only take effect on the next plugin start.
type Report struct {
	// Applied lists the keys of Reloadable settings whose resolved value
	// differed from what was running and was applied live.
	Applied []string
	// PendingRestart lists the keys of RestartRequired settings whose
	// resolved value differs from the plugin's startup baseline. These are
	// reported, never applied — they can only take effect on the plugin's
	// next start.
	PendingRestart []string
}

// JobExpiry is the resolved state of the job-expiry-hours setting, held in
// a [Snapshot] inside [Reloader].
type JobExpiry struct {
	// Hours is the raw resolved value, in hours.
	Hours float64
	// Duration is Hours converted to a time.Duration, matching
	// launcher.DefaultOptions.JobExpiry's conversion
	// (time.Duration(float64(time.Hour) * hours)).
	Duration time.Duration
}

// Reloader resolves and applies the Launcher's dual-homed [server] settings
// on each config reload, and reports the outcome. It mirrors the C++
// impls::SettingsReloadRoutine (impls/SettingsReloadRoutine.cpp): resolve →
// validate all → swap → apply → report, serialized under one mutex, with no
// partial apply on a validation failure.
//
// Construct with [NewReloader]. The zero value is not ready for use.
type Reloader struct {
	// ApplyExtra, if set, is called on every successful reload (after
	// job-expiry-hours has been resolved and applied), for plugin-specific
	// apply side effects the SDK cannot know about (e.g. reloading resource
	// or user profiles) — the same role bindings_.applyExtra plays in the
	// C++ reload routine. A failure is logged (always — see [NewReloader]'s
	// lgr parameter) but never propagated: it does not fail the reload or
	// affect Applied/PendingRestart, which describe only the dual-homed
	// [server] settings.
	//
	// Timing contract: set this once, before the Reloader's first Reload
	// call (typically right after [NewReloader] returns, before the
	// Reloader is wired into a running plugin). Reload reads this field
	// under its internal mutex, but nothing synchronizes a concurrent
	// write to it against that read - assigning it after reloads may
	// already be in flight is a data race.
	ApplyExtra func(ctx context.Context) error

	mu sync.Mutex

	registry []SettingDescriptor
	ownConf  OwnConfSource
	lgr      *slog.Logger

	// startupInherited is the immutable startup baseline used for
	// PendingRestart comparisons. cachedInherited is the mutable,
	// presence-aware cache updated by IPC pushes. Splitting these mirrors
	// the C++ startupInherited_/cachedInherited_ split: a RestartRequired
	// setting's running value never moves without a restart, so a push
	// that changes it - or later restores it - must be measured against
	// startup, never against the previous push.
	startupInherited api.InheritedSettings
	cachedInherited  api.InheritedSettings

	jobExpiry *Snapshot[JobExpiry]

	lastResolved ResolvedSettings
}

// NewReloader creates a Reloader for the given registry, own-conf source,
// and startup state. seedInherited is the InheritedSettings this plugin
// instance was actually started with (both the initial cache and the
// immutable startup baseline) — see [launcher.DefaultOptions.InheritedSettings]
// for a helper that builds this from the plugin's own parsed options;
// seedJobExpiryHours is the job-expiry-hours value startup resolved
// (typically [launcher.DefaultOptions.JobExpiryHours]). lgr receives
// non-fatal error logs (e.g. ApplyExtra failures, or an own-conf read/parse
// failure from an [OwnConfSource] like [IniOwnConfSource]); pass nil to use
// [slog.Default] rather than to disable logging — both of those failure
// paths are logged unconditionally, matching the C++ reload routine, which
// always LOG_ERRORs them and has no "no logger" mode.
//
// The job-expiry snapshot is seeded immediately from seedJobExpiryHours —
// callers must not rely on a first Reload() call to make a non-default
// startup value visible; see [Snapshot]'s seeding requirement.
func NewReloader(registry []SettingDescriptor, ownConf OwnConfSource, seedInherited api.InheritedSettings, seedJobExpiryHours float64, lgr *slog.Logger) *Reloader {
	if lgr == nil {
		lgr = slog.Default()
	}
	return &Reloader{
		registry:         registry,
		ownConf:          ownConf,
		lgr:              lgr,
		startupInherited: seedInherited,
		cachedInherited:  seedInherited,
		jobExpiry: NewSnapshot(JobExpiry{
			Hours:    seedJobExpiryHours,
			Duration: time.Duration(float64(time.Hour) * seedJobExpiryHours),
		}),
	}
}

// JobExpiry returns the currently-resolved job-expiry-hours value, both as
// raw hours and as a time.Duration. Plugin business logic should call this
// instead of caching launcher.DefaultOptions.JobExpiry at startup, so it
// observes reloads.
func (r *Reloader) JobExpiry() (hours float64, duration time.Duration) {
	je := r.jobExpiry.Load()
	return je.Hours, je.Duration
}

// LastResolved returns a copy of the [ResolvedSettings] computed by the most
// recent [Reloader.Reload] call (or nil if Reload has never been called).
// This is informational: it includes settings (like logging-dir and
// enable-debug-logging) that Reload resolves but does not itself apply — see
// the package doc and this repo's task18b report for why.
func (r *Reloader) LastResolved() ResolvedSettings {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastResolved == nil {
		return nil
	}
	out := make(ResolvedSettings, len(r.lastResolved))
	for k, v := range r.lastResolved {
		out[k] = v
	}
	return out
}

// Reload resolves every setting in the registry, validates the Reloadable
// ones, and — only if validation passes — applies job-expiry-hours (if
// changed), runs ApplyExtra, and reports which RestartRequired settings
// differ from the startup baseline. The whole sequence is serialized under
// one mutex, so a direct signal-triggered reload and an IPC-triggered
// reload (if a plugin author wires both to the same Reloader) can never
// interleave.
//
// pushed carries the Launcher's dual-homed settings for this reload, or nil
// if the Launcher had nothing to push this time (e.g. a plugin-internal
// reload trigger, or an older Launcher that never populates the field).
// pushed's presence-aware update of the cached inherited settings happens
// under the same lock acquisition as the resolve that follows, so there is
// no gap in which another caller could observe an inconsistent cache/resolve
// pair.
//
// On a validation error, Reload returns a *[ValidationError] and leaves all
// state (including the job-expiry snapshot) exactly as it was — no partial
// apply.
//
// Reload does not itself take a generation parameter: the Launcher's
// request generation is a wire-protocol concern (see
// internal/reloaddispatch.Handle, which echoes it back verbatim), not
// something the resolve/validate/apply/report sequence here needs. The C++
// reload routine's equivalent value tags the .active last-known-good
// artifact it writes after every reload (SettingsReloadRoutine.cpp); the Go
// SDK writes no such artifact (see the package doc and docs/GUIDE.md's
// known limitations), so there is currently nothing for a generation
// parameter here to do.
func (r *Reloader) Reload(ctx context.Context, pushed *api.InheritedSettings) (Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Presence-aware cache update, performed first so the update and the
	// resolve below are one consistent critical section.
	if pushed != nil {
		r.cachedInherited = *pushed
	}
	inherited := r.cachedInherited

	ownPresent := map[string]string{}
	if r.ownConf != nil {
		ownPresent = r.ownConf.OwnConfKeys()
	}

	resolved := Resolve(r.registry, ownPresent, inherited)

	rawFor := func(key string) string {
		if v, ok := resolved[key]; ok {
			return v.Raw
		}
		return ""
	}

	// Validate ALL reloadable inputs before applying anything.
	current := r.jobExpiry.Load()
	var newJobExpiryHours float64
	haveJobExpiry := false
	if v, ok := resolved["job-expiry-hours"]; ok {
		parsed, err := strconv.ParseFloat(v.Raw, 64)
		if err != nil {
			return Report{}, &ValidationError{
				Message: fmt.Sprintf("job-expiry-hours is not a valid number: %q", v.Raw),
			}
		}
		newJobExpiryHours = parsed
		haveJobExpiry = true
	}
	if haveJobExpiry && newJobExpiryHours < 0 {
		return Report{}, &ValidationError{Message: "job-expiry-hours must be a positive number."}
	}

	// Validation passed: apply.
	var report Report
	if haveJobExpiry && newJobExpiryHours != current.Hours {
		r.jobExpiry.Store(JobExpiry{
			Hours:    newJobExpiryHours,
			Duration: time.Duration(float64(time.Hour) * newJobExpiryHours),
		})
		report.Applied = append(report.Applied, "job-expiry-hours")
	}

	// logging-dir / enable-debug-logging are resolved above (available via
	// LastResolved) but deliberately NOT applied here: see the package doc
	// and task18b-report.md for why the SDK does not attempt runtime log
	// reconfiguration.

	if r.ApplyExtra != nil {
		if err := r.ApplyExtra(ctx); err != nil {
			r.lgr.Error("settings: plugin-specific reload apply failed", "error", err)
		}
	}

	// RestartRequired settings whose resolved value differs from the
	// STARTUP baseline (never cachedInherited, which a push mutates).
	baseline := RawByKey(r.startupInherited)
	for _, d := range r.registry {
		if d.ReloadClass != RestartRequired {
			continue
		}
		if _, ok := resolved[d.Key]; !ok {
			continue
		}
		if rawFor(d.Key) != baseline[d.Key] {
			report.PendingRestart = append(report.PendingRestart, d.Key)
		}
	}

	r.lastResolved = resolved

	return report, nil
}
