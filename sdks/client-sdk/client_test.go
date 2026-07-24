package clientsdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Nene7ko/NeKiro/contracts"
)

const validResultJSON = `{"schemaVersion":"1","invocationId":"inv-client","rootTaskId":"task-client","traceId":"trace-client","status":"succeeded","result":{"answer":42}}`

func TestInvokeSendsExactV4RequestAndReturnsValidatedResult(t *testing.T) {
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v4/workspaces/workspace-a/invocations" || request.URL.RawQuery != "" {
			t.Errorf("unexpected request target: %s %s", request.Method, request.URL.String())
		}
		if values := request.Header.Values("Authorization"); len(values) != 1 || values[0] != "Bearer application-secret" {
			t.Error("Authorization header was not exactly one expected Bearer value")
		}
		if request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Accept") != "application/json" {
			t.Error("request media headers did not match the invocation contract")
		}
		body, _ := io.ReadAll(request.Body)
		requests <- body
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set(traceHeader, "trace-client")
		_, _ = io.WriteString(writer, validResultJSON)
	}))
	defer server.Close()
	client, err := NewClient(validTestConfig(server.Client(), server.URL))
	if err != nil {
		t.Fatal(err)
	}
	input := json.RawMessage(`{ "large": 9007199254740993, "decimal": 1.2300 }`)
	result, err := client.Invoke(t.Context(), InvokeRequest{AgentID: "agent-a", Capability: "answer", Input: input})
	if err != nil {
		t.Fatal(err)
	}
	body := <-requests
	if strings.Contains(string(body), "application-secret") {
		t.Fatal("request body exposed the application credential")
	}
	if !strings.Contains(string(body), `9007199254740993`) || !strings.Contains(string(body), `1.2300`) {
		t.Fatalf("request body lost secrecy or number spelling: %s", body)
	}
	var requestMembers map[string]json.RawMessage
	if err := json.Unmarshal(body, &requestMembers); err != nil || len(requestMembers) != 4 || string(requestMembers["agentId"]) != `"agent-a"` || string(requestMembers["capability"]) != `"answer"` || string(requestMembers["stream"]) != "false" {
		t.Fatalf("wire request=%s decode=%v members=%v", body, err, requestMembers)
	}
	if result.InvocationID != "inv-client" || result.RootTaskID != "task-client" || result.TraceID != "trace-client" || string(result.Output) != `{"answer":42}` {
		t.Fatalf("result=%#v", result)
	}
}

func TestInvokeRejectsInvalidBusinessInputBeforeTransport(t *testing.T) {
	var calls atomic.Int64
	client := mustTransportClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return validHTTPResultResponse(&trackedBody{Reader: strings.NewReader(validResultJSON)}), nil
	}), 4096)
	tests := []InvokeRequest{
		{AgentID: "", Capability: "answer", Input: json.RawMessage(`{}`)},
		{AgentID: "-agent", Capability: "answer", Input: json.RawMessage(`{}`)},
		{AgentID: "agent-a", Capability: "", Input: json.RawMessage(`{}`)},
		{AgentID: "agent-a", Capability: "bad capability", Input: json.RawMessage(`{}`)},
		{AgentID: "agent-a", Capability: "answer", Input: nil},
		{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`null`)},
		{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`[]`)},
		{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`1`)},
		{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`{"a":1,"a":2}`)},
		{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`{} {}`)},
	}
	for index, request := range tests {
		if _, err := client.Invoke(t.Context(), request); err == nil {
			t.Fatalf("invalid request %d was accepted: %#v", index, request)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("transport calls=%d", calls.Load())
	}
}

func TestInvokeEnforcesCompleteEncodedRequestLimit(t *testing.T) {
	request := InvokeRequest{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`{"n":9007199254740993}`)}
	payload, err := encodeInvocationRequest(request, false, contracts.RuntimeByteLimitMaximum)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return validHTTPResultResponse(&trackedBody{Reader: strings.NewReader(validResultJSON)}), nil
	})
	client := mustTransportClient(t, transport, int64(len(payload)))
	if _, err := client.Invoke(t.Context(), request); err != nil {
		t.Fatalf("request at limit rejected: %v", err)
	}
	client = mustTransportClient(t, transport, int64(len(payload)-1))
	if _, err := client.Invoke(t.Context(), request); err == nil {
		t.Fatal("request one byte over limit was accepted")
	}
	if calls.Load() != 1 {
		t.Fatalf("transport calls=%d, want 1", calls.Load())
	}
	largeRequest := request
	largeRequest.Input = json.RawMessage(`{"payload":"` + strings.Repeat("x", 1<<20) + `"}`)
	client = mustTransportClient(t, transport, 4096)
	if _, err := client.Invoke(t.Context(), largeRequest); err == nil || !strings.Contains(err.Error(), "exceeds the configured limit") {
		t.Fatal("large input was not rejected by the encoded-request limit before transport")
	}
	if calls.Load() != 1 {
		t.Fatalf("large over-limit input reached transport: calls=%d", calls.Load())
	}
}

func TestInvokeStrictlyValidatesSuccessResponseAndClosesBody(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		content    []string
		traces     []string
		body       string
		limitDelta int64
	}{
		{name: "wrong status", status: http.StatusCreated, content: []string{"application/json"}, traces: []string{"trace-client"}, body: validResultJSON},
		{name: "missing media", status: 200, traces: []string{"trace-client"}, body: validResultJSON},
		{name: "parameterized media", status: 200, content: []string{"application/json; charset=utf-8"}, traces: []string{"trace-client"}, body: validResultJSON},
		{name: "duplicate media", status: 200, content: []string{"application/json", "application/json"}, traces: []string{"trace-client"}, body: validResultJSON},
		{name: "missing Trace", status: 200, content: []string{"application/json"}, body: validResultJSON},
		{name: "duplicate Trace", status: 200, content: []string{"application/json"}, traces: []string{"trace-client", "trace-client"}, body: validResultJSON},
		{name: "malformed Trace", status: 200, content: []string{"application/json"}, traces: []string{"bad trace"}, body: validResultJSON},
		{name: "mismatched Trace", status: 200, content: []string{"application/json"}, traces: []string{"trace-other"}, body: validResultJSON},
		{name: "unknown member", status: 200, content: []string{"application/json"}, traces: []string{"trace-client"}, body: strings.Replace(validResultJSON, `,"status"`, `,"unknown":true,"status"`, 1)},
		{name: "duplicate member", status: 200, content: []string{"application/json"}, traces: []string{"trace-client"}, body: strings.Replace(validResultJSON, `"schemaVersion":"1"`, `"schemaVersion":"1","schemaVersion":"1"`, 1)},
		{name: "trailing value", status: 200, content: []string{"application/json"}, traces: []string{"trace-client"}, body: validResultJSON + `{}`},
		{name: "invalid status", status: 200, content: []string{"application/json"}, traces: []string{"trace-client"}, body: strings.Replace(validResultJSON, `"succeeded"`, `"failed"`, 1)},
		{name: "oversized", status: 200, content: []string{"application/json"}, traces: []string{"trace-client"}, body: validResultJSON, limitDelta: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(test.body)}
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				header := http.Header{}
				for _, value := range test.content {
					header.Add("Content-Type", value)
				}
				for _, value := range test.traces {
					header.Add(traceHeader, value)
				}
				return &http.Response{StatusCode: test.status, Header: header, Body: body}, nil
			})
			limit := int64(4096)
			if test.limitDelta != 0 {
				limit = int64(len(test.body)) + test.limitDelta
			}
			client := mustTransportClientWithResponseLimit(t, transport, limit)
			if _, err := client.Invoke(t.Context(), InvokeRequest{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`{}`)}); err == nil {
				t.Fatal("invalid Gateway response was accepted")
			}
			if !body.closed {
				t.Fatal("Gateway response body was not closed")
			}
		})
	}

	body := &trackedBody{Reader: strings.NewReader(validResultJSON)}
	client := mustTransportClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return validHTTPResultResponse(body), nil
	}), 4096)
	if _, err := client.Invoke(t.Context(), InvokeRequest{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`{}`)}); err != nil || !body.closed {
		t.Fatalf("valid response error=%v closed=%v", err, body.closed)
	}
}

func TestInvokePropagatesCancellationAndTransportFailureWithoutRetryOrSecret(t *testing.T) {
	var calls atomic.Int64
	transportErr := errors.New("network offline")
	client := mustTransportClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, transportErr
	}), 4096)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := client.Invoke(ctx, InvokeRequest{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`{}`)})
	if !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("pre-canceled error=%v calls=%d", err, calls.Load())
	}
	_, err = client.Invoke(t.Context(), InvokeRequest{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`{}`)})
	if !errors.Is(err, transportErr) || calls.Load() != 1 {
		t.Fatalf("transport error=%v calls=%d", err, calls.Load())
	}
	if strings.Contains(err.Error(), "application-secret") {
		t.Fatal("transport error exposed the application credential")
	}
}

func TestClientAndConfigLogRepresentationsRedactCredential(t *testing.T) {
	credential := "credential-sentinel-never-log"
	config := validTestConfig(&http.Client{}, "https://gateway.example")
	config.ApplicationCredential = credential
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{config, &config, *client, client} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			formatted := fmt.Sprintf(format, value)
			if strings.Contains(formatted, credential) {
				t.Fatalf("format %s exposed the application credential", format)
			}
			if !strings.Contains(formatted, "[REDACTED]") {
				t.Fatalf("format %s produced unsafe representation %q", format, formatted)
			}
		}
	}
}

func TestGatewayResultValidationDoesNotExposeRawResponseContent(t *testing.T) {
	secret := "credential-sentinel-from-response"
	body := strings.Replace(validResultJSON, `,"status"`, `,"`+secret+`":"`+secret+`","status"`, 1)
	client := mustTransportClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return validHTTPResultResponse(&trackedBody{Reader: strings.NewReader(body)}), nil
	}), 4096)
	_, err := client.Invoke(t.Context(), validStreamRequest())
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), body) {
		t.Fatal("Gateway result validation exposed raw response content")
	}
}

func TestClientSupportsConcurrentIndependentInvocations(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set(traceHeader, "trace-client")
		_, _ = io.WriteString(writer, validResultJSON)
	}))
	defer server.Close()
	client, err := NewClient(validTestConfig(server.Client(), server.URL))
	if err != nil {
		t.Fatal(err)
	}
	const total = 24
	var wait sync.WaitGroup
	errorsSeen := make(chan error, total)
	for range total {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := client.Invoke(t.Context(), InvokeRequest{AgentID: "agent-a", Capability: "answer", Input: json.RawMessage(`{}`)})
			errorsSeen <- err
		}()
	}
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != total {
		t.Fatalf("calls=%d, want %d", calls.Load(), total)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (body *trackedBody) Close() error {
	body.closed = true
	return nil
}

func validHTTPResultResponse(body io.ReadCloser) *http.Response {
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	header.Set(traceHeader, "trace-client")
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: body}
}

func mustTransportClient(t *testing.T, transport http.RoundTripper, requestLimit int64) *Client {
	t.Helper()
	config := validTestConfig(&http.Client{Transport: transport}, "https://gateway.example")
	config.RequestLimitBytes = requestLimit
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func mustTransportClientWithResponseLimit(t *testing.T, transport http.RoundTripper, responseLimit int64) *Client {
	t.Helper()
	config := validTestConfig(&http.Client{Transport: transport}, "https://gateway.example")
	config.ResponseLimitBytes = responseLimit
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

var _ = contracts.RuntimeByteLimitMinimum
