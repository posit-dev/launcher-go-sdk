# API Reference

Complete API documentation for the Launcher Go SDK.

## Table of contents

1. [Wire Protocol](#wire-protocol)
2. [Package: launcher](#package-launcher)
3. [Package: api](#package-api)
4. [Package: cache](#package-cache)
5. [Package: logger](#package-logger)
6. [Package: plugintest](#package-plugintest)
7. [Package: conformance](#package-conformance)

---

## Wire protocol

This section describes the underlying protocol between the Launcher and plugins. The SDK handles this automatically -- plugin developers don't interact with the wire protocol directly. We provide this information for debugging and deeper understanding.

### Message format

All messages are length-prefixed JSON:

```
[4-byte big-endian length][JSON payload]
```

### Common request fields

Every request from the Launcher includes:

| Field | Description |
|-------|-------------|
| `messageType` | Integer identifying the request type (e.g., 2 = submit job, 3 = get job). |
| `requestId` | Monotonically increasing ID for correlation with responses. |

Many requests also include:

| Field | Description |
|-------|-------------|
| `username` | The user who initiated the request. May be `*` for admin requests that apply to all users. |
| `requestUsername` | The actual username used when the request was submitted (for auditing). |

### Common response fields

Every response from the plugin includes:

| Field | Description |
|-------|-------------|
| `messageType` | Integer identifying the response type. |
| `requestId` | The ID of the request this response answers. |
| `responseId` | Monotonically increasing response ID (first must be 0). |

### Request types

| Type ID | Name | Description |
|---------|------|-------------|
| 0 | Heartbeat | Periodic health check. SDK handles automatically. |
| 1 | Bootstrap | Sent once at startup for version negotiation and initialization. |
| 2 | Submit Job | Submit a new job for execution. |
| 3 | Get Job / Get Jobs | Retrieve information about one or more jobs. |
| 4 | Get Job Status | Stream status updates for a job (or all jobs). |
| 5 | Control Job | Control a job (cancel, stop, kill, suspend, resume). |
| 6 | Get Job Output | Stream stdout/stderr output for a job. |
| 7 | Get Job Resource Util | Stream resource utilization metrics for a job. |
| 8 | Get Job Network | Get network information (hostname, IPs) for a job. |
| 9 | Get Cluster Info | Get cluster capabilities and configuration. |

### Stream responses

Streaming methods (status, output, resource utilization) send multiple response messages. Each includes a sequence ID to maintain ordering. The stream ends when the plugin sends a response with `complete: true` or when the Launcher sends a cancel request.

---

## Package: launcher

Import path: `github.com/posit-dev/launcher-go-sdk/launcher`

The launcher package provides the core runtime and plugin interfaces.

### Type: Plugin

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

All launcher plugins must implement this interface.

#### Method: SubmitJob

```go
SubmitJob(w ResponseWriter, user string, job *api.Job)
```

Called when a user submits a new job. The plugin should:
1. Assign a unique job ID
2. Set initial timestamps and status
3. Submit to the scheduler
4. Store in cache
5. Return the job via ResponseWriter

**Parameters**:
- `w` - ResponseWriter to send the result
- `user` - Username submitting the job
- `job` - Job specification to execute

#### Method: GetJob

```go
GetJob(w ResponseWriter, user string, id api.JobID, fields []string)
```

Returns information about a specific job.

**Parameters**:
- `w` - ResponseWriter to send the job
- `user` - Username requesting the job
- `id` - Job identifier
- `fields` - Optional list of fields to include (empty = all fields)

#### Method: GetJobs

```go
GetJobs(w ResponseWriter, user string, filter *api.JobFilter, fields []string)
```

Returns information about multiple jobs matching a filter.

**Parameters**:
- `w` - ResponseWriter to send jobs
- `user` - Username requesting jobs
- `filter` - Filter criteria (status, tags, time range)
- `fields` - Optional list of fields to include

#### Method: ControlJob

```go
ControlJob(w ResponseWriter, user string, id api.JobID, op api.JobOperation)
```

Performs a control operation on a job (stop, kill, cancel, suspend, resume).

**Parameters**:
- `w` - ResponseWriter to send the result
- `user` - Username performing the operation
- `id` - Job identifier
- `op` - Operation to perform (see [api.JobOperation](#type-joboperation))

#### Method: GetJobStatus

```go
GetJobStatus(ctx context.Context, w StreamResponseWriter, user string, id api.JobID)
```

Streams status updates for a specific job. Should send updates whenever the job's status changes.

**Parameters**:
- `ctx` - Context for cancellation
- `w` - StreamResponseWriter for sending updates
- `user` - Username requesting status
- `id` - Job identifier

#### Method: GetJobStatuses

```go
GetJobStatuses(ctx context.Context, w StreamResponseWriter, user string)
```

Streams status updates for all jobs belonging to the user.

**Parameters**:
- `ctx` - Context for cancellation
- `w` - StreamResponseWriter for sending updates
- `user` - Username requesting statuses

#### Method: GetJobOutput

```go
GetJobOutput(ctx context.Context, w StreamResponseWriter, user string, id api.JobID, outputType api.JobOutput)
```

Streams job output (stdout/stderr).

**Parameters**:
- `ctx` - Context for cancellation
- `w` - StreamResponseWriter for sending output
- `user` - Username requesting output
- `id` - Job identifier
- `outputType` - Type of output (stdout, stderr, or both)

#### Method: GetJobResourceUtil

```go
GetJobResourceUtil(ctx context.Context, w StreamResponseWriter, user string, id api.JobID)
```

Streams resource utilization data (CPU, memory) for a job.

**Parameters**:
- `ctx` - Context for cancellation
- `w` - StreamResponseWriter for sending data
- `user` - Username requesting data
- `id` - Job identifier

#### Method: GetJobNetwork

```go
GetJobNetwork(w ResponseWriter, user string, id api.JobID)
```

Returns network information for a job (hostname, IP addresses).

**Parameters**:
- `w` - ResponseWriter to send network info
- `user` - Username requesting info
- `id` - Job identifier

#### Method: ClusterInfo

```go
ClusterInfo(w ResponseWriter, user string)
```

Returns information about cluster capabilities (queues, resource limits, container support, etc.).

**Parameters**:
- `w` - ResponseWriter to send cluster info
- `user` - Username requesting info

### Type: ResponseWriter

```go
type ResponseWriter interface {
    WriteJobs(jobs []*api.Job)
    WriteError(err error)
    WriteErrorf(code int, format string, args ...interface{})
    WriteControlJob(success bool, message string)
    WriteJobNetwork(hostname string, ips []string)
    WriteClusterInfo(info ClusterOptions)
}
```

Used by plugin methods to send single responses back to the Launcher.

#### Method: WriteJobs

```go
WriteJobs(jobs []*api.Job)
```

Sends one or more jobs as the response.

#### Method: WriteError

```go
WriteError(err error)
```

Sends an error response. The error should be of type `*api.Error` with an error code.

#### Method: WriteErrorf

```go
WriteErrorf(code int, format string, args ...interface{})
```

Convenience method to create and send an error.

**Parameters**:
- `code` - Error code (see [Error Codes](#error-codes))
- `format` - Printf-style format string
- `args` - Format arguments

#### Method: WriteControlJob

```go
WriteControlJob(success bool, message string)
```

Sends the result of a control operation.

**Parameters**:
- `success` - Whether the operation succeeded
- `message` - Optional message (typically empty on success)

#### Method: WriteJobNetwork

```go
WriteJobNetwork(hostname string, ips []string)
```

Sends network information for a job.

#### Method: WriteClusterInfo

```go
WriteClusterInfo(info ClusterOptions)
```

Sends cluster capability information.

### Type: StreamResponseWriter

```go
type StreamResponseWriter interface {
    ResponseWriter
    WriteJobStatus(status *api.Job)
    WriteJobOutput(output string, outputType api.JobOutput)
    WriteJobResourceUtil(cpuPercent, cpuSeconds, residentMem, virtualMem float64)
    Close()
}
```

Extends ResponseWriter for methods that send multiple responses (streaming).

Always call `Close()` when done streaming.

#### Method: WriteJobStatus

```go
WriteJobStatus(status *api.Job)
```

Sends a job status update in a status stream.

#### Method: WriteJobOutput

```go
WriteJobOutput(output string, outputType api.JobOutput)
```

Sends a chunk of job output.

**Parameters**:
- `output` - Output text
- `outputType` - Type of output being sent

#### Method: WriteJobResourceUtil

```go
WriteJobResourceUtil(cpuPercent, cpuSeconds, residentMem, virtualMem float64)
```

Sends resource utilization data.

**Parameters**:
- `cpuPercent` - CPU usage percentage (0-100)
- `cpuSeconds` - Cumulative CPU time in seconds
- `residentMem` - Resident memory in MB
- `virtualMem` - Virtual memory in MB

#### Method: Close

```go
Close()
```

Closes the stream. Must be called when streaming is complete.

### Type: Runtime

```go
type Runtime struct {
    // contains filtered or unexported fields
}
```

The Runtime handles the request/response protocol and dispatches to plugin methods.

#### Function: NewRuntime

```go
func NewRuntime(lgr *slog.Logger, plugin Plugin) *Runtime
```

Creates a new Runtime instance.

**Parameters**:
- `lgr` - Structured logger
- `plugin` - Plugin implementation

**Returns**: Runtime instance

#### Method: Run

```go
func (r *Runtime) Run(ctx context.Context) error
```

Starts the plugin runtime. Blocks until context is cancelled or fatal error occurs.

**Parameters**:
- `ctx` - Context for cancellation

**Returns**: Error if runtime fails

### Type: DefaultOptions

```go
type DefaultOptions struct {
    Debug             bool
    JobExpiry         time.Duration
    HeartbeatInterval time.Duration
    LauncherConfig    string
    PluginName        string
    ScratchPath       string
    ServerUser        string
    Unprivileged      bool
    LoggingDir        string
    ConfigFile        string
}
```

Standard configuration options for launcher plugins.

**Fields**:
- `Debug` - Enable debug logging
- `JobExpiry` - Duration after which completed jobs are removed
- `HeartbeatInterval` - Expected heartbeat frequency from Launcher
- `LauncherConfig` - Path to Launcher configuration file
- `PluginName` - Name of this plugin instance
- `ScratchPath` - Directory for temporary files
- `ServerUser` - User the server runs as
- `Unprivileged` - Whether running without root privileges
- `LoggingDir` - Directory for log files
- `ConfigFile` - Path to plugin-specific config file

#### Method: AddFlags

```go
func (o *DefaultOptions) AddFlags(f *flag.FlagSet, pluginName string)
```

Adds standard flags to a flag set.

#### Method: Validate

```go
func (o *DefaultOptions) Validate() error
```

Validates the options.

#### Function: MustLoadOptions

```go
func MustLoadOptions(options OptionsLoader, pluginName string)
```

Loads options from command-line flags. Panics on error.

### Type: ClusterOptions

```go
type ClusterOptions struct {
    Queues       []string
    DefaultQueue string
    Limits       []api.ResourceLimit
    ImageOpt     ImageOptions
    Configs      []api.JobConfig
    Constraints  []api.PlacementConstraint
    Profiles     []api.ResourceProfile
}
```

Describes cluster capabilities returned by ClusterInfo. Posit products use this information to determine what UI to show end users when they launch jobs (e.g., queue selectors, resource limit sliders, container image dropdowns).

**Fields**:
- `Queues` - Available job queues, if the scheduler supports queues
- `DefaultQueue` - Default queue if none is specified
- `Limits` - Resource limit types that may be requested, including default and maximum values
- `ImageOpt` - Container image options (available images, default, whether unknown images are allowed)
- `Configs` - Custom configuration options beyond the built-in settings
- `Constraints` - Placement constraints that may be requested for a job (e.g., node type, availability zone)
- `Profiles` - Predefined resource profiles (named bundles of resource limits for easy selection)

### Type: ImageOptions

```go
type ImageOptions struct {
    Images       []string
    Default      string
    AllowUnknown bool
}
```

Container image configuration.

**Fields**:
- `Images` - List of available container images
- `Default` - Default image if not specified
- `AllowUnknown` - Whether to allow images not in the list

---

## Package: api

Import path: `github.com/posit-dev/launcher-go-sdk/api`

The api package contains all type definitions matching the Launcher Plugin API v3.5.

### Type: Job

```go
type Job struct {
    ID          string                  `json:"id"`
    Cluster     string                  `json:"cluster,omitempty"`
    Name        string                  `json:"name,omitempty"`
    User        string                  `json:"user,omitempty"`
    Group       string                  `json:"group,omitempty"`
    Queues      []string                `json:"queues,omitempty"`
    WorkDir     string                  `json:"workingDirectory,omitempty"`
    Container   *Container              `json:"container,omitempty"`
    Host        string                  `json:"host,omitempty"`
    Status      string                  `json:"status,omitempty"`
    StatusMsg   string                  `json:"statusMessage,omitempty"`
    StatusCode  string                  `json:"statusCode,omitempty"`
    Pid         *int                    `json:"pid,omitempty"`
    ExitCode    *int                    `json:"exitCode,omitempty"`
    Command     string                  `json:"command,omitempty"`
    Exe         string                  `json:"exe,omitempty"`
    Stdout      string                  `json:"stdoutFile,omitempty"`
    Stderr      string                  `json:"stderrFile,omitempty"`
    Stdin       string                  `json:"stdin,omitempty"`
    Args        []string                `json:"args,omitempty"`
    Env         []Env                   `json:"environment,omitempty"`
    Constraints []PlacementConstraint   `json:"placementConstraints,omitempty"`
    LastUpdated *time.Time              `json:"lastUpdateTime,omitempty"`
    Submitted   *time.Time              `json:"submissionTime,omitempty"`
    Ports       []Port                  `json:"exposedPorts,omitempty"`
    Mounts      []Mount                 `json:"mounts,omitempty"`
    Config      []JobConfig             `json:"config,omitempty"`
    Limits      []ResourceLimit         `json:"resourceLimits,omitempty"`
    Tags        []string                `json:"tags,omitempty"`
    Metadata    map[string]interface{}  `json:"metadata,omitempty"`
    Profile     string                  `json:"resourceProfile,omitempty"`
}
```

Represents a job in the launcher system.

**Key fields**:

| Field | Description |
|-------|-------------|
| `ID` | Unique identifier assigned by the plugin on submission. |
| `Name` | Human-readable name for the job. |
| `User` | The username of the user who launched the job. |
| `Group` | The group of the user who launched the job. |
| `Status` | Current status (see [Job Statuses](#job-statuses)). |
| `StatusMsg` | Optional message or reason for the current status. |
| `StatusCode` | Standard code/enum for the current status, if known. |
| `Command` | Shell command to execute. Mutually exclusive with `Exe`. |
| `Exe` | Executable path to run. Mutually exclusive with `Command`. |
| `Args` | Arguments for the command or executable. |
| `Stdin` | Standard input to pass to the process. |
| `Env` | Environment variables for the job (`[]Env` name/value pairs). |
| `Stdout` / `Stderr` | File locations for capturing output. |
| `WorkDir` | Working directory for the process. |
| `Host` | The host on which the job is (or was) running. |
| `Cluster` | The cluster the job belongs to. |
| `Queues` | The scheduler queues available or used for the job. |
| `Pid` | Process ID of the job, if applicable. |
| `ExitCode` | Exit code of the process (set when job completes). |
| `Submitted` / `LastUpdated` | Timestamps for job submission and last update. |
| `Limits` | Resource limits set for the job (`[]ResourceLimit`). |
| `Mounts` | Filesystem mounts to apply when running the job. |
| `Container` | Container configuration, if the cluster supports containers. |
| `Ports` | Exposed network ports, if containers are used. |
| `Tags` | Tags for filtering jobs. |
| `Config` | Custom configuration values (`[]JobConfig` name/value/type). |
| `Constraints` | Placement constraints selected for the job. |
| `Metadata` | User-specified metadata for extension attributes. |
| `Profile` | Resource profile for the job (default `"custom"`). |

### Type: JobID

```go
type JobID string
```

Unique identifier for a job.

### Type: JobFilter

```go
type JobFilter struct {
    Statuses  []string   `json:"statuses,omitempty"`
    Tags      []string   `json:"tags,omitempty"`
    StartTime *time.Time `json:"startTime,omitempty"`
    EndTime   *time.Time `json:"endTime,omitempty"`
}
```

Filter criteria for querying jobs.

**Fields**:
- `Statuses` - Only return jobs with these statuses
- `Tags` - Only return jobs with these tags
- `StartTime` - Only return jobs submitted after this time
- `EndTime` - Only return jobs submitted before this time

### Type: JobOperation

```go
type JobOperation int
```

Operation to perform on a job.

**Constants**:
```go
const (
    OperationSuspend JobOperation = iota
    OperationResume
    OperationStop
    OperationKill
    OperationCancel
)
```

| Operation | Valid When | Description |
|-----------|-----------|-------------|
| `cancel` | Pending | Cancel the job before it starts running. |
| `stop` | Running | Gracefully stop the running job (e.g., `SIGTERM`). |
| `kill` | Running | Forcibly kill the running job (e.g., `SIGKILL`). |
| `suspend` | Running | Pause execution; may be resumed later. |
| `resume` | Suspended | Resume a previously suspended job. |

The SDK validates that the job is in the correct state before invoking `ControlJob`. If the state is invalid, the SDK returns an error with `CodeInvalidJobState`.

#### Method: ValidForStatus

```go
func (op JobOperation) ValidForStatus() string
```

Returns the job status this operation is valid for.

- `cancel` → `StatusPending`
- `kill` → `StatusRunning`
- `stop` → `StatusRunning`
- `suspend` → `StatusRunning`
- `resume` → `StatusSuspended`

### Type: JobOutput

```go
type JobOutput string
```

Type of job output to retrieve.

**Constants**:
```go
const (
    OutputStdout JobOutput = "stdout"
    OutputStderr JobOutput = "stderr"
    OutputBoth   JobOutput = "both"
)
```

### Job statuses

```go
const (
    StatusPending   = "Pending"
    StatusRunning   = "Running"
    StatusSuspended = "Suspended"
    StatusFinished  = "Finished"
    StatusFailed    = "Failed"
    StatusKilled    = "Killed"
    StatusCanceled  = "Canceled"
)
```

| Status | Description |
|--------|-------------|
| `Pending` | The scheduler accepted the job but has not started running it yet. |
| `Running` | The job is currently executing. |
| `Suspended` | Execution paused; the job may resume later. |
| `Finished` | The job ran and finished executing. This includes jobs where the process exited with a non-zero exit code. |
| `Failed` | The scheduler could not launch the job due to an error. Does *not* refer to jobs that exited with a non-zero exit code. |
| `Killed` | A user or the system forcibly killed the job while running (i.e., the process received `SIGKILL`). |
| `Canceled` | The user canceled the job before it began to run. |

#### Function: TerminalStatus

```go
func TerminalStatus(status string) bool
```

Returns true if the status is terminal (job won't change again).

Terminal statuses: Finished, Failed, Killed, Canceled

### Error codes

```go
const (
    CodeUnknown             ErrCode = iota // 0
    CodeRequestNotSupported                // 1
    CodeInvalidRequest                     // 2
    CodeJobNotFound                        // 3
    CodePluginRestarted                    // 4
    CodeTimeout                            // 5
    CodeJobNotRunning                      // 6
    CodeJobOutputNotFound                  // 7
    CodeInvalidJobState                    // 8
    CodeJobControlFailure                  // 9
    CodeUnsupportedVersion                 // 10
)
```

Standard error codes for plugin responses:

| Code | Constant | When to Use |
|------|----------|-------------|
| 0 | `CodeUnknown` | The request failed for an undetermined reason. Used when the Plugin cannot determine an appropriate error code for the error. |
| 1 | `CodeRequestNotSupported` | The request is not supported by the Plugin. The runtime may also return this if the Launcher sends a request that is not understood by the SDK. |
| 2 | `CodeInvalidRequest` | The request is malformed. A Plugin may return this if it receives an unexpected message from the Launcher. Usually this is only used by the runtime. |
| 3 | `CodeJobNotFound` | The job does not exist in the scheduling system. The Plugin should return this if the user-specified job ID does not exist. |
| 4 | `CodePluginRestarted` | The request could not be completed because the Plugin had to restart. |
| 5 | `CodeTimeout` | The request timed out while waiting for a response from the job scheduling system. |
| 6 | `CodeJobNotRunning` | The job exists in the job scheduling system but is not in the running state. |
| 7 | `CodeJobOutputNotFound` | The job does not have output. |
| 8 | `CodeInvalidJobState` | The job has an invalid job state for the requested action. |
| 9 | `CodeJobControlFailure` | The job control action failed. |
| 10 | `CodeUnsupportedVersion` | The Launcher is using a Launcher Plugin API version that is not supported by the Plugin. Sent automatically by the runtime if appropriate. |

### Type: Error

```go
type Error struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}
```

Represents an error response.

#### Function: Errorf

```go
func Errorf(code int, format string, args ...interface{}) error
```

Creates a new Error with the given code and message.

#### Method: Error

```go
func (e *Error) Error() string
```

Implements the error interface.

### Type: Container

```go
type Container struct {
    Image      string   `json:"image,omitempty"`
    RunAsUser  *int     `json:"runAsUserId,omitempty"`
    RunAsGroup *int     `json:"runAsGroupId,omitempty"`
    SupGroups  []int    `json:"supplementalGroupIds,omitempty"`
}
```

Container configuration for containerized jobs.

### Type: Mount

```go
type Mount struct {
    Source      string `json:"source"`
    Destination string `json:"destination"`
    ReadOnly    bool   `json:"readOnly,omitempty"`
}
```

Volume mount specification. The Launcher Plugin API supports several mount types, including:

- **`host`** - Mount a path from the host machine
- **`nfs`** - Mount from an NFS server
- **`azureFile`** - Mount an Azure File share
- **`cephFs`** - Mount a Ceph filesystem
- **`glusterFs`** - Mount a GlusterFS volume
- **`passthrough`** - Plugin-defined mount type

The `Source` field contains mount source information whose format depends on the mount type. The plugin is responsible for interpreting the source and performing the mount on the scheduler.

### Type: Port

```go
type Port struct {
    Port       int    `json:"port"`
    TargetPort int    `json:"targetPort,omitempty"`
    Protocol   string `json:"protocol,omitempty"`
}
```

Network port mapping.

### Type: ResourceLimit

```go
type ResourceLimit struct {
    Type  string `json:"type"`
    Value string `json:"value,omitempty"`
    Max   string `json:"max,omitempty"`
    Min   string `json:"min,omitempty"`
}
```

Resource limit specification.

**Standard limit types**:

| Type | Description |
|------|-------------|
| `cpuCount` | Number of CPUs. |
| `cpuTime` | Maximum CPU time allowed. |
| `memory` | Memory allocation (e.g., `"8GB"`). |
| `memorySwap` | Swap memory limit. |

The `Value` field is the requested value, `Max` is the maximum allowed, and `Min` is the minimum allowed. These are used both in ClusterInfo responses (to declare limits) and in Job objects (to request resources).

### Type: ResourceProfile

```go
type ResourceProfile struct {
    Name        string          `json:"name"`
    DisplayName string          `json:"displayName"`
    Description string          `json:"description,omitempty"`
    Limits      []ResourceLimit `json:"limits,omitempty"`
}
```

Predefined resource profile.

### Type: PlacementConstraint

```go
type PlacementConstraint struct {
    Name  string `json:"name"`
    Value string `json:"value"`
}
```

Job placement constraint (e.g., node type, availability zone).

### Type: JobConfig

```go
type JobConfig struct {
    Name        string `json:"name"`
    Type        string `json:"type"`
    Default     string `json:"default,omitempty"`
    Description string `json:"description,omitempty"`
}
```

Custom configuration option definition. These are declared in ClusterInfo responses to let Posit products know what custom settings your plugin supports. When a job is submitted, the user's chosen values appear in `Job.Config`.

**Supported types**: `string`, `int`, `float`, `enum`

For `enum` type configs, the possible values should be documented in the `Description` field.

### Type: Node

```go
type Node struct {
    Host   string `json:"host"`
    IP     string `json:"ip"`
    Status string `json:"status"`
}
```

Cluster node information (for load balancer awareness).

#### Method: Online

```go
func (n *Node) Online() bool
```

Returns true if the node is online.

---

## Package: cache

Import path: `github.com/posit-dev/launcher-go-sdk/cache`

The cache package provides job storage with pub/sub for status updates.

### Type: JobCache

```go
type JobCache struct {
    // contains filtered or unexported fields
}
```

Thread-safe job storage with permission enforcement and pub/sub.

#### Function: NewJobCache

```go
func NewJobCache(ctx context.Context, lgr *slog.Logger) (*JobCache, error)
```

Creates a new in-memory job cache. The scheduler is the source of truth for job state; plugins should populate the cache during `Bootstrap()` and keep it in sync via periodic polling.

**Parameters**:
- `ctx` - Context for background operations
- `lgr` - Structured logger

**Returns**: JobCache instance and error

#### Method: AddOrUpdate

```go
func (c *JobCache) AddOrUpdate(job *api.Job) error
```

Adds a new job or updates an existing job.

**Parameters**:
- `job` - Job to add or update

**Returns**: Error if operation fails

#### Method: Update

```go
func (c *JobCache) Update(user, jobID string, fn func(*api.Job) *api.Job) error
```

Atomically updates a job using a callback function.

**Parameters**:
- `user` - Username (for permission check)
- `jobID` - Job identifier
- `fn` - Update function (receives job, returns modified job)

**Returns**: Error if job not found or permission denied

**Example**:
```go
cache.Update("alice", "job-123", func(job *api.Job) *api.Job {
    job.Status = api.StatusRunning
    return job
})
```

#### Method: Lookup

```go
func (c *JobCache) Lookup(user, jobID string, fn func(*api.Job)) error
```

Looks up a job and executes a callback with it.

**Parameters**:
- `user` - Username (for permission check)
- `jobID` - Job identifier
- `fn` - Callback function

**Returns**: Error if job not found or permission denied

#### Method: WriteJob

```go
func (c *JobCache) WriteJob(w launcher.ResponseWriter, user, jobID string)
```

Writes a single job to a ResponseWriter.

**Parameters**:
- `w` - ResponseWriter to send job
- `user` - Username (for permission check)
- `jobID` - Job identifier

Writes error if job not found or permission denied.

#### Method: WriteJobs

```go
func (c *JobCache) WriteJobs(w launcher.ResponseWriter, user string, filter *api.JobFilter)
```

Writes multiple jobs matching a filter to a ResponseWriter.

**Parameters**:
- `w` - ResponseWriter to send jobs
- `user` - Username (for permission check)
- `filter` - Filter criteria (nil = all jobs)

#### Method: StreamJobStatus

```go
func (c *JobCache) StreamJobStatus(ctx context.Context, w launcher.StreamResponseWriter, user, jobID string)
```

Streams status updates for a specific job.

Sends initial status immediately, then sends updates when status changes. Automatically closes when job reaches terminal state or context is cancelled.

**Parameters**:
- `ctx` - Context for cancellation
- `w` - StreamResponseWriter for sending updates
- `user` - Username (for permission check)
- `jobID` - Job identifier

#### Method: StreamJobStatuses

```go
func (c *JobCache) StreamJobStatuses(ctx context.Context, w launcher.StreamResponseWriter, user string)
```

Streams status updates for all jobs belonging to the user.

**Parameters**:
- `ctx` - Context for cancellation
- `w` - StreamResponseWriter for sending updates
- `user` - Username (for permission check)

---

## Package: logger

Import path: `github.com/posit-dev/launcher-go-sdk/logger`

The logger package provides Workbench-style structured logging.

#### Function: NewLogger

```go
func NewLogger(name string, debug bool, dir string) (*slog.Logger, error)
```

Creates a new structured logger.

**Parameters**:
- `name` - Plugin name (for log filenames)
- `debug` - Enable debug-level logging
- `dir` - Directory for log files (empty string = stderr only)

**Returns**: Logger instance and error

#### Function: MustNewLogger

```go
func MustNewLogger(name string, debug bool, dir string) *slog.Logger
```

Creates a new logger. Panics on error.

---

## Package: plugintest

Import path: `github.com/posit-dev/launcher-go-sdk/plugintest`

The plugintest package provides testing utilities for plugin development.

### Type: MockResponseWriter

```go
type MockResponseWriter struct {
    // contains filtered or unexported fields
}
```

Mock implementation of ResponseWriter that captures all responses.

#### Function: NewMockResponseWriter

```go
func NewMockResponseWriter() *MockResponseWriter
```

Creates a new MockResponseWriter.

#### Method: WriteJobs

```go
func (m *MockResponseWriter) WriteJobs(jobs []*api.Job)
```

Records jobs that were written.

#### Method: WriteError

```go
func (m *MockResponseWriter) WriteError(err error)
```

Records an error that was written.

#### Method: WriteErrorf

```go
func (m *MockResponseWriter) WriteErrorf(code int, format string, args ...interface{})
```

Records an error that was written.

#### Method: WriteControlJob

```go
func (m *MockResponseWriter) WriteControlJob(success bool, message string)
```

Records a control operation result.

#### Method: WriteJobNetwork

```go
func (m *MockResponseWriter) WriteJobNetwork(hostname string, ips []string)
```

Records network information.

#### Method: WriteClusterInfo

```go
func (m *MockResponseWriter) WriteClusterInfo(info launcher.ClusterOptions)
```

Records cluster information.

#### Method: HasError

```go
func (m *MockResponseWriter) HasError() bool
```

Returns true if any errors were written.

#### Method: LastError

```go
func (m *MockResponseWriter) LastError() *api.Error
```

Returns the most recent error, or nil if none.

#### Method: AllJobs

```go
func (m *MockResponseWriter) AllJobs() []*api.Job
```

Returns all jobs that were written.

#### Method: LastJobs

```go
func (m *MockResponseWriter) LastJobs() []*api.Job
```

Returns the most recent set of jobs that were written.

#### Method: Reset

```go
func (m *MockResponseWriter) Reset()
```

Clears all recorded data.

### Type: MockStreamResponseWriter

```go
type MockStreamResponseWriter struct {
    MockResponseWriter
    // contains filtered or unexported fields
}
```

Mock implementation of StreamResponseWriter.

#### Function: NewMockStreamResponseWriter

```go
func NewMockStreamResponseWriter() *MockStreamResponseWriter
```

Creates a new MockStreamResponseWriter.

#### Method: WriteJobStatus

```go
func (m *MockStreamResponseWriter) WriteJobStatus(job *api.Job)
```

Records a status update.

#### Method: WriteJobOutput

```go
func (m *MockStreamResponseWriter) WriteJobOutput(output string, outputType api.JobOutput)
```

Records output data.

#### Method: WriteJobResourceUtil

```go
func (m *MockStreamResponseWriter) WriteJobResourceUtil(cpu, cpuTime, resMem, virtMem float64)
```

Records resource utilization data.

#### Method: Close

```go
func (m *MockStreamResponseWriter) Close()
```

Marks the stream as closed.

#### Method: IsClosed

```go
func (m *MockStreamResponseWriter) IsClosed() bool
```

Returns true if Close() was called.

#### Method: AllStatuses

```go
func (m *MockStreamResponseWriter) AllStatuses() []*api.Job
```

Returns all status updates that were written.

#### Method: CombinedOutput

```go
func (m *MockStreamResponseWriter) CombinedOutput() string
```

Returns all output data concatenated.

#### Method: AllResourceUtil

```go
func (m *MockStreamResponseWriter) AllResourceUtil() []ResourceUtilData
```

Returns all resource utilization data points.

### Type: JobBuilder

```go
type JobBuilder struct {
    // contains filtered or unexported fields
}
```

Fluent builder for creating test jobs.

#### Function: NewJob

```go
func NewJob() *JobBuilder
```

Creates a new JobBuilder.

#### Selected methods

```go
func (b *JobBuilder) WithID(id string) *JobBuilder
func (b *JobBuilder) WithUser(user string) *JobBuilder
func (b *JobBuilder) WithName(name string) *JobBuilder
func (b *JobBuilder) WithCommand(cmd string) *JobBuilder
func (b *JobBuilder) WithExe(exe string) *JobBuilder
func (b *JobBuilder) WithArgs(args ...string) *JobBuilder
func (b *JobBuilder) WithStatus(status string) *JobBuilder
func (b *JobBuilder) WithQueue(queue string) *JobBuilder
func (b *JobBuilder) WithCPUCount(count int) *JobBuilder
func (b *JobBuilder) WithMemory(mem string) *JobBuilder
func (b *JobBuilder) WithEnv(key, value string) *JobBuilder
func (b *JobBuilder) WithTag(tag string) *JobBuilder
func (b *JobBuilder) WithMount(source, dest string, readOnly bool) *JobBuilder
func (b *JobBuilder) WithContainer(image string) *JobBuilder

// Status shortcuts
func (b *JobBuilder) Pending() *JobBuilder
func (b *JobBuilder) Running() *JobBuilder
func (b *JobBuilder) Finished() *JobBuilder
func (b *JobBuilder) Failed() *JobBuilder

func (b *JobBuilder) Build() *api.Job
```

**Example**:
```go
job := plugintest.NewJob().
    WithUser("alice").
    WithCommand("python train.py").
    WithMemory("8GB").
    Running().
    Build()
```

### Type: JobFilterBuilder

```go
type JobFilterBuilder struct {
    // contains filtered or unexported fields
}
```

Fluent builder for creating job filters.

#### Function: NewJobFilter

```go
func NewJobFilter() *JobFilterBuilder
```

Creates a new JobFilterBuilder.

#### Methods

```go
func (b *JobFilterBuilder) WithStatus(status string) *JobFilterBuilder
func (b *JobFilterBuilder) WithTag(tag string) *JobFilterBuilder
func (b *JobFilterBuilder) WithStartTime(t time.Time) *JobFilterBuilder
func (b *JobFilterBuilder) WithEndTime(t time.Time) *JobFilterBuilder
func (b *JobFilterBuilder) Build() *api.JobFilter
```

### Type: ClusterOptionsBuilder

```go
type ClusterOptionsBuilder struct {
    // contains filtered or unexported fields
}
```

Fluent builder for creating cluster options.

#### Function: NewClusterOptions

```go
func NewClusterOptions() *ClusterOptionsBuilder
```

Creates a new ClusterOptionsBuilder.

#### Methods

```go
func (b *ClusterOptionsBuilder) WithQueue(name string) *ClusterOptionsBuilder
func (b *ClusterOptionsBuilder) WithDefaultQueue(name string) *ClusterOptionsBuilder
func (b *ClusterOptionsBuilder) WithLimit(limitType, max string) *ClusterOptionsBuilder
func (b *ClusterOptionsBuilder) WithImage(image string) *ClusterOptionsBuilder
func (b *ClusterOptionsBuilder) WithDefaultImage(image string) *ClusterOptionsBuilder
func (b *ClusterOptionsBuilder) Build() launcher.ClusterOptions
```

### Assertion helpers

#### Function: AssertNoError

```go
func AssertNoError(t *testing.T, w *MockResponseWriter)
```

Asserts that no errors were written.

#### Function: AssertErrorCode

```go
func AssertErrorCode(t *testing.T, w *MockResponseWriter, expectedCode int)
```

Asserts that an error with the specified code was written.

#### Function: AssertJobCount

```go
func AssertJobCount(t *testing.T, w *MockResponseWriter, expected int)
```

Asserts the number of jobs written.

#### Function: AssertJobStatus

```go
func AssertJobStatus(t *testing.T, job *api.Job, expected string)
```

Asserts a job's status.

#### Function: AssertJobUser

```go
func AssertJobUser(t *testing.T, job *api.Job, expected string)
```

Asserts a job's user.

#### Function: AssertStreamClosed

```go
func AssertStreamClosed(t *testing.T, w *MockStreamResponseWriter)
```

Asserts that the stream was closed.

#### Function: AssertMinimumStatuses

```go
func AssertMinimumStatuses(t *testing.T, w *MockStreamResponseWriter, minCount int)
```

Asserts at least N status updates were sent.

### Helper functions

#### Function: FindJobByID

```go
func FindJobByID(jobs []*api.Job, id string) *api.Job
```

Finds a job by ID in a slice of jobs.

#### Function: FindJobsByStatus

```go
func FindJobsByStatus(jobs []*api.Job, status string) []*api.Job
```

Filters jobs by status.

#### Function: FindJobsByUser

```go
func FindJobsByUser(jobs []*api.Job, user string) []*api.Job
```

Filters jobs by user.

---

## Package: conformance

Import path: `github.com/posit-dev/launcher-go-sdk/conformance`

The conformance package provides automated behavioral tests that verify a plugin implementation conforms to the contracts expected by Posit products (Workbench, Connect).

### Type: Profile

```go
type Profile struct {
    JobFactory         func(user string) *api.Job  // Returns a fresh, submittable job
    LongRunningJob     func(user string) *api.Job  // Returns a long-running job for control tests
    StopStatus         string                      // Terminal status after Stop
    StopExitCodes      []int                       // Acceptable exit codes after Stop
    KillStatus         string                      // Terminal status after Kill
    KillExitCodes      []int                       // Acceptable exit codes after Kill
    OutputAvailable    bool                        // Whether GetJobOutput returns data
    SuspendSupported   bool                        // Whether Suspend/Resume is supported
    NetworkAvailable   bool                        // Whether GetJobNetwork returns data (default true)
    PollInterval       time.Duration               // Polling interval (default 50ms)
    JobStartTimeout    time.Duration               // Wait for Running (default 30s)
    JobCompleteTimeout time.Duration               // Wait for terminal (default 60s)
    OutputTimeout      time.Duration               // Wait for output (default 10s)
}
```

Describes the behavioral expectations for a plugin. Required fields: `JobFactory`, `LongRunningJob`, `StopStatus`, `StopExitCodes`, `KillStatus`, `KillExitCodes`. Zero-valued timeout fields use defaults.

### Function: Run

```go
func Run(t *testing.T, p launcher.Plugin, user string, profile Profile)
```

Executes universal invariant tests that hold for all correct plugins. Tests are registered as subtests under `Invariants/` (e.g., `Invariants/Submit/ReturnsNonEmptyID`).

### Function: RunWorkflows

```go
func RunWorkflows(t *testing.T, p launcher.Plugin, user string, profile Profile)
```

Executes product workflow tests that replay the request sequences Posit products produce. Tests are registered under `Workflows/` (e.g., `Workflows/Launch`, `Workflows/Stop`).

### Scenario functions

Individual parameterized scenarios for isolated testing:

```go
func RunStopJob(t *testing.T, p launcher.Plugin, user string, opts StopOpts)
func RunKillJob(t *testing.T, p launcher.Plugin, user string, opts KillOpts)
func RunCancelJob(t *testing.T, p launcher.Plugin, user string, opts CancelOpts)
func RunSuspendResume(t *testing.T, p launcher.Plugin, user string, opts SuspendResumeOpts)
func RunOutputStream(t *testing.T, p launcher.Plugin, user string, opts OutputStreamOpts)
func RunStatusStream(t *testing.T, p launcher.Plugin, user string, opts StatusStreamOpts)
func RunStreamCancellation(t *testing.T, p launcher.Plugin, user string, opts StreamCancelOpts)
func RunFieldFiltering(t *testing.T, p launcher.Plugin, user string, opts FieldFilterOpts)
func RunControlInvalidState(t *testing.T, p launcher.Plugin, user string, opts InvalidStateOpts)
```

### Option structs

Each scenario function accepts an options struct:

| Struct | Key Fields |
|--------|------------|
| `StopOpts` | `Job`, `ExpectStatus`, `ExpectExitCodes`, `Timeout` |
| `KillOpts` | `Job`, `ExpectStatus`, `ExpectExitCodes`, `Timeout` |
| `CancelOpts` | `Job`, `Timeout` |
| `SuspendResumeOpts` | `Job`, `Timeout` |
| `OutputStreamOpts` | `Job`, `OutputType`, `ExpectNonEmpty`, `Timeout` |
| `StatusStreamOpts` | `Job`, `Timeout` |
| `StreamCancelOpts` | `Job`, `Timeout` |
| `FieldFilterOpts` | `Job`, `Fields` |
| `InvalidStateOpts` | `Job`, `Operation`, `Timeout` |

### Helper functions

Exported helpers for use in custom tests:

#### Function: SubmitJob

```go
func SubmitJob(t *testing.T, p launcher.Plugin, user string, job *api.Job) string
```

Calls `p.SubmitJob` and returns the job ID. Fails the test if the plugin returns an error.

#### Function: GetJob

```go
func GetJob(p launcher.Plugin, user string, id string, fields []string) (*api.Job, *api.Error)
```

Calls `p.GetJob` and returns the job or error.

#### Function: GetJobs

```go
func GetJobs(p launcher.Plugin, user string, filter *api.JobFilter) []*api.Job
```

Calls `p.GetJobs` with the given filter and returns matching jobs.

#### Function: ControlJob

```go
func ControlJob(p launcher.Plugin, user string, id string, op api.JobOperation) (*plugintest.ControlResult, *api.Error)
```

Calls `p.ControlJob` and returns the result or error.

#### Function: WaitForStatus

```go
func WaitForStatus(ctx context.Context, p launcher.Plugin, user, id, status string) (*api.Job, error)
```

Polls `p.GetJob` until the job reaches the expected status or the context expires.

#### Function: WaitForTerminalStatus

```go
func WaitForTerminalStatus(ctx context.Context, p launcher.Plugin, user, id string) (*api.Job, error)
```

Polls `p.GetJob` until the job reaches any terminal status or the context expires.

#### Function: CollectStatusStream

```go
func CollectStatusStream(ctx context.Context, p launcher.Plugin, user string) (*plugintest.MockStreamResponseWriter, <-chan struct{})
```

Starts a `GetJobStatuses` stream in a background goroutine. Returns the mock writer and a done channel. Cancel ctx to stop the stream.

#### Function: CollectOutputStream

```go
func CollectOutputStream(ctx context.Context, p launcher.Plugin, user, id string, outputType api.JobOutput) (*plugintest.MockStreamResponseWriter, <-chan struct{})
```

Starts a `GetJobOutput` stream in a background goroutine. Same lifecycle contract as `CollectStatusStream`.

---

## See also

- [Developer Guide](GUIDE.md) - Learn how to build plugins
- [Testing Guide](TESTING.md) - Comprehensive testing strategies
- [Architecture](ARCHITECTURE.md) - Design decisions and patterns
- [Examples](../examples/) - Complete example plugins
