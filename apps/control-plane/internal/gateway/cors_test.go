package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSAllowsConfiguredPublicOriginAndPreflightWithoutAuth(t *testing.T) {
	handler := CORS([]string{"http://localhost:3000"}, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusTeapot)
	}))
	request := httptest.NewRequest(http.MethodOptions, "/v4/workspaces/ws-1/invocations", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("preflight response = %d %#v", response.Code, response.Header())
	}
	if response.Header().Get("Access-Control-Allow-Headers") != CORSAllowedHeaders || response.Header().Get("Access-Control-Expose-Headers") != TraceHeader {
		t.Fatalf("preflight headers = %#v", response.Header())
	}
}

func TestCORSDoesNotGrantUnknownOrInternalOrigin(t *testing.T) {
	for _, path := range []string{"/v4/agents", "/internal/v4/invocations", "/healthz"} {
		t.Run(path, func(t *testing.T) {
			handler := CORS([]string{"http://localhost:3000"}, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("Origin", "http://evil.example")
			if strings.HasPrefix(path, "/internal/") {
				request.Header.Set("Origin", "http://localhost:3000")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatalf("unexpected CORS grant for %s: %#v", path, response.Header())
			}
		})
	}
}
