package contracts

import "time"

const (
	AgentCardSchemaVersion       = "0.1"
	InvocationEventSchemaVersion = "0.1"
	A2AProtocolVersion           = "0.3.0"
)

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

type Caller struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type PlatformError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	TraceID string `json:"traceId,omitempty"`
}

type InvocationEvent struct {
	SchemaVersion      string         `json:"schemaVersion"`
	EventID            string         `json:"eventId"`
	Sequence           int64          `json:"sequence"`
	OccurredAt         time.Time      `json:"occurredAt"`
	Type               string         `json:"type"`
	Status             string         `json:"status"`
	InvocationID       string         `json:"invocationId"`
	RootTaskID         string         `json:"rootTaskId"`
	ParentInvocationID string         `json:"parentInvocationId,omitempty"`
	TraceID            string         `json:"traceId"`
	Caller             Caller         `json:"caller"`
	WorkspaceID        string         `json:"workspaceId"`
	TargetAgentID      string         `json:"targetAgentId"`
	AgentCardVersion   string         `json:"agentCardVersion"`
	Capability         string         `json:"capability"`
	ChunkIndex         *int64         `json:"chunkIndex,omitempty"`
	ChunkBytes         *int64         `json:"chunkBytes,omitempty"`
	LatencyMS          *int64         `json:"latencyMs,omitempty"`
	Error              *PlatformError `json:"error,omitempty"`
}

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

type A2AProfile struct {
	SchemaVersion   string            `json:"schemaVersion"`
	ProtocolVersion string            `json:"protocolVersion"`
	SDK             A2ASDK            `json:"sdk"`
	Transport       string            `json:"transport"`
	AgentCardPath   string            `json:"agentCardPath"`
	RequiredMethods []string          `json:"requiredMethods"`
	ContextHeaders  A2AContextHeaders `json:"contextHeaders"`
}

type ResolveAgentRequest struct {
	WorkspaceID string `json:"workspaceId"`
	AgentID     string `json:"agentId"`
	Version     string `json:"version"`
	Capability  string `json:"capability"`
}

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
	TraceID            string         `json:"traceId"`
	Caller             Caller         `json:"caller"`
	WorkspaceID        string         `json:"workspaceId"`
	TargetAgentID      string         `json:"targetAgentId"`
	AgentCardVersion   string         `json:"agentCardVersion"`
	Capability         string         `json:"capability"`
	Input              map[string]any `json:"input"`
	Stream             bool           `json:"stream"`
}
