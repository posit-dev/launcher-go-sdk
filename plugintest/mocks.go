package plugintest

import (
	"errors"
	"sync"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/launcher"
)

// MockResponseWriter is a mock implementation of launcher.ResponseWriter that
// captures all responses for test assertions. All state is private; observe
// captured values via the accessor methods, which are safe to call
// concurrently with Write* methods.
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

// Errors returns a copy of every error written to the writer.
func (m *MockResponseWriter) Errors() []*api.Error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*api.Error, len(m.errors))
	copy(out, m.errors)
	return out
}

// HasError returns true if any errors were written.
func (m *MockResponseWriter) HasError() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.errors) > 0
}

// LastError returns the most recent error, or nil if no errors were written.
func (m *MockResponseWriter) LastError() *api.Error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.errors) == 0 {
		return nil
	}
	return m.errors[len(m.errors)-1]
}

// FirstError returns the first error, or nil if no errors were written.
func (m *MockResponseWriter) FirstError() *api.Error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.errors) == 0 {
		return nil
	}
	return m.errors[0]
}

// Jobs returns a copy of every job slice written via WriteJobs.
// The outer slice is freshly allocated; the inner slices are shared with the
// writer's recorded state (the mock never mutates already-recorded entries).
func (m *MockResponseWriter) Jobs() [][]*api.Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([][]*api.Job, len(m.jobs))
	copy(out, m.jobs)
	return out
}

// LastJobs returns the most recent job list, or nil if no jobs were written.
func (m *MockResponseWriter) LastJobs() []*api.Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.jobs) == 0 {
		return nil
	}
	return m.jobs[len(m.jobs)-1]
}

// AllJobs returns all jobs from all WriteJobs calls, flattened into a single slice.
func (m *MockResponseWriter) AllJobs() []*api.Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var all []*api.Job
	for _, jobs := range m.jobs {
		all = append(all, jobs...)
	}
	return all
}

// ControlResults returns a copy of every control result captured.
func (m *MockResponseWriter) ControlResults() []ControlResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ControlResult, len(m.controlResults))
	copy(out, m.controlResults)
	return out
}

// Networks returns a copy of every network response captured.
func (m *MockResponseWriter) Networks() []NetworkInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]NetworkInfo, len(m.networks))
	copy(out, m.networks)
	return out
}

// ClusterInfo returns a copy of the most recently captured cluster options,
// or nil if WriteClusterInfo has not been called.
func (m *MockResponseWriter) ClusterInfo() *launcher.ClusterOptions {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.clusterInfo == nil {
		return nil
	}
	cp := *m.clusterInfo
	return &cp
}

// ConfigReloadResults returns a copy of every config reload result captured.
func (m *MockResponseWriter) ConfigReloadResults() []ConfigReloadResult {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ConfigReloadResult, len(m.configReloadResults))
	copy(out, m.configReloadResults)
	return out
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
// launcher.StreamResponseWriter that captures all streaming responses for test
// assertions. State is shared with its embedded MockResponseWriter under a
// single mutex; all observation must use the accessor methods.
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

// Statuses returns a copy of every status update captured.
func (m *MockStreamResponseWriter) Statuses() []StatusUpdate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]StatusUpdate, len(m.statuses))
	copy(out, m.statuses)
	return out
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

// Outputs returns a copy of every output chunk captured.
func (m *MockStreamResponseWriter) Outputs() []OutputChunk {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]OutputChunk, len(m.outputs))
	copy(out, m.outputs)
	return out
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

// ResourceUtils returns a copy of every resource utilization sample captured.
func (m *MockStreamResponseWriter) ResourceUtils() []ResourceUtilData {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ResourceUtilData, len(m.resourceUtils))
	copy(out, m.resourceUtils)
	return out
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
