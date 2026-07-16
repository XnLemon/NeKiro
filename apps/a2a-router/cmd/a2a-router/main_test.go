package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/auth"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/config"
	"github.com/Nene7ko/NeKiro/contracts"
)

type failingDoer struct{}

func (failingDoer) Do(*http.Request) (*http.Response, error) {
	panic("readiness must not probe dependencies")
}

type ledgerAppenderStub struct{}

func (ledgerAppenderStub) Append(context.Context, contracts.InvocationEventV03) error { return nil }

func TestRunRequiresExplicitCommandAndMigrationDirection(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing command", want: "command is required: serve or migrate"},
		{name: "unknown command", args: []string{"status"}, want: `unknown command "status"`},
		{name: "migration direction", args: []string{"migrate"}, want: "migrate requires exactly one direction: up"},
		{name: "migration down", args: []string{"migrate", "down"}, want: "migrate requires exactly one direction: up"},
		{name: "serve arguments", args: []string{"serve", "extra"}, want: "serve accepts no arguments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := run(context.Background(), test.args, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want substring=%q", err, test.want)
			}
		})
	}
}

func TestNewHandlerAssemblesReadinessWithoutDependencyProbe(t *testing.T) {
	handler, err := newHandler(config.Config{
		ListenAddress:                  "127.0.0.1:9090",
		RouterPrincipals:               []auth.Principal{{ID: "router", TokenSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}},
		ControlPlaneResolveURL:         "https://control.internal/internal/v2/resolve-agent",
		ControlPlaneServiceToken:       "control-token",
		InternalRequestLimitBytes:      1024,
		ControlPlaneResponseLimitBytes: 2048,
		AgentResponseLimitBytes:        4096,
		A2AEventLimitBytes:             4096,
		ResolutionDeadline:             time.Second,
	}, failingDoer{}, &http.Client{}, ledgerAppenderStub{})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
}
