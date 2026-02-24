# Scheduler Plugin Template

This example provides a template for integrating an external scheduler with the Posit Workbench Launcher. It demonstrates how to structure a plugin that delegates job management to a backend you don't control — whether that backend exposes a CLI, REST API, Go SDK, gRPC service, or something else.

For a working reference that implements every plugin method end-to-end, see the [inmemory example](../inmemory/).

## Architecture

Your plugin sits between the Launcher protocol (JSON over stdio) and your scheduler's native interface:

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

## The SchedulerClient interface

The central idea in this template is a `SchedulerClient` interface that isolates all scheduler-specific logic from the plugin protocol handling:

```go
type SchedulerClient interface {
    SubmitJob(ctx context.Context, job *api.Job) (string, error)
    GetJobStatus(ctx context.Context, schedulerID string) (string, error)
    CancelJob(ctx context.Context, schedulerID string) error
    KillJob(ctx context.Context, schedulerID string) error
    GetJobOutput(ctx context.Context, schedulerID string) (stdout, stderr string, err error)
}
```

The plugin methods (`SubmitJob`, `ControlJob`, etc.) call through this interface without knowing how the scheduler is reached. Your implementation of `SchedulerClient` is where the backend-specific code lives.

### CLI backend (Slurm, PBS, LSF, SGE)

```go
type SlurmClient struct{}

func (c *SlurmClient) SubmitJob(ctx context.Context, job *api.Job) (string, error) {
    cmd := exec.CommandContext(ctx, "sbatch",
        "--job-name="+job.Name,
        "--partition="+job.Queues[0],
        scriptPath)
    output, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("sbatch failed: %w", err)
    }
    // Parse "Submitted batch job 12345"
    return parseJobID(string(output)), nil
}

func (c *SlurmClient) CancelJob(ctx context.Context, schedulerID string) error {
    return exec.CommandContext(ctx, "scancel", schedulerID).Run()
}
```

### REST API backend

```go
type NomadClient struct {
    baseURL    string
    httpClient *http.Client
    token      string
}

func (c *NomadClient) SubmitJob(ctx context.Context, job *api.Job) (string, error) {
    body, _ := json.Marshal(toNomadJob(job))
    req, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/jobs", bytes.NewReader(body))
    req.Header.Set("X-Nomad-Token", c.token)

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("nomad submit: %w", err)
    }
    defer resp.Body.Close()

    var result struct{ EvalID string }
    json.NewDecoder(resp.Body).Decode(&result)
    return result.EvalID, nil
}
```

### Go SDK backend

```go
type CustomClient struct {
    sdk *scheduler.Client
}

func (c *CustomClient) SubmitJob(ctx context.Context, job *api.Job) (string, error) {
    result, err := c.sdk.Submit(ctx, &scheduler.JobSpec{
        Name:    job.Name,
        Command: job.Command,
    })
    if err != nil {
        return "", err
    }
    return result.ID, nil
}
```

The plugin code (`SchedulerPlugin`) doesn't change between these — only the `SchedulerClient` implementation does.

## Status updates

Most external schedulers don't push status changes to your plugin. The template includes a polling loop that periodically queries the scheduler and updates the cache:

```go
func (p *SchedulerPlugin) pollJobStatuses() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-p.ctx.Done():
            return
        case <-ticker.C:
            // Query scheduler for active jobs, update cache
        }
    }
}
```

If your scheduler supports push notifications (webhooks, event streams, watch APIs), you can replace the polling loop with a listener that updates the cache as events arrive.

## Status mapping

Every scheduler has its own vocabulary for job states. You need a function that maps those to Launcher statuses:

```go
func mapSchedulerStatus(native string) string {
    switch native {
    case "PENDING", "QUEUED":
        return api.StatusPending
    case "RUNNING", "ACTIVE":
        return api.StatusRunning
    case "COMPLETED", "SUCCEEDED":
        return api.StatusFinished
    case "FAILED", "ERROR":
        return api.StatusFailed
    case "CANCELLED", "CANCELED":
        return api.StatusCanceled
    default:
        return api.StatusPending
    }
}
```

## Implementation checklist

### Required

- [ ] **Implement SchedulerClient**
  - [ ] `SubmitJob` — submit a job and return the scheduler's ID for it
  - [ ] `GetJobStatus` — query current status by scheduler ID
  - [ ] `CancelJob` — cancel a pending job
  - [ ] `KillJob` — kill a running job
  - [ ] `GetJobOutput` — retrieve stdout/stderr

- [ ] **Map between scheduler and Launcher concepts**
  - [ ] Scheduler job states to Launcher statuses
  - [ ] Launcher control operations to scheduler actions
  - [ ] Scheduler queues/partitions to Launcher queues

- [ ] **Handle errors**
  - [ ] Connection/communication failures
  - [ ] Job not found
  - [ ] Permission denied
  - [ ] Scheduler-specific error codes

### Optional

- [ ] **Resource utilization** — stream CPU/memory metrics if your scheduler exposes them
- [ ] **Push-based status** — replace polling with webhooks or event streams
- [ ] **Batch status queries** — query all job statuses in a single call for efficiency
- [ ] **Advanced features** — job arrays, dependencies, priorities, checkpoint/restart

## Testing

### Conformance tests

Use the `conformance` package to verify your plugin against the behavioral contracts Posit products expect:

```go
func TestConformance(t *testing.T) {
    plugin := setupPlugin()  // Your assembled plugin

    profile := conformance.Profile{
        JobFactory:      myJobFactory,
        LongRunningJob:  myLongRunningJob,
        StopStatus:      api.StatusKilled,
        StopExitCodes:   []int{143},
        KillStatus:      api.StatusKilled,
        KillExitCodes:   []int{137, 143},
        OutputAvailable: true,
    }

    conformance.Run(t, plugin, "testuser", profile)
    conformance.RunWorkflows(t, plugin, "testuser", profile)
}
```

### SchedulerClient unit tests

Test your scheduler client separately from the plugin:

```go
func TestSubmit(t *testing.T) {
    client := &MySchedulerClient{}
    job := plugintest.NewJob().
        WithCommand("echo hello").
        WithQueue("batch").
        Build()

    jobID, err := client.SubmitJob(context.Background(), job)
    if err != nil {
        t.Fatalf("Submit failed: %v", err)
    }
    if jobID == "" {
        t.Error("Expected job ID")
    }

    client.CancelJob(context.Background(), jobID)
}
```

## Deployment

1. Build the plugin:
   ```bash
   go build -o rstudio-myplugin-launcher
   ```

2. Install:
   ```bash
   sudo cp rstudio-myplugin-launcher /usr/lib/rstudio-server/bin/
   ```

3. Configure Posit Workbench (`/etc/rstudio/launcher.conf`):
   ```ini
   [cluster]
   name=myplugin
   type=Plugin
   exe=/usr/lib/rstudio-server/bin/rstudio-myplugin-launcher
   ```

## Troubleshooting

**Jobs stuck in Pending**: Check scheduler logs, verify user permissions, check resource availability.

**Status not updating**: Verify the polling loop is running, check that your `GetJobStatus` implementation returns correct values, confirm the scheduler ID mapping is correct.

**Output not available**: Confirm the job has completed, verify your `GetJobOutput` implementation can reach the output (file paths, API permissions, etc.).
