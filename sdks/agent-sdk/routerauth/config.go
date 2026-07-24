package routerauth

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/Nene7ko/NeKiro/contracts"
)

const (
	IssuerEnvironment    = "NEKIRO_AGENT_ROUTER_ISSUER"
	AudienceEnvironment  = "NEKIRO_AGENT_ROUTER_AUDIENCE"
	KeyIDEnvironment     = "NEKIRO_AGENT_ROUTER_KEY_ID"
	PublicKeyEnvironment = "NEKIRO_AGENT_ROUTER_PUBLIC_KEY_BASE64URL"
)

type Config struct {
	Issuer    string
	Audience  string
	KeyID     string
	PublicKey ed25519.PublicKey
}

func LoadConfig(lookup func(string) (string, bool)) (Config, error) {
	issuer, err := requiredValue(lookup, IssuerEnvironment, false)
	if err != nil {
		return Config{}, err
	}
	if err := contracts.ValidateRouterAgentIssuer(issuer); err != nil {
		return Config{}, fmt.Errorf("%s is invalid: %w", IssuerEnvironment, err)
	}
	audience, err := requiredValue(lookup, AudienceEnvironment, false)
	if err != nil {
		return Config{}, err
	}
	if err := contracts.ValidateRouterAgentAudience(audience); err != nil {
		return Config{}, fmt.Errorf("%s is invalid: %w", AudienceEnvironment, err)
	}
	keyID, err := requiredValue(lookup, KeyIDEnvironment, false)
	if err != nil {
		return Config{}, err
	}
	if err := contracts.ValidateRouterAgentKeyID(keyID); err != nil {
		return Config{}, fmt.Errorf("%s is invalid: %w", KeyIDEnvironment, err)
	}
	encodedKey, err := requiredValue(lookup, PublicKeyEnvironment, true)
	if err != nil {
		return Config{}, err
	}
	publicKey, err := decodePublicKey(encodedKey)
	if err != nil {
		return Config{}, fmt.Errorf("%s is invalid", PublicKeyEnvironment)
	}
	return Config{Issuer: issuer, Audience: audience, KeyID: keyID, PublicKey: publicKey}, nil
}

func (config Config) Validate() error {
	if err := contracts.ValidateRouterAgentIssuer(config.Issuer); err != nil {
		return fmt.Errorf("router issuer is invalid: %w", err)
	}
	if err := contracts.ValidateRouterAgentAudience(config.Audience); err != nil {
		return fmt.Errorf("router audience is invalid: %w", err)
	}
	if err := contracts.ValidateRouterAgentKeyID(config.KeyID); err != nil {
		return fmt.Errorf("router key ID is invalid: %w", err)
	}
	if len(config.PublicKey) != ed25519.PublicKeySize {
		return errors.New("router public key is invalid")
	}
	return nil
}

func requiredValue(lookup func(string) (string, bool), name string, secret bool) (string, error) {
	value, exists := lookup(name)
	if !exists {
		return "", fmt.Errorf("%s is required", name)
	}
	if value == "" {
		return "", fmt.Errorf("%s must not be empty", name)
	}
	if !secret && value != strings.TrimSpace(value) {
		return "", fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	return value, nil
}

func decodePublicKey(value string) (ed25519.PublicKey, error) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("public key must be 32 raw bytes encoded as strict unpadded Base64url")
	}
	return ed25519.PublicKey(decoded), nil
}
