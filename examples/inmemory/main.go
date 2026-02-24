// Package main demonstrates an in-memory launcher plugin with job lifecycle simulation.
//
// This example shows how to:
//   - Generate unique job IDs
//   - Simulate job lifecycle (pending -> running -> finished)
//   - Use background goroutines to update job status
//   - Handle job control operations
//   - Stream job output and status
//   - Use the JobCache effectively
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/cache"
	"github.com/posit-dev/launcher-go-sdk/launcher"
	"github.com/posit-dev/launcher-go-sdk/logger"
)

// InMemoryPlugin simulates a job scheduler that runs jobs in memory.
// It demonstrates the complete lifecycle of a job from submission to completion.
type InMemoryPlugin struct {
	cache  *cache.JobCache
	nextID int32          // Atomic counter for generating unique job IDs
	wg     sync.WaitGroup // Tracks background goroutines for clean shutdown
}

// SubmitJob accepts a new job submission and begins simulating its execution.
//
// This method:
//  1. Assigns a unique job ID
//  2. Sets initial timestamps and status
//  3. Stores the job in the cache
//  4. Starts a background goroutine to simulate job execution
//  5. Returns the job to the caller
func (p *InMemoryPlugin) SubmitJob(w launcher.ResponseWriter, user string, job *api.Job) {
	// Generate a unique job ID using an atomic counter
	id := fmt.Sprintf("job-%d", atomic.AddInt32(&p.nextID, 1))

	// Set job metadata
	now := time.Now().UTC()
	job.ID = id
	job.User = user
	job.Submitted = &now
	job.LastUpdated = &now
	job.Status = api.StatusPending

	// Store the job in the cache
	if err := p.cache.AddOrUpdate(job); err != nil {
		w.WriteError(err)
		return
	}

	// Return the job to the caller
	p.cache.WriteJob(w, user, id)

	// Check if this is a long-running job (e.g., an interactive session).
	// Jobs tagged "long-running" stay in the Running state indefinitely
	// until explicitly stopped, killed, or canceled.
	longRunning := false
	for _, tag := range job.Tags {
		if tag == "long-running" {
			longRunning = true
			break
		}
	}

	// Start a background goroutine to simulate job execution.
	// This goroutine will:
	//  1. Wait 500ms, then transition from Pending to Running
	//  2. Wait 5s, then transition from Running to Finished (unless long-running)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.simulateJobLifecycle(user, id, longRunning)
	}()
}

// simulateJobLifecycle runs in a background goroutine to simulate job execution.
// This demonstrates how a real plugin would update job status as the actual
// job progresses through its lifecycle.
//
// If longRunning is true, the job stays in Running state indefinitely until
// explicitly controlled (stop, kill, cancel). This models interactive sessions.
func (p *InMemoryPlugin) simulateJobLifecycle(user, id string, longRunning bool) {
	// After 500ms, mark the job as running if it's still pending.
	// We use a short delay so that control operations (cancel/kill/stop)
	// can be tested while the job is pending.
	time.Sleep(500 * time.Millisecond)
	p.cache.Update(user, id, func(job *api.Job) *api.Job {
		if job.Status == api.StatusPending {
			job.Status = api.StatusRunning
		}
		return job
	})

	// Long-running jobs stay in Running state until explicitly controlled.
	if longRunning {
		return
	}

	// After 5 seconds total, mark the job as finished if it's still running.
	time.Sleep(4500 * time.Millisecond)
	p.cache.Update(user, id, func(job *api.Job) *api.Job {
		if job.Status == api.StatusRunning {
			exitCode := 0
			job.Status = api.StatusFinished
			job.ExitCode = &exitCode
		}
		return job
	})
}

// GetJob returns information about a specific job.
// The cache handles looking up the job and checking permissions.
func (p *InMemoryPlugin) GetJob(w launcher.ResponseWriter, user string, id api.JobID, fields []string) {
	p.cache.WriteJob(w, user, string(id))
}

// GetJobs returns information about all jobs matching the filter.
// The filter can specify:
//   - Status (e.g., only running jobs)
//   - Tags (e.g., jobs with specific labels)
//   - Time range (e.g., jobs submitted in the last hour)
func (p *InMemoryPlugin) GetJobs(w launcher.ResponseWriter, user string, filter *api.JobFilter, fields []string) {
	p.cache.WriteJobs(w, user, filter)
}

// GetJobOutput streams the output (stdout/stderr) of a job.
//
// This demonstrates:
//   - Using StreamResponseWriter to send multiple chunks
//   - Running the streaming in a separate goroutine
//   - Calling Close() when streaming is complete
//   - Respecting context cancellation
func (p *InMemoryPlugin) GetJobOutput(ctx context.Context, w launcher.StreamResponseWriter, user string, id api.JobID, outputType api.JobOutput) {
	// First, verify the job exists
	err := p.cache.Lookup(user, string(id), func(_ *api.Job) {})
	if err != nil {
		w.WriteError(err)
		return
	}

	// Stream output in a background goroutine.
	// This is safe because StreamResponseWriter is thread-safe.
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		// Always call Close() when done streaming
		defer w.Close()

		// Simulate streaming output for 3 seconds
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for i := 0; i < 3; i++ {
			select {
			case <-ctx.Done():
				// Client cancelled the stream
				return
			case <-ticker.C:
				// Send a chunk of output
				var output string
				switch outputType {
				case api.OutputStdout:
					output = fmt.Sprintf("Line %d from stdout\n", i+1)
				case api.OutputStderr:
					output = fmt.Sprintf("Line %d from stderr\n", i+1)
				default: // api.OutputBoth
					output = fmt.Sprintf("Line %d from stdout\nLine %d from stderr\n", i+1, i+1)
				}
				w.WriteJobOutput(output, outputType)
			}
		}
	}()
}

// GetJobStatus streams status updates for a specific job.
// The cache handles the streaming and automatically sends updates
// when the job's status changes.
func (p *InMemoryPlugin) GetJobStatus(ctx context.Context, w launcher.StreamResponseWriter, user string, id api.JobID) {
	p.cache.StreamJobStatus(ctx, w, user, string(id))
}

// GetJobStatuses streams status updates for all jobs belonging to the user.
// This is used by the Launcher to monitor job progress.
func (p *InMemoryPlugin) GetJobStatuses(ctx context.Context, w launcher.StreamResponseWriter, user string) {
	p.cache.StreamJobStatuses(ctx, w, user)
}

// GetJobResourceUtil streams resource utilization data for a job.
//
// This demonstrates:
//   - Polling for resource data periodically
//   - Stopping when the job reaches a terminal status
//   - Respecting context cancellation
func (p *InMemoryPlugin) GetJobResourceUtil(ctx context.Context, w launcher.StreamResponseWriter, user string, id api.JobID) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		// Look up the job and send resource utilization
		err := p.cache.Lookup(user, string(id), func(job *api.Job) {
			// In a real plugin, you would measure actual CPU/memory usage
			// For this demo, we send simulated values
			w.WriteJobResourceUtil(
				15.0,  // CPU percent
				100.0, // CPU time (seconds)
				350.0, // Resident memory (MB)
				80.0,  // Virtual memory (MB)
			)

			// Stop streaming if the job is done
			if api.TerminalStatus(job.Status) {
				w.Close()
			}
		})

		if err != nil {
			// Job not found or permission denied
			return
		}

		// Wait for next poll or cancellation
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			continue
		}
	}
}

// ControlJob performs an operation on a running job (stop, kill, cancel, suspend, resume).
//
// This demonstrates:
//   - Validating the operation is valid for the current job status
//   - Updating job status based on the operation
//   - Handling unsupported operations
func (p *InMemoryPlugin) ControlJob(w launcher.ResponseWriter, user string, id api.JobID, op api.JobOperation) {
	err := p.cache.Update(user, string(id), func(job *api.Job) *api.Job {
		// Validate that the operation is valid for the current status
		expectedStatus := op.ValidForStatus()
		if expectedStatus != job.Status {
			w.WriteErrorf(api.CodeInvalidJobState,
				"Job must be %s to %s it (current status: %s)",
				expectedStatus, op, job.Status)
			return job // Return unchanged job
		}

		// This example plugin doesn't support suspend/resume
		if op == api.OperationSuspend || op == api.OperationResume {
			w.WriteErrorf(api.CodeRequestNotSupported,
				"Operation %s is not supported by this plugin", op)
			return job
		}

		// Perform the operation
		switch op {
		case api.OperationStop:
			// Graceful stop (SIGTERM)
			job.Status = api.StatusFinished
			exitCode := 143 // 128 + 15 (SIGTERM)
			job.ExitCode = &exitCode
		case api.OperationKill:
			// Forceful kill (SIGKILL)
			job.Status = api.StatusKilled
			exitCode := 137 // 128 + 9 (SIGKILL)
			job.ExitCode = &exitCode
		case api.OperationCancel:
			// Cancel before job starts
			job.Status = api.StatusCanceled
		}

		// Report success
		w.WriteControlJob(true, fmt.Sprintf("Job %s successful", op))
		return job
	})

	if err != nil {
		w.WriteError(err)
	}
}

// GetJobNetwork returns network information for a job.
// For containerized jobs, this would include the container's IP addresses.
// For this example, we just return the hostname.
func (p *InMemoryPlugin) GetJobNetwork(w launcher.ResponseWriter, user string, id api.JobID) {
	err := p.cache.Lookup(user, string(id), func(_ *api.Job) {
		hostname, _ := os.Hostname()
		// In a real plugin, you would query the actual job's network info
		w.WriteJobNetwork(hostname, []string{"127.0.0.1"})
	})
	if err != nil {
		w.WriteError(err)
	}
}

// ClusterInfo returns information about the cluster's capabilities.
//
// This tells the Launcher:
//   - What queues are available
//   - What resource limits can be set
//   - What container images are available (if containers are supported)
//   - What custom configuration options are available
//   - What placement constraints are available
func (p *InMemoryPlugin) ClusterInfo(w launcher.ResponseWriter, user string) {
	w.WriteClusterInfo(launcher.ClusterOptions{
		// Define available queues
		Queues:       []string{"default", "high-priority", "batch"},
		DefaultQueue: "default",

		// Define resource limits
		Limits: []api.ResourceLimit{
			{Type: "cpuCount", Max: "32"},
			{Type: "memory", Max: "128GB"},
		},

		// This example doesn't support containers
		ImageOpt: launcher.ImageOptions{},

		// Example custom configuration options
		Configs: []api.JobConfig{
			{Name: "email", Type: "string"},
			{Name: "notifications", Type: "bool"},
		},

		// No placement constraints in this example
		Constraints: []api.PlacementConstraint{},

		// No resource profiles in this example
		Profiles: []api.ResourceProfile{},
	})
}

func main() {
	// Set up signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load command-line options
	// The Launcher will pass options like --enable-debug-logging, --plugin-name, etc.
	options := &launcher.DefaultOptions{}
	launcher.MustLoadOptions(options, "inmemory")

	// Create a logger that writes to stderr and log files
	lgr := logger.MustNewLogger("inmemory", options.Debug, options.LoggingDir)
	lgr.Info("Starting InMemory plugin")

	// Create the job cache (in-memory only, no persistence)
	// Pass an empty string for the directory to use in-memory storage
	jobCache, err := cache.NewJobCache(ctx, lgr, "")
	if err != nil {
		lgr.Error("Failed to create job cache", "error", err)
		os.Exit(1)
	}

	// Create the plugin instance
	plugin := &InMemoryPlugin{
		cache:  jobCache,
		nextID: 0,
	}

	// Create the runtime and start handling requests
	// This blocks until the context is cancelled (e.g., Ctrl+C)
	lgr.Info("Plugin ready to accept requests")
	if err := launcher.NewRuntime(lgr, plugin).Run(ctx); err != nil {
		lgr.Error("Plugin runtime error", "error", err)
		os.Exit(1)
	}

	lgr.Info("Plugin shutting down")
}
