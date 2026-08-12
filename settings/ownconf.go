package settings

import (
	"log/slog"

	"gopkg.in/ini.v1"
)

// OwnConfSource supplies the presence and raw values of dual-homed settings
// from a plugin's own config file, for use by [Resolve]. It is the Go SDK
// equivalent of the C++ parseOwnConfKeysInIsolation()
// (impls/SettingsResolver.cpp): the SDK cannot know a plugin's own config
// format, so plugin authors supply this themselves (or use
// [IniOwnConfSource] if their own-conf happens to be INI, matching the
// Launcher's own .conf format).
//
// Implementations are best-effort. A source that cannot read its backing
// store (missing file, permission error, malformed syntax) should return an
// empty map rather than an error: own-conf presence is best-effort input to
// provenance, not a hard dependency the reload path can fail on — matching
// the C++ behavior exactly.
type OwnConfSource interface {
	// OwnConfKeys returns the raw string value of each dual-homed setting
	// key that is textually present in the plugin's own config. A key
	// absent from the config must be absent from the returned map — never
	// mapped to "" or a default — since [Resolve] determines provenance by
	// the key's presence in this map, not by its value. A key present with
	// no value token (a bare flag-style entry) should map to some
	// implementation-defined truthy placeholder ("" or "true" are both
	// fine, and are treated identically by this package's bool parsing) —
	// what matters is that the key is present in the map at all.
	OwnConfKeys() map[string]string
}

// StaticOwnConfSource is a trivial [OwnConfSource] backed by a fixed map.
// Useful for tests, or for plugin authors who already have their own-conf
// presence available in some other form.
type StaticOwnConfSource map[string]string

// OwnConfKeys implements [OwnConfSource].
func (s StaticOwnConfSource) OwnConfKeys() map[string]string {
	return map[string]string(s)
}

// IniOwnConfSource is an [OwnConfSource] backed by an INI-style file with no
// section headers (matching the Launcher's own .conf format, e.g.
// launcher.local.conf) — the same flat key=value shape
// parseOwnConfKeysInIsolation() parses on the C++ side. Values are read from
// the unnamed default section.
//
// Read failures (missing file, permission error, malformed syntax) are
// best-effort: OwnConfKeys logs (if Logger is set) and returns an empty map,
// rather than erroring, matching the OwnConfSource contract.
type IniOwnConfSource struct {
	// Path is the plugin's own config file path. Must be resolved by the
	// caller using the exact same procedure the plugin's startup option
	// parsing uses (e.g. --config-file if set, else the plugin's compiled
	// default path if it exists) — otherwise reload provenance can disagree
	// with what startup would have computed for identical input. An empty
	// Path yields an empty result (matching "no own-conf file configured").
	Path string

	// Keys lists the dual-homed keys to check for presence. If empty,
	// defaults to [DualHomedKeys] of [Registry].
	Keys []string

	// Logger, if set, receives a warning when Path cannot be read. Optional.
	Logger *slog.Logger
}

// OwnConfKeys implements [OwnConfSource].
func (s IniOwnConfSource) OwnConfKeys() map[string]string {
	result := map[string]string{}
	if s.Path == "" {
		return result
	}

	keys := s.Keys
	if len(keys) == 0 {
		keys = DualHomedKeys(Registry)
	}

	cfg, err := ini.LoadSources(ini.LoadOptions{Loose: true, AllowBooleanKeys: true}, s.Path)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("settings: failed to read own-conf file for reload provenance; treating as no keys present",
				"path", s.Path, "error", err)
		}
		return result
	}

	sec := cfg.Section("")
	for _, key := range keys {
		if sec.HasKey(key) {
			result[key] = sec.Key(key).String()
		}
	}
	return result
}
