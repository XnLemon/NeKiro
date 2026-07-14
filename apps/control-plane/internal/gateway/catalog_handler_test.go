package gateway

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/config"
	"github.com/Nene7ko/NeKiro/contracts"
)

type fakeAuthenticator struct {
	caller catalog.AuthenticatedCaller
	err    error
}

func (authenticator fakeAuthenticator) Authenticate(*http.Request) (catalog.AuthenticatedCaller, error) {
	return authenticator.caller, authenticator.err
}

type fakeReadiness struct{ err error }

func (readiness fakeReadiness) Check(context.Context) error { return readiness.err }

type fakeCatalogService struct {
	registerCaller catalog.AuthenticatedCaller
	registerBody   []byte
	entry          contracts.CatalogEntry
	searchResult   catalog.SearchResult
	err            error
	registerCalls  int
}

func (service *fakeCatalogService) Register(_ context.Context, caller catalog.AuthenticatedCaller, body []byte) (contracts.CatalogEntry, error) {
	service.registerCaller = caller
	service.registerBody = append([]byte(nil), body...)
	service.registerCalls++
	return service.entry, service.err
}
func (service *fakeCatalogService) Get(context.Context, catalog.AuthenticatedCaller, string, string) (contracts.CatalogEntry, error) {
	return service.entry, service.err
}
func (service *fakeCatalogService) Publish(context.Context, catalog.AuthenticatedCaller, string, string) (contracts.CatalogEntry, error) {
	return service.entry, service.err
}
func (service *fakeCatalogService) Disable(context.Context, catalog.AuthenticatedCaller, string, string) (contracts.CatalogEntry, error) {
	return service.entry, service.err
}
func (service *fakeCatalogService) Search(context.Context, contracts.SearchAgentsQuery) (catalog.SearchResult, error) {
	return service.searchResult, service.err
}

func TestDevelopmentStaticAuthenticatorUsesBearerDigestOnly(t *testing.T) {
	token := "local-secret-token"
	digest := sha256.Sum256([]byte(token))
	authenticator, err := NewDevelopmentStaticAuthenticator([]config.StaticPrincipal{{
		ID: "owner-a", TokenSHA256: hex.EncodeToString(digest[:]),
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v3/agents", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("x-caller-id", "forged-owner")
	caller, err := authenticator.Authenticate(request)
	if err != nil {
		t.Fatalf("authenticate valid token: %v", err)
	}
	if caller.ID != "owner-a" || caller.AuthenticationKind != config.DevelopmentStaticAuthMode {
		t.Fatalf("caller = %#v", caller)
	}

	for _, authorization := range []string{"", "Bearer", "Bearer wrong", "Bearer " + token + " extra"} {
		request := httptest.NewRequest(http.MethodGet, "/v3/agents", nil)
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		if _, err := authenticator.Authenticate(request); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("authorization %q error = %v", authorization, err)
		}
	}
	lowercaseScheme := httptest.NewRequest(http.MethodGet, "/v3/agents", nil)
	lowercaseScheme.Header.Set("Authorization", "bearer "+token)
	if _, err := authenticator.Authenticate(lowercaseScheme); err != nil {
		t.Fatalf("case-insensitive Bearer scheme was rejected: %v", err)
	}
}

func TestHandlerAuthenticationErrorHasMatchingTrace(t *testing.T) {
	handler := newTestHandler(t, fakeAuthenticator{err: ErrUnauthenticated}, &fakeCatalogService{}, fakeReadiness{})
	request := httptest.NewRequest(http.MethodGet, "/v3/agents", nil)
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	var platformError contracts.PlatformError
	if err := json.Unmarshal(response.Body.Bytes(), &platformError); err != nil {
		t.Fatalf("decode Platform Error: %v", err)
	}
	if platformError.Code != contracts.ErrorCodeUnauthenticated || string(platformError.TraceID) != response.Header().Get(TraceHeader) {
		t.Fatalf("error/header correlation = %#v / %q", platformError, response.Header().Get(TraceHeader))
	}
}

func TestActiveNorthboundV3CatalogRoutesComposeWithWorkspaceRoutes(t *testing.T) {
	catalogHandler := newTestHandler(t, fakeAuthenticator{caller: catalog.AuthenticatedCaller{ID: "owner-a"}}, &fakeCatalogService{
		searchResult: catalog.SearchResult{Entries: []contracts.CatalogEntry{}},
	}, fakeReadiness{})
	workspaceHandler := newWorkspaceTestHandler(t, workspaceTestAuthenticator{caller: catalog.AuthenticatedCaller{ID: "owner-a"}}, &workspaceTestService{})
	mux := http.NewServeMux()
	catalogHandler.RegisterRoutes(mux)
	workspaceHandler.RegisterRoutes(mux)

	catalogRequest := httptest.NewRequest(http.MethodGet, "/v3/agents", nil)
	catalogResponse := httptest.NewRecorder()
	mux.ServeHTTP(catalogResponse, catalogRequest)
	if catalogResponse.Code != http.StatusOK {
		t.Fatalf("composed Catalog route status = %d, want 200", catalogResponse.Code)
	}
	legacyResponse := httptest.NewRecorder()
	mux.ServeHTTP(legacyResponse, httptest.NewRequest(http.MethodGet, "/v2/agents", nil))
	if legacyResponse.Code != http.StatusNotFound {
		t.Fatalf("historical Catalog route status = %d, want 404", legacyResponse.Code)
	}

	workspaceRequest := httptest.NewRequest(http.MethodPost, "/v3/workspaces", strings.NewReader(`{"workspaceId":"workspace-a"}`))
	workspaceResponse := httptest.NewRecorder()
	mux.ServeHTTP(workspaceResponse, workspaceRequest)
	if workspaceResponse.Code != http.StatusCreated {
		t.Fatalf("composed Workspace route status = %d, want 201", workspaceResponse.Code)
	}
}

func TestHandlerRegisterAndFixedDomainErrors(t *testing.T) {
	caller := catalog.AuthenticatedCaller{ID: "owner-a", AuthenticationKind: config.DevelopmentStaticAuthMode}
	service := &fakeCatalogService{entry: contracts.CatalogEntry{PublicationStatus: "draft", RegisteredAt: time.Now().UTC()}}
	handler := newTestHandler(t, fakeAuthenticator{caller: caller}, service, fakeReadiness{})
	request := httptest.NewRequest(http.MethodPost, "/v3/agents", bytes.NewBufferString(`{"card":{}}`))
	request.Header.Set("Content-Type", "application/json")
	response := newDeadlineRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusCreated || response.Header().Get(TraceHeader) == "" {
		t.Fatalf("register response = %d, trace %q", response.Code, response.Header().Get(TraceHeader))
	}
	if service.registerCaller != caller || string(service.registerBody) != `{"card":{}}` {
		t.Fatalf("register adaptation = caller %#v body %q", service.registerCaller, service.registerBody)
	}
	if len(response.deadlines) != 2 || response.deadlines[0].IsZero() || !response.deadlines[1].IsZero() {
		t.Fatalf("registration read deadlines = %v", response.deadlines)
	}

	tests := []struct {
		err    error
		status int
		code   contracts.PlatformErrorCode
	}{
		{catalog.ErrInvalid, 400, contracts.ErrorCodeValidationError},
		{catalog.ErrForbidden, 403, contracts.ErrorCodeForbidden},
		{catalog.ErrNotFound, 404, contracts.ErrorCodeNotFound},
		{catalog.ErrConflict, 409, contracts.ErrorCodeConflict},
		{catalog.ErrDependency, 503, contracts.ErrorCodeDependency},
	}
	for _, test := range tests {
		service.err = test.err
		request := httptest.NewRequest(http.MethodGet, "/v3/agents/agent-a/versions/1.0.0", nil)
		response := httptest.NewRecorder()
		handler.Routes().ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("%v status = %d, want %d", test.err, response.Code, test.status)
			continue
		}
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["code"] != string(test.code) || body["traceId"] != response.Header().Get(TraceHeader) {
			t.Errorf("%v response = %#v, trace %q", test.err, body, response.Header().Get(TraceHeader))
		}
	}
}

func TestHandlerRejectsInvalidMediaAndSearchParameters(t *testing.T) {
	caller := catalog.AuthenticatedCaller{ID: "owner-a"}
	service := &fakeCatalogService{searchResult: catalog.SearchResult{Entries: []contracts.CatalogEntry{}}}
	handler := newTestHandler(t, fakeAuthenticator{caller: caller}, service, fakeReadiness{})

	request := httptest.NewRequest(http.MethodPost, "/v3/agents", bytes.NewBufferString(`{"card":{}}`))
	request.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid media status = %d", response.Code)
	}

	for _, rawQuery := range []string{"unknown=value", "limit=0", "limit=abc", "query=a&query=b", "query=%ZZ"} {
		request := httptest.NewRequest(http.MethodGet, "/v3/agents?"+rawQuery, nil)
		response := httptest.NewRecorder()
		handler.Routes().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("query %q status = %d, want 400", rawQuery, response.Code)
		}
	}
}

func TestHandlerRejectsOversizedRegistrationBeforeCatalog(t *testing.T) {
	caller := catalog.AuthenticatedCaller{ID: "owner-a"}
	service := &fakeCatalogService{}
	handler := newTestHandler(t, fakeAuthenticator{caller: caller}, service, fakeReadiness{})
	body := io.LimitReader(repeatingReader{}, contracts.RegistrationMaximumBodyBytes+1)
	request := httptest.NewRequest(http.MethodPost, "/v3/agents", body)
	request.Header.Set("Content-Type", "application/json")
	response := newDeadlineRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized registration status = %d, want 400", response.Code)
	}
	if service.registerCalls != 0 {
		t.Fatalf("Catalog received %d oversized registrations", service.registerCalls)
	}
}

func TestHandlerFailsBeforeCatalogWhenBodyDeadlineCannotBeControlled(t *testing.T) {
	caller := catalog.AuthenticatedCaller{ID: "owner-a"}
	service := &fakeCatalogService{}
	handler := newTestHandler(t, fakeAuthenticator{caller: caller}, service, fakeReadiness{})
	request := httptest.NewRequest(http.MethodPost, "/v3/agents", bytes.NewBufferString(`{"card":{}}`))
	request.Header.Set("Content-Type", "application/json")
	unsupported := httptest.NewRecorder()
	handler.Routes().ServeHTTP(unsupported, request)
	if unsupported.Code != http.StatusInternalServerError {
		t.Fatalf("unsupported writer status = %d, want 500", unsupported.Code)
	}
	if service.registerCalls != 0 {
		t.Fatalf("unsupported writer reached Catalog %d times", service.registerCalls)
	}

	for _, failCall := range []int{0, 1} {
		service := &fakeCatalogService{}
		handler := newTestHandler(t, fakeAuthenticator{caller: caller}, service, fakeReadiness{})
		request := httptest.NewRequest(http.MethodPost, "/v3/agents", bytes.NewBufferString(`{"card":{}}`))
		request.Header.Set("Content-Type", "application/json")
		response := newDeadlineRecorder()
		response.failCall = failCall
		handler.Routes().ServeHTTP(response, request)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("deadline call %d failure status = %d, want 500", failCall, response.Code)
		}
		if service.registerCalls != 0 {
			t.Fatalf("deadline call %d failure reached Catalog", failCall)
		}
	}
}

func TestRegistrationBodyDeadlineStartsAfterHeadersAndClearsForConnectionReuse(t *testing.T) {
	caller := catalog.AuthenticatedCaller{ID: "owner-a"}
	service := &fakeCatalogService{entry: contracts.CatalogEntry{PublicationStatus: "draft", RegisteredAt: time.Now().UTC()}}
	handler := newTestHandler(t, fakeAuthenticator{caller: caller}, service, fakeReadiness{})
	handler.bodyReadTimeout = 100 * time.Millisecond

	server := httptest.NewUnstartedServer(handler.Routes())
	server.Config.ReadHeaderTimeout = 2 * time.Second
	server.Start()
	defer server.Close()

	body := []byte(`{"card":{}}`)
	connection := dialTestServer(t, server)
	defer func() {
		if err := connection.Close(); err != nil {
			t.Errorf("close test connection: %v", err)
		}
	}()
	writeRequestHeaders(t, connection, server.Listener.Addr().String(), len(body), false)
	time.Sleep(250 * time.Millisecond)
	if _, err := connection.Write(append([]byte("\r\n"), body...)); err != nil {
		t.Fatalf("finish delayed-header request: %v", err)
	}
	response := readSocketResponse(t, connection)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("delayed-header registration status = %d", response.StatusCode)
	}
	if err := response.Body.Close(); err != nil {
		t.Errorf("close registration response body: %v", err)
	}

	time.Sleep(250 * time.Millisecond)
	if _, err := fmt.Fprintf(connection, "GET /livez HTTP/1.1\r\nHost: %s\r\n\r\n", server.Listener.Addr().String()); err != nil {
		t.Fatalf("reuse connection after registration: %v", err)
	}
	reused := readSocketResponse(t, connection)
	if reused.StatusCode != http.StatusNoContent {
		t.Fatalf("connection reuse status = %d", reused.StatusCode)
	}
	if err := reused.Body.Close(); err != nil {
		t.Errorf("close reused response body: %v", err)
	}

	partial := dialTestServer(t, server)
	defer func() {
		if err := partial.Close(); err != nil {
			t.Errorf("close partial test connection: %v", err)
		}
	}()
	writeRequestHeaders(t, partial, server.Listener.Addr().String(), len(body), true)
	if _, err := partial.Write(body[:4]); err != nil {
		t.Fatalf("write partial registration body: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	timedOut := readSocketResponse(t, partial)
	if timedOut.StatusCode != http.StatusBadRequest {
		t.Fatalf("partial registration status = %d, want 400", timedOut.StatusCode)
	}
	if err := timedOut.Body.Close(); err != nil {
		t.Errorf("close timed-out response body: %v", err)
	}
	if service.registerCalls != 1 {
		t.Fatalf("Catalog registrations = %d, want only completed body", service.registerCalls)
	}
}

func TestReadinessFailureIsExplicit(t *testing.T) {
	handler := newTestHandler(t, fakeAuthenticator{}, &fakeCatalogService{}, fakeReadiness{err: errors.New("database unavailable")})
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d", response.Code)
	}
}

func TestTraceGeneratorFailsAtInitialization(t *testing.T) {
	if _, err := newTraceGenerator(errorReader{}); err == nil {
		t.Fatal("failed entropy source was accepted")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type repeatingReader struct{}

func (repeatingReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 'x'
	}
	return len(buffer), nil
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
	failCall  int
}

func newDeadlineRecorder() *deadlineRecorder {
	return &deadlineRecorder{ResponseRecorder: httptest.NewRecorder(), failCall: -1}
}

func (recorder *deadlineRecorder) SetReadDeadline(deadline time.Time) error {
	call := len(recorder.deadlines)
	recorder.deadlines = append(recorder.deadlines, deadline)
	if call == recorder.failCall {
		return errors.New("injected read deadline failure")
	}
	return nil
}

func dialTestServer(t *testing.T, server *httptest.Server) net.Conn {
	t.Helper()
	connection, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("dial test server: %v", err)
	}
	if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		if closeErr := connection.Close(); closeErr != nil {
			t.Errorf("close test connection after deadline failure: %v", closeErr)
		}
		t.Fatalf("set test connection deadline: %v", err)
	}
	return connection
}

func writeRequestHeaders(t *testing.T, connection net.Conn, host string, bodyLength int, complete bool) {
	t.Helper()
	ending := ""
	if complete {
		ending = "\r\n"
	}
	if _, err := fmt.Fprintf(connection, "POST /v3/agents HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\n%s", host, bodyLength, ending); err != nil {
		t.Fatalf("write registration headers: %v", err)
	}
}

func readSocketResponse(t *testing.T, connection net.Conn) *http.Response {
	t.Helper()
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatalf("read socket response: %v", err)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Errorf("close failed response body: %v", closeErr)
		}
		t.Fatalf("read socket response body: %v", err)
	}
	return response
}

func newTestHandler(t *testing.T, authenticator Authenticator, service CatalogService, readiness ReadinessChecker) *Handler {
	t.Helper()
	traces, err := newTraceGenerator(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(authenticator, service, readiness, traces, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
