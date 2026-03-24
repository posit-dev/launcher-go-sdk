package plugintest

import (
	"errors"
	"sync"

	"github.com/posit-dev/launcher-go-sdk/api"
	"github.com/posit-dev/launcher-go-sdk/launcher"
)

// MockResponseWriter is a mock implementation of launcher.ResponseWriter that
// captures all responses for test assertions.
type MockResponseWriter struct {
	mu sync.Mutex

	// Errors contains all errors written via WriteError or WriteErrorf.
	Errors []*api.Error

	// Jobs contains all job lists written via WriteJobs.
	Jobs [][]*api.Job

	// ControlResults contains all control job results written via WriteControlJob.
	ControlResults []ControlResult

	// Networks contains all job network responses written via WriteJobNetwork.
	Networks []NetworkInfo

	// ClusterInfo contains the cluster info written via WriteClusterInfo.
	ClusterInfo *launcher.ClusterOptions

	// ConfigReloadResults contains all config reload responses written via WriteConfigReload.
	ConfigReloadResults []ConfigReloadResult
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

// NewMockResponseWriter creates a new MockResponseWriter.
func NewMockResponseWriter() *MockResponseWriter {
	return &MockResponseWriter{
		Errors:              []*api.Error{},
		Jobs:                [][]*api.Job{},
		ControlResults:      []ControlResult{},
		Networks:            []NetworkInfo{},
		ConfigReloadResults: []ConfigReloadResult{},
	}
}

// WriteErrorf implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteErrorf(code api.ErrCode, format string, a ...interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Errors = append(m.Errors, api.Errorf(code, format, a...))
	return nil
}

// WriteError implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteError(err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		m.Errors = append(m.Errors, apiErr)
	} else {
		m.Errors = append(m.Errors, &api.Error{Code: api.CodeUnknown, Msg: err.Error()})
	}
	return nil
}

// WriteJobs implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteJobs(jobs []*api.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Jobs = append(m.Jobs, jobs)
	return nil
}

// WriteControlJob implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteControlJob(complete bool, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ControlResults = append(m.ControlResults, ControlResult{
		Complete: complete,
		Message:  msg,
	})
	return nil
}

// WriteJobNetwork implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteJobNetwork(host string, addr []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Networks = append(m.Networks, NetworkInfo{
		Host:      host,
		Addresses: addr,
	})
	return nil
}

// WriteClusterInfo implements launcher.ResponseWriter.
func (m *MockResponseWriter) WriteClusterInfo(opts launcher.ClusterOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ClusterInfo = &opts
	return nil
}

// ConfigReloadResult represents a config reload operation result.
type ConfigReloadResult struct {
	ErrorType    api.ConfigReloadErrorType
	ErrorMessage string
}

// WriteConfigReload captures config reload responses for test assertions.
func (m *MockResponseWriter) WriteConfigReload(errorType api.ConfigReloadErrorType, errorMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConfigReloadResults = append(m.ConfigReloadResults, ConfigReloadResult{
		ErrorType:    errorType,
		ErrorMessage: errorMessage,
	})
	return nil
}

// HasError returns true if any errors were written.
func (m *MockResponseWriter) HasError() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Errors) > 0
}

// LastError returns the most recent error, or nil if no errors were written.
func (m *MockResponseWriter) LastError() *api.Error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Errors) == 0 {
		return nil
	}
	return m.Errors[len(m.Errors)-1]
}

// FirstError returns the first error, or nil if no errors were written.
func (m *MockResponseWriter) FirstError() *api.Error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Errors) == 0 {
		return nil
	}
	return m.Errors[0]
}

// LastJobs returns the most recent job list, or nil if no jobs were written.
func (m *MockResponseWriter) LastJobs() []*api.Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Jobs) == 0 {
		return nil
	}
	return m.Jobs[len(m.Jobs)-1]
}

// AllJobs returns all jobs from all WriteJobs calls, flattened into a single slice.
func (m *MockResponseWriter) AllJobs() []*api.Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []*api.Job
	for _, jobs := range m.Jobs {
		all = append(all, jobs...)
	}
	return all
}

// Reset clears all captured responses.
func (m *MockResponseWriter) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Errors = []*api.Error{}
	m.Jobs = [][]*api.Job{}
	m.ControlResults = []ControlResult{}
	m.Networks = []NetworkInfo{}
	m.ClusterInfo = nil
	m.ConfigReloadResults = []ConfigReloadResult{}
}

// MockStreamResponseWriter is a mock implementation of launcher.StreamResponseWriter
// that captures all streaming responses for test assertions.
type MockStreamResponseWriter struct {
	MockResponseWriter
	mu sync.Mutex

	// Statuses contains all job status updates written via WriteJobStatus.
	Statuses []StatusUpdate

	// Outputs contains all output chunks written via WriteJobOutput.
	Outputs []OutputChunk

	// ResourceUtils contains all resource utilization data written via WriteJobResourceUtil.
	ResourceUtils []ResourceUtilData

	// Closed indicates whether Close() was called.
	Closed bool
}

// StatusUpdate represents a job status update.
type StatusUpdate struct {
	ID      api.JobID
	Name    string
	Status  string
	Message string
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
			Errors:         []*api.Error{},
			Jobs:           [][]*api.Job{},
			ControlResults: []ControlResult{},
			Networks:       []NetworkInfo{},
		},
		Statuses:      []StatusUpdate{},
		Outputs:       []OutputChunk{},
		ResourceUtils: []ResourceUtilData{},
		Closed:        false,
	}
}

// WriteJobStatus implements launcher.StreamResponseWriter.
func (m *MockStreamResponseWriter) WriteJobStatus(id api.JobID, name, status, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Statuses = append(m.Statuses, StatusUpdate{
		ID:      id,
		Name:    name,
		Status:  status,
		Message: msg,
	})
	return nil
}

// WriteJobOutput implements launcher.StreamResponseWriter.
func (m *MockStreamResponseWriter) WriteJobOutput(output string, outputType api.JobOutput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Outputs = append(m.Outputs, OutputChunk{
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
	m.ResourceUtils = append(m.ResourceUtils, ResourceUtilData{
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
	m.Closed = true
	return nil
}

// LastStatus returns the most recent status update, or nil if no statuses were written.
func (m *MockStreamResponseWriter) LastStatus() *StatusUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Statuses) == 0 {
		return nil
	}
	return &m.Statuses[len(m.Statuses)-1]
}

// StatusCount returns the number of status updates written.
func (m *MockStreamResponseWriter) StatusCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Statuses)
}

// OutputCount returns the number of output chunks written.
func (m *MockStreamResponseWriter) OutputCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Outputs)
}

// CombinedOutput returns all output chunks concatenated into a single string.
func (m *MockStreamResponseWriter) CombinedOutput() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var combined string
	for _, chunk := range m.Outputs {
		combined += chunk.Output
	}
	return combined
}

// ResetStream clears all streaming-specific captured responses.
func (m *MockStreamResponseWriter) ResetStream() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Statuses = []StatusUpdate{}
	m.Outputs = []OutputChunk{}
	m.ResourceUtils = []ResourceUtilData{}
	m.Closed = false
}

// Reset clears all captured responses including base MockResponseWriter.
func (m *MockStreamResponseWriter) Reset() {
	m.MockResponseWriter.Reset()
	m.ResetStream()
}
