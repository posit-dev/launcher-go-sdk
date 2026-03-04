# Architecture

This document explains the design decisions, patterns, and architecture of the Launcher Go SDK.

## Table of contents

1. [Overview](#overview)
2. [Package Structure](#package-structure)
3. [Protocol Layer](#protocol-layer)
4. [Runtime Architecture](#runtime-architecture)
5. [Job Cache Design](#job-cache-design)
6. [Testing Utilities](#testing-utilities)
7. [Conformance Testing](#conformance-testing)
8. [Design Patterns](#design-patterns)
9. [Design Decisions](#design-decisions)
10. [Trade-offs](#trade-offs)
11. [Future Considerations](#future-considerations)

## Overview

The Launcher Go SDK provides a complete framework for building launcher plugins. The architecture is designed around several key principles:

- **Simplicity**: Plugin developers should focus on business logic, not protocol details
- **Safety**: Type-safe APIs prevent common mistakes
- **Testability**: Comprehensive testing utilities make plugin testing straightforward
- **Performance**: Efficient caching and streaming minimize overhead
- **Extensibility**: Clean interfaces allow for advanced features

### The Launcher ecosystem

The Launcher is a REST API service that provides a generic interface between Posit products (Posit Workbench, Posit Connect) and job scheduling systems. It is not specific to R processes -- it can launch arbitrary work through any scheduler for which a plugin exists.

On the front end, the Launcher exposes an HTTP API. Posit products send requests (e.g., `GET /jobs`, `POST /jobs`) to the Launcher, which handles authentication and authorization, distills the necessary information, and forwards it to the appropriate plugin.

On the back end, the Launcher manages plugins as child processes. It communicates with them via a JSON protocol over stdin/stdout. This means the Launcher and its plugins must run on the same machine, but the Posit product using the Launcher can run on a different machine (it only needs HTTP(S) access to the Launcher on port 5559 by default).

Multiple Launcher instances can be load balanced for improved throughput. In this configuration, every plugin instance must be able to return the same data, which typically means using the scheduler as the source of truth.

### Layered architecture

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

## Package structure

### Public packages

#### `launcher` - Core Runtime

The launcher package is the main entry point:

```go
// Core interfaces
type Plugin interface { /* 10 methods */ }
type ResponseWriter interface { /* 5 methods */ }
type StreamResponseWriter interface { /* 6 methods */ }

// Runtime for request handling
type Runtime struct { /* ... */ }

// Configuration
type DefaultOptions struct { /* ... */ }
```

**Design rationale**: Keep the Plugin interface minimal and focused. All methods receive ResponseWriter and user context, making the API consistent and predictable.

#### `api` - Type Definitions

Contains all types matching the Launcher Plugin API v3.5:

```go
type Job struct { /* 30+ fields */ }
type JobFilter struct { /* ... */ }
type Error struct { /* ... */ }
// ... 40+ more types
```

**Design rationale**: One package for all API types prevents circular dependencies and makes the type system discoverable. Types are kept flat (no deep nesting) for JSON serialization efficiency.

#### `cache` - Job Storage

Provides in-memory job storage with pub/sub for status updates:

```go
type JobCache struct { /* ... */ }
```

**Design rationale**: Separate package allows plugins to opt out if they have their own storage. Pub/sub is built-in for status streaming rather than requiring external message queues.

#### `logger` - Structured Logging

Workbench-style logging utilities:

```go
func NewLogger(name string, debug bool, dir string) (*slog.Logger, error)
func MustNewLogger(name string, debug bool, dir string) *slog.Logger
```

**Design rationale**: Wraps `slog` with Posit product conventions (file rotation, formatting) so plugins match the logging style of Workbench and Connect.

#### `plugintest` - Testing Utilities

Mocks, builders, and assertions:

```go
type MockResponseWriter struct { /* ... */ }
type JobBuilder struct { /* ... */ }
func AssertNoError(t *testing.T, w *MockResponseWriter)
```

**Design rationale**: Separate testing package follows Go conventions. Fluent builders make test data construction readable. Assertions provide helpful error messages.

#### `conformance` - Behavioral Conformance Tests

Automated tests that verify plugin behavior against product contracts:

```go
func Run(t *testing.T, p launcher.Plugin, user string, profile Profile)
func RunWorkflows(t *testing.T, p launcher.Plugin, user string, profile Profile)
func RunStopJob(t *testing.T, p launcher.Plugin, user string, opts StopOpts)
```

**Design rationale**: Products (Workbench, Connect) expect specific behavioral contracts from plugins (e.g., Stop yields a terminal status, status streams deliver Running updates). Conformance tests codify these contracts so plugin authors get automated verification. The `Profile` struct parameterizes cross-scheduler behavioral deltas rather than hard-coding expectations.

### Internal packages

#### `internal/protocol` - Wire Protocol

Handles JSON serialization over stdin/stdout:

```go
type Communicator struct { /* ... */ }
type Request struct { /* ... */ }
type Response struct { /* ... */ }
```

**Design rationale**: Internal package prevents plugins from depending on protocol details. Protocol changes don't affect plugin code. Message framing prevents partial reads.

## Component architecture

The SDK is structured into several logical components. Understanding their responsibilities helps with advanced implementation decisions.

### Protocol / communicator component

The internal protocol layer receives and interprets requests from the Launcher, then translates and sends responses back. It listens for data on stdin in a background goroutine, parses and validates each request, and converts it into the appropriate typed request object for the Runtime to dispatch. When the plugin has a response, the protocol layer formats it and writes it to stdout.

The SDK fully implements this component. Plugin developers never interact with it directly.

### Runtime / Plugin API component

The Runtime understands the meaning of each request and dispatches the correct action on the Plugin implementation. Given a request, it routes to the appropriate Plugin method, then converts the output to the appropriate response. Each request is processed in its own goroutine, so Plugin methods must be safe for concurrent use.

### Job cache / repository component

The cache maintains an in-memory store of jobs, provides pub/sub for status update notifications, enforces user permissions, and automatically expires old jobs. The cache acts as both the job repository and the status notification system -- when a job is updated via `Update` or `AddOrUpdate`, the cache notifies any active status stream subscribers automatically. The scheduler is always the source of truth for job state; the cache is a local working copy that plugins should populate during `Bootstrap()` and keep in sync via periodic polling.

### Stream components

Three types of requests produce streamed responses: job status, job output, and resource utilization. Each stream is independent -- the SDK constructs a new stream handler for each request. This means:

- Multiple output streams can be active for the same job simultaneously
- Individual stream instances don't need to coordinate with each other
- Each stream respects its own context for cancellation
- For job status streams, the cache's pub/sub handles fan-out automatically

## Plugin lifecycle

### Startup sequence

When a plugin is launched by the Launcher, the following steps occur:

1. The `main` function parses command-line options via `MustLoadOptions`
2. The logger is created via `MustNewLogger`
3. The job cache is created via `NewJobCache`
4. The plugin implementation is constructed
5. `NewRuntime` creates the runtime with the logger and plugin
6. `Runtime.Run` is called, which:
   a. Initializes the protocol communicator (stdin/stdout)
   b. Receives and responds to the Bootstrap request (version negotiation)
   c. If the plugin implements `BootstrappedPlugin`, calls `Bootstrap` — this is where plugins should re-read active jobs from the scheduler into the cache
   d. Begins the heartbeat response loop
   e. Enters normal operation, dispatching requests to plugin methods
7. The plugin runs until the context is cancelled or a fatal error occurs

### Teardown sequence

When the Launcher terminates the plugin (or the context is cancelled):

1. The context cancellation propagates to all active goroutines
2. All active streams receive context cancellation and should return
3. The protocol communicator stops reading from stdin
4. The Runtime waits for in-flight requests to complete
5. The process exits

## Protocol layer

### Message format

All messages are JSON with length prefix:

```
[4-byte length][JSON payload]
```

**Why length-prefixed?**
- Prevents partial message reads
- Allows streaming large payloads
- Simpler than delimiter-based protocols
- No escaping concerns

### Request/response types

Each operation has typed request/response:

```go
type SubmitJobRequest struct {
    User string
    Job  *api.Job
}

type SubmitJobResponse struct {
    Jobs []*api.Job
}
```

**Why separate types?**
- Type safety prevents sending wrong data
- Clear documentation of what each operation needs
- Easy to add fields without breaking compatibility

### Streaming protocol

Streaming methods use a different pattern:

1. Initial request
2. Multiple response messages
3. Final close message

```go
type StreamResponse struct {
    Type    string          // "status", "output", "resource", "close"
    Payload json.RawMessage
}
```

**Why separate stream type?**
- Allows different payload types on same stream
- Client knows when stream is complete
- Error can be sent mid-stream

### Stream concurrency

The Launcher can request multiple streams simultaneously (e.g., multiple users watching job output, or status streams for different jobs). The SDK handles this by:

- Creating a new goroutine for each streaming request
- Each goroutine receives its own `context.Context` for cancellation
- When the Launcher sends a cancel request for a stream, the SDK cancels the corresponding context
- Job status streams use the cache's pub/sub, so multiple subscribers can watch the same job without additional scheduler queries
- Output and resource utilization streams are independent instances -- each one queries the scheduler separately

**Important**: Because multiple streams can be active simultaneously, your plugin's scheduler interaction code should be safe for concurrent use.

## Runtime architecture

### Request routing

The Runtime dispatches requests to plugin methods:

```
Request arrives → Parse type → Route to method → Execute → Send response
```

```go
func (r *Runtime) handleRequest(ctx context.Context, req *protocol.Request) {
    switch req.Type {
    case "submit_job":
        r.plugin.SubmitJob(...)
    case "get_job":
        r.plugin.GetJob(...)
    // ... 8 more cases
    }
}
```

**Why switch-based routing?**
- Simple and explicit
- Easy to debug
- Fast (no reflection)
- Type-safe

### Context propagation

Each request gets a context:

```go
ctx, cancel := context.WithCancel(parentCtx)
defer cancel()

go r.plugin.GetJobOutput(ctx, w, user, id, outputType)
```

**Why context?**
- Cancellation propagates (client disconnect stops work)
- Timeouts can be added at runtime level
- Standard Go pattern for cancellation

### Error handling

Errors are typed with error codes:

```go
type Error struct {
    Code    int    // CodeJobNotFound, CodeInvalidJobState, etc.
    Message string
}
```

**Why error codes?**
- Workbench and Connect can handle errors differently based on code
- Clients can retry transient errors (CodeTimeout)
- Better than string parsing
- Follows Launcher API specification

## Job cache design

### Storage and startup

The cache uses in-memory storage. The scheduler is the source of truth for job state — the cache is a local working copy that plugins populate at startup and keep in sync during operation.

```go
cache, _ := cache.NewJobCache(ctx, lgr)
```

Plugins should implement `BootstrappedPlugin` and use `Bootstrap()` to re-read active jobs from the scheduler into the cache before accepting requests. A periodic sync loop (e.g., every 5 seconds) should then reconcile cache state with the scheduler during normal operation. This is consistent with how all existing Launcher plugins (Local, Kubernetes, Slurm) operate.

### Pub/sub for status updates

Cache includes built-in pub/sub:

```go
// Subscriber
cache.StreamJobStatus(ctx, w, user, jobID)

// Publisher (different goroutine)
cache.Update(user, jobID, func(job *api.Job) *api.Job {
    job.Status = api.StatusRunning  // Triggers notification
    return job
})
```

**Why built-in pub/sub?**
- No external message queue needed
- In-process is fast
- Automatic cleanup when subscribers disconnect
- Matches Launcher's streaming model

**Implementation**:
```go
type JobCache struct {
    subscribers map[string][]chan *api.Job  // jobID -> subscribers
    mu          sync.RWMutex
}
```

### Permission enforcement

Cache enforces user isolation:

```go
cache.WriteJob(w, "alice", jobID)  // Only succeeds if job.User == "alice"
```

**Why in cache?**
- Prevents permission bugs in plugin code
- Consistent across all operations
- Single source of truth

### Job expiration

Old jobs are automatically removed:

```go
go cache.cleanupExpiredJobs(ctx, expiry)
```

**Why automatic?**
- Prevents cache from growing unbounded
- Plugin doesn't need to track expiration
- Configurable per deployment

## Testing utilities

### Mock ResponseWriter

Captures all responses for assertions:

```go
w := plugintest.NewMockResponseWriter()
plugin.SubmitJob(context.Background(), w, "alice", job)

assert.True(t, w.HasError() == false)
assert.Equal(t, 1, len(w.AllJobs()))
```

**Why capture all responses?**
- Single call may write multiple times
- Helps test error handling
- Makes assertions straightforward

**Thread-safety**: Mock uses `sync.Mutex` because plugin methods might spawn goroutines.

### Fluent builders

Readable test data construction:

```go
job := plugintest.NewJob().
    WithUser("alice").
    WithCommand("python train.py").
    WithMemory("8GB").
    Running().
    Build()
```

**Why fluent API?**
- Tests read like English
- Only specify what matters for the test
- Defaults handle required fields
- Discoverability via autocomplete

### Assertion helpers

Descriptive error messages:

```go
func AssertJobStatus(t *testing.T, job *api.Job, expected string) {
    if job.Status != expected {
        t.Errorf("Expected job status %s, got %s (job: %+v)",
            expected, job.Status, job)
    }
}
```

**Why custom assertions?**
- Better error messages than `assert.Equal`
- Job-specific context in failures
- Reduce test boilerplate

## Conformance testing

### Why conformance tests?

Plugin authors implement the `launcher.Plugin` interface but historically had no automated way to verify their implementation will work correctly with Posit products. The `plugintest` package provides unit-test building blocks (mocks, builders, assertions) but no orchestrated test scenarios. Conformance tests fill this gap: automated behavioral tests that verify a plugin handles the request sequences Posit products produce.

### Three-tier architecture

```go
// Tier 1: Universal invariants
conformance.Run(t, plugin, user, profile)

// Tier 2: Product workflow tests
conformance.RunWorkflows(t, plugin, user, profile)

// Tier 3: Individual scenarios
conformance.RunStopJob(t, plugin, user, opts)
```

**Why three tiers?**
- **Tier 1** catches fundamental contract violations (missing IDs, wrong error codes)
- **Tier 2** verifies the end-to-end request sequences products rely on
- **Tier 3** allows testing specific behaviors in isolation, useful for debugging

Each tier builds on the one below — `RunWorkflows` calls the Tier 3 scenario functions internally. Plugin authors can use any combination.

### Profile-based parameterization

Different schedulers produce different outcomes for identical operations. For example, `ControlJob(Stop)` yields `StatusFinished` on Local/Kubernetes but `StatusKilled` on Slurm (because `scancel` reports a KILLED state). Rather than hard-coding expectations, the `Profile` struct parameterizes these deltas:

```go
type Profile struct {
    JobFactory     func(user string) *api.Job  // How to create a submittable job
    LongRunningJob func(user string) *api.Job  // How to create a job for control tests
    StopStatus     string                      // Terminal status after Stop
    KillExitCodes  []int                       // Acceptable exit codes after Kill
    // ...
}
```

**Why a single struct?** Both `Run` and `RunWorkflows` need the same behavioral parameters. A single struct avoids duplicating configuration across tiers and makes it clear that the deltas are a property of the plugin, not of the test tier.

### Helper design decisions

Exported helpers (`SubmitJob`, `WaitForStatus`, `WaitForTerminalStatus`, etc.) follow two patterns:

- **Fatal on prerequisite failure**: `SubmitJob` calls `t.Fatal` because subsequent test logic can't proceed without a job ID
- **Return errors for assertions**: `GetJob`, `WaitForStatus` return errors so callers can assert on error conditions or decide how to handle timeouts

**Why poll-based waiting?** `WaitForStatus` polls `GetJob` rather than using the streaming API. This is deliberate: it tests the same code path products use for quick status checks, and it avoids goroutine management complexity in test code. The streaming API is tested separately in `RunStatusStream`.

### Relationship to plugintest

The conformance package depends on `plugintest` (mocks, assertions) but not the reverse. This maintains a clear dependency direction:

```
conformance → plugintest → launcher, api
```

Plugin authors use `plugintest` for unit tests and `conformance` for behavioral verification. The two packages complement each other.

## Design patterns

### Interface-based design

Core types are interfaces:

```go
type Plugin interface { /* ... */ }
type ResponseWriter interface { /* ... */ }
```

**Benefits**:
- Easy to mock for testing
- Allows alternative implementations
- Clear contracts
- Follows Go conventions

### Functional options

Used in builders:

```go
type JobBuilder struct { /* ... */ }

func (b *JobBuilder) WithUser(user string) *JobBuilder {
    b.job.User = user
    return b
}
```

**Benefits**:
- Chainable
- Self-documenting
- Optional parameters
- Type-safe

### Callback pattern

Used in cache updates:

```go
cache.Update(user, jobID, func(job *api.Job) *api.Job {
    job.Status = api.StatusRunning
    return job
})
```

**Benefits**:
- Atomic updates
- Job only locked during callback
- Clear what's being modified
- Can abort by returning unchanged job

### Context-based cancellation

All streaming methods accept context:

```go
func (p *Plugin) GetJobOutput(ctx context.Context, w StreamResponseWriter, ...) {
    for {
        select {
        case <-ctx.Done():
            return  // Client disconnected
        case data := <-outputChan:
            w.WriteJobOutput(data, outputType)
        }
    }
}
```

**Benefits**:
- Automatic cleanup on client disconnect
- Standard Go pattern
- Works with timeouts
- Composable

## Design decisions

### Why Go?

Chosen over C++, Python, Java:

**Pros**:
- Fast compilation and startup
- Small binary size
- Excellent concurrency primitives
- Strong standard library
- Easy deployment (single binary)
- Good HTTP/gRPC libraries

**Cons**:
- No generics in scheduler code (Go 1.18+)
- Less mature than C++ SDK
- Smaller ecosystem than Python

**Verdict**: Go's simplicity and deployment model outweigh the cons.

### Why stdin/stdout protocol?

Alternative: gRPC, HTTP

**Pros of stdin/stdout**:
- Simple process model
- No port management
- Automatic cleanup (process death)
- Works in containers
- Matches existing C++ SDK

**Cons**:
- Can't debug with multiple plugins
- No connection multiplexing

**Verdict**: Simplicity wins for this use case.

### Why separate `api` package?

Alternative: Types in `launcher` package

**Pros of separate**:
- No circular dependencies
- Clear API boundary
- Can import types without launcher
- Matches Posit product conventions

**Cons**:
- More packages to import
- Longer type names (`api.Job`)

**Verdict**: Separation provides better structure.

### Why hide protocol package?

Alternative: Export protocol types

**Pros of hiding**:
- Plugin can't depend on protocol details
- Free to change protocol implementation
- Cleaner API surface
- Prevents misuse

**Cons**:
- Can't customize protocol
- Can't reuse types

**Verdict**: Hiding gives flexibility for future changes.

## Trade-offs

### Type safety vs flexibility

**Decision**: Prefer type safety

```go
// Type-safe (chosen)
type JobStatus string
const StatusPending JobStatus = "Pending"

// vs flexible (rejected)
type JobStatus string  // any string
```

**Rationale**: Catch errors at compile time, better IDE support.

### Caching vs fresh data

**Decision**: Cache with expiration

Plugins cache jobs rather than querying scheduler every time. Stale data is acceptable for short periods.

**Rationale**: Reduces scheduler load, faster responses, acceptable for UI.

### In-process vs external pub/sub

**Decision**: In-process pub/sub

Alternatives: Redis, NATS

**Rationale**: One plugin = one process. No cross-plugin communication needed. External pub/sub adds complexity and dependencies.

### Polling vs webhooks

**Decision**: Polling for job status

Most schedulers (Slurm, PBS, LSF) don't provide event notifications.

**Rationale**: Matches scheduler capabilities, simpler implementation, consistent behavior.

### Testing approach

**Decision**: Provide utilities, not framework

Alternatives: Full testing framework, test runner

**Rationale**: Plugins should use standard `go test`. Utilities (mocks, builders, assertions) are sufficient.

## Future considerations

### API stability

Current version: **v0.x** (pre-1.0)

Breaking changes allowed in minor versions with migration guides. After v1.0, semantic versioning with backwards compatibility guarantees.

### Advanced features

Potential additions:

1. **Job Dependencies**: Directed Acyclic Graph (DAG) execution
2. **Autoscaling**: Scale cluster based on load
3. **Cost Tracking**: Track compute costs per job
4. **Spot Instance Support**: Preemptible jobs

All can be added via new interfaces without breaking existing plugins.

### Multi-cluster improvements

Current multi-cluster support is basic. Future improvements:

1. **Cluster Health**: Monitor cluster availability
2. **Failover**: Automatic cluster switching
3. **Load Balancing**: Distribute jobs across clusters
4. **Cluster Groups**: Logical grouping of clusters

### Observability

Potential additions:

1. **Metrics**: Prometheus endpoint for job metrics
2. **Tracing**: OpenTelemetry support
3. **Health Checks**: HTTP endpoint for monitoring
4. **Profiling**: pprof endpoint for debugging

### Performance optimizations

Potential improvements:

1. **Connection Pooling**: Reuse scheduler connections
2. **Result Caching**: Cache scheduler queries
3. **Batch Status Updates**: Update multiple jobs atomically
4. **Lazy Loading**: Load job details on demand

### Security enhancements

Potential additions:

1. **Audit Logging**: Track all operations
2. **Secret Management**: Integration with secret stores
3. **Role-Based Access Control (RBAC)**
4. **Encryption**: Encrypt job data at rest

## Conclusion

The Launcher Go SDK architecture prioritizes:

1. **Developer experience** - Simple, intuitive API
2. **Type safety** - Catch errors at compile time
3. **Testability** - Easy to write good tests
4. **Performance** - Fast enough for production
5. **Extensibility** - Can evolve without breaking changes

These principles guide all design decisions and will continue to guide future development.

## References

- [C++ Launcher Plugin SDK](https://github.com/rstudio/rstudio-launcher-plugin-sdk)
- [Go Proverbs](https://go-proverbs.github.io/)
- [Effective Go](https://golang.org/doc/effective_go)
