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

## Known cross-implementation disagreement (open, not a copy defect)

As of the above commit, `../resolve_conformance_test.go` reports two failing vectors:
`job-expiry-hours-lossless-1234.567` and `job-expiry-hours-lossless-0.1-adversarial`. This is a
real behavioral difference between the Go SDK's `FormatJobExpiryHoursLossless`
(`../inherited.go`) and the C++ `formatJobExpiryHoursLossless()` it is documented to mirror — see
task19b-report.md in this repo's `.superpowers/sdd/` for the full analysis. It is **not** a fixture
or copy problem, and per that report it has deliberately not been fixed on either side pending a
routing decision. Do not "fix" it by editing this fixture.
