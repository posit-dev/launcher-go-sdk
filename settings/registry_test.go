package settings

import "testing"

// TestRegistry pins the exact seven keys, types, and reload classes the
// C++ Launcher registers for every plugin type (SettingsRegistry.hpp /
// SettingsTable.cpp). A future change to any of these is a breaking,
// cross-repo change (see docs/plugin-settings-registry.md's compatibility
// rule) and must not happen silently.
func TestRegistry(t *testing.T) {
	want := map[string]struct {
		Type        SettingType
		DualHomed   bool
		ReloadClass ReloadClass
	}{
		"job-expiry-hours":                {TypeFloat, true, Reloadable},
		"logging-dir":                     {TypePath, true, Reloadable},
		"enable-debug-logging":            {TypeBool, true, Reloadable},
		"server-user":                     {TypeString, true, RestartRequired},
		"scratch-path":                    {TypePath, true, RestartRequired},
		"heartbeat-interval-seconds":      {TypeUInt, true, RestartRequired},
		"plugin-metrics-interval-seconds": {TypeUInt, true, RestartRequired},
	}

	if len(Registry) != len(want) {
		t.Fatalf("len(Registry) = %d, want %d", len(Registry), len(want))
	}

	seen := map[string]bool{}
	for _, d := range Registry {
		seen[d.Key] = true
		w, ok := want[d.Key]
		if !ok {
			t.Errorf("unexpected registry key %q", d.Key)
			continue
		}
		if d.Type != w.Type {
			t.Errorf("%s: Type = %v, want %v", d.Key, d.Type, w.Type)
		}
		if d.DualHomed != w.DualHomed {
			t.Errorf("%s: DualHomed = %v, want %v", d.Key, d.DualHomed, w.DualHomed)
		}
		if d.ReloadClass != w.ReloadClass {
			t.Errorf("%s: ReloadClass = %v, want %v", d.Key, d.ReloadClass, w.ReloadClass)
		}
	}
	for k := range want {
		if !seen[k] {
			t.Errorf("missing registry key %q", k)
		}
	}
}

func TestDualHomedKeys(t *testing.T) {
	registry := []SettingDescriptor{
		{Key: "a", DualHomed: true},
		{Key: "b", DualHomed: false},
		{Key: "c", DualHomed: true},
	}
	got := DualHomedKeys(registry)
	want := []string{"a", "c"}
	if len(got) != len(want) {
		t.Fatalf("DualHomedKeys() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("DualHomedKeys()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
