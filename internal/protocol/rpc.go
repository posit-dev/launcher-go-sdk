package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/posit-dev/launcher-go-sdk/api"
)

// ErrUnknownRequestType is returned when an unknown request type is encountered.
var ErrUnknownRequestType = fmt.Errorf("unknown request type")

// Request is a generic Launcher request message.
type Request interface {
	// ID returns the request ID, which must be sent in the corresponding
	// response or stream of responses.
	ID() uint64

	// Type returns the request type. It may panic if there is none.
	Type() int
}

// RequestFromJSON parses a JSON-encoded request message into the appropriate
// concrete request type.
func RequestFromJSON(buf []byte) (Request, error) {
	var base BaseRequest
	if err := json.Unmarshal(buf, &base); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMsgInvalid, err) //nolint:errorlint // intentionally wrapping only the sentinel error
	}
	if !base.Valid() {
		return nil, fmt.Errorf("%w: %s", ErrMsgInvalid, string(buf))
	}
	rt := *base.MessageType
	if rt == requestHeartbeat {
		// We don't need to unmarshall again.
		return &HeartbeatRequest{base}, nil
	}
	req, err := requestForType(rt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(buf, req); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMsgInvalid, err) //nolint:errorlint // intentionally wrapping only the sentinel error
	}
	r, ok := req.(Request)
	if !ok {
		panic("requestForType returned a non-Request type")
	}
	return r, nil
}

func requestForType(rt requestType) (interface{}, error) {
	switch rt {
	case requestHeartbeat:
		// Heartbeat is handled before requestForType is called; reaching
		// here indicates a programming error.
		return nil, fmt.Errorf("%w: unexpected heartbeat in requestForType", ErrUnknownRequestType)
	case requestBootstrap:
		return &BootstrapRequest{}, nil
	case requestSubmitJob:
		return &SubmitJobRequest{}, nil
	case requestJob:
		return &JobStateRequest{}, nil
	case requestJobStatus:
		return &JobStatusStreamRequest{}, nil
	case requestControlJob:
		return &ControlJobRequest{}, nil
	case requestJobOutput:
		return &JobOutputRequest{}, nil
	case requestJobResourceUtil:
		return &JobResourceUtilRequest{}, nil
	case requestJobNetwork:
		return &JobNetworkRequest{}, nil
	case requestClusterInfo:
		return &ClusterInfoRequest{}, nil
	case requestSetLoadBalancerNodes:
		return &SetLoadBalancerNodesRequest{}, nil
	case requestConfigReload:
		return &ConfigReloadRequest{}, nil
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownRequestType, rt)
	}
}

type requestType int

const (
	requestHeartbeat requestType = iota
	requestBootstrap
	requestSubmitJob
	requestJob
	requestJobStatus
	requestControlJob
	requestJobOutput
	requestJobResourceUtil
	requestJobNetwork
	requestClusterInfo
	requestSetLoadBalancerNodes requestType = 201
	requestConfigReload         requestType = 202
)

// BaseRequest contains base fields shared by all request types.
type BaseRequest struct {
	MessageType *requestType `json:"messageType"`
	RequestID   *uint64      `json:"requestId"`
}

// Type partially satisfies the Request interface. It returns the request type
// or panics if there is none.
func (r BaseRequest) Type() int {
	return int(*r.MessageType)
}

// ID satisfies the Request interface. It returns the request ID or panics if
// there is none.
func (r BaseRequest) ID() uint64 {
	return *r.RequestID
}

// Valid checks whether the request looks like an actual Launcher request
// message -- namely, whether it includes the required fields.
func (r BaseRequest) Valid() bool {
	return r.MessageType != nil && r.RequestID != nil
}

// HeartbeatRequest is the heartbeat request.
type HeartbeatRequest struct {
	BaseRequest
}

// BootstrapRequest is the bootstrap request.
type BootstrapRequest struct {
	BaseRequest
	Version api.Version `json:"version"`
}

// BaseUserRequest contains base fields shared by all request types that involve a user.
type BaseUserRequest struct {
	BaseRequest
	Username    string `json:"username"`
	ReqUsername string `json:"requestUsername"`
}

// SubmitJobRequest is the submit job request.
type SubmitJobRequest struct {
	BaseUserRequest
	Job *api.Job `json:"job"`
}

// BaseJobRequest contains base fields shared by all request types that specify a job.
type BaseJobRequest struct {
	BaseUserRequest
	JobID        api.JobID `json:"jobId"`
	EncodedJobID string    `json:"encodedJobId"`
}

// JobStateRequest is the job state request.
type JobStateRequest struct {
	BaseJobRequest
	api.JobFilter
}

// BaseJobStreamRequest contains base fields shared by all streaming request types.
type BaseJobStreamRequest struct {
	BaseJobRequest
	Cancel bool `json:"cancel"`
}

// JobStatusStreamRequest is the job status stream request.
type JobStatusStreamRequest struct {
	BaseJobStreamRequest
}

// ControlJobRequest is the control job request.
type ControlJobRequest struct {
	BaseJobRequest
	Operation api.JobOperation `json:"operation"`
}

// JobOutputRequest is the job output stream request.
type JobOutputRequest struct {
	BaseJobStreamRequest
	Output api.JobOutput `json:"outputType"`
}

// JobResourceUtilRequest is the job resource utilization stream request.
type JobResourceUtilRequest struct {
	BaseJobStreamRequest
}

// JobNetworkRequest is the job network request.
type JobNetworkRequest struct {
	BaseJobRequest
}

// ClusterInfoRequest is the cluster info request.
type ClusterInfoRequest struct {
	BaseUserRequest
}

// SetLoadBalancerNodesRequest is the set load balanced nodes request.
type SetLoadBalancerNodesRequest struct {
	BaseUserRequest
	Nodes []api.Node `json:"nodes"`
}

// ConfigReloadRequest is the config reload request.
type ConfigReloadRequest struct {
	BaseUserRequest
}

type responseType int

const (
	responseError responseType = iota - 1
	responseHeartbeat
	responseBootstrap
	responseJobState
	responseJobStatus
	responseControlJob
	responseJobOutput
	responseJobResourceUtil
	responseJobNetwork
	responseClusterInfo
	responseSetLoadBalancerNodes responseType = 201
	responseConfigReload         responseType = 202
	responseMetrics              responseType = 203
)

type responseBase struct {
	MessageType responseType `json:"messageType"`
	RequestID   uint64       `json:"requestId"`
	ResponseID  uint64       `json:"responseId"`
}

// HeartbeatResponse is the heartbeat response.
type HeartbeatResponse = responseBase

// NewHeartbeatResponse creates a new heartbeat response.
func NewHeartbeatResponse() *HeartbeatResponse {
	return &HeartbeatResponse{responseHeartbeat, 0, 0}
}

// ErrorResponse is the error response.
type ErrorResponse struct {
	responseBase
	Code api.ErrCode `json:"errorCode"`
	Msg  string      `json:"errorMessage"`
}

// NewErrorResponse creates a new error response.
func NewErrorResponse(requestID uint64, code api.ErrCode, msg string) *ErrorResponse {
	base := responseBase{responseError, requestID, 0}
	return &ErrorResponse{responseBase: base, Code: code, Msg: msg}
}

// BootstrapResponse is the bootstrap response.
type BootstrapResponse struct {
	responseBase
	Version api.Version `json:"version"`
}

// NewBootstrapResponse creates a new bootstrap response.
func NewBootstrapResponse(requestID, responseID uint64) *BootstrapResponse {
	base := responseBase{responseBootstrap, requestID, responseID}
	return &BootstrapResponse{base, api.APIVersion}
}

// JobStateResponse is the job state response.
type JobStateResponse struct {
	responseBase
	Jobs []*api.Job `json:"jobs"`
}

// NewJobStateResponse creates a new job state response.
func NewJobStateResponse(requestID, responseID uint64, jobs []*api.Job) *JobStateResponse {
	base := responseBase{responseJobState, requestID, responseID}
	if jobs == nil {
		// Ensure we never send null.
		jobs = []*api.Job{}
	}
	return &JobStateResponse{responseBase: base, Jobs: jobs}
}

// JobStatusStreamResponse is the job status stream response.
type JobStatusStreamResponse struct {
	responseBase
	Sequences []StreamSequence `json:"sequences"`
	ID        api.JobID        `json:"id"`
	Name      string           `json:"name"`
	Status    string           `json:"status"`
	Msg       string           `json:"statusMessage,omitempty"`
	Code      string           `json:"statusCode,omitempty"`
}

// NewJobStatusStreamResponse creates a new job status stream response.
func NewJobStatusStreamResponse(responseID uint64, id, status, msg string) *JobStatusStreamResponse {
	base := responseBase{responseJobStatus, 0, responseID}
	return &JobStatusStreamResponse{
		responseBase: base,
		Sequences:    []StreamSequence{}, // Ensure we never send null.
		ID:           api.JobID(id),
		Status:       status,
		Msg:          msg,
	}
}

// JobOutputResponse is the job output stream response.
type JobOutputResponse struct {
	responseBase
	SequenceID uint64 `json:"seqId"`
	Output     string `json:"output"`
	OutputType string `json:"outputType"`
	Complete   bool   `json:"complete"`
}

// NewJobOutputStreamResponse creates a new job output stream response.
func NewJobOutputStreamResponse(requestID, responseID uint64) *JobOutputResponse {
	base := responseBase{responseJobOutput, requestID, responseID}
	return &JobOutputResponse{responseBase: base}
}

// JobResourceResponse is the job resource utilization stream response.
type JobResourceResponse struct {
	responseBase
	Sequences   []StreamSequence `json:"sequences"`
	CPUPercent  float64          `json:"cpuPercent,omitempty"`
	CPUTime     float64          `json:"cpuTime,omitempty"`
	VirtualMem  float64          `json:"virtualMemory,omitempty"`
	ResidentMem float64          `json:"residentMemory,omitempty"`
	Complete    bool             `json:"complete"`
}

// NewJobResourceResponse creates a new job resource utilization stream response.
func NewJobResourceResponse(responseID uint64, complete bool) *JobResourceResponse {
	base := responseBase{responseJobResourceUtil, 0, responseID}
	return &JobResourceResponse{
		responseBase: base,
		Sequences:    []StreamSequence{}, // Ensure we never send null.
		Complete:     complete,
	}
}

// StreamSequence is a request/sequence structure for streaming requests.
type StreamSequence struct {
	RequestID  uint64 `json:"requestId"`
	SequenceID uint64 `json:"seqId"`
}

// ControlJobResponse is the control job response.
type ControlJobResponse struct {
	responseBase
	Msg      string `json:"statusMessage"`
	Complete bool   `json:"operationComplete"`
}

// NewControlJobResponse creates a new control job response.
func NewControlJobResponse(requestID, responseID uint64, complete bool, msg string) *ControlJobResponse {
	base := responseBase{responseControlJob, requestID, responseID}
	return &ControlJobResponse{
		responseBase: base, Msg: msg, Complete: complete,
	}
}

// JobNetworkResponse is the job network response.
type JobNetworkResponse struct {
	responseBase
	Host      string   `json:"host"`
	Addr      []string `json:"ipAddresses"`
	Endpoints []string `json:"endpoints,omitempty"`
}

// NewJobNetworkResponse creates a new job network response.
func NewJobNetworkResponse(requestID, responseID uint64, host string, addr []string) *JobNetworkResponse {
	base := responseBase{responseJobNetwork, requestID, responseID}
	if addr == nil {
		addr = []string{} // Ensure we never send null.
	}
	return &JobNetworkResponse{responseBase: base, Host: host, Addr: addr}
}

// ClusterInfoResponse is the cluster info response.
type ClusterInfoResponse struct {
	responseBase
	ClusterInfo
}

// ClusterInfo is the body of a cluster info response.
type ClusterInfo struct {
	Containers     bool                      `json:"supportsContainers"`
	InitContainers bool                      `json:"supportsInitContainers"`
	Configs        []api.JobConfig           `json:"config"`
	Constraints    []api.PlacementConstraint `json:"placementConstraints"`
	Queues         []string                  `json:"queues,omitempty"`
	DefaultQueue   string                    `json:"defaultQueue,omitempty"`
	Limits         []api.ResourceLimit       `json:"resourceLimits"`
	Images         []string                  `json:"images,omitempty"`
	DefaultImage   string                    `json:"defaultImage,omitempty"`
	AllowUnknown   bool                      `json:"allowUnknownImages"`
	Profiles       []api.ResourceProfile     `json:"resourceProfiles"`
	HostNetwork    bool                      `json:"containersUseHostNetwork"`
}

// NewClusterInfoResponse creates a new cluster info response.
func NewClusterInfoResponse(requestID, responseID uint64, cluster ClusterInfo) *ClusterInfoResponse {
	base := responseBase{responseClusterInfo, requestID, responseID}
	if cluster.Configs == nil {
		cluster.Configs = []api.JobConfig{}
	}
	if cluster.Constraints == nil {
		cluster.Constraints = []api.PlacementConstraint{}
	}
	if cluster.Limits == nil {
		cluster.Limits = []api.ResourceLimit{}
	}
	if cluster.Profiles == nil {
		cluster.Profiles = []api.ResourceProfile{}
	}
	return &ClusterInfoResponse{responseBase: base, ClusterInfo: cluster}
}

// SetLoadBalancerNodesResponse is the set load balanced nodes response.
type SetLoadBalancerNodesResponse = responseBase

// NewSetLoadBalancerNodesResponse creates a new set load balanced nodes response.
func NewSetLoadBalancerNodesResponse(requestID, responseID uint64) *SetLoadBalancerNodesResponse {
	return &SetLoadBalancerNodesResponse{
		responseSetLoadBalancerNodes, requestID, responseID,
	}
}

// ConfigReloadResponse is the config reload response.
type ConfigReloadResponse struct {
	responseBase
	ErrorType    api.ConfigReloadErrorType `json:"errorType"`
	ErrorMessage string                    `json:"errorMessage"`
}

// NewConfigReloadResponse creates a new config reload response.
func NewConfigReloadResponse(requestID, responseID uint64, errorType api.ConfigReloadErrorType, errorMessage string) *ConfigReloadResponse {
	base := responseBase{responseConfigReload, requestID, responseID}
	return &ConfigReloadResponse{responseBase: base, ErrorType: errorType, ErrorMessage: errorMessage}
}

// HistogramSample is a portable snapshot of a Prometheus histogram's data,
// containing non-cumulative per-bucket counts and the sum of observed values.
// Unlike Prometheus's cumulative bucket format, each element in Buckets
// represents only the count for that specific bucket range, matching the
// format expected by the Launcher and the C++ SDK.
type HistogramSample struct {
	Buckets []float64 `json:"buckets"`
	Sum     float64   `json:"sum"`
}

// MetricsResponse is a periodic metrics report sent by the plugin to the
// Launcher. Unlike other responses, this is unsolicited — it is not sent in
// response to any request. Both requestId and responseId are zero.
type MetricsResponse struct {
	responseBase
	UptimeSeconds                   uint64           `json:"uptimeSeconds"`
	MemoryUsageBytes                uint64           `json:"memoryUsageBytes"`
	ClusterInteractionLatencySample *HistogramSample `json:"clusterInteractionLatencySample,omitempty"`
}

// NewMetricsResponse creates a new metrics response. The requestId and
// responseId are both zero because this message is not a response to a
// request.
func NewMetricsResponse(uptimeSeconds, memoryBytes uint64, latency *HistogramSample) *MetricsResponse {
	base := responseBase{responseMetrics, 0, 0}
	return &MetricsResponse{
		responseBase:                    base,
		UptimeSeconds:                   uptimeSeconds,
		MemoryUsageBytes:                memoryBytes,
		ClusterInteractionLatencySample: latency,
	}
}
