# Launcher Plugin Developer Guide

This guide walks you through building launcher plugins using the Go SDK.

## Table of contents

1. [Introduction](#introduction)
2. [Getting Started](#getting-started)
3. [Core Concepts](#core-concepts)
4. [Implementing Plugin Methods](#implementing-plugin-methods)
5. [Configuration](#configuration)
6. [State Management](#state-management)
7. [Testing Your Plugin](#testing-your-plugin)
8. [Production Considerations](#production-considerations)
9. [Advanced Features](#advanced-features)
10. [Deployment](#deployment)

## Introduction

### What is a launcher plugin?

A launcher plugin connects Posit Workbench or Posit Connect to a job scheduler or execution environment. When users launch RStudio sessions, Jupyter notebooks, or other workloads from Workbench, or when Posit Connect executes content, the plugin translates those requests into jobs on your scheduler.

### How it works

```
User / Content → Workbench / Connect → Launcher → Your Plugin → Scheduler/Executor
                                          ↓
                                       JSON over
                                      stdin/stdout
```

The Launcher communicates with plugins via a JSON-based protocol over stdin/stdout. The SDK handles this protocol, so you only need to implement business logic.

Here is a concrete example of a user launching an R session via Kubernetes:

1. Workbench builds the command and arguments to run the R Session.
2. Workbench sends a job submission request to the Launcher over HTTP, including the command, arguments, the user-requested cluster, and any resource constraints.
3. The Launcher verifies that a cluster matching the requested cluster exists.
4. The Launcher forwards the request to the appropriate plugin via the JSON Launcher Plugin API over stdin.
5. The plugin creates the workload on the scheduler (e.g., a Kubernetes pod) with the requested resource restrictions.
6. The plugin reports success or failure back to the Launcher via stdout.
7. The Launcher forwards the result to Workbench via the HTTP API.

### The Launcher service

The Launcher itself is a REST API service that sits between Posit products and plugins. Understanding how it manages plugins is useful context for plugin development.

**Public HTTP API**: The Launcher exposes an HTTP API that Posit products send requests to (e.g., `GET /jobs`, `POST /jobs`). The Launcher handles authentication and authorization, distills the necessary information, and forwards details to the appropriate plugin. The Launcher does not need to run on the same machine as Workbench or Connect.

**Plugin Management**: When the Launcher starts, it reads `/etc/rstudio/launcher.conf`, which contains a `[cluster]` section for each plugin. The Launcher starts a child process for each plugin and sends a Bootstrap request. During bootstrap, the plugin ensures it has an accurate list of current jobs. If any plugin fails to bootstrap, the Launcher fails to start. During normal operation, the Launcher forwards requests to plugins via the JSON protocol. When the Launcher terminates, it sends a termination signal to each plugin.

**Network Architecture**: The Launcher and each plugin must run on the same machine (since they communicate over stdin/stdout). However, the Posit product using the Launcher can run on a different machine -- it only needs HTTP(S) access to the Launcher on the configured port (default: 5559).

**Load Balancing**: Multiple Launcher instances can be load balanced for improved throughput. For this to work correctly, every plugin instance in the cluster must be able to return the same job data. This means plugins need shared state (e.g., shared filesystem, shared database, or the scheduler itself as the source of truth).

**Workbench Integration**: When integrating with Workbench, there are two modes to consider:
- **Launcher Sessions**: Workbench must be able to communicate with the R, Jupyter, or other session over Transmission Control Protocol (TCP) on an arbitrary port.
- **Launcher Jobs**: When used with Launcher Sessions, R sessions must be able to communicate back with Workbench over HTTP(S).

### Communication protocol

All communication happens via length-prefixed JSON messages:

1. **Launcher sends request** → stdin
2. **Plugin processes** → your code
3. **Plugin sends response** → stdout

The SDK's Runtime handles serialization, routing, and error handling. The length prefix prevents partial message reads and eliminates delimiter/escaping concerns.

**Heartbeat**: The Launcher periodically sends heartbeat requests. If the plugin doesn't respond within the configured interval, the Launcher restarts it. The SDK handles heartbeats automatically. During development, you can set `heartbeat-interval-seconds=0` in `launcher.conf` to disable heartbeating (not recommended for production).

## Getting started

### Prerequisites

- Go 1.25 or later
- Basic understanding of job schedulers
- Posit Workbench 2023.09.0+ or Posit Connect 2024.08.0+ (for deployment)

### Installation

```bash
go get github.com/posit-dev/launcher-go-sdk
```

### Your first plugin

Let's build a plugin step by step.

#### Step 1: Create the plugin struct {#create-plugin-struct}

```go
package main

import (
    "github.com/posit-dev/launcher-go-sdk/cache"
)

type MyPlugin struct {
    cache *cache.JobCache
}
```

#### Step 2: Implement required methods {#implement-methods}

All plugins must implement the `launcher.Plugin` interface (10 methods):

```go
func (p *MyPlugin) SubmitJob(ctx context.Context, w launcher.ResponseWriter, user string, job *api.Job) {
    // Handle job submission
}

func (p *MyPlugin) GetJob(ctx context.Context, w launcher.ResponseWriter, user string, id api.JobID, fields []string) {
    // Return job information
}

// ... 8 more methods
```

#### Step 3: Set up main function {#setup-main-function}

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(),
        os.Interrupt, syscall.SIGTERM)
    defer stop()

    // Load configuration
    options := &launcher.DefaultOptions{}
    launcher.MustLoadOptions(options, "myplugin")

    // Create logger
    lgr := logger.MustNewLogger("myplugin", options.Debug, options.LoggingDir)

    // Create job cache
    cache, _ := cache.NewJobCache(ctx, lgr)

    // Create and run plugin
    plugin := &MyPlugin{cache: cache}
    launcher.NewRuntime(lgr, plugin).Run(ctx)
}
```

See [`examples/inmemory`](../examples/inmemory) for a complete working example.

## Core concepts

### Plugin interface

The `launcher.Plugin` interface defines 10 methods your plugin must implement:

| Method | Purpose | Response Type |
|--------|---------|---------------|
| `SubmitJob` | Accept new job | Single |
| `GetJob` | Return single job | Single |
| `GetJobs` | Return multiple jobs | Single |
| `ControlJob` | Control job (stop/kill/cancel) | Single |
| `GetJobStatus` | Stream status for one job | Streaming |
| `GetJobStatuses` | Stream status for all jobs | Streaming |
| `GetJobOutput` | Stream stdout/stderr | Streaming |
| `GetJobResourceUtil` | Stream CPU/memory usage | Streaming |
| `GetJobNetwork` | Return network info | Single |
| `ClusterInfo` | Return cluster capabilities | Single |

### Job lifecycle

Jobs progress through these states:

```
Pending → Running → Finished
                  ↘ Failed
                  ↘ Killed
                  ↘ Canceled
          ↓
     Suspended (optional)
```

**State definitions**:

| State | Description |
|-------|-------------|
| `Pending` | Successfully submitted to the scheduler but not yet running. |
| `Running` | Currently executing. |
| `Suspended` | Was running, but execution was paused and may be resumed later. |
| `Finished` | Launched and finished executing. Includes jobs that exited with non-zero exit codes. |
| `Failed` | Could not be launched due to an error. Does *not* refer to jobs where the process exited with a non-zero exit code. |
| `Killed` | Forcibly killed while running (i.e., the process received `SIGKILL`). |
| `Canceled` | Canceled by the user before it began to run. |

**Terminal states** (job won't change): Finished, Failed, Killed, Canceled

Use `api.TerminalStatus(status)` to check if a status is terminal.

**Control operations and valid states**:

| Operation | Valid When | Result |
|-----------|-----------|--------|
| `cancel` | Pending | Canceled |
| `stop` | Running | Finished |
| `kill` | Running | Killed |
| `suspend` | Running | Suspended |
| `resume` | Suspended | Running |

### Response writers

Plugins communicate back to the Launcher via ResponseWriters:

#### ResponseWriter (single response)

For methods that send one response:

```go
func (p *MyPlugin) GetJob(_ context.Context, w launcher.ResponseWriter, user string, id api.JobID, fields []string) {
    // Option 1: Use cache helper
    p.cache.WriteJob(w, user, id)

    // Option 2: Write error
    w.WriteError(api.Errorf(api.CodeJobNotFound, "Job %s not found", id))

    // Option 3: Write jobs directly
    w.WriteJobs([]*api.Job{job})
}
```

#### StreamResponseWriter (multiple responses)

For methods that stream multiple updates:

```go
func (p *MyPlugin) GetJobOutput(ctx context.Context, w launcher.StreamResponseWriter,
    user string, id api.JobID, outputType api.JobOutput) {

    defer w.Close() // ALWAYS call Close when done

    for {
        select {
        case <-ctx.Done():
            return // Client cancelled
        case output := <-outputChan:
            w.WriteJobOutput(output, outputType)
        }
    }
}
```

Always call `Close()` on StreamResponseWriter when done.

### Job cache

The SDK provides a job cache for storing and querying jobs:

```go
// Create cache (in main)
cache, err := cache.NewJobCache(ctx, logger)

// Store a job
job.ID = "job-123"
job.Status = api.StatusPending
cache.AddOrUpdate(job)

// Query single job
cache.WriteJob(w, user, jobID)

// Query multiple jobs
filter := &api.JobFilter{
    Statuses: []string{api.StatusRunning},
}
cache.WriteJobs(w, user, filter)

// Stream status updates
cache.StreamJobStatus(ctx, w, user, jobID)
```

The cache:
- Stores jobs in memory (the scheduler is the source of truth)
- Enforces user permissions (users only see their own jobs)
- Provides pub/sub for status updates
- Handles job expiration

## Implementing plugin methods

### SubmitJob - accept new jobs

```go
func (p *MyPlugin) SubmitJob(ctx context.Context, w launcher.ResponseWriter, user string, job *api.Job) {
    // 1. Generate unique job ID
    id := generateJobID()  // Your implementation
    job.ID = id

    // 2. Set initial metadata
    now := time.Now().UTC()
    job.Submitted = &now
    job.LastUpdated = &now
    job.Status = api.StatusPending

    // 3. Submit to your scheduler
    schedulerID, err := submitToScheduler(job)
    if err != nil {
        w.WriteErrorf(api.CodeUnknown, "Failed to submit: %v", err)
        return
    }

    // 4. Store in cache
    if err := p.cache.AddOrUpdate(job); err != nil {
        w.WriteError(err)
        return
    }

    // 5. Return the job
    p.cache.WriteJob(w, user, id)

    // 6. Start monitoring (optional)
    go p.monitorJob(user, id, schedulerID)
}
```

**Key points**:
- Always generate a unique ID
- Set `Submitted` and `LastUpdated` timestamps
- Start with `StatusPending`
- Store before returning
- Handle errors appropriately

### GetJob / GetJobs - query jobs

```go
func (p *MyPlugin) GetJob(_ context.Context, w launcher.ResponseWriter, user string,
    id api.JobID, fields []string) {

    // The cache handles permission checking
    p.cache.WriteJob(w, user, id)
}

func (p *MyPlugin) GetJobs(_ context.Context, w launcher.ResponseWriter, user string,
    filter *api.JobFilter, fields []string) {

    // Filter can specify:
    // - Statuses: []string{api.StatusRunning, api.StatusPending}
    // - Tags: []string{"ml-training"}
    // - Time range: StartTime, EndTime
    p.cache.WriteJobs(w, user, filter)
}
```

### ControlJob - stop/kill/cancel jobs

```go
func (p *MyPlugin) ControlJob(_ context.Context, w launcher.ResponseWriter, user string,
    id api.JobID, op api.JobOperation) {

    err := p.cache.Update(user, id, func(job *api.Job) *api.Job {
        // 1. Validate operation is valid for current status
        if op.ValidForStatus() != job.Status {
            w.WriteErrorf(api.CodeInvalidJobState,
                "Job must be %s to %s it", op.ValidForStatus(), op)
            return job // Return unchanged
        }

        // 2. Perform operation on scheduler
        switch op {
        case api.OperationStop:
            stopJob(job.ID)  // Send SIGTERM
            job.Status = api.StatusFinished
        case api.OperationKill:
            killJob(job.ID)  // Send SIGKILL
            job.Status = api.StatusKilled
        case api.OperationCancel:
            cancelJob(job.ID)
            job.Status = api.StatusCanceled
        }

        // 3. Report success
        w.WriteControlJob(true, "")
        return job
    })

    if err != nil {
        w.WriteError(err)
    }
}
```

### GetJobStatus / GetJobStatuses - stream status

```go
func (p *MyPlugin) GetJobStatus(ctx context.Context,
    w launcher.StreamResponseWriter, user string, id api.JobID) {

    // The cache handles streaming status updates automatically
    // It will send an update whenever the job's status changes
    p.cache.StreamJobStatus(ctx, w, user, id)
}
```

The cache's `StreamJobStatus` method:
- Sends initial status immediately
- Sends updates when status changes (via pub/sub)
- Respects context cancellation
- Automatically closes when job reaches terminal state

### GetJobOutput - stream output

```go
func (p *MyPlugin) GetJobOutput(ctx context.Context,
    w launcher.StreamResponseWriter, user string,
    id api.JobID, outputType api.JobOutput) {

    defer w.Close()

    // Verify job exists
    err := p.cache.Lookup(user, id, func(_ *api.Job) {})
    if err != nil {
        w.WriteError(err)
        return
    }

    // Stream output in a goroutine
    go func() {
        // Read from output file or tail logs
        for {
            select {
            case <-ctx.Done():
                return
            case line := <-outputChannel:
                w.WriteJobOutput(line, outputType)
            }
        }
    }()
}
```

For `outputType`:
- `api.OutputStdout` - stdout only
- `api.OutputStderr` - stderr only
- `api.OutputBoth` - interleaved stdout/stderr

### GetJobResourceUtil - stream resource usage

```go
func (p *MyPlugin) GetJobResourceUtil(ctx context.Context,
    w launcher.StreamResponseWriter, user string, id api.JobID) {

    defer w.Close()

    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        err := p.cache.Lookup(user, id, func(job *api.Job) {
            // Query resource usage from scheduler
            cpu, mem := getResourceUsage(job.ID)

            w.WriteJobResourceUtil(
                cpu,    // CPU percent
                0,      // CPU time (seconds)
                mem,    // Resident memory (MB)
                0,      // Virtual memory (MB)
            )

            // Stop if job is done
            if api.TerminalStatus(job.Status) {
                w.Close()
            }
        })

        if err != nil {
            return
        }

        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            continue
        }
    }
}
```

### GetJobNetwork - return network info

```go
func (p *MyPlugin) GetJobNetwork(_ context.Context, w launcher.ResponseWriter,
    user string, id api.JobID) {

    err := p.cache.Lookup(user, id, func(job *api.Job) {
        // Query which node the job is on
        hostname, ips := getJobNetwork(job.ID)

        w.WriteJobNetwork(hostname, ips)
    })

    if err != nil {
        w.WriteError(err)
    }
}
```

### ClusterInfo - return cluster capabilities

```go
func (p *MyPlugin) ClusterInfo(_ context.Context, w launcher.ResponseWriter, user string) {
    w.WriteClusterInfo(launcher.ClusterOptions{
        // Available queues
        Queues:       []string{"default", "high-priority", "gpu"},
        DefaultQueue: "default",

        // Resource limits
        Limits: []api.ResourceLimit{
            {Type: "cpuCount", Max: "128"},
            {Type: "memory", Max: "512GB"},
        },

        // Container support
        ImageOpt: launcher.ImageOptions{
            Images:       []string{"ubuntu:22.04", "rocker/rstudio:latest"},
            Default:      "ubuntu:22.04",
            AllowUnknown: false,
        },

        // Placement constraints
        Constraints: []api.PlacementConstraint{
            {Name: "node-type", Value: "cpu"},
            {Name: "node-type", Value: "gpu"},
        },

        // Resource profiles
        Profiles: []api.ResourceProfile{
            {
                Name:        "small",
                DisplayName: "Small (2 CPU, 8GB RAM)",
                Limits: []api.ResourceLimit{
                    {Type: "cpuCount", Value: "2"},
                    {Type: "memory", Value: "8GB"},
                },
            },
        },
    })
}
```

## Configuration

### Default options

The SDK provides `DefaultOptions` with standard Launcher flags:

```go
type DefaultOptions struct {
    Debug             bool          // Enable debug logging
    JobExpiry         time.Duration // When to expire old jobs
    HeartbeatInterval time.Duration // Expected heartbeat frequency
    LauncherConfig    string        // Path to Launcher config
    PluginName        string        // Plugin instance name
    ScratchPath       string        // Temporary file directory
    ServerUser        string        // User to run as
    Unprivileged      bool          // Running without root
    LoggingDir        string        // Log file directory
    ConfigFile        string        // Plugin config file
}
```

### Custom configuration

Add plugin-specific options by embedding `DefaultOptions`:

```go
type SlurmOptions struct {
    launcher.DefaultOptions

    // Custom fields
    SlurmBin    string
    DefaultPartition string
    MaxJobs     int
}

func (o *SlurmOptions) AddFlags(f *flag.FlagSet, pluginName string) {
    // Add default flags
    o.DefaultOptions.AddFlags(f, pluginName)

    // Add custom flags
    f.StringVar(&o.SlurmBin, "slurm-bin", "/usr/bin",
        "path to Slurm binaries")
    f.StringVar(&o.DefaultPartition, "default-partition", "batch",
        "default Slurm partition")
    f.IntVar(&o.MaxJobs, "max-jobs", 1000,
        "maximum concurrent jobs")
}

func (o *SlurmOptions) Validate() error {
    if err := o.DefaultOptions.Validate(); err != nil {
        return err
    }

    // Custom validation
    if _, err := os.Stat(o.SlurmBin); err != nil {
        return fmt.Errorf("slurm-bin directory not found: %w", err)
    }

    return nil
}
```

## State management

### Using JobCache

The JobCache provides in-memory storage with pub/sub. The scheduler is always the source of truth — the cache is a local working copy.

```go
cache, err := cache.NewJobCache(ctx, logger)
```

Plugins should populate the cache from the scheduler during `Bootstrap()` and keep it in sync via a periodic polling loop (e.g., every 5 seconds). This is consistent with how all existing Launcher plugins (Local, Kubernetes, Slurm) operate.

**Adding/Updating Jobs**:

```go
job := &api.Job{
    ID:     "job-123",
    Status: api.StatusPending,
    User:   "alice",
}

// Add new job
if err := cache.AddOrUpdate(job); err != nil {
    // Handle error
}

// Update existing job
err := cache.Update(user, jobID, func(job *api.Job) *api.Job {
    job.Status = api.StatusRunning
    return job
})
```

**Querying Jobs**:

```go
// Single job
cache.WriteJob(w, user, jobID)

// Multiple jobs with filter
filter := &api.JobFilter{
    Statuses: []string{api.StatusRunning},
    Tags:     []string{"ml-training"},
}
cache.WriteJobs(w, user, filter)

// Direct lookup
err := cache.Lookup(user, jobID, func(job *api.Job) {
    // Do something with job
})
```

**Pub/Sub for Status Updates**:

The cache automatically notifies subscribers when jobs change:

```go
// This will send updates automatically when the job's status changes
cache.StreamJobStatus(ctx, w, user, jobID)

// Update from another goroutine
cache.Update(user, jobID, func(job *api.Job) *api.Job {
    job.Status = api.StatusRunning  // Triggers notification
    return job
})
```

### Job expiration

Old jobs are automatically removed based on `JobExpiry`:

```go
options := &launcher.DefaultOptions{
    JobExpiry: 24 * time.Hour, // Remove jobs after 24 hours
}
```

The cache removes jobs in terminal states (Finished, Failed, Killed, Canceled) after this period.

## Testing your plugin

See [TESTING.md](TESTING.md) for the comprehensive testing guide.

### Conformance tests (recommended)

The `conformance` package verifies your plugin against the behavioral contracts that Posit products expect. This is the fastest way to validate a new plugin implementation:

```go
import (
    "testing"

    "github.com/posit-dev/launcher-go-sdk/api"
    "github.com/posit-dev/launcher-go-sdk/conformance"
)

func TestConformance(t *testing.T) {
    plugin := setupMyPlugin()

    profile := conformance.Profile{
        JobFactory:      func(u string) *api.Job { return &api.Job{User: u, Command: "echo hello"} },
        LongRunningJob:  func(u string) *api.Job { return &api.Job{User: u, Command: "sleep 300"} },
        StopStatus:      api.StatusFinished,
        StopExitCodes:   []int{143},
        KillStatus:      api.StatusKilled,
        KillExitCodes:   []int{137},
        OutputAvailable: true,
    }

    conformance.Run(t, plugin, "testuser", profile)
    conformance.RunWorkflows(t, plugin, "testuser", profile)
}
```

`conformance.Run` checks universal invariants (ID generation, timestamps, filtering, error codes). `conformance.RunWorkflows` replays the request sequences products actually produce (launch, stop, kill, cancel). See the [conformance testing section in TESTING.md](TESTING.md#conformance-testing) for details.

### Unit tests

For testing individual methods in isolation, use the `plugintest` package:

```go
func TestSubmitJob(t *testing.T) {
    // Setup
    ctx := context.Background()
    lgr := logger.MustNewLogger("test", false, "")
    cache, _ := cache.NewJobCache(ctx, lgr)
    plugin := &MyPlugin{cache: cache}

    // Create mock writer
    w := plugintest.NewMockResponseWriter()

    // Create test job
    job := plugintest.NewJob().
        WithUser("alice").
        WithCommand("echo hello").
        Build()

    // Test
    plugin.SubmitJob(context.Background(), w, "alice", job)

    // Assert
    plugintest.AssertNoError(t, w)
    plugintest.AssertJobCount(t, w, 1)
}
```

## Production considerations

### Error handling

Always use typed errors so Workbench and Connect can handle them appropriately (e.g., retrying timeouts, showing user-friendly messages for not-found errors):

```go
// Good
w.WriteErrorf(api.CodeJobNotFound, "Job %s not found", jobID)

// Also good
w.WriteError(api.Errorf(api.CodeJobNotFound, "Job %s not found", jobID))
```

Available error codes:

| Code | Constant | Description |
|------|----------|-------------|
| 1 | `CodeUnknown` | The request failed for an undetermined reason. Use when no more specific code applies. |
| 2 | `CodeRequestNotSupported` | The operation is not supported by this plugin. |
| 3 | `CodeInvalidRequest` | The request is malformed. |
| 10 | `CodeJobNotFound` | The job does not exist in the scheduling system. |
| 20 | `CodeTimeout` | The request timed out waiting for the scheduler. |
| 30 | `CodeJobNotRunning` | The job exists but is not in the running state. |
| 31 | `CodeInvalidJobState` | The job's current state is invalid for the requested operation. |
| 32 | `CodeJobControlFailure` | The control operation (stop/kill/cancel) failed. |

**Systematic error handling**: For larger plugins, consider categorizing errors (e.g., scheduler API errors vs. internal errors) and creating helper functions that consistently produce well-formatted error messages with context.

### Logging

Use structured logging:

```go
lgr.Info("Job submitted", "job_id", job.ID, "user", user)
lgr.Error("Failed to submit", "error", err, "job_id", job.ID)
lgr.Debug("Querying scheduler", "command", cmd)
```

### Performance

**Batch Operations**: Query scheduler status in batches when possible:

```go
// Good - single query for all jobs
statuses := slurmClient.GetAllJobStatuses(user)

// Bad - query each job individually
for _, jobID := range jobIDs {
    status := slurmClient.GetJobStatus(jobID)
}
```

**Polling Frequency**: Balance freshness vs load:

```go
// For active jobs: poll more frequently
activeTicker := time.NewTicker(10 * time.Second)

// For completed jobs: poll less frequently
completedTicker := time.NewTicker(5 * time.Minute)
```

### Security

**User Isolation**: The cache enforces user permissions automatically:

```go
// User can only see their own jobs
cache.WriteJob(w, "alice", jobID)  // Returns error if job belongs to Bob
```

**Command Injection**: Sanitize user input:

```go
// Bad
cmd := exec.Command("sh", "-c", job.Command) // Dangerous!

// Good
cmd := exec.Command(job.Exe, job.Args...)
```

## Advanced features

### Bootstrap phase

The Launcher sends a Bootstrap request once, immediately after starting the plugin. During this phase, the SDK verifies API version compatibility with the Launcher. You can implement the `BootstrappedPlugin` interface to hook into this phase for initialization that might fail:

```go
type MyPlugin struct {
    cache *cache.JobCache
    // ...
}

func (p *MyPlugin) Bootstrap(w launcher.ResponseWriter) {
    // This runs once when the plugin first connects
    // Use it for initialization that might fail

    if err := p.validateSchedulerConnection(); err != nil {
        w.WriteErrorf(api.CodeUnknown,
            "Cannot connect to scheduler: %v", err)
        return
    }

    lgr.Info("Plugin bootstrapped successfully")
}

// Implement all Plugin methods...
```

During bootstrap, the plugin should ensure it has an accurate list of existing jobs from the scheduler. If the plugin fails to bootstrap, the Launcher will fail to start.

### Job status update strategies

Your plugin needs to keep an accurate record of job statuses. There are two common approaches:

**Polling** (most common): Periodically query the scheduler for job statuses and update the cache. This is the simplest approach and works with all schedulers (Slurm, PBS, LSF, etc.) since most don't provide event notifications.

```go
func (p *MyPlugin) pollStatuses(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            statuses := p.scheduler.GetAllJobStatuses()
            for _, s := range statuses {
                p.cache.Update("", s.ID, func(job *api.Job) *api.Job {
                    job.Status = s.Status
                    return job
                })
            }
        }
    }
}
```

**Streaming** (preferred when available): If the scheduler provides an event stream or callback API for status changes, use it. Streaming is more efficient because you only process each status change once, rather than re-reading the same statuses repeatedly.

```go
func (p *MyPlugin) streamStatuses(ctx context.Context) {
    ch := p.scheduler.StreamStatusChanges(ctx)
    for update := range ch {
        p.cache.Update("", update.ID, func(job *api.Job) *api.Job {
            job.Status = update.Status
            job.StatusMessage = update.Message
            return job
        })
    }
}
```

**Batch queries**: When polling, always query scheduler status in batches rather than per-job. A single `squeue` call is far more efficient than one per job.

### Output stream considerations

When implementing `GetJobOutput`, be aware of these edge cases:

- **Buffered output**: Some schedulers buffer output and write it shortly after a job enters a terminal state. Your output stream may need to stay open briefly after the job finishes to capture the remaining output.
- **End-of-stream detection**: Consider how to detect when all output has been emitted. Some implementations write a sentinel value as the last line of output that the stream watches for.
- **Concurrent streams**: Multiple output streams can be active for the same job simultaneously (e.g., a user refreshing the output view). Each stream instance should be independent.
- **Output types**: The caller may request stdout only, stderr only, or both interleaved. Do your best to report only the requested type. If the scheduler doesn't distinguish between them, use `api.OutputBoth`.

### Custom job configuration

The ClusterInfo response includes a `Configs` field for declaring custom configuration values that users can set per job. A `JobConfig` has a name, type (`string`, `int`, `float`, `bool`), and optionally a default value and description.

```go
Configs: []api.JobConfig{
    {Name: "gpu-type", Type: "string", Description: "GPU model to request"},
    {Name: "priority", Type: "int", Default: "5", Description: "Job priority (1-10)"},
},
```

When the user submits a job, any custom configuration values they set are available in `job.Config`.

### Resource utilization

Resource utilization streaming is a best-effort feature. Not all schedulers expose the same metrics. The four metrics are:

- **CPU Percent**: Current CPU usage (0-100)
- **CPU Seconds**: Cumulative CPU time
- **Resident Memory**: Physical memory usage in MB
- **Virtual Memory**: Virtual memory usage in MB

Report whichever metrics are available from your scheduler. Pass `0` for unavailable metrics.

### Multi-cluster support

Implement `MultiClusterPlugin` to support multiple clusters:

```go
func (p *MyPlugin) GetClusters(w launcher.MultiClusterResponseWriter, user string) {
    clusters := []launcher.ClusterOptions{
        {
            Name:   "cpu-cluster",
            Queues: []string{"batch", "interactive"},
        },
        {
            Name:   "gpu-cluster",
            Queues: []string{"gpu", "gpu-interactive"},
        },
    }

    w.WriteClusters(clusters)
}
```

### Load balancer awareness

When multiple Launcher instances are load balanced, each plugin instance must be able to return the same data. This typically means using the scheduler itself as the source of truth and populating each plugin's in-memory cache on startup.

Implement `LoadBalancedPlugin` for multi-node deployments:

```go
func (p *MyPlugin) SyncNodes(nodes []api.Node) {
    // Called when nodes in the load-balanced cluster change
    p.lgr.Info("Cluster nodes updated", "count", len(nodes))

    for _, node := range nodes {
        if node.Online() {
            p.lgr.Debug("Node online", "host", node.Host, "ip", node.IP)
        }
    }
}
```

### Configuration reloading

The Launcher can ask plugins to reload their configuration at runtime without restarting (API v3.6.0+). At minimum, plugins should reload user profiles and resource profiles. Reloading additional configuration is permitted but not required.

Implement `ConfigReloadablePlugin` to support this:

```go
func (p *MyPlugin) ReloadConfig(ctx context.Context) error {
    profiles, err := loadProfiles(p.profilePath)
    if err != nil {
        return &launcher.ConfigReloadError{
            Type:    api.ReloadErrorLoad,
            Message: fmt.Sprintf("failed to load profiles: %v", err),
        }
    }
    if err := profiles.Validate(); err != nil {
        return &launcher.ConfigReloadError{
            Type:    api.ReloadErrorValidate,
            Message: fmt.Sprintf("invalid profiles: %v", err),
        }
    }
    p.mu.Lock()
    p.profiles = profiles
    p.mu.Unlock()
    return nil
}
```

Plugins that do not implement this interface automatically send a success response. Error types help the Launcher classify failures: `ReloadErrorLoad` for file read errors, `ReloadErrorValidate` for invalid configuration, and `ReloadErrorSave` for errors persisting state. Returning a plain `error` (instead of `*ConfigReloadError`) defaults to `ReloadErrorUnknown`.

**Best practice: preserve last-known-good configuration.** When reloading file-based configuration, only replace your in-memory state after the new files have been successfully loaded and validated. This ensures that a malformed config file doesn't leave the plugin in a broken state — the previous working configuration stays active. The first-party plugins take this further by writing hidden backup copies of each profile file (e.g., `.launcher.local.profiles.conf.active`) at two points: once at startup after the initial successful load, and again after every successful reload. The startup copy seeds the backup so it exists before any reload is attempted. If your plugin uses file-based profiles, consider adopting the same pattern:

```go
func (p *MyPlugin) Bootstrap(ctx context.Context) error {
    // ... load jobs from scheduler, etc.

    // Seed last-known-good copies from the config we just booted with.
    writeBackupCopy(p.profilePath)
    return nil
}

func (p *MyPlugin) ReloadConfig(ctx context.Context) error {
    // Load and validate BEFORE replacing anything.
    newProfiles, err := loadProfiles(p.profilePath)
    if err != nil {
        return &launcher.ConfigReloadError{
            Type:    api.ReloadErrorLoad,
            Message: fmt.Sprintf("failed to load profiles: %v", err),
        }
    }
    if err := newProfiles.Validate(); err != nil {
        return &launcher.ConfigReloadError{
            Type:    api.ReloadErrorValidate,
            Message: fmt.Sprintf("invalid profiles: %v", err),
        }
    }

    // Swap in the validated config.
    p.mu.Lock()
    p.profiles = newProfiles
    p.mu.Unlock()

    // Update the backup with the new known-good config.
    writeBackupCopy(p.profilePath)
    return nil
}
```

### User profiles

System administrators may want to set default or maximum values for certain features on a per-user or per-group basis. For example, different groups of users could have different memory limits or CPU counts.

System administrators configure user profiles via a file at `/etc/rstudio/launcher.<plugin-name>.profiles.conf`. Your plugin can read this file during bootstrap and use the values when returning ClusterInfo (to set per-user resource limits) or when validating job submissions.

## Deployment

### Building

```bash
# Build for Linux
GOOS=linux GOARCH=amd64 go build -o rstudio-myplugin-launcher

# Build with version info
go build -ldflags="-X main.version=1.0.0" -o rstudio-myplugin-launcher
```

### Installation

1. **Copy binary**:
```bash
sudo cp rstudio-myplugin-launcher /usr/lib/rstudio-server/bin/
sudo chmod +x /usr/lib/rstudio-server/bin/rstudio-myplugin-launcher
```

2. **Configure Launcher** (`/etc/rstudio/launcher.conf`):
```ini
[cluster]
name=mycluster
type=Plugin
exe=/usr/lib/rstudio-server/bin/rstudio-myplugin-launcher
```

3. **Create plugin config** (`/etc/rstudio/launcher.mycluster.conf`):
```ini
# Plugin-specific configuration
```

4. **Restart services**:
```bash
sudo rstudio-server restart
sudo rstudio-launcher restart
```

### Testing deployment

Use `launcher-proxy` to test your plugin:

```bash
/usr/lib/rstudio-server/bin/launcher-proxy \
    --plugin=/usr/lib/rstudio-server/bin/rstudio-myplugin-launcher \
    job-submit --name=test --command="echo hello"
```

### Troubleshooting

**Check logs**:
```bash
# Launcher logs
tail -f /var/log/rstudio/launcher/rstudio-launcher.log

# Plugin logs
tail -f /var/log/rstudio/launcher/myplugin.log
```

**Common issues**:

1. **Plugin not starting**: Check binary permissions and path
2. **Jobs stuck in Pending**: Verify scheduler connectivity
3. **Permission errors**: Check user mapping and file permissions
4. **Protocol errors**: Ensure API version compatibility

## Next steps

- Review the [examples](../examples/) for complete implementations
- Read the [API Reference](API.md) for detailed type information
- Check the [Testing Guide](TESTING.md) for testing strategies
- See the [Architecture](ARCHITECTURE.md) document for design decisions

## Resources

- [Posit Workbench Documentation](https://docs.posit.co/ide/server-pro/)
- [Launcher Plugin API](https://docs.posit.co/job-launcher/)
- [Example Plugins](../examples/)
- [pkg.go.dev Documentation](https://pkg.go.dev/github.com/posit-dev/launcher-go-sdk)
