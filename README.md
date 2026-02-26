# Launcher Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/posit-dev/launcher-go-sdk.svg)](https://pkg.go.dev/github.com/posit-dev/launcher-go-sdk)
[![Go Report Card](https://goreportcard.com/badge/github.com/posit-dev/launcher-go-sdk)](https://goreportcard.com/report/github.com/posit-dev/launcher-go-sdk)

A Go Software Development Kit (SDK) for building launcher plugins that integrate job schedulers with [Posit Workbench](https://posit.co/products/enterprise/workbench/) and [Posit Connect](https://posit.co/products/enterprise/connect/).

## Overview

The Launcher Go SDK provides a complete framework for building plugins that connect Workbench and Connect to various job schedulers and execution environments. While Posit offers generally available integrations for Slurm and Kubernetes, this SDK enables you to integrate:

- **High-Performance Computing (HPC) Schedulers**: Slurm, Portable Batch System (PBS), Load Sharing Facility (LSF), Sun Grid Engine (SGE), HTCondor
- **Container Orchestrators**: Kubernetes (custom configurations)
- **Cloud Platforms**: AWS Batch, Azure Batch, Google Cloud Batch
- **Custom Schedulers**: Any system that can execute jobs

## Features

- Complete plugin framework - Implement the `Plugin` interface and let the SDK handle the protocol
- Job state management - Built-in job cache with pub/sub for status updates
- Streaming support - Stream job output, status, and resource utilization
- Conformance testing - Automated behavioral tests that verify your plugin against Posit product contracts
- Testing utilities - Mock response writers, job builders, and assertion helpers
- Comprehensive examples - In-memory example and scheduler design guide
- Type-safe API - Strongly typed interfaces matching the Launcher Plugin API v3.5

## Quick start

### Installation

```bash
go get github.com/posit-dev/launcher-go-sdk
```

### Your first plugin

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "sync/atomic"
    "syscall"

    "github.com/posit-dev/launcher-go-sdk/api"
    "github.com/posit-dev/launcher-go-sdk/cache"
    "github.com/posit-dev/launcher-go-sdk/launcher"
    "github.com/posit-dev/launcher-go-sdk/logger"
)

type MyPlugin struct {
    cache  *cache.JobCache
    nextID int32
}

func (p *MyPlugin) SubmitJob(w launcher.ResponseWriter, user string, job *api.Job) {
    job.ID = fmt.Sprintf("job-%d", atomic.AddInt32(&p.nextID, 1))
    job.Status = api.StatusPending
    if err := p.cache.AddOrUpdate(job); err != nil {
        w.WriteError(err)
        return
    }
    p.cache.WriteJob(w, user, job.ID)
}

func (p *MyPlugin) GetJob(w launcher.ResponseWriter, user string, id api.JobID, fields []string) {
    p.cache.WriteJob(w, user, string(id))
}

// ... implement other Plugin methods ...

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    options := &launcher.DefaultOptions{}
    launcher.MustLoadOptions(options, "myplugin")

    lgr := logger.MustNewLogger("myplugin", options.Debug, options.LoggingDir)
    jobCache, _ := cache.NewJobCache(ctx, lgr)

    plugin := &MyPlugin{cache: jobCache}
    launcher.NewRuntime(lgr, plugin).Run(ctx)
}
```

See [examples/inmemory](examples/inmemory) for a complete working example with job lifecycle simulation, streaming, and conformance tests.

## Documentation

- **[Plugin Developer Guide](docs/GUIDE.md)** - Comprehensive guide to building plugins
- **[Architecture](docs/ARCHITECTURE.md)** - Design decisions and patterns
- **[API Reference](docs/API.md)** - Complete API documentation
- **[Testing Guide](docs/TESTING.md)** - How to test your plugin

## Examples

The SDK includes examples to help you get started:

1. **[In-Memory](examples/inmemory)** (~400 lines) - Complete plugin with job lifecycle simulation, conformance tests, and deployment guidance
2. **[Scheduler](examples/scheduler)** - Design guide for adapting the SDK to CLI-based schedulers (Slurm, PBS, LSF, etc.)

## Core concepts

### Plugin interface

All plugins implement the `launcher.Plugin` interface:

```go
type Plugin interface {
    SubmitJob(w ResponseWriter, user string, job *api.Job)
    GetJob(w ResponseWriter, user string, id api.JobID, fields []string)
    GetJobs(w ResponseWriter, user string, filter *api.JobFilter, fields []string)
    ControlJob(w ResponseWriter, user string, id api.JobID, op api.JobOperation)
    GetJobStatus(ctx context.Context, w StreamResponseWriter, user string, id api.JobID)
    GetJobStatuses(ctx context.Context, w StreamResponseWriter, user string)
    GetJobOutput(ctx context.Context, w StreamResponseWriter, user string, id api.JobID, outputType api.JobOutput)
    GetJobResourceUtil(ctx context.Context, w StreamResponseWriter, user string, id api.JobID)
    GetJobNetwork(w ResponseWriter, user string, id api.JobID)
    ClusterInfo(w ResponseWriter, user string)
}
```

### Job cache

The SDK provides a job cache for storing and querying jobs:

```go
cache, err := cache.NewJobCache(ctx, logger)

// Store a job
cache.AddOrUpdate(job)

// Query jobs
cache.WriteJob(w, user, jobID)
cache.WriteJobs(w, user, filter)

// Stream status updates
cache.StreamJobStatus(ctx, w, user, jobID)
```

### Testing utilities

Write tests using the provided mocks and builders:

```go
import "github.com/posit-dev/launcher-go-sdk/plugintest"

func TestSubmitJob(t *testing.T) {
    w := plugintest.NewMockResponseWriter()
    plugin := &MyPlugin{cache: testCache}

    job := plugintest.NewJob().
        WithUser("alice").
        WithCommand("echo hello").
        Build()

    plugin.SubmitJob(w, "alice", job)

    plugintest.AssertNoError(t, w)
    plugintest.AssertJobCount(t, w, 1)
}
```

## Package overview

- **`launcher`** - Core plugin interfaces and runtime
- **`api`** - Job types, error codes, and API types
- **`cache`** - Job storage with pub/sub for status updates
- **`logger`** - Workbench-style logging utilities
- **`conformance`** - Automated behavioral tests against Posit product contracts
- **`plugintest`** - Testing utilities (mocks, builders, assertions)
- **`internal/protocol`** - Wire protocol implementation (not exposed)

## API version

This SDK implements **Launcher Plugin API v3.5.0**.

## Stability

Current status: Pre-1.0 (v0.x) - API may change

While in v0.x versions, minor releases (v0.1 → v0.2) may include breaking changes. We'll provide migration guides and detailed CHANGELOG entries for all breaking changes.

Once we release v1.0.0, we will strictly maintain backwards compatibility within major versions following [Semantic Versioning](https://semver.org/).

## Requirements

- Go 1.25 or later
- Linux, macOS (we test on Linux and macOS)
- Workbench 2023.09.0 or later or Connect 2024.08.0 or later (for deployment)

## Development

### Building the SDK

```bash
git clone https://github.com/posit-dev/launcher-go-sdk.git
cd launcher-go-sdk
go build ./...
```

### Using Just (recommended)

This project uses [`just`](https://github.com/casey/just) as a command runner for common development tasks:

```bash
# See all available commands
just --list

# Common commands
just test                 # Run tests
just test-coverage        # Run tests with coverage
just lint                 # Run linter
just fmt                  # Format code
just build-examples       # Build all examples
just pre-commit           # Quick checks before committing
```

Install development tools:
```bash
just install-tools          # golangci-lint, goimports
brew install difftastic yq  # structural diffs, YAML processing
```

### Running tests

```bash
just test
# or
go test ./...
```

### Building examples

```bash
just build-examples
# or manually
cd examples/inmemory
go build
./inmemory --enable-debug-logging
```

## Architecture

The Launcher is a REST API service that provides a generic interface between Posit products and job scheduling systems. It exposes an HTTP API to Workbench and Connect, handles authentication, and forwards requests to plugins via a JSON protocol over stdin/stdout.

```
┌─────────────────────────────────────┐
│  Posit Workbench / Posit Connect    │  ← Product layer (HTTP API)
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│  Launcher (Job Launcher Service)    │  ← Launcher service (auth, routing)
└──────────────┬──────────────────────┘
               │ JSON over
               │ stdin/stdout
┌──────────────▼──────────────────────┐
│  Internal Protocol Layer            │  ← internal/protocol
│  - Request/response serialization   │
│  - Message framing                  │
│  - Stream management                │
│  - stdin/stdout communication       │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│  Public SDK Layer                   │  ← launcher, api, cache, logger
│  - Plugin interface                 │
│  - Job cache                        │
│  - Response writers                 │
│  - Type definitions                 │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│  Plugin Implementation              │  ← Your code
│  (implements Plugin interface)      │
└──────────────┬──────────────────────┘
               │
               ▼
    Job Schedulers / Execution Environments
```

The Launcher manages plugins as child processes -- it starts each one on launch, sends a bootstrap request for initialization, forwards requests during operation, and sends a termination signal on shutdown. The Launcher automatically restarts any plugins that terminate unexpectedly.

The SDK Runtime handles:
- Request routing and dispatching
- Request/response serialization
- Stream management (status, output, resource utilization)
- Heartbeat responses
- Error handling

Your plugin only needs to implement business logic.

## Contributing

We appreciate your interest in the Launcher Go SDK!

**Issue reports** are always welcome - please report bugs and feature requests in the [GitHub issue tracker](https://github.com/posit-dev/launcher-go-sdk/issues)

**Pull requests** from external contributors are evaluated on a case-by-case basis. We recommend opening an issue to discuss significant changes before investing time in a PR. Please see [CONTRIBUTING.md](CONTRIBUTING.md) for full guidelines and expectations.

## License

Copyright (C) 2026 Posit Software, PBC

See [LICENSE](LICENSE) for details.

## Resources

- [Posit Workbench Documentation](https://docs.posit.co/ide/server-pro/)
- [Launcher Plugin SDK (C++)](https://github.com/rstudio/rstudio-launcher-plugin-sdk)
- [Example Plugins](examples/)

## Support

- **Community Support**: [Posit Community](https://forum.posit.co/)
- **Commercial Support**: [Posit Support](https://support.posit.co/)
- **Documentation**: [docs.posit.co](https://docs.posit.co/)
