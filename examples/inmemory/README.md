# In-Memory Launcher Plugin Example

This example demonstrates a complete launcher plugin implementation with job lifecycle simulation. It shows all the key patterns and best practices for building a real plugin.

## Features

This plugin demonstrates:

- **Job submission** with unique ID generation
- **Job lifecycle simulation** (pending → running → finished)
- **Background goroutines** for asynchronous job status updates
- **Job control operations** (stop, kill, cancel)
- **Streaming** job output and status updates
- **Resource utilization** reporting
- **Job filtering** by status, tags, and time range
- **Error handling** and validation
- **Cluster capabilities** reporting

## Building

```bash
cd examples/inmemory
go build
```

## Running

```bash
./inmemory --enable-debug-logging
```

The plugin will start and wait for requests from the Launcher. All communication happens over stdin/stdout using a JSON-based protocol.

## How it works

### Job lifecycle

When a job is submitted:

1. **Immediate**: Job is created with status `Pending`
2. **After 500ms**: Job transitions to `Running`
3. **After 5s total**: Job transitions to `Finished` with exit code 0

Jobs tagged `"long-running"` skip step 3 and stay in `Running` indefinitely until explicitly stopped, killed, or canceled. This models interactive sessions (e.g., RStudio Pro, Jupyter) that run until the user ends them.

This timing is intentionally short to make testing easier. In a real plugin, jobs would run for as long as needed.

### Job cache

The plugin uses the SDK's `JobCache` to store job information. The cache:

- Stores jobs in memory (or optionally on disk)
- Handles permission checking (users can only see their own jobs)
- Supports pub/sub for status updates
- Provides helper methods for writing jobs to ResponseWriters

### Streaming

Several methods use `StreamResponseWriter` to send multiple updates:

- **GetJobStatus**: Sends status updates as the job progresses
- **GetJobStatuses**: Sends status for all jobs
- **GetJobOutput**: Sends chunks of stdout/stderr
- **GetJobResourceUtil**: Periodically sends CPU/memory usage

Streaming methods:
- Run in a separate goroutine (the ResponseWriter is thread-safe)
- Always call `Close()` when done
- Respect context cancellation

### Control operations

The plugin supports these control operations:

- **Cancel**: Cancel a pending job (before it starts)
- **Stop**: Gracefully stop a running job (SIGTERM, exit code 143)
- **Kill**: Forcefully kill a running job (SIGKILL, exit code 137)
- **Suspend**: Not supported in this example
- **Resume**: Not supported in this example

Each operation validates that the job is in the correct state before proceeding.

## Code structure

```
main.go
├── InMemoryPlugin          Plugin struct with job cache
├── SubmitJob               Accept new job, start lifecycle simulation
├── simulateJobLifecycle    Background goroutine for status updates
├── GetJob                  Return single job info
├── GetJobs                 Return multiple jobs with filtering
├── ControlJob              Stop/kill/cancel jobs
├── GetJobStatus            Stream status for one job
├── GetJobStatuses          Stream status for all jobs
├── GetJobOutput            Stream stdout/stderr
├── GetJobResourceUtil      Stream CPU/memory usage
├── GetJobNetwork           Return network info
├── ClusterInfo             Return cluster capabilities
└── main                    Setup and run the plugin

conformance_test.go
├── newTestPlugin           Create plugin instance for testing
├── testProfile             Conformance profile for this plugin
├── TestRun                 Tier 1: Universal invariants
├── TestRunWorkflows        Tier 2: Product workflows
└── TestRun*                Tier 3: Individual scenario tests
```

## Key patterns

### Unique ID generation

```go
id := fmt.Sprintf("job-%d", atomic.AddInt32(&p.nextID, 1))
```

Uses an atomic counter for thread-safe ID generation.

### Background job updates

```go
p.wg.Add(1)
go func() {
    defer p.wg.Done()
    time.Sleep(500 * time.Millisecond)
    p.cache.Update(user, id, func(job *api.Job) *api.Job {
        job.Status = api.StatusRunning
        return job
    })
}()
```

Updates happen asynchronously in a goroutine. The `WaitGroup` tracks active goroutines for clean shutdown.

### Streaming with Close

```go
go func() {
    defer w.Close()  // Always close when done
    for i := 0; i < 3; i++ {
        w.WriteJobOutput(output, outputType)
    }
}()
```

### Context cancellation

```go
select {
case <-ctx.Done():
    return  // Client cancelled
case <-ticker.C:
    // Continue processing
}
```

Always check if the context was cancelled.

## Testing

### Conformance tests

This plugin includes a full conformance test suite in `conformance_test.go` that verifies it against the behavioral contracts Posit products expect. The suite covers three tiers:

- **Tier 1 — Invariants** (`TestRun`): Fundamental protocol requirements (submit, get, filter, cluster info, lifecycle, errors)
- **Tier 2 — Workflows** (`TestRunWorkflows`): Real product request sequences (launch, stop, kill, cancel, list)
- **Tier 3 — Scenarios** (`TestRunStopJob`, `TestRunKillJob`, etc.): Individual operations tested in isolation

Run the tests:

```bash
# All conformance tests
go test -v ./examples/inmemory/

# Individual tiers
go test -v ./examples/inmemory/ -run TestRun$
go test -v ./examples/inmemory/ -run TestRunWorkflows
go test -v ./examples/inmemory/ -run TestRunStopJob

# With race detector
go test -race ./examples/inmemory/
```

The key to conformance is the `Profile`, which describes the plugin's expected behavior:

```go
conformance.Profile{
    JobFactory:     func(user string) *api.Job { /* short-lived job */ },
    LongRunningJob: func(user string) *api.Job { /* job that stays Running */ },
    StopStatus:     api.StatusFinished,
    StopExitCodes:  []int{143},   // 128 + SIGTERM
    KillStatus:     api.StatusKilled,
    KillExitCodes:  []int{137},   // 128 + SIGKILL
    OutputAvailable: true,
}
```

See `conformance_test.go` for the complete setup.

### Unit tests

You can also test individual methods using the SDK's testing utilities:

```go
import (
    "testing"
    "github.com/posit-dev/launcher-go-sdk/plugintest"
)

func TestSubmitJob(t *testing.T) {
    // Create plugin
    ctx := context.Background()
    lgr := logger.MustNewLogger("test", false, "")
    cache, _ := cache.NewJobCache(ctx, lgr, "")
    plugin := &InMemoryPlugin{cache: cache}

    // Create mock writer
    w := plugintest.NewMockResponseWriter()

    // Submit a job
    job := plugintest.NewJob().
        WithUser("alice").
        WithCommand("echo hello").
        Build()

    plugin.SubmitJob(w, "alice", job)

    // Verify the response
    plugintest.AssertNoError(t, w)
    plugintest.AssertJobCount(t, w, 1)

    returnedJob := w.LastJobs()[0]
    plugintest.AssertJobStatus(t, returnedJob, api.StatusPending)
    plugintest.AssertJobUser(t, returnedJob, "alice")

    // Wait for job to transition to running
    time.Sleep(600 * time.Millisecond)

    // Check status changed
    w2 := plugintest.NewMockResponseWriter()
    plugin.GetJob(w2, "alice", api.JobID(returnedJob.ID), nil)
    updatedJob := w2.LastJobs()[0]
    plugintest.AssertJobStatus(t, updatedJob, api.StatusRunning)
}
```

## Adapting for your scheduler

To adapt this plugin for a real scheduler:

1. **Replace simulation with real execution**
   - Instead of `time.Sleep()`, actually start the job
   - Use `exec.Command()` for local processes
   - Call your scheduler's API for cluster jobs

2. **Monitor actual job status**
   - Poll your scheduler's API or job files
   - Parse status from command output
   - Update the cache when status changes

3. **Capture real output**
   - Read from stdout/stderr files
   - Tail log files
   - Stream from your scheduler's logs

4. **Implement real control operations**
   - Send signals to processes (SIGTERM, SIGKILL)
   - Call your scheduler's cancel/kill commands
   - Update job status after control operations

5. **Report accurate resource usage**
   - Query /proc filesystem for local jobs
   - Call your scheduler's resource API
   - Parse output from monitoring commands

See `examples/scheduler/README.md` for a design guide covering these patterns.

## Next steps

- See `examples/scheduler/README.md` for a design guide for CLI-based schedulers
- Read the SDK documentation for more details on each method
- Write tests using the `plugintest` package
