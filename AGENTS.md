# AGENTS.md

## Project Overview

Go SDK for building Launcher plugins that integrate job schedulers (Slurm, Kubernetes, PBS, cloud platforms, etc.) with Posit Workbench and Posit Connect. Implements Launcher Plugin API v3.5.0.

**Language:** Go 1.24+
**License:** MIT
**Module:** `github.com/posit-dev/launcher-go-sdk`
**Status:** Pre-1.0 (v0.1.0) — API may change in minor versions

## Build & Test Commands

Requires [just](https://github.com/casey/just) command runner.

```bash
just install-tools      # Install golangci-lint and goimports
just test               # Run all tests
just test-race          # Tests with race detector
just test-coverage      # Tests with coverage report
just lint               # Run golangci-lint
just lint-fix           # Lint with auto-fix
just fmt                # Format code (gofmt + goimports)
just fmt-check          # Check formatting without modifying
just build              # Build all packages
just build-examples     # Build example plugins
just check              # fmt-check + lint + test
just ci                 # Full CI: check + build + build-examples
just pre-commit         # Fast checks: fmt + lint
just pre-push           # Comprehensive: check + build-examples
just security           # Run govulncheck
just bench              # Run benchmarks
```

## Package Structure

| Package | Purpose |
|---------|---------|
| `api/` | Type definitions for Launcher Plugin API (Job, JobFilter, errors, etc.) |
| `launcher/` | Core plugin interface (`Plugin`) and runtime — the main SDK entry point |
| `cache/` | Thread-safe job storage with pub/sub for status updates (in-memory or BoltDB) |
| `logger/` | Workbench-style structured logging via `log/slog` |
| `conformance/` | Automated behavioral tests to verify plugin compliance |
| `plugintest/` | Testing utilities: mock writers, builders, assertions |
| `internal/protocol/` | Wire protocol over stdin/stdout (not public API) |
| `cmd/smoketest/` | Smoke test utility for plugin testing |
| `examples/inmemory/` | Complete in-memory example plugin |
| `examples/scheduler/` | Design guide for CLI-based scheduler plugins |
| `docs/` | GUIDE.md, ARCHITECTURE.md, API.md, TESTING.md |

## Code Style

- **Formatting:** `gofmt` + `goimports` with local prefix `github.com/posit-dev/launcher-go-sdk`
- **Linting:** golangci-lint v2 config in `.golangci.yml` — SDK correctness over cosmetics
- **Key enabled linters:** errcheck, govet, staticcheck, unused, revive, errorlint, gosec, gocritic, exhaustive, nilerr, nilnil
- **Naming:** `New*` constructors, `Must*` for panic-on-error init, short receiver names
- **Errors:** Use `api.Errorf(code, fmt, args...)` for structured errors; wrap with `fmt.Errorf("context: %w", err)`
- **Tests:** Standard library `testing` only, table-driven tests, subtests with `t.Run`, `t.Helper()` in helpers
- **Concurrency:** `sync.Mutex`/`sync.RWMutex` for shared state, context-based cancellation, thread-safe response writers
- **Documentation:** All exported types and functions must be documented; comments start with the identifier name

## Key Dependencies

- `go.etcd.io/bbolt` — Persistent job storage backend
- `golang.org/x/tools` — Go tooling (goimports)
- `golangci-lint` — Dev-only via `tools.go` build tag

## Available Tools

These CLI tools are installed and available for use:

- **`goimports`** — Use for formatting imports with local prefix `github.com/posit-dev/launcher-go-sdk`. Prefer over `gofmt` alone.
- **`difft`** (difftastic) — Structural diff that understands Go syntax. Use `difft file1 file2` or `GIT_EXTERNAL_DIFF=difft git diff` for more meaningful diff output.
- **`yq`** — YAML processor (jq for YAML). Use for querying/modifying `.golangci.yml`, GitHub Actions workflows, or any YAML config files.

## CI

GitHub Actions workflows in `.github/workflows/`:
- **test.yml** — Matrix: ubuntu/macos × Go 1.24/1.25, race detector, coverage (min 15%), Codecov
- **lint.yml** — golangci-lint, gofmt check, go vet
- **examples.yml** — Build examples, verify READMEs
- **release.yml** — On `v*` tags: test, build, create GitHub release, trigger pkg.go.dev indexing
