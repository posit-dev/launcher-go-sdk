package settings

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestStaticOwnConfSource(t *testing.T) {
	src := StaticOwnConfSource{"job-expiry-hours": "48"}
	got := src.OwnConfKeys()
	if got["job-expiry-hours"] != "48" {
		t.Errorf("OwnConfKeys() = %v, want job-expiry-hours=48", got)
	}
}

func writeConf(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "launcher.local.conf")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func TestIniOwnConfSource_PresentKeys(t *testing.T) {
	path := writeConf(t, "job-expiry-hours=48\nload-balancer-hostname=node1\n")
	src := IniOwnConfSource{Path: path, Keys: DualHomedKeys(Registry)}

	got := src.OwnConfKeys()

	if v, ok := got["job-expiry-hours"]; !ok || v != "48" {
		t.Errorf("OwnConfKeys()[job-expiry-hours] = (%q, %v), want (48, true)", v, ok)
	}
	// Not a dual-homed key we asked about, must not leak in.
	if _, ok := got["load-balancer-hostname"]; ok {
		t.Errorf("OwnConfKeys() included non-requested key load-balancer-hostname: %v", got)
	}
	// Not present in the file at all.
	if _, ok := got["logging-dir"]; ok {
		t.Errorf("OwnConfKeys() included absent key logging-dir: %v", got)
	}
}

func TestIniOwnConfSource_BareFlagIsPresentAndTruthy(t *testing.T) {
	// ini.v1's AllowBooleanKeys represents a bare "key" line (no "=value")
	// as present with the string "true". This is a different literal than
	// the C++ isolation parser's "" for the same shape, but both are
	// treated as truthy by downstream bool parsing (see
	// SettingsReloadRoutine.cpp's resolvedDebugLogging derivation, which
	// accepts "" and "true" identically) — what matters for OwnConfSource's
	// contract is that the key is PRESENT, which this asserts.
	path := writeConf(t, "enable-debug-logging\n")
	src := IniOwnConfSource{Path: path, Keys: []string{"enable-debug-logging"}}

	got := src.OwnConfKeys()

	v, ok := got["enable-debug-logging"]
	if !ok {
		t.Fatal("OwnConfKeys() missing enable-debug-logging")
	}
	if v != "true" {
		t.Errorf("OwnConfKeys()[enable-debug-logging] = %q, want %q", v, "true")
	}
}

func TestIniOwnConfSource_MissingFile_BestEffortEmpty(t *testing.T) {
	src := IniOwnConfSource{Path: filepath.Join(t.TempDir(), "does-not-exist.conf"), Keys: []string{"job-expiry-hours"}}

	got := src.OwnConfKeys()

	if len(got) != 0 {
		t.Errorf("OwnConfKeys() = %v, want empty map for missing file", got)
	}
}

func TestIniOwnConfSource_EmptyPath_BestEffortEmpty(t *testing.T) {
	src := IniOwnConfSource{Path: "", Keys: []string{"job-expiry-hours"}}
	got := src.OwnConfKeys()
	if len(got) != 0 {
		t.Errorf("OwnConfKeys() = %v, want empty map for empty path", got)
	}
}

func TestIniOwnConfSource_UnreadableFile_BestEffortEmptyAndLogged(t *testing.T) {
	// A directory is not a valid conf file; ini.v1 fails to open it. This
	// exercises the same "reading failure" path a permission error would.
	dirAsPath := t.TempDir()

	var logged bool
	handler := slog.NewTextHandler(&testWriter{t: t, seen: &logged}, nil)
	lgr := slog.New(handler)

	src := IniOwnConfSource{Path: dirAsPath, Keys: []string{"job-expiry-hours"}, Logger: lgr}
	got := src.OwnConfKeys()

	if len(got) != 0 {
		t.Errorf("OwnConfKeys() = %v, want empty map for unreadable file", got)
	}
	if !logged {
		t.Error("expected a log message for the read failure")
	}
}

// testWriter is a minimal io.Writer that records whether anything was
// written to it, for asserting that a failure path logged something without
// coupling the test to the exact log message.
type testWriter struct {
	t    *testing.T
	seen *bool
}

func (w *testWriter) Write(p []byte) (int, error) {
	*w.seen = true
	return len(p), nil
}

func TestIniOwnConfSource_DefaultsKeysToRegistry(t *testing.T) {
	path := writeConf(t, "job-expiry-hours=48\n")
	src := IniOwnConfSource{Path: path} // Keys left unset.

	got := src.OwnConfKeys()

	if v, ok := got["job-expiry-hours"]; !ok || v != "48" {
		t.Errorf("OwnConfKeys() with default Keys = %v, want job-expiry-hours=48", got)
	}
}
