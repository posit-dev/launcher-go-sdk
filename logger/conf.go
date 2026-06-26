package logger

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/ini.v1"
)

// Destination identifies the logging destination for a plugin.
type Destination string

const (
	// DestinationFile writes logs to a file in the configured directory.
	DestinationFile Destination = "file"
	// DestinationStderr writes logs to standard error only.
	DestinationStderr Destination = "stderr"
)

// Config holds the resolved logging configuration for a plugin.
// Use [LoadConfig] to populate it from Workbench's logging.conf.
// Prefer [LoadConfig] over constructing this directly.
//
// The zero value of [Config] has Level [slog.LevelInfo] (numeric zero), an
// empty Type, and an empty Dir. An empty Type with an empty Dir causes
// [NewLogger] to write to stderr; an empty Type with a non-empty Dir causes
// [NewLogger] to write to a log file. Use [DestinationFile] or
// [DestinationStderr] explicitly to avoid relying on this fallback.
type Config struct {
	// Level is the minimum level to emit.
	Level slog.Level
	// Type selects the log destination ([DestinationFile] or [DestinationStderr]).
	// An empty Type falls back to file mode when Dir is set, stderr otherwise.
	Type Destination
	// Dir is the directory for log files. Used only when Type is [DestinationFile].
	// Must be non-empty when Type is [DestinationFile]; if empty, [NewLogger]
	// falls back to stderr regardless of Type.
	Dir string
}

const defaultConfPath = "/etc/rstudio/logging.conf"

// LoadConfig reads the Workbench logging configuration from logging.conf
// (or $RS_LOG_CONF_FILE when set), applies [*] global and [@executableName]
// per-binary overrides, then applies env-var overrides (RS_LOG_LEVEL,
// RS_LOGGER_TYPE, RS_LOG_DIR). defaults is used as the initial [Config]; the
// conf file and env vars layer on top of it, so fields absent from the conf
// file retain their value from defaults. On error the returned Config equals
// defaults.
func LoadConfig(executableName string, defaults Config) (Config, error) {
	cfg := defaults

	confPath := os.Getenv("RS_LOG_CONF_FILE")
	if confPath == "" {
		confPath = defaultConfPath
	}

	// Reject non-regular-file paths (e.g. directories) before calling
	// ini.LoadSources, which uses Loose:true to swallow os.IsNotExist but
	// propagates EISDIR when os.Open succeeds on a directory.
	fi, statErr := os.Stat(confPath)
	if statErr != nil {
		if !os.IsNotExist(statErr) {
			return cfg, fmt.Errorf("reading logging config %q: %w", confPath, statErr)
		}
		// file absent — fall through to env overrides
	} else if !fi.Mode().IsRegular() {
		return cfg, fmt.Errorf("reading logging config %q: not a regular file", confPath)
	} else {
		// file exists and is regular
		iniCfg, err := ini.LoadSources(ini.LoadOptions{Loose: true}, confPath)
		if err != nil {
			return cfg, fmt.Errorf("reading logging config %q: %w", confPath, err)
		}
		applySection(&cfg, iniCfg.Section("*"))
		applySection(&cfg, iniCfg.Section("@"+executableName))
	}
	// Env vars always win, whether or not a conf file was present.
	applyEnvOverrides(&cfg)
	return cfg, nil
}

// applySection merges keys from a logging.conf section into cfg.
// Only keys present in the section are applied; absent keys leave cfg unchanged.
func applySection(cfg *Config, sec *ini.Section) {
	if sec.HasKey("log-level") {
		if v := sec.Key("log-level").String(); v != "" {
			cfg.Level = parseLevel(v)
		}
	}
	if sec.HasKey("logger-type") {
		if v := sec.Key("logger-type").String(); v != "" {
			cfg.Type = parseLoggerType(v)
		}
	}
	if sec.HasKey("log-dir") {
		if v := sec.Key("log-dir").String(); v != "" {
			cfg.Dir = v
		}
	}
}

// applyEnvOverrides applies RS_LOG_LEVEL, RS_LOGGER_TYPE, and RS_LOG_DIR to
// cfg. Non-empty env vars always win over conf file values.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("RS_LOG_LEVEL"); v != "" {
		cfg.Level = parseLevel(v)
	}
	if v := os.Getenv("RS_LOGGER_TYPE"); v != "" {
		cfg.Type = parseLoggerType(v)
	}
	if v := os.Getenv("RS_LOG_DIR"); v != "" {
		cfg.Dir = v
	}
}

// parseLevel maps a logging.conf log-level string to a [slog.Level].
// Unrecognized values map to [slog.LevelWarn] and print a warning to stderr,
// matching the Workbench default while making misconfiguration visible.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		fmt.Fprintf(os.Stderr, "warning: unrecognized log-level %q, defaulting to warn\n", s)
		return slog.LevelWarn
	}
}

// parseLoggerType maps a logging.conf logger-type string to a [Destination].
// "stderr" (case-insensitive) maps to [DestinationStderr]; unrecognized values
// (including "file") map to [DestinationFile] and print a warning to stderr
// when the value is not the literal string "file".
func parseLoggerType(s string) Destination {
	if strings.EqualFold(s, "stderr") {
		return DestinationStderr
	}
	if !strings.EqualFold(s, "file") {
		fmt.Fprintf(os.Stderr, "warning: unrecognized logger-type %q, defaulting to file\n", s)
	}
	return DestinationFile
}
