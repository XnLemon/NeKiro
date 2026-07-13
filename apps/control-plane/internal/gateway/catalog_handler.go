package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/contracts"
)

const registrationBodyReadTimeout = 30 * time.Second

type ReadinessChecker interface {
	Check(context.Context) error
}

type CatalogService interface {
	Register(context.Context, catalog.AuthenticatedCaller, []byte) (contracts.CatalogEntry, error)
	Get(context.Context, catalog.AuthenticatedCaller, string, string) (contracts.CatalogEntry, error)
	Publish(context.Context, catalog.AuthenticatedCaller, string, string) (contracts.CatalogEntry, error)
	Disable(context.Context, catalog.AuthenticatedCaller, string, string) (contracts.CatalogEntry, error)
	Search(context.Context, contracts.SearchAgentsQuery) (catalog.SearchResult, error)
}

type Handler struct {
	authenticator   Authenticator
	catalog         CatalogService
	readiness       ReadinessChecker
	traces          *TraceGenerator
	logger          *slog.Logger
	bodyReadTimeout time.Duration
}

func NewHandler(
	authenticator Authenticator,
	catalogService CatalogService,
	readiness ReadinessChecker,
	traces *TraceGenerator,
	logger *slog.Logger,
) (*Handler, error) {
	if authenticator == nil || catalogService == nil || readiness == nil || traces == nil || logger == nil {
		return nil, errors.New("gateway dependencies are required")
	}
	return &Handler{
		authenticator:   authenticator,
		catalog:         catalogService,
		readiness:       readiness,
		traces:          traces,
		logger:          logger,
		bodyReadTimeout: registrationBodyReadTimeout,
	}, nil
}

func (handler *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", handler.liveness)
	mux.HandleFunc("GET /readyz", handler.readinessCheck)
	mux.HandleFunc("POST /v2/agents", handler.register)
	mux.HandleFunc("GET /v2/agents", handler.search)
	mux.HandleFunc("GET /v2/agents/{agentId}/versions/{version}", handler.get)
	mux.HandleFunc("POST /v2/agents/{agentId}/versions/{version}/publish", handler.publish)
	mux.HandleFunc("POST /v2/agents/{agentId}/versions/{version}/disable", handler.disable)
	return mux
}

func (handler *Handler) liveness(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) readinessCheck(writer http.ResponseWriter, request *http.Request) {
	if err := handler.readiness.Check(request.Context()); err != nil {
		handler.logger.ErrorContext(request.Context(), "readiness check failed", "component", "catalog")
		http.Error(writer, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) register(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.beginCatalogRequest(writer, request, "register")
	if !ok {
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		handler.fail(writer, request, traceID, "register", catalog.ErrInvalid)
		return
	}
	controller := http.NewResponseController(writer)
	if err := controller.SetReadDeadline(time.Now().Add(handler.bodyReadTimeout)); err != nil {
		handler.fail(writer, request, traceID, "register", err)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, contracts.RegistrationMaximumBodyBytes)
	body, readErr := io.ReadAll(request.Body)
	closeErr := request.Body.Close()
	if err := controller.SetReadDeadline(time.Time{}); err != nil {
		handler.fail(writer, request, traceID, "register", err)
		return
	}
	if readErr != nil || closeErr != nil {
		handler.fail(writer, request, traceID, "register", catalog.ErrInvalid)
		return
	}
	entry, err := handler.catalog.Register(request.Context(), caller, body)
	if err != nil {
		handler.fail(writer, request, traceID, "register", err)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusCreated, entry)
}

func (handler *Handler) get(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.beginCatalogRequest(writer, request, "get")
	if !ok {
		return
	}
	entry, err := handler.catalog.Get(request.Context(), caller, request.PathValue("agentId"), request.PathValue("version"))
	if err != nil {
		handler.fail(writer, request, traceID, "get", err)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, entry)
}

func (handler *Handler) publish(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.beginCatalogRequest(writer, request, "publish")
	if !ok {
		return
	}
	entry, err := handler.catalog.Publish(request.Context(), caller, request.PathValue("agentId"), request.PathValue("version"))
	if err != nil {
		handler.fail(writer, request, traceID, "publish", err)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, entry)
}

func (handler *Handler) disable(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.beginCatalogRequest(writer, request, "disable")
	if !ok {
		return
	}
	entry, err := handler.catalog.Disable(request.Context(), caller, request.PathValue("agentId"), request.PathValue("version"))
	if err != nil {
		handler.fail(writer, request, traceID, "disable", err)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, entry)
}

func (handler *Handler) search(writer http.ResponseWriter, request *http.Request) {
	traceID, _, ok := handler.beginCatalogRequest(writer, request, "search")
	if !ok {
		return
	}
	query, err := parseSearchQuery(request.URL.RawQuery)
	if err != nil {
		handler.fail(writer, request, traceID, "search", catalog.ErrInvalid)
		return
	}
	result, err := handler.catalog.Search(request.Context(), query)
	if err != nil {
		handler.fail(writer, request, traceID, "search", err)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, contracts.SearchAgentsResponse{
		Items: result.Entries, NextCursor: result.NextCursor,
	})
}

func (handler *Handler) beginCatalogRequest(
	writer http.ResponseWriter,
	request *http.Request,
	operation string,
) (contracts.TraceID, catalog.AuthenticatedCaller, bool) {
	traceID := handler.traces.Next()
	writer.Header().Set(TraceHeader, string(traceID))
	caller, err := handler.authenticator.Authenticate(request)
	if err != nil {
		handler.logFailure(request.Context(), traceID, operation, contracts.ErrorCodeUnauthenticated)
		_ = writePlatformError(writer, traceID, contracts.ErrorCodeUnauthenticated)
		return traceID, catalog.AuthenticatedCaller{}, false
	}
	return traceID, caller, true
}

func (handler *Handler) fail(writer http.ResponseWriter, request *http.Request, traceID contracts.TraceID, operation string, err error) {
	code := catalogErrorCode(err)
	handler.logFailure(request.Context(), traceID, operation, code)
	if writeErr := writePlatformError(writer, traceID, code); writeErr != nil {
		handler.logger.ErrorContext(request.Context(), "write Platform Error failed", "trace_id", traceID, "operation", operation)
	}
}

func (handler *Handler) logFailure(ctx context.Context, traceID contracts.TraceID, operation string, code contracts.PlatformErrorCode) {
	handler.logger.WarnContext(ctx, "catalog request failed", "trace_id", traceID, "operation", operation, "code", code)
}

func (handler *Handler) writeJSON(writer http.ResponseWriter, traceID contracts.TraceID, status int, value any) {
	writer.Header().Set(TraceHeader, string(traceID))
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		handler.logger.Error("write Catalog response failed", "trace_id", traceID)
	}
}

func catalogErrorCode(err error) contracts.PlatformErrorCode {
	switch {
	case errors.Is(err, catalog.ErrInvalid):
		return contracts.ErrorCodeValidationError
	case errors.Is(err, catalog.ErrForbidden):
		return contracts.ErrorCodeForbidden
	case errors.Is(err, catalog.ErrNotFound):
		return contracts.ErrorCodeNotFound
	case errors.Is(err, catalog.ErrConflict):
		return contracts.ErrorCodeConflict
	case errors.Is(err, catalog.ErrDependency):
		return contracts.ErrorCodeDependency
	default:
		return contracts.ErrorCodeInternal
	}
}

func parseSearchQuery(rawQuery string) (contracts.SearchAgentsQuery, error) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return contracts.SearchAgentsQuery{}, err
	}
	allowed := map[string]bool{"query": true, "capability": true, "ownerId": true, "limit": true, "cursor": true}
	for name, value := range values {
		if !allowed[name] || len(value) != 1 {
			return contracts.SearchAgentsQuery{}, errors.New("unknown or duplicate search parameter")
		}
	}
	var query contracts.SearchAgentsQuery
	query.Query = optionalQueryValue(values, "query")
	query.Capability = optionalQueryValue(values, "capability")
	query.OwnerID = optionalQueryValue(values, "ownerId")
	query.Cursor = optionalQueryValue(values, "cursor")
	if value := optionalQueryValue(values, "limit"); value != nil {
		limit, err := strconv.Atoi(*value)
		if err != nil {
			return contracts.SearchAgentsQuery{}, err
		}
		if limit < contracts.DiscoveryMinimumLimit || limit > contracts.DiscoveryMaximumLimit {
			return contracts.SearchAgentsQuery{}, errors.New("search limit is outside the accepted range")
		}
		query.Limit = &limit
	}
	return query, nil
}

func optionalQueryValue(values url.Values, name string) *string {
	value, exists := values[name]
	if !exists {
		return nil
	}
	return &value[0]
}
