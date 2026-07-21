package runtimea

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Nene7ko/NeKiro/contracts"
)

const (
	ListenAddressEnvironment = "RUNTIME_A_LISTEN_ADDR"
	AgentIDEnvironment       = "RUNTIME_A_AGENT_ID"
	RouterEnvironment        = "RUNTIME_A_ROUTER_URL"
	RouterTokenEnvironment   = "RUNTIME_A_ROUTER_TOKEN"
	TargetAgentEnvironment   = "RUNTIME_A_TARGET_AGENT_ID"
	CapabilityEnvironment    = "RUNTIME_A_TARGET_CAPABILITY"
	ResponseLimitEnvironment = "RUNTIME_A_RESPONSE_LIMIT_BYTES"
	EventLimitEnvironment    = "RUNTIME_A_EVENT_LIMIT_BYTES"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// Config contains only explicit deployment settings for Runtime A. Required
// values have no defaults and credentials are never included in errors.
type Config struct {
	ListenAddress string
	AgentID       string
	RouterURL     string
	RouterToken   string
	TargetAgentID string
	Capability    string
	ResponseLimit int64
	EventLimit    int64
}

// LoadConfig reads and validates every required Runtime A setting.
func LoadConfig(lookup func(string) (string, bool)) (Config, error) {
	listenAddress, err := requiredValue(lookup, ListenAddressEnvironment)
	if err != nil {
		return Config{}, err
	}
	if err := validateListenAddress(listenAddress); err != nil {
		return Config{}, err
	}
	agentID, err := requiredIdentifier(lookup, AgentIDEnvironment)
	if err != nil {
		return Config{}, err
	}
	routerURL, err := requiredValue(lookup, RouterEnvironment)
	if err != nil {
		return Config{}, err
	}
	if err := validateRouterURL(routerURL); err != nil {
		return Config{}, err
	}
	routerToken, err := requiredValue(lookup, RouterTokenEnvironment)
	if err != nil {
		return Config{}, err
	}
	targetAgentID, err := requiredIdentifier(lookup, TargetAgentEnvironment)
	if err != nil {
		return Config{}, err
	}
	capability, err := requiredIdentifier(lookup, CapabilityEnvironment)
	if err != nil {
		return Config{}, err
	}
	responseLimit, err := requiredLimit(lookup, ResponseLimitEnvironment)
	if err != nil {
		return Config{}, err
	}
	eventLimit, err := requiredLimit(lookup, EventLimitEnvironment)
	if err != nil {
		return Config{}, err
	}
	config := Config{
		ListenAddress: listenAddress,
		AgentID:       agentID,
		RouterURL:     routerURL,
		RouterToken:   routerToken,
		TargetAgentID: targetAgentID,
		Capability:    capability,
		ResponseLimit: responseLimit,
		EventLimit:    eventLimit,
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate checks a Config value supplied by a caller that is not using
// LoadConfig. It applies the same required, exact, and range rules without
// inventing any missing value.
func (config Config) Validate() error {
	if err := validateRequiredValue(ListenAddressEnvironment, config.ListenAddress); err != nil {
		return err
	}
	if err := validateListenAddress(config.ListenAddress); err != nil {
		return err
	}
	if err := validateRequiredValue(AgentIDEnvironment, config.AgentID); err != nil {
		return err
	}
	if err := validateIdentifierValue(AgentIDEnvironment, config.AgentID); err != nil {
		return err
	}
	if err := validateRequiredValue(RouterEnvironment, config.RouterURL); err != nil {
		return err
	}
	if err := validateRouterURL(config.RouterURL); err != nil {
		return err
	}
	if err := validateRequiredValue(RouterTokenEnvironment, config.RouterToken); err != nil {
		return err
	}
	if err := validateRequiredValue(TargetAgentEnvironment, config.TargetAgentID); err != nil {
		return err
	}
	if err := validateIdentifierValue(TargetAgentEnvironment, config.TargetAgentID); err != nil {
		return err
	}
	if err := validateRequiredValue(CapabilityEnvironment, config.Capability); err != nil {
		return err
	}
	if err := validateIdentifierValue(CapabilityEnvironment, config.Capability); err != nil {
		return err
	}
	if config.ResponseLimit < contracts.RuntimeByteLimitMinimum || config.ResponseLimit > contracts.RuntimeByteLimitMaximum {
		return fmt.Errorf("%s must be an integer from %d through %d", ResponseLimitEnvironment, contracts.RuntimeByteLimitMinimum, contracts.RuntimeByteLimitMaximum)
	}
	if config.EventLimit < contracts.RuntimeByteLimitMinimum || config.EventLimit > contracts.RuntimeByteLimitMaximum {
		return fmt.Errorf("%s must be an integer from %d through %d", EventLimitEnvironment, contracts.RuntimeByteLimitMinimum, contracts.RuntimeByteLimitMaximum)
	}
	return nil
}

func requiredValue(lookup func(string) (string, bool), name string) (string, error) {
	value, exists := lookup(name)
	if !exists {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, validateRequiredValue(name, value)
}

func validateRequiredValue(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s must be non-empty", name)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	return nil
}

func requiredIdentifier(lookup func(string) (string, bool), name string) (string, error) {
	value, err := requiredValue(lookup, name)
	if err != nil {
		return "", err
	}
	return value, validateIdentifierValue(name, value)
}

func validateIdentifierValue(name, value string) error {
	if !identifierPattern.MatchString(value) {
		return fmt.Errorf("%s must be a safe identifier", name)
	}
	return nil
}

func requiredLimit(lookup func(string) (string, bool), name string) (int64, error) {
	value, err := requiredValue(lookup, name)
	if err != nil {
		return 0, err
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, fmt.Errorf("%s must be an unsigned base-10 integer", name)
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < contracts.RuntimeByteLimitMinimum || parsed > contracts.RuntimeByteLimitMaximum {
		return 0, fmt.Errorf("%s must be an integer from %d through %d", name, contracts.RuntimeByteLimitMinimum, contracts.RuntimeByteLimitMaximum)
	}
	return parsed, nil
}

func validateListenAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return fmt.Errorf("%s must be a host:port TCP address", ListenAddressEnvironment)
	}
	port, err := parsePort(portText)
	if err != nil {
		return fmt.Errorf("%s: %w", ListenAddressEnvironment, err)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s port must be an integer from 1 through 65535", ListenAddressEnvironment)
	}
	return nil
}

func parsePort(value string) (int64, error) {
	if value == "" {
		return 0, fmt.Errorf("port is required")
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, fmt.Errorf("port must be an unsigned base-10 integer")
		}
	}
	return strconv.ParseInt(value, 10, 64)
}

func validateRouterURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(value, "#") || parsed.Path != "" {
		return fmt.Errorf("%s must be an http or https origin URL without credentials, query, or fragment", RouterEnvironment)
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("%s must declare a host", RouterEnvironment)
	}
	if portText := parsed.Port(); portText != "" {
		port, err := parsePort(portText)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("%s port must be an integer from 1 through 65535", RouterEnvironment)
		}
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return fmt.Errorf("%s must use a valid host:port", RouterEnvironment)
	}
	return nil
}
