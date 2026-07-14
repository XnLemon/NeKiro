package contracts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
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
	controlPlane := loadResultOpenAPIDocument(t, filepath.Join("openapi", "control-plane-internal.v2.yaml"))
	router := loadResultOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))

	if controlPlane.Paths.Len() != 1 || controlPlane.Paths.Find("/internal/v2/resolve-agent") == nil {
		t.Fatalf("Control Plane internal paths = %v, want resolution only", controlPlane.Paths.Keys())
	}
	if controlPlane.Paths.Find("/internal/v2/invocations") != nil {
		t.Fatal("Control Plane internal API contains Router-owned dispatch")
	}
	resolveAgent := controlPlane.Paths.Find("/internal/v2/resolve-agent").Post
	assertDeterministicErrorCodeStatuses(t, resolveAgent)
	assertResponseErrorCode(t, resolveAgent, 404, "AGENT_NOT_INSTALLED")
	assertResponseOmitsErrorCode(t, resolveAgent, 403, "AGENT_NOT_INSTALLED")
	if router.Paths.Find("/internal/v2/resolve-agent") != nil {
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

	resolvedCard := controlPlane.Paths.Find("/internal/v2/resolve-agent").Post.Responses.Status(200).Value.Content["application/json"].Schema.Value.Properties["card"]
	if resolvedCard == nil || resolvedCard.Value == nil || resolvedCard.Value.Title != "NeKiro Agent Card v0.2" {
		t.Fatal("Control Plane resolution does not use Agent Card v0.2")
	}
}

func TestResolveAgentOpenAPIPreservesExistingCorrelation(t *testing.T) {
	controlPlane := loadResultOpenAPIDocument(t, filepath.Join("openapi", "control-plane-internal.v2.yaml"))
	operation := controlPlane.Paths.Find("/internal/v2/resolve-agent").Post
	requestSchema := operation.RequestBody.Value.Content["application/json"].Schema
	assertExactStringSet(t, "Resolve Agent required fields", requestSchema.Value.Required, []string{
		"invocationId",
		"rootTaskId",
		"traceId",
		"workspaceId",
		"agentId",
		"version",
		"capability",
	})
	if len(requestSchema.Value.Properties) != 7 {
		t.Fatalf("Resolve Agent request properties = %v, want exactly seven versioned fields", requestSchema.Value.Properties)
	}
	request := ResolveAgentRequestV1{
		InvocationID: "inv-resolve",
		RootTaskID:   "task-root",
		TraceID:      "trace-resolve",
		WorkspaceID:  "workspace-resolve",
		AgentID:      "agent-resolve",
		Version:      "1.2.3",
		Capability:   "answer",
	}
	validateOpenAPIValue(t, requestSchema, request)

	responseCodes := map[int][]string{
		400: {"VALIDATION_ERROR"},
		401: {"UNAUTHENTICATED"},
		403: {"INSTALLATION_DISABLED", "AGENT_DISABLED", "CAPABILITY_NOT_ALLOWED"},
		404: {"AGENT_NOT_INSTALLED"},
		503: {"DEPENDENCY_ERROR"},
	}
	for status, expectedCodes := range responseCodes {
		assertExactResponseErrorCodes(t, operation, status, expectedCodes)
		response := operation.Responses.Status(status)
		if status == 400 || status == 401 {
			assertPreCorrelationResponse(t, status, response)
			continue
		}
		assertExactResponseCorrelation(t, status, response, "request", []string{"invocationId", "rootTaskId", "traceId"})
	}
}

func TestRouterInternalReadAndDispatchUnavailableMappings(t *testing.T) {
	router := loadResultOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))
	dispatch := router.Paths.Find("/internal/v2/invocations").Post
	assertExactResponseErrorCodes(t, dispatch, 503, []string{"ROUTE_NOT_FOUND", "AGENT_UNAVAILABLE", "DEPENDENCY_ERROR"})

	readPaths := []string{
		"/internal/v2/invocations/{invocationId}",
		"/internal/v2/invocations/{invocationId}/events",
		"/internal/v2/traces/{traceId}",
	}
	for _, path := range readPaths {
		t.Run(path, func(t *testing.T) {
			operation := router.Paths.Find(path).Get
			assertExactResponseErrorCodes(t, operation, 503, []string{"DEPENDENCY_ERROR"})
			assertResponseOmitsErrorCode(t, operation, 503, "ROUTE_NOT_FOUND")
			assertResponseOmitsErrorCode(t, operation, 503, "AGENT_UNAVAILABLE")
		})
	}
}

func TestInvocationOpenAPIResultMediaAndStatusMapping(t *testing.T) {
	northbound := loadResultOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v3.yaml"))
	router := loadResultOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))

	assertDirectResultOperation(t, northbound, "/v3/workspaces/{workspaceId}/invocations")
	assertDirectResultOperation(t, router, "/internal/v2/invocations")
	northboundInvocation := northbound.Paths.Find("/v3/workspaces/{workspaceId}/invocations").Post
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
		t.Fatal("Northbound v3 does not reference Agent Card v0.2")
	}
	if strings.Contains(northbound.Servers[0].URL, "localhost") {
		t.Fatal("Northbound v3 defines a localhost destination fallback")
	}
}

func TestInvocationPostCreationErrorsRequireExactCorrelation(t *testing.T) {
	northbound := loadResultOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v3.yaml"))
	router := loadResultOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))
	northboundInvocation := northbound.Paths.Find("/v3/workspaces/{workspaceId}/invocations").Post
	routerInvocation := router.Paths.Find("/internal/v2/invocations").Post

	dispatchRequest := DispatchInvocationRequest{
		InvocationID:     "inv-post-create",
		RootTaskID:       "task-post-create",
		TraceID:          "trace-post-create",
		Caller:           Caller{Type: "user", ID: "user-post-create"},
		WorkspaceID:      "workspace-post-create",
		TargetAgentID:    "agent-post-create",
		AgentCardVersion: "1.2.3",
		Capability:       "answer",
		Input:            map[string]any{"question": "contract"},
		Stream:           false,
	}
	dispatchRequestSchema := routerInvocation.RequestBody.Value.Content["application/json"].Schema
	validateOpenAPIValue(t, dispatchRequestSchema, dispatchRequest)
	for _, field := range []string{"invocationId", "rootTaskId", "traceId"} {
		if !containsString(dispatchRequestSchema.Value.Required, field) {
			t.Fatalf("Router dispatch request does not require %s", field)
		}
	}

	statusCodes := map[int]PlatformErrorCode{
		502: ErrorCodeAgentExecutionFailed,
		503: ErrorCodeDependency,
		504: ErrorCodeTimeout,
	}
	preCreationStatusCodes := map[int]PlatformErrorCode{
		400: ErrorCodeValidationError,
		406: ErrorCodeNotAcceptable,
	}
	operations := []struct {
		name              string
		operation         *openapi3.Operation
		correlationSource string
	}{
		{name: "Northbound", operation: northboundInvocation, correlationSource: "created-invocation-context"},
		{name: "Router", operation: routerInvocation, correlationSource: "request"},
	}
	for _, operationCase := range operations {
		t.Run(operationCase.name, func(t *testing.T) {
			for status, code := range statusCodes {
				t.Run(strconv.Itoa(status), func(t *testing.T) {
					response := operationCase.operation.Responses.Status(status)
					assertExactResponseCorrelation(
						t,
						status,
						response,
						operationCase.correlationSource,
						[]string{"invocationId", "rootTaskId", "traceId"},
					)
					platformError, err := NewCorrelatedPlatformErrorV2(
						code,
						dispatchRequest.TraceID,
						dispatchRequest.InvocationID,
						dispatchRequest.RootTaskID,
					)
					if err != nil {
						t.Fatalf("create correlated response: %v", err)
					}
					validateOpenAPIValue(t, response.Value.Content["application/json"].Schema, platformError)

					uncorrelated, err := NewPlatformErrorV2(code, dispatchRequest.TraceID)
					if err != nil {
						t.Fatalf("create base response: %v", err)
					}
					assertOpenAPIValueRejected(t, response.Value.Content["application/json"].Schema, uncorrelated)
				})
			}
			for status, code := range preCreationStatusCodes {
				t.Run("base-"+strconv.Itoa(status), func(t *testing.T) {
					response := operationCase.operation.Responses.Status(status)
					if response == nil || response.Value == nil {
						t.Fatalf("response %d is missing", status)
					}
					if _, exists := response.Value.Extensions["x-platform-error-correlation"]; exists {
						t.Fatalf("pre-creation response %d declares post-creation correlation", status)
					}
					platformError, err := NewPlatformErrorV2(code, dispatchRequest.TraceID)
					if err != nil {
						t.Fatalf("create base response: %v", err)
					}
					validateOpenAPIValue(t, response.Value.Content["application/json"].Schema, platformError)
				})
			}
		})
	}
}

func TestInvocationOpenAPIsExposeOnlyMetadataLedgerReads(t *testing.T) {
	northbound := loadResultOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v3.yaml"))
	router := loadResultOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))

	for _, operation := range []*openapi3.Operation{
		northbound.Paths.Find("/v3/workspaces/{workspaceId}/invocations").Post,
		northbound.Paths.Find("/v3/invocations/{invocationId}").Get,
		northbound.Paths.Find("/v3/traces/{traceId}").Get,
	} {
		if operation == nil || operation.Security == nil || len(*operation.Security) != 1 {
			t.Fatal("Northbound invocation operation is missing Bearer security")
		}
		if _, exists := (*operation.Security)[0]["bearerAuth"]; !exists {
			t.Fatalf("Northbound invocation security = %#v, want bearerAuth", *operation.Security)
		}
	}

	for _, document := range []*openapi3.T{northbound, router} {
		for _, path := range document.Paths.Keys() {
			lowerPath := strings.ToLower(path)
			if strings.Contains(lowerPath, "result") || strings.Contains(lowerPath, "replay") {
				t.Fatalf("active API defines a result persistence/replay path: %s", path)
			}
		}
	}

	northboundLedger := northbound.Paths.Find("/v3/invocations/{invocationId}").Get.Responses.Status(200).Value.Content["application/json"].Schema.Value
	eventSchema := northboundLedger.Properties["events"].Value.Items
	if eventSchema == nil || eventSchema.Value == nil || eventSchema.Value.Title != "NeKiro Invocation Event v0.2" {
		t.Fatal("Northbound Ledger read is not backed by Invocation Event v0.2")
	}
	routerLedger := router.Paths.Find("/internal/v2/invocations/{invocationId}").Get.Responses.Status(200).Value.Content["application/json"].Schema.Value.Items
	if routerLedger == nil || routerLedger.Value == nil || routerLedger.Value.Title != "NeKiro Invocation Event v0.2" {
		t.Fatal("Router Ledger read is not backed by Invocation Event v0.2")
	}
}

func TestActiveOpenAPIErrorMappingsAreCompleteAndDeterministic(t *testing.T) {
	northbound := loadResultOpenAPIDocument(t, filepath.Join("openapi", "control-plane.v3.yaml"))
	controlPlaneInternal := loadResultOpenAPIDocument(t, filepath.Join("openapi", "control-plane-internal.v2.yaml"))
	routerInternal := loadResultOpenAPIDocument(t, filepath.Join("openapi", "router-internal.v2.yaml"))

	for name, document := range map[string]*openapi3.T{
		"Northbound v3":             northbound,
		"Control Plane Internal v2": controlPlaneInternal,
		"Router Internal v2":        routerInternal,
	} {
		t.Run(name, func(t *testing.T) {
			assertAllOpenAPIErrorMappings(t, document)
		})
	}

	testCases := []struct {
		path   string
		method string
		status int
		codes  []string
	}{
		{path: "/v3/agents", method: "POST", status: 400, codes: []string{"VALIDATION_ERROR"}},
		{path: "/v3/agents", method: "POST", status: 409, codes: []string{"CONFLICT"}},
		{path: "/v3/agents/{agentId}/versions/{version}", method: "GET", status: 404, codes: []string{"NOT_FOUND"}},
		{path: "/v3/agents/{agentId}/versions/{version}/publish", method: "POST", status: 404, codes: []string{"NOT_FOUND"}},
		{path: "/v3/agents/{agentId}/versions/{version}/publish", method: "POST", status: 409, codes: []string{"CONFLICT"}},
		{path: "/v3/agents/{agentId}/versions/{version}/disable", method: "POST", status: 404, codes: []string{"NOT_FOUND"}},
		{path: "/v3/workspaces/{workspaceId}/installations", method: "POST", status: 400, codes: []string{"VALIDATION_ERROR"}},
		{path: "/v3/workspaces/{workspaceId}/installations", method: "POST", status: 404, codes: []string{"NOT_FOUND"}},
		{path: "/v3/workspaces/{workspaceId}/installations", method: "POST", status: 409, codes: []string{"CONFLICT"}},
		{path: "/v3/workspaces/{workspaceId}/installations/{installationId}", method: "PATCH", status: 404, codes: []string{"NOT_FOUND"}},
		{path: "/v3/workspaces/{workspaceId}/installations/{installationId}", method: "DELETE", status: 404, codes: []string{"NOT_FOUND"}},
		{path: "/v3/workspaces/{workspaceId}/invocations", method: "POST", status: 400, codes: []string{"VALIDATION_ERROR"}},
		{path: "/v3/workspaces/{workspaceId}/invocations", method: "POST", status: 401, codes: []string{"UNAUTHENTICATED"}},
		{path: "/v3/workspaces/{workspaceId}/invocations", method: "POST", status: 403, codes: []string{"FORBIDDEN", "AGENT_DISABLED", "CAPABILITY_NOT_ALLOWED"}},
		{path: "/v3/workspaces/{workspaceId}/invocations", method: "POST", status: 404, codes: []string{"NOT_FOUND", "AGENT_NOT_INSTALLED"}},
		{path: "/v3/workspaces/{workspaceId}/invocations", method: "POST", status: 406, codes: []string{"NOT_ACCEPTABLE"}},
		{path: "/v3/workspaces/{workspaceId}/invocations", method: "POST", status: 409, codes: []string{"CONFLICT", "CANCELED"}},
		{path: "/v3/workspaces/{workspaceId}/invocations", method: "POST", status: 502, codes: []string{"AGENT_EXECUTION_FAILED", "A2A_PROTOCOL_ERROR"}},
		{path: "/v3/workspaces/{workspaceId}/invocations", method: "POST", status: 503, codes: []string{"ROUTE_NOT_FOUND", "AGENT_UNAVAILABLE", "DEPENDENCY_ERROR"}},
		{path: "/v3/workspaces/{workspaceId}/invocations", method: "POST", status: 504, codes: []string{"TIMEOUT"}},
		{path: "/v3/invocations/{invocationId}", method: "GET", status: 401, codes: []string{"UNAUTHENTICATED"}},
		{path: "/v3/invocations/{invocationId}", method: "GET", status: 403, codes: []string{"FORBIDDEN", "AGENT_DISABLED", "CAPABILITY_NOT_ALLOWED"}},
		{path: "/v3/invocations/{invocationId}", method: "GET", status: 404, codes: []string{"NOT_FOUND"}},
		{path: "/v3/invocations/{invocationId}", method: "GET", status: 503, codes: []string{"DEPENDENCY_ERROR"}},
		{path: "/v3/traces/{traceId}", method: "GET", status: 401, codes: []string{"UNAUTHENTICATED"}},
		{path: "/v3/traces/{traceId}", method: "GET", status: 403, codes: []string{"FORBIDDEN", "AGENT_DISABLED", "CAPABILITY_NOT_ALLOWED"}},
		{path: "/v3/traces/{traceId}", method: "GET", status: 404, codes: []string{"NOT_FOUND"}},
		{path: "/v3/traces/{traceId}", method: "GET", status: 503, codes: []string{"DEPENDENCY_ERROR"}},
	}
	for _, testCase := range testCases {
		operation := northbound.Paths.Find(testCase.path).GetOperation(testCase.method)
		assertExactResponseErrorCodes(t, operation, testCase.status, testCase.codes)
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
	return responseErrorCodesFromRef(t, fmt.Sprintf("response %d", status), response)
}

func responseErrorCodesFromRef(t *testing.T, label string, response *openapi3.ResponseRef) []string {
	t.Helper()
	if response == nil || response.Value == nil {
		t.Fatalf("%s is missing", label)
	}
	encoded, err := json.Marshal(response.Value.Extensions["x-platform-error-codes"])
	if err != nil {
		t.Fatalf("marshal %s error codes: %v", label, err)
	}
	var codes []string
	if err := json.Unmarshal(encoded, &codes); err != nil {
		t.Fatalf("decode %s error codes: %v", label, err)
	}
	if len(codes) == 0 {
		t.Fatalf("%s has no error codes", label)
	}
	return codes
}

func assertAllOpenAPIErrorMappings(t *testing.T, document *openapi3.T) {
	t.Helper()
	for path, pathItem := range document.Paths.Map() {
		for method, operation := range pathItem.Operations() {
			seen := make(map[string]string)
			for statusText, response := range operation.Responses.Map() {
				status, err := strconv.Atoi(statusText)
				if err != nil {
					t.Fatalf("%s %s has unsupported response key %q", method, path, statusText)
				}
				if status < 400 {
					continue
				}
				label := fmt.Sprintf("%s %s response %s", method, path, statusText)
				for _, code := range responseErrorCodesFromRef(t, label, response) {
					if _, known := platformErrorV3Messages[PlatformErrorCode(code)]; !known {
						t.Fatalf("%s declares unknown error code %s", label, code)
					}
					if previousStatus, exists := seen[code]; exists {
						t.Fatalf("%s %s maps %s under both %s and %s", method, path, code, previousStatus, statusText)
					}
					seen[code] = statusText
				}
			}
		}
	}
	for name, response := range document.Components.Responses {
		label := fmt.Sprintf("component response %s", name)
		for _, code := range responseErrorCodesFromRef(t, label, response) {
			if _, known := platformErrorV3Messages[PlatformErrorCode(code)]; !known {
				t.Fatalf("%s declares unknown error code %s", label, code)
			}
		}
	}
}

func assertExactResponseErrorCodes(t *testing.T, operation *openapi3.Operation, status int, expected []string) {
	t.Helper()
	actual := responseErrorCodes(t, operation, status)
	if len(actual) != len(expected) {
		t.Fatalf("response %d error codes = %v, want %v", status, actual, expected)
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, code := range actual {
		actualSet[code] = struct{}{}
	}
	for _, code := range expected {
		if _, exists := actualSet[code]; !exists {
			t.Fatalf("response %d error codes = %v, want %v", status, actual, expected)
		}
	}
}

func assertExactResponseCorrelation(
	t *testing.T,
	status int,
	response *openapi3.ResponseRef,
	expectedSource string,
	expectedFields []string,
) {
	t.Helper()
	if response == nil || response.Value == nil {
		t.Fatalf("response %d is missing", status)
	}
	encoded, err := json.Marshal(response.Value.Extensions["x-platform-error-correlation"])
	if err != nil {
		t.Fatalf("marshal response %d correlation contract: %v", status, err)
	}
	var correlation struct {
		Source      string   `json:"source"`
		ExactFields []string `json:"exactFields"`
	}
	if err := json.Unmarshal(encoded, &correlation); err != nil {
		t.Fatalf("decode response %d correlation contract: %v", status, err)
	}
	if correlation.Source != expectedSource {
		t.Fatalf("response %d correlation source = %q, want %q", status, correlation.Source, expectedSource)
	}
	assertExactStringSet(t, fmt.Sprintf("response %d exact correlation fields", status), correlation.ExactFields, expectedFields)

	schema := response.Value.Content["application/json"].Schema
	if schema == nil || schema.Value == nil {
		t.Fatalf("response %d Platform Error schema is missing", status)
	}
	if len(schema.Value.AllOf) != 2 || schema.Value.AllOf[1] == nil || schema.Value.AllOf[1].Value == nil {
		t.Fatalf("response %d does not compose the correlated Platform Error schema", status)
	}
	assertExactStringSet(
		t,
		fmt.Sprintf("response %d correlated error required fields", status),
		schema.Value.AllOf[1].Value.Required,
		expectedFields,
	)
}

func assertPreCorrelationResponse(t *testing.T, status int, response *openapi3.ResponseRef) {
	t.Helper()
	if response == nil || response.Value == nil {
		t.Fatalf("response %d is missing", status)
	}
	encoded, err := json.Marshal(response.Value.Extensions["x-platform-error-correlation"])
	if err != nil {
		t.Fatalf("marshal response %d pre-correlation contract: %v", status, err)
	}
	if status == 400 && !strings.Contains(string(encoded), "preCorrelation") {
		t.Fatalf("response %d does not declare pre-correlation behavior: %s", status, encoded)
	}
	schema := response.Value.Content["application/json"].Schema
	if schema == nil || schema.Value == nil {
		t.Fatalf("response %d pre-correlation Platform Error schema is missing", status)
	}
	if status == 400 && len(schema.Value.OneOf) != 2 {
		t.Fatalf("response %d must allow pre- and post-correlation errors", status)
	}
	if status == 401 && len(schema.Value.AllOf) != 2 {
		t.Fatalf("response %d must compose the pre-correlation error schema", status)
	}
}

func assertOpenAPIValueRejected(t *testing.T, schemaRef *openapi3.SchemaRef, value any) {
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
	if err := schemaRef.Value.VisitJSON(document, openapi3.EnableJSONSchema2020()); err == nil {
		t.Fatal("OpenAPI schema accepted an uncorrelated Platform Error")
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func assertExactStringSet(t *testing.T, label string, actual []string, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("%s = %v, want %v", label, actual, expected)
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		if _, exists := actualSet[value]; exists {
			t.Fatalf("%s repeats %q", label, value)
		}
		actualSet[value] = struct{}{}
	}
	for _, value := range expected {
		if _, exists := actualSet[value]; !exists {
			t.Fatalf("%s = %v, want %v", label, actual, expected)
		}
	}
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
				"/installation/v2":                   "schemas/installation.v2.schema.json",
				"/platform-error/v2":                 "schemas/platform-error.v2.schema.json",
				"/platform-error/v3":                 "schemas/platform-error.v3.schema.json",
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
