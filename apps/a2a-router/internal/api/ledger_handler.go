package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

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
	if errors.Is(err, ledger.ErrNotFound) {
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
