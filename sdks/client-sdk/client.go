package clientsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Nene7ko/NeKiro/contracts"
)

// InvokeRequest contains the only business-controlled invocation fields.
// Workspace, routing, version, Release, correlation, and credentials are not
// accepted per call.
type InvokeRequest struct {
	AgentID    string
	Capability string
	Input      json.RawMessage
}

// Result is a validated non-streaming Gateway result.
type Result struct {
	InvocationID string
	RootTaskID   string
	TraceID      contracts.TraceID
	Output       json.RawMessage
}

// Invoke performs exactly one non-streaming Gateway invocation.
func (client *Client) Invoke(ctx context.Context, request InvokeRequest) (*Result, error) {
	response, err := client.do(ctx, request, false, "application/json")
	if err != nil {
		return nil, err
	}
	if response == nil || response.Body == nil {
		return nil, errors.New("clientsdk: Gateway response is empty")
	}
	if response.StatusCode != http.StatusOK {
		return nil, client.decodePlatformError(response)
	}
	if err := requireMediaType(response.Header, "application/json"); err != nil {
		return nil, closeWithError(response.Body, err)
	}
	traceID, err := requireTraceHeader(response.Header)
	if err != nil {
		return nil, closeWithError(response.Body, err)
	}
	body, err := readAndCloseBounded(response.Body, client.responseLimitBytes)
	if err != nil {
		return nil, fmt.Errorf("clientsdk: read Gateway result: %w", err)
	}
	if err := rejectDuplicateJSONMembers(body); err != nil {
		return nil, errors.New("clientsdk: Gateway result response is invalid")
	}
	var wire contracts.InvocationResult
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return nil, errors.New("clientsdk: Gateway result response is invalid")
	}
	if err := requireEOF(decoder); err != nil {
		return nil, errors.New("clientsdk: Gateway result response is invalid")
	}
	if err := client.results.ValidateInvocationResult(wire); err != nil {
		return nil, errors.New("clientsdk: Gateway result response is invalid")
	}
	if wire.TraceID != traceID {
		return nil, errors.New("clientsdk: Gateway result Trace does not match response header")
	}
	return &Result{
		InvocationID: wire.InvocationID,
		RootTaskID:   wire.RootTaskID,
		TraceID:      wire.TraceID,
		Output:       append(json.RawMessage(nil), wire.Result...),
	}, nil
}

func (client *Client) do(ctx context.Context, request InvokeRequest, stream bool, accept string) (*http.Response, error) {
	if ctx == nil {
		return nil, errors.New("clientsdk: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("clientsdk: invocation context is done: %w", err)
	}
	if !safeIdentifier(request.AgentID) {
		return nil, errors.New("clientsdk: Agent ID is invalid")
	}
	if !safeIdentifier(request.Capability) {
		return nil, errors.New("clientsdk: capability is invalid")
	}
	payload, err := encodeInvocationRequest(request, stream, client.requestLimitBytes)
	if err != nil {
		return nil, err
	}
	target := client.gatewayOrigin + "/v4/workspaces/" + client.workspaceID + "/invocations"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("clientsdk: construct Gateway request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+client.applicationCredential)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", accept)
	response, err := client.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("clientsdk: Gateway invocation failed: %w", err)
	}
	return response, nil
}
