package settings

import "github.com/posit-dev/launcher-go-sdk/api"

// Provenance identifies where a resolved setting's raw value came from. It
// mirrors the C++ Provenance enum (impls/SettingsResolver.hpp).
type Provenance int

const (
	// ProvenanceOwnConf indicates the plugin's own config file explicitly
	// set this key.
	ProvenanceOwnConf Provenance = iota
	// ProvenanceInherited indicates the Launcher's cascade
	// (api.InheritedSettings) supplied this key.
	ProvenanceInherited
	// ProvenanceDefault indicates neither of the above applied, so the
	// setting's built-in default applies. Reserved for future
	// non-dual-homed registry entries: every entry in [Registry] today is
	// dual-homed, so this case cannot currently be reached with a
	// populated built-in-default source, and Raw is always "" for it. A
	// future non-dual-homed SettingDescriptor would need a default-value
	// source added alongside this case.
	ProvenanceDefault
)

// String returns a human-readable name for p.
func (p Provenance) String() string {
	switch p {
	case ProvenanceOwnConf:
		return "own-conf"
	case ProvenanceInherited:
		return "inherited"
	case ProvenanceDefault:
		return "default"
	default:
		return "unknown"
	}
}

// ResolvedValue is one resolved dual-homed setting: its key, the winning raw
// string value (in the same string form used on the wire/argv — see
// [RawByKey]), and where that value came from.
type ResolvedValue struct {
	Key        string
	Raw        string
	Provenance Provenance
}

// ResolvedSettings holds every resolved setting for one plugin instance,
// keyed by SettingDescriptor.Key.
type ResolvedSettings map[string]ResolvedValue

// Resolve resolves every entry of registry per the design's provenance
// rule (own-conf > inherited > default), mirroring the C++ resolve()
// (impls/SettingsResolver.cpp):
//
//  1. If the key is present in ownConfPresent (regardless of its value) →
//     ProvenanceOwnConf, raw from ownConfPresent.
//  2. Else if the descriptor is dual-homed → ProvenanceInherited, raw from
//     the corresponding field of inherited, formatted identically to
//     [RawByKey] (== the C++ InheritedSettings::toArgs() cascade).
//  3. Else → ProvenanceDefault (see that case's doc comment for the current
//     limitation on Raw).
//
// ownConfPresent must come from the SAME own-conf source startup would
// resolve for this plugin instance (e.g. via [OwnConfSource]) — otherwise
// reload provenance can disagree with what startup would have computed for
// an identical config.
func Resolve(registry []SettingDescriptor, ownConfPresent map[string]string, inherited api.InheritedSettings) ResolvedSettings {
	result := make(ResolvedSettings, len(registry))
	inheritedByKey := RawByKey(inherited)

	for _, d := range registry {
		value := ResolvedValue{Key: d.Key}

		switch {
		case isPresent(ownConfPresent, d.Key):
			// Step 1: presence in the plugin's OWN conf wins, regardless of
			// what value it set — this is the provenance-by-presence rule
			// the design hinges on.
			value.Provenance = ProvenanceOwnConf
			value.Raw = ownConfPresent[d.Key]
		case d.DualHomed:
			// Step 2: otherwise, a dual-homed setting falls back to
			// whatever the Launcher's cascade supplied.
			value.Provenance = ProvenanceInherited
			value.Raw = inheritedByKey[d.Key]
		default:
			// Step 3: a non-dual-homed setting has no cascade at all, so it
			// always falls to its built-in default (see ProvenanceDefault).
			value.Provenance = ProvenanceDefault
		}

		result[d.Key] = value
	}

	return result
}

// isPresent reports whether key is present in m, distinct from being absent
// — a map lookup alone can't distinguish "present with zero value" from
// "absent" without the ok-idiom, and that distinction is exactly what
// provenance-by-presence depends on.
func isPresent(m map[string]string, key string) bool {
	_, ok := m[key]
	return ok
}
