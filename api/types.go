// Package api defines types for the Launcher Plugin API.
package api

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"path"
	"strconv"
	"strings"
	"time"
)

// ErrCode represents a Launcher Plugin error code.
type ErrCode int

const (
	// CodeUnknown indicates the request failed for an undetermined reason.
	// Used when the Plugin cannot determine an appropriate error code for
	// the error.
	CodeUnknown ErrCode = iota

	// CodeRequestNotSupported indicates the request is not supported by the
	// Plugin. The runtime may also return this if the Launcher sends a
	// request that is not understood by this package.
	CodeRequestNotSupported

	// CodeInvalidRequest indicates the request is malformed. A Plugin may
	// return this if it receives an unexpected message from the Launcher.
	// Usually this is only used by the runtime.
	CodeInvalidRequest

	// CodeJobNotFound indicates the job does not exist in the scheduling
	// system. The Plugin should return this if the user-specified job ID
	// does not exist.
	CodeJobNotFound

	// CodePluginRestarted indicates the request could not be completed
	// because the Plugin had to restart.
	CodePluginRestarted

	// CodeTimeout indicates the request timed out while waiting for a
	// response from the job scheduling system.
	CodeTimeout

	// CodeJobNotRunning indicates the job exists in the job scheduling
	// system but is not in the running state.
	CodeJobNotRunning

	// CodeJobOutputNotFound indicates the job does not have output.
	CodeJobOutputNotFound

	// CodeInvalidJobState indicates the job has an invalid job state for
	// the requested action.
	CodeInvalidJobState

	// CodeJobControlFailure indicates the job control action failed.
	CodeJobControlFailure

	// CodeUnsupportedVersion indicates the Launcher is using a Launcher
	// Plugin API version that is not supported by the Plugin. This is sent
	// automatically by the runtime if appropriate.
	CodeUnsupportedVersion
)

func (e ErrCode) String() string {
	switch e {
	case CodeUnknown:
		return "an error occurred"
	case CodeRequestNotSupported:
		return "not supported"
	case CodeInvalidRequest:
		return "invalid request"
	case CodeJobNotFound:
		return "job not found"
	case CodePluginRestarted:
		return "plugin is restarting"
	case CodeTimeout:
		return "timeout"
	case CodeJobNotRunning:
		return "job not running"
	case CodeJobOutputNotFound:
		return "job output not found"
	case CodeInvalidJobState:
		return "invalid job state"
	case CodeJobControlFailure:
		return "job control failure"
	case CodeUnsupportedVersion:
		return "unsupported version"
	}
	return "an error occurred"
}

// Error is a structured error representation for Launcher plugins.
type Error struct {
	Code ErrCode `json:"code"`
	Msg  string  `json:"message"`
}

// Errorf creates an error with the corresponding API code.
func Errorf(code ErrCode, format string, a ...interface{}) *Error {
	return &Error{code, fmt.Sprintf(format, a...)}
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return e.Code.String()
}

// Is makes comparisons with errors.Is() possible.
func (e *Error) Is(err error) bool {
	apiErr, ok := err.(*Error)
	if !ok {
		return false
	}
	// Only compare messages if we have both of them. This allows checking
	// for generic error codes.
	if apiErr.Msg != "" && e.Msg != "" && apiErr.Msg != e.Msg {
		return false
	}
	return apiErr.Code == e.Code
}

// Common API errors.
var (
	ErrJobNotRunning   = &Error{Code: CodeJobNotRunning}
	ErrJobNotFound     = &Error{Code: CodeJobNotFound}
	ErrInvalidJobState = &Error{Code: CodeInvalidJobState}
)

// The available statuses (or states) of a Job.
const (
	// The job was canceled by the user before it began to run.
	StatusCanceled = "Canceled"

	// The job could not be launched due to an error. This status does not
	// refer to jobs where the process exited with a non-zero exit code.
	StatusFailed = "Failed"

	// The job was launched and finished executing. This includes jobs where
	// the process exited with a non-zero exit code.
	StatusFinished = "Finished"

	// The job was forcibly killed while it was running, i.e. the job
	// process received SIGKILL.
	StatusKilled = "Killed"

	// The job was successfully submitted to the job scheduling system but
	// has not started running yet.
	StatusPending = "Pending"

	// The job is currently running.
	StatusRunning = "Running"

	// The job was running, but execution was paused and may be resumed at a
	// later time.
	StatusSuspended = "Suspended"
)

// TerminalStatus returns true when the passed status is "terminal" -- i.e. the job's status
// will not change in the future.
func TerminalStatus(status string) bool {
	switch status {
	case StatusCanceled, StatusFailed, StatusFinished, StatusKilled:
		return true
	}
	return false
}

// JobFilter describes a set of conditions (all optional) that must be met by
// a job to be included in a response.
type JobFilter struct {
	// The set of tags that a job must have to be included.
	Tags []string `json:"tags,omitempty"`

	// If non-nil, the minimum submission time that a job must have to be
	// included.
	StartTime *time.Time `json:"startTime,omitempty"`

	// If non-nil, the maximum submission time that a job must have to be
	// included.
	EndTime *time.Time `json:"endTime,omitempty"`

	// If non-empty, a job must have one of these statuses to be included.
	Statuses []string `json:"statuses,omitempty"`

	// Narrow the list of returned fields.
	Fields []string `json:"fields,omitempty"`
}

// Includes returns true if a Job meets all conditions for the filter, if any.
func (f *JobFilter) Includes(job *Job) bool {
	if f.StartTime != nil && job.Submitted.Before(*f.StartTime) {
		return false
	}
	if f.EndTime != nil && job.Submitted.After(*f.EndTime) {
		return false
	}
	match := !(len(f.Statuses) > 0)
	for _, status := range f.Statuses {
		if job.Status == status {
			match = true
			break
		}
	}
	if !match {
		return false
	}
	for _, tag := range f.Tags {
		match = false
		for _, jTag := range job.Tags {
			if tag == jTag {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

// JobOutput represents the type of output stream (stdout, stderr, or both).
type JobOutput int

// JobOutput constants for output stream types.
const (
	// OutputStdout represents the standard output stream.
	OutputStdout JobOutput = iota

	// OutputStderr represents the standard error stream.
	OutputStderr

	// OutputBoth represents both stdout and stderr streams combined.
	OutputBoth
)

func (o JobOutput) String() string {
	switch o {
	case OutputStdout:
		return "stdout"
	case OutputStderr:
		return "stderr"
	case OutputBoth:
		return "mixed"
	}
	return "mixed"
}

// MarshalText implements `encoding.TextMarshaler`.
func (o *JobOutput) MarshalText() ([]byte, error) {
	return []byte(o.String()), nil
}

// UnmarshalText implements `encoding.TextUnmarshaler`.
func (o *JobOutput) UnmarshalText(text []byte) error {
	switch string(text) {
	case "stdout":
		*o = OutputStdout
	case "stderr":
		*o = OutputStderr
	case "mixed":
		*o = OutputBoth
	default:
		return fmt.Errorf("invalid output type: %s", string(text))
	}
	return nil
}

// JobID is represented by a string, but may be "*" in some cases to indicate
// any or all jobs.
type JobID string

// Container holds container fields for a Job.
type Container struct {
	// The name of the container image to use.
	Image string `json:"image"`

	// The ID of the user to run the container as. Optional.
	RunAsUser *int `json:"runAsUserId,omitempty"`

	// The ID of the group to run the container as. Optional.
	RunAsGroup *int `json:"runAsGroupId,omitempty"`

	// The list of additional group IDs to be added to the run-as user in
	// the container. Optional.
	SupplementalGroups []int `json:"supplementalGroupIds,omitempty"`
}

// Env is an environment variable.
type Env struct {
	// The name of the environment variable.
	Name string `json:"name"`

	// The value of the environment variable.
	Value string `json:"value"`
}

// String implements fmt.Stringer and flag.Value by emitting the standard
// `env=var` text representation of an environment variable.
func (e *Env) String() string {
	if e == nil {
		return ""
	}
	return e.Name + "=" + e.Value
}

// Set implements flag.Value by converting a text representation of the form
// `env=var` to an environment variable.
func (e *Env) Set(s string) error {
	spec := strings.Split(s, "=")
	if len(spec) != 2 {
		return fmt.Errorf("%w: %q", ErrInvalidEnvSpec, s)
	}
	*e = Env{
		Name:  spec[0],
		Value: spec[1],
	}
	return nil
}

// Job is Launcher's representation of a job.
type Job struct {
	// The unique ID of the Job.
	ID string `json:"id"`

	// The cluster of the Job. Optional.
	Cluster string `json:"cluster,omitempty"`

	// The name of the Job.
	Name string `json:"name,omitempty"`

	// The username of the user who launched the Job.
	User string `json:"user,omitempty"`

	// The group of the user who launched the Job. Optional.
	Group string `json:"group,omitempty"`

	// The list of queues that may be used to launch the Job, or the queue
	// that was used to run the Job.
	Queues []string `json:"queues,omitempty"`

	// The directory to use as the working directory for the Command or Exe.
	WorkDir string `json:"workingDirectory,omitempty"`

	// The container configuration of the Job, if the Cluster supports
	// containers. Optional.
	Container *Container `json:"container,omitempty"`

	// The host on which the Job was (or is being) run.
	Host string `json:"host,omitempty"`

	// The current status of the Job.
	Status string `json:"status,omitempty"`

	// The message or reason of the current status of the Job. Optional.
	StatusMsg string `json:"statusMessage,omitempty"`

	// The standard code/enum for the current status of the Job, if known.
	// Optional.
	StatusCode string `json:"statusCode,omitempty"`

	// The process ID of the Job, if applicable. Optional.
	Pid *int `json:"pid,omitempty"`

	// The exit code of the Command or Exe.
	ExitCode *int `json:"exitCode,omitempty"`

	// The shell command of the Job. Mutually exclusive with Exe.
	Command string `json:"command,omitempty"`

	// The executable of the Job. Mutually exclusive with Command.
	Exe string `json:"exe,omitempty"`

	// The location of the file which contains the standard output of the
	// Job.
	Stdout string `json:"stdoutFile,omitempty"`

	// The location of the file which contains the standard error output of
	// the Job.
	Stderr string `json:"stderrFile,omitempty"`

	// The standard input to be passed to the Command or Exe of the Job.
	Stdin string `json:"stdin,omitempty"`

	// The arguments of the Command or Exe of the Job.
	Args []string `json:"args,omitempty"`

	// The environment variables for the Job.
	Env []Env `json:"environment,omitempty"`

	// The list of placement constraints that were selected for the Job.
	Constraints []PlacementConstraint `json:"placementConstraints,omitempty"`

	// The time of the last update to the Job. Optional.
	LastUpdated *time.Time `json:"lastUpdateTime,omitempty"`

	// The time at which the Job was submitted to the Cluster. Optional.
	Submitted *time.Time `json:"submissionTime,omitempty"`

	// The exposed ports of the Job, if containers are used.
	Ports []Port `json:"exposedPorts,omitempty"`

	// The file system mounts to apply when the Job is run.
	Mounts []Mount `json:"mounts,omitempty"`

	// The custom configuration values of the Job.
	Config []JobConfig `json:"config,omitempty"`

	// The list of resource limits that were set for the Job.
	Limits []ResourceLimit `json:"resourceLimits,omitempty"`

	// The tags that were set for the Job. Used for filtering Jobs.
	Tags []string `json:"tags,omitempty"`

	// User-specified metadata for storing extension attributes.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// The resource profile of the Job, if any. The default is always
	// "custom".
	Profile string `json:"resourceProfile,omitempty"`

	// Plugin-local storage of job attributes not exposed through Launcher's
	// existing API.
	Misc map[string]interface{} `json:"-"`
}

// WithFields returns a copy of the job with only the given fields populated.
// When fields is empty it returns the original job.
func (job *Job) WithFields(fields []string) *Job {
	if len(fields) == 0 {
		return job
	}
	scrubbed := &Job{ID: job.ID} // ID is required.
	for _, f := range fields {
		switch f {
		case "cluster":
			scrubbed.Cluster = job.Cluster
		case "name":
			scrubbed.Name = job.Name
		case "user":
			scrubbed.User = job.User
		case "group":
			scrubbed.Group = job.Group
		case "queues":
			scrubbed.Queues = job.Queues
		case "workingDirectory":
			scrubbed.WorkDir = job.WorkDir
		case "container":
			scrubbed.Container = job.Container
		case "host":
			scrubbed.Host = job.Host
		case "status":
			scrubbed.Status = job.Status
		case "statusMessage":
			scrubbed.StatusMsg = job.StatusMsg
		case "statusCode":
			scrubbed.StatusCode = job.StatusCode
		case "pid":
			scrubbed.Pid = job.Pid
		case "exitCode":
			scrubbed.ExitCode = job.ExitCode
		case "command":
			scrubbed.Command = job.Command
		case "exe":
			scrubbed.Exe = job.Exe
		case "stdoutFile":
			scrubbed.Stdout = job.Stdout
		case "stderrFile":
			scrubbed.Stderr = job.Stderr
		case "stdin":
			scrubbed.Stdin = job.Stdin
		case "args":
			scrubbed.Args = job.Args
		case "env":
			scrubbed.Env = job.Env
		case "placementConstraints":
			scrubbed.Constraints = job.Constraints
		case "lastUpdateTime":
			scrubbed.LastUpdated = job.LastUpdated
		case "submissionTime":
			scrubbed.Submitted = job.Submitted
		case "exposedPorts":
			scrubbed.Ports = job.Ports
		case "mounts":
			scrubbed.Mounts = job.Mounts
		case "config":
			scrubbed.Config = job.Config
		case "resourceLimits":
			scrubbed.Limits = job.Limits
		case "tags":
			scrubbed.Tags = job.Tags
		case "metadata":
			scrubbed.Metadata = job.Metadata
		case "resourceProfile":
			scrubbed.Profile = job.Profile
		}
	}
	return scrubbed
}

// JobConfig holds custom Job configuration fields.
type JobConfig struct {
	// The name of the custom configuration value.
	Name string `json:"name"`

	// The type of the custom configuration value. Optional.
	Type string `json:"valueType,omitempty"`

	// The value of the custom configuration value. Optional.
	Value string `json:"value,omitempty"`
}

// JobOperation represents operations to control the state of a job.
type JobOperation int

const (
	// OperationSuspend indicates that the job should be suspended. This
	// operation should be equivalent to sending SIGSTOP.
	OperationSuspend JobOperation = iota

	// OperationResume indicates that the job should be resumed. This
	// operation should be equivalent to sending SIGCONT.
	OperationResume

	// OperationStop indicates that the job should be stopped. This
	// operation should be equivalent to sending SIGTERM.
	OperationStop

	// OperationKill indicates that the job should be killed. This
	// operation should be equivalent to sending SIGKILL.
	OperationKill

	// OperationCancel indicates that a pending job should be canceled, if
	// possible.
	OperationCancel
)

// ValidForStatus returns the job status required for this operation.
func (o JobOperation) ValidForStatus() string {
	switch o {
	case OperationSuspend, OperationStop, OperationKill:
		return StatusRunning
	case OperationResume:
		return StatusSuspended
	case OperationCancel:
		return StatusPending
	}
	return StatusPending
}

func (o JobOperation) String() string {
	switch o {
	case OperationSuspend:
		return "Suspend"
	case OperationResume:
		return "Resume"
	case OperationStop:
		return "Stop"
	case OperationKill:
		return "Kill"
	case OperationCancel:
		return "Cancel"
	}
	return "Cancel"
}

// Mount is a volume mounted into a Job.
type Mount struct {
	// The destination path of the mount.
	Path string `json:"mountPath"`

	// Whether the source path should be mounted with write permissions.
	ReadOnly bool `json:"readOnly,omitempty"`

	// The source of the mount.
	Source MountSource `json:"mountSource"`
}

// String implements fmt.Stringer and flag.Value by emitting a text
// representation of the form `src:dst:[:rw|:ro]` for a host mount.
func (m *Mount) String() string {
	if m == nil || m.Source.Type != MountTypeHost {
		return ""
	}
	opt := "rw"
	if m.ReadOnly {
		opt = "ro"
	}
	host, ok := m.Source.Source.(HostMount)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s:%s:%s", host.Path, m.Path, opt)
}

// Set implements flag.Value by converting a text representation of the form
// `src[:dst[:ro|:rw]]` to a host mount.
func (m *Mount) Set(s string) error {
	spec := strings.Split(s, ":")
	src := spec[0]
	if !path.IsAbs(src) {
		return fmt.Errorf("%w: %q", ErrInvalidMountPath, src)
	}
	var dst string
	readonly := false
	switch len(spec) {
	case 1:
		// A single path, like "/mnt/data".
		dst = src
	case 2:
		// A src:dst form like "/mnt/data:/home".
		dst = spec[1]
	case 3:
		// A full spec like "/mnt/data:/home:ro".
		dst = spec[1]
		switch spec[2] {
		case "rw": // The default.
		case "ro":
			readonly = true
		default:
			return fmt.Errorf("%w: %q", ErrInvalidMountSpec, s)
		}
	default:
		return fmt.Errorf("%w: %q", ErrInvalidMountSpec, s)
	}
	if !path.IsAbs(dst) {
		return fmt.Errorf("%w: %q", ErrInvalidMountPath, dst)
	}
	*m = Mount{
		Path:     dst,
		ReadOnly: readonly,
		Source: MountSource{
			Type: MountTypeHost,
			Source: HostMount{
				Path: src,
			},
		},
	}
	return nil
}

// MountSource is the underlying source for a volume mounted into a Job.
type MountSource struct {
	// The type of mount. The default supported options are "azureFile",
	// "cephFs", "glusterFs", "host", and "nfs". The "passthrough" value or
	// a custom value may be used for other mount types.
	Type string `json:"type"`

	// The mount source description. Must match the specified mount type.
	Source interface{} `json:"source"`
}

// UnmarshalJSON implements json.Unmarshaler for MountSource.
func (ms *MountSource) UnmarshalJSON(data []byte) error {
	type rawMountSource struct {
		Type   string          `json:"type"`
		Source json.RawMessage `json:"source"`
	}
	var raw rawMountSource
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch raw.Type {
	case MountTypeHost:
		source := HostMount{}
		if err := json.Unmarshal(raw.Source, &source); err != nil {
			return err
		}
		ms.Source = source
	default:
		// Allow mount types we don't explicitly validate pass through
		// unmangled.
		ms.Source = raw.Source
	}
	ms.Type = raw.Type
	return nil
}

// HostMount is a volume mounted from the host filesystem. This is the only widely-supported
// mount type.
type HostMount struct {
	// The path of the mount on the host filesystem.
	Path string `json:"path"`
}

// MountTypeHost is the mount type for host filesystem mounts.
const MountTypeHost = "host"

// PlacementConstraint is a generic Job placement constraint.
type PlacementConstraint struct {
	// The name of the placement constraint.
	Name string `json:"name"`

	// One of the possible values of the placement constraint. Optional.
	Value string `json:"value,omitempty"`
}

// Port is an exposed port for containerized jobs.
type Port struct {
	// The target port, within the container.
	TargetPort int `json:"targetPort"`

	// The published port, if different from the container port. Optional.
	PublishedPort *int `json:"publishedPort,omitempty"`

	// The network protocol to use. It should default to "TCP".
	Protocol string `json:"protocol"`
}

// String implements fmt.Stringer and flag.Value by emitting the standard
// `port:target/protocol` text representation of a port.
func (p *Port) String() string {
	if p == nil {
		return ""
	}
	proto := p.Protocol
	if proto == "" {
		proto = "tcp"
	}
	published := p.TargetPort
	if p.PublishedPort != nil {
		published = *p.PublishedPort
	}
	return fmt.Sprintf("%d:%d/%s", published, p.TargetPort, proto)
}

// Set implements flag.Value by converting a text representation of the form
// `port[:target][/protocol]` to a port.
func (p *Port) Set(s string) error {
	proto := "tcp"
	spec := strings.SplitN(s, "/", 2)
	if len(spec) == 2 {
		proto = spec[1]
	}
	spec = strings.Split(spec[0], ":")
	if len(spec) > 2 {
		return fmt.Errorf("%w: %q", ErrInvalidPortSpec, s)
	}
	port, err := strconv.Atoi(spec[0])
	if err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidPort, spec[0])
	}
	target := port
	var published *int
	if len(spec) == 2 {
		target, err = strconv.Atoi(spec[1])
		if err != nil {
			return fmt.Errorf("%w: %q", ErrInvalidPort, spec[1])
		}
		if target != port {
			published = &port
		}
	}
	*p = Port{
		TargetPort:    target,
		PublishedPort: published,
		Protocol:      proto,
	}
	return nil
}

// ResourceLimit holds resource controls for a Job.
type ResourceLimit struct {
	// The type of the resource. One of "cpuCount", "cpuTime", "memory", or
	// "memorySwap".
	Type string `json:"type"`

	// The requested value of the resource. Optional when Default and/or Max
	// is given instead.
	Value string `json:"value,omitempty"`

	// The default value of the resource type. Optional when Value is present.
	Default string `json:"defaultValue,omitempty"`

	// The maximum value of the resource type. Optional when Value is present.
	Max string `json:"maxValue,omitempty"`
}

// ResourceProfile holds details for a resource profile available on a cluster.
type ResourceProfile struct {
	// The name of a resource profile.
	Name string `json:"name"`

	// A user-friendly name for the resource profile. Optional.
	DisplayName string `json:"displayName,omitempty"`

	// The corresponding resource limits for this profile. Optional.
	Limits []ResourceLimit `json:"limits,omitempty"`

	// The submission queue for this profile, if applicable. Optional.
	Queue string `json:"queue"`

	// Placement constraints for this profile. Optional.
	Constraints []PlacementConstraint `json:"placementConstraints"`
}

// Node represents a Launcher/plugin node when running in a load-balanced scenario.
type Node struct {
	Host     string     `json:"host"`
	IP       netip.Addr `json:"ipv4"`
	Port     string     `json:"port"`
	LastSeen time.Time  `json:"lastSeen"`
	Status   string     `json:"status"`
}

// Online returns true when a node can be considered available, and false
// otherwise.
func (n *Node) Online() bool {
	return n.Status == "Online"
}

// Version is a Launcher version, including optional build and revision information.
type Version struct {
	Major    int    `json:"major"`
	Minor    int    `json:"minor"`
	Patch    int    `json:"patch"`
	Build    int    `json:"build,omitempty"`
	Revision string `json:"revision,omitempty"`
}

// APIVersion is the Launcher plugin API version supported by the types defined
// in this package.
var APIVersion = Version{Major: 3, Minor: 5, Patch: 0}

// Parsing errors.
var (
	// Indicates that a mount spec (of the form `dst[:src][:ro]`) is
	// invalid.
	ErrInvalidMountSpec = fmt.Errorf("invalid mount spec")

	// Indicates that a path cannot be used as written in a mount spec.
	ErrInvalidMountPath = fmt.Errorf("invalid mount path")

	// Indicates that a port spec (of the form `port[:target]`) is invalid.
	ErrInvalidPortSpec = fmt.Errorf("invalid port spec")

	// Indicates that a port cannot be used as written in a port spec.
	ErrInvalidPort = fmt.Errorf("invalid port")

	// Indicates that an environment variable spec (of the form `env=var`)
	// is invalid.
	ErrInvalidEnvSpec = fmt.Errorf("invalid env spec")
)
