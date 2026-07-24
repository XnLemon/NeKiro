package invocation

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

func TestRouterClientUsesOnlyFrozenInternalV3Direction(t *testing.T) {
	var received contracts.DispatchInvocationRequestV4
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/internal/v4/invocations" || request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer service-secret" || request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("unexpected Router request: %s %s %#v", request.Method, request.URL.Path, request.Header)
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("x-nek-trace-id", "trace-a")
		_, _ = io.WriteString(writer, "data: {}\n\n")
	}))
	defer server.Close()
	client, err := NewRouterClient(server.Client(), server.URL+"/internal/v4/invocations", "service-secret")
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	request := contracts.DispatchInvocationRequestV4{InvocationID: "inv-a", RootTaskID: "task-a", TraceID: "trace-a", Caller: contracts.Caller{Type: "user", ID: "owner-a"}, WorkspaceID: "workspace-a", TargetAgentID: "agent-a", AgentCardVersion: "1.0.0", AgentReleaseID: "release-a", AgentCardDigest: digest, Capability: "capability-a", Input: []byte(`{}`), Stream: true}
	response, err := client.Dispatch(context.Background(), request, contracts.InvocationResultModeSSE)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if received.InvocationID != request.InvocationID || received.AgentReleaseID != request.AgentReleaseID || received.AgentCardDigest != digest || response.StatusCode != 200 || response.ContentType != "text/event-stream" || response.Headers.Get("x-nek-trace-id") != "trace-a" {
		t.Fatalf("received=%#v response=%#v", received, response)
	}
}

func TestRouterClientRequiresOneMatchingTraceAndClosesRejectedBodies(t *testing.T) {
	tests := []struct {
		name     string
		response func(*trackedReadCloser) *http.Response
		closed   bool
	}{
		{name: "nil response", response: func(*trackedReadCloser) *http.Response { return nil }},
		{name: "bodyless response", response: func(*trackedReadCloser) *http.Response {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}}
		}},
		{name: "missing Trace", closed: true, response: traceTestResponse(nil)},
		{name: "duplicate Trace", closed: true, response: traceTestResponse([]string{"trace-a", "trace-a"})},
		{name: "malformed Trace", closed: true, response: traceTestResponse([]string{"bad trace"})},
		{name: "mismatched Trace", closed: true, response: traceTestResponse([]string{"trace-b"})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &trackedReadCloser{Reader: strings.NewReader(`{}`)}
			client, err := NewRouterClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
				return test.response(body), nil
			}), "https://router.example/internal/v4/invocations", "service-secret")
			if err != nil {
				t.Fatal(err)
			}
			request := contracts.DispatchInvocationRequestV4{TraceID: "trace-a"}
			if _, err := client.Dispatch(context.Background(), request, contracts.InvocationResultModeJSON); err == nil {
				t.Fatal("invalid Router response was accepted")
			}
			if body.closed != test.closed {
				t.Fatalf("body closed=%v, want %v", body.closed, test.closed)
			}
		})
	}
}

func traceTestResponse(values []string) func(*trackedReadCloser) *http.Response {
	return func(body *trackedReadCloser) *http.Response {
		header := http.Header{"Content-Type": []string{"application/json"}}
		for _, value := range values {
			header.Add(routerTraceHeader, value)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: body}
	}
}

type trackedReadCloser struct {
	io.Reader
	closed bool
}

func (body *trackedReadCloser) Close() error {
	body.closed = true
	return nil
}

func TestRouterClientRejectsWrongResultMediaWithoutFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{}`)
	}))
	defer server.Close()
	client, _ := NewRouterClient(server.Client(), server.URL, "service-secret")
	if _, err := client.Dispatch(context.Background(), contracts.DispatchInvocationRequestV4{}, contracts.InvocationResultModeSSE); err == nil {
		t.Fatal("wrong Router result media was accepted")
	}
}

func TestRouterClientReadsExactV3MetadataPathsOnSameOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.Header.Get("Authorization") != "Bearer service-secret" || request.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected metadata request: %s %s %#v", request.Method, request.URL.Path, request.Header)
		}
		if request.URL.Path == "/internal/v3/workspaces/workspace-a/invocations/inv-a" {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"invocation":{"invocationId":"inv-a"},"events":[]}`)
			return
		}
		if request.URL.Path == "/internal/v3/workspaces/workspace-a/traces/trace-a" {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"traceId":"trace-a","invocations":[]}`)
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	client, err := NewRouterClient(server.Client(), server.URL+"/internal/v4/invocations", "service-secret")
	if err != nil {
		t.Fatal(err)
	}
	invocationResponse, err := client.GetInvocation(context.Background(), "workspace-a", "inv-a")
	if err != nil {
		t.Fatalf("get Invocation: %v", err)
	}
	defer func() { _ = invocationResponse.Body.Close() }()
	if invocationResponse.StatusCode != http.StatusOK || invocationResponse.ContentType != "application/json" {
		t.Fatalf("Invocation response = %#v", invocationResponse)
	}
	traceResponse, err := client.GetTrace(context.Background(), "workspace-a", "trace-a")
	if err != nil {
		t.Fatalf("get Trace: %v", err)
	}
	defer func() { _ = traceResponse.Body.Close() }()
	if traceResponse.StatusCode != http.StatusOK || traceResponse.ContentType != "application/json" {
		t.Fatalf("Trace response = %#v", traceResponse)
	}
}

func TestRouterClientRejectsInvalidMetadataIdentifiersWithoutRequest(t *testing.T) {
	client, err := NewRouterClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("request must not be made")
	}), "https://router.example/internal/v4/invocations", "service-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetInvocation(context.Background(), "bad workspace", "inv-a"); err == nil {
		t.Fatal("invalid Workspace identifier was accepted")
	}
	if _, err := client.GetTrace(context.Background(), "workspace-a", "bad trace"); err == nil {
		t.Fatal("invalid Trace identifier was accepted")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}
