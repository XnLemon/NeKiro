package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/auth"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/credential"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/nested"
	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	ListenAddress                  string
	RouterPrincipals               []auth.Principal
	AgentPrincipals                []nested.AgentPrincipal
	DatabaseURL                    string
	ControlPlaneResolveURL         string
	ControlPlaneVersionURL         string
	ControlPlaneServiceToken       string
	InternalRequestLimitBytes      int64
	AgentRequestLimitBytes         int64
	ControlPlaneResponseLimitBytes int64
	AgentResponseLimitBytes        int64
	A2AEventLimitBytes             int64
	SSEEventLimitBytes             int64
	ResolutionDeadline             time.Duration
	AgentDeadline                  time.Duration
	AgentCredential                credential.Config
}

type jsonFrame struct {
	object    bool
	expecting bool
	members   map[string]struct{}
}

func Load() (Config, error) {
	listen, err := requiredEnv("NEKIRO_ROUTER_LISTEN_ADDRESS")
	if err != nil {
		return Config{}, err
	}
	if err := validateListenAddress(listen); err != nil {
		return Config{}, fmt.Errorf("NEKIRO_ROUTER_LISTEN_ADDRESS is invalid: %w", err)
	}
	principalsJSON, err := requiredEnv("NEKIRO_ROUTER_SERVICE_PRINCIPALS_JSON")
	if err != nil {
		return Config{}, err
	}
	principals, err := decodePrincipals([]byte(principalsJSON))
	if err != nil {
		return Config{}, fmt.Errorf("NEKIRO_ROUTER_SERVICE_PRINCIPALS_JSON is invalid: %w", err)
	}
	agentPrincipalsJSON, err := requiredEnv("NEKIRO_ROUTER_AGENT_PRINCIPALS_JSON")
	if err != nil {
		return Config{}, err
	}
	agentPrincipals, err := decodeAgentPrincipals([]byte(agentPrincipalsJSON))
	if err != nil {
		return Config{}, fmt.Errorf("NEKIRO_ROUTER_AGENT_PRINCIPALS_JSON is invalid: %w", err)
	}
	resolveURL, err := requiredEnv("NEKIRO_CONTROL_PLANE_RESOLVE_URL")
	if err != nil {
		return Config{}, err
	}
	if err := validateResolveURL(resolveURL); err != nil {
		return Config{}, fmt.Errorf("NEKIRO_CONTROL_PLANE_RESOLVE_URL is invalid: %w", err)
	}
	versionURL, err := requiredEnv("NEKIRO_CONTROL_PLANE_VERSION_URL")
	if err != nil {
		return Config{}, err
	}
	if err := validateControlPlaneURL(versionURL, "/internal/v3/resolve-installed-version"); err != nil {
		return Config{}, fmt.Errorf("NEKIRO_CONTROL_PLANE_VERSION_URL is invalid: %w", err)
	}
	token, err := requiredEnv("NEKIRO_CONTROL_PLANE_SERVICE_TOKEN")
	if err != nil {
		return Config{}, err
	}
	if err := validateVisibleASCII("NEKIRO_CONTROL_PLANE_SERVICE_TOKEN", token); err != nil {
		return Config{}, err
	}
	databaseURL, err := LoadDatabaseURL()
	if err != nil {
		return Config{}, err
	}
	requestLimit, err := requiredInt64("NEKIRO_ROUTER_INTERNAL_REQUEST_LIMIT_BYTES", contracts.RuntimeByteLimitMinimum, contracts.RuntimeByteLimitMaximum)
	if err != nil {
		return Config{}, err
	}
	agentRequestLimit, err := requiredInt64("NEKIRO_ROUTER_AGENT_REQUEST_LIMIT_BYTES", contracts.RuntimeByteLimitMinimum, contracts.RuntimeByteLimitMaximum)
	if err != nil {
		return Config{}, err
	}
	responseLimit, err := requiredInt64("NEKIRO_ROUTER_CONTROL_PLANE_RESPONSE_LIMIT_BYTES", contracts.RuntimeByteLimitMinimum, contracts.RuntimeByteLimitMaximum)
	if err != nil {
		return Config{}, err
	}
	agentResponseLimit, err := requiredInt64("NEKIRO_ROUTER_AGENT_RESPONSE_LIMIT_BYTES", contracts.RuntimeByteLimitMinimum, contracts.RuntimeByteLimitMaximum)
	if err != nil {
		return Config{}, err
	}
	a2aEventLimit, err := requiredInt64("NEKIRO_ROUTER_A2A_EVENT_LIMIT_BYTES", contracts.RuntimeByteLimitMinimum, contracts.RuntimeByteLimitMaximum)
	if err != nil {
		return Config{}, err
	}
	sseEventLimit, err := requiredInt64("NEKIRO_ROUTER_SSE_EVENT_LIMIT_BYTES", contracts.RuntimeByteLimitMinimum, contracts.RuntimeByteLimitMaximum)
	if err != nil {
		return Config{}, err
	}
	deadlineMS, err := requiredInt64("NEKIRO_ROUTER_RESOLUTION_DEADLINE_MS", contracts.RuntimeDeadlineMinimumMS, contracts.RuntimeDeadlineMaximumMS)
	if err != nil {
		return Config{}, err
	}
	agentDeadlineMS, err := requiredInt64("NEKIRO_ROUTER_AGENT_DEADLINE_MS", contracts.RuntimeDeadlineMinimumMS, contracts.RuntimeDeadlineMaximumMS)
	if err != nil {
		return Config{}, err
	}
	agentCredential, err := credential.LoadConfig(os.LookupEnv)
	if err != nil {
		return Config{}, err
	}
	return Config{ListenAddress: listen, RouterPrincipals: principals, AgentPrincipals: agentPrincipals, DatabaseURL: databaseURL, ControlPlaneResolveURL: resolveURL, ControlPlaneVersionURL: versionURL, ControlPlaneServiceToken: token, InternalRequestLimitBytes: requestLimit, AgentRequestLimitBytes: agentRequestLimit, ControlPlaneResponseLimitBytes: responseLimit, AgentResponseLimitBytes: agentResponseLimit, A2AEventLimitBytes: a2aEventLimit, SSEEventLimitBytes: sseEventLimit, ResolutionDeadline: time.Duration(deadlineMS) * time.Millisecond, AgentDeadline: time.Duration(agentDeadlineMS) * time.Millisecond, AgentCredential: agentCredential}, nil
}

// LoadDatabaseURL validates the database boundary shared by the serving and
// migration commands. The migration command must not require serving-only
// credentials or endpoint configuration.
func LoadDatabaseURL() (string, error) {
	databaseURL, err := requiredEnv("NEKIRO_DATABASE_URL")
	if err != nil {
		return "", err
	}
	if err := validateDatabaseURL(databaseURL); err != nil {
		return "", fmt.Errorf("NEKIRO_DATABASE_URL is invalid: %w", err)
	}
	return databaseURL, nil
}

func requiredEnv(name string) (string, error) {
	value, exists := os.LookupEnv(name)
	if !exists {
		return "", fmt.Errorf("%s is required", name)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must not be blank", name)
	}
	if value != strings.TrimSpace(value) {
		return "", fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	return value, nil
}

func validateListenAddress(value string) error {
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return errors.New("must be a host and port")
	}
	if host == "" {
		return errors.New("host is required")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	return nil
}

func validateResolveURL(value string) error {
	return validateControlPlaneURL(value, "/internal/v2/resolve-agent")
}

func validateControlPlaneURL(value, requiredPath string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.ForceQuery || parsed.RawQuery != "" || strings.Contains(value, "#") || parsed.Fragment != "" || parsed.Path != requiredPath {
		return errors.New("must be an absolute HTTP(S) Control Plane resolve URL without userinfo, query, or fragment")
	}
	if portText := parsed.Port(); portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return errors.New("control plane resolve URL port must be an integer from 1 through 65535")
		}
	}
	return nil
}

func decodeAgentPrincipals(data []byte) ([]nested.AgentPrincipal, error) {
	if err := rejectDuplicateMembers(data); err != nil {
		return nil, err
	}
	var wire []struct {
		WorkspaceID string `json:"workspaceId"`
		AgentID     string `json:"agentId"`
		TokenSHA256 string `json:"tokenSha256"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return nil, err
	}
	if err := requireEOF(decoder); err != nil {
		return nil, err
	}
	principals := make([]nested.AgentPrincipal, len(wire))
	for index, principal := range wire {
		principals[index] = nested.AgentPrincipal{WorkspaceID: principal.WorkspaceID, AgentID: principal.AgentID, TokenSHA256: principal.TokenSHA256}
	}
	if _, err := nested.NewAgentBinding(principals); err != nil {
		return nil, err
	}
	return principals, nil
}

func validateDatabaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" || parsed.Port() == "" || parsed.Path == "" || parsed.Path == "/" || parsed.User == nil || parsed.User.Username() == "" {
		return errors.New("must be an absolute PostgreSQL URL with explicit user, host, port, and database")
	}
	password, hasPassword := parsed.User.Password()
	if !hasPassword || password == "" || parsed.Query().Get("sslmode") == "" {
		return errors.New("PostgreSQL URL must include a non-empty password and sslmode")
	}
	if _, err := pgxpool.ParseConfig(value); err != nil {
		return errors.New("PostgreSQL URL is not accepted by pgxpool")
	}
	return nil
}

func validateVisibleASCII(name, value string) error {
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return fmt.Errorf("%s must contain only visible ASCII characters", name)
		}
	}
	return nil
}

func requiredInt64(name string, minimum, maximum int64) (int64, error) {
	value, err := requiredEnv(name)
	if err != nil {
		return 0, err
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, fmt.Errorf("%s must be an unsigned base-10 integer", name)
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return parsed, nil
}

func decodePrincipals(data []byte) ([]auth.Principal, error) {
	if err := rejectDuplicateMembers(data); err != nil {
		return nil, err
	}
	var wire []struct {
		ID          string `json:"id"`
		TokenSHA256 string `json:"tokenSha256"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return nil, err
	}
	if err := requireEOF(decoder); err != nil {
		return nil, err
	}
	principals := make([]auth.Principal, len(wire))
	for index, principal := range wire {
		principals[index] = auth.Principal{ID: principal.ID, TokenSHA256: principal.TokenSHA256}
	}
	if _, err := auth.NewStaticAuthenticator(principals); err != nil {
		return nil, err
	}
	return principals, nil
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}

func rejectDuplicateMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
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
				stack = stack[:len(stack)-1]
				markValueConsumed(stack)
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].expecting {
				current := &stack[len(stack)-1]
				if _, exists := current.members[value]; exists {
					return fmt.Errorf("duplicate member %q", value)
				}
				current.members[value] = struct{}{}
				current.expecting = false
			} else {
				markValueConsumed(stack)
			}
		default:
			markValueConsumed(stack)
		}
	}
}

func markValueConsumed(stack []jsonFrame) {
	if len(stack) > 0 && stack[len(stack)-1].object {
		stack[len(stack)-1].expecting = true
	}
}
