package routerauth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

func TestMiddlewareRejectsBeforeRuntimeAndUsesStableResponses(t *testing.T) {
	middleware, err := NewMiddleware(testVerifierConfig(), func() time.Time { return verifierTime })
	if err != nil {
		t.Fatal(err)
	}
	var executions atomic.Int64
	handler := middleware.Handler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		executions.Add(1)
		if claims, ok := ClaimsFromContext(request.Context()); !ok || claims.JWTID == "" {
			t.Error("verified claims missing from request context")
		}
		writer.WriteHeader(http.StatusNoContent)
	}))

	validResponse := httptest.NewRecorder()
	handler.ServeHTTP(validResponse, validSignedRequest(t, "rtj_middleware_valid"))
	if validResponse.Code != http.StatusNoContent || executions.Load() != 1 {
		t.Fatalf("valid response=%d executions=%d", validResponse.Code, executions.Load())
	}

	directResponse := httptest.NewRecorder()
	directRequest := httptest.NewRequest(http.MethodPost, "http://runtime-b:8092", nil)
	handler.ServeHTTP(directResponse, directRequest)
	if directResponse.Code != http.StatusUnauthorized || directResponse.Header().Get("WWW-Authenticate") != "Bearer" || directResponse.Header().Get("Cache-Control") != "no-store" || directResponse.Body.String() != `{"code":"UNAUTHENTICATED","message":"Authentication is required."}` || executions.Load() != 1 {
		t.Fatalf("direct response=%d body=%q executions=%d", directResponse.Code, directResponse.Body.String(), executions.Load())
	}

	forbiddenResponse := httptest.NewRecorder()
	forbiddenRequest := validSignedRequest(t, "rtj_middleware_forbidden")
	forbiddenRequest.Header.Del("x-nek-agent-release-id")
	handler.ServeHTTP(forbiddenResponse, forbiddenRequest)
	if forbiddenResponse.Code != http.StatusForbidden || forbiddenResponse.Header().Get("WWW-Authenticate") != "" || forbiddenResponse.Header().Get("Cache-Control") != "no-store" || forbiddenResponse.Body.String() != `{"code":"FORBIDDEN","message":"The managed invocation context is not allowed."}` || executions.Load() != 1 {
		t.Fatalf("forbidden response=%d body=%q executions=%d", forbiddenResponse.Code, forbiddenResponse.Body.String(), executions.Load())
	}
}

func TestMiddlewareRejectsEverySignedContextMismatchBeforeRuntime(t *testing.T) {
	headers := []string{
		contracts.RouterAgentWorkspaceHeader,
		contracts.RouterAgentTargetAgentHeader,
		contracts.RouterAgentCardVersionHeader,
		contracts.RouterAgentReleaseHeader,
		contracts.RouterAgentCardDigestHeader,
		contracts.RouterAgentCapabilityHeader,
		contracts.RouterAgentInvocationHeader,
		contracts.RouterAgentRootTaskHeader,
		contracts.RouterAgentTraceHeader,
	}
	for index, name := range headers {
		t.Run(name, func(t *testing.T) {
			middleware, err := NewMiddleware(testVerifierConfig(), func() time.Time { return verifierTime })
			if err != nil {
				t.Fatal(err)
			}
			var executions atomic.Int64
			handler := middleware.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { executions.Add(1) }))
			request := validSignedRequest(t, fmt.Sprintf("rtj_context_%d", index))
			request.Header.Set(name, "mismatched-value")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden || executions.Load() != 0 {
				t.Fatalf("status=%d executions=%d", response.Code, executions.Load())
			}
		})
	}

	middleware, _ := NewMiddleware(testVerifierConfig(), func() time.Time { return verifierTime })
	var executions atomic.Int64
	handler := middleware.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { executions.Add(1) }))
	request := validSignedRequest(t, "rtj_unexpected_parent")
	request.Header.Set(contracts.RouterAgentParentInvocationHeader, "inv_unexpected_parent")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || executions.Load() != 0 {
		t.Fatalf("unexpected parent status=%d executions=%d", response.Code, executions.Load())
	}
}
