package gateway

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/contracts"
)

type ReleaseCatalogService interface {
	CreateRelease(context.Context, catalog.AuthenticatedCaller, string, string, contracts.CreateAgentReleaseRequest) (catalog.AgentRelease, error)
	GetRelease(context.Context, catalog.AuthenticatedCaller, string) (catalog.AgentRelease, error)
	VerifyRelease(context.Context, catalog.AuthenticatedCaller, string) (catalog.AgentRelease, error)
	PublishRelease(context.Context, catalog.AuthenticatedCaller, string) (catalog.AgentRelease, error)
	SuspendRelease(context.Context, catalog.AuthenticatedCaller, string) (catalog.AgentRelease, error)
	RevokeRelease(context.Context, catalog.AuthenticatedCaller, string) (catalog.AgentRelease, error)
}

type ReleaseHandler struct {
	authenticator Authenticator
	service       ReleaseCatalogService
	traces        *TraceGenerator
	logger        *slog.Logger
}

func NewReleaseHandler(authenticator Authenticator, service ReleaseCatalogService, traces *TraceGenerator, logger *slog.Logger) (*ReleaseHandler, error) {
	if authenticator == nil || service == nil || traces == nil || logger == nil {
		return nil, errors.New("agent release gateway dependencies are required")
	}
	return &ReleaseHandler{authenticator: authenticator, service: service, traces: traces, logger: logger}, nil
}

func (handler *ReleaseHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v4/providers/{providerId}/agents/{agentId}/releases", handler.create)
	mux.HandleFunc("GET /v4/releases/{releaseId}", handler.get)
	mux.HandleFunc("POST /v4/releases/{releaseId}/verify", handler.verify)
	mux.HandleFunc("POST /v4/releases/{releaseId}/publish", handler.publish)
	mux.HandleFunc("POST /v4/releases/{releaseId}/suspend", handler.suspend)
	mux.HandleFunc("POST /v4/releases/{releaseId}/revoke", handler.revoke)
}

func (handler *ReleaseHandler) create(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request)
	if !ok {
		return
	}
	var payload contracts.CreateAgentReleaseRequest
	if err := decodeJSON(request, &payload); err != nil {
		handler.fail(writer, request, traceID, err)
		return
	}
	release, err := handler.service.CreateRelease(request.Context(), caller, request.PathValue("providerId"), request.PathValue("agentId"), payload)
	if err != nil {
		handler.fail(writer, request, traceID, err)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusCreated, agentReleaseResponse(release))
}

func (handler *ReleaseHandler) get(writer http.ResponseWriter, request *http.Request) {
	handler.readOrTransition(writer, request, handler.service.GetRelease)
}

func (handler *ReleaseHandler) verify(writer http.ResponseWriter, request *http.Request) {
	handler.readOrTransition(writer, request, handler.service.VerifyRelease)
}

func (handler *ReleaseHandler) publish(writer http.ResponseWriter, request *http.Request) {
	handler.readOrTransition(writer, request, handler.service.PublishRelease)
}

func (handler *ReleaseHandler) suspend(writer http.ResponseWriter, request *http.Request) {
	handler.readOrTransition(writer, request, handler.service.SuspendRelease)
}

func (handler *ReleaseHandler) revoke(writer http.ResponseWriter, request *http.Request) {
	handler.readOrTransition(writer, request, handler.service.RevokeRelease)
}

func (handler *ReleaseHandler) readOrTransition(writer http.ResponseWriter, request *http.Request, operation func(context.Context, catalog.AuthenticatedCaller, string) (catalog.AgentRelease, error)) {
	traceID, caller, ok := handler.begin(writer, request)
	if !ok {
		return
	}
	release, err := operation(request.Context(), caller, request.PathValue("releaseId"))
	if err != nil {
		handler.fail(writer, request, traceID, err)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, agentReleaseResponse(release))
}

func (handler *ReleaseHandler) begin(writer http.ResponseWriter, request *http.Request) (contracts.TraceID, catalog.AuthenticatedCaller, bool) {
	traceID := handler.traces.Next()
	writer.Header().Set(TraceHeader, string(traceID))
	caller, err := handler.authenticator.Authenticate(request)
	if err != nil {
		_ = writeTrustError(writer, traceID, contracts.TrustedErrorUnauthenticated)
		return traceID, catalog.AuthenticatedCaller{}, false
	}
	return traceID, caller, true
}

func (handler *ReleaseHandler) fail(writer http.ResponseWriter, request *http.Request, traceID contracts.TraceID, err error) {
	code := trustErrorCode(err)
	handler.logger.WarnContext(request.Context(), "Agent Release request failed", "trace_id", traceID, "code", code)
	if writeErr := writeTrustError(writer, traceID, code); writeErr != nil {
		handler.logger.ErrorContext(request.Context(), "write Agent Release error failed", "trace_id", traceID, "code", code)
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (handler *ReleaseHandler) writeJSON(writer http.ResponseWriter, traceID contracts.TraceID, status int, value contracts.AgentReleaseResponse) {
	writer.Header().Set(TraceHeader, string(traceID))
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		handler.logger.Error("write Agent Release response failed", "trace_id", traceID)
	}
}

func agentReleaseResponse(release catalog.AgentRelease) contracts.AgentReleaseResponse {
	response := contracts.AgentReleaseResponse{
		ReleaseID: release.ReleaseID, ProviderID: release.ProviderID, AgentID: release.AgentID,
		AgentCardVersion: release.AgentCardVersion, CardDigest: hex.EncodeToString(release.CardDigest[:]),
		EndpointBindingID: release.EndpointBindingID, EndpointOrigin: release.EndpointOrigin,
		EndpointPath: release.EndpointPath, VerificationMethod: release.VerificationMethod,
		State: string(release.State), CreatedAt: release.CreatedAt, UpdatedAt: release.UpdatedAt,
		VerifiedAt: release.VerifiedAt, PublishedAt: release.PublishedAt,
		SuspendedAt: release.SuspendedAt, RevokedAt: release.RevokedAt,
	}
	if release.VerificationEvidenceDigest != nil {
		digest := hex.EncodeToString(release.VerificationEvidenceDigest[:])
		response.VerificationEvidenceDigest = &digest
	}
	return response
}
