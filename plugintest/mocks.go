package plugintest

import (
	"errors"
	"slices"
	"sync"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/launcher"
)

// MockResponseWriter is a mock implementation of launcher.ResponseWriter that
// captures all responses for test assertions.
//
// All accessor methods are safe to call concurrently with Write* methods.
// They return snapshots taken under a read lock; the writer does not
// subsequently mutate recorded entries. The returned slice headers (and,
// for accessors that return value-typed elements such as ControlResults,
// the elements themselves) are independent of the writer's stored state
// and may be freely mutated by the caller.
//
// Pointer-typed elements such as *api.Error and *api.Job, and slice
// fields inside value-typed elements that are not explicitly noted as
// deep-copied, alias the writer's stored values. Treat them as read-only.
// Accessors that DO return a deep copy say so explicitly in their godoc.
type MockResponseWriter struct {
	mu sync.RWMutex

	errors              []*api.Error
	jobs                [][]*api.Job
	controlResults      []ControlResult
	networks            []NetworkInfo
	clusterInfo         *launcher.ClusterOptions
	configReloadResults []ConfigReloadResult
}

// ControlResult represents a control job operation result.
type ControlResult struct {
	Complete bool
	Message  string
}

// NetworkInfo represents job network information.
type NetworkInfo struct {
	Host      string
	Addresses []string
}

// ConfigReloadResult represents a config reload operation result.
type ConfigReloadResult struct {
	ErrorType    api.ConfigReloadErrorType
	ErrorMessage string
}

// NewMockResponseWriter creates a new MockResponseWriter.
func NewMockResponseWriter() *MockResponseWriter {
	return &MockResponseWriter{
		errors:              []*api.Error{},
		jobs:                [][]*api.Job{},
		controlResults:      []ControlResult{},
		networks:            []NetworkInfo{},
		configReloadResults: []ConfigReloadResult{},
	}
}

// WriteErrorf implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteErrorf(code api.ErrCode, format string, a ...interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = append(m.errors, api.Errorf(code, format, a...))
	return nil
}

// WriteError implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteError(err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		m.errors = append(m.errors, apiErr)
	} else {
		m.errors = append(m.errors, &api.Error{Code: api.CodeUnknown, Msg: err.Error()})
	}
	return nil
}

// WriteJobs implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteJobs(jobs []*api.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs = append(m.jobs, jobs)
	return nil
}

// WriteControlJob implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteControlJob(complete bool, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.controlResults = append(m.controlResults, ControlResult{
		Complete: complete,
		Message:  msg,
	})
	return nil
}

// WriteJobNetwork implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteJobNetwork(host string, addr []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.networks = append(m.networks, NetworkInfo{
		Host:      host,
		Addresses: addr,
	})
	return nil
}

// WriteClusterInfo implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteClusterInfo(opts launcher.ClusterOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clusterInfo = &opts
	return nil
}

// WriteConfigReload captures config reload responses for test assertions.
func (m *MockResponseWriter) WriteConfigReload(errorType api.ConfigReloadErrorType, errorMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configReloadResults = append(m.configReloadResults, ConfigReloadResult{
		ErrorType:    errorType,
		ErrorMessage: errorMessage,
	})
	return nil
}

// Errors returns a snapshot slice of every error written. The *api.Error
// elements alias the writer's stored entries; treat them as read-only.
func (m *MockResponseWriter) Errors() []*api.Error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.errors)
}

// HasError returns true if any errors were written.
func (m *MockResponseWriter) HasError() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.errors) > 0
}

// LastError returns the most recent error, or nil if no errors were written.
// The returned *api.Error aliases the writer's stored entry; treat as read-only.
func (m *MockResponseWriter) LastError() *api.Error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.errors) == 0 {
		return nil
	}
	return m.errors[len(m.errors)-1]
}

// FirstError returns the first error, or nil if no errors were written.
// The returned *api.Error aliases the writer's stored entry; treat as read-only.
func (m *MockResponseWriter) FirstError() *api.Error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.errors) == 0 {
		return nil
	}
	return m.errors[0]
}

// Jobs returns a snapshot of every job batch written via WriteJobs. The outer
// slice and each inner []*api.Job slice header are freshly allocated, so
// callers may freely append or reorder. The *api.Job elements within those
// inner slices alias the writer's stored entries; treat them as read-only.
func (m *MockResponseWriter) Jobs() [][]*api.Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([][]*api.Job, len(m.jobs))
	for i, batch := range m.jobs {
		out[i] = slices.Clone(batch)
	}
	return out
}

// LastJobs returns a snapshot slice of the most recently written job batch,
// or nil if no jobs were written. The *api.Job elements alias the writer's
// stored entries; treat them as read-only.
func (m *MockResponseWriter) LastJobs() []*api.Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.jobs) == 0 {
		return nil
	}
	return slices.Clone(m.jobs[len(m.jobs)-1])
}

// AllJobs returns a snapshot slice of every job from every WriteJobs call,
// flattened in insertion order. The *api.Job elements alias the writer's
// stored entries; treat them as read-only.
func (m *MockResponseWriter) AllJobs() []*api.Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var all []*api.Job
	for _, jobs := range m.jobs {
		all = append(all, jobs...)
	}
	return all
}

// ControlResults returns a snapshot slice of every control result captured.
// The slice and its value-typed elements are independent of the writer.
func (m *MockResponseWriter) ControlResults() []ControlResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.controlResults)
}

// Networks returns a deep-copy snapshot of every network response captured.
// Each entry's Addresses slice is freshly allocated, so callers may freely
// mutate both the outer slice and the per-entry Addresses.
func (m *MockResponseWriter) Networks() []NetworkInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]NetworkInfo, len(m.networks))
	for i, n := range m.networks {
		out[i] = NetworkInfo{
			Host:      n.Host,
			Addresses: slices.Clone(n.Addresses),
		}
	}
	return out
}

// ClusterInfo returns a deep-copy snapshot of the most recently captured
// cluster options, or nil if WriteClusterInfo has not been called. The
// returned pointer, its slice fields (Constraints, Queues, Limits, Configs,
// Profiles), the nested ImageOpt.Images slice, and each ResourceProfile's
// inner Limits slice are all freshly allocated.
func (m *MockResponseWriter) ClusterInfo() *launcher.ClusterOptions {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.clusterInfo == nil {
		return nil
	}
	cp := *m.clusterInfo
	cp.Constraints = slices.Clone(m.clusterInfo.Constraints)
	cp.Queues = slices.Clone(m.clusterInfo.Queues)
	cp.Limits = slices.Clone(m.clusterInfo.Limits)
	cp.Configs = slices.Clone(m.clusterInfo.Configs)
	cp.Profiles = slices.Clone(m.clusterInfo.Profiles)
	for i := range cp.Profiles {
		cp.Profiles[i].Limits = slices.Clone(cp.Profiles[i].Limits)
	}
	cp.ImageOpt.Images = slices.Clone(m.clusterInfo.ImageOpt.Images)
	return &cp
}

// ConfigReloadResults returns a snapshot slice of every config reload result
// captured. The slice and its value-typed elements are independent of the
// writer.
func (m *MockResponseWriter) ConfigReloadResults() []ConfigReloadResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.configReloadResults)
}

// Reset clears all captured responses.
func (m *MockResponseWriter) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = []*api.Error{}
	m.jobs = [][]*api.Job{}
	m.controlResults = []ControlResult{}
	m.networks = []NetworkInfo{}
	m.clusterInfo = nil
	m.configReloadResults = []ConfigReloadResult{}
}

// MockStreamResponseWriter is a mock implementation of
// launcher.StreamResponseWriter that captures all streaming responses for
// test assertions. State is shared with the embedded MockResponseWriter
// under a single mutex (this writer intentionally does not declare its own).
// The aliasing contract documented on MockResponseWriter applies here too;
// the StatusUpdate, OutputChunk, and ResourceUtilData element types are
// pure value types so the stream-only accessors return fully independent
// snapshots.
type MockStreamResponseWriter struct {
	MockResponseWriter

	statuses      []StatusUpdate
	outputs       []OutputChunk
	resourceUtils []ResourceUtilData
	closed        bool
}

// StatusUpdate represents a job status update.
type StatusUpdate struct {
	ID         api.JobID
	Name       string
	Status     string
	StatusCode string
	Message    string
}

// OutputChunk represents a chunk of job output.
type OutputChunk struct {
	Output     string
	OutputType api.JobOutput
}

// ResourceUtilData represents resource utilization data.
type ResourceUtilData struct {
	CPUPercent  float64
	CPUTime     float64
	ResidentMem float64
	VirtualMem  float64
}

// NewMockStreamResponseWriter creates a new MockStreamResponseWriter.
func NewMockStreamResponseWriter() *MockStreamResponseWriter {
	return &MockStreamResponseWriter{
		MockResponseWriter: MockResponseWriter{
			errors:              []*api.Error{},
			jobs:                [][]*api.Job{},
			controlResults:      []ControlResult{},
			networks:            []NetworkInfo{},
			configReloadResults: []ConfigReloadResult{},
		},
		statuses:      []StatusUpdate{},
		outputs:       []OutputChunk{},
		resourceUtils: []ResourceUtilData{},
		closed:        false,
	}
}

// WriteJobStatus implements launcher.StreamResponseWriter.
func (m *MockStreamResponseWriter) WriteJobStatus(id api.JobID, name, status, statusCode, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses = append(m.statuses, StatusUpdate{
		ID:         id,
		Name:       name,
		Status:     status,
		StatusCode: statusCode,
		Message:    msg,
	})
	return nil
}

// WriteJobOutput implements launcher.StreamResponseWriter.
func (m *MockStreamResponseWriter) WriteJobOutput(output string, outputType api.JobOutput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outputs = append(m.outputs, OutputChunk{
		Output:     output,
		OutputType: outputType,
	})
	return nil
}

// WriteJobResourceUtil implements launcher.StreamResponseWriter.
func (m *MockStreamResponseWriter) WriteJobResourceUtil(cpuPercent float64, cpuTime float64,
	residentMem float64, virtualMem float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resourceUtils = append(m.resourceUtils, ResourceUtilData{
		CPUPercent:  cpuPercent,
		CPUTime:     cpuTime,
		ResidentMem: residentMem,
		VirtualMem:  virtualMem,
	})
	return nil
}

// Close implements launcher.StreamResponseWriter.
func (m *MockStreamResponseWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// Statuses returns a snapshot slice of every status update captured. The
// slice and its value-typed elements are independent of the writer.
func (m *MockStreamResponseWriter) Statuses() []StatusUpdate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.statuses)
}

// LastStatus returns the most recent status update, or nil if no statuses were written.
func (m *MockStreamResponseWriter) LastStatus() *StatusUpdate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.statuses) == 0 {
		return nil
	}
	cp := m.statuses[len(m.statuses)-1]
	return &cp
}

// StatusCount returns the number of status updates written.
func (m *MockStreamResponseWriter) StatusCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.statuses)
}

// Outputs returns a snapshot slice of every output chunk captured. The slice
// and its value-typed elements are independent of the writer.
func (m *MockStreamResponseWriter) Outputs() []OutputChunk {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.outputs)
}

// OutputCount returns the number of output chunks written.
func (m *MockStreamResponseWriter) OutputCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.outputs)
}

// CombinedOutput returns all output chunks concatenated into a single string.
func (m *MockStreamResponseWriter) CombinedOutput() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var combined string
	for _, chunk := range m.outputs {
		combined += chunk.Output
	}
	return combined
}

// ResourceUtils returns a snapshot slice of every resource utilization
// sample captured. The slice and its value-typed elements are independent
// of the writer.
func (m *MockStreamResponseWriter) ResourceUtils() []ResourceUtilData {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.resourceUtils)
}

// IsClosed reports whether Close has been called.
func (m *MockStreamResponseWriter) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// ResetStream clears all streaming-specific captured responses.
func (m *MockStreamResponseWriter) ResetStream() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses = []StatusUpdate{}
	m.outputs = []OutputChunk{}
	m.resourceUtils = []ResourceUtilData{}
	m.closed = false
}

// Reset clears all captured responses including base MockResponseWriter.
func (m *MockStreamResponseWriter) Reset() {
	m.MockResponseWriter.Reset()
	m.ResetStream()
}
