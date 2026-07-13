package contracts

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestActiveContractVersionSynchronization(t *testing.T) {
	wantConstants := map[string]string{
		"Agent Card Schema":          "0.2",
		"Invocation Event Schema":    "0.2",
		"Platform Error Schema":      "2",
		"Invocation Result Schema":   "1",
		"Result Stream Event Schema": "1",
		"A2A Profile Schema":         "0.2",
		"A2A protocol":               "0.3.0",
		"Northbound API":             "2",
		"Control Plane Internal API": "1",
		"Router Internal API":        "2",
	}
	actualConstants := map[string]string{
		"Agent Card Schema":          AgentCardSchemaVersion,
		"Invocation Event Schema":    InvocationEventSchemaVersion,
		"Platform Error Schema":      PlatformErrorSchemaVersion,
		"Invocation Result Schema":   InvocationResultSchemaVersion,
		"Result Stream Event Schema": InvocationResultStreamEventSchemaVersion,
		"A2A Profile Schema":         A2AProfileSchemaVersion,
		"A2A protocol":               A2AProtocolVersion,
		"Northbound API":             NorthboundAPIVersion,
		"Control Plane Internal API": ControlPlaneInternalAPIVersion,
		"Router Internal API":        RouterInternalAPIVersion,
	}
	for name, want := range wantConstants {
		if actualConstants[name] != want {
			t.Errorf("%s constant = %q, want %q", name, actualConstants[name], want)
		}
	}

	schemaVersions := []struct {
		path     string
		property string
		want     string
	}{
		{path: "schemas/agent-card.v0.2.schema.json", property: "schemaVersion", want: AgentCardSchemaVersion},
		{path: "schemas/invocation-event.v0.2.schema.json", property: "schemaVersion", want: InvocationEventSchemaVersion},
		{path: "schemas/invocation-result.v1.schema.json", property: "schemaVersion", want: InvocationResultSchemaVersion},
		{path: "schemas/invocation-result-stream-event.v1.schema.json", property: "schemaVersion", want: InvocationResultStreamEventSchemaVersion},
		{path: "schemas/a2a-profile.v0.2.schema.json", property: "schemaVersion", want: A2AProfileSchemaVersion},
	}
	for _, schema := range schemaVersions {
		t.Run(schema.path, func(t *testing.T) {
			document := readContractJSONObject(t, schema.path)
			properties := requiredJSONObject(t, document, "properties")
			property := requiredJSONObject(t, properties, schema.property)
			if actual, ok := property["const"].(string); !ok || actual != schema.want {
				t.Fatalf("%s const = %#v, want %q", schema.property, property["const"], schema.want)
			}
		})
	}

	profile, err := LoadA2AProfile()
	if err != nil {
		t.Fatalf("load active A2A Profile: %v", err)
	}
	if profile.SchemaVersion != A2AProfileSchemaVersion || profile.ProtocolVersion != A2AProtocolVersion {
		t.Fatalf("active A2A Profile identity = schema %q protocol %q", profile.SchemaVersion, profile.ProtocolVersion)
	}
	manifest, err := LoadA2AConformanceManifestV02()
	if err != nil {
		t.Fatalf("load active A2A conformance manifest: %v", err)
	}
	if manifest.ProfileSchemaVersion != A2AProfileSchemaVersion || manifest.ProtocolVersion != A2AProtocolVersion {
		t.Fatalf("A2A manifest identity = profile %q protocol %q", manifest.ProfileSchemaVersion, manifest.ProtocolVersion)
	}

	documents := []struct {
		path string
		want string
	}{
		{path: filepath.Join("openapi", "control-plane.v2.yaml"), want: "2.0.0"},
		{path: filepath.Join("openapi", "control-plane-internal.v1.yaml"), want: "1.0.0"},
		{path: filepath.Join("openapi", "router-internal.v2.yaml"), want: "2.0.0"},
	}
	for _, document := range documents {
		if actual := loadOpenAPIDocument(t, document.path).Info.Version; actual != document.want {
			t.Errorf("%s version = %q, want %q", document.path, actual, document.want)
		}
	}
}

func TestActiveOpenAPIToGoMappings(t *testing.T) {
	card := validAgentCard()
	installation := validInstallation()
	event := validStartedEvent()
	result := InvocationResult{
		SchemaVersion: InvocationResultSchemaVersion,
		InvocationID:  event.InvocationID,
		RootTaskID:    event.RootTaskID,
		TraceID:       event.TraceID,
		Status:        "succeeded",
		Result:        json.RawMessage(`{"answer":42}`),
	}

	northbound := loadOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v2.yaml"))
	validateOpenAPIValue(
		t,
		northbound.Paths.Find("/v2/agents").Post.RequestBody.Value.Content["application/json"].Schema,
		RegisterAgentRequest{Card: card},
	)
	validateOpenAPIValue(
		t,
		northbound.Paths.Find("/v2/workspaces/{workspaceId}/invocations").Post.Responses.Status(200).Value.Content["application/json"].Schema,
		result,
	)

	controlPlaneInternal := loadOpenAPIDocument(t, filepath.Join("openapi", "control-plane-internal.v1.yaml"))
	resolveRequest := ResolveAgentRequest{
		InvocationID: event.InvocationID,
		RootTaskID:   event.RootTaskID,
		TraceID:      event.TraceID,
		WorkspaceID:  installation.WorkspaceID,
		AgentID:      card.AgentID,
		Version:      card.Version,
		Capability:   event.Capability,
	}
	resolveOperation := controlPlaneInternal.Paths.Find("/internal/v1/resolve-agent").Post
	validateOpenAPIValue(t, resolveOperation.RequestBody.Value.Content["application/json"].Schema, resolveRequest)
	validateOpenAPIValue(t, resolveOperation.Responses.Status(200).Value.Content["application/json"].Schema, ResolveAgentResponse{
		Card: card,
		Installation: ResolvedInstallation{
			InstallationID:      installation.InstallationID,
			WorkspaceID:         installation.WorkspaceID,
			AgentID:             installation.AgentID,
			InstalledVersion:    installation.InstalledVersion,
			AcceptedPermissions: installation.AcceptedPermissions,
			Status:              installation.Status,
		},
	})

	router := loadOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))
	dispatchRequest := DispatchInvocationRequest{
		InvocationID:     event.InvocationID,
		RootTaskID:       event.RootTaskID,
		TraceID:          event.TraceID,
		Caller:           event.Caller,
		WorkspaceID:      event.WorkspaceID,
		TargetAgentID:    event.TargetAgentID,
		AgentCardVersion: event.AgentCardVersion,
		Capability:       event.Capability,
		Input:            map[string]any{"contract": "active"},
		Stream:           false,
	}
	dispatchOperation := router.Paths.Find("/internal/v2/invocations").Post
	validateOpenAPIValue(t, dispatchOperation.RequestBody.Value.Content["application/json"].Schema, dispatchRequest)
	validateOpenAPIValue(t, dispatchOperation.Responses.Status(200).Value.Content["application/json"].Schema, result)
	validateOpenAPIValue(t, router.Components.Schemas["RouterEventEnvelope"], RouterEventEnvelope{Event: event})

	var _ PlatformError = PlatformErrorV2{}
	var _ InvocationEvent = InvocationEventV02{}
	var _ RouterEventEnvelope = RouterEventEnvelopeV02{}
	var _ A2AProfile = A2AProfileV02{}
	var _ ResolveAgentRequest = ResolveAgentRequestV1{}
}

func TestActiveContractCorporaAreDiscoverable(t *testing.T) {
	contractFS := ContractFiles()
	patterns := map[string]int{
		"agent-card/v0.2/conformance/*.json":    2,
		"invocation/v1/conformance/*.json":      2,
		"a2a-profile/v0.3.0/conformance/*.json": 2,
		"a2a-profile/v0.3.0/conformance/*.sse":  1,
	}
	for pattern, minimum := range patterns {
		matches, err := fs.Glob(contractFS, pattern)
		if err != nil {
			t.Fatalf("discover %s: %v", pattern, err)
		}
		if len(matches) < minimum {
			t.Errorf("%s discovered %d files, want at least %d", pattern, len(matches), minimum)
		}
	}

	agentManifestData, err := fs.ReadFile(contractFS, "agent-card/v0.2/conformance/manifest.json")
	if err != nil {
		t.Fatalf("read embedded Agent Card manifest: %v", err)
	}
	agentManifest, err := DecodeAgentCardConformanceManifest(agentManifestData)
	if err != nil {
		t.Fatalf("decode embedded Agent Card manifest: %v", err)
	}
	for _, manifestCase := range agentManifest.Cases {
		assertEmbeddedContractFile(t, contractFS, path.Join("agent-card/v0.2/conformance", manifestCase.File))
		for _, contextFile := range manifestCase.ContextFiles {
			assertEmbeddedContractFile(t, contractFS, path.Join("agent-card/v0.2/conformance", contextFile))
		}
	}

	invocationManifest, err := loadInvocationConformanceManifest()
	if err != nil {
		t.Fatalf("load Invocation conformance manifest: %v", err)
	}
	for _, manifestCase := range invocationManifest.Cases {
		assertEmbeddedContractFile(t, contractFS, path.Join("invocation/v1/conformance", manifestCase.File))
	}

	a2aManifest, err := LoadA2AConformanceManifestV02()
	if err != nil {
		t.Fatalf("load A2A conformance manifest: %v", err)
	}
	for _, manifestCase := range a2aManifest.Cases {
		assertEmbeddedContractFile(t, contractFS, path.Join("a2a-profile/v0.3.0/conformance", manifestCase.File))
		if manifestCase.RequestFile != "" {
			assertEmbeddedContractFile(t, contractFS, path.Join("a2a-profile/v0.3.0/conformance", manifestCase.RequestFile))
		}
	}
}

func TestHistoricalContractsRemainReadableWithoutActiveDualRead(t *testing.T) {
	historicalJSON := []string{
		"schemas/agent-card.v0.1.schema.json",
		"schemas/invocation-event.v0.1.schema.json",
		"schemas/platform-error.v1.schema.json",
		"schemas/a2a-profile.v0.3.0.schema.json",
		"a2a-profile/v0.3.0.json",
	}
	for _, file := range historicalJSON {
		t.Run(file, func(t *testing.T) {
			data, err := fs.ReadFile(ContractFiles(), file)
			if err != nil {
				t.Fatalf("read historical artifact: %v", err)
			}
			if err := rejectDuplicateJSONMemberNames(data); err != nil {
				t.Fatalf("parse historical artifact: %v", err)
			}
		})
	}
	loadOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v1.yaml"))
	loadOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v1.yaml"))

	validator := mustValidator(t)
	historicalCard := validAgentCard()
	historicalCard.SchemaVersion = "0.1"
	if err := validator.ValidateAgentCard(historicalCard); err == nil {
		t.Fatal("active Validator accepted historical Agent Card 0.1")
	}
	historicalEvent := validStartedEvent()
	historicalEvent.SchemaVersion = "0.1"
	if err := validator.ValidateInvocationEvent(historicalEvent); err == nil {
		t.Fatal("active Validator accepted historical Invocation Event 0.1")
	}
}

func TestActiveContractsExcludeSecretsAndResultsFromMetadata(t *testing.T) {
	agentCardSchema := readContractJSONObject(t, "schemas/agent-card.v0.2.schema.json")
	agentProperties := requiredJSONObject(t, agentCardSchema, "properties")
	assertObjectKeysAbsent(t, "Agent Card", agentProperties, "apiKey", "token", "secret", "healthStatus", "latencyMs", "successRate")
	authentication := requiredJSONObject(t, requiredJSONObject(t, agentProperties, "authentication"), "properties")
	assertExactObjectKeys(t, "Agent Card authentication", authentication, []string{"type"})
	protocol := requiredJSONObject(t, requiredJSONObject(t, agentProperties, "protocol"), "properties")
	assertExactObjectKeys(t, "Agent Card protocol", protocol, []string{"endpoint", "transport", "type", "version"})

	eventSchema := readContractJSONObject(t, "schemas/invocation-event.v0.2.schema.json")
	eventProperties := requiredJSONObject(t, eventSchema, "properties")
	assertObjectKeysAbsent(t, "Invocation Event", eventProperties, "input", "result", "chunk", "output", "payload")

	errorSchema := readContractJSONObject(t, "schemas/platform-error.v2.schema.json")
	errorProperties := requiredJSONObject(t, errorSchema, "properties")
	assertExactObjectKeys(t, "Platform Error", errorProperties, []string{"code", "invocationId", "message", "rootTaskId", "traceId"})

	eventJSON, err := json.Marshal(validStartedEvent())
	if err != nil {
		t.Fatalf("marshal active Invocation Event: %v", err)
	}
	platformError, err := NewPlatformError(ErrorCodeInternal, "trace-secret-safe")
	if err != nil {
		t.Fatalf("create active Platform Error: %v", err)
	}
	errorJSON, err := json.Marshal(platformError)
	if err != nil {
		t.Fatalf("marshal active Platform Error: %v", err)
	}
	for label, data := range map[string][]byte{"Invocation Event": eventJSON, "Platform Error": errorJSON} {
		lower := strings.ToLower(string(data))
		for _, forbidden := range []string{"apikey", "token", "secret=", `"input"`, `"result"`, `"chunk"`, `"output"`} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s contains forbidden marker %q: %s", label, forbidden, data)
			}
		}
	}
}

func TestActiveInternalAPIsPreserveDirectionalOwnership(t *testing.T) {
	controlPlane := loadOpenAPIDocument(t, filepath.Join("openapi", "control-plane-internal.v1.yaml"))
	router := loadOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))

	assertExactStringSlice(t, "Control Plane Internal paths", controlPlane.Paths.Keys(), []string{"/internal/v1/resolve-agent"})
	assertExactStringSlice(t, "Router Internal paths", router.Paths.Keys(), []string{
		"/internal/v2/invocations",
		"/internal/v2/invocations/{invocationId}",
		"/internal/v2/invocations/{invocationId}/events",
		"/internal/v2/traces/{traceId}",
	})
	if router.Paths.Find("/internal/v1/resolve-agent") != nil {
		t.Fatal("Router Internal API owns Control Plane resolution")
	}
	if controlPlane.Paths.Find("/internal/v2/invocations") != nil {
		t.Fatal("Control Plane Internal API owns Router dispatch")
	}
	if len(controlPlane.Servers) != 1 || len(router.Servers) != 1 {
		t.Fatal("active internal APIs must each declare one explicit owner destination")
	}
	if controlPlane.Servers[0].URL == router.Servers[0].URL {
		t.Fatal("active internal APIs share a destination")
	}
	for _, server := range []string{controlPlane.Servers[0].URL, router.Servers[0].URL} {
		if strings.Contains(server, "localhost") {
			t.Fatalf("active internal destination contains localhost fallback: %s", server)
		}
	}
}

func TestActiveValidatorComposesReviewedContractBehavior(t *testing.T) {
	validator := mustValidator(t)
	card := validAgentCard()
	if err := validator.ValidateAgentCard(card); err != nil {
		t.Fatalf("active Agent Card rejected: %v", err)
	}
	duplicatePermission := card
	duplicatePermission.Permissions = append(duplicatePermission.Permissions, duplicatePermission.Permissions[0])
	var semanticError *SemanticValidationError
	if err := validator.ValidateAgentCard(duplicatePermission); !errors.As(err, &semanticError) {
		t.Fatalf("duplicate permission error = %v, want SemanticValidationError", err)
	}
	if len(semanticError.Issues) != 1 || semanticError.Issues[0].RuleID != AgentCardRuleUniquePermissionIDs {
		t.Fatalf("duplicate permission issues = %+v, want %s", semanticError.Issues, AgentCardRuleUniquePermissionIDs)
	}

	encodedCard, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal active Agent Card: %v", err)
	}
	duplicateCard := append([]byte(`{"schemaVersion":"0.2",`), encodedCard[1:]...)
	if _, err := validator.DecodeAgentCard(duplicateCard); err == nil || !strings.Contains(err.Error(), "duplicate JSON object member") {
		t.Fatalf("duplicate Agent Card member error = %v", err)
	}

	profile, err := LoadA2AProfile()
	if err != nil {
		t.Fatalf("load active A2A Profile: %v", err)
	}
	if err := validator.ValidateA2AProfile(profile); err != nil {
		t.Fatalf("validate active A2A Profile: %v", err)
	}
	if err := validator.ValidateInvocationEvent(validStartedEvent()); err != nil {
		t.Fatalf("validate active Invocation Event: %v", err)
	}

	resultJSON := []byte(`{"schemaVersion":"1","invocationId":"inv-large","rootTaskId":"task-large","traceId":"trace-large","status":"succeeded","result":{"top":1e400,"nested":[1e400]}}`)
	var result InvocationResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatalf("decode legal large-number result: %v", err)
	}
	if string(result.Result) != `{"top":1e400,"nested":[1e400]}` {
		t.Fatalf("result number tokens changed: %s", result.Result)
	}
	if err := validator.ValidateInvocationResultForRequest(result, "inv-large", "task-large", "trace-large"); err != nil {
		t.Fatalf("validate request-bound result: %v", err)
	}

	streamJSON := []byte(`{"schemaVersion":"1","sequence":1,"type":"chunk","status":"running","invocationId":"inv-large","rootTaskId":"task-large","traceId":"trace-large","chunkIndex":0,"chunk":{"value":1e400}}`)
	var streamEvent InvocationResultStreamEvent
	if err := json.Unmarshal(streamJSON, &streamEvent); err != nil {
		t.Fatalf("decode legal large-number stream chunk: %v", err)
	}
	if string(streamEvent.Chunk) != `{"value":1e400}` {
		t.Fatalf("stream chunk number token changed: %s", streamEvent.Chunk)
	}
	if err := validator.ValidateInvocationResultStreamEvent(streamEvent); err != nil {
		t.Fatalf("validate active stream chunk: %v", err)
	}

	duplicateResult := []byte(`{"schemaVersion":"1","invocationId":"inv-large","rootTaskId":"task-large","traceId":"trace-large","status":"succeeded","result":{"value":1,"value":2}}`)
	if err := json.Unmarshal(duplicateResult, &result); err == nil || !strings.Contains(err.Error(), "duplicate JSON object member") {
		t.Fatalf("duplicate result member error = %v", err)
	}
}

func readContractJSONObject(t *testing.T, file string) map[string]any {
	t.Helper()
	data, err := fs.ReadFile(ContractFiles(), file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	if err := rejectDuplicateJSONMemberNames(data); err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var document map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("decode %s: %v", file, err)
	}
	return document
}

func requiredJSONObject(t *testing.T, object map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := object[key].(map[string]any)
	if !ok {
		t.Fatalf("%s is not a JSON object: %#v", key, object[key])
	}
	return value
}

func assertEmbeddedContractFile(t *testing.T, contractFS fs.FS, file string) {
	t.Helper()
	info, err := fs.Stat(contractFS, file)
	if err != nil {
		t.Fatalf("embedded contract file %s: %v", file, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("embedded contract path %s is not a regular file", file)
	}
}

func assertObjectKeysAbsent(t *testing.T, label string, object map[string]any, forbidden ...string) {
	t.Helper()
	for _, key := range forbidden {
		if _, exists := object[key]; exists {
			t.Errorf("%s exposes forbidden property %q", label, key)
		}
	}
}

func assertExactObjectKeys(t *testing.T, label string, object map[string]any, want []string) {
	t.Helper()
	actual := make([]string, 0, len(object))
	for key := range object {
		actual = append(actual, key)
	}
	assertExactStringSlice(t, label, actual, want)
}

func assertExactStringSlice(t *testing.T, label string, actual, want []string) {
	t.Helper()
	actual = slices.Clone(actual)
	want = slices.Clone(want)
	slices.Sort(actual)
	slices.Sort(want)
	if !slices.Equal(actual, want) {
		t.Errorf("%s = %v, want %v", label, actual, want)
	}
}
