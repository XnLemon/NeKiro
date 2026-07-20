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
	"iter"
	"net/http"
	"time"

	semver "github.com/Masterminds/semver/v3"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/auth"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/resolution"
	streammodel "github.com/Nene7ko/NeKiro/apps/a2a-router/internal/stream"
	"github.com/Nene7ko/NeKiro/contracts"
)

const TraceHeader = "x-nek-trace-id"

// ledgerCommitGrace bounds the one terminal-fact persistence attempt after a
// caller deadline/cancellation without turning it into an unbounded write.
const ledgerCommitGrace = time.Second

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

type StreamingTransport interface {
	SendStreaming(context.Context, contracts.DispatchInvocationRequestV3, contracts.ResolveAgentResponse) iter.Seq2[streammodel.Event, error]
	ValidateStreamingTarget(contracts.DispatchInvocationRequestV3, contracts.ResolveAgentResponse) error
	ValidateStreamingInput(contracts.DispatchInvocationRequestV3, contracts.ResolveAgentResponse) error
}

var errSSEFrameTooLarge = errors.New("SSE event exceeds the configured limit")

type sseFrameError struct {
	code  contracts.PlatformErrorCode
	cause error
}

func (err *sseFrameError) Error() string                                  { return err.cause.Error() }
func (err *sseFrameError) Unwrap() error                                  { return err.cause }
func (err *sseFrameError) PlatformErrorCode() contracts.PlatformErrorCode { return err.code }

type resultStreamWriter struct {
	writer    http.ResponseWriter
	flusher   http.Flusher
	limit     int64
	committed bool
}

func newResultStreamWriter(writer http.ResponseWriter, limit int64) (*resultStreamWriter, error) {
	if writer == nil {
		return nil, errors.New("SSE response writer is required")
	}
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return nil, errors.New("SSE streaming is not supported")
	}
	if limit < contracts.RuntimeByteLimitMinimum || limit > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("SSE event limit is invalid")
	}
	return &resultStreamWriter{writer: writer, flusher: flusher, limit: limit}, nil
}

func (writer *resultStreamWriter) Write(event contracts.InvocationResultStreamEventV2) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return &sseFrameError{code: contracts.ErrorCodeA2AProtocol, cause: err}
	}
	if bytes.IndexByte(payload, '\n') >= 0 || bytes.IndexByte(payload, '\r') >= 0 {
		return &sseFrameError{code: contracts.ErrorCodeA2AProtocol, cause: errors.New("SSE JSON contains a physical line break")}
	}
	frame := make([]byte, 0, len(payload)+len("data: \n\n"))
	frame = append(frame, "data: "...)
	frame = append(frame, payload...)
	frame = append(frame, '\n', '\n')
	if int64(len(frame)) > writer.limit {
		return &sseFrameError{code: contracts.ErrorCodeAgentResponseTooLarge, cause: errSSEFrameTooLarge}
	}
	if !writer.committed {
		header := writer.writer.Header()
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-cache")
		header.Set("Connection", "keep-alive")
		header.Set("X-Accel-Buffering", "no")
		writer.writer.WriteHeader(http.StatusOK)
		writer.committed = true
	}
	count, writeErr := writer.writer.Write(frame)
	if writeErr != nil {
		return writeErr
	}
	if count != len(frame) {
		return io.ErrShortWrite
	}
	if err := http.NewResponseController(writer.writer).Flush(); err != nil {
		return err
	}
	return nil
}

type InvocationLedgerAppender interface {
	Append(context.Context, contracts.InvocationEventV03) error
}

type platformErrorCoder interface {
	PlatformErrorCode() contracts.PlatformErrorCode
}

type DispatchHandler struct {
	authenticator      Authenticator
	resolver           Resolver
	transport          NonStreamingTransport
	streaming          StreamingTransport
	ledger             InvocationLedgerAppender
	requestLimit       int64
	deadline           time.Duration
	sseEventLimitBytes int64
	streamValidator    *contracts.RuntimeContractValidator
}

func NewDispatchHandler(authenticator Authenticator, resolver Resolver, requestLimit int64, deadline time.Duration) (*DispatchHandler, error) {
	if authenticator == nil || resolver == nil || requestLimit < contracts.RuntimeByteLimitMinimum || requestLimit > contracts.RuntimeByteLimitMaximum || deadline < time.Duration(contracts.RuntimeDeadlineMinimumMS)*time.Millisecond || deadline > time.Duration(contracts.RuntimeDeadlineMaximumMS)*time.Millisecond {
		return nil, errors.New("router dispatch dependencies are required")
	}
	streamValidator, err := contracts.NewRuntimeContractValidator()
	if err != nil {
		return nil, fmt.Errorf("router runtime stream validator is unavailable: %w", err)
	}
	return &DispatchHandler{authenticator: authenticator, resolver: resolver, requestLimit: requestLimit, deadline: deadline, streamValidator: streamValidator}, nil
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
	if streaming, ok := transport.(StreamingTransport); ok {
		handler.streaming = streaming
	}
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

func NewDispatchHandlerWithTransportAndLedgerAndStreaming(authenticator Authenticator, resolver Resolver, transport NonStreamingTransport, ledger InvocationLedgerAppender, sseEventLimitBytes int64, requestLimit int64, deadline time.Duration) (*DispatchHandler, error) {
	handler, err := NewDispatchHandlerWithTransportAndLedger(authenticator, resolver, transport, ledger, requestLimit, deadline)
	if err != nil {
		return nil, err
	}
	streaming, ok := transport.(StreamingTransport)
	if !ok {
		return nil, errors.New("router streaming transport is required")
	}
	if sseEventLimitBytes < contracts.RuntimeByteLimitMinimum || sseEventLimitBytes > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("router SSE event limit is invalid")
	}
	handler.streaming = streaming
	handler.sseEventLimitBytes = sseEventLimitBytes
	return handler, nil
}

func (handler *DispatchHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/v3/invocations", handler.dispatch)
}

// DispatchChild performs resolution, transport, and Ledger for an
// already-validated child Invocation request. It is called by the nested
// Agent handler after authentication, parent validation, and child context
// derivation. The accept header controls JSON/SSE result mode.
// Unlike the internal dispatch path, DispatchChild accepts caller type
// "agent" and propagates ParentInvocationID to Ledger events.
func (handler *DispatchHandler) DispatchChild(writer http.ResponseWriter, request *http.Request, dispatchRequest contracts.DispatchInvocationRequestV3, accept string) {
	if _, err := contracts.NegotiateInvocationResultMode(dispatchRequest.Stream, accept); err != nil {
		handler.writePreError(writer, dispatchRequest.TraceID, contracts.ErrorCodeNotAcceptable)
		return
	}
	if err := validateChildDispatch(dispatchRequest); err != nil {
		handler.writePreError(writer, dispatchRequest.TraceID, contracts.ErrorCodeValidationError)
		return
	}
	invocationStartedAt := time.Now()
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
			// Map through the Agent boundary; never forward internal
			// Control Plane codes or body across Agent Router v1.
			code = mapControlPlaneCodeToAgentBoundary(failure.Code)
		} else if errors.Is(err, context.DeadlineExceeded) {
			code = contracts.ErrorCodeTimeout
		} else if errors.Is(err, context.Canceled) {
			code = contracts.ErrorCodeCanceled
		}
		handler.writePreError(writer, dispatchRequest.TraceID, code)
		return
	}
	if handler.transport != nil && dispatchRequest.Stream {
		if handler.streaming == nil || handler.ledger == nil {
			handler.writeCorrelatedError(writer, dispatchRequest, contracts.ErrorCodeRouteNotFound)
			return
		}
		streamingPreflight, ok := handler.transport.(StreamingTransport)
		if !ok {
			handler.writeCorrelatedError(writer, dispatchRequest, contracts.ErrorCodeRouteNotFound)
			return
		}
		targetErr := streamingPreflight.ValidateStreamingTarget(dispatchRequest, resolved)
		inputValidator := streamingPreflight.ValidateStreamingInput
		if targetErr != nil {
			handler.dispatchNonStreamingWithLedger(ctx, writer, dispatchRequest, resolved, targetErr)
			return
		}
		if err := inputValidator(dispatchRequest, resolved); err != nil {
			handler.writePreError(writer, dispatchRequest.TraceID, dispatchErrorCode(err))
			return
		}
		streamCtx, streamCancel := resolvedDeadlineContext(ctx, resolved.Card.Limits.TimeoutMS, invocationStartedAt)
		defer streamCancel()
		handler.dispatchStreamingWithLedger(streamCtx, func() {
			cancel()
			streamCancel()
		}, writer, dispatchRequest, resolved)
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
		nonStreamingCtx, nonStreamingCancel := resolvedDeadlineContext(ctx, resolved.Card.Limits.TimeoutMS, invocationStartedAt)
		defer nonStreamingCancel()
		if handler.ledger != nil {
			handler.dispatchNonStreamingWithLedger(nonStreamingCtx, writer, dispatchRequest, resolved, nil)
			return
		}
		result, err := handler.transport.SendNonStreaming(nonStreamingCtx, dispatchRequest, resolved)
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
	invocationStartedAt := time.Now()
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
	if handler.transport != nil && dispatchRequest.Stream {
		if handler.streaming == nil || handler.ledger == nil {
			handler.writeCorrelatedError(writer, dispatchRequest, contracts.ErrorCodeRouteNotFound)
			return
		}
		streamingPreflight, ok := handler.transport.(StreamingTransport)
		if !ok {
			handler.writeCorrelatedError(writer, dispatchRequest, contracts.ErrorCodeRouteNotFound)
			return
		}
		targetErr := streamingPreflight.ValidateStreamingTarget(dispatchRequest, resolved)
		inputValidator := streamingPreflight.ValidateStreamingInput
		if targetErr != nil {
			handler.dispatchNonStreamingWithLedger(ctx, writer, dispatchRequest, resolved, targetErr)
			return
		}
		if err := inputValidator(dispatchRequest, resolved); err != nil {
			handler.writePreError(writer, dispatchRequest.TraceID, dispatchErrorCode(err))
			return
		}
		streamCtx, streamCancel := resolvedDeadlineContext(ctx, resolved.Card.Limits.TimeoutMS, invocationStartedAt)
		defer streamCancel()
		handler.dispatchStreamingWithLedger(streamCtx, func() {
			cancel()
			streamCancel()
		}, writer, dispatchRequest, resolved)
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
		nonStreamingCtx, nonStreamingCancel := resolvedDeadlineContext(ctx, resolved.Card.Limits.TimeoutMS, invocationStartedAt)
		defer nonStreamingCancel()
		if handler.ledger != nil {
			handler.dispatchNonStreamingWithLedger(nonStreamingCtx, writer, dispatchRequest, resolved, nil)
			return
		}
		result, err := handler.transport.SendNonStreaming(nonStreamingCtx, dispatchRequest, resolved)
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

func resolvedDeadlineContext(parent context.Context, timeoutMS int64, invocationStartedAt time.Time) (context.Context, context.CancelFunc) {
	cardDeadline := time.Duration(timeoutMS) * time.Millisecond
	if cardDeadline <= 0 {
		return parent, func() {}
	}
	cardDeadlineAt := invocationStartedAt.Add(cardDeadline)
	if parentDeadline, ok := parent.Deadline(); ok && !cardDeadlineAt.Before(parentDeadline) {
		return parent, func() {}
	}
	return context.WithDeadline(parent, cardDeadlineAt)
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

// validateChildDispatch validates a trusted child dispatch request. Unlike
// validateDispatch, it accepts caller type "agent" and requires a non-empty
// ParentInvocationID for Ledger lineage.
func validateChildDispatch(value contracts.DispatchInvocationRequestV3) error {
	for _, identifier := range []string{value.InvocationID, value.RootTaskID, value.WorkspaceID, value.TargetAgentID, value.Capability, value.Caller.ID} {
		if !validIdentifier(identifier) {
			return errors.New("child dispatch identifier is invalid")
		}
	}
	if _, err := semver.StrictNewVersion(value.AgentCardVersion); err != nil {
		return errors.New("child dispatch Agent Card version is invalid")
	}
	if _, err := contracts.ParseTraceID(string(value.TraceID)); err != nil {
		return err
	}
	if value.Caller.Type != "agent" {
		return errors.New("child dispatch caller must be agent")
	}
	if !validIdentifier(value.ParentInvocationID) {
		return errors.New("child dispatch parent invocation id is invalid")
	}
	var input map[string]json.RawMessage
	if json.Unmarshal(value.Input, &input) != nil || input == nil {
		return errors.New("child dispatch input must be object")
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
	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	initialEvents := []contracts.InvocationEventV03{
		lifecycleEvent(request, 0, "created", "pending", startedAt),
		lifecycleEvent(request, 1, "routing", "routing", startedAt.Add(time.Microsecond)),
	}
	if targetErr == nil {
		initialEvents = append(initialEvents, lifecycleEvent(request, 2, "started", "running", startedAt.Add(2*time.Microsecond)))
	}
	childMode := request.ParentInvocationID != ""
	if !handler.appendInitialLedgerEventsMode(ctx, writer, request, startedAt, initialEvents, childMode) {
		return
	}
	if targetErr != nil {
		code := dispatchErrorCode(targetErr)
		event, buildErr := terminalLifecycleEvent(request, 2, "failed", "failed", terminalOccurredAt(startedAt, 2), 0, code)
		appendCtx, release := ledgerContext(ctx)
		defer release()
		if buildErr != nil || handler.ledger.Append(appendCtx, event) != nil {
			handler.writeCorrelatedError(writer, request, contracts.ErrorCodeDependency)
			return
		}
		handler.writeCorrelatedError(writer, request, code)
		return
	}
	result, err := handler.transport.SendNonStreaming(ctx, request, resolved)
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	latencyMS := time.Since(startedAt).Milliseconds()
	terminalAt := terminalOccurredAt(startedAt, 3)
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
		appendCtx, release := ledgerContext(ctx)
		defer release()
		if buildErr != nil || handler.ledger.Append(appendCtx, event) != nil {
			handler.writeCorrelatedError(writer, request, contracts.ErrorCodeDependency)
			return
		}
		handler.writeCorrelatedError(writer, request, code)
		return
	}

	event := lifecycleEvent(request, 3, "succeeded", "succeeded", terminalAt)
	event.LatencyMS = &latencyMS
	appendCtx, release := ledgerContext(ctx)
	defer release()
	if err := handler.ledger.Append(appendCtx, event); err != nil {
		handler.writeCorrelatedError(writer, request, contracts.ErrorCodeDependency)
		return
	}
	handler.writeInvocationResult(writer, request, result)
}

func (handler *DispatchHandler) dispatchStreamingWithLedger(ctx context.Context, cancel context.CancelFunc, response http.ResponseWriter, request contracts.DispatchInvocationRequestV3, resolved contracts.ResolveAgentResponse) {
	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	childMode := request.ParentInvocationID != ""
	if !handler.appendInitialLedgerEventsMode(ctx, response, request, startedAt, []contracts.InvocationEventV03{
		lifecycleEvent(request, 0, "created", "pending", startedAt),
		lifecycleEvent(request, 1, "routing", "routing", startedAt.Add(time.Microsecond)),
		lifecycleEvent(request, 2, "started", "running", startedAt.Add(2*time.Microsecond)),
	}, childMode) {
		return
	}
	appendEvent := func(event contracts.InvocationEventV03) error {
		appendCtx, release := ledgerContext(ctx)
		defer release()
		return handler.ledger.Append(appendCtx, event)
	}

	streamSequence, err := contracts.NewRuntimeResultStreamSequenceValidator(handler.streamValidator, request.InvocationID, request.RootTaskID, request.TraceID)
	if err != nil {
		handler.writeCorrelatedError(response, request, contracts.ErrorCodeInternal)
		return
	}
	streamWriter, err := newResultStreamWriter(response, handler.sseEventLimitBytes)
	if err != nil {
		if appendErr := handler.appendStreamingTerminal(ctx, request, 3, startedAt, 0, contracts.ErrorCodeDependency, "failed", "failed"); appendErr != nil {
			handler.writeCorrelatedError(response, request, contracts.ErrorCodeDependency)
			return
		}
		handler.writeCorrelatedError(response, request, contracts.ErrorCodeDependency)
		return
	}
	response.Header().Set(TraceHeader, string(request.TraceID))
	accepted := contracts.InvocationResultStreamEventV2{
		SchemaVersion: contracts.RuntimeResultStreamEventSchemaVersion,
		Sequence:      0,
		Type:          contracts.ResultStreamEventAccepted,
		Status:        "pending",
		InvocationID:  request.InvocationID,
		RootTaskID:    request.RootTaskID,
		TraceID:       request.TraceID,
	}
	if err := streamSequence.Accept(accepted); err != nil {
		handler.writeCorrelatedError(response, request, contracts.ErrorCodeInternal)
		return
	}
	if err := streamWriter.Write(accepted); err != nil {
		code := streamWriteErrorCode(ctx, err)
		if !streamWriter.committed {
			if appendErr := handler.appendStreamingTerminal(ctx, request, 3, startedAt, 0, code, "failed", "failed"); appendErr != nil {
				handler.writeCorrelatedError(response, request, contracts.ErrorCodeDependency)
				return
			}
			handler.writeCorrelatedError(response, request, code)
		} else {
			handler.finishStreamingFailure(ctx, cancel, streamWriter, streamSequence, request, startedAt, 1, 3, code)
		}
		return
	}

	sequence := int64(1)
	chunkIndex := int64(0)
	ledgerSequence := int64(3)
	for event, eventErr := range handler.streaming.SendStreaming(ctx, request, resolved) {
		if eventErr != nil {
			code := streamErrorCode(ctx, eventErr)
			handler.finishStreamingFailure(ctx, cancel, streamWriter, streamSequence, request, startedAt, sequence, ledgerSequence, code)
			return
		}
		if len(event.Payload) == 0 || !json.Valid(event.Payload) {
			handler.finishStreamingFailure(ctx, cancel, streamWriter, streamSequence, request, startedAt, sequence, ledgerSequence, contracts.ErrorCodeA2AProtocol)
			return
		}
		streamChunkIndex := chunkIndex
		chunk := contracts.InvocationResultStreamEventV2{
			SchemaVersion: contracts.RuntimeResultStreamEventSchemaVersion,
			Sequence:      sequence,
			Type:          contracts.ResultStreamEventChunk,
			Status:        "running",
			InvocationID:  request.InvocationID,
			RootTaskID:    request.RootTaskID,
			TraceID:       request.TraceID,
			ChunkIndex:    &streamChunkIndex,
			Chunk:         append(json.RawMessage(nil), event.Payload...),
		}
		if err := streamSequence.Accept(chunk); err != nil {
			handler.finishStreamingFailure(ctx, cancel, streamWriter, streamSequence, request, startedAt, sequence, ledgerSequence, contracts.ErrorCodeA2AProtocol)
			return
		}
		chunkBytes := int64(len(event.Payload))
		ledgerChunkIndex := chunkIndex
		ledgerChunkBytes := chunkBytes
		ledgerChunk := lifecycleEvent(request, ledgerSequence, "stream", "running", startedAt.Add(time.Duration(ledgerSequence)*time.Microsecond))
		ledgerChunk.ChunkIndex = &ledgerChunkIndex
		ledgerChunk.ChunkBytes = &ledgerChunkBytes
		if err := appendEvent(ledgerChunk); err != nil {
			handler.finishStreamingFailureWithoutLedger(ctx, cancel, streamWriter, streamSequence, request, sequence+1, contracts.ErrorCodeDependency)
			return
		}
		if err := streamWriter.Write(chunk); err != nil {
			handler.finishStreamingFailure(ctx, cancel, streamWriter, streamSequence, request, startedAt, sequence+1, ledgerSequence+1, streamWriteErrorCode(ctx, err))
			return
		}
		sequence++
		chunkIndex++
		ledgerSequence++

		if event.TerminalType == "" {
			continue
		}
		if ctx.Err() != nil {
			handler.finishStreamingFailure(ctx, cancel, streamWriter, streamSequence, request, startedAt, sequence, ledgerSequence, streamErrorCode(ctx, ctx.Err()))
			return
		}
		terminalType, terminalStatus := streamTerminalType(event.TerminalType, event.TerminalStatus)
		var terminal contracts.InvocationResultStreamEventV2
		if terminalType == contracts.ResultStreamEventCompleted {
			terminal = contracts.InvocationResultStreamEventV2{
				SchemaVersion: contracts.RuntimeResultStreamEventSchemaVersion,
				Sequence:      sequence, Type: terminalType, Status: terminalStatus,
				InvocationID: request.InvocationID, RootTaskID: request.RootTaskID, TraceID: request.TraceID,
			}
		} else {
			code := event.ErrorCode
			if code == "" {
				handler.finishStreamingFailure(ctx, cancel, streamWriter, streamSequence, request, startedAt, sequence, ledgerSequence, contracts.ErrorCodeA2AProtocol)
				return
			}
			terminal, err = streamFailureEvent(request, sequence, terminalType, terminalStatus, code)
			if err != nil {
				handler.finishStreamingFailureWithoutLedger(ctx, cancel, streamWriter, streamSequence, request, sequence, contracts.ErrorCodeInternal)
				return
			}
		}
		if terminal.Type == contracts.ResultStreamEventCompleted {
			ledgerTerminal := lifecycleEvent(request, ledgerSequence, "succeeded", "succeeded", terminalOccurredAt(startedAt, ledgerSequence))
			latency := time.Since(startedAt).Milliseconds()
			ledgerTerminal.LatencyMS = &latency
			if err := appendEvent(ledgerTerminal); err != nil {
				handler.finishStreamingFailureWithoutLedger(ctx, cancel, streamWriter, streamSequence, request, sequence, contracts.ErrorCodeDependency)
				return
			}
		} else {
			code := terminal.Error.Code
			if err := handler.appendStreamingTerminal(ctx, request, ledgerSequence, startedAt, time.Since(startedAt).Milliseconds(), code, string(terminal.Type), terminal.Status); err != nil {
				handler.finishStreamingFailureWithoutLedger(ctx, cancel, streamWriter, streamSequence, request, sequence, contracts.ErrorCodeDependency)
				return
			}
		}
		if err := streamSequence.Accept(terminal); err != nil {
			handler.finishStreamingFailureWithoutLedger(ctx, cancel, streamWriter, streamSequence, request, sequence, contracts.ErrorCodeA2AProtocol)
			return
		}
		if err := streamWriter.Write(terminal); err != nil {
			cancel()
			return
		}
		_ = streamSequence.Finish()
		return
	}

	code := contracts.ErrorCodeA2AProtocol
	if ctx.Err() != nil {
		code = streamErrorCode(ctx, ctx.Err())
	}
	handler.finishStreamingFailure(ctx, cancel, streamWriter, streamSequence, request, startedAt, sequence, ledgerSequence, code)
}

func ledgerContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx.Err() == nil {
		return ctx, func() {}
	}
	return context.WithTimeout(context.WithoutCancel(ctx), ledgerCommitGrace)
}

func (handler *DispatchHandler) appendInitialLedgerEvents(ctx context.Context, writer http.ResponseWriter, request contracts.DispatchInvocationRequestV3, startedAt time.Time, events []contracts.InvocationEventV03) bool {
	return handler.appendInitialLedgerEventsMode(ctx, writer, request, startedAt, events, false)
}

// appendInitialLedgerEventsMode appends initial lifecycle events. When
// childMode is true and the sequence-0 created event fails, a
// pre-correlation error is written because child acceptance never occurred
// (FR-008). After sequence-0 commits, correlated errors are used.
func (handler *DispatchHandler) appendInitialLedgerEventsMode(ctx context.Context, writer http.ResponseWriter, request contracts.DispatchInvocationRequestV3, startedAt time.Time, events []contracts.InvocationEventV03, childMode bool) bool {
	for _, event := range events {
		if err := handler.ledger.Append(ctx, event); err == nil {
			continue
		} else if event.Sequence > 0 {
			if code, eventType, status, ok := contextTerminal(ctx); ok {
				terminal, buildErr := terminalLifecycleEvent(request, event.Sequence, eventType, status, terminalOccurredAt(startedAt, event.Sequence), time.Since(startedAt).Milliseconds(), code)
				appendCtx, release := ledgerContext(ctx)
				appendErr := buildErr
				if appendErr == nil {
					appendErr = handler.ledger.Append(appendCtx, terminal)
				}
				release()
				if appendErr == nil {
					handler.writeCorrelatedError(writer, request, code)
					return false
				}
			}
		}
		// In child mode, sequence-0 failure means child acceptance never
		// occurred; emit a pre-correlation error per FR-008.
		if childMode && event.Sequence == 0 {
			handler.writePreError(writer, request.TraceID, contracts.ErrorCodeDependency)
		} else {
			handler.writeCorrelatedError(writer, request, contracts.ErrorCodeDependency)
		}
		return false
	}
	return true
}

func contextTerminal(ctx context.Context) (contracts.PlatformErrorCode, string, string, bool) {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return contracts.ErrorCodeTimeout, "timed_out", "timed_out", true
	case errors.Is(ctx.Err(), context.Canceled):
		return contracts.ErrorCodeCanceled, "canceled", "canceled", true
	default:
		return "", "", "", false
	}
}

func (handler *DispatchHandler) appendStreamingTerminal(ctx context.Context, request contracts.DispatchInvocationRequestV3, sequence int64, startedAt time.Time, latency int64, code contracts.PlatformErrorCode, eventType, status string) error {
	event, err := terminalLifecycleEvent(request, sequence, eventType, status, terminalOccurredAt(startedAt, sequence), latency, code)
	if err != nil {
		return err
	}
	appendCtx, release := ledgerContext(ctx)
	defer release()
	return handler.ledger.Append(appendCtx, event)
}

func terminalOccurredAt(startedAt time.Time, sequence int64) time.Time {
	occurredAt := time.Now().UTC().Truncate(time.Microsecond)
	if minimum := startedAt.Add(time.Duration(sequence) * time.Microsecond); occurredAt.Before(minimum) {
		return minimum
	}
	return occurredAt
}

func (handler *DispatchHandler) finishStreamingFailure(ctx context.Context, cancel context.CancelFunc, writer *resultStreamWriter, sequence *contracts.RuntimeResultStreamSequenceValidator, request contracts.DispatchInvocationRequestV3, startedAt time.Time, streamSequence, ledgerSequence int64, code contracts.PlatformErrorCode) {
	typeValue, status := streamFailureType(code)
	event, err := streamFailureEvent(request, streamSequence, typeValue, status, code)
	if err != nil {
		cancel()
		return
	}
	if err := handler.appendStreamingTerminal(ctx, request, ledgerSequence, startedAt, time.Since(startedAt).Milliseconds(), code, string(typeValue), status); err != nil {
		handler.finishStreamingFailureWithoutLedger(ctx, cancel, writer, sequence, request, streamSequence, contracts.ErrorCodeDependency)
		return
	}
	if err := sequence.Accept(event); err != nil {
		cancel()
		return
	}
	if err := writer.Write(event); err != nil {
		cancel()
		return
	}
	_ = sequence.Finish()
}

func (handler *DispatchHandler) finishStreamingFailureWithoutLedger(ctx context.Context, cancel context.CancelFunc, writer *resultStreamWriter, sequence *contracts.RuntimeResultStreamSequenceValidator, request contracts.DispatchInvocationRequestV3, streamSequence int64, code contracts.PlatformErrorCode) {
	typeValue, status := streamFailureType(code)
	event, err := streamFailureEvent(request, streamSequence, typeValue, status, code)
	if err != nil {
		cancel()
		return
	}
	if err := sequence.Accept(event); err != nil {
		cancel()
		return
	}
	if err := writer.Write(event); err != nil {
		cancel()
		return
	}
	_ = sequence.Finish()
}

func streamFailureEvent(request contracts.DispatchInvocationRequestV3, sequence int64, eventType contracts.ResultStreamEventType, status string, code contracts.PlatformErrorCode) (contracts.InvocationResultStreamEventV2, error) {
	platformError, err := contracts.NewCorrelatedPlatformErrorV4(code, request.TraceID, request.InvocationID, request.RootTaskID)
	if err != nil {
		return contracts.InvocationResultStreamEventV2{}, err
	}
	return contracts.InvocationResultStreamEventV2{
		SchemaVersion: contracts.RuntimeResultStreamEventSchemaVersion,
		Sequence:      sequence, Type: eventType, Status: status,
		InvocationID: request.InvocationID, RootTaskID: request.RootTaskID, TraceID: request.TraceID,
		Error: &platformError,
	}, nil
}

func streamFailureType(code contracts.PlatformErrorCode) (contracts.ResultStreamEventType, string) {
	switch code {
	case contracts.ErrorCodeTimeout:
		return contracts.ResultStreamEventTimedOut, "timed_out"
	case contracts.ErrorCodeCanceled:
		return contracts.ResultStreamEventCanceled, "canceled"
	default:
		return contracts.ResultStreamEventFailed, "failed"
	}
}

func streamTerminalType(eventType contracts.ResultStreamEventType, status string) (contracts.ResultStreamEventType, string) {
	switch eventType {
	case contracts.ResultStreamEventCompleted:
		return contracts.ResultStreamEventCompleted, "succeeded"
	case contracts.ResultStreamEventCanceled:
		return contracts.ResultStreamEventCanceled, "canceled"
	case contracts.ResultStreamEventTimedOut:
		return contracts.ResultStreamEventTimedOut, "timed_out"
	default:
		if status == "canceled" {
			return contracts.ResultStreamEventCanceled, status
		}
		return contracts.ResultStreamEventFailed, "failed"
	}
}

func streamErrorCode(ctx context.Context, err error) contracts.PlatformErrorCode {
	if errors.Is(err, streammodel.ErrInterrupted) {
		return contracts.ErrorCodeA2AProtocol
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return contracts.ErrorCodeTimeout
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return contracts.ErrorCodeCanceled
	}
	return dispatchErrorCode(err)
}

func streamWriteErrorCode(ctx context.Context, err error) contracts.PlatformErrorCode {
	if ctx.Err() != nil {
		return streamErrorCode(ctx, ctx.Err())
	}
	if dispatchErrorCode(err) == contracts.ErrorCodeAgentResponseTooLarge {
		return contracts.ErrorCodeAgentResponseTooLarge
	}
	return contracts.ErrorCodeDependency
}

func lifecycleEvent(request contracts.DispatchInvocationRequestV3, sequence int64, eventType, status string, occurredAt time.Time) contracts.InvocationEventV03 {
	return contracts.InvocationEventV03{
		SchemaVersion:      contracts.RuntimeInvocationEventSchemaVersion,
		EventID:            lifecycleEventID(request.InvocationID, sequence, eventType),
		Sequence:           sequence,
		OccurredAt:         occurredAt.UTC().Format(time.RFC3339Nano),
		Type:               eventType,
		Status:             status,
		InvocationID:       request.InvocationID,
		RootTaskID:         request.RootTaskID,
		ParentInvocationID: request.ParentInvocationID,
		TraceID:            request.TraceID,
		Caller:             request.Caller,
		WorkspaceID:        request.WorkspaceID,
		TargetAgentID:      request.TargetAgentID,
		AgentCardVersion:   request.AgentCardVersion,
		Capability:         request.Capability,
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
