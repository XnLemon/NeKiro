package clientsdk

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

func validTestConfig(httpClient *http.Client, origin string) Config {
	return Config{
		HTTPClient:            httpClient,
		GatewayOrigin:         origin,
		WorkspaceID:           "workspace-a",
		ApplicationCredential: "application-secret",
		RequestLimitBytes:     4096,
		ResponseLimitBytes:    4096,
		StreamEventLimitBytes: 4096,
	}
}

func TestNewClientRequiresExplicitHTTPClientAndAcceptsItsNilTransport(t *testing.T) {
	config := validTestConfig(nil, "https://gateway.example")
	if _, err := NewClient(config); err == nil {
		t.Fatal("nil HTTP client was accepted")
	}
	original := &http.Client{}
	config.HTTPClient = original
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	if client.httpClient == original || client.httpClient.Transport != nil || original.Transport != nil {
		t.Fatal("documented nil Transport behavior or client clone was changed")
	}
}

func TestNewClientRejectsNonCanonicalGatewayOriginsWithoutNormalization(t *testing.T) {
	invalid := []string{
		"", " ", " https://gateway.example", "https://gateway.example ",
		"HTTP://gateway.example", "https://Gateway.example", "ftp://gateway.example",
		"https://", "gateway.example", "//gateway.example", "https://user:pass@gateway.example",
		"https://gateway.example/", "https://gateway.example/v4", "https://gateway.example?x=1",
		"https://gateway.example?", "https://gateway.example#fragment", "https://gateway.example#",
		"https://gateway.example:443", "http://gateway.example:80", "https://gateway.example:0",
		"https://gateway.example:", "https://gateway.example:08443", "https://gateway.example:65536",
		"https://gateway.example\n", "https://例子.example",
		"https://[fe80::1%25eth0]", "https://gateway.example/%2F",
	}
	for _, origin := range invalid {
		t.Run(origin, func(t *testing.T) {
			if _, err := NewClient(validTestConfig(&http.Client{}, origin)); err == nil {
				t.Fatalf("invalid Gateway origin %q was accepted", origin)
			}
		})
	}
	for _, origin := range []string{"https://gateway.example", "http://127.0.0.1:8080", "https://[::1]:8443"} {
		t.Run("valid "+origin, func(t *testing.T) {
			client, err := NewClient(validTestConfig(&http.Client{}, origin))
			if err != nil || client.gatewayOrigin != origin {
				t.Fatalf("origin=%q client=%v error=%v", origin, client, err)
			}
		})
	}
}

func TestNewClientRejectsInvalidWorkspaceCredentialAndLimits(t *testing.T) {
	base := validTestConfig(&http.Client{}, "https://gateway.example")
	workspaceValues := []string{"", " workspace-a", "workspace-a ", "-workspace", "workspace a", strings.Repeat("a", 129)}
	for _, value := range workspaceValues {
		config := base
		config.WorkspaceID = value
		if _, err := NewClient(config); err == nil {
			t.Fatalf("invalid Workspace ID %q was accepted", value)
		}
	}
	credentialValues := []string{"", " ", " secret", "secret ", "sec ret", "sec\tret", "sec\nret", "sec\x7fret", "密钥"}
	for index, value := range credentialValues {
		config := base
		config.ApplicationCredential = value
		if _, err := NewClient(config); err == nil {
			t.Fatalf("invalid credential case %d was accepted", index)
		}
	}
	for _, mutate := range []func(*Config){
		func(config *Config) { config.RequestLimitBytes = 0 },
		func(config *Config) { config.RequestLimitBytes = contracts.RuntimeByteLimitMaximum + 1 },
		func(config *Config) { config.ResponseLimitBytes = 0 },
		func(config *Config) { config.ResponseLimitBytes = contracts.RuntimeByteLimitMaximum + 1 },
		func(config *Config) { config.StreamEventLimitBytes = 0 },
		func(config *Config) { config.StreamEventLimitBytes = contracts.RuntimeByteLimitMaximum + 1 },
	} {
		config := base
		mutate(&config)
		if _, err := NewClient(config); err == nil {
			t.Fatalf("invalid limits were accepted: %#v", config)
		}
	}
	for _, limit := range []int64{contracts.RuntimeByteLimitMinimum, contracts.RuntimeByteLimitMaximum} {
		config := base
		config.RequestLimitBytes, config.ResponseLimitBytes, config.StreamEventLimitBytes = limit, limit, limit
		if _, err := NewClient(config); err != nil {
			t.Fatalf("boundary limit %d rejected: %v", limit, err)
		}
	}
}

func TestNewClientClonesCallerClientAndRejectsRedirects(t *testing.T) {
	sentinel := errors.New("caller redirect policy")
	original := &http.Client{
		Timeout:   3 * time.Second,
		Transport: &http.Transport{},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return sentinel
		},
	}
	client, err := NewClient(validTestConfig(original, "https://gateway.example"))
	if err != nil {
		t.Fatal(err)
	}
	if client.httpClient == original || client.httpClient.Timeout != original.Timeout || client.httpClient.Transport != original.Transport {
		t.Fatal("HTTP client was not cloned exactly")
	}
	if !errors.Is(original.CheckRedirect(nil, nil), sentinel) {
		t.Fatal("caller redirect policy was mutated")
	}
	if !errors.Is(client.httpClient.CheckRedirect(nil, nil), http.ErrUseLastResponse) {
		t.Fatal("SDK redirect policy does not reject forwarding")
	}
	original.Timeout = 9 * time.Second
	original.Transport = nil
	if client.httpClient.Timeout != 3*time.Second || client.httpClient.Transport == nil {
		t.Fatal("caller mutations changed the constructed Client")
	}
}

func TestInvokeDoesNotFollowGatewayRedirect(t *testing.T) {
	redirected := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirected" {
			redirected++
			_, _ = io.WriteString(writer, "must not be reached")
			return
		}
		http.Redirect(writer, request, "/redirected", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, err := NewClient(validTestConfig(server.Client(), server.URL))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Invoke(t.Context(), InvokeRequest{AgentID: "agent-a", Capability: "answer", Input: jsonObject(`{}`)})
	if err == nil || redirected != 0 {
		t.Fatalf("redirect error=%v redirected calls=%d", err, redirected)
	}
}

func jsonObject(value string) []byte { return []byte(value) }
