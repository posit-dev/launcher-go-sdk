package plugintest

import (
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/launcher"
)

// JobBuilder provides a fluent API for constructing test jobs.
type JobBuilder struct {
	job *api.Job
}

// NewJob creates a new JobBuilder with default values.
func NewJob() *JobBuilder {
	now := time.Now()
	return &JobBuilder{
		job: &api.Job{
			ID:          "test-job-1",
			Status:      api.StatusPending,
			Submitted:   &now,
			LastUpdated: &now,
		},
	}
}

// NewJobWithID creates a new JobBuilder with the specified ID.
func NewJobWithID(id api.JobID) *JobBuilder {
	b := NewJob()
	b.job.ID = id
	return b
}

// WithID sets the job ID.
func (b *JobBuilder) WithID(id api.JobID) *JobBuilder {
	b.job.ID = id
	return b
}

// WithName sets the job name.
func (b *JobBuilder) WithName(name string) *JobBuilder {
	b.job.Name = name
	return b
}

// WithUser sets the job user.
func (b *JobBuilder) WithUser(user string) *JobBuilder {
	b.job.User = user
	return b
}

// WithGroup sets the job group.
func (b *JobBuilder) WithGroup(group string) *JobBuilder {
	b.job.Group = group
	return b
}

// WithStatus sets the job status.
func (b *JobBuilder) WithStatus(status string) *JobBuilder {
	b.job.Status = status
	return b
}

// WithStatusMessage sets the job status message.
func (b *JobBuilder) WithStatusMessage(msg string) *JobBuilder {
	b.job.StatusMsg = msg
	return b
}

// WithHost sets the job host.
func (b *JobBuilder) WithHost(host string) *JobBuilder {
	b.job.Host = host
	return b
}

// WithPid sets the job process ID.
func (b *JobBuilder) WithPid(pid int) *JobBuilder {
	b.job.Pid = &pid
	return b
}

// WithExitCode sets the job exit code.
func (b *JobBuilder) WithExitCode(code int) *JobBuilder {
	b.job.ExitCode = &code
	return b
}

// WithCommand sets the job command.
func (b *JobBuilder) WithCommand(command string) *JobBuilder {
	b.job.Command = command
	return b
}

// WithExe sets the job executable.
func (b *JobBuilder) WithExe(exe string) *JobBuilder {
	b.job.Exe = exe
	return b
}

// WithArgs sets the job arguments.
func (b *JobBuilder) WithArgs(args ...string) *JobBuilder {
	b.job.Args = args
	return b
}

// WithEnv sets a single environment variable.
func (b *JobBuilder) WithEnv(name, value string) *JobBuilder {
	if b.job.Env == nil {
		b.job.Env = []api.Env{}
	}
	b.job.Env = append(b.job.Env, api.Env{Name: name, Value: value})
	return b
}

// WithEnvVars sets multiple environment variables.
func (b *JobBuilder) WithEnvVars(env []api.Env) *JobBuilder {
	b.job.Env = env
	return b
}

// WithWorkDir sets the job working directory.
func (b *JobBuilder) WithWorkDir(dir string) *JobBuilder {
	b.job.WorkDir = dir
	return b
}

// WithStdout sets the stdout file path.
func (b *JobBuilder) WithStdout(path string) *JobBuilder {
	b.job.Stdout = path
	return b
}

// WithStderr sets the stderr file path.
func (b *JobBuilder) WithStderr(path string) *JobBuilder {
	b.job.Stderr = path
	return b
}

// WithStdin sets the stdin content.
func (b *JobBuilder) WithStdin(stdin string) *JobBuilder {
	b.job.Stdin = stdin
	return b
}

// WithQueue sets a single queue.
func (b *JobBuilder) WithQueue(queue string) *JobBuilder {
	b.job.Queues = []string{queue}
	return b
}

// WithQueues sets multiple queues.
func (b *JobBuilder) WithQueues(queues ...string) *JobBuilder {
	b.job.Queues = queues
	return b
}

// WithCluster sets the cluster name.
func (b *JobBuilder) WithCluster(cluster string) *JobBuilder {
	b.job.Cluster = cluster
	return b
}

// WithProfile sets the resource profile.
func (b *JobBuilder) WithProfile(profile string) *JobBuilder {
	b.job.Profile = profile
	return b
}

// WithTag adds a tag to the job.
func (b *JobBuilder) WithTag(tag string) *JobBuilder {
	if b.job.Tags == nil {
		b.job.Tags = []string{}
	}
	b.job.Tags = append(b.job.Tags, tag)
	return b
}

// WithTags sets multiple tags.
func (b *JobBuilder) WithTags(tags ...string) *JobBuilder {
	b.job.Tags = tags
	return b
}

// WithContainer sets container configuration.
func (b *JobBuilder) WithContainer(image string) *JobBuilder {
	b.job.Container = &api.Container{
		Image: image,
	}
	return b
}

// WithContainerUser sets the container run-as user ID.
func (b *JobBuilder) WithContainerUser(uid int) *JobBuilder {
	if b.job.Container == nil {
		b.job.Container = &api.Container{}
	}
	b.job.Container.RunAsUser = &uid
	return b
}

// WithContainerGroup sets the container run-as group ID.
func (b *JobBuilder) WithContainerGroup(gid int) *JobBuilder {
	if b.job.Container == nil {
		b.job.Container = &api.Container{}
	}
	b.job.Container.RunAsGroup = &gid
	return b
}

// WithMount adds a host mount to the job.
func (b *JobBuilder) WithMount(src, dst string, readOnly bool) *JobBuilder {
	if b.job.Mounts == nil {
		b.job.Mounts = []api.Mount{}
	}
	b.job.Mounts = append(b.job.Mounts, api.Mount{
		Path:     dst,
		ReadOnly: readOnly,
		Source: api.MountSource{
			Type:   api.MountTypeHost,
			Source: api.HostMount{Path: src},
		},
	})
	return b
}

// WithPort adds an exposed port to the job.
func (b *JobBuilder) WithPort(target int, protocol string) *JobBuilder {
	if b.job.Ports == nil {
		b.job.Ports = []api.Port{}
	}
	b.job.Ports = append(b.job.Ports, api.Port{
		TargetPort: target,
		Protocol:   protocol,
	})
	return b
}

// WithPublishedPort adds an exposed port with a published port to the job.
func (b *JobBuilder) WithPublishedPort(published, target int, protocol string) *JobBuilder {
	if b.job.Ports == nil {
		b.job.Ports = []api.Port{}
	}
	b.job.Ports = append(b.job.Ports, api.Port{
		TargetPort:    target,
		PublishedPort: &published,
		Protocol:      protocol,
	})
	return b
}

// WithLimit adds a resource limit to the job.
func (b *JobBuilder) WithLimit(limitType, value string) *JobBuilder {
	if b.job.Limits == nil {
		b.job.Limits = []api.ResourceLimit{}
	}
	b.job.Limits = append(b.job.Limits, api.ResourceLimit{
		Type:  limitType,
		Value: value,
	})
	return b
}

// WithConstraint adds a placement constraint to the job.
func (b *JobBuilder) WithConstraint(name, value string) *JobBuilder {
	if b.job.Constraints == nil {
		b.job.Constraints = []api.PlacementConstraint{}
	}
	b.job.Constraints = append(b.job.Constraints, api.PlacementConstraint{
		Name:  name,
		Value: value,
	})
	return b
}

// WithConfig adds a custom configuration value to the job.
func (b *JobBuilder) WithConfig(name, valueType, value string) *JobBuilder {
	if b.job.Config == nil {
		b.job.Config = []api.JobConfig{}
	}
	b.job.Config = append(b.job.Config, api.JobConfig{
		Name:  name,
		Type:  valueType,
		Value: value,
	})
	return b
}

// WithMetadata sets a metadata value.
func (b *JobBuilder) WithMetadata(key string, value interface{}) *JobBuilder {
	if b.job.Metadata == nil {
		b.job.Metadata = make(map[string]interface{})
	}
	b.job.Metadata[key] = value
	return b
}

// WithSubmissionTime sets the submission time.
func (b *JobBuilder) WithSubmissionTime(t time.Time) *JobBuilder {
	b.job.Submitted = &t
	return b
}

// WithLastUpdated sets the last updated time.
func (b *JobBuilder) WithLastUpdated(t time.Time) *JobBuilder {
	b.job.LastUpdated = &t
	return b
}

// Pending sets the job status to Pending.
func (b *JobBuilder) Pending() *JobBuilder {
	b.job.Status = api.StatusPending
	return b
}

// Running sets the job status to Running.
func (b *JobBuilder) Running() *JobBuilder {
	b.job.Status = api.StatusRunning
	return b
}

// Finished sets the job status to Finished with an optional exit code.
func (b *JobBuilder) Finished(exitCode int) *JobBuilder {
	b.job.Status = api.StatusFinished
	b.job.ExitCode = &exitCode
	return b
}

// Failed sets the job status to Failed with an optional message.
func (b *JobBuilder) Failed(msg string) *JobBuilder {
	b.job.Status = api.StatusFailed
	if msg != "" {
		b.job.StatusMsg = msg
	}
	return b
}

// Killed sets the job status to Killed.
func (b *JobBuilder) Killed() *JobBuilder {
	b.job.Status = api.StatusKilled
	return b
}

// Canceled sets the job status to Canceled.
func (b *JobBuilder) Canceled() *JobBuilder {
	b.job.Status = api.StatusCanceled
	return b
}

// Suspended sets the job status to Suspended.
func (b *JobBuilder) Suspended() *JobBuilder {
	b.job.Status = api.StatusSuspended
	return b
}

// Build returns the constructed Job.
func (b *JobBuilder) Build() *api.Job {
	return b.job
}

// Clone creates a copy of the builder with a cloned job.
func (b *JobBuilder) Clone() *JobBuilder {
	jobCopy := *b.job
	// Deep copy slices and maps
	if b.job.Queues != nil {
		jobCopy.Queues = make([]string, len(b.job.Queues))
		copy(jobCopy.Queues, b.job.Queues)
	}
	if b.job.Args != nil {
		jobCopy.Args = make([]string, len(b.job.Args))
		copy(jobCopy.Args, b.job.Args)
	}
	if b.job.Env != nil {
		jobCopy.Env = make([]api.Env, len(b.job.Env))
		copy(jobCopy.Env, b.job.Env)
	}
	if b.job.Tags != nil {
		jobCopy.Tags = make([]string, len(b.job.Tags))
		copy(jobCopy.Tags, b.job.Tags)
	}
	if b.job.Metadata != nil {
		jobCopy.Metadata = make(map[string]interface{})
		for k, v := range b.job.Metadata {
			jobCopy.Metadata[k] = v
		}
	}
	return &JobBuilder{job: &jobCopy}
}

// JobFilterBuilder provides a fluent API for constructing test job filters.
type JobFilterBuilder struct {
	filter *api.JobFilter
}

// NewJobFilter creates a new JobFilterBuilder.
func NewJobFilter() *JobFilterBuilder {
	return &JobFilterBuilder{
		filter: &api.JobFilter{},
	}
}

// WithTag adds a tag filter.
func (b *JobFilterBuilder) WithTag(tag string) *JobFilterBuilder {
	if b.filter.Tags == nil {
		b.filter.Tags = []string{}
	}
	b.filter.Tags = append(b.filter.Tags, tag)
	return b
}

// WithTags sets multiple tag filters.
func (b *JobFilterBuilder) WithTags(tags ...string) *JobFilterBuilder {
	b.filter.Tags = tags
	return b
}

// WithStartTime sets the start time filter.
func (b *JobFilterBuilder) WithStartTime(t time.Time) *JobFilterBuilder {
	b.filter.StartTime = &t
	return b
}

// WithEndTime sets the end time filter.
func (b *JobFilterBuilder) WithEndTime(t time.Time) *JobFilterBuilder {
	b.filter.EndTime = &t
	return b
}

// WithStatus adds a status filter.
func (b *JobFilterBuilder) WithStatus(status string) *JobFilterBuilder {
	if b.filter.Statuses == nil {
		b.filter.Statuses = []string{}
	}
	b.filter.Statuses = append(b.filter.Statuses, status)
	return b
}

// WithStatuses sets multiple status filters.
func (b *JobFilterBuilder) WithStatuses(statuses ...string) *JobFilterBuilder {
	b.filter.Statuses = statuses
	return b
}

// WithField adds a field filter.
func (b *JobFilterBuilder) WithField(field string) *JobFilterBuilder {
	if b.filter.Fields == nil {
		b.filter.Fields = []string{}
	}
	b.filter.Fields = append(b.filter.Fields, field)
	return b
}

// WithFields sets multiple field filters.
func (b *JobFilterBuilder) WithFields(fields ...string) *JobFilterBuilder {
	b.filter.Fields = fields
	return b
}

// Build returns the constructed JobFilter.
func (b *JobFilterBuilder) Build() *api.JobFilter {
	return b.filter
}

// ClusterOptionsBuilder provides a fluent API for constructing test cluster options.
type ClusterOptionsBuilder struct {
	opts *launcher.ClusterOptions
}

// NewClusterOptions creates a new ClusterOptionsBuilder.
func NewClusterOptions() *ClusterOptionsBuilder {
	return &ClusterOptionsBuilder{
		opts: &launcher.ClusterOptions{
			Constraints: []api.PlacementConstraint{},
			Queues:      []string{},
			Limits:      []api.ResourceLimit{},
			Configs:     []api.JobConfig{},
			Profiles:    []api.ResourceProfile{},
		},
	}
}

// WithQueue adds a queue.
func (b *ClusterOptionsBuilder) WithQueue(queue string) *ClusterOptionsBuilder {
	b.opts.Queues = append(b.opts.Queues, queue)
	return b
}

// WithQueues sets multiple queues.
func (b *ClusterOptionsBuilder) WithQueues(queues ...string) *ClusterOptionsBuilder {
	b.opts.Queues = queues
	return b
}

// WithDefaultQueue sets the default queue.
func (b *ClusterOptionsBuilder) WithDefaultQueue(queue string) *ClusterOptionsBuilder {
	b.opts.DefaultQueue = queue
	return b
}

// WithConstraint adds a placement constraint.
func (b *ClusterOptionsBuilder) WithConstraint(name, value string) *ClusterOptionsBuilder {
	b.opts.Constraints = append(b.opts.Constraints, api.PlacementConstraint{
		Name:  name,
		Value: value,
	})
	return b
}

// WithLimit adds a resource limit.
func (b *ClusterOptionsBuilder) WithLimit(limitType, value string) *ClusterOptionsBuilder {
	b.opts.Limits = append(b.opts.Limits, api.ResourceLimit{
		Type:  limitType,
		Value: value,
	})
	return b
}

// WithImage adds a container image to the allowed list.
func (b *ClusterOptionsBuilder) WithImage(image string) *ClusterOptionsBuilder {
	b.opts.ImageOpt.Images = append(b.opts.ImageOpt.Images, image)
	return b
}

// WithDefaultImage sets the default container image.
func (b *ClusterOptionsBuilder) WithDefaultImage(image string) *ClusterOptionsBuilder {
	b.opts.ImageOpt.Default = image
	return b
}

// WithAllowUnknownImages sets whether unknown images are allowed.
func (b *ClusterOptionsBuilder) WithAllowUnknownImages(allow bool) *ClusterOptionsBuilder {
	b.opts.ImageOpt.AllowUnknown = allow
	return b
}

// WithHostNetwork sets whether containers use host networking.
func (b *ClusterOptionsBuilder) WithHostNetwork(hostNetwork bool) *ClusterOptionsBuilder {
	b.opts.ImageOpt.HostNetwork = hostNetwork
	return b
}

// WithProfile adds a resource profile.
func (b *ClusterOptionsBuilder) WithProfile(name, displayName string) *ClusterOptionsBuilder {
	b.opts.Profiles = append(b.opts.Profiles, api.ResourceProfile{
		Name:        name,
		DisplayName: displayName,
	})
	return b
}

// WithConfig adds a custom configuration option.
func (b *ClusterOptionsBuilder) WithConfig(name, valueType, value string) *ClusterOptionsBuilder {
	b.opts.Configs = append(b.opts.Configs, api.JobConfig{
		Name:  name,
		Type:  valueType,
		Value: value,
	})
	return b
}

// Build returns the constructed ClusterOptions.
func (b *ClusterOptionsBuilder) Build() launcher.ClusterOptions {
	return *b.opts
}
