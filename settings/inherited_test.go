package settings

import (
	"strconv"
	"testing"

	"github.com/posit-dev/launcher-go-sdk/api"
)

func TestFormatJobExpiryHoursLossless_RoundTrips(t *testing.T) {
	// The brief requires proving round-trip losslessness for these two
	// specific values (a fractional value and a value requiring more than
	// one significant digit past the decimal point).
	for _, v := range []float64{1234.567, 0.5} {
		t.Run(strconv.FormatFloat(v, 'g', -1, 64), func(t *testing.T) {
			raw := FormatJobExpiryHoursLossless(v)
			parsed, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				t.Fatalf("ParseFloat(%q) error = %v", raw, err)
			}
			if parsed != v {
				t.Errorf("round-trip: FormatJobExpiryHoursLossless(%v) = %q, ParseFloat back = %v, want %v", v, raw, parsed, v)
			}
		})
	}
}

func TestRawByKey(t *testing.T) {
	inherited := api.InheritedSettings{
		ServerUser:                          "rstudio-server",
		EnableDebugLogging:                  true,
		ScratchPath:                         "/var/lib/rstudio-launcher/local",
		LoggingDir:                          "/var/log/rstudio/launcher",
		HeartbeatIntervalSeconds:            5,
		JobExpiryHours:                      24,
		PluginMetricsIntervalSeconds:        60,
		IncludePluginMetricsIntervalSeconds: true,
	}

	got := RawByKey(inherited)

	want := map[string]string{
		"server-user":                     "rstudio-server",
		"enable-debug-logging":            "1",
		"scratch-path":                    "/var/lib/rstudio-launcher/local",
		"logging-dir":                     "/var/log/rstudio/launcher",
		"heartbeat-interval-seconds":      "5",
		"job-expiry-hours":                "24",
		"plugin-metrics-interval-seconds": "60",
	}

	if len(got) != len(want) {
		t.Fatalf("RawByKey() = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("RawByKey()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestRawByKey_ExcludesPluginMetricsWhenNotIncluded(t *testing.T) {
	inherited := api.InheritedSettings{
		IncludePluginMetricsIntervalSeconds: false,
		PluginMetricsIntervalSeconds:        60,
	}

	got := RawByKey(inherited)

	if _, ok := got["plugin-metrics-interval-seconds"]; ok {
		t.Errorf("RawByKey() included plugin-metrics-interval-seconds when IncludePluginMetricsIntervalSeconds is false: %v", got)
	}
}

func TestRawByKey_EnableDebugLoggingFalse(t *testing.T) {
	inherited := api.InheritedSettings{EnableDebugLogging: false}
	got := RawByKey(inherited)
	if got["enable-debug-logging"] != "0" {
		t.Errorf("RawByKey()[enable-debug-logging] = %q, want %q", got["enable-debug-logging"], "0")
	}
}
