package contracts

import "encoding/json"

const (
	NorthboundInvocationAPIVersion        = "4"
	RouterInternalRuntimeAPIVersion       = "3"
	AgentRouterAPIVersion                 = "1"
	RuntimePlatformErrorSchemaVersion     = "4"
	RuntimeInvocationEventSchemaVersion   = "0.3"
	RuntimeResultStreamEventSchemaVersion = "2"

	RuntimeDeadlineMinimumMS int64 = 1
	RuntimeDeadlineMaximumMS int64 = 600000
	RuntimeByteLimitMinimum  int64 = 1
	RuntimeByteLimitMaximum  int64 = 2147483647

	ErrorCodePayloadTooLarge      PlatformErrorCode = "PAYLOAD_TOO_LARGE"
	ErrorCodeAgentAuthUnsupported PlatformErrorCode = "AGENT_AUTH_UNSUPPORTED"
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
	InvocationID     string          `json:"invocationId"`
	RootTaskID       string          `json:"rootTaskId"`
	TraceID          TraceID         `json:"traceId"`
	Caller           Caller          `json:"caller"`
	WorkspaceID      string          `json:"workspaceId"`
	TargetAgentID    string          `json:"targetAgentId"`
	AgentCardVersion string          `json:"agentCardVersion"`
	Capability       string          `json:"capability"`
	Input            json.RawMessage `json:"input"`
	Stream           bool            `json:"stream"`
}

type PlatformErrorV4 struct {
	Code         PlatformErrorCode `json:"code"`
	Message      string            `json:"message"`
	TraceID      TraceID           `json:"traceId"`
	InvocationID string            `json:"invocationId,omitempty"`
	RootTaskID   string            `json:"rootTaskId,omitempty"`
}

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
