package settings

import (
	"strconv"

	"github.com/posit-dev/launcher-go-sdk/api"
)

// FormatJobExpiryHoursLossless renders hours the same lossless way the C++
// Launcher's InheritedSettings::toArgs() does (formatJobExpiryHoursLossless
// in launcher/InheritedSettings.cpp, std::setprecision(max_digits10) on a
// float). The Go SDK's canonical job-expiry-hours type is float64 (see
// api.InheritedSettings.JobExpiryHours), so rather than hard-coding a
// digit count tuned for float32, this uses strconv's shortest
// round-trip-exact representation for float64, which is lossless by
// construction — see TestFormatJobExpiryHoursLossless_RoundTrips.
func FormatJobExpiryHoursLossless(hours float64) string {
	return strconv.FormatFloat(hours, 'g', -1, 64)
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
