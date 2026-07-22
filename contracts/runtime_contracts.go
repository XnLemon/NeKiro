package contracts

import (
	"encoding/json"
	"time"
)

const (
	NorthboundInvocationAPIVersion        = "4"
	RouterInternalRuntimeAPIVersion       = "3"
	AgentRouterAPIVersion                 = "1"
	ControlPlaneInternalV3APIVersion      = "3"
	RuntimePlatformErrorSchemaVersion     = "4"
	RuntimeInvocationEventSchemaVersion   = "0.3"
	RuntimeResultStreamEventSchemaVersion = "2"

	RuntimeDeadlineMinimumMS int64 = 1
	RuntimeDeadlineMaximumMS int64 = 600000
	RuntimeByteLimitMinimum  int64 = 1
	RuntimeByteLimitMaximum  int64 = 2147483647

	ErrorCodePayloadTooLarge       PlatformErrorCode = "PAYLOAD_TOO_LARGE"
	ErrorCodeAgentAuthUnsupported  PlatformErrorCode = "AGENT_AUTH_UNSUPPORTED"
	ErrorCodeAgentResponseTooLarge PlatformErrorCode = "AGENT_RESPONSE_TOO_LARGE"
)

// NestedInvocationRequestV1 intentionally has no trusted caller, Workspace,
// correlation, Card-version, endpoint, or credential field.
type NestedInvocationRequestV1 struct {
	ParentInvocationID string          `json:"parentInvocationId"`
	TargetAgentID      string          `json:"targetAgentId"`
	Capability         string          `json:"capability"`
	Input              json.RawMessage `json:"input"`
	Stream             bool            `json:"stream"`
}

type DispatchInvocationRequestV3 struct {
	InvocationID string `json:"invocationId"`
	RootTaskID   string `json:"rootTaskId"`
	// ParentInvocationID is trusted in-process lineage for DispatchChild. It
	// is deliberately excluded from the Router Internal v3 root HTTP contract.
	ParentInvocationID string          `json:"-"`
	TraceID            TraceID         `json:"traceId"`
	Caller             Caller          `json:"caller"`
	WorkspaceID        string          `json:"workspaceId"`
	TargetAgentID      string          `json:"targetAgentId"`
	AgentCardVersion   string          `json:"agentCardVersion"`
	AgentReleaseID     string          `json:"agentReleaseId,omitempty"`
	AgentCardDigest    string          `json:"agentCardDigest,omitempty"`
	Capability         string          `json:"capability"`
	Input              json.RawMessage `json:"input"`
	Stream             bool            `json:"stream"`
}

type PreCorrelationPlatformErrorV4 struct {
	Code    PlatformErrorCode `json:"code"`
	Message string            `json:"message"`
	TraceID TraceID           `json:"traceId"`
}

type CorrelatedPlatformErrorV4 struct {
	Code         PlatformErrorCode `json:"code"`
	Message      string            `json:"message"`
	TraceID      TraceID           `json:"traceId"`
	InvocationID string            `json:"invocationId"`
	RootTaskID   string            `json:"rootTaskId"`
}

type PlatformErrorV4 = CorrelatedPlatformErrorV4

type InvocationEventV03 struct {
	SchemaVersion      string           `json:"schemaVersion"`
	EventID            string           `json:"eventId"`
	Sequence           int64            `json:"sequence"`
	OccurredAt         string           `json:"occurredAt"`
	Type               string           `json:"type"`
	Status             string           `json:"status"`
	InvocationID       string           `json:"invocationId"`
	RootTaskID         string           `json:"rootTaskId"`
	ParentInvocationID string           `json:"parentInvocationId,omitempty"`
	TraceID            TraceID          `json:"traceId"`
	Caller             Caller           `json:"caller"`
	WorkspaceID        string           `json:"workspaceId"`
	TargetAgentID      string           `json:"targetAgentId"`
	AgentCardVersion   string           `json:"agentCardVersion"`
	AgentReleaseID     string           `json:"agentReleaseId,omitempty"`
	AgentCardDigest    string           `json:"agentCardDigest,omitempty"`
	Capability         string           `json:"capability"`
	ChunkIndex         *int64           `json:"chunkIndex,omitempty"`
	ChunkBytes         *int64           `json:"chunkBytes,omitempty"`
	LatencyMS          *int64           `json:"latencyMs,omitempty"`
	Error              *PlatformErrorV4 `json:"error,omitempty"`
}

type InvocationResultStreamEventV2 struct {
	SchemaVersion string                `json:"schemaVersion"`
	Sequence      int64                 `json:"sequence"`
	Type          ResultStreamEventType `json:"type"`
	Status        string                `json:"status"`
	InvocationID  string                `json:"invocationId"`
	RootTaskID    string                `json:"rootTaskId"`
	TraceID       TraceID               `json:"traceId"`
	ChunkIndex    *int64                `json:"chunkIndex,omitempty"`
	Chunk         json.RawMessage       `json:"chunk,omitempty"`
	Error         *PlatformErrorV4      `json:"error,omitempty"`
}

type InvocationRecordV4 struct {
	InvocationID       string            `json:"invocationId"`
	RootTaskID         string            `json:"rootTaskId"`
	ParentInvocationID string            `json:"parentInvocationId,omitempty"`
	TraceID            TraceID           `json:"traceId"`
	Caller             Caller            `json:"caller"`
	WorkspaceID        string            `json:"workspaceId"`
	TargetAgentID      string            `json:"targetAgentId"`
	AgentCardVersion   string            `json:"agentCardVersion"`
	AgentReleaseID     string            `json:"agentReleaseId,omitempty"`
	AgentCardDigest    string            `json:"agentCardDigest,omitempty"`
	Capability         string            `json:"capability"`
	Status             string            `json:"status"`
	LatencyMS          *int64            `json:"latencyMs,omitempty"`
	ErrorCode          PlatformErrorCode `json:"errorCode,omitempty"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

type InvocationDetailResponseV4 struct {
	Invocation InvocationRecordV4   `json:"invocation"`
	Events     []InvocationEventV03 `json:"events"`
}

type TraceResponseV4 struct {
	TraceID     TraceID              `json:"traceId"`
	Invocations []InvocationRecordV4 `json:"invocations"`
}

// ResolveInstalledVersionRequest is the Control Plane Internal v3 request for
// resolving the deterministic installed Agent Card version from the enabled
// Installation. It intentionally has no version field; the Control Plane
// derives it from the pinned installedVersion.
type ResolveInstalledVersionRequest struct {
	InvocationID string  `json:"invocationId"`
	RootTaskID   string  `json:"rootTaskId"`
	TraceID      TraceID `json:"traceId"`
	WorkspaceID  string  `json:"workspaceId"`
	AgentID      string  `json:"agentId"`
	Capability   string  `json:"capability"`
}

// ResolveInstalledVersionResponse carries the exact pinned installedVersion.
type ResolveInstalledVersionResponse struct {
	Version         string `json:"version"`
	ReleaseID       string `json:"releaseId,omitempty"`
	AgentCardDigest string `json:"agentCardDigest,omitempty"`
}
