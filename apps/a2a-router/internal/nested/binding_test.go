package nested

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
)

func testTokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func TestNewAgentBindingValidation(t *testing.T) {
	validDigest := testTokenDigest("test-token")

	tests := []struct {
		name       string
		principals []AgentPrincipal
		wantErr    bool
	}{
		{
			"valid single principal",
			[]AgentPrincipal{{AgentID: "agent01", TokenSHA256: validDigest}},
			false,
		},
		{
			"empty principals",
			[]AgentPrincipal{},
			true,
		},
		{
			"nil principals",
			nil,
			true,
		},
		{
			"invalid agent id",
			[]AgentPrincipal{{AgentID: "agent 01", TokenSHA256: validDigest}},
			true,
		},
		{
			"empty agent id",
			[]AgentPrincipal{{AgentID: "", TokenSHA256: validDigest}},
			true,
		},
		{
			"invalid token digest",
			[]AgentPrincipal{{AgentID: "agent01", TokenSHA256: "not-hex"}},
			true,
		},
		{
			"short token digest",
			[]AgentPrincipal{{AgentID: "agent01", TokenSHA256: "abcd"}},
			true,
		},
		{
			"uppercase token digest",
			[]AgentPrincipal{{AgentID: "agent01", TokenSHA256: strings.ToUpper(validDigest)}},
			true,
		},
		{
			"duplicate agent id",
			[]AgentPrincipal{
				{AgentID: "agent01", TokenSHA256: validDigest},
				{AgentID: "agent01", TokenSHA256: testTokenDigest("other-token")},
			},
			true,
		},
		{
			"duplicate token digest",
			[]AgentPrincipal{
				{AgentID: "agent01", TokenSHA256: validDigest},
				{AgentID: "agent02", TokenSHA256: validDigest},
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAgentBinding(tt.principals)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAgentBinding() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAgentBindingAuthenticate(t *testing.T) {
	token := "my-secret-token"
	digest := testTokenDigest(token)
	binding, err := NewAgentBinding([]AgentPrincipal{{AgentID: "agent01", TokenSHA256: digest}})
	if err != nil {
		t.Fatalf("NewAgentBinding() error = %v", err)
	}

	tests := []struct {
		name          string
		authHeader    string
		headerCount   int
		wantAgentID   string
		wantErr       bool
		wantForbidden bool
	}{
		{
			"valid token",
			"Bearer " + token,
			1,
			"agent01",
			false,
			false,
		},
		{
			"missing header",
			"",
			0,
			"",
			true,
			false,
		},
		{
			"empty header",
			"",
			1,
			"",
			true,
			false,
		},
		{
			"no bearer prefix",
			token,
			1,
			"",
			true,
			false,
		},
		{
			"empty bearer token",
			"Bearer ",
			1,
			"",
			true,
			false,
		},
		{
			"whitespace token",
			"Bearer  token ",
			1,
			"",
			true,
			false,
		},
		{
			"unknown token",
			"Bearer unknown-token",
			1,
			"",
			true,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/agent/v1/invocations", nil)
			if tt.headerCount > 0 {
				req.Header.Set("Authorization", tt.authHeader)
			}
			agentID, err := binding.Authenticate(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Authenticate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if agentID != tt.wantAgentID {
				t.Errorf("Authenticate() agentID = %v, want %v", agentID, tt.wantAgentID)
			}
			if tt.wantForbidden && err != ErrForbidden {
				t.Errorf("Authenticate() expected ErrForbidden, got %v", err)
			}
		})
	}
}

func TestAgentBindingMultiplePrincipals(t *testing.T) {
	token1 := "token-agent-1"
	token2 := "token-agent-2"
	binding, err := NewAgentBinding([]AgentPrincipal{
		{AgentID: "agent01", TokenSHA256: testTokenDigest(token1)},
		{AgentID: "agent02", TokenSHA256: testTokenDigest(token2)},
	})
	if err != nil {
		t.Fatalf("NewAgentBinding() error = %v", err)
	}

	req1 := httptest.NewRequest("POST", "/agent/v1/invocations", nil)
	req1.Header.Set("Authorization", "Bearer "+token1)
	agentID1, err := binding.Authenticate(req1)
	if err != nil || agentID1 != "agent01" {
		t.Errorf("expected agent01, got %s, err %v", agentID1, err)
	}

	req2 := httptest.NewRequest("POST", "/agent/v1/invocations", nil)
	req2.Header.Set("Authorization", "Bearer "+token2)
	agentID2, err := binding.Authenticate(req2)
	if err != nil || agentID2 != "agent02" {
		t.Errorf("expected agent02, got %s, err %v", agentID2, err)
	}
}
