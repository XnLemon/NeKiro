package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace"
	"github.com/Nene7ko/NeKiro/contracts"
)

type WorkspaceService interface {
	CreateWorkspace(context.Context, workspace.AuthenticatedCaller, contracts.CreateWorkspaceRequest) (contracts.Workspace, error)
	GetWorkspace(context.Context, workspace.AuthenticatedCaller, string) (contracts.Workspace, error)
	Install(context.Context, workspace.AuthenticatedCaller, string, contracts.InstallAgentRequest) (contracts.Installation, error)
	GetInstallation(context.Context, workspace.AuthenticatedCaller, string, string) (contracts.Installation, error)
	ListInstallations(context.Context, workspace.AuthenticatedCaller, string, int, *string) (contracts.InstallationList, error)
	UpdateInstallation(context.Context, workspace.AuthenticatedCaller, string, string, string) (contracts.Installation, error)
	Uninstall(context.Context, workspace.AuthenticatedCaller, string, string) (contracts.Installation, error)
	Resolve(context.Context, contracts.ResolveAgentRequest) (contracts.ResolveAgentResponse, error)
}

type WorkspaceHandler struct {
	authenticator         Authenticator
	internalAuthenticator Authenticator
	service               WorkspaceService
	traces                *TraceGenerator
	logger                *slog.Logger
}

type installRequestWire struct {
	AgentID             string          `json:"agentId"`
	VersionConstraint   string          `json:"versionConstraint"`
	AcceptedPermissions json.RawMessage `json:"acceptedPermissions"`
}

type updateInstallationRequestWire struct {
	Status json.RawMessage `json:"status"`
}

func NewWorkspaceHandler(authenticator, internalAuthenticator Authenticator, service WorkspaceService, traces *TraceGenerator, logger *slog.Logger) (*WorkspaceHandler, error) {
	if authenticator == nil || internalAuthenticator == nil || service == nil || traces == nil || logger == nil {
		return nil, errors.New("workspace gateway dependencies are required")
	}
	return &WorkspaceHandler{authenticator: authenticator, internalAuthenticator: internalAuthenticator, service: service, traces: traces, logger: logger}, nil
}

func (handler *WorkspaceHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

// RegisterRoutes adds Workspace and internal resolution routes to the composed
// Gateway mux.
func (handler *WorkspaceHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v3/workspaces", handler.createWorkspace)
	mux.HandleFunc("GET /v3/workspaces/{workspaceId}", handler.getWorkspace)
	mux.HandleFunc("POST /v3/workspaces/{workspaceId}/installations", handler.install)
	mux.HandleFunc("GET /v3/workspaces/{workspaceId}/installations", handler.listInstallations)
	mux.HandleFunc("GET /v3/workspaces/{workspaceId}/installations/{installationId}", handler.getInstallation)
	mux.HandleFunc("PATCH /v3/workspaces/{workspaceId}/installations/{installationId}", handler.updateInstallation)
	mux.HandleFunc("DELETE /v3/workspaces/{workspaceId}/installations/{installationId}", handler.uninstall)
	mux.HandleFunc("POST /internal/v2/resolve-agent", handler.resolveAgent)
}

func (handler *WorkspaceHandler) begin(writer http.ResponseWriter, request *http.Request, operation string, authenticator Authenticator) (contracts.TraceID, workspace.AuthenticatedCaller, bool) {
	traceID := handler.traces.Next()
	writer.Header().Set(TraceHeader, string(traceID))
	caller, err := authenticator.Authenticate(request)
	if err != nil {
		handler.logger.WarnContext(request.Context(), "workspace authentication failed", "trace_id", traceID, "operation", operation)
		_ = writeWorkspaceError(writer, traceID, contracts.ErrorCodeUnauthenticated, nil)
		return traceID, workspace.AuthenticatedCaller{}, false
	}
	return traceID, workspace.AuthenticatedCaller{ID: caller.ID, AuthenticationKind: caller.AuthenticationKind}, true
}

func (handler *WorkspaceHandler) createWorkspace(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request, "create_workspace", handler.authenticator)
	if !ok {
		return
	}
	var body contracts.CreateWorkspaceRequest
	if err := readStrictJSON(writer, request, &body); err != nil {
		handler.fail(writer, request, traceID, "create_workspace", workspace.ErrInvalid, nil)
		return
	}
	value, err := handler.service.CreateWorkspace(request.Context(), caller, body)
	if err != nil {
		handler.fail(writer, request, traceID, "create_workspace", err, nil)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusCreated, value)
}

func (handler *WorkspaceHandler) getWorkspace(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request, "get_workspace", handler.authenticator)
	if !ok {
		return
	}
	value, err := handler.service.GetWorkspace(request.Context(), caller, request.PathValue("workspaceId"))
	if err != nil {
		handler.fail(writer, request, traceID, "get_workspace", err, nil)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, value)
}

func (handler *WorkspaceHandler) install(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request, "install", handler.authenticator)
	if !ok {
		return
	}
	body, err := readInstallRequest(writer, request)
	if err != nil {
		handler.fail(writer, request, traceID, "install", workspace.ErrInvalid, nil)
		return
	}
	value, err := handler.service.Install(request.Context(), caller, request.PathValue("workspaceId"), body)
	if err != nil {
		handler.fail(writer, request, traceID, "install", err, nil)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusCreated, value)
}

func (handler *WorkspaceHandler) listInstallations(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request, "list_installations", handler.authenticator)
	if !ok {
		return
	}
	limit, cursor, err := parseInstallationQuery(request.URL.RawQuery)
	if err != nil {
		handler.fail(writer, request, traceID, "list_installations", workspace.ErrInvalid, nil)
		return
	}
	value, err := handler.service.ListInstallations(request.Context(), caller, request.PathValue("workspaceId"), limit, cursor)
	if err != nil {
		handler.fail(writer, request, traceID, "list_installations", err, nil)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, value)
}

func (handler *WorkspaceHandler) getInstallation(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request, "get_installation", handler.authenticator)
	if !ok {
		return
	}
	value, err := handler.service.GetInstallation(request.Context(), caller, request.PathValue("workspaceId"), request.PathValue("installationId"))
	if err != nil {
		handler.fail(writer, request, traceID, "get_installation", err, nil)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, value)
}

func (handler *WorkspaceHandler) updateInstallation(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request, "update_installation", handler.authenticator)
	if !ok {
		return
	}
	status, err := readUpdateInstallationStatus(writer, request)
	if err != nil {
		handler.fail(writer, request, traceID, "update_installation", workspace.ErrInvalid, nil)
		return
	}
	value, err := handler.service.UpdateInstallation(request.Context(), caller, request.PathValue("workspaceId"), request.PathValue("installationId"), status)
	if err != nil {
		handler.fail(writer, request, traceID, "update_installation", err, nil)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, value)
}

func (handler *WorkspaceHandler) uninstall(writer http.ResponseWriter, request *http.Request) {
	traceID, caller, ok := handler.begin(writer, request, "uninstall", handler.authenticator)
	if !ok {
		return
	}
	value, err := handler.service.Uninstall(request.Context(), caller, request.PathValue("workspaceId"), request.PathValue("installationId"))
	if err != nil {
		handler.fail(writer, request, traceID, "uninstall", err, nil)
		return
	}
	handler.writeJSON(writer, traceID, http.StatusOK, value)
}

func (handler *WorkspaceHandler) resolveAgent(writer http.ResponseWriter, request *http.Request) {
	generatedTrace := handler.traces.Next()
	writer.Header().Set(TraceHeader, string(generatedTrace))
	caller, err := handler.internalAuthenticator.Authenticate(request)
	if err != nil {
		_ = writeWorkspaceError(writer, generatedTrace, contracts.ErrorCodeUnauthenticated, nil)
		return
	}
	if caller.ID == "" {
		_ = writeWorkspaceError(writer, generatedTrace, contracts.ErrorCodeUnauthenticated, nil)
		return
	}
	var wire struct {
		InvocationID string            `json:"invocationId"`
		RootTaskID   string            `json:"rootTaskId"`
		TraceID      contracts.TraceID `json:"traceId"`
		WorkspaceID  string            `json:"workspaceId"`
		AgentID      string            `json:"agentId"`
		Version      string            `json:"version"`
		Capability   string            `json:"capability"`
	}
	if err := readStrictJSON(writer, request, &wire); err != nil {
		_ = writeWorkspaceError(writer, generatedTrace, contracts.ErrorCodeValidationError, nil)
		return
	}
	requestValue := contracts.ResolveAgentRequestV2{
		InvocationID: wire.InvocationID, RootTaskID: wire.RootTaskID, TraceID: wire.TraceID,
		WorkspaceID: wire.WorkspaceID, AgentID: wire.AgentID, Version: wire.Version, Capability: wire.Capability,
	}
	if !validCorrelation(requestValue) {
		_ = writeWorkspaceError(writer, generatedTrace, contracts.ErrorCodeValidationError, nil)
		return
	}
	if err := contracts.ValidateResolveAgentRequestV1(requestValue); err != nil {
		_ = writeWorkspaceError(writer, requestValue.TraceID, contracts.ErrorCodeValidationError, &requestValue)
		return
	}
	value, err := handler.service.Resolve(request.Context(), requestValue)
	if err != nil {
		handler.fail(writer, request, requestValue.TraceID, "resolve_agent", err, &requestValue)
		return
	}
	handler.writeJSON(writer, requestValue.TraceID, http.StatusOK, value)
}

func (handler *WorkspaceHandler) fail(writer http.ResponseWriter, request *http.Request, traceID contracts.TraceID, operation string, err error, correlation *contracts.ResolveAgentRequest) {
	code := workspaceErrorCode(err)
	handler.logger.WarnContext(request.Context(), "workspace request failed", "trace_id", traceID, "operation", operation, "code", code)
	_ = writeWorkspaceError(writer, traceID, code, correlation)
}

func (handler *WorkspaceHandler) writeJSON(writer http.ResponseWriter, traceID contracts.TraceID, status int, value any) {
	writer.Header().Set(TraceHeader, string(traceID))
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		handler.logger.Error("write Workspace response failed", "trace_id", traceID)
	}
}

func parseInstallationQuery(rawQuery string) (int, *string, error) {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return 0, nil, err
	}
	for name, items := range values {
		if name != "limit" && name != "cursor" || len(items) != 1 {
			return 0, nil, errors.New("invalid installation query")
		}
	}
	limitValue, exists := values["limit"]
	if !exists || len(limitValue) != 1 {
		return 0, nil, errors.New("installation limit is required")
	}
	limit, err := strconv.Atoi(limitValue[0])
	if err != nil || limit < contracts.InstallationMinimumLimit || limit > contracts.InstallationMaximumLimit {
		return 0, nil, errors.New("installation limit is invalid")
	}
	cursorValue, exists := values["cursor"]
	if !exists {
		return limit, nil, nil
	}
	return limit, &cursorValue[0], nil
}

func readStrictJSON(writer http.ResponseWriter, request *http.Request, destination any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, contracts.WorkspaceRequestMaximumBodyBytes)
	data, err := io.ReadAll(request.Body)
	if closeErr := request.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := rejectDuplicateMembers(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func readInstallRequest(writer http.ResponseWriter, request *http.Request) (contracts.InstallAgentRequest, error) {
	var wire installRequestWire
	if err := readStrictJSON(writer, request, &wire); err != nil {
		return contracts.InstallAgentRequest{}, err
	}
	permissions := bytes.TrimSpace(wire.AcceptedPermissions)
	if len(permissions) == 0 || bytes.Equal(permissions, []byte("null")) {
		return contracts.InstallAgentRequest{}, errors.New("acceptedPermissions is required")
	}
	var accepted []string
	if err := json.Unmarshal(permissions, &accepted); err != nil || accepted == nil {
		return contracts.InstallAgentRequest{}, errors.New("acceptedPermissions must be an array")
	}
	return contracts.InstallAgentRequest{
		AgentID:             wire.AgentID,
		VersionConstraint:   wire.VersionConstraint,
		AcceptedPermissions: accepted,
	}, nil
}

func readUpdateInstallationStatus(writer http.ResponseWriter, request *http.Request) (string, error) {
	var wire updateInstallationRequestWire
	if err := readStrictJSON(writer, request, &wire); err != nil {
		return "", err
	}
	status := bytes.TrimSpace(wire.Status)
	if len(status) == 0 || bytes.Equal(status, []byte("null")) {
		return "", errors.New("status is required")
	}
	var value string
	if err := json.Unmarshal(status, &value); err != nil || (value != "enabled" && value != "disabled") {
		return "", errors.New("status is invalid")
	}
	return value, nil
}

func rejectDuplicateMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	type jsonFrame struct {
		object, expecting bool
		members           map[string]struct{}
	}
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
				if len(stack) == 0 {
					return errors.New("unexpected JSON delimiter")
				}
				stack = stack[:len(stack)-1]
				if len(stack) > 0 && stack[len(stack)-1].object {
					stack[len(stack)-1].expecting = true
				}
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].expecting {
				current := &stack[len(stack)-1]
				if _, exists := current.members[value]; exists {
					return fmt.Errorf("duplicate JSON member %q", value)
				}
				current.members[value] = struct{}{}
				current.expecting = false
			} else if len(stack) > 0 && stack[len(stack)-1].object {
				stack[len(stack)-1].expecting = true
			}
		default:
			if len(stack) > 0 && stack[len(stack)-1].object {
				stack[len(stack)-1].expecting = true
			}
		}
	}
}

func validCorrelation(request contracts.ResolveAgentRequest) bool {
	if !workspace.ValidIdentifier(request.InvocationID) || !workspace.ValidIdentifier(request.RootTaskID) || !workspace.ValidIdentifier(request.WorkspaceID) || !workspace.ValidIdentifier(request.AgentID) || !workspace.ValidIdentifier(request.Capability) {
		return false
	}
	_, err := contracts.ParseTraceID(string(request.TraceID))
	return err == nil
}

func workspaceErrorCode(err error) contracts.PlatformErrorCode {
	switch {
	case errors.Is(err, workspace.ErrInvalid):
		return contracts.ErrorCodeValidationError
	case errors.Is(err, workspace.ErrForbidden):
		return contracts.ErrorCodeForbidden
	case errors.Is(err, workspace.ErrNotFound):
		return contracts.ErrorCodeNotFound
	case errors.Is(err, workspace.ErrConflict):
		return contracts.ErrorCodeConflict
	case errors.Is(err, workspace.ErrAgentNotInstalled):
		return contracts.ErrorCodeAgentNotInstalled
	case errors.Is(err, workspace.ErrInstallationDisabled):
		return contracts.ErrorCodeInstallationDisabled
	case errors.Is(err, workspace.ErrAgentDisabled):
		return contracts.ErrorCodeAgentDisabled
	case errors.Is(err, workspace.ErrCapabilityNotAllowed):
		return contracts.ErrorCodeCapabilityNotAllowed
	case errors.Is(err, workspace.ErrDependency):
		return contracts.ErrorCodeDependency
	default:
		return contracts.ErrorCodeInternal
	}
}

func writeWorkspaceError(writer http.ResponseWriter, traceID contracts.TraceID, code contracts.PlatformErrorCode, correlation *contracts.ResolveAgentRequest) error {
	status, err := workspaceErrorStatus(code)
	if err != nil {
		return err
	}
	var payload any
	if correlation != nil {
		correlated, err := contracts.NewCorrelatedPlatformErrorV3(code, correlation.TraceID, correlation.InvocationID, correlation.RootTaskID)
		if err != nil {
			return err
		}
		payload = correlated
	} else {
		base, err := contracts.NewPlatformErrorV3(code, traceID)
		if err != nil {
			return err
		}
		payload = base
	}
	writer.Header().Set(TraceHeader, string(traceID))
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	return json.NewEncoder(writer).Encode(payload)
}

func workspaceErrorStatus(code contracts.PlatformErrorCode) (int, error) {
	switch code {
	case contracts.ErrorCodeValidationError:
		return http.StatusBadRequest, nil
	case contracts.ErrorCodeUnauthenticated:
		return http.StatusUnauthorized, nil
	case contracts.ErrorCodeForbidden, contracts.ErrorCodeAgentDisabled, contracts.ErrorCodeInstallationDisabled, contracts.ErrorCodeCapabilityNotAllowed:
		return http.StatusForbidden, nil
	case contracts.ErrorCodeNotFound, contracts.ErrorCodeAgentNotInstalled:
		return http.StatusNotFound, nil
	case contracts.ErrorCodeConflict:
		return http.StatusConflict, nil
	case contracts.ErrorCodeDependency:
		return http.StatusServiceUnavailable, nil
	case contracts.ErrorCodeInternal:
		return http.StatusInternalServerError, nil
	default:
		return 0, fmt.Errorf("unsupported Workspace error code %q", code)
	}
}
