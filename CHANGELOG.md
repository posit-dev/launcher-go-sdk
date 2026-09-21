# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Plugin API 3.10.0**: Protocol/version parity for the Launcher's hot-reload of dual-homed `[server]` settings. `api.InheritedSettings` mirrors the Launcher's inherited-settings payload (`serverUser`, `enableDebugLogging`, `scratchPath`, `loggingDir`, `heartbeatIntervalSeconds`, `jobExpiryHours`, `pluginMetricsIntervalSeconds`, `includePluginMetricsIntervalSeconds`). `protocol.ConfigReloadRequest` gains `InheritedSettings` (a presence-aware `*api.InheritedSettings`, nil when the Launcher omits it) and `Generation`; `protocol.ConfigReloadResponse` gains `Applied`, `PendingRestart`, and `Generation`. `api.ConfigReloadErrorType` gains `ReloadErrorRequestNotSupported`. See the new `settings` package (below) for what actually consumes this wire shape.
- **New `settings` package**: resolves the Launcher's dual-homed `[server]` settings — the seven keys that can arrive both via the Launcher's cascaded CLI args at spawn and via a plugin's own config file (`job-expiry-hours`, `logging-dir`, `enable-debug-logging`, `server-user`, `scratch-path`, `heartbeat-interval-seconds`, `plugin-metrics-interval-seconds`) — and reports the outcome of a config reload back to the Launcher. `settings.Registry` classifies each key `Reloadable` (can be applied to an already-running plugin: `job-expiry-hours`, `logging-dir`, `enable-debug-logging`) or `RestartRequired` (only takes effect on next start: the other four). `settings.Resolve` implements presence-based precedence — own-conf beats inherited beats default, decided by map-key presence, never by comparing values, so an operator's own-conf value that happens to equal the inherited default still wins as `ProvenanceOwnConf`. `settings.NewReloader` builds an atomically-swapped `Reloader` that plugins construct once at startup and reuse across every reload; `Reloader.Reload` validates, applies `job-expiry-hours` (readable live via `Reloader.JobExpiry()`), and computes `Applied`/`PendingRestart` against the plugin's startup baseline (not the mutable cache), so a later reload that restores the original startup value correctly reports "no longer pending restart."
- **New `settings.OwnConfSource` interface**: implement this against your own plugin's config format to tell the resolver which dual-homed keys are textually present in it (`OwnConfKeys() map[string]string`; a key's *presence*, not its value, drives precedence). If your own config happens to be a flat INI-style file like the Launcher's own `.conf` format, use `settings.IniOwnConfSource{Path: ...}` instead of writing your own — it defaults `Keys` to the registry's seven keys and matches the Launcher's own bare-flag semantics (a flag with no `=value` resolves to `""`, not `"true"`).
- **New `launcher.SettingsReloadablePlugin` interface**: implement `SettingsReloader() *settings.Reloader` to have the SDK automatically resolve, apply, and report the dual-homed settings on every config reload. This is one of **two** reload-related interfaces now, and they are additive, not interchangeable or mutually exclusive: `ConfigReloadablePlugin.ReloadConfig` is a plain hook for your own reload work (profiles, etc.) with no dual-homed-settings involvement; `SettingsReloadablePlugin` handles dual-homed settings via the returned `Reloader` (and its own `ApplyExtra` hook, if preferred). Implementing both runs both, in that order, on every reload — the settings reload never silently suppresses `ReloadConfig`, or vice versa. `launcher.DefaultOptions` gains an exported `JobExpiryHours` field and an `InheritedSettings(includePluginMetricsIntervalSeconds bool) api.InheritedSettings` helper so a plugin can seed a `Reloader` directly from its own parsed options instead of hand-assembling the seed value.
- New reload conformance area in `conformance/`: `conformance.RunSettingsRegistry` pins the SDK's own `settings.Registry` classification against the Launcher's, and `conformance.RunReload` drives a plugin through its real reload wiring (interface detection, `RequestNotSupported` reporting, applied/pending-restart classification, no-partial-apply on a rejected reload, and presence-aware cache handling so an absent `InheritedSettings` push doesn't clobber previously-cached values) so plugin authors can verify their own compliance.
- Cross-SDK conformance fixtures under `settings/testdata/` (`settings-resolver-conformance.json`/`.md`, plus `PROVENANCE.md`): a byte-identical copy of the fixture the C++ Launcher's own resolver conformance runner uses, verified here against `settings.Resolve` directly. **The C++ launcher repo (`docs/fixtures/`) is canonical** — this copy is read-only; the two copies must be updated together, and a diverging `formatVersion` between them is treated as a hard failure, not something to paper over locally.
- **Plugin API 3.9.0**: `Job` gains an optional `SystemJob` field (`"systemJob"` on the wire). Marks a system/utility job that bypasses resource-profile enforcement; only honored on privileged requests.
- **Plugin API 3.8.0**: `PlacementConstraint` gains an optional `Default` field (`"defaultValue"` on the wire). The Launcher uses this to surface a pre-fill hint for constraint inputs in Workbench.

### Changed
- **Config reload now reports `RequestNotSupported` instead of silently succeeding when a plugin implements no reload interface.** A plugin implementing neither `ConfigReloadablePlugin` nor `SettingsReloadablePlugin` previously received `ReloadErrorNone` on a config reload request — reporting success without having reloaded anything. It now receives `api.ReloadErrorRequestNotSupported`. If your plugin relies on the old silent-success response (deliberately or not), you will see a different `errorType` on the wire after upgrading; no Go-level API changed, so this does not require code changes to compile, but it is a real, intentional runtime-behavior change — not marked **BREAKING** here because this file reserves that marker for changes that break compilation of existing plugin code, and this change is compile-transparent. Plugins that don't implement either interface and want the previous behavior should implement `ConfigReloadablePlugin` with a no-op `ReloadConfig` that returns `nil`.
- **`job-expiry-hours` is now parsed as `float64`** (was `uint`), so fractional hours (e.g. `0.5` for a 30-minute expiry) parse correctly instead of aborting the plugin at startup ([#54](https://github.com/posit-dev/launcher-go-sdk/issues/54)).
- `ConfigReloadResponse`'s `applied`, `pendingRestart`, and `generation` fields are now always present on the wire (previously omitted when empty/zero), matching what the C++ Launcher emits, so "nothing applied" and "this plugin doesn't report applied at all" are no longer wire-indistinguishable.
- The inherited `job-expiry-hours` raw string (as seen via `Reloader.LastResolved()`) is now formatted at `float32` precision to agree byte-for-byte with the C++ Launcher, which stores this setting as a 32-bit float — this only affects the string's last few digits on values with float32 rounding noise (e.g. `0.1` renders as `0.100000001`) and matters for cross-implementation agreement, not for typical use.
- **BREAKING**: `cache.NewJobCache` no longer accepts a `context.Context` parameter. The context implied the cache reacted to cancellation, but `Close()` was always required and sufficient to stop it. `Close()` is now the sole lifecycle control; callers must call it to release resources and stop the cache's internal goroutine.
- **Minimum Go version is now 1.25** (was 1.24). Go 1.24 is no longer supported; the `go` directive in `go.mod` is now `1.25.0`.

### Known limitations
- **Go SDK plugins cannot apply `logging-dir` / `enable-debug-logging` on reload.** Both keys are correctly classified `Reloadable` and correctly resolved (value + provenance) by the `settings` package, but the SDK's `logger` package has no runtime reconfiguration surface (no adjustable level, no swappable/reopenable sink), so neither key is ever added to a reload's `Applied` list — the C++ plugins do apply them, so this is a real, currently-open parity gap. If you need to react to a change in either key, read it yourself via `Reloader.LastResolved()` (e.g. from your `Reloader.ApplyExtra` hook).
- **The Go SDK writes no `.active` last-known-good artifacts.** The C++ Launcher and its plugins persist a resolved-settings snapshot (and own-conf/launcher.conf copies) as hidden `.active` files for crash recovery; the Go SDK has no equivalent today.

### CI
- Test matrix now runs against Go 1.25 and 1.26 (dropped 1.24, added 1.26).

### Fixed
- Goroutine leak in `cache.JobCache` when the passed context was canceled without calling `Close()` ([#18](https://github.com/posit-dev/launcher-go-sdk/issues/18)); the context parameter has been removed

## [0.2.0] - 2026-05-27

### Added
- **Plugin API 3.7.0**: Plugin metrics framework. Plugins can now report periodic metrics to the Launcher for Prometheus exposition.
  - `MetricsPlugin` optional interface in `launcher` package — plugins implement `Metrics(ctx context.Context) PluginMetrics` to report custom metrics
  - `PluginMetrics` struct with `ClusterInteractionLatency` field for scheduler command latency histograms
  - `Histogram` type for thread-safe metric accumulation with `Observe()` and `Drain()` methods
  - `ClusterInteractionLatencyBuckets` variable with standard bucket boundaries matching the Launcher
  - `MetricsInterval` field on `Runtime` and `DefaultOptions` for configuring the collection interval
  - `--plugin-metrics-interval-seconds` CLI flag (default: 60, 0 to disable)
  - Framework automatically reports `uptimeSeconds`; custom metrics are opt-in via `MetricsPlugin`
  - `RunMetrics` conformance test scenario for validating `MetricsPlugin` implementations
- Protocol support for metrics response (message type 203) in `internal/protocol`
- New dependency: `github.com/prometheus/client_golang` for histogram accumulation
- **Plugin API 3.6.0**: Config reload support. The Launcher can now request plugins to reload configuration at runtime without restarting.
  - `ConfigReloadablePlugin` optional interface in `launcher` package — plugins implement `ReloadConfig(ctx context.Context) error` to handle reload requests
  - `ConfigReloadError` type for classified reload failures (Load, Validate, Save)
  - `ConfigReloadErrorType` enum in `api` package with `String()` method
  - Plugins that do not implement `ConfigReloadablePlugin` automatically send a success response
  - `MockResponseWriter.WriteConfigReload` and `ConfigReloadResult` in `plugintest` for testing reload implementations
- Protocol support for config reload request/response (message type 202) in `internal/protocol`
- Exported `protocol.RequestFromJSON` for use in handler tests
- Unit tests for `internal/protocol` and `launcher` packages

### Changed
- **BREAKING**: `cache.NewJobCache` no longer accepts a `dir` parameter. The SDK now defaults to in-memory caching, which aligns with how Launcher plugins are expected to work: the scheduler owns job state, and plugins populate the cache during `Bootstrap()` and keep it in sync via periodic polling.
- **BREAKING**: Add `context.Context` as the first parameter to all non-streaming `Plugin` methods (`SubmitJob`, `GetJob`, `GetJobs`, `ControlJob`, `GetJobNetwork`, `ClusterInfo`) and extension interfaces (`Bootstrap`, `GetClusters`). Streaming methods already accepted context.
- **BREAKING**: `Job.ID` type changed from `string` to `api.JobID` for end-to-end type safety. Since `api.JobID` is a named `string` type, JSON serialization and literal assignments work unchanged.
- **BREAKING**: Cache public methods (`Lookup`, `Update`, `WriteJob`, `RunningJobContext`, `StreamJobStatus`) now accept `api.JobID` instead of `string`.
- **BREAKING**: Conformance and plugintest helpers updated to use `api.JobID` (`SubmitJob` returns `api.JobID`; `GetJob`, `ControlJob`, `WaitForStatus`, `FindJobByID`, `AssertJobID`, `NewJobWithID`, `WithID` accept `api.JobID`).
- Replace `goto`-based poll loops with idiomatic `for`+`select` loops in cache and protocol packages
- Add panic recovery to cache background goroutine
- Use non-blocking channel sends to prevent deadlocks under load
- Add nil guards to stream `ResponseWriter` methods
- Convert `Prune` to range-over-func syntax

### Fixed
- File handle leak in logger when debug log creation fails
- Race window in `RunningJobContext` with post-subscribe recheck
- JSON unmarshal error in `requestFromJSON` now handled instead of silently discarded
- Go version requirement corrected from 1.25 to 1.24 in README
- `WithMemory()` reference corrected to `WithLimit()` in CONTRIBUTING.md

### Removed
- BoltDB (`go.etcd.io/bbolt`) dependency — in-memory caching is now the standard approach

## [0.1.0] - 2024-02-05

### Added

#### Core SDK
- Initial release of the Launcher Go SDK
- `launcher` package with Plugin interface and Runtime
- `api` package with complete type definitions matching Launcher Plugin API v3.5
- `cache` package with JobCache for job storage and pub/sub
- `logger` package for Posit Workbench-style structured logging
- `internal/protocol` package for JSON-based wire protocol over stdin/stdout

#### Plugin Interface
- 10 required methods for plugin implementation:
  - `SubmitJob` - Accept new job submissions
  - `GetJob` - Return single job information
  - `GetJobs` - Return multiple jobs with filtering
  - `ControlJob` - Control job operations (stop, kill, cancel)
  - `GetJobStatus` - Stream status updates for a job
  - `GetJobStatuses` - Stream status updates for all jobs
  - `GetJobOutput` - Stream job stdout/stderr
  - `GetJobResourceUtil` - Stream CPU/memory usage
  - `GetJobNetwork` - Return network information
  - `ClusterInfo` - Return cluster capabilities

#### Response Writers
- `ResponseWriter` interface for single-response methods
- `StreamResponseWriter` interface for streaming methods
- Thread-safe implementations for concurrent access

#### Job Cache
- In-memory storage backend
- User permission enforcement
- Pub/sub for job status updates
- Automatic job expiration
- Helper methods for writing to ResponseWriters
- Atomic job updates with callback pattern

#### Testing Utilities (`plugintest` package)
- `MockResponseWriter` - Capture plugin responses for assertions
- `MockStreamResponseWriter` - Capture streaming responses
- `JobBuilder` - Fluent API for building test jobs
- `JobFilterBuilder` - Fluent API for building job filters
- `ClusterOptionsBuilder` - Fluent API for building cluster options
- 25+ assertion helpers with clear error messages
- Helper functions for finding and filtering jobs

#### Configuration
- `DefaultOptions` with standard Launcher flags
- Support for custom plugin-specific configuration
- Command-line flag parsing and validation

#### Examples
- **In-Memory Example** (~400 lines) - Complete plugin with job lifecycle simulation
- **Scheduler Design Guide** (`examples/scheduler/README.md`) - Guide for adapting the SDK to CLI-based schedulers (Slurm, PBS, LSF)
- Each example includes a detailed README

#### Documentation
- Comprehensive README with quick start and overview
- **Developer Guide** (`docs/GUIDE.md`) - Complete guide to building plugins
- **Architecture** (`docs/ARCHITECTURE.md`) - Design decisions and patterns
- **API Reference** (`docs/API.md`) - Complete API documentation
- **Testing Guide** (`docs/TESTING.md`) - Testing strategies and best practices
- Contributing guidelines (`CONTRIBUTING.md`)

### Technical Details

- **Go Version**: Requires Go 1.25 or later
- **API Version**: Implements Launcher Plugin API v3.5.0
- **Dependencies**: No external runtime dependencies
- **License**: MIT

### Stability

This is a pre-1.0 release (v0.x). The API may change in minor version updates. We will document breaking changes with migration guides.

[Unreleased]: https://github.com/posit-dev/launcher-go-sdk/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/posit-dev/launcher-go-sdk/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/posit-dev/launcher-go-sdk/releases/tag/v0.1.0
