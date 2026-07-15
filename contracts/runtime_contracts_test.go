package contracts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestRuntimeContractOpenAPIDirectionsAndVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path       string
		version    string
		route      string
		security   string
		serverPart string
	}{
		{"openapi/control-plane-invocation.v4.yaml", "4.0.0", "/v4/workspaces/{workspaceId}/invocations", "bearerAuth", "api.nekiro.dev"},
		{"openapi/router-internal.v3.yaml", "3.0.0", "/internal/v3/invocations", "serviceBearerAuth", "a2a-router.internal"},
		{"openapi/router-agent.v1.yaml", "1.0.0", "/agent/v1/invocations", "agentBearerAuth", "a2a-router.agent"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.path, func(t *testing.T) {
			document := loadOpenAPIDocument(t, filepath.FromSlash(test.path))
			if document.Info.Version != test.version {
				t.Fatalf("version = %q, want %q", document.Info.Version, test.version)
			}
			operation := document.Paths.Find(test.route).Post
			if operation == nil {
				t.Fatalf("missing POST %s", test.route)
			}
			if operation.Security == nil || len(*operation.Security) != 1 {
				t.Fatal("operation must declare exactly one security alternative")
			}
			if _, exists := (*operation.Security)[0][test.security]; !exists {
				t.Fatalf("operation security does not use %s", test.security)
			}
			if len(document.Servers) != 1 || !strings.Contains(document.Servers[0].URL, test.serverPart) {
				t.Fatalf("unexpected explicit destination: %#v", document.Servers)
			}
		})
	}
}

func TestRuntimeContractNestedRequestContainsOnlyUntrustedWork(t *testing.T) {
	t.Parallel()

	document := loadOpenAPIDocument(t, filepath.FromSlash("openapi/router-agent.v1.yaml"))
	schema := document.Components.Schemas["NestedInvocationRequest"].Value
	want := []string{"capability", "input", "parentInvocationId", "stream", "targetAgentId"}
	got := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		got = append(got, name)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("nested request fields = %v, want %v", got, want)
	}
	if schema.AdditionalProperties.Has == nil || *schema.AdditionalProperties.Has {
		t.Fatal("nested request must reject additional trusted fields")
	}

	encoded, err := json.Marshal(NestedInvocationRequestV1{
		ParentInvocationID: "parent-1",
		TargetAgentID:      "agent-b",
		Capability:         "summarize",
		Input:              json.RawMessage(`{"text":"safe"}`),
		Stream:             true,
	})
	if err != nil {
		t.Fatalf("marshal nested request: %v", err)
	}
	text := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"caller", "workspace", "roottask", "traceid", "agentcardversion", "endpoint", "credential", "token"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("nested request leaks trusted/secret field %q: %s", forbidden, encoded)
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode nested request fields: %v", err)
	}
	if _, exists := fields["invocationId"]; exists {
		t.Fatal("nested request must not supply the child Invocation ID")
	}
}

func TestRuntimeContractExactFailureMappings(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"openapi/control-plane-invocation.v4.yaml",
		"openapi/router-internal.v3.yaml",
		"openapi/router-agent.v1.yaml",
	} {
		document := loadOpenAPIDocument(t, filepath.FromSlash(path))
		var route string
		switch path {
		case "openapi/control-plane-invocation.v4.yaml":
			route = "/v4/workspaces/{workspaceId}/invocations"
		case "openapi/router-internal.v3.yaml":
			route = "/internal/v3/invocations"
		default:
			route = "/agent/v1/invocations"
		}
		responses := document.Paths.Find(route).Post.Responses
		assertExtensionStringSliceContains(t, responses.Status(413).Value.Extensions, "x-platform-error-codes", "PAYLOAD_TOO_LARGE")
		assertExtensionStringSliceContains(t, responses.Status(502).Value.Extensions, "x-platform-error-codes", "AGENT_AUTH_UNSUPPORTED")
		assertExtensionStringSliceContains(t, responses.Status(502).Value.Extensions, "x-platform-error-codes", "AGENT_RESPONSE_TOO_LARGE")
	}

	if ErrorCodeAgentAuthUnsupported == ErrorCodeRouteNotFound ||
		ErrorCodeAgentAuthUnsupported == ErrorCodeAgentUnavailable ||
		ErrorCodeAgentAuthUnsupported == ErrorCodeDependency {
		t.Fatal("unsupported Agent auth must remain a distinct outcome")
	}
}

func TestRuntimeContractLimitsAndSSEHaveNoDefaults(t *testing.T) {
	t.Parallel()

	for _, test := range []struct{ path, route string }{
		{"openapi/control-plane-invocation.v4.yaml", "/v4/workspaces/{workspaceId}/invocations"},
		{"openapi/router-internal.v3.yaml", "/internal/v3/invocations"},
		{"openapi/router-agent.v1.yaml", "/agent/v1/invocations"},
	} {
		document := loadOpenAPIDocument(t, filepath.FromSlash(test.path))
		operation := document.Paths.Find(test.route).Post
		request := operation.RequestBody.Value.Extensions
		if request["x-nekiro-max-body-bytes-source"] == nil || request["x-nekiro-limit-default"] != false {
			t.Fatalf("%s request limit must be required with no default: %#v", test.path, request)
		}
		stream := operation.Responses.Status(200).Value.Content["text/event-stream"]
		if stream.Extensions["x-nekiro-sse-framing"] != "single-data-line-blank-line-flush" ||
			stream.Extensions["x-nekiro-max-event-bytes-source"] == nil || stream.Extensions["x-nekiro-limit-default"] != false {
			t.Fatalf("%s SSE framing/limit is incomplete: %#v", test.path, stream.Extensions)
		}
		success := operation.Responses.Status(200).Value.Extensions
		if success["x-nekiro-max-agent-response-bytes-source"] == nil ||
			success["x-nekiro-max-a2a-event-bytes-source"] == nil || success["x-nekiro-limit-default"] != false {
			t.Fatalf("%s Agent response/A2A limits must be separate required no-default sources: %#v", test.path, success)
		}
		if operation.Extensions["x-nekiro-media-negotiation"] != "invocation-result-v1" {
			t.Fatalf("%s does not declare the shared media rule", test.path)
		}
	}

	if RuntimeDeadlineMinimumMS != 1 || RuntimeDeadlineMaximumMS != 600000 ||
		RuntimeByteLimitMinimum != 1 || RuntimeByteLimitMaximum != 2147483647 {
		t.Fatal("Go runtime limit mapping differs from the language-neutral contract ranges")
	}
}

func TestRuntimeContractWorkspaceScopedProjectionAndLineageReads(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	record := InvocationRecordV4{
		InvocationID: "inv-1", RootTaskID: "task-1", TraceID: "trace-1",
		Caller: Caller{Type: "user", ID: "user-1"}, WorkspaceID: "workspace-1",
		TargetAgentID: "agent-1", AgentCardVersion: "1.0.0", Capability: "summarize",
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	event := InvocationEventV03{
		SchemaVersion: "0.3", EventID: "event-1", Sequence: 0, OccurredAt: now.Format(time.RFC3339),
		Type: "created", Status: "pending", InvocationID: "inv-1", RootTaskID: "task-1", TraceID: "trace-1",
		Caller: record.Caller, WorkspaceID: "workspace-1", TargetAgentID: "agent-1",
		AgentCardVersion: "1.0.0", Capability: "summarize",
	}

	for _, test := range []struct {
		path, invocationRoute, traceRoute string
	}{
		{"openapi/control-plane-invocation.v4.yaml", "/v4/workspaces/{workspaceId}/invocations/{invocationId}", "/v4/workspaces/{workspaceId}/traces/{traceId}"},
		{"openapi/router-internal.v3.yaml", "/internal/v3/workspaces/{workspaceId}/invocations/{invocationId}", "/internal/v3/workspaces/{workspaceId}/traces/{traceId}"},
	} {
		document := loadOpenAPIDocument(t, filepath.FromSlash(test.path))
		invocationOperation := document.Paths.Find(test.invocationRoute)
		traceOperation := document.Paths.Find(test.traceRoute)
		if invocationOperation == nil || invocationOperation.Get == nil || traceOperation == nil || traceOperation.Get == nil {
			t.Fatalf("%s missing Workspace-scoped metadata reads", test.path)
		}
		assertOperationHasPathParameter(t, invocationOperation.Get.Parameters, "workspaceId")
		assertOperationHasPathParameter(t, traceOperation.Get.Parameters, "workspaceId")
		validateOpenAPIValue(t, invocationOperation.Get.Responses.Status(200).Value.Content["application/json"].Schema, InvocationDetailResponseV4{Invocation: record, Events: []InvocationEventV03{event}})
		validateOpenAPIValue(t, traceOperation.Get.Responses.Status(200).Value.Content["application/json"].Schema, TraceResponseV4{TraceID: "trace-1", Invocations: []InvocationRecordV4{record}})
	}

	northbound := loadOpenAPIDocument(t, filepath.FromSlash("openapi/control-plane-invocation.v4.yaml"))
	if northbound.Paths.Find("/v4/invocations/{invocationId}") != nil || northbound.Paths.Find("/v4/traces/{traceId}") != nil {
		t.Fatal("Northbound v4 must not expose unscoped raw metadata routes")
	}

	detail := InvocationDetailResponseV4{Invocation: record, Events: []InvocationEventV03{event}}
	validator, err := NewRuntimeContractValidator()
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.ValidateInvocationDetailResponseV4("workspace-1", detail); err != nil {
		t.Fatalf("valid Invocation detail rejected: %v", err)
	}
	detail.Invocation.Status = "running"
	if validator.ValidateInvocationDetailResponseV4("workspace-1", detail) == nil {
		t.Fatal("Invocation projection status mismatch was accepted")
	}
	detail.Invocation.Status = "pending"
	detail.Invocation.WorkspaceID = "workspace-other"
	if validator.ValidateInvocationDetailResponseV4("workspace-1", detail) == nil {
		t.Fatal("cross-Workspace Invocation projection was accepted")
	}

	trace := TraceResponseV4{TraceID: "trace-1", Invocations: []InvocationRecordV4{record}}
	if err := ValidateTraceResponseV4("workspace-1", "trace-1", trace); err != nil {
		t.Fatalf("valid Trace projection rejected: %v", err)
	}
	trace.Invocations[0].WorkspaceID = "workspace-other"
	if ValidateTraceResponseV4("workspace-1", "trace-1", trace) == nil {
		t.Fatal("cross-Workspace Trace projection was accepted")
	}
}

func TestRuntimeContractExecutableConformanceCorpus(t *testing.T) {
	t.Parallel()

	validator, err := NewRuntimeContractValidator()
	if err != nil {
		t.Fatalf("create runtime contract validator: %v", err)
	}

	var media struct {
		Cases []struct {
			ID     string               `json:"id"`
			Stream bool                 `json:"stream"`
			Accept string               `json:"accept"`
			Valid  bool                 `json:"valid"`
			Mode   InvocationResultMode `json:"mode"`
		} `json:"cases"`
	}
	readRuntimeCorpus(t, "media.json", &media)
	for _, test := range media.Cases {
		mode, err := NegotiateInvocationResultMode(test.Stream, test.Accept)
		if test.Valid && (err != nil || mode != test.Mode) {
			t.Errorf("media %s = %q, %v; want %q", test.ID, mode, err, test.Mode)
		}
		if !test.Valid && !errors.Is(err, ErrRuntimeMediaNotAcceptable) {
			t.Errorf("media %s error = %v, want not acceptable", test.ID, err)
		}
	}

	var errorCases struct {
		Cases []struct {
			ID, Phase string
			Valid     bool
			Error     json.RawMessage `json:"error"`
		} `json:"cases"`
	}
	readRuntimeCorpus(t, "errors.json", &errorCases)
	for _, test := range errorCases.Cases {
		var err error
		if test.Phase == "pre" {
			err = validator.ValidatePreCorrelationPlatformErrorV4JSON(test.Error)
		} else {
			err = validator.ValidateCorrelatedPlatformErrorV4JSON(test.Error)
		}
		if (err == nil) != test.Valid {
			t.Errorf("error corpus %s valid=%v, error=%v", test.ID, test.Valid, err)
		}
	}

	var nested struct {
		Cases []struct {
			ID     string             `json:"id"`
			Valid  bool               `json:"valid"`
			Parent InvocationRecordV4 `json:"parent"`
			Child  InvocationEventV03 `json:"child"`
		} `json:"cases"`
	}
	readRuntimeCorpus(t, "nested.json", &nested)
	for _, test := range nested.Cases {
		err := ValidateNestedInvocationCorrelation(test.Parent, test.Child)
		if (err == nil) != test.Valid {
			t.Errorf("nested corpus %s valid=%v, error=%v", test.ID, test.Valid, err)
		}
	}

	var lifecycle struct {
		Cases []struct {
			ID     string               `json:"id"`
			Valid  bool                 `json:"valid"`
			Events []InvocationEventV03 `json:"events"`
		} `json:"cases"`
	}
	readRuntimeCorpus(t, "lifecycle.json", &lifecycle)
	for _, test := range lifecycle.Cases {
		sequence, err := NewRuntimeInvocationSequenceValidator(validator)
		if err != nil {
			t.Fatalf("create lifecycle validator: %v", err)
		}
		for _, event := range test.Events {
			if err = sequence.Accept(event); err != nil {
				break
			}
		}
		if (err == nil) != test.Valid {
			t.Errorf("lifecycle corpus %s valid=%v, error=%v", test.ID, test.Valid, err)
		}
	}

	var resultStream struct {
		Cases []struct {
			ID     string                          `json:"id"`
			Valid  bool                            `json:"valid"`
			Events []InvocationResultStreamEventV2 `json:"events"`
		} `json:"cases"`
	}
	readRuntimeCorpus(t, "result-stream.json", &resultStream)
	for _, test := range resultStream.Cases {
		sequence, err := NewRuntimeResultStreamSequenceValidator(validator, "inv-1", "task-1", "trace-1")
		if err != nil {
			t.Fatalf("create result stream validator: %v", err)
		}
		for _, event := range test.Events {
			if err = sequence.Accept(event); err != nil {
				break
			}
		}
		if err == nil {
			err = sequence.Finish()
		}
		if (err == nil) != test.Valid {
			t.Errorf("result stream corpus %s valid=%v, error=%v", test.ID, test.Valid, err)
		}
	}

	var projection struct {
		DetailCases []struct {
			ID          string                     `json:"id"`
			WorkspaceID string                     `json:"workspaceId"`
			Valid       bool                       `json:"valid"`
			Detail      InvocationDetailResponseV4 `json:"detail"`
		} `json:"detailCases"`
		TraceCases []struct {
			ID          string          `json:"id"`
			WorkspaceID string          `json:"workspaceId"`
			TraceID     TraceID         `json:"traceId"`
			Valid       bool            `json:"valid"`
			Response    TraceResponseV4 `json:"response"`
		} `json:"traceCases"`
	}
	readRuntimeCorpus(t, "projection.json", &projection)
	for _, test := range projection.DetailCases {
		err := validator.ValidateInvocationDetailResponseV4(test.WorkspaceID, test.Detail)
		if (err == nil) != test.Valid {
			t.Errorf("detail projection corpus %s valid=%v, error=%v", test.ID, test.Valid, err)
		}
	}
	for _, test := range projection.TraceCases {
		err := ValidateTraceResponseV4(test.WorkspaceID, test.TraceID, test.Response)
		if (err == nil) != test.Valid {
			t.Errorf("Trace projection corpus %s valid=%v, error=%v", test.ID, test.Valid, err)
		}
	}
}

func TestRuntimeContractCorpusManifestIsCompleteAndEmbedded(t *testing.T) {
	t.Parallel()

	var manifest struct {
		SchemaVersion string   `json:"schemaVersion"`
		Fixtures      []string `json:"fixtures"`
	}
	readRuntimeCorpus(t, "manifest.json", &manifest)
	if manifest.SchemaVersion != "1" {
		t.Fatalf("runtime corpus schemaVersion = %q", manifest.SchemaVersion)
	}
	want := []string{"errors.json", "lifecycle.json", "media.json", "nested.json", "projection.json", "result-stream.json"}
	slices.Sort(manifest.Fixtures)
	if !slices.Equal(manifest.Fixtures, want) {
		t.Fatalf("runtime corpus fixtures = %v, want %v", manifest.Fixtures, want)
	}
	assertEmbeddedContractFile(t, contractFiles, "invocation-runtime/v1/semantic-rules.md")
	assertEmbeddedContractFile(t, contractFiles, "invocation-runtime/v1/conformance/manifest.json")
	for _, fixture := range want {
		assertEmbeddedContractFile(t, contractFiles, filepath.ToSlash(filepath.Join("invocation-runtime", "v1", "conformance", fixture)))
	}
}

func TestRuntimeContractPostAcceptanceErrorsRequireCorrelation(t *testing.T) {
	t.Parallel()

	validator, err := NewRuntimeContractValidator()
	if err != nil {
		t.Fatalf("create runtime validator: %v", err)
	}
	pre, err := NewPreCorrelationPlatformErrorV4(ErrorCodePayloadTooLarge, "trace-1")
	if err != nil || validator.ValidatePreCorrelationPlatformErrorV4(pre) != nil {
		t.Fatalf("valid pre-correlation error rejected: %v, %#v", err, pre)
	}
	correlated, err := NewCorrelatedPlatformErrorV4(ErrorCodeAgentResponseTooLarge, "trace-1", "inv-1", "task-1")
	if err != nil || validator.ValidateCorrelatedPlatformErrorV4(correlated) != nil {
		t.Fatalf("valid correlated error rejected: %v, %#v", err, correlated)
	}
	correlated.RootTaskID = ""
	if validator.ValidateCorrelatedPlatformErrorV4(correlated) == nil {
		t.Fatal("post-acceptance error without root Task correlation was accepted")
	}

	document := loadOpenAPIDocument(t, filepath.FromSlash("openapi/router-internal.v3.yaml"))
	phase := document.Components.Schemas["PhasePlatformError"].Value.Extensions
	if phase["x-nekiro-phase-boundary"] != "successful-created-commit" ||
		phase["x-nekiro-pre-acceptance-schema"] != "PreCorrelationPlatformError" ||
		phase["x-nekiro-post-acceptance-schema"] != "CorrelatedPlatformError" {
		t.Fatalf("phase error schema does not bind correlation to acceptance: %#v", phase)
	}
	agentFailure := document.Paths.Find("/internal/v3/invocations").Post.Responses.Status(502).Value.Content["application/json"].Schema
	valid := CorrelatedPlatformErrorV4{Code: ErrorCodeAgentAuthUnsupported, Message: platformErrorV4Messages[ErrorCodeAgentAuthUnsupported], TraceID: "trace-1", InvocationID: "inv-1", RootTaskID: "task-1"}
	validateOpenAPIValue(t, agentFailure, valid)
}

func TestRuntimeContractStreamV2ValidatorRequiresCorrelatedError(t *testing.T) {
	t.Parallel()

	validator, err := NewRuntimeContractValidator()
	if err != nil {
		t.Fatalf("create runtime validator: %v", err)
	}
	platformError, err := NewCorrelatedPlatformErrorV4(ErrorCodeAgentResponseTooLarge, "trace-1", "inv-1", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	event := InvocationResultStreamEventV2{
		SchemaVersion: "2", Sequence: 1, Type: ResultStreamEventFailed, Status: "failed",
		InvocationID: "inv-1", RootTaskID: "task-1", TraceID: "trace-1", Error: &platformError,
	}
	if err := validator.ValidateInvocationResultStreamEventV2(event); err != nil {
		t.Fatalf("valid Stream Event v2 rejected: %v", err)
	}
	event.Error.RootTaskID = ""
	if validator.ValidateInvocationResultStreamEventV2(event) == nil {
		t.Fatal("Stream Event v2 accepted an uncorrelated post-acceptance error")
	}
	event.Error.RootTaskID = "task-1"
	event.Error.InvocationID = "inv-other"
	if validator.ValidateInvocationResultStreamEventV2(event) == nil {
		t.Fatal("Stream Event v2 accepted nested error correlation different from its outer event")
	}
	one := int64(1)
	ledgerEvent := InvocationEventV03{
		SchemaVersion: "0.3", EventID: "event-1", Sequence: 1, OccurredAt: "2026-07-16T00:00:00Z",
		Type: "failed", Status: "failed", InvocationID: "inv-1", RootTaskID: "task-1", TraceID: "trace-1",
		Caller: Caller{Type: "user", ID: "user-1"}, WorkspaceID: "workspace-1", TargetAgentID: "agent-1",
		AgentCardVersion: "1.0.0", Capability: "summarize", LatencyMS: &one, Error: event.Error,
	}
	if validator.ValidateInvocationEventV03(ledgerEvent) == nil {
		t.Fatal("Invocation Event 0.3 accepted nested error correlation different from its outer event")
	}
}

func TestRuntimeContractSchemasAndContentExclusion(t *testing.T) {
	t.Parallel()

	document := loadOpenAPIDocument(t, filepath.FromSlash("openapi/router-internal.v3.yaml"))
	request := DispatchInvocationRequestV3{
		InvocationID: "inv-1", RootTaskID: "task-1", TraceID: "trace-1",
		Caller: Caller{Type: "user", ID: "user-1"}, WorkspaceID: "workspace-1",
		TargetAgentID: "agent-1", AgentCardVersion: "1.0.0", Capability: "summarize",
		Input: json.RawMessage(`{"text":"value"}`), Stream: false,
	}
	validateOpenAPIValue(t, document.Components.Schemas["DispatchInvocationRequest"], request)

	for _, schemaPath := range []string{
		"schemas/platform-error.v4.schema.json",
		"schemas/invocation-event.v0.3.schema.json",
		"schemas/invocation-result-stream-event.v2.schema.json",
	} {
		data, err := os.ReadFile(schemaPath)
		if err != nil {
			t.Fatalf("read %s: %v", schemaPath, err)
		}
		lower := strings.ToLower(string(data))
		for _, forbidden := range []string{"apikey", "credentiallocator", "rawdependency", "stacktrace", "endpoint\""} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s contains forbidden metadata field %q", schemaPath, forbidden)
			}
		}
	}

	eventSchema := readContractJSONObject(t, "schemas/invocation-event.v0.3.schema.json")
	properties := requiredJSONObject(t, eventSchema, "properties")
	assertObjectKeysAbsent(t, "Invocation Event 0.3", properties, "input", "result", "chunk", "output", "payload", "endpoint", "credential")
}

func TestRuntimeContractPolicyFreezesAcceptanceAndInterruption(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.FromSlash("../docs/decisions/0006-invocation-runtime-trust-and-failure-policy.md"))
	if err != nil {
		t.Fatalf("read ADR 0006: %v", err)
	}
	text := string(data)
	for _, required := range []string{
		"`created` append/projection transaction is the\naccepted-Invocation boundary",
		"last committed non-terminal fact",
		"does not fabricate a terminal event/projection",
		"at most one `tasks/cancel` request",
		"first successfully committed terminal Ledger\ntransaction wins",
		"have no defaults",
		"exactly one `data:` line",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("ADR 0006 missing policy evidence %q", required)
		}
	}
}

func TestRuntimeContractHistoricalArtifactsRemainHistorical(t *testing.T) {
	t.Parallel()

	for _, test := range []struct{ path, version string }{
		{"openapi/control-plane.v3.yaml", "3.0.0"},
		{"openapi/router-internal.v2.yaml", "2.0.0"},
	} {
		document := loadOpenAPIDocument(t, filepath.FromSlash(test.path))
		if document.Info.Version != test.version {
			t.Fatalf("historical %s version changed to %s", test.path, document.Info.Version)
		}
	}
	compatibility, err := os.ReadFile(filepath.FromSlash("../docs/contracts/compatibility.md"))
	if err != nil {
		t.Fatalf("read compatibility guide: %v", err)
	}
	for _, required := range []string{"invocation-only", "Catalog, Workspace, and Installation", "not a second fact", "Do not run v3/v4"} {
		if !strings.Contains(string(compatibility), required) {
			t.Fatalf("compatibility guide missing %q", required)
		}
	}
}

func assertExtensionStringSliceContains(t *testing.T, extensions map[string]any, key, want string) {
	t.Helper()
	value, exists := extensions[key]
	if !exists {
		t.Fatalf("missing extension %s", key)
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("extension %s type = %s, value %#v", key, reflect.TypeOf(value), value)
	}
	for _, item := range items {
		if item == want {
			return
		}
	}
	t.Fatalf("extension %s = %#v, missing %s", key, value, want)
}

func assertOperationHasPathParameter(t *testing.T, parameters openapi3.Parameters, name string) {
	t.Helper()
	for _, parameter := range parameters {
		if parameter.Value != nil && parameter.Value.In == "path" && parameter.Value.Name == name && parameter.Value.Required {
			return
		}
	}
	t.Fatalf("missing required path parameter %s", name)
}

func readRuntimeCorpus(t *testing.T, name string, destination any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("invocation-runtime", "v1", "conformance", name))
	if err != nil {
		t.Fatalf("read runtime corpus %s: %v", name, err)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		t.Fatalf("decode runtime corpus %s: %v", name, err)
	}
}
