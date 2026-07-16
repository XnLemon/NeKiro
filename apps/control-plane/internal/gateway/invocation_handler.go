package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/invocation"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace"
	"github.com/Nene7ko/NeKiro/contracts"
)

type InvocationDispatcher interface {
	Dispatch(context.Context, workspace.AuthenticatedCaller, contracts.TraceID, string, contracts.InvokeAgentRequest, []byte, contracts.InvocationResultMode) (*invocation.RouterResponse, error)
}

type InvocationHandler struct {
	authenticator Authenticator
	dispatcher    InvocationDispatcher
	traces        *TraceGenerator
	logger        *slog.Logger
	requestLimit  int64
	sseEventLimit int64
	deadline      time.Duration
}

type invokeRequestWire struct {
	AgentID    json.RawMessage `json:"agentId"`
	Capability json.RawMessage `json:"capability"`
	Input      json.RawMessage `json:"input"`
	Stream     json.RawMessage `json:"stream"`
}

func NewInvocationHandler(authenticator Authenticator, dispatcher InvocationDispatcher, traces *TraceGenerator, logger *slog.Logger, requestLimit, sseEventLimit int64, deadline time.Duration) (*InvocationHandler, error) {
	if authenticator == nil || dispatcher == nil || traces == nil || logger == nil || requestLimit < contracts.RuntimeByteLimitMinimum || requestLimit > contracts.RuntimeByteLimitMaximum || sseEventLimit < contracts.RuntimeByteLimitMinimum || sseEventLimit > contracts.RuntimeByteLimitMaximum || deadline < time.Duration(contracts.RuntimeDeadlineMinimumMS)*time.Millisecond || deadline > time.Duration(contracts.RuntimeDeadlineMaximumMS)*time.Millisecond {
		return nil, errors.New("invocation gateway dependencies and limits are required")
	}
	return &InvocationHandler{authenticator: authenticator, dispatcher: dispatcher, traces: traces, logger: logger, requestLimit: requestLimit, sseEventLimit: sseEventLimit, deadline: deadline}, nil
}

func (handler *InvocationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v4/workspaces/{workspaceId}/invocations", handler.invoke)
}

func (handler *InvocationHandler) invoke(writer http.ResponseWriter, request *http.Request) {
	traceID := handler.traces.Next()
	writer.Header().Set(TraceHeader, string(traceID))
	caller, err := handler.authenticator.Authenticate(request)
	if err != nil {
		handler.writePreError(writer, traceID, contracts.ErrorCodeUnauthenticated)
		return
	}
	if request.Header.Get("Content-Type") != "application/json" {
		handler.writePreError(writer, traceID, contracts.ErrorCodeValidationError)
		return
	}
	invokeRequest, input, err := handler.readRequest(request)
	if err != nil {
		code := contracts.ErrorCodeValidationError
		if errors.Is(err, errInvocationPayloadTooLarge) {
			code = contracts.ErrorCodePayloadTooLarge
		}
		handler.writePreError(writer, traceID, code)
		return
	}
	mode, err := contracts.NegotiateInvocationResultMode(invokeRequest.Stream, request.Header.Get("Accept"))
	if err != nil {
		handler.writePreError(writer, traceID, contracts.ErrorCodeNotAcceptable)
		return
	}
	workspaceID := request.PathValue("workspaceId")
	if !workspace.ValidIdentifier(workspaceID) || !workspace.ValidIdentifier(invokeRequest.AgentID) || !workspace.ValidIdentifier(invokeRequest.Capability) {
		handler.writePreError(writer, traceID, contracts.ErrorCodeValidationError)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.deadline)
	defer cancel()
	response, err := handler.dispatcher.Dispatch(ctx, workspace.AuthenticatedCaller{ID: caller.ID, AuthenticationKind: caller.AuthenticationKind}, traceID, workspaceID, invokeRequest, input, mode)
	if err != nil {
		handler.writeDispatchError(writer, traceID, err)
		return
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			handler.logger.WarnContext(request.Context(), "close Router invocation response", "trace_id", traceID)
		}
	}()
	writer.Header().Set("Content-Type", response.ContentType)
	writer.WriteHeader(response.StatusCode)
	if response.ContentType == "text/event-stream" {
		if err := proxySSE(writer, response.Body, handler.sseEventLimit); err != nil {
			handler.logger.WarnContext(request.Context(), "Router SSE proxy interrupted", "trace_id", traceID)
		}
		return
	}
	if err := proxyFlushed(writer, response.Body); err != nil {
		handler.logger.WarnContext(request.Context(), "Router JSON proxy interrupted", "trace_id", traceID)
	}
}

var errInvocationPayloadTooLarge = errors.New("invocation payload is too large")

func (handler *InvocationHandler) readRequest(request *http.Request) (contracts.InvokeAgentRequest, []byte, error) {
	if request.ContentLength > handler.requestLimit {
		return contracts.InvokeAgentRequest{}, nil, errInvocationPayloadTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(request.Body, handler.requestLimit+1))
	if closeErr := request.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return contracts.InvokeAgentRequest{}, nil, err
	}
	if int64(len(data)) > handler.requestLimit {
		return contracts.InvokeAgentRequest{}, nil, errInvocationPayloadTooLarge
	}
	if err := rejectDuplicateMembers(data); err != nil {
		return contracts.InvokeAgentRequest{}, nil, err
	}
	var wire invokeRequestWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return contracts.InvokeAgentRequest{}, nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return contracts.InvokeAgentRequest{}, nil, errors.New("trailing JSON value")
		}
		return contracts.InvokeAgentRequest{}, nil, err
	}
	var agentID, capability string
	var stream bool
	if len(wire.AgentID) == 0 || len(wire.Capability) == 0 || len(wire.Input) == 0 || len(wire.Stream) == 0 || json.Unmarshal(wire.AgentID, &agentID) != nil || json.Unmarshal(wire.Capability, &capability) != nil || json.Unmarshal(wire.Stream, &stream) != nil {
		return contracts.InvokeAgentRequest{}, nil, errors.New("required invocation field is invalid")
	}
	var input map[string]json.RawMessage
	if json.Unmarshal(wire.Input, &input) != nil || input == nil {
		return contracts.InvokeAgentRequest{}, nil, errors.New("invocation input must be an object")
	}
	return contracts.InvokeAgentRequest{AgentID: agentID, Capability: capability, Stream: stream}, append([]byte(nil), wire.Input...), nil
}

func (handler *InvocationHandler) writeDispatchError(writer http.ResponseWriter, traceID contracts.TraceID, err error) {
	var dispatchError *invocation.DispatchError
	if errors.As(err, &dispatchError) {
		if dispatchError.InvocationID != "" {
			handler.writeCorrelatedError(writer, traceID, dispatchError.Code, dispatchError.InvocationID, dispatchError.RootTaskID)
			return
		}
		handler.writePreError(writer, traceID, dispatchError.Code)
		return
	}
	handler.writePreError(writer, traceID, workspaceErrorCode(err))
}

func (handler *InvocationHandler) writePreError(writer http.ResponseWriter, traceID contracts.TraceID, code contracts.PlatformErrorCode) {
	status, err := invocationErrorStatus(code)
	if err != nil {
		status = http.StatusInternalServerError
		code = contracts.ErrorCodeInternal
	}
	payload, buildErr := contracts.NewPreCorrelationPlatformErrorV4(code, traceID)
	if buildErr != nil {
		handler.logger.Error("construct Invocation error", "trace_id", traceID)
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeInvocationJSON(writer, status, payload)
}

func (handler *InvocationHandler) writeCorrelatedError(writer http.ResponseWriter, traceID contracts.TraceID, code contracts.PlatformErrorCode, invocationID, rootTaskID string) {
	status, err := invocationErrorStatus(code)
	if err != nil {
		status = http.StatusInternalServerError
		code = contracts.ErrorCodeInternal
	}
	payload, buildErr := contracts.NewCorrelatedPlatformErrorV4(code, traceID, invocationID, rootTaskID)
	if buildErr != nil {
		handler.logger.Error("construct correlated Invocation error", "trace_id", traceID)
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeInvocationJSON(writer, status, payload)
}

func invocationErrorStatus(code contracts.PlatformErrorCode) (int, error) {
	switch code {
	case contracts.ErrorCodeValidationError:
		return http.StatusBadRequest, nil
	case contracts.ErrorCodeUnauthenticated:
		return http.StatusUnauthorized, nil
	case contracts.ErrorCodeForbidden, contracts.ErrorCodeCapabilityNotAllowed:
		return http.StatusForbidden, nil
	case contracts.ErrorCodeNotFound, contracts.ErrorCodeAgentNotInstalled:
		return http.StatusNotFound, nil
	case contracts.ErrorCodeNotAcceptable:
		return http.StatusNotAcceptable, nil
	case contracts.ErrorCodeConflict, contracts.ErrorCodeInstallationDisabled, contracts.ErrorCodeAgentDisabled, contracts.ErrorCodeCanceled:
		return http.StatusConflict, nil
	case contracts.ErrorCodePayloadTooLarge:
		return http.StatusRequestEntityTooLarge, nil
	case contracts.ErrorCodeAgentAuthUnsupported, contracts.ErrorCodeAgentResponseTooLarge, contracts.ErrorCodeAgentExecutionFailed, contracts.ErrorCodeA2AProtocol:
		return http.StatusBadGateway, nil
	case contracts.ErrorCodeRouteNotFound, contracts.ErrorCodeAgentUnavailable, contracts.ErrorCodeDependency:
		return http.StatusServiceUnavailable, nil
	case contracts.ErrorCodeTimeout:
		return http.StatusGatewayTimeout, nil
	case contracts.ErrorCodeInternal:
		return http.StatusInternalServerError, nil
	default:
		return 0, fmt.Errorf("unsupported Invocation error code %q", code)
	}
}

func writeInvocationJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func proxyFlushed(writer http.ResponseWriter, source io.Reader) error {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return errors.New("response writer does not support flushing")
	}
	buffer := make([]byte, 32*1024)
	for {
		count, readErr := source.Read(buffer)
		if count > 0 {
			if _, err := writer.Write(buffer[:count]); err != nil {
				return err
			}
			flusher.Flush()
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func proxySSE(writer http.ResponseWriter, source io.Reader, limit int64) error {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return errors.New("response writer does not support flushing")
	}
	reader := bufio.NewReader(source)
	for {
		event, err := readSSEEvent(reader, limit)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := writer.Write(event); err != nil {
			return err
		}
		flusher.Flush()
	}
}

func readSSEEvent(reader *bufio.Reader, limit int64) ([]byte, error) {
	first, err := readBoundedLine(reader, limit)
	if err != nil {
		return nil, err
	}
	if bytes.ContainsRune(first, '\r') || !bytes.HasPrefix(first, []byte("data: ")) || len(first) <= len("data: \n") || first[len(first)-1] != '\n' || !json.Valid(first[len("data: "):len(first)-1]) {
		return nil, errors.New("invalid Router SSE data line")
	}
	remaining := limit - int64(len(first))
	if remaining < 1 {
		return nil, errInvocationPayloadTooLarge
	}
	second, err := readBoundedLine(reader, remaining)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
	if !bytes.Equal(second, []byte("\n")) {
		return nil, errors.New("invalid Router SSE delimiter")
	}
	return append(first, second...), nil
}

func readBoundedLine(reader *bufio.Reader, limit int64) ([]byte, error) {
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if int64(len(line)+len(fragment)) > limit {
			return nil, errInvocationPayloadTooLarge
		}
		line = append(line, fragment...)
		if err == nil {
			return line, nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			if errors.Is(err, io.EOF) && len(line) == 0 {
				return nil, io.EOF
			}
			if errors.Is(err, io.EOF) {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
	}
}
