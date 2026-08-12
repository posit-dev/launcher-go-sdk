package settings

import (
	"testing"

	"github.com/posit-dev/launcher-go-sdk/api"
)

func testInherited() api.InheritedSettings {
	return api.InheritedSettings{
		ServerUser:                          "rstudio-server",
		EnableDebugLogging:                  false,
		ScratchPath:                         "/var/lib/rstudio-launcher/local",
		LoggingDir:                          "/var/log/rstudio/launcher",
		HeartbeatIntervalSeconds:            5,
		JobExpiryHours:                      24,
		PluginMetricsIntervalSeconds:        60,
		IncludePluginMetricsIntervalSeconds: true,
	}
}

// TestResolve_OwnConfPresenceWins_EvenWhenValueEqualsDefault pins the exact
// bug this design fixes: an operator who explicitly sets job-expiry-hours to
// a value that happens to equal the inherited/default value must still get
// Provenance = OwnConf, never Inherited, because provenance is determined by
// KEY PRESENCE, not by comparing values.
func TestResolve_OwnConfPresenceWins_EvenWhenValueEqualsDefault(t *testing.T) {
	inherited := testInherited() // starts at 24 hours
	ownConfPresent := map[string]string{"job-expiry-hours": "24"}

	resolved := Resolve(Registry, ownConfPresent, inherited)

	got, ok := resolved["job-expiry-hours"]
	if !ok {
		t.Fatal("resolved map missing job-expiry-hours")
	}
	if got.Provenance != ProvenanceOwnConf {
		t.Errorf("Provenance = %v, want ProvenanceOwnConf (value-comparison bug)", got.Provenance)
	}
	if got.Raw != "24" {
		t.Errorf("Raw = %q, want %q", got.Raw, "24")
	}
}

func TestResolve_FallsBackToInherited_WhenNotInOwnConf(t *testing.T) {
	inherited := testInherited()
	resolved := Resolve(Registry, map[string]string{}, inherited)

	got, ok := resolved["job-expiry-hours"]
	if !ok {
		t.Fatal("resolved map missing job-expiry-hours")
	}
	if got.Provenance != ProvenanceInherited {
		t.Errorf("Provenance = %v, want ProvenanceInherited", got.Provenance)
	}
	if got.Raw != "24" {
		t.Errorf("Raw = %q, want %q", got.Raw, "24")
	}
}

func TestResolve_OwnConfDifferentValue(t *testing.T) {
	inherited := testInherited()
	ownConfPresent := map[string]string{"job-expiry-hours": "48"}

	resolved := Resolve(Registry, ownConfPresent, inherited)

	got := resolved["job-expiry-hours"]
	if got.Provenance != ProvenanceOwnConf || got.Raw != "48" {
		t.Errorf("got %+v, want OwnConf/48", got)
	}
}

func TestResolve_NonDualHomed_FallsBackToDefault(t *testing.T) {
	registry := []SettingDescriptor{
		{Key: "future-setting", Type: TypeString, DualHomed: false, ReloadClass: Reloadable},
	}
	resolved := Resolve(registry, map[string]string{}, testInherited())

	got, ok := resolved["future-setting"]
	if !ok {
		t.Fatal("resolved map missing future-setting")
	}
	if got.Provenance != ProvenanceDefault {
		t.Errorf("Provenance = %v, want ProvenanceDefault", got.Provenance)
	}
	if got.Raw != "" {
		t.Errorf("Raw = %q, want empty (no default-value source exists yet)", got.Raw)
	}
}

func TestResolve_EveryRegistryKeyPresent(t *testing.T) {
	resolved := Resolve(Registry, map[string]string{}, testInherited())
	if len(resolved) != len(Registry) {
		t.Fatalf("len(resolved) = %d, want %d", len(resolved), len(Registry))
	}
	for _, d := range Registry {
		if _, ok := resolved[d.Key]; !ok {
			t.Errorf("resolved map missing key %q", d.Key)
		}
	}
}

func TestResolve_OwnConfBareFlagIsEmptyString(t *testing.T) {
	// A key present with no value token (bare flag style) maps to "" — the
	// same convention the C++ isolation parser uses.
	inherited := testInherited()
	ownConfPresent := map[string]string{"enable-debug-logging": ""}

	resolved := Resolve(Registry, ownConfPresent, inherited)

	got := resolved["enable-debug-logging"]
	if got.Provenance != ProvenanceOwnConf {
		t.Errorf("Provenance = %v, want ProvenanceOwnConf", got.Provenance)
	}
	if got.Raw != "" {
		t.Errorf("Raw = %q, want empty", got.Raw)
	}
}
