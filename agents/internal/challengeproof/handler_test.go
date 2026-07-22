package challengeproof

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewHandlerRequiresExplicitValidDirectory(t *testing.T) {
	agent := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for _, test := range []struct {
		name   string
		value  string
		exists bool
	}{
		{name: "missing"},
		{name: "empty", exists: true},
		{name: "whitespace", value: " " + t.TempDir(), exists: true},
		{name: "relative", value: "challenge-proofs", exists: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := NewHandler(agent, func(name string) (string, bool) {
				if name != DirectoryEnvironment {
					t.Fatalf("lookup name = %q", name)
				}
				return test.value, test.exists
			})
			if err == nil || result != nil {
				t.Fatalf("invalid directory was accepted: handler=%#v error=%v", result, err)
			}
		})
	}
}

func TestHandlerServesExactProofAndKeepsAgentBoundary(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "proofs")
	agentCalls := 0
	agent := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		agentCalls++
		writer.WriteHeader(http.StatusNoContent)
	})
	application, err := NewHandler(agent, func(name string) (string, bool) {
		if name != DirectoryEnvironment {
			t.Fatalf("lookup name = %q", name)
		}
		return directory, true
	})
	if err != nil {
		t.Fatal(err)
	}
	proof := "proof"
	if err := os.WriteFile(filepath.Join(directory, "challenge-1"), []byte(proof), 0o600); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, challengePathPrefix+"challenge-1", nil)
	response := httptest.NewRecorder()
	application.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != proof || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("proof response status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response = httptest.NewRecorder()
	application.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || agentCalls != 1 {
		t.Fatalf("agent response status=%d calls=%d", response.Code, agentCalls)
	}
}

func TestHandlerRejectsMissingInvalidAndNonGETProofRequests(t *testing.T) {
	directory := t.TempDir()
	application, err := NewHandler(http.NotFoundHandler(), func(string) (string, bool) { return directory, true })
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "challenge-invalid"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		method string
		path   string
		status int
	}{
		{name: "missing", method: http.MethodGet, path: challengePathPrefix + "challenge-missing", status: http.StatusNotFound},
		{name: "invalid proof", method: http.MethodGet, path: challengePathPrefix + "challenge-invalid", status: http.StatusInternalServerError},
		{name: "invalid id", method: http.MethodGet, path: challengePathPrefix + "bad/id", status: http.StatusNotFound},
		{name: "method", method: http.MethodPost, path: challengePathPrefix + "challenge-invalid", status: http.StatusMethodNotAllowed},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()
			application.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%q", response.Code, test.status, response.Body.String())
			}
		})
	}
}
