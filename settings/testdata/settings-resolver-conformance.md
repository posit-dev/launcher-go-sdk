# Settings-resolver conformance fixtures

`settings-resolver-conformance.json` (this directory) is a language-neutral set of test vectors
for the dual-homed settings **precedence/provenance rule**: for a dual-homed setting, the plugin's
own conf wins over the launcher's inherited cascade **by key presence**, never by comparing
values. Both the C++ launcher (`impls/SettingsResolver.cpp`, `resolve()`) and the Go plugin SDK
(`settings/resolve.go`, `Resolve()`) implement this rule independently. This file is what keeps
the two implementations from drifting: both run the exact same vectors.

The **C++ launcher repo is the canonical home** of this file. The Go SDK repo copies it in
(read-only, no independent edits) as part of its own conformance test suite.

This document is written so that a Go developer with no C++ context can write a conformant test
runner from this file alone.

## Format version

Top-level `formatVersion` (integer, currently `1`) lets either repo detect skew - if a consumer
does not recognize the format version it reads, it must fail loudly (not silently skip cases).

## Top-level shape

```json
{
  "formatVersion": 1,
  "dualHomedKeys": ["server-user", "enable-debug-logging", "scratch-path", "logging-dir",
                    "heartbeat-interval-seconds", "job-expiry-hours",
                    "plugin-metrics-interval-seconds"],
  "cases": [ /* ... */ ]
}
```

`dualHomedKeys` is the full list of registry keys every case's `expected` object must cover. It is
provided for self-description / iteration convenience; it is not itself an input to resolution.
All seven keys are dual-homed in every registry table that exists today (Local, Kubernetes,
Slurm carry identical tables) - see `src/cpp/job_launcher/include/launcher/SettingsRegistry.hpp`
and each plugin's `SettingsTable.cpp`. Do not add or remove keys here without updating those
tables and this file's cases together; the C++ runner asserts this list matches
`localSettingsRegistry()`'s key set.

## Case shape

Each entry of `cases` has this shape:

```json
{
  "name": "derivation-job-expiry-hours-equals-inherited-default",
  "description": "human-readable, optional - explains what the case pins",
  "ownConf": { "job-expiry-hours": "24" },
  "inherited": {
    "serverUser": "", "enableDebugLogging": false, "scratchPath": "", "loggingDir": "",
    "heartbeatIntervalSeconds": 5, "jobExpiryHours": 24.0, "pluginMetricsIntervalSeconds": 60,
    "includePluginMetricsIntervalSeconds": true
  },
  "expected": {
    "server-user": { "value": "", "provenance": "inherited" },
    "enable-debug-logging": { "value": "0", "provenance": "inherited" },
    "scratch-path": { "value": "", "provenance": "inherited" },
    "logging-dir": { "value": "", "provenance": "inherited" },
    "heartbeat-interval-seconds": { "value": "5", "provenance": "inherited" },
    "job-expiry-hours": { "value": "24", "provenance": "own-conf" },
    "plugin-metrics-interval-seconds": { "value": "60", "provenance": "inherited" }
  }
}
```

### `ownConf` - the plugin's own-conf layer

A JSON **object**, not an array or a separate presence-list-plus-value-map: **key presence in
this object is itself the presence signal**. A dual-homed key that is absent from `ownConf` was
not set in the plugin's own conf file at all. A key that IS present - even with an empty string
value - was explicitly set (including the "bare flag" case: an operator wrote a key with no
`=value` token at all; both C++ and the Go SDK treat that as present-with-empty-string).

**Do not encode presence as "non-empty value."** A key present with a value equal to the
default, and a key present with an empty value, are both legitimate "present" states distinct
from "absent." Collapsing "present with a falsy-looking value" into "absent" is exactly the bug
this whole fixture exists to prevent (see the `derivation-*` cases below).

This directly matches the Go SDK's `Resolve(registry, ownConfPresent map[string]string,
inherited)` signature - `ownConf` here IS `ownConfPresent`. On the C++ side, the runner splits
this one object into `resolve()`'s two parameters: the key set (`ownConfKeysPresent`) and the
value map (`ownConfValues`) - both are trivially derived from the same JSON object.

An absent `ownConf` key must never appear in the object at all (no `null` values - a JSON object
key that maps to `null` is not "absent" in this format and is not exercised by any case here).

### `inherited` - the launcher's cascade layer

A JSON object with the **exact wire keys** the launcher sends over IPC to a running plugin: this
is `InheritedSettings::toJson()`'s shape (`src/cpp/job_launcher/InheritedSettings.cpp`), and the
exact shape the Go SDK's `api.InheritedSettings` struct decodes via its own `json` tags
(`api/types.go` in the Go SDK repo). Field names:

| Field | Type | Notes |
|---|---|---|
| `serverUser` | string | |
| `enableDebugLogging` | bool | |
| `scratchPath` | string | |
| `loggingDir` | string | |
| `heartbeatIntervalSeconds` | integer | |
| `jobExpiryHours` | number | a float32 on the C++ side; encode as a JSON number, e.g. `1234.567`. See "Lossless job-expiry-hours" below for exactly what raw string a given number must produce. |
| `pluginMetricsIntervalSeconds` | integer | |
| `includePluginMetricsIntervalSeconds` | bool | when `false`, the wire form omits `plugin-metrics-interval-seconds` entirely - see the `plugin-metrics-interval-excluded-for-third-party-cluster` case. |

The Go SDK receives this object over IPC and has no `launcher.conf` parser of its own (the
launcher performs the `[server]`-to-cascade derivation, not the plugin) - so this object, not
launcher.conf text, is the cross-language contract for the inherited layer.

### `launcherConf` (reserved, not currently used)

The design allows an *optional*, C++-only companion field carrying literal `launcher.conf` text,
so the C++ runner can additionally verify that its own cascade (`resolveInheritedSettings()`)
produces the exact `inherited` object a case states, closing the loop between `launcher.conf` and
the wire object. This fixture file does not currently use it: doing so requires an OS user that
actually exists on the machine running the test (`Config::parseOnly()` verifies `server-user`
against the system's user database), which is either fragile across dev boxes/CI or requires a
placeholder-substitution convention this format does not otherwise need. The C++-side cascade
itself is already pinned directly by `src/cpp/job_launcher/tests/CascadeConfigTests.cpp` and
`src/cpp/job_launcher/tests/InheritedSettingsTests.cpp`. A future case MAY add this field; if it
does, the Go SDK ignores it (it has no launcher.conf parser to exercise).

### `expected` - the resolution outcome

A JSON object keyed by dual-homed setting key (matching `dualHomedKeys`/`SettingDescriptor::key`
exactly), one entry per key, no more and no fewer. Each entry has:

| Field | Type | Values | Meaning |
|---|---|---|---|
| `value` | string | e.g. `"24"`, `""` | The resolved raw value, in the same string form used on the wire/argv - `InheritedSettings::toArgs()` on the C++ side. Always a string regardless of the setting's canonical type (a bool renders as `"1"`/`"0"`, never JSON `true`/`false`). |
| `provenance` | string | `"own-conf"`, `"inherited"`, `"default"` | Where `value` came from. These are the exact strings the Go SDK's `Provenance.String()` already returns (`settings/resolve.go`) and the exact strings the C++ resolved-settings snapshot format uses (`docs/plugin-settings-registry.md`) - not the C++ enum's PascalCase spelling (`OwnConf`/`Inherited`/`Default`). A conformant runner maps between this string and its own language's enum/constant. |

#### The `default` provenance is reserved and currently unexercised

Every registry entry in every plugin's table today is dual-homed (`dualHomed: true` in
`SettingDescriptor`/`SettingType`). Because of that, an own-conf-absent key always falls through
to `inherited`, never to `default` - see the `Provenance::Default` doc comment in
`impls/SettingsResolver.hpp` and the mirrored comment in the Go SDK's `resolve.go`. **No case in
this file exercises `provenance: "default"`, and none should be added by inventing a fake
non-dual-homed key just to manufacture one** - that would test a registry shape that does not
exist in production. A future task that adds a genuinely non-dual-homed `SettingDescriptor` should
add the first real `default` case here, once there is an actual built-in-default value source to
assert on.

## The load-bearing cases

Two cases are the actual point of this fixture (the rest are supporting coverage):

- **`derivation-job-expiry-hours-equals-inherited-default`** and
  **`derivation-enable-debug-logging-equals-inherited-default`**: own-conf explicitly sets a key
  to a value that is *identical* to what the inherited layer would have supplied anyway. Expected
  provenance is `own-conf`, not `inherited`. This is the exact bug this design fixes - the
  pre-existing Kubernetes-plugin behavior conflated "operator explicitly chose the default value"
  with "operator didn't set it at all," silently misattributing provenance. A resolver that
  compares values instead of checking presence will fail these two cases and pass every other
  case in this file.
- **`bare-flag-own-conf-enable-debug-logging`**: own-conf carries a key with no value token at
  all (a bare flag). Both C++ (`parseOwnConfKeysInIsolation`) and the Go SDK (fixed in Go SDK
  commit `5f5a513`) must treat this as present with an empty raw string, not absent.

## KNOWN DIVERGENCE: malformed own-conf lines are not handled the same way

**Status: known and accepted, not fixed. A follow-up is tracked outside this branch. Do not treat
this as resolved, harmless, or a documentation nit.**

The `bare-flag-own-conf-enable-debug-logging` case above covers the one bare-flag spelling both
implementations agree on: `key=` (trailing equals, empty value) is present-with-`""` on both
sides. Underneath that agreement, the two own-conf parsers handle a genuinely malformed line -
specifically a bare key with **no** `=` at all (e.g. a literal `enable-debug-logging` line, no
equals sign) - differently:

- **C++** (`parseOwnConfKeysInIsolation`, `impls/SettingsResolver.cpp`) parses the plugin's own
  conf via `boost::program_options::parse_config_file`. A bare key with no `=` is not valid syntax
  for that parser and it throws. The function's catch block converts that exception into "no keys
  present" - but for the **entire file**, not just the offending line. Only `key=` (trailing
  equals) is the supported bare-flag spelling; a bare key with no `=` is not a supported spelling
  at all, it is a parse failure.
- **The Go SDK** (`IniOwnConfSource` in `settings/ownconf.go`, via `markBareKeysAsEmpty`) rewrites
  any line that is exactly one of the dual-homed keys (trimmed, not a comment, no `=` present)
  into `key=` **before** handing the file to its INI parser. A malformed/bare line for one key is
  isolated to that one key; it does not affect presence for any other key in the file.

**Observable consequence on the C++ side.** At reload, a single malformed line anywhere in the
plugin's own conf file makes `parseOwnConfKeysInIsolation` report **zero** keys present for that
file - not just the malformed one. Every dual-homed setting the operator explicitly set in that
own-conf file is therefore treated as absent, all of them resolve to `Inherited`, and the reload
applies `launcher.conf`'s cascade values over the operator's explicit own-conf choices - silently,
and the reload still reports success. This does not happen at startup: a plugin's normal own-conf
parse at startup is not best-effort the same way, and a malformed conf file stops the plugin
rather than silently discarding presence. So reload and startup can disagree about the same
malformed file - reload proceeds (wrongly) where startup would have failed loudly.

**Fixture coverage: none.** This class of divergence sits below what these fixtures can catch by
construction. The `ownConf` layer in this format is deliberately abstract (a key/value map with
presence encoded as JSON-key-presence - see the `ownConf` section above), precisely because the
plugin's own-conf file *format* is not part of what this fixture tests. Own-conf **text** parsing
- including how a parser reacts to a malformed line - happens entirely below this fixture's input
layer. No vector in `settings-resolver-conformance.json`, and no vector that could be added to it
in its current form, exercises this divergence. A future test for it would have to be a
C++-specific (and separately, Go-specific) unit test against a literal malformed own-conf file,
not a shared fixture case.

## Lossless `job-expiry-hours`

Three cases (`job-expiry-hours-lossless-*`) pin that `jobExpiryHours` round-trips through its
wire string form without losing precision. C++ formats this field with
`std::numeric_limits<float>::max_digits10` (9 significant digits, effectively `%.9g`) rather than
the default 6-significant-digit float formatting - see `formatJobExpiryHoursLossless()` in
`src/cpp/job_launcher/InheritedSettings.cpp`. `1234.567` and `0.5` are the values already pinned
in C++'s own `InheritedSettingsTests.cpp`; `0.1` is a new, more adversarial vector added here
specifically because it is not exactly representable in binary floating point, so its nearest
float32 carries visible rounding noise (`0.100000001`) - any precision or rounding-mode
difference between the two implementations' formatters shows up immediately on this value.

If you are implementing a Go-side runner: take the JSON number for `inherited.jobExpiryHours` as
a `float64`, narrow it to `float32` (matching how the C++ side's `InheritedSettings::fromJson()`
narrows a JSON double to its `float` field), then format with the equivalent of C's `%.9g` (Go:
`strconv.FormatFloat(f, 'g', 9, 32)` produces the same digit sequence, though you may need to
adjust exponent/case conventions to match `%g`'s output exactly - verify against the vectors here
rather than assuming).

## Case ordering and evolution

Cases are not required to run in file order, and case order carries no meaning. Do not remove or
reinterpret an existing case's meaning when adding new ones - add a new case instead. A change to
an existing case's expected output is a behavior change to the underlying resolution rule and
must be treated as such (coordinated update to both this file and both consuming repos), not a
routine test edit.
