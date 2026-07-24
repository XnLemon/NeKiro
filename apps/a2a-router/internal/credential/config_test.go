package credential

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
)

func validCredentialEnvironment() map[string]string {
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	return map[string]string{
		IssuerEnvironment:     "https://a2a-router.nekiro.test",
		KeyIDEnvironment:      "router-key-1",
		PrivateKeyEnvironment: base64.RawURLEncoding.EncodeToString(privateKey),
		TTLEnvironment:        "30",
	}
}

func credentialLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, exists := values[name]
		return value, exists
	}
}

func TestLoadConfigRequiresExactRouterSigningValues(t *testing.T) {
	config, err := LoadConfig(credentialLookup(validCredentialEnvironment()))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if config.Issuer != "https://a2a-router.nekiro.test" || config.KeyID != "router-key-1" || len(config.PrivateKey) != ed25519.PrivateKeySize || config.TTL.Seconds() != 30 {
		t.Fatalf("config = %#v", config)
	}
	for name := range validCredentialEnvironment() {
		environment := validCredentialEnvironment()
		delete(environment, name)
		if _, err := LoadConfig(credentialLookup(environment)); err == nil {
			t.Errorf("missing %s was accepted", name)
		}
	}
}

func TestLoadConfigRejectsMalformedRouterSigningValues(t *testing.T) {
	validKey := validCredentialEnvironment()[PrivateKeyEnvironment]
	inconsistentKey, err := base64.RawURLEncoding.DecodeString(validKey)
	if err != nil {
		t.Fatal(err)
	}
	inconsistentKey[len(inconsistentKey)-1] ^= 1
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "blank issuer", key: IssuerEnvironment, value: " "},
		{name: "non-https issuer", key: IssuerEnvironment, value: "http://a2a-router.nekiro.test"},
		{name: "issuer path", key: IssuerEnvironment, value: "https://a2a-router.nekiro.test/path"},
		{name: "unsafe key id", key: KeyIDEnvironment, value: "router key"},
		{name: "private key whitespace", key: PrivateKeyEnvironment, value: " " + validKey},
		{name: "private key padded", key: PrivateKeyEnvironment, value: validKey + "="},
		{name: "private key wrong length", key: PrivateKeyEnvironment, value: base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize-1))},
		{name: "private key inconsistent public half", key: PrivateKeyEnvironment, value: base64.RawURLEncoding.EncodeToString(inconsistentKey)},
		{name: "zero ttl", key: TTLEnvironment, value: "0"},
		{name: "large ttl", key: TTLEnvironment, value: "301"},
		{name: "signed ttl", key: TTLEnvironment, value: "+30"},
		{name: "fractional ttl", key: TTLEnvironment, value: "30.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			environment := validCredentialEnvironment()
			environment[test.key] = test.value
			if _, err := LoadConfig(credentialLookup(environment)); err == nil {
				t.Fatalf("invalid %s was accepted", test.key)
			} else if strings.Contains(err.Error(), validKey) {
				t.Fatal("configuration error exposed private key")
			}
		})
	}
}
