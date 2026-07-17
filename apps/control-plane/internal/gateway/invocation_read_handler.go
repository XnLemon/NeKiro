package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/invocation"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace"
	"github.com/Nene7ko/NeKiro/contracts"
)

type InvocationMetadataReader interface {
	GetInvocation(context.Context, workspace.AuthenticatedCaller, string, string) (*invocation.RouterResponse, error)
	GetTrace(context.Context, workspace.AuthenticatedCaller, string, contracts.TraceID) (*invocation.RouterResponse, error)
}

type InvocationReadHandler struct {
	authenticator Authenticator
	reader        InvocationMetadataReader
	traces        *TraceGenerator
	logger        *slog.Logger
	deadline      time.Duration
	metadataLimit int64
	validator     *contracts.RuntimeContractValidator
}

func NewInvocationReadHandler(authenticator Authenticator, reader InvocationMetadataReader, traces *TraceGenerator, logger *slog.Logger, deadline time.Duration, metadataLimit int64) (*InvocationReadHandler, error) {
	if authenticator == nil || reader == nil || traces == nil || logger == nil || deadline < time.Duration(contracts.RuntimeDeadlineMinimumMS)*time.Millisecond || deadline > time.Duration(contracts.RuntimeDeadlineMaximumMS)*time.Millisecond || metadataLimit < contracts.RuntimeByteLimitMinimum || metadataLimit > contracts.RuntimeByteLimitMaximum {
		return nil, errors.New("invocation read dependencies are required")
	}
	validator, err := contracts.NewRuntimeContractValidator()
	if err != nil {
		return nil, errors.New("invocation read contract validator is unavailable")
	}
	return &InvocationReadHandler{authenticator: authenticator, reader: reader, traces: traces, logger: logger, deadline: deadline, metadataLimit: metadataLimit, validator: validator}, nil
}

func (handler *InvocationReadHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v4/workspaces/{workspaceId}/invocations/{invocationId}", handler.getInvocation)
	mux.HandleFunc("GET /v4/workspaces/{workspaceId}/traces/{traceId}", handler.getTrace)
}

func (handler *InvocationReadHandler) getInvocation(writer http.ResponseWriter, request *http.Request) {
	traceID := handler.traces.Next()
	writer.Header().Set(TraceHeader, string(traceID))
	caller, err := handler.authenticator.Authenticate(request)
	if err != nil {
		handler.writeReadError(writer, traceID, contracts.ErrorCodeUnauthenticated)
		return
	}
	workspaceID, invocationID := request.PathValue("workspaceId"), request.PathValue("invocationId")
	if !workspace.ValidIdentifier(workspaceID) || !workspace.ValidIdentifier(invocationID) {
		handler.writeReadError(writer, traceID, contracts.ErrorCodeValidationError)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.deadline)
	defer cancel()
	response, err := handler.reader.GetInvocation(ctx, workspace.AuthenticatedCaller{ID: caller.ID, AuthenticationKind: caller.AuthenticationKind}, workspaceID, invocationID)
	if err != nil {
		handler.writeReadError(writer, traceID, workspaceReadErrorCode(err))
		return
	}
	handler.writeRouterResponse(writer, request, traceID, response, "Invocation", workspaceID, invocationID)
}

func (handler *InvocationReadHandler) getTrace(writer http.ResponseWriter, request *http.Request) {
	traceID := handler.traces.Next()
	writer.Header().Set(TraceHeader, string(traceID))
	caller, err := handler.authenticator.Authenticate(request)
	if err != nil {
		handler.writeReadError(writer, traceID, contracts.ErrorCodeUnauthenticated)
		return
	}
	workspaceID := request.PathValue("workspaceId")
	requestedTrace, err := contracts.ParseTraceID(request.PathValue("traceId"))
	if !workspace.ValidIdentifier(workspaceID) || err != nil {
		handler.writeReadError(writer, traceID, contracts.ErrorCodeValidationError)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.deadline)
	defer cancel()
	response, err := handler.reader.GetTrace(ctx, workspace.AuthenticatedCaller{ID: caller.ID, AuthenticationKind: caller.AuthenticationKind}, workspaceID, requestedTrace)
	if err != nil {
		handler.writeReadError(writer, traceID, workspaceReadErrorCode(err))
		return
	}
	handler.writeRouterResponse(writer, request, traceID, response, "Trace", workspaceID, string(requestedTrace))
}

func (handler *InvocationReadHandler) writeRouterResponse(writer http.ResponseWriter, request *http.Request, traceID contracts.TraceID, response *invocation.RouterResponse, resource, workspaceID, resourceID string) {
	if response == nil || response.Body == nil {
		handler.writeReadError(writer, traceID, contracts.ErrorCodeDependency)
		return
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			handler.logger.WarnContext(request.Context(), "close Router metadata response", "resource", resource, "trace_id", traceID)
		}
	}()
	if response.ContentType != "application/json" {
		handler.writeReadError(writer, traceID, contracts.ErrorCodeDependency)
		return
	}
	if response.StatusCode == http.StatusNotFound {
		handler.writeReadError(writer, traceID, contracts.ErrorCodeNotFound)
		return
	}
	if response.StatusCode != http.StatusOK {
		handler.writeReadError(writer, traceID, contracts.ErrorCodeDependency)
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, handler.metadataLimit+1))
	if err != nil || int64(len(body)) > handler.metadataLimit || handler.validateMetadataBody(body, resource, workspaceID, resourceID) != nil {
		handler.writeReadError(writer, traceID, contracts.ErrorCodeDependency)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write(body); err != nil {
		handler.logger.WarnContext(request.Context(), "proxy Router metadata response interrupted", "resource", resource, "trace_id", traceID)
	}
}

func (handler *InvocationReadHandler) validateMetadataBody(body []byte, resource, workspaceID, resourceID string) error {
	if err := rejectDuplicateMembers(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	switch resource {
	case "Invocation":
		var detail contracts.InvocationDetailResponseV4
		if err := decoder.Decode(&detail); err != nil {
			return err
		}
		if err := requireReadJSONEOF(decoder); err != nil {
			return err
		}
		if detail.Invocation.InvocationID != resourceID {
			return errors.New("invocation response identity does not match request")
		}
		return handler.validator.ValidateInvocationDetailResponseV4(workspaceID, detail)
	case "Trace":
		var trace contracts.TraceResponseV4
		if err := decoder.Decode(&trace); err != nil {
			return err
		}
		if err := requireReadJSONEOF(decoder); err != nil {
			return err
		}
		requested, err := contracts.ParseTraceID(resourceID)
		if err != nil || trace.TraceID != requested {
			return errors.New("trace response identity does not match request")
		}
		return contracts.ValidateTraceResponseV4(workspaceID, requested, trace)
	default:
		return errors.New("metadata response kind is unsupported")
	}
}

func requireReadJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("metadata response contains trailing JSON")
	}
	return err
}

func (handler *InvocationReadHandler) writeReadError(writer http.ResponseWriter, traceID contracts.TraceID, code contracts.PlatformErrorCode) {
	status, err := invocationErrorStatus(code)
	if err != nil {
		status = http.StatusInternalServerError
		code = contracts.ErrorCodeInternal
	}
	payload, err := contracts.NewPreCorrelationPlatformErrorV4(code, traceID)
	if err != nil {
		handler.logger.Error("construct Invocation metadata error", "trace_id", traceID)
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = jsonEncode(writer, payload)
}

func workspaceReadErrorCode(err error) contracts.PlatformErrorCode {
	switch {
	case errors.Is(err, workspace.ErrInvalid):
		return contracts.ErrorCodeValidationError
	case errors.Is(err, workspace.ErrNotFound):
		return contracts.ErrorCodeNotFound
	case errors.Is(err, workspace.ErrForbidden):
		return contracts.ErrorCodeForbidden
	default:
		return contracts.ErrorCodeDependency
	}
}

func jsonEncode(writer http.ResponseWriter, value any) error {
	return json.NewEncoder(writer).Encode(value)
}
