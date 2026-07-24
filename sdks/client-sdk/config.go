package clientsdk

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Nene7ko/NeKiro/contracts"
)

// Config binds one Client to one Gateway origin and Workspace authorization
// context. Every field is required; the SDK supplies no transport, identity,
// credential, or byte-limit default.
type Config struct {
	HTTPClient            *http.Client
	GatewayOrigin         string
	WorkspaceID           string
	ApplicationCredential string `json:"-"`
	RequestLimitBytes     int64
	ResponseLimitBytes    int64
	StreamEventLimitBytes int64
}

// Format prevents generic log formatting from exposing configuration fields,
// including the application credential.
func (Config) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "clientsdk.Config{ApplicationCredential:[REDACTED]}")
}

// Client invokes installed Agents through one configured Gateway and
// Workspace. A Client is immutable after construction and safe for concurrent
// independent calls.
type Client struct {
	httpClient            *http.Client
	gatewayOrigin         string
	workspaceID           string
	applicationCredential string
	requestLimitBytes     int64
	responseLimitBytes    int64
	streamEventLimitBytes int64
	results               *contracts.ResultContractValidator
	runtime               *contracts.RuntimeContractValidator
}

// Format prevents generic log formatting from exposing the Client's bound
// application credential or transport configuration.
func (Client) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "clientsdk.Client{ApplicationCredential:[REDACTED]}")
}

// NewClient validates and copies an explicit application configuration.
func NewClient(config Config) (*Client, error) {
	if config.HTTPClient == nil {
		return nil, errors.New("clientsdk: HTTP client is required")
	}
	origin, err := validateGatewayOrigin(config.GatewayOrigin)
	if err != nil {
		return nil, err
	}
	if !safeIdentifier(config.WorkspaceID) {
		return nil, errors.New("clientsdk: Workspace ID is invalid")
	}
	if !validApplicationCredential(config.ApplicationCredential) {
		return nil, errors.New("clientsdk: application credential is invalid")
	}
	limits := []struct {
		name  string
		value int64
	}{
		{name: "request", value: config.RequestLimitBytes},
		{name: "response", value: config.ResponseLimitBytes},
		{name: "stream event", value: config.StreamEventLimitBytes},
	}
	for _, limit := range limits {
		if limit.value < contracts.RuntimeByteLimitMinimum || limit.value > contracts.RuntimeByteLimitMaximum {
			return nil, fmt.Errorf("clientsdk: %s limit is invalid", limit.name)
		}
	}

	httpClient := *config.HTTPClient
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	results, err := contracts.NewResultContractValidator()
	if err != nil {
		return nil, fmt.Errorf("clientsdk: initialize result validator: %w", err)
	}
	runtime, err := contracts.NewRuntimeContractValidator()
	if err != nil {
		return nil, fmt.Errorf("clientsdk: initialize runtime validator: %w", err)
	}
	return &Client{
		httpClient:            &httpClient,
		gatewayOrigin:         origin,
		workspaceID:           config.WorkspaceID,
		applicationCredential: config.ApplicationCredential,
		requestLimitBytes:     config.RequestLimitBytes,
		responseLimitBytes:    config.ResponseLimitBytes,
		streamEventLimitBytes: config.StreamEventLimitBytes,
		results:               results,
		runtime:               runtime,
	}, nil
}

func validateGatewayOrigin(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return "", errors.New("clientsdk: Gateway origin is invalid")
	}
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e {
			return "", errors.New("clientsdk: Gateway origin is invalid")
		}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" || parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" || parsed.String() != value || parsed.Host != strings.ToLower(parsed.Host) || strings.Contains(parsed.Hostname(), "%") {
		return "", errors.New("clientsdk: Gateway origin is invalid")
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return "", errors.New("clientsdk: Gateway origin is invalid")
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || strconv.Itoa(number) != port || number < 1 || number > 65535 || parsed.Scheme == "http" && number == 80 || parsed.Scheme == "https" && number == 443 {
			return "", errors.New("clientsdk: Gateway origin is invalid")
		}
	}
	return value, nil
}

func validApplicationCredential(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}
