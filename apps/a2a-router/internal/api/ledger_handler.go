package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/auth"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/ledger"
	"github.com/Nene7ko/NeKiro/contracts"
)

type LedgerReader interface {
	GetInvocation(context.Context, string, string) (contracts.InvocationDetailResponseV4, error)
	GetTrace(context.Context, string, contracts.TraceID) (contracts.TraceResponseV4, error)
}

type LedgerHandler struct {
	reader    LedgerReader
	validator *contracts.RuntimeContractValidator
}

func NewLedgerHandler(reader LedgerReader) (*LedgerHandler, error) {
	if reader == nil {
		return nil, errors.New("ledger reader is required")
	}
	validator, err := contracts.NewRuntimeContractValidator()
	if err != nil {
		return nil, err
	}
	return &LedgerHandler{reader: reader, validator: validator}, nil
}

// RegisterRoutes exposes the Router Internal v3 metadata reads. The caller
// owns the process mux and supplies the same authenticated service principal
// boundary used by dispatch; LedgerHandler remains responsible only for
// validating and reading its owned metadata.
func (handler *LedgerHandler) RegisterRoutes(mux *http.ServeMux, authenticator Authenticator) error {
	if mux == nil {
		return errors.New("router read mux is required")
	}
	if authenticator == nil {
		return errors.New("router read authenticator is required")
	}
	mux.HandleFunc("GET /internal/v3/workspaces/{workspaceId}/invocations/{invocationId}", func(writer http.ResponseWriter, request *http.Request) {
		handler.serveInvocationRoute(writer, request, authenticator)
	})
	mux.HandleFunc("GET /internal/v3/workspaces/{workspaceId}/traces/{traceId}", func(writer http.ResponseWriter, request *http.Request) {
		handler.serveTraceRoute(writer, request, authenticator)
	})
	return nil
}

func (handler *LedgerHandler) serveInvocationRoute(writer http.ResponseWriter, request *http.Request, authenticator Authenticator) {
	traceID, err := newTraceID()
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	writer.Header().Set(TraceHeader, string(traceID))
	if _, err := authenticator.Authenticate(request); err != nil {
		handler.writeReadAuthError(writer, traceID, err)
		return
	}
	workspaceID, invocationID := request.PathValue("workspaceId"), request.PathValue("invocationId")
	if !validIdentifier(workspaceID) || !validIdentifier(invocationID) {
		_ = handler.writeReadError(writer, traceID, ledger.ErrNotFound)
		return
	}
	_ = handler.ServeInvocationRead(writer, request, workspaceID, invocationID, traceID)
}

func (handler *LedgerHandler) serveTraceRoute(writer http.ResponseWriter, request *http.Request, authenticator Authenticator) {
	requestTraceID, err := newTraceID()
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	writer.Header().Set(TraceHeader, string(requestTraceID))
	if _, err := authenticator.Authenticate(request); err != nil {
		handler.writeReadAuthError(writer, requestTraceID, err)
		return
	}
	workspaceID := request.PathValue("workspaceId")
	resourceTraceID, err := contracts.ParseTraceID(request.PathValue("traceId"))
	if !validIdentifier(workspaceID) || err != nil {
		_ = handler.writeReadError(writer, requestTraceID, ledger.ErrNotFound)
		return
	}
	_ = handler.ServeTraceRead(writer, request, workspaceID, resourceTraceID)
}

// ServeInvocationRead adapts an already authenticated and path-validated
// Router Internal v3 request. Router authentication and mux ownership stay in
// the process integration layer.
func (handler *LedgerHandler) ServeInvocationRead(
	w http.ResponseWriter,
	r *http.Request,
	workspaceID, invocationID string,
	traceID contracts.TraceID,
) error {
	result, err := handler.reader.GetInvocation(r.Context(), workspaceID, invocationID)
	if err != nil {
		return handler.writeReadError(w, traceID, err)
	}
	if err := handler.validator.ValidateInvocationDetailResponseV4(workspaceID, result); err != nil {
		return handler.writeReadError(w, traceID, ledger.ErrDependency)
	}
	return writeLedgerJSON(w, http.StatusOK, result)
}

// ServeTraceRead has the same authenticated integration precondition as
// ServeInvocationRead.
func (handler *LedgerHandler) ServeTraceRead(
	w http.ResponseWriter,
	r *http.Request,
	workspaceID string,
	traceID contracts.TraceID,
) error {
	result, err := handler.reader.GetTrace(r.Context(), workspaceID, traceID)
	if err != nil {
		return handler.writeReadError(w, traceID, err)
	}
	if err := contracts.ValidateTraceResponseV4(workspaceID, traceID, result); err != nil {
		return handler.writeReadError(w, traceID, ledger.ErrDependency)
	}
	return writeLedgerJSON(w, http.StatusOK, result)
}

func (handler *LedgerHandler) writeReadError(w http.ResponseWriter, traceID contracts.TraceID, err error) error {
	status := http.StatusServiceUnavailable
	code := contracts.ErrorCodeDependency
	switch {
	case errors.Is(err, ledger.ErrValidation):
		status = http.StatusBadRequest
		code = contracts.ErrorCodeValidationError
	case errors.Is(err, ledger.ErrNotFound):
		status = http.StatusNotFound
		code = contracts.ErrorCodeNotFound
	}
	platformError, constructorErr := contracts.NewPreCorrelationPlatformErrorV4(code, traceID)
	if constructorErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return constructorErr
	}
	return writeLedgerJSON(w, status, platformError)
}

func (handler *LedgerHandler) writeReadAuthError(w http.ResponseWriter, traceID contracts.TraceID, authErr error) {
	status := http.StatusUnauthorized
	code := contracts.ErrorCodeUnauthenticated
	if errors.Is(authErr, auth.ErrForbidden) {
		status = http.StatusForbidden
		code = contracts.ErrorCodeForbidden
	}
	platformError, err := contracts.NewPreCorrelationPlatformErrorV4(code, traceID)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	_ = writeLedgerJSON(w, status, platformError)
}

func writeLedgerJSON(w http.ResponseWriter, status int, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(body)
	return err
}
