# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- **BREAKING**: `cache.NewJobCache` no longer accepts a `dir` parameter. The SDK now defaults to in-memory caching, which aligns with how Launcher plugins are expected to work: the scheduler owns job state, and plugins populate the cache during `Bootstrap()` and keep it in sync via periodic polling.

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
