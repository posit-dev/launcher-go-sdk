package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_NoFile(t *testing.T) {
	t.Setenv("RS_LOG_CONF_FILE", filepath.Join(t.TempDir(), "nonexistent.conf"))
	defaults := Config{Level: slog.LevelInfo, Type: DestinationFile, Dir: "/tmp/logs"}
	cfg, err := LoadConfig("myplugin", defaults)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelInfo {
		t.Errorf("Level: got %v, want %v", cfg.Level, slog.LevelInfo)
	}
	if cfg.Type != DestinationFile {
		t.Errorf("Type: got %v, want %v", cfg.Type, DestinationFile)
	}
	if cfg.Dir != "/tmp/logs" {
		t.Errorf("Dir: got %q, want %q", cfg.Dir, "/tmp/logs")
	}
}

func TestLoadConfig_GlobalSection(t *testing.T) {
	f := writeConfFile(t, `
[*]
log-level     = debug
logger-type   = stderr
log-dir       = /custom/logs
`)
	t.Setenv("RS_LOG_CONF_FILE", f)
	cfg, err := LoadConfig("myplugin", Config{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelDebug {
		t.Errorf("Level: got %v, want %v", cfg.Level, slog.LevelDebug)
	}
	if cfg.Type != DestinationStderr {
		t.Errorf("Type: got %v, want %v", cfg.Type, DestinationStderr)
	}
	if cfg.Dir != "/custom/logs" {
		t.Errorf("Dir: got %q, want %q", cfg.Dir, "/custom/logs")
	}
}

func TestLoadConfig_PerExecutableOverridesGlobal(t *testing.T) {
	f := writeConfFile(t, `
[*]
log-level   = warn
logger-type = file

[@myplugin]
log-level   = debug
logger-type = stderr
`)
	t.Setenv("RS_LOG_CONF_FILE", f)
	cfg, err := LoadConfig("myplugin", Config{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelDebug {
		t.Errorf("Level: got %v, want %v", cfg.Level, slog.LevelDebug)
	}
	if cfg.Type != DestinationStderr {
		t.Errorf("Type: got %v, want %v", cfg.Type, DestinationStderr)
	}
}

func TestLoadConfig_OtherExecutableSectionIgnored(t *testing.T) {
	f := writeConfFile(t, `
[*]
log-level = warn

[@otherplugin]
log-level = debug
`)
	t.Setenv("RS_LOG_CONF_FILE", f)
	cfg, err := LoadConfig("myplugin", Config{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelWarn {
		t.Errorf("Level: got %v, want %v", cfg.Level, slog.LevelWarn)
	}
}

func TestLoadConfig_EnvVarOverridesConf(t *testing.T) {
	f := writeConfFile(t, `
[*]
log-level   = warn
logger-type = file
log-dir     = /from/conf
`)
	t.Setenv("RS_LOG_CONF_FILE", f)
	t.Setenv("RS_LOG_LEVEL", "debug")
	t.Setenv("RS_LOGGER_TYPE", "stderr")
	t.Setenv("RS_LOG_DIR", "/from/env")

	cfg, err := LoadConfig("myplugin", Config{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelDebug {
		t.Errorf("Level: got %v, want %v", cfg.Level, slog.LevelDebug)
	}
	if cfg.Type != DestinationStderr {
		t.Errorf("Type: got %v, want %v", cfg.Type, DestinationStderr)
	}
	if cfg.Dir != "/from/env" {
		t.Errorf("Dir: got %q, want %q", cfg.Dir, "/from/env")
	}
}

func TestLoadConfig_BlankValueLeavesDefaultUnchanged(t *testing.T) {
	// A key present with a blank value (log-level =) must not override the
	// default. Without the empty-value guard in applySection, parseLevel("")
	// returns LevelWarn, silently clobbering a LevelDebug default.
	f := writeConfFile(t, "[*]\nlog-level =\nlog-dir =\n")
	t.Setenv("RS_LOG_CONF_FILE", f)
	defaults := Config{Level: slog.LevelDebug, Type: DestinationFile, Dir: "/keep/me"}
	cfg, err := LoadConfig("myplugin", defaults)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelDebug {
		t.Errorf("Level: got %v, want LevelDebug — blank value must not override default", cfg.Level)
	}
	if cfg.Dir != "/keep/me" {
		t.Errorf("Dir: got %q, want %q — blank value must not override default", cfg.Dir, "/keep/me")
	}
}

func TestLoadConfig_DefaultsBeatAbsentConf(t *testing.T) {
	t.Setenv("RS_LOG_CONF_FILE", filepath.Join(t.TempDir(), "no.conf"))
	defaults := Config{Level: slog.LevelDebug, Type: DestinationFile, Dir: "/legacy/dir"}
	cfg, err := LoadConfig("myplugin", defaults)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelDebug {
		t.Errorf("Level: got %v, want %v", cfg.Level, slog.LevelDebug)
	}
	if cfg.Dir != "/legacy/dir" {
		t.Errorf("Dir: got %q, want %q", cfg.Dir, "/legacy/dir")
	}
}

func TestLoadConfig_MalformedFile(t *testing.T) {
	// gopkg.in/ini.v1 with Loose:true silently ignores lines without a
	// key-value delimiter, but a binary blob that contains non-UTF-8 bytes
	// and no valid delimiter triggers "key-value delimiter not found".
	// Null bytes mixed with ASCII text are silently ignored, so we use a
	// pure binary payload that the parser cannot interpret.
	f := writeConfFile(t, "\x00\x01\x02\x03\xff\xfe")
	t.Setenv("RS_LOG_CONF_FILE", f)
	_, err := LoadConfig("myplugin", Config{})
	if err == nil {
		t.Fatal("expected error for malformed conf file, got nil")
	}
}

func TestLoadConfig_NonRegularFile(t *testing.T) {
	// A directory path that exists but is not a regular file must return an error,
	// not silently fall back to defaults. This catches RS_LOG_CONF_FILE
	// misconfiguration (e.g. pointing at /etc/rstudio/ instead of the file).
	t.Setenv("RS_LOG_CONF_FILE", t.TempDir())
	_, err := LoadConfig("myplugin", Config{})
	if err == nil {
		t.Fatal("expected error for directory path, got nil")
	}
}

func TestLoadConfig_PerExecutableSectionNoGlobal(t *testing.T) {
	// A conf file with only a per-executable section (no [*]) must still apply
	// that section's values. Tests the applySection path when the [*] section
	// is absent (ini.v1 returns an empty section, not nil).
	f := writeConfFile(t, "[@myplugin]\nlog-level = error\n")
	t.Setenv("RS_LOG_CONF_FILE", f)
	cfg, err := LoadConfig("myplugin", Config{Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelError {
		t.Errorf("Level: got %v, want LevelError", cfg.Level)
	}
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelWarn},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := parseLevel(tc.in); got != tc.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseLoggerType(t *testing.T) {
	cases := []struct {
		in   string
		want Destination
	}{
		{"stderr", DestinationStderr},
		{"STDERR", DestinationStderr},
		{"file", DestinationFile},
		{"unknown", DestinationFile},
		{"", DestinationFile},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := parseLoggerType(tc.in); got != tc.want {
				t.Errorf("parseLoggerType(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestLoadConfig_EnvOverrideWithNoFile(t *testing.T) {
	// RS_LOG_LEVEL must be applied even when no conf file exists.
	t.Setenv("RS_LOG_CONF_FILE", filepath.Join(t.TempDir(), "nonexistent.conf"))
	t.Setenv("RS_LOG_LEVEL", "debug")
	cfg, err := LoadConfig("myplugin", Config{Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != slog.LevelDebug {
		t.Errorf("Level: got %v, want LevelDebug — env override must apply even without a conf file", cfg.Level)
	}
}

// TestDebugFlagClamp demonstrates that LoadConfig does not itself enforce a
// minimum level when the debug flag is set — that responsibility belongs to the
// caller. If logging.conf overrides the debug default to a higher level, the
// caller must clamp after loading (as inmemory/main.go does).
func TestDebugFlagClamp(t *testing.T) {
	f := writeConfFile(t, "[*]\nlog-level = warn\n")
	t.Setenv("RS_LOG_CONF_FILE", f)

	// Simulate --enable-debug-logging setting the default to DEBUG.
	defaults := Config{Level: slog.LevelDebug, Type: DestinationStderr}
	logCfg, err := LoadConfig("myplugin", defaults)
	if err != nil {
		t.Fatal(err)
	}

	// LoadConfig applies conf over defaults: level is now WARN.
	if logCfg.Level != slog.LevelWarn {
		t.Errorf("before clamp: got %v, want LevelWarn", logCfg.Level)
	}

	// Caller clamps to enforce --enable-debug-logging contract.
	if logCfg.Level > slog.LevelDebug {
		logCfg.Level = slog.LevelDebug
	}

	if logCfg.Level != slog.LevelDebug {
		t.Errorf("after clamp: got %v, want LevelDebug", logCfg.Level)
	}
}

// writeConfFile writes content to a temp file and returns its path.
func writeConfFile(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "logging.conf")
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}
