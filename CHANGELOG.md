# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/posit-dev/launcher-go-sdk/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/posit-dev/launcher-go-sdk/releases/tag/v0.1.0
