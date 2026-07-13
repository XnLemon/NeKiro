package contracts

import (
	"fmt"
	"regexp"
	"time"
)

const (
	AgentCardSchemaVersion         = "0.2"
	InvocationEventSchemaVersion   = InvocationEventV02SchemaVersion
	PlatformErrorSchemaVersion     = "2"
	A2AProfileSchemaVersion        = A2AProfileSchemaVersionV02
	A2AProtocolVersion             = A2AProfileProtocolVersion
	NorthboundAPIVersion           = "2"
	ControlPlaneInternalAPIVersion = "1"
	RouterInternalAPIVersion       = "2"
)

var safeIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._:-]{0,127})$`)

type TraceID string

func ParseTraceID(value string) (TraceID, error) {
	if !safeIdentifierPattern.MatchString(value) {
		return "", fmt.Errorf("invalid trace id")
	}
	return TraceID(value), nil
}

type JSONSchema map[string]any

type AgentOwner struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type AgentProtocol struct {
	Type      string `json:"type"`
	Version   string `json:"version"`
	Transport string `json:"transport"`
	Endpoint  string `json:"endpoint"`
}

type AgentSkill struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	InputSchema         JSONSchema `json:"inputSchema"`
	OutputSchema        JSONSchema `json:"outputSchema"`
	RequiredPermissions []string   `json:"requiredPermissions"`
}

type AgentAuthentication struct {
	Type string `json:"type"`
}

type PermissionDeclaration struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type AgentLimits struct {
	TimeoutMS      int64 `json:"timeoutMs"`
	MaxInputBytes  int64 `json:"maxInputBytes"`
	MaxOutputBytes int64 `json:"maxOutputBytes"`
	Streaming      bool  `json:"streaming"`
}

type AgentCard struct {
	SchemaVersion  string                  `json:"schemaVersion"`
	AgentID        string                  `json:"agentId"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description"`
	Owner          AgentOwner              `json:"owner"`
	Version        string                  `json:"version"`
	Protocol       AgentProtocol           `json:"protocol"`
	Skills         []AgentSkill            `json:"skills"`
	Authentication AgentAuthentication     `json:"authentication"`
	Permissions    []PermissionDeclaration `json:"permissions"`
	Limits         AgentLimits             `json:"limits"`
}

type CatalogEntry struct {
	Card              AgentCard  `json:"card"`
	PublicationStatus string     `json:"publicationStatus"`
	RegisteredAt      time.Time  `json:"registeredAt"`
	PublishedAt       *time.Time `json:"publishedAt,omitempty"`
}

type RegisterAgentRequest struct {
	Card AgentCard `json:"card"`
}

type SearchAgentsQuery struct {
	Query      *string `json:"query,omitempty"`
	Capability *string `json:"capability,omitempty"`
	OwnerID    *string `json:"ownerId,omitempty"`
	Limit      *int    `json:"limit,omitempty"`
	Cursor     *string `json:"cursor,omitempty"`
}

type SearchAgentsResponse struct {
	Items      []CatalogEntry `json:"items"`
	NextCursor *string        `json:"nextCursor,omitempty"`
}

type Installation struct {
	InstallationID      string    `json:"installationId"`
	WorkspaceID         string    `json:"workspaceId"`
	AgentID             string    `json:"agentId"`
	VersionConstraint   string    `json:"versionConstraint"`
	InstalledVersion    string    `json:"installedVersion"`
	AcceptedPermissions []string  `json:"acceptedPermissions"`
	Status              string    `json:"status"`
	InstalledAt         time.Time `json:"installedAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type InstallAgentRequest struct {
	AgentID             string   `json:"agentId"`
	VersionConstraint   string   `json:"versionConstraint"`
	AcceptedPermissions []string `json:"acceptedPermissions"`
}

type UpdateInstallationRequest struct {
	Status string `json:"status"`
}

type Caller struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type PlatformErrorCode string

const (
	ErrorCodeValidationError      PlatformErrorCode = "VALIDATION_ERROR"
	ErrorCodeUnauthenticated      PlatformErrorCode = "UNAUTHENTICATED"
	ErrorCodeForbidden            PlatformErrorCode = "FORBIDDEN"
	ErrorCodeNotFound             PlatformErrorCode = "NOT_FOUND"
	ErrorCodeConflict             PlatformErrorCode = "CONFLICT"
	ErrorCodeAgentNotInstalled    PlatformErrorCode = "AGENT_NOT_INSTALLED"
	ErrorCodeAgentDisabled        PlatformErrorCode = "AGENT_DISABLED"
	ErrorCodeCapabilityNotAllowed PlatformErrorCode = "CAPABILITY_NOT_ALLOWED"
	ErrorCodeRouteNotFound        PlatformErrorCode = "ROUTE_NOT_FOUND"
	ErrorCodeA2AProtocol          PlatformErrorCode = "A2A_PROTOCOL_ERROR"
	ErrorCodeAgentUnavailable     PlatformErrorCode = "AGENT_UNAVAILABLE"
	ErrorCodeAgentExecutionFailed PlatformErrorCode = "AGENT_EXECUTION_FAILED"
	ErrorCodeDependency           PlatformErrorCode = "DEPENDENCY_ERROR"
	ErrorCodeTimeout              PlatformErrorCode = "TIMEOUT"
	ErrorCodeCanceled             PlatformErrorCode = "CANCELED"
	ErrorCodeInternal             PlatformErrorCode = "INTERNAL_ERROR"
)

type PlatformError = PlatformErrorV2

func NewPlatformError(code PlatformErrorCode, traceID TraceID) (PlatformError, error) {
	return NewPlatformErrorV2(code, traceID)
}

type InvocationEvent = InvocationEventV02

type InvokeAgentRequest struct {
	AgentID    string         `json:"agentId"`
	Capability string         `json:"capability"`
	Input      map[string]any `json:"input"`
	Stream     bool           `json:"stream"`
}

type InvocationRecord struct {
	InvocationID       string            `json:"invocationId"`
	RootTaskID         string            `json:"rootTaskId"`
	ParentInvocationID string            `json:"parentInvocationId,omitempty"`
	TraceID            TraceID           `json:"traceId"`
	Caller             Caller            `json:"caller"`
	WorkspaceID        string            `json:"workspaceId"`
	TargetAgentID      string            `json:"targetAgentId"`
	AgentCardVersion   string            `json:"agentCardVersion"`
	Capability         string            `json:"capability"`
	Status             string            `json:"status"`
	LatencyMS          *int64            `json:"latencyMs,omitempty"`
	ErrorCode          PlatformErrorCode `json:"errorCode,omitempty"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

type InvocationDetailResponse struct {
	Invocation InvocationRecord  `json:"invocation"`
	Events     []InvocationEvent `json:"events"`
}

type TraceResponse struct {
	TraceID     TraceID            `json:"traceId"`
	Invocations []InvocationRecord `json:"invocations"`
}

type RouterEventEnvelope = RouterEventEnvelopeV02

type A2ASDK struct {
	Module  string `json:"module"`
	Version string `json:"version"`
}

type A2AContextHeaders struct {
	TraceID            string `json:"traceId"`
	InvocationID       string `json:"invocationId"`
	RootTaskID         string `json:"rootTaskId"`
	ParentInvocationID string `json:"parentInvocationId"`
	WorkspaceID        string `json:"workspaceId"`
}

type A2AProfile = A2AProfileV02

type ResolveAgentRequest = ResolveAgentRequestV1

type ResolvedInstallation struct {
	InstallationID      string   `json:"installationId"`
	WorkspaceID         string   `json:"workspaceId"`
	AgentID             string   `json:"agentId"`
	InstalledVersion    string   `json:"installedVersion"`
	AcceptedPermissions []string `json:"acceptedPermissions"`
	Status              string   `json:"status"`
}

type ResolveAgentResponse struct {
	Card         AgentCard            `json:"card"`
	Installation ResolvedInstallation `json:"installation"`
}

type DispatchInvocationRequest struct {
	InvocationID       string         `json:"invocationId"`
	RootTaskID         string         `json:"rootTaskId"`
	ParentInvocationID string         `json:"parentInvocationId,omitempty"`
	TraceID            TraceID        `json:"traceId"`
	Caller             Caller         `json:"caller"`
	WorkspaceID        string         `json:"workspaceId"`
	TargetAgentID      string         `json:"targetAgentId"`
	AgentCardVersion   string         `json:"agentCardVersion"`
	Capability         string         `json:"capability"`
	Input              map[string]any `json:"input"`
	Stream             bool           `json:"stream"`
}
