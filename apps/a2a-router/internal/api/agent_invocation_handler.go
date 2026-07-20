package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/nested"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/resolution"
	"github.com/Nene7ko/NeKiro/contracts"
)

// VersionResolver resolves the deterministic installed Agent Card version
// from the Control Plane Internal v3 endpoint.
type VersionResolver interface {
	ResolveInstalledVersion(context.Context, contracts.ResolveInstalledVersionRequest) (contracts.ResolveInstalledVersionResponse, error)
}

// NestedLedgerReader reads the committed parent Invocation from the Router Ledger.
type NestedLedgerReader interface {
	GetInvocation(context.Context, string, string) (contracts.InvocationDetailResponseV4, error)
}

// AgentInvocationHandler handles Agent-facing nested invocation requests at
// the /agent/v1/invocations boundary. It authenticates the Agent binding,
// validates the strict request shape, reads and validates the parent, derives
// child context, resolves the installed version, and delegates to the existing
// DispatchHandler for resolution, transport, and Ledger.
type AgentInvocationHandler struct {
	binding         *nested.AgentBinding
	ledgerReader    NestedLedgerReader
	versionResolver VersionResolver
	dispatchHandler *DispatchHandler
	requestLimit    int64
}

// NewAgentInvocationHandler creates the Agent-facing nested invocation handler.
func NewAgentInvocationHandler(
	binding *nested.AgentBinding,
	ledgerReader NestedLedgerReader,
	versionResolver VersionResolver,
	dispatchHandler *DispatchHandler,
	requestLimit int64,
) (*AgentInvocationHandler, error) {
	if binding == nil {
		return nil, errors.New("agent binding is required")
	}
	if ledgerReader == nil {
		return nil, errors.New("nested ledger reader is required")
	}
	if versionResolver == nil {
		return nil, errors.New("version resolver is required")
	}
	if dispatchHandler == nil {
		return nil, errors.New("dispatch handler is required")
	}
	if requestLimit < contracts.RuntimeByteLimitMinimum || requestLimit > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("agent request limit is invalid")
	}
	return &AgentInvocationHandler{
		binding:         binding,
		ledgerReader:    ledgerReader,
		versionResolver: versionResolver,
		dispatchHandler: dispatchHandler,
		requestLimit:    requestLimit,
	}, nil
}

// RegisterRoutes exposes the Agent Router v1 nested invocation boundary.
func (handler *AgentInvocationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /agent/v1/invocations", handler.serve)
}

func (handler *AgentInvocationHandler) serve(writer http.ResponseWriter, request *http.Request) {
	// Step 1: Authenticate the Agent binding. Auth failures are pre-correlation.
	authenticatedAgentID, err := handler.binding.Authenticate(request)
	if err != nil {
		code := contracts.ErrorCodeUnauthenticated
		if errors.Is(err, nested.ErrForbidden) {
			code = contracts.ErrorCodeForbidden
		}
		handler.writePreError(writer, code)
		return
	}

	// Step 2: Read and strictly validate the nested request.
	if request.Header.Get("Content-Type") != "application/json" {
		handler.writePreError(writer, contracts.ErrorCodeValidationError)
		return
	}
	nestedRequest, err := handler.readNestedRequest(request)
	if err != nil {
		code := contracts.ErrorCodeValidationError
		if errors.Is(err, errPayloadTooLarge) {
			code = contracts.ErrorCodePayloadTooLarge
		}
		handler.writePreError(writer, code)
		return
	}

	// Step 3: Negotiate result mode before acceptance.
	accept := request.Header.Get("Accept")
	if _, err := contracts.NegotiateInvocationResultMode(nestedRequest.Stream, accept); err != nil {
		handler.writePreError(writer, contracts.ErrorCodeNotAcceptable)
		return
	}

	// Step 4: Read the committed parent from the Ledger.
	parent, err := handler.ledgerReader.GetInvocation(request.Context(), "", nestedRequest.ParentInvocationID)
	if err != nil {
		handler.writePreError(writer, contracts.ErrorCodeNotFound)
		return
	}

	// Step 5: Derive child context from the parent.
	childContext, err := nested.DeriveChildContext(parent, authenticatedAgentID)
	if err != nil {
		code := contracts.ErrorCodeConflict
		if errors.Is(err, nested.ErrParentNotFound) {
			code = contracts.ErrorCodeNotFound
		} else if errors.Is(err, nested.ErrParentTargetMismatch) {
			code = contracts.ErrorCodeForbidden
		}
		handler.writePreError(writer, code)
		return
	}

	// Step 6: Resolve the installed version from the Control Plane.
	versionResponse, err := handler.versionResolver.ResolveInstalledVersion(request.Context(), contracts.ResolveInstalledVersionRequest{
		InvocationID: childContext.ChildInvocationID,
		RootTaskID:   childContext.RootTaskID,
		TraceID:      childContext.TraceID,
		WorkspaceID:  childContext.WorkspaceID,
		AgentID:      nestedRequest.TargetAgentID,
		Capability:   nestedRequest.Capability,
	})
	if err != nil {
		var failure *resolution.Failure
		if errors.As(err, &failure) {
			writeRawJSON(writer, failure.StatusCode, failure.TraceID, failure.Body)
			return
		}
		handler.writePreError(writer, contracts.ErrorCodeDependency)
		return
	}

	// Step 7: Build the trusted child dispatch request and delegate.
	childDispatchRequest := nested.BuildChildDispatchRequest(
		childContext,
		nestedRequest.TargetAgentID,
		nestedRequest.Capability,
		nestedRequest.Input,
		nestedRequest.Stream,
		versionResponse.Version,
	)
	handler.dispatchHandler.DispatchChild(writer, request, childDispatchRequest, accept)
}

func (handler *AgentInvocationHandler) readNestedRequest(request *http.Request) (contracts.NestedInvocationRequestV1, error) {
	if request.ContentLength > handler.requestLimit {
		return contracts.NestedInvocationRequestV1{}, errPayloadTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(request.Body, handler.requestLimit+1))
	if closeErr := request.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return contracts.NestedInvocationRequestV1{}, err
	}
	if int64(len(data)) > handler.requestLimit {
		return contracts.NestedInvocationRequestV1{}, errPayloadTooLarge
	}
	if err := rejectDuplicateMembers(data); err != nil {
		return contracts.NestedInvocationRequestV1{}, err
	}
	// Reject trusted fields that the nested request must not contain.
	if err := rejectTrustedFields(data); err != nil {
		return contracts.NestedInvocationRequestV1{}, err
	}
	var value contracts.NestedInvocationRequestV1
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return contracts.NestedInvocationRequestV1{}, err
	}
	if err := requireEOF(decoder); err != nil {
		return contracts.NestedInvocationRequestV1{}, err
	}
	// Validate identifiers.
	if !validIdentifier(value.ParentInvocationID) {
		return contracts.NestedInvocationRequestV1{}, errors.New("parentInvocationId is invalid")
	}
	if !validIdentifier(value.TargetAgentID) {
		return contracts.NestedInvocationRequestV1{}, errors.New("targetAgentId is invalid")
	}
	if !validIdentifier(value.Capability) {
		return contracts.NestedInvocationRequestV1{}, errors.New("capability is invalid")
	}
	var input map[string]json.RawMessage
	if json.Unmarshal(value.Input, &input) != nil || input == nil {
		return contracts.NestedInvocationRequestV1{}, errors.New("input must be a JSON object")
	}
	return value, nil
}

// rejectTrustedFields ensures the nested request does not contain caller,
// Workspace, root Task, Trace, endpoint, credential, or child identity fields.
func rejectTrustedFields(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	forbidden := []string{
		"invocationId", "rootTaskId", "traceId", "workspaceId",
		"caller", "callerType", "callerId",
		"agentCardVersion", "version",
		"endpoint", "url", "credential", "token", "authorization",
		"childInvocationId", "childId",
	}
	for _, field := range forbidden {
		if _, exists := fields[field]; exists {
			return errors.New("nested request contains a forbidden trusted field")
		}
	}
	return nil
}

func (handler *AgentInvocationHandler) writePreError(writer http.ResponseWriter, code contracts.PlatformErrorCode) {
	traceID, err := newTraceID()
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	status := errorStatus(code)
	payload, err := contracts.NewPreCorrelationPlatformErrorV4(code, traceID)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeJSON(writer, status, traceID, payload)
}
