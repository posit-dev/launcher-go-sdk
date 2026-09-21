package settings

import (
	"strconv"
	"testing"

	"github.com/posit-dev/launcher-go-sdk/api"
)

// TestFormatJobExpiryHoursLossless_ExactVectors pins the three canonical
// cross-language vectors from
// testdata/settings-resolver-conformance.json's job-expiry-hours-lossless-*
// cases directly against FormatJobExpiryHoursLossless, so a regression here
// is caught even independent of resolve_conformance_test.go's full-fixture
// run. 1234.567 and 0.1 exercise float32 rounding noise (see the function's
// doc comment); 0.5 is exactly representable in binary and agrees with or
// without narrowing.
func TestFormatJobExpiryHoursLossless_ExactVectors(t *testing.T) {
	tests := []struct {
		hours float64
		want  string
	}{
		{1234.567, "1234.56702"},
		{0.5, "0.5"},
		{0.1, "0.100000001"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := FormatJobExpiryHoursLossless(tc.hours)
			if got != tc.want {
				t.Errorf("FormatJobExpiryHoursLossless(%v) = %q, want %q", tc.hours, got, tc.want)
			}
		})
	}
}

// TestFormatJobExpiryHoursLossless_RoundTripsAtFloat32Precision verifies
// round-trip losslessness relative to the float32-narrowed value, not the
// original float64 input: FormatJobExpiryHoursLossless deliberately narrows
// to float32 first (matching the C++ Launcher's own float precision for
// this field - see the function's doc comment), so the raw string it
// produces can only round-trip to that narrowed value.
func TestFormatJobExpiryHoursLossless_RoundTripsAtFloat32Precision(t *testing.T) {
	for _, v := range []float64{1234.567, 0.5, 0.1} {
		t.Run(strconv.FormatFloat(v, 'g', -1, 64), func(t *testing.T) {
			raw := FormatJobExpiryHoursLossless(v)
			// Parse as float32, not float64: the 9-significant-digit
			// (max_digits10) formatting this function uses is calibrated to
			// round-trip losslessly through a float32 parse, not a float64
			// one - parsing "0.100000001" back as float64 does not exactly
			// equal float64(float32(0.1))'s full ~17-digit expansion, but
			// parsing it as float32 does.
			parsed, err := strconv.ParseFloat(raw, 32)
			if err != nil {
				t.Fatalf("ParseFloat(%q) error = %v", raw, err)
			}
			want := float32(v)
			if float32(parsed) != want {
				t.Errorf("round-trip: FormatJobExpiryHoursLossless(%v) = %q, ParseFloat(_, 32) back = %v, want %v (the float32-narrowed value)", v, raw, float32(parsed), want)
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
