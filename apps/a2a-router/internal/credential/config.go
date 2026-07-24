package credential

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

const (
	IssuerEnvironment     = "NEKIRO_ROUTER_AGENT_CREDENTIAL_ISSUER"
	KeyIDEnvironment      = "NEKIRO_ROUTER_AGENT_CREDENTIAL_KEY_ID"
	PrivateKeyEnvironment = "NEKIRO_ROUTER_AGENT_CREDENTIAL_PRIVATE_KEY_BASE64URL"
	TTLEnvironment        = "NEKIRO_ROUTER_AGENT_CREDENTIAL_TTL_SECONDS"
)

type Config struct {
	Issuer     string
	KeyID      string
	PrivateKey ed25519.PrivateKey
	TTL        time.Duration
}

func LoadConfig(lookup func(string) (string, bool)) (Config, error) {
	issuer, err := requiredValue(lookup, IssuerEnvironment)
	if err != nil {
		return Config{}, err
	}
	if err := contracts.ValidateRouterAgentIssuer(issuer); err != nil {
		return Config{}, fmt.Errorf("%s is invalid: %w", IssuerEnvironment, err)
	}
	keyID, err := requiredValue(lookup, KeyIDEnvironment)
	if err != nil {
		return Config{}, err
	}
	if err := contracts.ValidateRouterAgentKeyID(keyID); err != nil {
		return Config{}, fmt.Errorf("%s is invalid: %w", KeyIDEnvironment, err)
	}
	encodedKey, err := requiredSecret(lookup, PrivateKeyEnvironment)
	if err != nil {
		return Config{}, err
	}
	privateKey, err := decodePrivateKey(encodedKey)
	if err != nil {
		return Config{}, fmt.Errorf("%s is invalid", PrivateKeyEnvironment)
	}
	ttlText, err := requiredValue(lookup, TTLEnvironment)
	if err != nil {
		return Config{}, err
	}
	ttlSeconds, err := parseTTL(ttlText)
	if err != nil {
		return Config{}, fmt.Errorf("%s is invalid: %w", TTLEnvironment, err)
	}
	return Config{Issuer: issuer, KeyID: keyID, PrivateKey: privateKey, TTL: time.Duration(ttlSeconds) * time.Second}, nil
}

func (config Config) Validate() error {
	if err := contracts.ValidateRouterAgentIssuer(config.Issuer); err != nil {
		return fmt.Errorf("router credential issuer is invalid: %w", err)
	}
	if err := contracts.ValidateRouterAgentKeyID(config.KeyID); err != nil {
		return fmt.Errorf("router credential key ID is invalid: %w", err)
	}
	if !validPrivateKey(config.PrivateKey) {
		return errors.New("router credential private key is invalid")
	}
	if config.TTL < time.Second || config.TTL > contracts.RouterAgentCredentialMaximumTTL || config.TTL%time.Second != 0 {
		return errors.New("router credential TTL is invalid")
	}
	return nil
}

func requiredValue(lookup func(string) (string, bool), name string) (string, error) {
	value, exists := lookup(name)
	if !exists {
		return "", fmt.Errorf("%s is required", name)
	}
	if value == "" {
		return "", fmt.Errorf("%s must not be empty", name)
	}
	if value != strings.TrimSpace(value) {
		return "", fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	return value, nil
}

func requiredSecret(lookup func(string) (string, bool), name string) (string, error) {
	value, exists := lookup(name)
	if !exists {
		return "", fmt.Errorf("%s is required", name)
	}
	if value == "" {
		return "", fmt.Errorf("%s must not be empty", name)
	}
	return value, nil
}

func decodePrivateKey(value string) (ed25519.PrivateKey, error) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || !validPrivateKey(decoded) {
		return nil, errors.New("private key must be 64 raw Ed25519 bytes encoded as strict unpadded Base64url")
	}
	return ed25519.PrivateKey(decoded), nil
}

func validPrivateKey(value []byte) bool {
	if len(value) != ed25519.PrivateKeySize {
		return false
	}
	canonical := ed25519.NewKeyFromSeed(value[:ed25519.SeedSize])
	return bytes.Equal(value, canonical)
}

func parseTTL(value string) (int64, error) {
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, errors.New("must be an unsigned base-10 integer")
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	maximum := int64(contracts.RouterAgentCredentialMaximumTTL / time.Second)
	if err != nil || parsed < 1 || parsed > maximum {
		return 0, fmt.Errorf("must be between 1 and %d", maximum)
	}
	return parsed, nil
}
