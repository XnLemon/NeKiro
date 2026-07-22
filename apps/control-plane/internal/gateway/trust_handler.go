package gateway

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/contracts"
)

type TrustCatalogService interface {
	CreateBindingForCaller(context.Context, catalog.AuthenticatedCaller, string, string, string, string, string) (catalog.EndpointBinding, error)
	CreateChallengeForCaller(context.Context, catalog.AuthenticatedCaller, string, string) (contracts.VerificationChallengeResponse, error)
	CompleteChallengeForCaller(context.Context, catalog.AuthenticatedCaller, string, string, string) (catalog.EndpointBinding, error)
	GetBindingForCaller(context.Context, catalog.AuthenticatedCaller, string, string) (catalog.EndpointBinding, error)
}

type TrustHandler struct {
	authenticator Authenticator
	trust         TrustCatalogService
	traces        *TraceGenerator
	logger        *slog.Logger
}

func NewTrustHandler(authenticator Authenticator, trust TrustCatalogService, traces *TraceGenerator, logger *slog.Logger) (*TrustHandler, error) {
	if authenticator == nil || trust == nil || traces == nil || logger == nil {
		return nil, errors.New("trusted publication gateway dependencies are required")
	}
	return &TrustHandler{authenticator: authenticator, trust: trust, traces: traces, logger: logger}, nil
}

func (handler *TrustHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v4/providers/{providerId}/agents/{agentId}/endpoint-bindings", handler.createBinding)
	mux.HandleFunc("GET /v4/providers/{providerId}/endpoint-bindings/{bindingId}", handler.getBinding)
	mux.HandleFunc("POST /v4/providers/{providerId}/endpoint-bindings/{bindingId}/challenges", handler.createChallenge)
	mux.HandleFunc("POST /v4/providers/{providerId}/endpoint-bindings/{bindingId}/challenges/{challengeId}/complete", handler.completeChallenge)
}

func (handler *TrustHandler) createBinding(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request, "create endpoint binding")
	if !ok {
		return
	}
	var payload contracts.CreateEndpointBindingRequest
	if err := decodeJSON(request, &payload); err != nil {
		handler.fail(writer, request, traceID, err)
		return
	}
	binding, err := handler.trust.CreateBindingForCaller(request.Context(), caller, request.PathValue("providerId"), request.PathValue("agentId"), payload.Version, payload.Endpoint, payload.Method)
	if err != nil {
		handler.fail(writer, request, traceID, err)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusCreated, endpointBindingResponse(binding))
}

func (handler *TrustHandler) getBinding(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request, "get endpoint binding")
	if !ok {
		return
	}
	binding, err := handler.trust.GetBindingForCaller(request.Context(), caller, request.PathValue("providerId"), request.PathValue("bindingId"))
	if err != nil {
		handler.fail(writer, request, traceID, err)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, endpointBindingResponse(binding))
}

func (handler *TrustHandler) createChallenge(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request, "create verification challenge")
	if !ok {
		return
	}
	challenge, err := handler.trust.CreateChallengeForCaller(request.Context(), caller, request.PathValue("providerId"), request.PathValue("bindingId"))
	if err != nil {
		handler.fail(writer, request, traceID, err)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusCreated, challenge)
}

func (handler *TrustHandler) completeChallenge(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request, "complete verification challenge")
	if !ok {
		return
	}
	binding, err := handler.trust.CompleteChallengeForCaller(request.Context(), caller, request.PathValue("providerId"), request.PathValue("bindingId"), request.PathValue("challengeId"))
	if err != nil {
		handler.fail(writer, request, traceID, err)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, endpointBindingResponse(binding))
}

func (handler *TrustHandler) begin(writer http.ResponseWriter, request *http.Request, operation string) (contracts.TraceID, catalog.AuthenticatedCaller, bool) {
	traceID := handler.traces.Next()
	writer.Header().Set(TraceHeader, string(traceID))
	caller, err := handler.authenticator.Authenticate(request)
	if err != nil {
		_ = writeTrustError(writer, traceID, contracts.TrustedErrorUnauthenticated)
		return traceID, catalog.AuthenticatedCaller{}, false
	}
	return traceID, caller, true
}

func (handler *TrustHandler) fail(writer http.ResponseWriter, request *http.Request, traceID contracts.TraceID, err error) {
	code := trustErrorCode(err)
	handler.logger.WarnContext(request.Context(), "trusted publication request failed", "trace_id", traceID, "code", code)
	if writeErr := writeTrustError(writer, traceID, code); writeErr != nil {
		handler.logger.ErrorContext(request.Context(), "write trusted publication Platform Error failed", "trace_id", traceID, "code", code)
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (handler *TrustHandler) writeJSON(writer http.ResponseWriter, traceID contracts.TraceID, status int, value any) {
	writer.Header().Set(TraceHeader, string(traceID))
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func decodeJSON(request *http.Request, destination any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return catalog.ErrInvalid
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, contracts.WorkspaceRequestMaximumBodyBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return catalog.ErrInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return catalog.ErrInvalid
	}
	return nil
}

func endpointBindingResponse(binding catalog.EndpointBinding) contracts.EndpointBindingResponse {
	response := contracts.EndpointBindingResponse{BindingID: binding.BindingID, ProviderID: binding.ProviderID, AgentID: binding.AgentID, AgentCardVersion: binding.AgentCardVersion, Endpoint: binding.Endpoint, VerificationMethod: binding.VerificationMethod, VerificationStatus: string(binding.VerificationStatus), VerificationFailureCode: binding.VerificationFailureCode, CreatedAt: binding.CreatedAt, UpdatedAt: binding.UpdatedAt, VerifiedAt: binding.VerifiedAt, RevokedAt: binding.RevokedAt}
	if binding.VerificationEvidenceDigest != nil {
		digest := hex.EncodeToString(binding.VerificationEvidenceDigest[:])
		response.VerificationEvidenceDigest = &digest
	}
	return response
}

func writeTrustError(writer http.ResponseWriter, traceID contracts.TraceID, code contracts.TrustedPublicationErrorCode) error {
	status, err := trustErrorStatus(code)
	if err != nil {
		return err
	}
	payload, err := contracts.NewTrustedPublicationError(code, traceID)
	if err != nil {
		return fmt.Errorf("construct Trusted Publication Error: %w", err)
	}
	writer.Header().Set(TraceHeader, string(traceID))
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(payload); err != nil {
		return fmt.Errorf("encode Trusted Publication Error: %w", err)
	}
	return nil
}

func trustErrorStatus(code contracts.TrustedPublicationErrorCode) (int, error) {
	switch code {
	case contracts.TrustedErrorValidation, contracts.TrustedErrorInvalidEndpoint, contracts.TrustedErrorWrongProof, contracts.TrustedErrorRedirectNotAllowed:
		return http.StatusBadRequest, nil
	case contracts.TrustedErrorUnauthenticated:
		return http.StatusUnauthorized, nil
	case contracts.TrustedErrorForbidden, contracts.TrustedErrorDisallowedNetwork:
		return http.StatusForbidden, nil
	case contracts.TrustedErrorNotFound:
		return http.StatusNotFound, nil
	case contracts.TrustedErrorConflict, contracts.TrustedErrorChallengeExpired, contracts.TrustedErrorChallengeReused:
		return http.StatusConflict, nil
	case contracts.TrustedErrorEndpointUnavailable, contracts.TrustedErrorDependency:
		return http.StatusServiceUnavailable, nil
	case contracts.TrustedErrorInternal:
		return http.StatusInternalServerError, nil
	default:
		return 0, fmt.Errorf("unsupported trusted publication error code %q", code)
	}
}

func trustErrorCode(err error) contracts.TrustedPublicationErrorCode {
	switch {
	case errors.Is(err, catalog.ErrInvalid), errors.Is(err, catalog.ErrReleaseInvalid):
		return contracts.TrustedErrorValidation
	case errors.Is(err, catalog.ErrEndpointInvalid):
		return contracts.TrustedErrorInvalidEndpoint
	case errors.Is(err, catalog.ErrWrongProof):
		return contracts.TrustedErrorWrongProof
	case errors.Is(err, catalog.ErrRedirectNotAllowed):
		return contracts.TrustedErrorRedirectNotAllowed
	case errors.Is(err, catalog.ErrForbidden):
		return contracts.TrustedErrorForbidden
	case errors.Is(err, catalog.ErrBindingNotFound), errors.Is(err, catalog.ErrChallengeNotFound), errors.Is(err, catalog.ErrProviderNotFound), errors.Is(err, catalog.ErrReleaseNotFound), errors.Is(err, catalog.ErrNotFound):
		return contracts.TrustedErrorNotFound
	case errors.Is(err, catalog.ErrChallengeExpired), errors.Is(err, catalog.ErrChallengeReused), errors.Is(err, catalog.ErrTrustConflict), errors.Is(err, catalog.ErrReleaseConflict):
		if errors.Is(err, catalog.ErrChallengeExpired) {
			return contracts.TrustedErrorChallengeExpired
		}
		if errors.Is(err, catalog.ErrChallengeReused) {
			return contracts.TrustedErrorChallengeReused
		}
		return contracts.TrustedErrorConflict
	case errors.Is(err, catalog.ErrDisallowedNetwork):
		return contracts.TrustedErrorDisallowedNetwork
	case errors.Is(err, catalog.ErrEndpointUnavailable):
		return contracts.TrustedErrorEndpointUnavailable
	case errors.Is(err, catalog.ErrTrustDependency), errors.Is(err, catalog.ErrDependency):
		return contracts.TrustedErrorDependency
	default:
		return contracts.TrustedErrorInternal
	}
}
