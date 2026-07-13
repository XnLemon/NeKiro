package contracts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestInvocationEventV02TerminalCoherence(t *testing.T) {
	validator := mustResultContractValidator(t)
	agentFailure := mustCorrelatedPlatformErrorV2(t, ErrorCodeAgentExecutionFailed)
	timeout := mustCorrelatedPlatformErrorV2(t, ErrorCodeTimeout)
	canceled := mustCorrelatedPlatformErrorV2(t, ErrorCodeCanceled)

	validCases := []InvocationEventV02{
		validInvocationEventV02("succeeded", "succeeded", nil),
		validInvocationEventV02("failed", "failed", &agentFailure),
		validInvocationEventV02("canceled", "canceled", &canceled),
		validInvocationEventV02("timed_out", "timed_out", &timeout),
	}
	for _, event := range validCases {
		if err := validator.ValidateInvocationEvent(event); err != nil {
			t.Fatalf("valid %s terminal event rejected: %v", event.Type, err)
		}
	}

	invalidCases := []InvocationEventV02{
		validInvocationEventV02("failed", "failed", &timeout),
		validInvocationEventV02("failed", "failed", &canceled),
		validInvocationEventV02("failed", "timed_out", &agentFailure),
		validInvocationEventV02("canceled", "canceled", &agentFailure),
		validInvocationEventV02("timed_out", "timed_out", &canceled),
		validInvocationEventV02("succeeded", "succeeded", &agentFailure),
		validInvocationEventV02("failed", "failed", nil),
	}
	for _, event := range invalidCases {
		if err := validator.ValidateInvocationEvent(event); err == nil {
			t.Fatalf("contradictory terminal event was accepted: type=%s status=%s error=%v", event.Type, event.Status, event.Error)
		}
	}
}

func TestInvocationEventV02RejectsMismatchedErrorCorrelation(t *testing.T) {
	validator := mustResultContractValidator(t)
	testCases := []struct {
		name   string
		mutate func(*PlatformErrorV2)
	}{
		{name: "invocation id", mutate: func(platformError *PlatformErrorV2) { platformError.InvocationID = "inv-other" }},
		{name: "root task id", mutate: func(platformError *PlatformErrorV2) { platformError.RootTaskID = "task-other" }},
		{name: "trace id", mutate: func(platformError *PlatformErrorV2) { platformError.TraceID = "trace-other" }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			platformError := mustCorrelatedPlatformErrorV2(t, ErrorCodeAgentExecutionFailed)
			testCase.mutate(&platformError)
			event := validInvocationEventV02("failed", "failed", &platformError)
			if err := validator.ValidateInvocationEvent(event); err == nil || !strings.Contains(err.Error(), "correlation changed") {
				t.Fatalf("mismatched %s error = %v, want correlation rejection", testCase.name, err)
			}
		})
	}
}

func TestDirectionalOpenAPIOwnership(t *testing.T) {
	controlPlane := loadResultOpenAPIDocument(t, filepath.Join("openapi", "control-plane-internal.v1.yaml"))
	router := loadResultOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))

	if controlPlane.Paths.Len() != 1 || controlPlane.Paths.Find("/internal/v1/resolve-agent") == nil {
		t.Fatalf("Control Plane internal paths = %v, want resolution only", controlPlane.Paths.Keys())
	}
	if controlPlane.Paths.Find("/internal/v2/invocations") != nil {
		t.Fatal("Control Plane internal API contains Router-owned dispatch")
	}
	resolveAgent := controlPlane.Paths.Find("/internal/v1/resolve-agent").Post
	assertDeterministicErrorCodeStatuses(t, resolveAgent)
	assertResponseErrorCode(t, resolveAgent, 404, "AGENT_NOT_INSTALLED")
	assertResponseOmitsErrorCode(t, resolveAgent, 403, "AGENT_NOT_INSTALLED")
	if router.Paths.Find("/internal/v1/resolve-agent") != nil {
		t.Fatal("Router internal API contains Control Plane-owned resolution")
	}
	for _, path := range []string{
		"/internal/v2/invocations",
		"/internal/v2/invocations/{invocationId}",
		"/internal/v2/invocations/{invocationId}/events",
		"/internal/v2/traces/{traceId}",
	} {
		if router.Paths.Find(path) == nil {
			t.Fatalf("Router internal API is missing %s", path)
		}
	}

	controlDestination := controlPlane.Servers[0].URL
	routerDestination := router.Servers[0].URL
	if controlDestination == routerDestination {
		t.Fatalf("internal API destinations are identical: %s", controlDestination)
	}
	if !strings.Contains(controlDestination, "control-plane") || !strings.Contains(routerDestination, "a2a-router") {
		t.Fatalf("internal destinations do not identify their owners: %s, %s", controlDestination, routerDestination)
	}
	if strings.Contains(controlDestination, "localhost") || strings.Contains(routerDestination, "localhost") {
		t.Fatal("active internal API defines a localhost destination fallback")
	}

	resolvedCard := controlPlane.Paths.Find("/internal/v1/resolve-agent").Post.Responses.Status(200).Value.Content["application/json"].Schema.Value.Properties["card"]
	if resolvedCard == nil || resolvedCard.Value == nil || resolvedCard.Value.Title != "NeKiro Agent Card v0.2" {
		t.Fatal("Control Plane resolution does not use Agent Card v0.2")
	}
}

func TestInvocationOpenAPIResultMediaAndStatusMapping(t *testing.T) {
	northbound := loadResultOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v2.yaml"))
	router := loadResultOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))

	assertDirectResultOperation(t, northbound, "/v2/workspaces/{workspaceId}/invocations")
	assertDirectResultOperation(t, router, "/internal/v2/invocations")
	northboundInvocation := northbound.Paths.Find("/v2/workspaces/{workspaceId}/invocations").Post
	routerInvocation := router.Paths.Find("/internal/v2/invocations").Post
	assertDeterministicErrorCodeStatuses(t, northboundInvocation)
	assertDeterministicErrorCodeStatuses(t, routerInvocation)
	assertResponseErrorCode(t, northboundInvocation, 404, "AGENT_NOT_INSTALLED")
	assertResponseOmitsErrorCode(t, northboundInvocation, 403, "AGENT_NOT_INSTALLED")
	assertResponseErrorCode(t, northboundInvocation, 503, "ROUTE_NOT_FOUND")
	assertResponseOmitsErrorCode(t, northboundInvocation, 404, "ROUTE_NOT_FOUND")
	assertResponseErrorCode(t, routerInvocation, 503, "ROUTE_NOT_FOUND")
	assertResponseOmitsErrorCode(t, routerInvocation, 404, "ROUTE_NOT_FOUND")

	catalogCard := northbound.Components.Schemas["CatalogEntry"].Value.Properties["card"]
	if catalogCard == nil || catalogCard.Value == nil || catalogCard.Value.Title != "NeKiro Agent Card v0.2" {
		t.Fatal("Northbound v2 does not reference Agent Card v0.2")
	}
	if strings.Contains(northbound.Servers[0].URL, "localhost") {
		t.Fatal("Northbound v2 defines a localhost destination fallback")
	}
}

func TestInvocationOpenAPIsExposeOnlyMetadataLedgerReads(t *testing.T) {
	northbound := loadResultOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v2.yaml"))
	router := loadResultOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))

	for _, document := range []*openapi3.T{northbound, router} {
		for _, path := range document.Paths.Keys() {
			lowerPath := strings.ToLower(path)
			if strings.Contains(lowerPath, "result") || strings.Contains(lowerPath, "replay") {
				t.Fatalf("active API defines a result persistence/replay path: %s", path)
			}
		}
	}

	northboundLedger := northbound.Paths.Find("/v2/invocations/{invocationId}").Get.Responses.Status(200).Value.Content["application/json"].Schema.Value
	eventSchema := northboundLedger.Properties["events"].Value.Items
	if eventSchema == nil || eventSchema.Value == nil || eventSchema.Value.Title != "NeKiro Invocation Event v0.2" {
		t.Fatal("Northbound Ledger read is not backed by Invocation Event v0.2")
	}
	routerLedger := router.Paths.Find("/internal/v2/invocations/{invocationId}").Get.Responses.Status(200).Value.Content["application/json"].Schema.Value.Items
	if routerLedger == nil || routerLedger.Value == nil || routerLedger.Value.Title != "NeKiro Invocation Event v0.2" {
		t.Fatal("Router Ledger read is not backed by Invocation Event v0.2")
	}
}

func assertDirectResultOperation(t *testing.T, document *openapi3.T, path string) {
	t.Helper()
	pathItem := document.Paths.Find(path)
	if pathItem == nil || pathItem.Post == nil {
		t.Fatalf("direct result POST is missing at %s", path)
	}
	operation := pathItem.Post
	accept := operation.Parameters.GetByInAndName("header", "Accept")
	if accept == nil || !accept.Required {
		t.Fatalf("%s does not require the Accept header contract", path)
	}
	stream := operation.RequestBody.Value.Content["application/json"].Schema.Value.Properties["stream"]
	if stream == nil || stream.Value == nil || stream.Value.Type == nil || !stream.Value.Type.Is("boolean") {
		t.Fatalf("%s request does not define the stream selector", path)
	}

	response := operation.Responses.Status(200)
	if response == nil || response.Value == nil {
		t.Fatalf("%s does not define a 200 direct result response", path)
	}
	jsonResult := response.Value.Content["application/json"]
	if jsonResult == nil || jsonResult.Schema == nil || jsonResult.Schema.Value == nil || jsonResult.Schema.Value.Title != "NeKiro Invocation Result v1" {
		t.Fatalf("%s does not map JSON success to Invocation Result v1", path)
	}
	streamResult := response.Value.Content["text/event-stream"]
	if streamResult == nil || streamResult.Extensions["x-sse-data-schema"] == nil {
		t.Fatalf("%s does not map SSE data to a result stream event schema", path)
	}

	for _, status := range []int{400, 403, 404, 406, 409, 502, 503, 504} {
		if operation.Responses.Status(status) == nil {
			t.Fatalf("%s is missing status %d", path, status)
		}
	}
	if operation.Responses.Status(202) != nil {
		t.Fatalf("%s still exposes historical 202 acceptance", path)
	}
	assertResponseErrorCode(t, operation, 406, "NOT_ACCEPTABLE")
	assertResponseErrorCode(t, operation, 409, "CANCELED")
	assertResponseErrorCode(t, operation, 502, "A2A_PROTOCOL_ERROR")
	assertResponseErrorCode(t, operation, 503, "DEPENDENCY_ERROR")
	assertResponseErrorCode(t, operation, 504, "TIMEOUT")

	description := strings.ToLower(operation.Description)
	for _, required := range []string{"must agree", "stream=false", "stream=true", "406", "not persisted", "replay"} {
		if !strings.Contains(description, required) {
			t.Fatalf("%s operation description is missing %q", path, required)
		}
	}
}

func assertResponseErrorCode(t *testing.T, operation *openapi3.Operation, status int, code string) {
	t.Helper()
	for _, candidate := range responseErrorCodes(t, operation, status) {
		if candidate == code {
			return
		}
	}
	t.Fatalf("response %d error codes do not contain %s", status, code)
}

func assertResponseOmitsErrorCode(t *testing.T, operation *openapi3.Operation, status int, code string) {
	t.Helper()
	for _, candidate := range responseErrorCodes(t, operation, status) {
		if candidate == code {
			t.Fatalf("response %d unexpectedly contains %s", status, code)
		}
	}
}

func assertDeterministicErrorCodeStatuses(t *testing.T, operation *openapi3.Operation) {
	t.Helper()
	seen := make(map[string]int)
	for _, status := range []int{400, 401, 403, 404, 406, 409, 502, 503, 504} {
		if operation.Responses.Status(status) == nil {
			continue
		}
		for _, code := range responseErrorCodes(t, operation, status) {
			if previousStatus, exists := seen[code]; exists {
				t.Fatalf("error code %s appears under both %d and %d", code, previousStatus, status)
			}
			seen[code] = status
		}
	}
}

func responseErrorCodes(t *testing.T, operation *openapi3.Operation, status int) []string {
	t.Helper()
	response := operation.Responses.Status(status)
	if response == nil || response.Value == nil {
		t.Fatalf("response %d is missing", status)
	}
	encoded, err := json.Marshal(response.Value.Extensions["x-platform-error-codes"])
	if err != nil {
		t.Fatalf("marshal response %d error codes: %v", status, err)
	}
	var codes []string
	if err := json.Unmarshal(encoded, &codes); err != nil {
		t.Fatalf("decode response %d error codes: %v", status, err)
	}
	if len(codes) == 0 {
		t.Fatalf("response %d has no error codes", status)
	}
	return codes
}

func loadResultOpenAPIDocument(t *testing.T, path string) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(loader *openapi3.Loader, location *url.URL) ([]byte, error) {
		if location.Scheme == "https" && location.Host == "schemas.nekiro.dev" {
			schemaFiles := map[string]string{
				"/common/v1":                         "schemas/common.v1.schema.json",
				"/agent-card/v0.2":                   "schemas/agent-card.v0.2.schema.json",
				"/installation/v1":                   "schemas/installation.v1.schema.json",
				"/platform-error/v2":                 "schemas/platform-error.v2.schema.json",
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

func mustCorrelatedPlatformErrorV2(t *testing.T, code PlatformErrorCode) PlatformErrorV2 {
	t.Helper()
	platformError, err := NewCorrelatedPlatformErrorV2(code, "trace-event", "inv-event", "task-event")
	if err != nil {
		t.Fatalf("create %s error: %v", code, err)
	}
	return platformError
}
