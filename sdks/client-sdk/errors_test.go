package clientsdk

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Nene7ko/NeKiro/contracts"
)

func TestPlatformErrorAcceptsEveryFrozenStatusCodePhaseRow(t *testing.T) {
	type matrixEntry struct {
		status int
		code   contracts.PlatformErrorCode
		phases []bool
	}
	entries := []matrixEntry{
		{400, contracts.ErrorCodeValidationError, []bool{false}},
		{401, contracts.ErrorCodeUnauthenticated, []bool{false}},
		{403, contracts.ErrorCodeForbidden, []bool{false}},
		{403, contracts.ErrorCodeCapabilityNotAllowed, []bool{false}},
		{404, contracts.ErrorCodeNotFound, []bool{false}},
		{404, contracts.ErrorCodeAgentNotInstalled, []bool{false}},
		{406, contracts.ErrorCodeNotAcceptable, []bool{false}},
		{409, contracts.ErrorCodeConflict, []bool{false, true}},
		{409, contracts.ErrorCodeInstallationDisabled, []bool{false, true}},
		{409, contracts.ErrorCodeAgentDisabled, []bool{false, true}},
		{409, contracts.ErrorCodeAgentReleaseUnpublished, []bool{false, true}},
		{409, contracts.ErrorCodeAgentReleaseSuspended, []bool{false, true}},
		{409, contracts.ErrorCodeAgentReleaseRevoked, []bool{false, true}},
		{409, contracts.ErrorCodeCanceled, []bool{false, true}},
		{413, contracts.ErrorCodePayloadTooLarge, []bool{false}},
		{500, contracts.ErrorCodeInternal, []bool{false, true}},
		{502, contracts.ErrorCodeAgentAuthUnsupported, []bool{true}},
		{502, contracts.ErrorCodeAgentResponseTooLarge, []bool{true}},
		{502, contracts.ErrorCodeAgentExecutionFailed, []bool{true}},
		{502, contracts.ErrorCodeA2AProtocol, []bool{true}},
		{503, contracts.ErrorCodeRouteNotFound, []bool{false, true}},
		{503, contracts.ErrorCodeAgentUnavailable, []bool{false, true}},
		{503, contracts.ErrorCodeDependency, []bool{false, true}},
		{504, contracts.ErrorCodeTimeout, []bool{false, true}},
	}
	for _, entry := range entries {
		for _, correlated := range entry.phases {
			name := strconv.Itoa(entry.status) + "/" + string(entry.code) + "/pre"
			if correlated {
				name = strconv.Itoa(entry.status) + "/" + string(entry.code) + "/correlated"
			}
			t.Run(name, func(t *testing.T) {
				bodyBytes := platformErrorJSON(t, entry.code, correlated, "trace-client")
				var wireMessage struct {
					Message string `json:"message"`
				}
				if err := json.Unmarshal(bodyBytes, &wireMessage); err != nil {
					t.Fatal(err)
				}
				body := &trackedBody{Reader: strings.NewReader(string(bodyBytes))}
				client := platformErrorClient(t, entry.status, body, []string{"application/json"}, []string{"trace-client"}, 4096, "application-secret")
				_, err := client.Invoke(t.Context(), validStreamRequest())
				var platformError *PlatformError
				if !errors.As(err, &platformError) {
					t.Fatal("expected a typed PlatformError")
				}
				if platformError.StatusCode != entry.status || platformError.Code != entry.code || platformError.TraceID != "trace-client" || platformError.Correlated() != correlated || !body.closed {
					t.Fatalf("PlatformError=%#v closed=%v", platformError, body.closed)
				}
				if correlated && (platformError.InvocationID != "inv-client" || platformError.RootTaskID != "task-client") {
					t.Fatalf("correlated PlatformError=%#v", platformError)
				}
				if !correlated && (platformError.InvocationID != "" || platformError.RootTaskID != "") {
					t.Fatalf("pre-correlation PlatformError=%#v", platformError)
				}
				if strings.Contains(err.Error(), wireMessage.Message) || strings.Contains(err.Error(), "application-secret") {
					t.Fatal("PlatformError text exposed wire or credential content")
				}
			})
		}
	}
}

func TestPlatformErrorAdapterIsSharedByStreamingInvocation(t *testing.T) {
	body := &trackedBody{Reader: strings.NewReader(string(platformErrorJSON(t, contracts.ErrorCodeDependency, true, "trace-client")))}
	client := platformErrorClient(t, http.StatusServiceUnavailable, body, []string{"application/json"}, []string{"trace-client"}, 4096, "application-secret")
	stream, err := client.InvokeStream(t.Context(), validStreamRequest())
	var platformError *PlatformError
	if stream != nil || !errors.As(err, &platformError) || !platformError.Correlated() || platformError.Code != contracts.ErrorCodeDependency || !body.closed {
		t.Fatalf("stream PlatformError contract failed: stream_nil=%t PlatformError=%#v closed=%v", stream == nil, platformError, body.closed)
	}
}

func TestPlatformErrorRejectsMalformedOrImpossibleResponsesWithoutRawContent(t *testing.T) {
	secret := "credential-sentinel-never-expose"
	validPre := string(platformErrorJSON(t, contracts.ErrorCodeValidationError, false, "trace-client"))
	validCorrelated := string(platformErrorJSON(t, contracts.ErrorCodeAgentExecutionFailed, true, "trace-client"))
	tests := []struct {
		name        string
		status      int
		body        string
		content     []string
		traces      []string
		limitOffset int64
	}{
		{name: "unsupported status", status: 299, body: validPre, content: []string{"application/json"}, traces: []string{"trace-client"}},
		{name: "wrong status code pairing", status: 401, body: validPre, content: []string{"application/json"}, traces: []string{"trace-client"}},
		{name: "correlated pre-only code", status: 400, body: string(platformErrorJSON(t, contracts.ErrorCodeValidationError, true, "trace-client")), content: []string{"application/json"}, traces: []string{"trace-client"}},
		{name: "uncorrelated post-only code", status: 502, body: string(platformErrorJSON(t, contracts.ErrorCodeAgentExecutionFailed, false, "trace-client")), content: []string{"application/json"}, traces: []string{"trace-client"}},
		{name: "missing media", status: 400, body: validPre, traces: []string{"trace-client"}},
		{name: "wrong media", status: 400, body: validPre, content: []string{"text/plain"}, traces: []string{"trace-client"}},
		{name: "duplicate media", status: 400, body: validPre, content: []string{"application/json", "application/json"}, traces: []string{"trace-client"}},
		{name: "missing Trace", status: 400, body: validPre, content: []string{"application/json"}},
		{name: "duplicate Trace", status: 400, body: validPre, content: []string{"application/json"}, traces: []string{"trace-client", "trace-client"}},
		{name: "malformed Trace", status: 400, body: validPre, content: []string{"application/json"}, traces: []string{"bad trace"}},
		{name: "mismatched Trace", status: 400, body: validPre, content: []string{"application/json"}, traces: []string{"trace-other"}},
		{name: "fixed message changed", status: 400, body: strings.Replace(validPre, "The request is invalid.", secret, 1), content: []string{"application/json"}, traces: []string{"trace-client"}},
		{name: "duplicate member", status: 400, body: strings.Replace(validPre, `"code":"VALIDATION_ERROR"`, `"code":"VALIDATION_ERROR","code":"VALIDATION_ERROR"`, 1), content: []string{"application/json"}, traces: []string{"trace-client"}},
		{name: "unknown secret member", status: 400, body: strings.TrimSuffix(validPre, "}") + `,"` + secret + `":"` + secret + `"}`, content: []string{"application/json"}, traces: []string{"trace-client"}},
		{name: "trailing value", status: 400, body: validPre + `{}`, content: []string{"application/json"}, traces: []string{"trace-client"}},
		{name: "non-object", status: 400, body: `[]`, content: []string{"application/json"}, traces: []string{"trace-client"}},
		{name: "incomplete correlation", status: 502, body: strings.Replace(validCorrelated, `,"rootTaskId":"task-client"`, "", 1), content: []string{"application/json"}, traces: []string{"trace-client"}},
		{name: "invalid body Trace", status: 400, body: strings.Replace(validPre, `"traceId":"trace-client"`, `"traceId":"bad trace"`, 1), content: []string{"application/json"}, traces: []string{"trace-client"}},
		{name: "oversized", status: 400, body: validPre, content: []string{"application/json"}, traces: []string{"trace-client"}, limitOffset: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(test.body)}
			limit := int64(4096)
			if test.limitOffset != 0 {
				limit = int64(len(test.body)) + test.limitOffset
			}
			client := platformErrorClient(t, test.status, body, test.content, test.traces, limit, secret)
			_, err := client.Invoke(t.Context(), validStreamRequest())
			var platformError *PlatformError
			if err == nil || errors.As(err, &platformError) || !body.closed {
				t.Fatalf("malformed response handling failed: error_nil=%t typed=%t closed=%v", err == nil, platformError != nil, body.closed)
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), test.body) {
				t.Fatal("local validation error exposed raw response content")
			}
		})
	}
}

func platformErrorJSON(t *testing.T, code contracts.PlatformErrorCode, correlated bool, traceID contracts.TraceID) []byte {
	t.Helper()
	var value any
	if correlated {
		platformError, err := contracts.NewCorrelatedPlatformErrorV4(code, traceID, "inv-client", "task-client")
		if err != nil {
			t.Fatal(err)
		}
		value = platformError
	} else {
		platformError, err := contracts.NewPreCorrelationPlatformErrorV4(code, traceID)
		if err != nil {
			t.Fatal(err)
		}
		value = platformError
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func platformErrorClient(t *testing.T, status int, body io.ReadCloser, content, traces []string, responseLimit int64, credential string) *Client {
	t.Helper()
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		header := http.Header{}
		for _, value := range content {
			header.Add("Content-Type", value)
		}
		for _, value := range traces {
			header.Add(traceHeader, value)
		}
		return &http.Response{StatusCode: status, Header: header, Body: body}, nil
	})
	config := validTestConfig(&http.Client{Transport: transport}, "https://gateway.example")
	config.ApplicationCredential = credential
	config.ResponseLimitBytes = responseLimit
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	return client
}
