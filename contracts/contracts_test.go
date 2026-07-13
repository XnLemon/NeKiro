package contracts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/getkin/kin-openapi/openapi3"
	"golang.org/x/mod/modfile"
)

func TestAgentCardContract(t *testing.T) {
	validator := mustValidator(t)
	card := validAgentCard()

	if err := validator.ValidateAgentCard(card); err != nil {
		t.Fatalf("valid card rejected: %v", err)
	}

	encoded, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	decoded, err := validator.DecodeAgentCard(encoded)
	if err != nil {
		t.Fatalf("decode valid card: %v", err)
	}
	if decoded.AgentID != card.AgentID {
		t.Fatalf("decoded agent id = %q, want %q", decoded.AgentID, card.AgentID)
	}
	if _, err := validator.DecodeAgentCard(append(encoded, []byte(` {}`)...)); err == nil {
		t.Fatal("Agent Card with trailing JSON value was accepted")
	}
}

func TestAgentCardRejectsSecretsAndRuntimeState(t *testing.T) {
	validator := mustValidator(t)
	card := validAgentCard()
	encoded, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("unmarshal card: %v", err)
	}

	for _, field := range []string{"apiKey", "token", "healthStatus", "latencyMs", "successRate"} {
		t.Run(field, func(t *testing.T) {
			candidate := make(map[string]any, len(document)+1)
			for key, value := range document {
				candidate[key] = value
			}
			candidate[field] = "forbidden"
			data, err := json.Marshal(candidate)
			if err != nil {
				t.Fatalf("marshal invalid card: %v", err)
			}
			if _, err := validator.DecodeAgentCard(data); err == nil {
				t.Fatalf("card with %s was accepted", field)
			}
		})
	}
}

func TestAgentCardSemanticValidation(t *testing.T) {
	validator := mustValidator(t)

	duplicateSkill := validAgentCard()
	duplicateSkill.Skills = append(duplicateSkill.Skills, AgentSkill{
		ID:                  duplicateSkill.Skills[0].ID,
		Name:                "Duplicate",
		Description:         "Duplicate semantic identifier.",
		InputSchema:         JSONSchema{"type": "object"},
		OutputSchema:        JSONSchema{"type": "object"},
		RequiredPermissions: []string{"document.read"},
	})
	var semanticError *SemanticValidationError
	if err := validator.ValidateAgentCard(duplicateSkill); !errors.As(err, &semanticError) {
		t.Fatalf("duplicate skill error = %v, want SemanticValidationError", err)
	}

	undeclaredPermission := validAgentCard()
	undeclaredPermission.Skills[0].RequiredPermissions = []string{"database.write"}
	if err := validator.ValidateAgentCard(undeclaredPermission); !errors.As(err, &semanticError) {
		t.Fatalf("undeclared permission error = %v, want SemanticValidationError", err)
	}
}

func TestAgentCardRejectsInvalidSemver(t *testing.T) {
	validator := mustValidator(t)
	card := validAgentCard()
	card.Version = "1.0.0-01"
	if err := validator.ValidateAgentCard(card); err == nil {
		t.Fatal("invalid semantic version was accepted")
	}
}

func TestAgentCardEndpointConstraints(t *testing.T) {
	validator := mustValidator(t)

	card := validAgentCard()
	card.Protocol.Endpoint = "http://localhost:4101/has whitespace"
	if err := validator.ValidateAgentCard(card); err == nil {
		t.Fatal("endpoint containing whitespace was accepted")
	}

	card = validAgentCard()
	card.Protocol.Endpoint = "https://example.test/" + strings.Repeat("a", 2_048)
	if err := validator.ValidateAgentCard(card); err == nil {
		t.Fatal("endpoint longer than 2048 bytes was accepted")
	}
}

func TestInstallationContract(t *testing.T) {
	validator := mustValidator(t)
	installation := validInstallation()
	if err := validator.ValidateInstallation(installation); err != nil {
		t.Fatalf("valid installation rejected: %v", err)
	}
	installation.VersionConstraint = "not a range"
	if err := validator.ValidateInstallation(installation); err == nil {
		t.Fatal("invalid version range was accepted")
	}
}

func TestInvocationEventLifecycleContract(t *testing.T) {
	validator := mustValidator(t)
	event := validStartedEvent()
	if err := validator.ValidateInvocationEvent(event); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}

	event.Status = "failed"
	if err := validator.ValidateInvocationEvent(event); err == nil {
		t.Fatal("inconsistent event status was accepted")
	}

	event = validStartedEvent()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	document["payload"] = map[string]any{"apiKey": "secret"}
	data, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal secret event: %v", err)
	}
	if err := validateJSONBytes(validator.invocationEvent, data); err == nil {
		t.Fatal("event with arbitrary payload was accepted")
	}
}

func TestPlatformErrorRejectsArbitraryDetails(t *testing.T) {
	validator := mustValidator(t)
	platformError, err := NewPlatformError(ErrorCodeInternal, "trace-1")
	if err != nil {
		t.Fatalf("create platform error: %v", err)
	}
	if err := validator.ValidatePlatformError(platformError); err != nil {
		t.Fatalf("valid platform error rejected: %v", err)
	}
	data := []byte(`{"code":"INTERNAL_ERROR","message":"failed","details":{"token":"secret"}}`)
	if err := validateJSONBytes(validator.platformError, data); err == nil {
		t.Fatal("platform error with arbitrary details was accepted")
	}
	secretMessage := PlatformError{Code: ErrorCodeInternal, Message: "token=secret", TraceID: "trace-1"}
	if err := validator.ValidatePlatformError(secretMessage); err == nil {
		t.Fatal("platform error with non-policy message was accepted")
	}
	if _, err := NewPlatformError(PlatformErrorCode("UNKNOWN"), "trace-1"); err == nil {
		t.Fatal("unknown platform error code was accepted")
	}
	if _, err := ParseTraceID("token=secret"); err == nil {
		t.Fatal("unsafe trace id was accepted")
	}
	if _, err := NewPlatformError(ErrorCodeInternal, TraceID("token=secret")); err == nil {
		t.Fatal("public error accepted an unsafe trace id")
	}

	if len(platformErrorV2Messages) != 17 {
		t.Fatalf("public error policy contains %d codes, want 17", len(platformErrorV2Messages))
	}
	for code := range platformErrorV2Messages {
		publicError, err := NewPlatformError(code, "trace-1")
		if err != nil {
			t.Fatalf("create public error %s: %v", code, err)
		}
		if err := validator.ValidatePlatformError(publicError); err != nil {
			t.Fatalf("public error %s does not match schema: %v", code, err)
		}
		message := strings.ToLower(publicError.Message)
		for _, forbidden := range []string{"api key", "token=", "secret="} {
			if strings.Contains(message, forbidden) {
				t.Fatalf("public error %s contains forbidden marker %q", code, forbidden)
			}
		}
	}
}

func TestA2AProfileUsesOfficialSDK(t *testing.T) {
	validator := mustValidator(t)
	profile, err := LoadA2AProfile()
	if err != nil {
		t.Fatalf("load A2A profile: %v", err)
	}
	if err := validator.ValidateA2AProfile(profile); err != nil {
		t.Fatalf("validate A2A profile: %v", err)
	}

	if profile.SchemaVersion != A2AProfileSchemaVersion || profile.ProtocolVersion != A2AProtocolVersion {
		t.Fatalf("active A2A profile versions = schema %q protocol %q", profile.SchemaVersion, profile.ProtocolVersion)
	}
	wantMethods := []string{"message/send", "message/stream", "tasks/get", "tasks/cancel"}
	if len(profile.Operations) != len(wantMethods) {
		t.Fatalf("required operation count = %d, want %d", len(profile.Operations), len(wantMethods))
	}
	for index := range wantMethods {
		if profile.Operations[index].Method != wantMethods[index] {
			t.Fatalf("required method %d = %q, want %q", index, profile.Operations[index].Method, wantMethods[index])
		}
	}

	message := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "Review this contract."})
	params := a2a.MessageSendParams{Message: message}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal official A2A params: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("official A2A params encoded to an empty payload")
	}

	goMod, err := os.ReadFile(filepath.Join("..", "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	parsed, err := modfile.Parse("go.mod", goMod, nil)
	if err != nil {
		t.Fatalf("parse go.mod: %v", err)
	}
	var requiredVersion string
	for _, requirement := range parsed.Require {
		if requirement.Mod.Path == profile.SDK.Module {
			requiredVersion = requirement.Mod.Version
			break
		}
	}
	if requiredVersion != profile.SDK.Version {
		t.Fatalf("A2A SDK version in go.mod = %q, profile = %q", requiredVersion, profile.SDK.Version)
	}
}

func TestOpenAPIDocuments(t *testing.T) {
	for _, path := range []string{
		filepath.Join("openapi", "control-plane.v2.yaml"),
		filepath.Join("openapi", "control-plane-internal.v1.yaml"),
		filepath.Join("openapi", "router-internal.v2.yaml"),
	} {
		t.Run(path, func(t *testing.T) {
			loadOpenAPIDocument(t, path)
		})
	}
}

func TestHistoricalV1AndV01ArtifactsRemainReadable(t *testing.T) {
	for _, path := range []string{
		"schemas/agent-card.v0.1.schema.json",
		"schemas/invocation-event.v0.1.schema.json",
		"schemas/platform-error.v1.schema.json",
		"schemas/a2a-profile.v0.3.0.schema.json",
	} {
		t.Run(path, func(t *testing.T) {
			if _, err := readJSONDocument(path); err != nil {
				t.Fatalf("historical schema is not readable: %v", err)
			}
		})
	}

	for _, path := range []string{
		filepath.Join("openapi", "control-plane.v1.yaml"),
		filepath.Join("openapi", "router-internal.v1.yaml"),
	} {
		t.Run(path, func(t *testing.T) {
			loadOpenAPIDocument(t, path)
		})
	}

	type historicalA2AProfile struct {
		SchemaVersion   string            `json:"schemaVersion"`
		ProtocolVersion string            `json:"protocolVersion"`
		SDK             A2ASDK            `json:"sdk"`
		Transport       string            `json:"transport"`
		AgentCardPath   string            `json:"agentCardPath"`
		RequiredMethods []string          `json:"requiredMethods"`
		ContextHeaders  A2AContextHeaders `json:"contextHeaders"`
	}
	data, err := contractFiles.ReadFile("a2a-profile/v0.3.0.json")
	if err != nil {
		t.Fatalf("read historical A2A profile: %v", err)
	}
	var profile historicalA2AProfile
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		t.Fatalf("decode historical A2A profile: %v", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		t.Fatalf("decode historical A2A profile: %v", err)
	}
	if profile.SchemaVersion != "0.1" || profile.ProtocolVersion != "0.3.0" || len(profile.RequiredMethods) == 0 {
		t.Fatalf("historical A2A profile identity changed: %+v", profile)
	}
}

func TestGoDTOsMatchOpenAPI(t *testing.T) {
	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	card := validAgentCard()
	catalogEntry := CatalogEntry{Card: card, PublicationStatus: "published", RegisteredAt: now, PublishedAt: &now}
	installation := validInstallation()
	event := validStartedEvent()
	record := InvocationRecord{
		InvocationID:     event.InvocationID,
		RootTaskID:       event.RootTaskID,
		TraceID:          event.TraceID,
		Caller:           event.Caller,
		WorkspaceID:      event.WorkspaceID,
		TargetAgentID:    event.TargetAgentID,
		AgentCardVersion: event.AgentCardVersion,
		Capability:       event.Capability,
		Status:           event.Status,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	result := InvocationResult{
		SchemaVersion: InvocationResultSchemaVersion,
		InvocationID:  event.InvocationID,
		RootTaskID:    event.RootTaskID,
		TraceID:       event.TraceID,
		Status:        "succeeded",
		Result:        json.RawMessage(`{"summary":"contract accepted"}`),
	}

	controlPlane := loadOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v2.yaml"))
	controlCases := []struct {
		name   string
		schema *openapi3.SchemaRef
		value  any
	}{
		{
			name:   "register request",
			schema: controlPlane.Paths.Find("/v2/agents").Post.RequestBody.Value.Content["application/json"].Schema,
			value:  RegisterAgentRequest{Card: card},
		},
		{
			name:   "search response",
			schema: controlPlane.Paths.Find("/v2/agents").Get.Responses.Status(200).Value.Content["application/json"].Schema,
			value:  SearchAgentsResponse{Items: []CatalogEntry{catalogEntry}},
		},
		{
			name:   "install request",
			schema: controlPlane.Paths.Find("/v2/workspaces/{workspaceId}/installations").Post.RequestBody.Value.Content["application/json"].Schema,
			value:  InstallAgentRequest{AgentID: card.AgentID, VersionConstraint: "^1.0.0", AcceptedPermissions: []string{"document.read"}},
		},
		{
			name:   "installation response",
			schema: controlPlane.Paths.Find("/v2/workspaces/{workspaceId}/installations").Post.Responses.Status(201).Value.Content["application/json"].Schema,
			value:  installation,
		},
		{
			name:   "update installation request",
			schema: controlPlane.Paths.Find("/v2/workspaces/{workspaceId}/installations/{installationId}").Patch.RequestBody.Value.Content["application/json"].Schema,
			value:  UpdateInstallationRequest{Status: "disabled"},
		},
		{
			name:   "invoke request",
			schema: controlPlane.Paths.Find("/v2/workspaces/{workspaceId}/invocations").Post.RequestBody.Value.Content["application/json"].Schema,
			value:  InvokeAgentRequest{AgentID: card.AgentID, Capability: "contract.review", Input: map[string]any{"text": "contract"}, Stream: true},
		},
		{
			name:   "invocation result",
			schema: controlPlane.Paths.Find("/v2/workspaces/{workspaceId}/invocations").Post.Responses.Status(200).Value.Content["application/json"].Schema,
			value:  result,
		},
		{
			name:   "invocation detail",
			schema: controlPlane.Paths.Find("/v2/invocations/{invocationId}").Get.Responses.Status(200).Value.Content["application/json"].Schema,
			value:  InvocationDetailResponse{Invocation: record, Events: []InvocationEvent{event}},
		},
		{
			name:   "trace response",
			schema: controlPlane.Paths.Find("/v2/traces/{traceId}").Get.Responses.Status(200).Value.Content["application/json"].Schema,
			value:  TraceResponse{TraceID: event.TraceID, Invocations: []InvocationRecord{record}},
		},
	}
	for _, testCase := range controlCases {
		t.Run(testCase.name, func(t *testing.T) {
			validateOpenAPIValue(t, testCase.schema, testCase.value)
		})
	}

	controlPlaneInternal := loadOpenAPIDocument(t, filepath.Join("openapi", "control-plane-internal.v1.yaml"))
	router := loadOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))
	streamOperation := router.Paths.Find("/internal/v2/invocations/{invocationId}/events")
	if streamOperation == nil || streamOperation.Get == nil {
		t.Fatal("Router SSE operation is missing")
	}
	if _, exists := streamOperation.Get.Responses.Status(200).Value.Content["text/event-stream"]; !exists {
		t.Fatal("Router SSE response does not declare text/event-stream")
	}
	resolvedInstallation := ResolvedInstallation{
		InstallationID:      installation.InstallationID,
		WorkspaceID:         installation.WorkspaceID,
		AgentID:             installation.AgentID,
		InstalledVersion:    installation.InstalledVersion,
		AcceptedPermissions: installation.AcceptedPermissions,
		Status:              "enabled",
	}
	internalCases := []struct {
		name   string
		schema *openapi3.SchemaRef
		value  any
	}{
		{
			name:   "resolve request",
			schema: controlPlaneInternal.Paths.Find("/internal/v1/resolve-agent").Post.RequestBody.Value.Content["application/json"].Schema,
			value: ResolveAgentRequest{
				InvocationID: event.InvocationID, RootTaskID: event.RootTaskID, TraceID: event.TraceID,
				WorkspaceID: installation.WorkspaceID, AgentID: card.AgentID, Version: card.Version, Capability: "contract.review",
			},
		},
		{
			name:   "resolve response",
			schema: controlPlaneInternal.Paths.Find("/internal/v1/resolve-agent").Post.Responses.Status(200).Value.Content["application/json"].Schema,
			value:  ResolveAgentResponse{Card: card, Installation: resolvedInstallation},
		},
		{
			name:   "dispatch request",
			schema: router.Paths.Find("/internal/v2/invocations").Post.RequestBody.Value.Content["application/json"].Schema,
			value: DispatchInvocationRequest{
				InvocationID: event.InvocationID, RootTaskID: event.RootTaskID, TraceID: event.TraceID,
				Caller: event.Caller, WorkspaceID: event.WorkspaceID, TargetAgentID: event.TargetAgentID,
				AgentCardVersion: event.AgentCardVersion, Capability: event.Capability,
				Input: map[string]any{"text": "contract"}, Stream: true,
			},
		},
		{
			name:   "dispatch result",
			schema: router.Paths.Find("/internal/v2/invocations").Post.Responses.Status(200).Value.Content["application/json"].Schema,
			value:  result,
		},
		{
			name:   "router event envelope",
			schema: router.Components.Schemas["RouterEventEnvelope"],
			value:  RouterEventEnvelope{Event: event},
		},
	}
	for _, testCase := range internalCases {
		t.Run(testCase.name, func(t *testing.T) {
			validateOpenAPIValue(t, testCase.schema, testCase.value)
		})
	}
}

func TestSearchAgentsQueryMatchesOpenAPI(t *testing.T) {
	document := loadOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v2.yaml"))
	operation := document.Paths.Find("/v2/agents").Get
	query := SearchAgentsQuery{
		Query:      stringPointer("contract"),
		Capability: stringPointer("contract.review"),
		OwnerID:    stringPointer("nene7ko"),
		Limit:      intPointer(25),
		Cursor:     stringPointer("cursor-1"),
	}
	data, err := json.Marshal(query)
	if err != nil {
		t.Fatalf("marshal search query: %v", err)
	}
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatalf("decode search query: %v", err)
	}
	if len(operation.Parameters) != len(values) {
		t.Fatalf("OpenAPI has %d search parameters, Go DTO has %d", len(operation.Parameters), len(values))
	}
	for _, parameterRef := range operation.Parameters {
		parameter := parameterRef.Value
		value, exists := values[parameter.Name]
		if !exists {
			t.Fatalf("Go search DTO is missing parameter %s", parameter.Name)
		}
		validateOpenAPIValue(t, parameter.Schema, value)
	}

	queryParameter := operation.Parameters.GetByInAndName("query", "query")
	if queryParameter == nil {
		t.Fatal("OpenAPI query parameter is missing")
	}
	if err := queryParameter.Schema.Value.VisitJSON("   ", openapi3.EnableJSONSchema2020()); err == nil {
		t.Fatal("whitespace-only search query was accepted")
	}
}

func loadOpenAPIDocument(t *testing.T, path string) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(loader *openapi3.Loader, location *url.URL) ([]byte, error) {
		if location.Scheme == "https" && location.Host == "schemas.nekiro.dev" {
			schemaFiles := map[string]string{
				"/common/v1":                         "schemas/common.v1.schema.json",
				"/agent-card/v0.1":                   "schemas/agent-card.v0.1.schema.json",
				"/agent-card/v0.2":                   "schemas/agent-card.v0.2.schema.json",
				"/platform-error/v1":                 "schemas/platform-error.v1.schema.json",
				"/platform-error/v2":                 "schemas/platform-error.v2.schema.json",
				"/installation/v1":                   "schemas/installation.v1.schema.json",
				"/invocation-event/v0.1":             "schemas/invocation-event.v0.1.schema.json",
				"/invocation-event/v0.2":             "schemas/invocation-event.v0.2.schema.json",
				"/invocation-result/v1":              "schemas/invocation-result.v1.schema.json",
				"/invocation-result-stream-event/v1": "schemas/invocation-result-stream-event.v1.schema.json",
			}
			localPath, exists := schemaFiles[location.Path]
			if !exists {
				return nil, fmt.Errorf("unsupported schema URI: %s", location.Redacted())
			}
			return contractFiles.ReadFile(localPath)
		}
		if location.Scheme == "" || location.Scheme == "file" {
			return openapi3.DefaultReadFromURI(loader, location)
		}
		return nil, fmt.Errorf("external URI is not allowed: %s", location.Redacted())
	}
	document, err := loader.LoadFromFile(path)
	if err != nil {
		t.Fatalf("load OpenAPI document: %v", err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatalf("validate OpenAPI document: %v", err)
	}
	return document
}

func validateOpenAPIValue(t *testing.T, schemaRef *openapi3.SchemaRef, value any) {
	t.Helper()
	if schemaRef == nil || schemaRef.Value == nil {
		t.Fatal("OpenAPI schema was not resolved")
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal DTO: %v", err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode DTO JSON: %v", err)
	}
	if err := schemaRef.Value.VisitJSON(document, openapi3.EnableJSONSchema2020()); err != nil {
		t.Fatalf("DTO does not match OpenAPI: %v", err)
	}
}

func stringPointer(value string) *string {
	return &value
}

func intPointer(value int) *int {
	return &value
}

func mustValidator(t *testing.T) *Validator {
	t.Helper()
	validator, err := NewValidator()
	if err != nil {
		t.Fatalf("create validator: %v", err)
	}
	return validator
}

func validAgentCard() AgentCard {
	return AgentCard{
		SchemaVersion: AgentCardSchemaVersion,
		AgentID:       "contract-review",
		Name:          "Contract Review Agent",
		Description:   "Reviews contracts against a declared checklist.",
		Owner:         AgentOwner{ID: "nene7ko", DisplayName: "Nene7ko"},
		Version:       "1.0.0",
		Protocol: AgentProtocol{
			Type: "a2a", Version: A2AProtocolVersion, Transport: "JSONRPC", Endpoint: "http://localhost:4101",
		},
		Skills: []AgentSkill{{
			ID:                  "contract.review",
			Name:                "Review contract",
			Description:         "Reviews a contract.",
			InputSchema:         JSONSchema{"type": "object"},
			OutputSchema:        JSONSchema{"type": "object"},
			RequiredPermissions: []string{"document.read"},
		}},
		Authentication: AgentAuthentication{Type: "none"},
		Permissions: []PermissionDeclaration{{
			ID: "document.read", Description: "Read the supplied document.",
		}},
		Limits: AgentLimits{TimeoutMS: 30_000, MaxInputBytes: 1_000_000, MaxOutputBytes: 1_000_000, Streaming: true},
	}
}

func validInstallation() Installation {
	now := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	return Installation{
		InstallationID:      "installation-1",
		WorkspaceID:         "workspace-1",
		AgentID:             "contract-review",
		VersionConstraint:   "^1.0.0",
		InstalledVersion:    "1.2.0",
		AcceptedPermissions: []string{"document.read"},
		Status:              "enabled",
		InstalledAt:         now,
		UpdatedAt:           now,
	}
}

func validStartedEvent() InvocationEvent {
	return InvocationEvent{
		SchemaVersion:    InvocationEventSchemaVersion,
		EventID:          "event-1",
		Sequence:         1,
		OccurredAt:       time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		Type:             "started",
		Status:           "running",
		InvocationID:     "invocation-1",
		RootTaskID:       "task-1",
		TraceID:          "trace-1",
		Caller:           Caller{Type: "user", ID: "user-1"},
		WorkspaceID:      "workspace-1",
		TargetAgentID:    "contract-review",
		AgentCardVersion: "1.0.0",
		Capability:       "contract.review",
	}
}

func validateJSONBytes(schema interface{ Validate(any) error }, data []byte) error {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	return schema.Validate(document)
}
