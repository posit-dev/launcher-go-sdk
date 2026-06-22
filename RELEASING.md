# Releasing

## Prerequisites

- Write access to the `posit-dev/launcher-go-sdk` repository (required to trigger
  `workflow_dispatch`).

## Normal release flow

1. **Update `CHANGELOG.md`**

   Move all content under `## [Unreleased]` to a new versioned section:

   ```
   ## [Unreleased]

   ## [0.3.0] - YYYY-MM-DD

   ### Added
   ...
   ```

   Also update the reference links at the bottom of `CHANGELOG.md` — change
   `[Unreleased]` to compare against the new version and add a `[0.3.0]` entry:

   ```
   [Unreleased]: https://github.com/posit-dev/launcher-go-sdk/compare/v0.3.0...HEAD
   [0.3.0]: https://github.com/posit-dev/launcher-go-sdk/compare/v0.2.0...v0.3.0
   ```

   Leave the empty `## [Unreleased]` section at the top for the next development cycle.
   Commit and merge to `main` before proceeding.

2. **Trigger the release workflow**

   Go to **Actions → Release → Run workflow**, enter the version (e.g. `v0.3.0`),
   and click **Run workflow**. The **Run workflow** button only appears when the
   workflow file exists on `main` (the default branch) — merge your CHANGELOG update
   before triggering.

   The workflow will:
   - Verify a matching `CHANGELOG.md` section exists (fails with a clear error if not;
     this check applies to pre-release versions too — see [Pre-releases](#pre-releases))
   - Run tests and build examples
   - Create and push the git tag
   - Create the GitHub Release with the changelog section as the release notes
   - Trigger `pkg.go.dev` to index the new version

3. **Verify**

   - Confirm the [GitHub Release](https://github.com/posit-dev/launcher-go-sdk/releases)
     was created with the correct notes.
   - Check `https://pkg.go.dev/github.com/posit-dev/launcher-go-sdk@vX.Y.Z` within a
     few minutes (indexing is usually fast but can take up to several hours in rare
     cases). If the page does not appear, visiting it directly in a browser triggers
     indexing on demand.

## Alternative: direct tag push

If you prefer the command line:

```bash
# Ensure CHANGELOG.md is already updated and merged to main.
git fetch origin
git checkout main && git pull origin main
git tag v0.3.0
git push origin v0.3.0
```

This skips the CHANGELOG validation guard — use it only if you have confirmed the
changelog is correct. Tests and example builds still run; only the changelog
pre-validation is skipped. The `push: tags` trigger in the release workflow will fire
and create the GitHub Release automatically.

## Troubleshooting

**`workflow_dispatch` failed after the tag was pushed**

If the release workflow fails after the "Create and push tag" step (e.g. the GitHub
Release step errors), rerunning via `workflow_dispatch` will hit a "tag already exists"
error. Use the direct tag push path to recover — the tag already exists on origin, so
just re-push it to trigger the `push: tags` workflow:

```bash
git push origin v0.3.0
```

## Versioning policy

This project follows [Semantic Versioning](https://semver.org/).

| Phase | Rule |
|-------|------|
| `v0.x.x` (current) | No API stability guarantee. Breaking changes bump the **minor** version (`v0.2.0` → `v0.3.0`). |
| `v1.0.0`+ | API stability contract. Breaking changes require a major version bump **and** a new module path suffix (`/v2`, `/v3`, …) per Go module conventions. |

Always prefix tags with `v` — Go tooling requires `vX.Y.Z`, not `X.Y.Z`.

## Pre-releases

For testing a significant change before a stable release, use the direct tag push
path (not `workflow_dispatch`):

```bash
git tag v0.3.0-rc.1
git push origin v0.3.0-rc.1
```

The `workflow_dispatch` path validates that a matching `CHANGELOG.md` section exists
before proceeding, and will fail for pre-release versions that don't have one. The
direct tag push path skips that validation; if no CHANGELOG section is found, the
release notes fall back to a generic message rather than failing.

The release workflow automatically marks any version containing `-` as a GitHub
pre-release. Pre-releases are visible on the releases page but not selected as the
latest stable release.
