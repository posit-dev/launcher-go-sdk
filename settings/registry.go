// Package settings resolves the Launcher's "dual-homed" [server] settings —
// settings that can be supplied both via the Launcher's cascaded CLI args to
// a plugin (see [api.InheritedSettings]) and via the plugin's own config
// file — and reports the outcome of a config reload back to the Launcher.
//
// This package mirrors the C++ Launcher's SettingsRegistry / SettingsResolver
// / SettingsReloadRoutine design (see the launcher repo's
// src/cpp/job_launcher/impls/SettingsResolver.cpp and
// SettingsReloadRoutine.cpp). The C++ implementation is normative; this
// package intentionally follows the same provenance rule, the same
// Reloadable/RestartRequired classification, and the same startup-baseline
// comparison for pendingRestart reporting.
package settings

// SettingType is the canonical value type of a dual-homed setting, used for
// parsing and validation. It mirrors the C++ SettingType enum
// (launcher/SettingsRegistry.hpp).
type SettingType int

const (
	// TypeString is a plain string value.
	TypeString SettingType = iota
	// TypeBool is a boolean value.
	TypeBool
	// TypeFloat is a floating-point value.
	TypeFloat
	// TypeUInt is an unsigned integer value.
	TypeUInt
	// TypePath is a filesystem path, represented as a string.
	TypePath
)

// ReloadClass indicates whether a config reload can apply a new value to an
// already-running plugin (Reloadable), or whether the setting can only take
// effect on the plugin's next start (RestartRequired). It mirrors the C++
// ReloadClass enum (launcher/SettingsRegistry.hpp).
type ReloadClass int

const (
	// Reloadable settings can be applied to an already-running plugin.
	Reloadable ReloadClass = iota
	// RestartRequired settings can only take effect on the next plugin
	// start; a config reload can only report that they are pending.
	RestartRequired
)

// SettingDescriptor describes one dual-homed setting. It mirrors the C++
// SettingDescriptor struct (launcher/SettingsRegistry.hpp) exactly: it is
// data only. Deciding how (or whether) a Reloadable setting's new value is
// adopted at runtime is the responsibility of the [Reloader], via its
// ApplyExtra hook and its built-in job-expiry-hours handling — not the
// registry.
type SettingDescriptor struct {
	// Key is the CLI flag / own-conf key name, without the "--" prefix
	// (e.g. "job-expiry-hours").
	Key string
	// Type is the setting's canonical value type.
	Type SettingType
	// DualHomed indicates whether the setting is dual-homed (reserved for
	// future non-dual-homed entries; always true today).
	DualHomed bool
	// ReloadClass indicates whether a config reload can apply a new value
	// live, or the plugin must be restarted.
	ReloadClass ReloadClass
}

// Registry is the SDK's settings registry: the seven dual-homed [server]
// settings the Launcher cascades to every plugin, exactly matching the C++
// registry tables (localSettingsRegistry / kubernetesSettingsRegistry /
// slurmSettingsRegistry in launcher/SettingsRegistry.hpp), which are
// identical across plugin types because the [server]-section cascade they
// describe is identical across plugin types. A Go SDK plugin implements
// exactly one of those cluster types' worth of behavior, so one shared table
// suffices here.
var Registry = []SettingDescriptor{
	{Key: "job-expiry-hours", Type: TypeFloat, DualHomed: true, ReloadClass: Reloadable},
	{Key: "logging-dir", Type: TypePath, DualHomed: true, ReloadClass: Reloadable},
	{Key: "enable-debug-logging", Type: TypeBool, DualHomed: true, ReloadClass: Reloadable},
	{Key: "server-user", Type: TypeString, DualHomed: true, ReloadClass: RestartRequired},
	{Key: "scratch-path", Type: TypePath, DualHomed: true, ReloadClass: RestartRequired},
	{Key: "heartbeat-interval-seconds", Type: TypeUInt, DualHomed: true, ReloadClass: RestartRequired},
	{Key: "plugin-metrics-interval-seconds", Type: TypeUInt, DualHomed: true, ReloadClass: RestartRequired},
}

// DualHomedKeys returns the key of every dual-homed descriptor in registry,
// in registry order. This is a convenience for building an [OwnConfSource]
// (e.g. [IniOwnConfSource]) without hand-listing the keys.
func DualHomedKeys(registry []SettingDescriptor) []string {
	keys := make([]string, 0, len(registry))
	for _, d := range registry {
		if d.DualHomed {
			keys = append(keys, d.Key)
		}
	}
	return keys
}
