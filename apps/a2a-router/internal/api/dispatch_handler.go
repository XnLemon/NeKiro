package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	semver "github.com/Masterminds/semver/v3"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/auth"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/resolution"
	"github.com/Nene7ko/NeKiro/contracts"
)

const TraceHeader = "x-nek-trace-id"

type Authenticator interface {
	Authenticate(*http.Request) (auth.Caller, error)
}

type Resolver interface {
	Resolve(context.Context, contracts.ResolveAgentRequest) (contracts.ResolveAgentResponse, error)
}

type NonStreamingTransport interface {
	ValidateNonStreamingTarget(contracts.DispatchInvocationRequestV3, contracts.ResolveAgentResponse) error
	SendNonStreaming(context.Context, contracts.DispatchInvocationRequestV3, contracts.ResolveAgentResponse) (json.RawMessage, error)
	ValidateNonStreamingInput(contracts.DispatchInvocationRequestV3, contracts.ResolveAgentResponse) error
}

type InvocationLedgerAppender interface {
	Append(context.Context, contracts.InvocationEventV03) error
}

type platformErrorCoder interface {
	PlatformErrorCode() contracts.PlatformErrorCode
}

type DispatchHandler struct {
	authenticator Authenticator
	resolver      Resolver
	transport     NonStreamingTransport
	ledger        InvocationLedgerAppender
	requestLimit  int64
	deadline      time.Duration
}

// ledgerCommitGrace bounds the one terminal-fact persistence attempt after a
// caller deadline/cancellation without turning it into an unbounded write.
const ledgerCommitGrace = time.Second

func NewDispatchHandler(authenticator Authenticator, resolver Resolver, requestLimit int64, deadline time.Duration) (*DispatchHandler, error) {
	if authenticator == nil || resolver == nil || requestLimit < contracts.RuntimeByteLimitMinimum || requestLimit > contracts.RuntimeByteLimitMaximum || deadline < time.Duration(contracts.RuntimeDeadlineMinimumMS)*time.Millisecond || deadline > time.Duration(contracts.RuntimeDeadlineMaximumMS)*time.Millisecond {
		return nil, errors.New("router dispatch dependencies are required")
	}
	return &DispatchHandler{authenticator: authenticator, resolver: resolver, requestLimit: requestLimit, deadline: deadline}, nil
}

func NewDispatchHandlerWithTransport(authenticator Authenticator, resolver Resolver, transport NonStreamingTransport, requestLimit int64, deadline time.Duration) (*DispatchHandler, error) {
	handler, err := NewDispatchHandler(authenticator, resolver, requestLimit, deadline)
	if err != nil {
		return nil, err
	}
	if transport == nil {
		return nil, errors.New("router non-streaming transport is required")
	}
	handler.transport = transport
	return handler, nil
}

func NewDispatchHandlerWithTransportAndLedger(authenticator Authenticator, resolver Resolver, transport NonStreamingTransport, ledger InvocationLedgerAppender, requestLimit int64, deadline time.Duration) (*DispatchHandler, error) {
	handler, err := NewDispatchHandlerWithTransport(authenticator, resolver, transport, requestLimit, deadline)
	if err != nil {
		return nil, err
	}
	if ledger == nil {
		return nil, errors.New("router invocation ledger appender is required")
	}
	handler.ledger = ledger
	return handler, nil
}

func (handler *DispatchHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/v3/invocations", handler.dispatch)
}

func (handler *DispatchHandler) dispatch(writer http.ResponseWriter, request *http.Request) {
	if _, err := handler.authenticator.Authenticate(request); err != nil {
		handler.writeGeneratedPreError(writer, authErrorCode(err))
		return
	}
	if request.Header.Get("Content-Type") != "application/json" {
		handler.writeGeneratedPreError(writer, contracts.ErrorCodeValidationError)
		return
	}
	dispatchRequest, err := handler.readRequest(request)
	if err != nil {
		code := contracts.ErrorCodeValidationError
		if errors.Is(err, errPayloadTooLarge) {
			code = contracts.ErrorCodePayloadTooLarge
		}
		handler.writeGeneratedPreError(writer, code)
		return
	}
	if _, err := contracts.NegotiateInvocationResultMode(dispatchRequest.Stream, request.Header.Get("Accept")); err != nil {
		handler.writePreError(writer, dispatchRequest.TraceID, contracts.ErrorCodeNotAcceptable)
		return
	}
	if err := validateDispatch(dispatchRequest); err != nil {
		handler.writePreError(writer, dispatchRequest.TraceID, contracts.ErrorCodeValidationError)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.deadline)
	defer cancel()
	resolved, err := handler.resolver.Resolve(ctx, contracts.ResolveAgentRequest{
		InvocationID: dispatchRequest.InvocationID, RootTaskID: dispatchRequest.RootTaskID,
		TraceID: dispatchRequest.TraceID, WorkspaceID: dispatchRequest.WorkspaceID,
		AgentID: dispatchRequest.TargetAgentID, Version: dispatchRequest.AgentCardVersion,
		Capability: dispatchRequest.Capability,
	})
	if err != nil {
		code := contracts.ErrorCodeDependency
		var failure *resolution.Failure
		if errors.As(err, &failure) {
			writeRawJSON(writer, failure.StatusCode, failure.TraceID, failure.Body)
			return
		} else if errors.Is(err, context.DeadlineExceeded) {
			code = contracts.ErrorCodeTimeout
		}
		handler.writeCorrelatedError(writer, dispatchRequest, code)
		return
	}
	if handler.transport != nil && !dispatchRequest.Stream {
		targetErr := handler.transport.ValidateNonStreamingTarget(dispatchRequest, resolved)
		if targetErr != nil {
			if handler.ledger != nil {
				handler.dispatchNonStreamingWithLedger(ctx, writer, dispatchRequest, resolved, targetErr)
				return
			}
			handler.writeCorrelatedError(writer, dispatchRequest, dispatchErrorCode(targetErr))
			return
		}
		if err := handler.transport.ValidateNonStreamingInput(dispatchRequest, resolved); err != nil {
			handler.writePreError(writer, dispatchRequest.TraceID, dispatchErrorCode(err))
			return
		}
		if handler.ledger != nil {
			handler.dispatchNonStreamingWithLedger(ctx, writer, dispatchRequest, resolved, nil)
			return
		}
		result, err := handler.transport.SendNonStreaming(ctx, dispatchRequest, resolved)
		if err != nil {
			code := dispatchErrorCode(err)
			handler.writeCorrelatedError(writer, dispatchRequest, code)
			return
		}
		handler.writeInvocationResult(writer, dispatchRequest, result)
		return
	}
	handler.writeCorrelatedError(writer, dispatchRequest, contracts.ErrorCodeRouteNotFound)
}

var errPayloadTooLarge = errors.New("router dispatch payload is too large")

func (handler *DispatchHandler) readRequest(request *http.Request) (contracts.DispatchInvocationRequestV3, error) {
	if request.ContentLength > handler.requestLimit {
		return contracts.DispatchInvocationRequestV3{}, errPayloadTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(request.Body, handler.requestLimit+1))
	if closeErr := request.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return contracts.DispatchInvocationRequestV3{}, err
	}
	if int64(len(data)) > handler.requestLimit {
		return contracts.DispatchInvocationRequestV3{}, errPayloadTooLarge
	}
	if err := rejectDuplicateMembers(data); err != nil {
		return contracts.DispatchInvocationRequestV3{}, err
	}
	var value contracts.DispatchInvocationRequestV3
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return contracts.DispatchInvocationRequestV3{}, err
	}
	if err := requireEOF(decoder); err != nil {
		return contracts.DispatchInvocationRequestV3{}, err
	}
	return value, nil
}

func validateDispatch(value contracts.DispatchInvocationRequestV3) error {
	for _, identifier := range []string{value.InvocationID, value.RootTaskID, value.WorkspaceID, value.TargetAgentID, value.Capability, value.Caller.ID} {
		if !validIdentifier(identifier) {
			return errors.New("dispatch identifier is invalid")
		}
	}
	if _, err := semver.StrictNewVersion(value.AgentCardVersion); err != nil {
		return errors.New("dispatch Agent Card version is invalid")
	}
	if _, err := contracts.ParseTraceID(string(value.TraceID)); err != nil {
		return err
	}
	if value.Caller.Type != "user" {
		return errors.New("dispatch caller is invalid")
	}
	var input map[string]json.RawMessage
	if json.Unmarshal(value.Input, &input) != nil || input == nil {
		return errors.New("dispatch input must be object")
	}
	return nil
}

func (handler *DispatchHandler) writeGeneratedPreError(writer http.ResponseWriter, code contracts.PlatformErrorCode) {
	traceID, err := newTraceID()
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	handler.writePreError(writer, traceID, code)
}

func (handler *DispatchHandler) writePreError(writer http.ResponseWriter, traceID contracts.TraceID, code contracts.PlatformErrorCode) {
	status := errorStatus(code)
	payload, err := contracts.NewPreCorrelationPlatformErrorV4(code, traceID)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeJSON(writer, status, traceID, payload)
}

func (handler *DispatchHandler) writeCorrelatedError(writer http.ResponseWriter, request contracts.DispatchInvocationRequestV3, code contracts.PlatformErrorCode) {
	status := errorStatus(code)
	payload, err := contracts.NewCorrelatedPlatformErrorV4(code, request.TraceID, request.InvocationID, request.RootTaskID)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeJSON(writer, status, request.TraceID, payload)
}

func (handler *DispatchHandler) writeInvocationResult(writer http.ResponseWriter, request contracts.DispatchInvocationRequestV3, result json.RawMessage) {
	payload := contracts.InvocationResult{
		SchemaVersion: contracts.InvocationResultSchemaVersion,
		InvocationID:  request.InvocationID,
		RootTaskID:    request.RootTaskID,
		TraceID:       request.TraceID,
		Status:        "succeeded",
		Result:        result,
	}
	writeJSON(writer, http.StatusOK, request.TraceID, payload)
}

func (handler *DispatchHandler) dispatchNonStreamingWithLedger(ctx context.Context, writer http.ResponseWriter, request contracts.DispatchInvocationRequestV3, resolved contracts.ResolveAgentResponse, targetErr error) {
	appendTerminal := func(event contracts.InvocationEventV03) error {
		appendCtx, release := ledgerContext(ctx)
		defer release()
		return handler.ledger.Append(appendCtx, event)
	}
	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	for _, event := range []contracts.InvocationEventV03{
		lifecycleEvent(request, 0, "created", "pending", startedAt),
		lifecycleEvent(request, 1, "routing", "routing", startedAt.Add(time.Microsecond)),
	} {
		if err := handler.ledger.Append(ctx, event); err != nil {
			handler.writeCorrelatedError(writer, request, contracts.ErrorCodeDependency)
			return
		}
	}
	if targetErr != nil {
		code := dispatchErrorCode(targetErr)
		event, buildErr := terminalLifecycleEvent(request, 2, "failed", "failed", startedAt.Add(2*time.Microsecond), 0, code)
		if buildErr != nil || appendTerminal(event) != nil {
			handler.writeCorrelatedError(writer, request, contracts.ErrorCodeDependency)
			return
		}
		handler.writeCorrelatedError(writer, request, code)
		return
	}
	if err := handler.ledger.Append(ctx, lifecycleEvent(request, 2, "started", "running", startedAt.Add(2*time.Microsecond))); err != nil {
		handler.writeCorrelatedError(writer, request, contracts.ErrorCodeDependency)
		return
	}

	result, err := handler.transport.SendNonStreaming(ctx, request, resolved)
	latencyMS := time.Since(startedAt).Milliseconds()
	terminalAt := startedAt.Add(3 * time.Microsecond)
	if err != nil {
		code := dispatchErrorCode(err)
		eventType, status := "failed", "failed"
		switch code {
		case contracts.ErrorCodeTimeout:
			eventType, status = "timed_out", "timed_out"
		case contracts.ErrorCodeCanceled:
			eventType, status = "canceled", "canceled"
		}
		event, buildErr := terminalLifecycleEvent(request, 3, eventType, status, terminalAt, latencyMS, code)
		if buildErr != nil || appendTerminal(event) != nil {
			handler.writeCorrelatedError(writer, request, contracts.ErrorCodeDependency)
			return
		}
		handler.writeCorrelatedError(writer, request, code)
		return
	}

	event := lifecycleEvent(request, 3, "succeeded", "succeeded", terminalAt)
	event.LatencyMS = &latencyMS
	if err := appendTerminal(event); err != nil {
		handler.writeCorrelatedError(writer, request, contracts.ErrorCodeDependency)
		return
	}
	handler.writeInvocationResult(writer, request, result)
}

func ledgerContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx.Err() == nil {
		return ctx, func() {}
	}
	return context.WithTimeout(context.WithoutCancel(ctx), ledgerCommitGrace)
}

func lifecycleEvent(request contracts.DispatchInvocationRequestV3, sequence int64, eventType, status string, occurredAt time.Time) contracts.InvocationEventV03 {
	return contracts.InvocationEventV03{
		SchemaVersion:    contracts.RuntimeInvocationEventSchemaVersion,
		EventID:          lifecycleEventID(request.InvocationID, sequence, eventType),
		Sequence:         sequence,
		OccurredAt:       occurredAt.UTC().Format(time.RFC3339Nano),
		Type:             eventType,
		Status:           status,
		InvocationID:     request.InvocationID,
		RootTaskID:       request.RootTaskID,
		TraceID:          request.TraceID,
		Caller:           request.Caller,
		WorkspaceID:      request.WorkspaceID,
		TargetAgentID:    request.TargetAgentID,
		AgentCardVersion: request.AgentCardVersion,
		Capability:       request.Capability,
	}
}

func terminalLifecycleEvent(request contracts.DispatchInvocationRequestV3, sequence int64, eventType, status string, occurredAt time.Time, latencyMS int64, code contracts.PlatformErrorCode) (contracts.InvocationEventV03, error) {
	event := lifecycleEvent(request, sequence, eventType, status, occurredAt)
	event.LatencyMS = &latencyMS
	platformError, err := contracts.NewCorrelatedPlatformErrorV4(code, request.TraceID, request.InvocationID, request.RootTaskID)
	if err != nil {
		return contracts.InvocationEventV03{}, err
	}
	event.Error = &platformError
	return event, nil
}

func lifecycleEventID(invocationID string, sequence int64, eventType string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", invocationID, sequence, eventType)))
	return fmt.Sprintf("evt-%s-%d", hex.EncodeToString(sum[:8]), sequence)
}

func writeJSON(writer http.ResponseWriter, status int, traceID contracts.TraceID, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set(TraceHeader, string(traceID))
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeRawJSON(writer http.ResponseWriter, status int, traceID contracts.TraceID, body []byte) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set(TraceHeader, string(traceID))
	writer.WriteHeader(status)
	_, _ = writer.Write(body)
}

func authErrorCode(err error) contracts.PlatformErrorCode {
	if errors.Is(err, auth.ErrForbidden) {
		return contracts.ErrorCodeForbidden
	}
	return contracts.ErrorCodeUnauthenticated
}

func errorStatus(code contracts.PlatformErrorCode) int {
	switch code {
	case contracts.ErrorCodeValidationError:
		return http.StatusBadRequest
	case contracts.ErrorCodeConflict, contracts.ErrorCodeCanceled:
		return http.StatusConflict
	case contracts.ErrorCodeUnauthenticated:
		return http.StatusUnauthorized
	case contracts.ErrorCodeForbidden, contracts.ErrorCodeCapabilityNotAllowed, contracts.ErrorCodeInstallationDisabled, contracts.ErrorCodeAgentDisabled:
		return http.StatusForbidden
	case contracts.ErrorCodeAgentNotInstalled, contracts.ErrorCodeNotFound:
		return http.StatusNotFound
	case contracts.ErrorCodeNotAcceptable:
		return http.StatusNotAcceptable
	case contracts.ErrorCodeAgentAuthUnsupported, contracts.ErrorCodeAgentResponseTooLarge, contracts.ErrorCodeAgentExecutionFailed, contracts.ErrorCodeA2AProtocol:
		return http.StatusBadGateway
	case contracts.ErrorCodePayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case contracts.ErrorCodeTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusServiceUnavailable
	}
}

func dispatchErrorCode(err error) contracts.PlatformErrorCode {
	if errors.Is(err, context.DeadlineExceeded) {
		return contracts.ErrorCodeTimeout
	}
	var coded platformErrorCoder
	if errors.As(err, &coded) {
		return coded.PlatformErrorCode()
	}
	return contracts.ErrorCodeDependency
}

var traceSource io.Reader = rand.Reader

func newTraceID() (contracts.TraceID, error) {
	data := make([]byte, 16)
	if _, err := io.ReadFull(traceSource, data); err != nil {
		return "", err
	}
	return contracts.TraceID("trc_" + hex.EncodeToString(data) + "_1"), nil
}

func validIdentifier(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for index, character := range []byte(value) {
		if character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == ':' || character == '-' {
			if index > 0 || character != '.' && character != '_' && character != ':' && character != '-' {
				continue
			}
		}
		return false
	}
	return true
}

type jsonFrame struct {
	object    bool
	expecting bool
	members   map[string]struct{}
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}

func rejectDuplicateMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var stack []jsonFrame
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				stack = append(stack, jsonFrame{object: true, expecting: true, members: map[string]struct{}{}})
			case '[':
				stack = append(stack, jsonFrame{})
			case '}', ']':
				stack = stack[:len(stack)-1]
				markValueConsumed(stack)
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].expecting {
				current := &stack[len(stack)-1]
				if _, exists := current.members[value]; exists {
					return fmt.Errorf("duplicate member %q", value)
				}
				current.members[value] = struct{}{}
				current.expecting = false
			} else {
				markValueConsumed(stack)
			}
		default:
			markValueConsumed(stack)
		}
	}
}

func markValueConsumed(stack []jsonFrame) {
	if len(stack) > 0 && stack[len(stack)-1].object {
		stack[len(stack)-1].expecting = true
	}
}
