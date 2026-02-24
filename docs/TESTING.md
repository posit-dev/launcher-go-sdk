# Testing Guide

This guide shows how to test launcher plugins using the SDK's testing utilities.

## Table of contents

1. [Overview](#overview)
2. [Conformance Testing](#conformance-testing)
3. [Testing Utilities](#testing-utilities)
4. [Unit Testing](#unit-testing)
5. [Integration Testing](#integration-testing)
6. [Testing Patterns](#testing-patterns)
7. [Coverage](#coverage)
8. [Best Practices](#best-practices)
9. [Common Scenarios](#common-scenarios)

## Overview

The SDK provides comprehensive testing utilities in the `plugintest` package:

- **Mocks**: MockResponseWriter and MockStreamResponseWriter
- **Builders**: Fluent APIs for creating test data
- **Assertions**: Helpful test assertions
- **Helpers**: Utility functions for common test operations

### Test philosophy

Good plugin tests should:

1. **Be fast** - Unit tests should run in milliseconds
2. **Be isolated** - Each test should be independent
3. **Be readable** - Clear intent and expectations
4. **Cover edge cases** - Test error conditions, not just happy paths
5. **Use real implementations** - Avoid over-mocking

## Conformance testing

The `conformance` package provides automated behavioral tests that verify your plugin implementation conforms to the contracts expected by Posit products (Workbench, Connect). Rather than testing individual methods in isolation, conformance tests replay the request sequences that products actually produce.

### Quick start

```go
import (
    "testing"

    "github.com/posit-dev/launcher-go-sdk/api"
    "github.com/posit-dev/launcher-go-sdk/conformance"
)

func TestConformance(t *testing.T) {
    plugin := setupMyPlugin() // Your plugin instance

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

### Three-tier architecture

The conformance package is organized into three tiers of increasing specificity:

**Tier 1: Universal invariants** (`conformance.Run`)

Tests fundamental protocol requirements that every plugin must satisfy:

- Submit returns a non-empty ID and sets a Submitted timestamp
- Two submissions produce different IDs
- GetJob returns by ID and returns `CodeJobNotFound` for bogus IDs
- GetJobs filters by tag (AND logic) and status
- ClusterInfo returns without error
- Jobs reach a terminal status that never regresses

```bash
go test -run TestConformance/Invariants/Submit/ReturnsNonEmptyID
```

**Tier 2: Product workflows** (`conformance.RunWorkflows`)

Replays the request sequences Posit products produce during normal operation:

- Launch workflow: ClusterInfo → Submit → stream status → wait Running → GetJobNetwork → stream output
- Stop/Kill/Cancel workflows with expected terminal statuses and exit codes
- List workflows with status and tag filtering
- Suspend/Resume (skipped if not supported)

```bash
go test -run TestConformance/Workflows/Launch
go test -run TestConformance/Workflows/Stop
```

**Tier 3: Individual scenarios** (`RunStopJob`, `RunKillJob`, etc.)

Parameterized scenario functions that can be called directly for isolated testing:

```go
conformance.RunStopJob(t, plugin, "testuser", conformance.StopOpts{
    Job:             &api.Job{User: "testuser", Command: "sleep 300"},
    ExpectStatus:    api.StatusFinished,
    ExpectExitCodes: []int{143},
})
```

Available scenarios: `RunStopJob`, `RunKillJob`, `RunCancelJob`, `RunSuspendResume`, `RunOutputStream`, `RunStatusStream`, `RunStreamCancellation`, `RunFieldFiltering`, `RunControlInvalidState`.

### Profile configuration

The `Profile` struct parameterizes cross-plugin behavioral deltas. Different schedulers produce different outcomes for the same operations:

| Field | Local/K8s | Slurm |
|-------|-----------|-------|
| `StopStatus` | `StatusFinished` | `StatusKilled` |
| `StopExitCodes` | `[]int{143}` | `[]int{143}` |
| `KillExitCodes` | `[]int{137}` | `[]int{137, 143}` |
| `OutputAvailable` | `true` | `false` |

Required fields: `JobFactory`, `LongRunningJob`, `StopStatus`, `StopExitCodes`, `KillStatus`, `KillExitCodes`. Optional fields have sensible defaults (e.g., `PollInterval` defaults to 50ms, `JobStartTimeout` to 30s).

### Exported helpers

The conformance package exports helpers that you can reuse in your own custom tests:

```go
// Submit a job and get its ID (fails test on error)
id := conformance.SubmitJob(t, plugin, "testuser", job)

// Wait for a specific status with timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
job, err := conformance.WaitForStatus(ctx, plugin, "testuser", id, api.StatusRunning)

// Wait for any terminal status
job, err := conformance.WaitForTerminalStatus(ctx, plugin, "testuser", id)

// Collect a status stream in the background
sw, done := conformance.CollectStatusStream(ctx, plugin, "testuser")
// ... later ...
cancel()
<-done // wait for goroutine to exit
```

## Testing utilities

### MockResponseWriter

Captures all responses for assertions:

```go
import "github.com/posit-dev/launcher-go-sdk/plugintest"

w := plugintest.NewMockResponseWriter()
plugin.SubmitJob(w, "alice", job)

// Check for errors
if w.HasError() {
    t.Errorf("Unexpected error: %v", w.LastError())
}

// Check jobs
jobs := w.AllJobs()
if len(jobs) != 1 {
    t.Errorf("Expected 1 job, got %d", len(jobs))
}
```

### MockStreamResponseWriter

For testing streaming methods:

```go
w := plugintest.NewMockStreamResponseWriter()
ctx := context.Background()

plugin.GetJobStatus(ctx, w, "alice", "job-123")

// Check stream was closed
if !w.IsClosed() {
    t.Error("Stream was not closed")
}

// Check statuses sent
statuses := w.AllStatuses()
if len(statuses) < 2 {
    t.Errorf("Expected at least 2 status updates, got %d", len(statuses))
}
```

### JobBuilder

Create test jobs easily:

```go
job := plugintest.NewJob().
    WithUser("alice").
    WithCommand("python train.py").
    WithMemory("8GB").
    WithCPUCount(4).
    WithTag("ml-training").
    Running().
    Build()
```

### Assertions

Helpful assertions with good error messages:

```go
// Assert no errors
plugintest.AssertNoError(t, w)

// Assert specific error code
plugintest.AssertErrorCode(t, w, api.CodeJobNotFound)

// Assert job count
plugintest.AssertJobCount(t, w, 3)

// Assert job status
plugintest.AssertJobStatus(t, job, api.StatusRunning)

// Assert stream was closed
plugintest.AssertStreamClosed(t, streamWriter)
```

## Unit testing

### Testing SubmitJob

```go
func TestSubmitJob(t *testing.T) {
    // Setup
    ctx := context.Background()
    lgr := logger.MustNewLogger("test", false, "")
    cache, _ := cache.NewJobCache(ctx, lgr, "")
    plugin := &MyPlugin{cache: cache}

    // Create test job
    job := plugintest.NewJob().
        WithUser("alice").
        WithCommand("echo hello").
        Build()

    // Execute
    w := plugintest.NewMockResponseWriter()
    plugin.SubmitJob(w, "alice", job)

    // Assert
    plugintest.AssertNoError(t, w)
    plugintest.AssertJobCount(t, w, 1)

    returnedJob := w.LastJobs()[0]
    plugintest.AssertJobStatus(t, returnedJob, api.StatusPending)

    if returnedJob.ID == "" {
        t.Error("Job ID was not assigned")
    }
}
```

### Testing GetJob

```go
func TestGetJob(t *testing.T) {
    // Setup
    ctx := context.Background()
    lgr := logger.MustNewLogger("test", false, "")
    cache, _ := cache.NewJobCache(ctx, lgr, "")
    plugin := &MyPlugin{cache: cache}

    // Add a job to cache
    job := plugintest.NewJob().
        WithID("job-123").
        WithUser("alice").
        Pending().
        Build()
    cache.AddOrUpdate(job)

    // Execute
    w := plugintest.NewMockResponseWriter()
    plugin.GetJob(w, "alice", "job-123", nil)

    // Assert
    plugintest.AssertNoError(t, w)
    plugintest.AssertJobCount(t, w, 1)

    returnedJob := w.LastJobs()[0]
    if returnedJob.ID != "job-123" {
        t.Errorf("Expected job ID job-123, got %s", returnedJob.ID)
    }
}
```

### Testing GetJob - not found

```go
func TestGetJob_NotFound(t *testing.T) {
    // Setup
    ctx := context.Background()
    lgr := logger.MustNewLogger("test", false, "")
    cache, _ := cache.NewJobCache(ctx, lgr, "")
    plugin := &MyPlugin{cache: cache}

    // Execute (job doesn't exist)
    w := plugintest.NewMockResponseWriter()
    plugin.GetJob(w, "alice", "nonexistent", nil)

    // Assert error
    plugintest.AssertErrorCode(t, w, api.CodeJobNotFound)
}
```

### Testing GetJobs with filtering

```go
func TestGetJobs_FilterByStatus(t *testing.T) {
    // Setup
    ctx := context.Background()
    lgr := logger.MustNewLogger("test", false, "")
    cache, _ := cache.NewJobCache(ctx, lgr, "")
    plugin := &MyPlugin{cache: cache}

    // Add multiple jobs with different statuses
    jobs := []*api.Job{
        plugintest.NewJob().WithID("job-1").WithUser("alice").Pending().Build(),
        plugintest.NewJob().WithID("job-2").WithUser("alice").Running().Build(),
        plugintest.NewJob().WithID("job-3").WithUser("alice").Finished().Build(),
    }
    for _, job := range jobs {
        cache.AddOrUpdate(job)
    }

    // Execute with filter
    filter := plugintest.NewJobFilter().
        WithStatus(api.StatusRunning).
        Build()

    w := plugintest.NewMockResponseWriter()
    plugin.GetJobs(w, "alice", filter, nil)

    // Assert
    plugintest.AssertNoError(t, w)
    plugintest.AssertJobCount(t, w, 1)

    returnedJob := w.LastJobs()[0]
    plugintest.AssertJobStatus(t, returnedJob, api.StatusRunning)
}
```

### Testing ControlJob

```go
func TestControlJob_Kill(t *testing.T) {
    // Setup
    ctx := context.Background()
    lgr := logger.MustNewLogger("test", false, "")
    cache, _ := cache.NewJobCache(ctx, lgr, "")
    plugin := &MyPlugin{cache: cache}

    // Add a running job
    job := plugintest.NewJob().
        WithID("job-123").
        WithUser("alice").
        Running().
        Build()
    cache.AddOrUpdate(job)

    // Execute kill operation
    w := plugintest.NewMockResponseWriter()
    plugin.ControlJob(w, "alice", "job-123", api.OperationKill)

    // Assert operation succeeded
    plugintest.AssertNoError(t, w)

    // Verify job status changed
    w2 := plugintest.NewMockResponseWriter()
    plugin.GetJob(w2, "alice", "job-123", nil)

    updatedJob := w2.LastJobs()[0]
    plugintest.AssertJobStatus(t, updatedJob, api.StatusKilled)
}
```

### Testing ControlJob - invalid state

```go
func TestControlJob_InvalidState(t *testing.T) {
    // Setup
    ctx := context.Background()
    lgr := logger.MustNewLogger("test", false, "")
    cache, _ := cache.NewJobCache(ctx, lgr, "")
    plugin := &MyPlugin{cache: cache}

    // Add a finished job
    job := plugintest.NewJob().
        WithID("job-123").
        WithUser("alice").
        Finished().
        Build()
    cache.AddOrUpdate(job)

    // Try to kill a finished job (invalid)
    w := plugintest.NewMockResponseWriter()
    plugin.ControlJob(w, "alice", "job-123", api.OperationKill)

    // Assert error
    plugintest.AssertErrorCode(t, w, api.CodeInvalidJobState)
}
```

### Testing GetJobStatus (streaming)

```go
func TestGetJobStatus(t *testing.T) {
    // Setup
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    lgr := logger.MustNewLogger("test", false, "")
    cache, _ := cache.NewJobCache(ctx, lgr, "")
    plugin := &MyPlugin{cache: cache}

    // Add a job
    job := plugintest.NewJob().
        WithID("job-123").
        WithUser("alice").
        Pending().
        Build()
    cache.AddOrUpdate(job)

    // Start streaming in background
    w := plugintest.NewMockStreamResponseWriter()
    done := make(chan struct{})

    go func() {
        plugin.GetJobStatus(ctx, w, "alice", "job-123")
        close(done)
    }()

    // Update job status
    time.Sleep(100 * time.Millisecond)
    cache.Update("alice", "job-123", func(j *api.Job) *api.Job {
        j.Status = api.StatusRunning
        return j
    })

    // Wait for completion
    <-done

    // Assert
    plugintest.AssertMinimumStatuses(t, w, 2) // Initial + update

    statuses := w.AllStatuses()
    if statuses[0].Status != api.StatusPending {
        t.Errorf("First status should be Pending, got %s", statuses[0].Status)
    }
    if statuses[len(statuses)-1].Status != api.StatusRunning {
        t.Errorf("Last status should be Running, got %s", statuses[len(statuses)-1].Status)
    }
}
```

### Testing GetJobOutput

```go
func TestGetJobOutput(t *testing.T) {
    // Setup
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    lgr := logger.MustNewLogger("test", false, "")
    cache, _ := cache.NewJobCache(ctx, lgr, "")
    plugin := &MyPlugin{cache: cache}

    // Add a job
    job := plugintest.NewJob().
        WithID("job-123").
        WithUser("alice").
        Running().
        Build()
    cache.AddOrUpdate(job)

    // Execute
    w := plugintest.NewMockStreamResponseWriter()
    done := make(chan struct{})

    go func() {
        plugin.GetJobOutput(ctx, w, "alice", "job-123", api.OutputStdout)
        close(done)
    }()

    // Wait for completion
    <-done

    // Assert
    plugintest.AssertStreamClosed(t, w)

    output := w.CombinedOutput()
    if output == "" {
        t.Error("Expected some output")
    }
}
```

### Testing ClusterInfo

```go
func TestClusterInfo(t *testing.T) {
    // Setup
    ctx := context.Background()
    lgr := logger.MustNewLogger("test", false, "")
    cache, _ := cache.NewJobCache(ctx, lgr, "")
    plugin := &MyPlugin{cache: cache}

    // Execute
    w := plugintest.NewMockResponseWriter()
    plugin.ClusterInfo(w, "alice")

    // Assert
    plugintest.AssertNoError(t, w)

    // Check cluster info fields
    if w.ClusterInfo == nil {
        t.Fatal("ClusterInfo was not set")
    }

    if len(w.ClusterInfo.Queues) == 0 {
        t.Error("Expected at least one queue")
    }

    if w.ClusterInfo.DefaultQueue == "" {
        t.Error("Default queue should be set")
    }
}
```

## Integration testing

Integration tests verify the plugin works with real schedulers.

### Testing with real cache

```go
func TestSubmitJob_WithPersistence(t *testing.T) {
    // Create temporary directory
    tmpDir := t.TempDir()

    // Setup with persistent cache
    ctx := context.Background()
    lgr := logger.MustNewLogger("test", false, "")
    cache, err := cache.NewJobCache(ctx, lgr, tmpDir)
    if err != nil {
        t.Fatalf("Failed to create cache: %v", err)
    }
    defer cache.Close()

    plugin := &MyPlugin{cache: cache}

    // Submit job
    job := plugintest.NewJob().
        WithUser("alice").
        WithCommand("echo hello").
        Build()

    w := plugintest.NewMockResponseWriter()
    plugin.SubmitJob(w, "alice", job)

    plugintest.AssertNoError(t, w)
    jobID := w.LastJobs()[0].ID

    // Create new cache instance (simulates restart)
    cache2, err := cache.NewJobCache(ctx, lgr, tmpDir)
    if err != nil {
        t.Fatalf("Failed to create cache: %v", err)
    }
    defer cache2.Close()

    plugin2 := &MyPlugin{cache: cache2}

    // Verify job persisted
    w2 := plugintest.NewMockResponseWriter()
    plugin2.GetJob(w2, "alice", api.JobID(jobID), nil)

    plugintest.AssertNoError(t, w2)
    plugintest.AssertJobCount(t, w2, 1)
}
```

### Testing with external scheduler

```go
// +build integration

func TestSubmitJob_RealSlurm(t *testing.T) {
    if os.Getenv("SLURM_INTEGRATION_TEST") == "" {
        t.Skip("Skipping Slurm integration test")
    }

    // Setup
    ctx := context.Background()
    lgr := logger.MustNewLogger("test", true, "")
    cache, _ := cache.NewJobCache(ctx, lgr, "")

    // Create real Slurm client
    client := NewSlurmClient("/usr/bin")
    plugin := &SlurmPlugin{
        cache:  cache,
        client: client,
        lgr:    lgr,
    }

    // Submit real job
    job := plugintest.NewJob().
        WithUser(os.Getenv("USER")).
        WithCommand("hostname").
        Build()

    w := plugintest.NewMockResponseWriter()
    plugin.SubmitJob(w, os.Getenv("USER"), job)

    plugintest.AssertNoError(t, w)

    jobID := w.LastJobs()[0].ID
    t.Logf("Submitted job %s to Slurm", jobID)

    // Wait for job to complete
    ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
    defer cancel()

    for {
        time.Sleep(5 * time.Second)

        w2 := plugintest.NewMockResponseWriter()
        plugin.GetJob(w2, os.Getenv("USER"), api.JobID(jobID), nil)

        if w2.HasError() {
            t.Fatalf("Error getting job: %v", w2.LastError())
        }

        job := w2.LastJobs()[0]
        if api.TerminalStatus(job.Status) {
            t.Logf("Job completed with status: %s", job.Status)
            break
        }

        if ctx.Err() != nil {
            t.Fatal("Job did not complete in time")
        }
    }
}
```

## Testing patterns

### Table-driven tests

```go
func TestControlJob_Operations(t *testing.T) {
    tests := []struct {
        name          string
        initialStatus string
        operation     api.JobOperation
        expectedStatus string
        expectError   bool
        errorCode     int
    }{
        {
            name:          "cancel pending job",
            initialStatus: api.StatusPending,
            operation:     api.OperationCancel,
            expectedStatus: api.StatusCanceled,
            expectError:   false,
        },
        {
            name:          "kill running job",
            initialStatus: api.StatusRunning,
            operation:     api.OperationKill,
            expectedStatus: api.StatusKilled,
            expectError:   false,
        },
        {
            name:          "cancel running job (invalid)",
            initialStatus: api.StatusRunning,
            operation:     api.OperationCancel,
            expectError:   true,
            errorCode:     api.CodeInvalidJobState,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup
            ctx := context.Background()
            lgr := logger.MustNewLogger("test", false, "")
            cache, _ := cache.NewJobCache(ctx, lgr, "")
            plugin := &MyPlugin{cache: cache}

            // Add job with initial status
            job := plugintest.NewJob().
                WithID("job-123").
                WithUser("alice").
                WithStatus(tt.initialStatus).
                Build()
            cache.AddOrUpdate(job)

            // Execute
            w := plugintest.NewMockResponseWriter()
            plugin.ControlJob(w, "alice", "job-123", tt.operation)

            // Assert
            if tt.expectError {
                plugintest.AssertErrorCode(t, w, tt.errorCode)
            } else {
                plugintest.AssertNoError(t, w)

                // Verify status changed
                w2 := plugintest.NewMockResponseWriter()
                plugin.GetJob(w2, "alice", "job-123", nil)
                updatedJob := w2.LastJobs()[0]
                plugintest.AssertJobStatus(t, updatedJob, tt.expectedStatus)
            }
        })
    }
}
```

### Test helpers

```go
// testPlugin returns a configured plugin for testing
func testPlugin(t *testing.T) (*MyPlugin, *cache.JobCache) {
    t.Helper()

    ctx := context.Background()
    lgr := logger.MustNewLogger("test", false, "")
    cache, err := cache.NewJobCache(ctx, lgr, "")
    if err != nil {
        t.Fatalf("Failed to create cache: %v", err)
    }

    return &MyPlugin{cache: cache}, cache
}

// submitTestJob submits a job and returns its ID
func submitTestJob(t *testing.T, plugin *MyPlugin, user string) string {
    t.Helper()

    job := plugintest.NewJob().
        WithUser(user).
        WithCommand("echo test").
        Build()

    w := plugintest.NewMockResponseWriter()
    plugin.SubmitJob(w, user, job)

    plugintest.AssertNoError(t, w)
    return w.LastJobs()[0].ID
}

// Usage
func TestMyFeature(t *testing.T) {
    plugin, cache := testPlugin(t)
    jobID := submitTestJob(t, plugin, "alice")

    // Test something with the job
    // ...
}
```

### Subtests for readability

```go
func TestSubmitJob(t *testing.T) {
    plugin, _ := testPlugin(t)

    t.Run("success", func(t *testing.T) {
        job := plugintest.NewJob().
            WithUser("alice").
            WithCommand("echo hello").
            Build()

        w := plugintest.NewMockResponseWriter()
        plugin.SubmitJob(w, "alice", job)

        plugintest.AssertNoError(t, w)
    })

    t.Run("assigns unique ID", func(t *testing.T) {
        job := plugintest.NewJob().
            WithUser("alice").
            WithCommand("echo hello").
            Build()

        w := plugintest.NewMockResponseWriter()
        plugin.SubmitJob(w, "alice", job)

        jobID := w.LastJobs()[0].ID
        if jobID == "" {
            t.Error("Job ID was not assigned")
        }
    })

    t.Run("sets initial status", func(t *testing.T) {
        job := plugintest.NewJob().
            WithUser("alice").
            WithCommand("echo hello").
            Build()

        w := plugintest.NewMockResponseWriter()
        plugin.SubmitJob(w, "alice", job)

        returnedJob := w.LastJobs()[0]
        plugintest.AssertJobStatus(t, returnedJob, api.StatusPending)
    })
}
```

## Coverage

### Running with coverage

```bash
# Run tests with coverage
go test ./... -cover

# Generate coverage report
go test ./... -coverprofile=coverage.out

# View coverage in browser
go tool cover -html=coverage.out
```

### Coverage goals

Aim for:
- **80%+ overall coverage** - Good baseline
- **100% of error paths** - Test all error conditions
- **100% of public API** - All exported functions tested

Don't chase 100% blindly. Focus on critical paths and edge cases.

### Coverage by package

```bash
# Coverage for specific package
go test ./launcher -cover

# Detailed coverage report
go test ./launcher -coverprofile=launcher.out
go tool cover -func=launcher.out
```

## Best practices

### 1. Use table-driven tests

**Good**:
```go
func TestJobStatus(t *testing.T) {
    tests := []struct {
        name   string
        status string
        want   bool
    }{
        {"pending is not terminal", api.StatusPending, false},
        {"running is not terminal", api.StatusRunning, false},
        {"finished is terminal", api.StatusFinished, true},
        {"failed is terminal", api.StatusFailed, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := api.TerminalStatus(tt.status)
            if got != tt.want {
                t.Errorf("TerminalStatus(%s) = %v, want %v", tt.status, got, tt.want)
            }
        })
    }
}
```

**Bad**:
```go
func TestJobStatusPending(t *testing.T) {
    got := api.TerminalStatus(api.StatusPending)
    if got != false {
        t.Error("pending should not be terminal")
    }
}

func TestJobStatusFinished(t *testing.T) {
    got := api.TerminalStatus(api.StatusFinished)
    if got != true {
        t.Error("finished should be terminal")
    }
}
// ... many more similar functions
```

### 2. Test error cases

**Good**:
```go
func TestGetJob_NotFound(t *testing.T) {
    plugin, _ := testPlugin(t)

    w := plugintest.NewMockResponseWriter()
    plugin.GetJob(w, "alice", "nonexistent", nil)

    plugintest.AssertErrorCode(t, w, api.CodeJobNotFound)
}
```

### 3. Use meaningful test names

**Good**:
- `TestSubmitJob_AssignsUniqueID`
- `TestControlJob_KillRunningJob`
- `TestGetJob_PermissionDenied`

**Bad**:
- `TestSubmit1`
- `TestKill`
- `TestError`

### 4. Keep tests fast

**Good**:
```go
// Use short timeouts
ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
defer cancel()
```

**Bad**:
```go
// Don't use long sleeps
time.Sleep(10 * time.Second)
```

### 5. Use t.Helper()

```go
func assertJobExists(t *testing.T, plugin *MyPlugin, jobID string) {
    t.Helper() // Makes failures report correct line numbers

    w := plugintest.NewMockResponseWriter()
    plugin.GetJob(w, "alice", api.JobID(jobID), nil)

    if w.HasError() {
        t.Fatalf("Job %s not found", jobID)
    }
}
```

### 6. Clean up resources

```go
func TestWithTempDir(t *testing.T) {
    tmpDir := t.TempDir() // Automatically cleaned up

    cache, _ := cache.NewJobCache(ctx, lgr, tmpDir)
    defer cache.Close() // Ensure cleanup

    // Test code...
}
```

### 7. Use parallel tests when possible

```go
func TestMultipleOperations(t *testing.T) {
    t.Parallel() // Run this test in parallel

    // Test code...
}
```

## Common scenarios

### Testing permission enforcement

```go
func TestGetJob_WrongUser(t *testing.T) {
    plugin, cache := testPlugin(t)

    // Alice submits a job
    job := plugintest.NewJob().
        WithID("job-123").
        WithUser("alice").
        Build()
    cache.AddOrUpdate(job)

    // Bob tries to get Alice's job
    w := plugintest.NewMockResponseWriter()
    plugin.GetJob(w, "bob", "job-123", nil)

    // Should get permission denied error
    plugintest.AssertErrorCode(t, w, api.CodeJobNotFound)
}
```

### Testing concurrent access

```go
func TestConcurrentJobSubmission(t *testing.T) {
    plugin, _ := testPlugin(t)

    const numJobs = 100
    var wg sync.WaitGroup
    wg.Add(numJobs)

    for i := 0; i < numJobs; i++ {
        go func(i int) {
            defer wg.Done()

            job := plugintest.NewJob().
                WithUser("alice").
                WithCommand(fmt.Sprintf("echo %d", i)).
                Build()

            w := plugintest.NewMockResponseWriter()
            plugin.SubmitJob(w, "alice", job)

            plugintest.AssertNoError(t, w)
        }(i)
    }

    wg.Wait()

    // Verify all jobs were created
    w := plugintest.NewMockResponseWriter()
    plugin.GetJobs(w, "alice", nil, nil)

    plugintest.AssertJobCount(t, w, numJobs)

    // Verify all IDs are unique
    jobs := w.AllJobs()
    ids := make(map[string]bool)
    for _, job := range jobs {
        if ids[job.ID] {
            t.Errorf("Duplicate job ID: %s", job.ID)
        }
        ids[job.ID] = true
    }
}
```

### Testing context cancellation

```go
func TestGetJobOutput_ContextCancelled(t *testing.T) {
    plugin, cache := testPlugin(t)

    job := plugintest.NewJob().
        WithID("job-123").
        WithUser("alice").
        Running().
        Build()
    cache.AddOrUpdate(job)

    // Create cancellable context
    ctx, cancel := context.WithCancel(context.Background())

    w := plugintest.NewMockStreamResponseWriter()
    done := make(chan struct{})

    go func() {
        plugin.GetJobOutput(ctx, w, "alice", "job-123", api.OutputStdout)
        close(done)
    }()

    // Cancel immediately
    cancel()

    // Wait for completion
    select {
    case <-done:
        // Success - method returned
    case <-time.After(time.Second):
        t.Fatal("GetJobOutput did not respect context cancellation")
    }
}
```

## Summary

The SDK provides comprehensive testing utilities that make it easy to write thorough tests for your plugin. Key takeaways:

1. **Use MockResponseWriter** for capturing responses
2. **Use JobBuilder** for readable test data
3. **Use assertions** for clear error messages
4. **Test error cases** not just happy paths
5. **Keep tests fast** with short timeouts
6. **Use table-driven tests** for multiple scenarios
7. **Test concurrency** if your plugin uses goroutines
8. **Integration test** with real schedulers when possible

Good tests give you confidence that your plugin works correctly and make refactoring safe.

The `conformance` package is the recommended starting point for verifying your plugin against real product request sequences. See the [Conformance Testing](#conformance-testing) section at the top of this guide.

## See also

- [API Reference](API.md) - Complete API documentation (includes conformance package reference)
- [Developer Guide](GUIDE.md) - Learn how to build plugins
- [Examples](../examples/) - Example plugin implementations
