package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/posit-dev/launcher-go-sdk/api"
)

var ErrUnknownRequestType = fmt.Errorf("unknown request type")

// Request is a generic Launcher request message.
type Request interface {
	// ID returns the request ID, which must be sent in the corresponding
	// response or stream of responses.
	ID() uint64

	// Type returns the request type. It may panic if there is none.
	Type() int
}

func requestFromJSON(buf []byte) (Request, error) {
	var base BaseRequest
	if err := json.Unmarshal(buf, &base); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMsgInvalid, err)
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
	_ = json.Unmarshal(buf, req) // We know this can be unmarshalled.
	return req.(Request), nil
}

func requestForType(rt requestType) (interface{}, error) {
	switch rt {
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
	case requestMultiClusterInfo:
		return &MultiClusterInfoRequest{}, nil
	case requestSetLoadBalancerNodes:
		return &SetLoadBalancerNodesRequest{}, nil
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
	requestMultiClusterInfo     requestType = 17
	requestSetLoadBalancerNodes requestType = 201
)

// Base fields shared by all request types.
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

// The heartbeat request.
type HeartbeatRequest struct {
	BaseRequest
}

// The bootstrap request.
type BootstrapRequest struct {
	BaseRequest
	Version api.Version `json:"version"`
}

// Base fields shared by all request types that involve a user.
type BaseUserRequest struct {
	BaseRequest
	Username    string `json:"username"`
	ReqUsername string `json:"requestUsername"`
}

// The submit job request.
type SubmitJobRequest struct {
	BaseUserRequest
	Job *api.Job `json:"job"`
}

// Base fields shared by all request types that specify a job.
type BaseJobRequest struct {
	BaseUserRequest
	JobID        api.JobID `json:"jobId"`
	EncodedJobID string    `json:"encodedJobId"`
}

// The job state request.
type JobStateRequest struct {
	BaseJobRequest
	api.JobFilter
}

// Base fields shared by all streaming request types.
type BaseJobStreamRequest struct {
	BaseJobRequest
	Cancel bool `json:"cancel"`
}

// The job status stream request.
type JobStatusStreamRequest struct {
	BaseJobStreamRequest
}

// The control job request.
type ControlJobRequest struct {
	BaseJobRequest
	Operation api.JobOperation `json:"operation"`
}

// The job output stream request.
type JobOutputRequest struct {
	BaseJobStreamRequest
	Output api.JobOutput `json:"outputType"`
}

// The job resource utilization stream request.
type JobResourceUtilRequest struct {
	BaseJobStreamRequest
}

// The job network request.
type JobNetworkRequest struct {
	BaseJobRequest
}

// The cluster info request.
type ClusterInfoRequest struct {
	BaseUserRequest
}

// The set load balanced nodes request.
type SetLoadBalancerNodesRequest struct {
	BaseUserRequest
	Nodes []api.Node `json:"nodes"`
}

// Extension mechanism for supporting multiple clusters.
type MultiClusterInfoRequest struct {
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
	responseMultiClusterInfo     responseType = 17
	responseSetLoadBalancerNodes responseType = 201
)

type responseBase struct {
	MessageType responseType `json:"messageType"`
	RequestID   uint64       `json:"requestId"`
	ResponseID  uint64       `json:"responseId"`
}

// The heartbeat response.
type HeartbeatResponse = responseBase

// Create a new heartbeat response.
func NewHeartbeatResponse() *HeartbeatResponse {
	return &HeartbeatResponse{responseHeartbeat, 0, 0}
}

// The error response.
type ErrorResponse struct {
	responseBase
	Code api.ErrCode `json:"errorCode"`
	Msg  string      `json:"errorMessage"`
}

// Create a new error response.
func NewErrorResponse(requestID uint64, code api.ErrCode, msg string) *ErrorResponse {
	base := responseBase{responseError, requestID, 0}
	return &ErrorResponse{responseBase: base, Code: code, Msg: msg}
}

// The bootstrap response.
type BootstrapResponse struct {
	responseBase
	Version api.Version `json:"version"`
}

// Create a new bootstrap response.
func NewBootstrapResponse(requestID uint64, responseID uint64) *BootstrapResponse {
	base := responseBase{responseBootstrap, requestID, responseID}
	return &BootstrapResponse{base, api.APIVersion}
}

// The job state response.
type JobStateResponse struct {
	responseBase
	Jobs []*api.Job `json:"jobs"`
}

// Create a new job state response.
func NewJobStateResponse(requestID uint64, responseID uint64, jobs []*api.Job) *JobStateResponse {
	base := responseBase{responseJobState, requestID, responseID}
	if jobs == nil {
		// Ensure we never send null.
		jobs = []*api.Job{}
	}
	return &JobStateResponse{responseBase: base, Jobs: jobs}
}

// The job status stream response.
type JobStatusStreamResponse struct {
	responseBase
	Sequences []StreamSequence `json:"sequences"`
	ID        api.JobID        `json:"id"`
	Name      string           `json:"name"`
	Status    string           `json:"status"`
	Msg       string           `json:"statusMessage,omitempty"`
	Code      string           `json:"statusCode,omitempty"`
}

// Create a new job status stream response.
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

// The job output stream response.
type JobOutputResponse struct {
	responseBase
	SequenceID uint64 `json:"seqId"`
	Output     string `json:"output"`
	OutputType string `json:"outputType"`
	Complete   bool   `json:"complete"`
}

// Create a new job output stream response.
func NewJobOutputStreamResponse(requestID uint64, responseID uint64) *JobOutputResponse {
	base := responseBase{responseJobOutput, requestID, responseID}
	return &JobOutputResponse{responseBase: base}
}

// The job resource utilization stream response.
type JobResourceResponse struct {
	responseBase
	Sequences   []StreamSequence `json:"sequences"`
	CPUPercent  float64          `json:"cpuPercent,omitempty"`
	CPUTime     float64          `json:"cpuTime,omitempty"`
	VirtualMem  float64          `json:"virtualMemory,omitempty"`
	ResidentMem float64          `json:"residentMemory,omitempty"`
	Complete    bool             `json:"complete"`
}

// Create a new job resource utilization stream response.
func NewJobResourceResponse(responseID uint64, complete bool) *JobResourceResponse {
	base := responseBase{responseJobResourceUtil, 0, responseID}
	return &JobResourceResponse{
		responseBase: base,
		Sequences:    []StreamSequence{}, // Ensure we never send null.
		Complete:     complete,
	}
}

// Request/sequence structure for streaming requests.
type StreamSequence struct {
	RequestID  uint64 `json:"requestId"`
	SequenceID uint64 `json:"seqId"`
}

// The control job response.
type ControlJobResponse struct {
	responseBase
	Msg      string `json:"statusMessage"`
	Complete bool   `json:"operationComplete"`
}

// Create a new control job response.
func NewControlJobResponse(requestID uint64, responseID uint64, complete bool, msg string) *ControlJobResponse {
	base := responseBase{responseControlJob, requestID, responseID}
	return &ControlJobResponse{
		responseBase: base, Msg: msg, Complete: complete,
	}
}

// The job network response.
type JobNetworkResponse struct {
	responseBase
	Host      string   `json:"host"`
	Addr      []string `json:"ipAddresses"`
	Endpoints []string `json:"endpoints,omitempty"`
}

// Create a new job network response.
func NewJobNetworkResponse(requestID uint64, responseID uint64, host string, addr []string) *JobNetworkResponse {
	base := responseBase{responseJobNetwork, requestID, responseID}
	if addr == nil {
		addr = []string{} // Ensure we never send null.
	}
	return &JobNetworkResponse{responseBase: base, Host: host, Addr: addr}
}

// The cluster info response.
type ClusterInfoResponse struct {
	responseBase
	ClusterInfo
}

// Body of a cluster info response; reused for the multicluster extension.
type ClusterInfo struct {
	Containers   bool                      `json:"supportsContainers"`
	Configs      []api.JobConfig           `json:"config"`
	Constraints  []api.PlacementConstraint `json:"placementConstraints"`
	Queues       []string                  `json:"queues,omitempty"`
	DefaultQueue string                    `json:"defaultQueue,omitempty"`
	Limits       []api.ResourceLimit       `json:"resourceLimits"`
	Images       []string                  `json:"images,omitempty"`
	DefaultImage string                    `json:"defaultImage,omitempty"`
	AllowUnknown bool                      `json:"allowUnknownImages"`
	Profiles     []api.ResourceProfile     `json:"resourceProfiles"`
	HostNetwork  bool                      `json:"containersUseHostNetwork"`
	Name         string                    `json:"name,omitempty"`
}

// Create a new cluster info response.
func NewClusterInfoResponse(requestID uint64, responseID uint64, cluster ClusterInfo) *ClusterInfoResponse {
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

// Extension mechanism for supporting multiple clusters.
type MultiClusterInfoResponse struct {
	responseBase
	Clusters []ClusterInfo `json:"clusters"`
}

// Create a new multicluster info response.
func NewMultiClusterInfoResponse(requestID uint64, responseID uint64, clusters []ClusterInfo) *MultiClusterInfoResponse {
	base := responseBase{responseMultiClusterInfo, requestID, responseID}
	if clusters == nil {
		clusters = []ClusterInfo{} // Ensure we never send null.
	}
	return &MultiClusterInfoResponse{responseBase: base, Clusters: clusters}
}

// The set load balanced nodes response.
type SetLoadBalancerNodesResponse = responseBase

// Create a new set load balanced nodes response.
func NewSetLoadBalancerNodesResponse(requestID uint64, responseID uint64) *SetLoadBalancerNodesResponse {
	return &SetLoadBalancerNodesResponse{
		responseSetLoadBalancerNodes, requestID, responseID,
	}
}
