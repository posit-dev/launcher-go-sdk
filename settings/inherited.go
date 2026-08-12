package settings

import (
	"strconv"

	"github.com/posit-dev/launcher-go-sdk/api"
)

// FormatJobExpiryHoursLossless renders the Launcher's INHERITED
// job-expiry-hours the same lossless way the C++ Launcher's
// InheritedSettings::toArgs() does (formatJobExpiryHoursLossless in
// launcher/InheritedSettings.cpp): narrow to float32, then format with
// max_digits10 (9 significant digits) precision.
//
// DO NOT "simplify" this back to formatting the full float64 value — that
// was tried (see task19b-report.md's cross-implementation finding) and is
// wrong. api.InheritedSettings.JobExpiryHours is typed float64 in this SDK,
// but the value it carries always originates from the C++ Launcher's
// `float jobExpiryHours` member (InheritedSettings.hpp) — whether it
// arrives via the argv cascade or this IPC field, float32 is genuinely the
// most precision the Launcher can ever express for this setting. Narrowing
// here loses no real information relative to the source of truth; it only
// reproduces the same float32 rounding noise C++ already committed to
// on values that are not exactly representable in binary (e.g. 0.1 ->
// "0.100000001"), so the two implementations' resolved raw strings agree
// byte for byte. Formatting the un-narrowed float64 instead is
// FLOAT64-lossless but produces a DIFFERENT string than C++ for any value
// with float32 rounding noise, silently breaking cross-SDK agreement on the
// resolved-settings snapshot. See
// settings/testdata/settings-resolver-conformance.json's
// job-expiry-hours-lossless-* cases, which pin the exact strings this must
// produce.
//
// This function is for the INHERITED layer only (see [RawByKey]) — it must
// never be applied to an own-conf raw value, which comes from the plugin's
// own config text verbatim and was never a C++ float32 to begin with.
func FormatJobExpiryHoursLossless(hours float64) string {
	narrowed := float64(float32(hours))
	return strconv.FormatFloat(narrowed, 'g', 9, 32)
}

// RawByKey renders inherited the same way it is sent over the wire
// (api.InheritedSettings / the C++ InheritedSettings::toArgs() cascade),
// re-indexed by bare key (e.g. "job-expiry-hours" rather than
// "--job-expiry-hours") so [Resolve] can look values up by
// SettingDescriptor.Key. This must stay in lock-step with what a Go SDK
// plugin actually receives on its own command line at spawn time; keeping a
// single function for both jobs is what makes that guarantee cheap to keep.
func RawByKey(inherited api.InheritedSettings) map[string]string {
	byKey := map[string]string{
		"server-user":                inherited.ServerUser,
		"enable-debug-logging":       boolFlag(inherited.EnableDebugLogging),
		"scratch-path":               inherited.ScratchPath,
		"logging-dir":                inherited.LoggingDir,
		"heartbeat-interval-seconds": strconv.FormatUint(uint64(inherited.HeartbeatIntervalSeconds), 10),
		"job-expiry-hours":           FormatJobExpiryHoursLossless(inherited.JobExpiryHours),
	}
	if inherited.IncludePluginMetricsIntervalSeconds {
		byKey["plugin-metrics-interval-seconds"] = strconv.FormatUint(uint64(inherited.PluginMetricsIntervalSeconds), 10)
	}
	return byKey
}

// boolFlag renders a bool the same way the Launcher's cascade does:
// "1"/"0", never "true"/"false".
func boolFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
