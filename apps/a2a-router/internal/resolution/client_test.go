package resolution

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nene7ko/NeKiro/contracts"
)

const validResolveResponse = `{"card":{"schemaVersion":"0.2","agentId":"agent-a","name":"Agent","description":"Agent","owner":{"id":"team","displayName":"Team"},"version":"1.0.0","protocol":{"type":"a2a","version":"0.3.0","transport":"JSONRPC","endpoint":"https://agent.example/a2a"},"skills":[{"id":"capability-a","name":"Capability","description":"Capability","inputSchema":{},"outputSchema":{},"requiredPermissions":[]}],"authentication":{"type":"none"},"permissions":[],"limits":{"timeoutMs":1000,"maxInputBytes":1024,"maxOutputBytes":1024,"streaming":true}},"installation":{"installationId":"inst-a","workspaceId":"workspace-a","agentId":"agent-a","installedVersion":"1.0.0","acceptedPermissions":[],"status":"enabled"}}`

func validResolveRequest() contracts.ResolveAgentRequest {
	return contracts.ResolveAgentRequest{
		InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
		WorkspaceID: "workspace-a", AgentID: "agent-a", Version: "1.0.0",
		Capability: "capability-a",
	}
}

func validInstalledVersionRequest() contracts.ResolveInstalledVersionRequest {
	return contracts.ResolveInstalledVersionRequest{
		InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a",
		WorkspaceID: "workspace-a", AgentID: "agent-a", Capability: "capability-a",
	}
}

func TestClientResolveSendsExactInternalV2Request(t *testing.T) {
	requestValue := validResolveRequest()
	var received contracts.ResolveAgentRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/internal/v2/resolve-agent" || request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer control-token" || request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected request: %s %s %#v", request.Method, request.URL.Path, request.Header)
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, validResolveResponse)
	}))
	defer server.Close()
	client, err := NewClient(server.Client(), server.URL+"/internal/v2/resolve-agent", "control-token", 4096)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := client.Resolve(context.Background(), requestValue)
	if err != nil {
		t.Fatal(err)
	}
	if received != requestValue || resolved.Card.AgentID != requestValue.AgentID || resolved.Installation.InstalledVersion != requestValue.Version {
		t.Fatalf("received=%#v resolved=%#v", received, resolved)
	}
}

func TestClientResolveAcceptsExactTrustedProvenancePair(t *testing.T) {
	digest := strings.Repeat("a", 64)
	trustedResponse := strings.Replace(validResolveResponse, `"acceptedPermissions":[]`, `"installedReleaseId":"release-a","agentCardDigest":"`+digest+`","acceptedPermissions":[]`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, trustedResponse)
	}))
	defer server.Close()
	client, err := NewClient(server.Client(), server.URL, "control-token", 4096)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := client.Resolve(context.Background(), validResolveRequest())
	if err != nil || resolved.Installation.InstalledReleaseID != "release-a" || resolved.Installation.AgentCardDigest != digest {
		t.Fatalf("trusted resolution = %#v, %v", resolved, err)
	}
}

func TestClientResolveMapsTypedFailuresAndDependenciesWithoutRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("x-nek-trace-id", "trace-a")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(writer, `{"code":"CAPABILITY_NOT_ALLOWED","message":"The requested capability is not allowed.","traceId":"trace-a","invocationId":"inv-a","rootTaskId":"task-a"}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.Client(), server.URL, "control-token", 1024)
	_, err := client.Resolve(context.Background(), validResolveRequest())
	var failure *Failure
	if !errors.As(err, &failure) || failure.Code != contracts.ErrorCodeCapabilityNotAllowed || failure.StatusCode != http.StatusForbidden || failure.TraceID != "trace-a" || calls != 1 || string(failure.Body) == "" {
		t.Fatalf("failure=%#v err=%v calls=%d", failure, err, calls)
	}
}

func TestClientResolveRejectsSuccessThatChangesAuthorizedFacts(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "changed Card Agent", body: strings.Replace(validResolveResponse, `"agentId":"agent-a"`, `"agentId":"agent-b"`, 1)},
		{name: "changed Workspace", body: strings.Replace(validResolveResponse, `"workspaceId":"workspace-a"`, `"workspaceId":"workspace-b"`, 1)},
		{name: "missing capability", body: strings.Replace(validResolveResponse, `"id":"capability-a"`, `"id":"capability-b"`, 1)},
		{name: "partial Release provenance", body: strings.Replace(validResolveResponse, `"acceptedPermissions":[]`, `"installedReleaseId":"release-a","acceptedPermissions":[]`, 1)},
		{name: "unknown member", body: strings.Replace(validResolveResponse, `{"card":`, `{"unexpected":true,"card":`, 1)},
		{name: "trailing value", body: validResolveResponse + `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()
			client, err := NewClient(server.Client(), server.URL, "control-token", 4096)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Resolve(context.Background(), validResolveRequest()); err == nil {
				t.Fatal("unbound Control Plane success accepted")
			}
		})
	}
}

func TestClientResolveRejectsUnsafeOrMismatchedError(t *testing.T) {
	validBody := `{"code":"CAPABILITY_NOT_ALLOWED","message":"The requested capability is not allowed.","traceId":"trace-a","invocationId":"inv-a","rootTaskId":"task-a"}`
	for _, test := range []struct {
		name   string
		body   string
		header string
	}{
		{name: "unknown member", body: strings.Replace(validBody, `{"code":`, `{"detail":"secret","code":`, 1), header: "trace-a"},
		{name: "non-fixed message", body: strings.Replace(validBody, "The requested capability is not allowed.", "raw dependency detail", 1), header: "trace-a"},
		{name: "changed body trace", body: strings.Replace(validBody, `"traceId":"trace-a"`, `"traceId":"trace-b"`, 1), header: "trace-a"},
		{name: "changed header trace", body: validBody, header: "trace-b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("x-nek-trace-id", test.header)
				writer.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()
			client, err := NewClient(server.Client(), server.URL, "control-token", 1024)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Resolve(context.Background(), validResolveRequest())
			var failure *Failure
			if err == nil || errors.As(err, &failure) {
				t.Fatalf("unsafe Control Plane error accepted: failure=%#v err=%v", failure, err)
			}
		})
	}
}

func TestClientResolveDoesNotFollowRedirects(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalls++
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", target.URL)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	client, err := NewClient(source.Client(), source.URL, "control-token", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Resolve(context.Background(), validResolveRequest()); err == nil {
		t.Fatal("redirect response accepted")
	}
	if targetCalls != 0 {
		t.Fatalf("redirect target calls = %d, want 0", targetCalls)
	}
}

func TestClientResolveRejectsBadMediaAndOversize(t *testing.T) {
	for _, test := range []struct {
		name        string
		contentType string
		body        string
		status      int
	}{
		{name: "bad media", contentType: "text/plain", body: "{}", status: http.StatusOK},
		{name: "oversize", contentType: "application/json", body: `{"code":"DEPENDENCY_ERROR"}`, status: http.StatusOK},
		{name: "missing trace header on error", contentType: "application/json", body: `{"code":"DEPENDENCY_ERROR"}`, status: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", test.contentType)
				writer.WriteHeader(test.status)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()
			client, _ := NewClient(server.Client(), server.URL, "control-token", 4)
			if _, err := client.Resolve(context.Background(), validResolveRequest()); err == nil {
				t.Fatal("invalid Control Plane response accepted")
			}
		})
	}
}

func TestClientResolveInstalledVersionAcceptsDeclaredErrorPhases(t *testing.T) {
	requestValue := validInstalledVersionRequest()
	tests := []struct {
		name       string
		statusCode int
		code       contracts.PlatformErrorCode
		correlated bool
		traceID    contracts.TraceID
	}{
		{name: "pre-correlation bad request", statusCode: http.StatusBadRequest, code: contracts.ErrorCodeValidationError, traceID: "trace-generated"},
		{name: "correlated bad request", statusCode: http.StatusBadRequest, code: contracts.ErrorCodeValidationError, correlated: true, traceID: requestValue.TraceID},
		{name: "pre-correlation unauthenticated", statusCode: http.StatusUnauthorized, code: contracts.ErrorCodeUnauthenticated, traceID: "trace-generated"},
		{name: "correlated forbidden", statusCode: http.StatusForbidden, code: contracts.ErrorCodeCapabilityNotAllowed, correlated: true, traceID: requestValue.TraceID},
		{name: "correlated revoked release", statusCode: http.StatusForbidden, code: contracts.ErrorCodeAgentReleaseRevoked, correlated: true, traceID: requestValue.TraceID},
		{name: "correlated not found", statusCode: http.StatusNotFound, code: contracts.ErrorCodeAgentNotInstalled, correlated: true, traceID: requestValue.TraceID},
		{name: "correlated dependency", statusCode: http.StatusServiceUnavailable, code: contracts.ErrorCodeDependency, correlated: true, traceID: requestValue.TraceID},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var payload contracts.PlatformErrorV3
			var err error
			if test.correlated {
				payload, err = contracts.NewCorrelatedPlatformErrorV3(test.code, test.traceID, requestValue.InvocationID, requestValue.RootTaskID)
			} else {
				payload, err = contracts.NewPlatformErrorV3(test.code, test.traceID)
			}
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("x-nek-trace-id", string(test.traceID))
				writer.WriteHeader(test.statusCode)
				_ = json.NewEncoder(writer).Encode(payload)
			}))
			defer server.Close()
			client, err := NewClientWithVersionURL(server.Client(), server.URL, server.URL, "control-token", 4096)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.ResolveInstalledVersion(context.Background(), requestValue)
			var failure *Failure
			if !errors.As(err, &failure) || failure.StatusCode != test.statusCode || failure.Code != test.code || failure.TraceID != test.traceID {
				t.Fatalf("failure=%#v err=%v", failure, err)
			}
		})
	}
}

func TestClientResolveInstalledVersionValidatesReleaseProvenancePair(t *testing.T) {
	requestValue := validInstalledVersionRequest()
	for _, test := range []struct {
		name  string
		body  string
		valid bool
	}{
		{name: "legacy pair absent", body: `{"version":"1.0.0"}`, valid: true},
		{name: "trusted pair present", body: `{"version":"1.0.0","releaseId":"release-a","agentCardDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, valid: true},
		{name: "release only", body: `{"version":"1.0.0","releaseId":"release-a"}`},
		{name: "digest only", body: `{"version":"1.0.0","agentCardDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`},
		{name: "invalid digest", body: `{"version":"1.0.0","releaseId":"release-a","agentCardDigest":"bad"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("x-nek-trace-id", string(requestValue.TraceID))
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()
			client, err := NewClientWithVersionURL(server.Client(), server.URL, server.URL, "control-token", 4096)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.ResolveInstalledVersion(context.Background(), requestValue)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t error=%v", test.valid, err)
			}
		})
	}
}

func TestClientResolveInstalledVersionRejectsInvalidErrorStatusPhaseAndCorrelation(t *testing.T) {
	requestValue := validInstalledVersionRequest()
	validCorrelated, err := contracts.NewCorrelatedPlatformErrorV3(contracts.ErrorCodeDependency, requestValue.TraceID, requestValue.InvocationID, requestValue.RootTaskID)
	if err != nil {
		t.Fatal(err)
	}
	validPre, err := contracts.NewPlatformErrorV3(contracts.ErrorCodeUnauthenticated, "trace-generated")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		statusCode int
		header     string
		body       string
	}{
		{name: "undeclared status", statusCode: http.StatusInternalServerError, header: "trace-a", body: mustJSON(t, validCorrelated)},
		{name: "wrong status code pair", statusCode: http.StatusForbidden, header: "trace-a", body: mustJSON(t, validCorrelated)},
		{name: "unauthenticated must be pre-correlation", statusCode: http.StatusUnauthorized, header: "trace-a", body: mustJSON(t, correlatedErrorV3(t, contracts.ErrorCodeUnauthenticated, requestValue))},
		{name: "forbidden requires correlation", statusCode: http.StatusForbidden, header: "trace-generated", body: mustJSON(t, preErrorV3(t, contracts.ErrorCodeCapabilityNotAllowed, "trace-generated"))},
		{name: "asymmetric correlation", statusCode: http.StatusBadRequest, header: "trace-a", body: `{"code":"VALIDATION_ERROR","message":"The request is invalid.","traceId":"trace-a","invocationId":"inv-a"}`},
		{name: "changed correlated trace", statusCode: http.StatusServiceUnavailable, header: "trace-other", body: mustJSON(t, correlatedErrorV3WithTrace(t, contracts.ErrorCodeDependency, requestValue, "trace-other"))},
		{name: "header body trace mismatch", statusCode: http.StatusUnauthorized, header: "trace-other", body: mustJSON(t, validPre)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("x-nek-trace-id", test.header)
				writer.WriteHeader(test.statusCode)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()
			client, err := NewClientWithVersionURL(server.Client(), server.URL, server.URL, "control-token", 4096)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.ResolveInstalledVersion(context.Background(), requestValue)
			var failure *Failure
			if err == nil || errors.As(err, &failure) {
				t.Fatalf("invalid error response accepted: failure=%#v err=%v", failure, err)
			}
		})
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func preErrorV3(t *testing.T, code contracts.PlatformErrorCode, traceID contracts.TraceID) contracts.PlatformErrorV3 {
	t.Helper()
	value, err := contracts.NewPlatformErrorV3(code, traceID)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func correlatedErrorV3(t *testing.T, code contracts.PlatformErrorCode, request contracts.ResolveInstalledVersionRequest) contracts.PlatformErrorV3 {
	t.Helper()
	return correlatedErrorV3WithTrace(t, code, request, request.TraceID)
}

func correlatedErrorV3WithTrace(t *testing.T, code contracts.PlatformErrorCode, request contracts.ResolveInstalledVersionRequest, traceID contracts.TraceID) contracts.PlatformErrorV3 {
	t.Helper()
	value, err := contracts.NewCorrelatedPlatformErrorV3(code, traceID, request.InvocationID, request.RootTaskID)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
