package routerauth

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

func validAgentAuthEnvironment() map[string]string {
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	return map[string]string{
		IssuerEnvironment:    "https://a2a-router.nekiro.test",
		AudienceEnvironment:  "http://runtime-b:8092",
		KeyIDEnvironment:     "router-key-1",
		PublicKeyEnvironment: base64.RawURLEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)),
	}
}

func agentAuthLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, exists := values[name]
		return value, exists
	}
}

func TestLoadConfigRequiresExactAgentVerificationValues(t *testing.T) {
	config, err := LoadConfig(agentAuthLookup(validAgentAuthEnvironment()))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.PublicKey) != ed25519.PublicKeySize || config.Audience != "http://runtime-b:8092" {
		t.Fatalf("config = %#v", config)
	}
	for name := range validAgentAuthEnvironment() {
		environment := validAgentAuthEnvironment()
		delete(environment, name)
		if _, err := LoadConfig(agentAuthLookup(environment)); err == nil {
			t.Errorf("missing %s was accepted", name)
		}
	}
}

func TestLoadConfigRejectsMalformedAgentVerificationValues(t *testing.T) {
	validKey := validAgentAuthEnvironment()[PublicKeyEnvironment]
	tests := []struct {
		key   string
		value string
	}{
		{IssuerEnvironment, "http://a2a-router.nekiro.test"},
		{AudienceEnvironment, "http://runtime-b:8092/path"},
		{AudienceEnvironment, "http://runtime-b:8092/"},
		{KeyIDEnvironment, "router key"},
		{PublicKeyEnvironment, validKey + "="},
		{PublicKeyEnvironment, " " + validKey},
		{PublicKeyEnvironment, base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize-1))},
	}
	for _, test := range tests {
		environment := validAgentAuthEnvironment()
		environment[test.key] = test.value
		if _, err := LoadConfig(agentAuthLookup(environment)); err == nil {
			t.Errorf("invalid %s=%q was accepted", test.key, test.value)
		}
	}
}
