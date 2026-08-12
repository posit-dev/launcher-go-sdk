# Provenance of the settings-resolver-conformance fixture

`settings-resolver-conformance.json` and `settings-resolver-conformance.md` in this directory are
**verbatim copies**, not independent files.

- **Canonical home:** the C++ launcher repo,
  `docs/fixtures/settings-resolver-conformance.json` and `.md`
  (`github.com/rstudio/launcher`, path `docs/fixtures/`).
- **Copied from commit:** `57dfb17ab3e1661fb4bfc75818b837c623b1ccfe` (2026-08-12), branch
  `dan/hot-reload-dual-homed-settings`.
- **Format version at copy time:** `1` (see `formatVersion` in the JSON file).
- This repo's copy is **read-only**: do not hand-edit either file here. A change to the shared
  precedence/provenance rule must land in the C++ repo first, then be re-copied into both this
  directory and (unchanged) into the C++ repo's own runner — see the `.md` file's "Case ordering
  and evolution" section for what counts as a case-meaning change versus a routine addition.
- If `../resolve_conformance_test.go` reports an unrecognized `formatVersion`, that means the two
  repos' copies have drifted (one side bumped the format, the other didn't) — do not silently
  bump this copy's expectations to match; re-sync deliberately, updating this file's "Copied from
  commit" line.
- A silently-diverging copy is worse than no copy at all, which is why the test runner fails
  loudly (not skips) on a missing file, a parse error, an unrecognized format version, or a
  zero-length case list.

## Resolved cross-implementation disagreement (fixed in the Fix Round 1 pass)

Task 19b originally found that two vectors — `job-expiry-hours-lossless-1234.567` and
`job-expiry-hours-lossless-0.1-adversarial` — failed against the Go SDK's
`FormatJobExpiryHoursLossless` (`../inherited.go`), because it formatted the full float64 value
instead of narrowing to float32 first like the C++ `formatJobExpiryHoursLossless()` it mirrors.
That was reported rather than silently fixed, per the task's "stop and report" instruction. The
controller ruled: narrow to float32 in the Go function (see its doc comment in `../inherited.go`
for the full rationale — a plugin's inherited job-expiry value always originates from the
Launcher's 32-bit float, so float32 is the most precision either side can ever express). Both
vectors now pass for real; see `.superpowers/sdd/task19b-report.md`'s Fix Round 1 section for the
before/after evidence. There is no longer a known disagreement in this fixture.
