package a2a

import (
	"errors"
	"testing"

	"github.com/Nene7ko/NeKiro/contracts"
)

func TestNewTargetAcceptsExactA2ANoneAuthEndpoint(t *testing.T) {
	resolved := contracts.ResolveAgentResponse{Card: targetCard("http://127.0.0.1:4101/a2a", "none", "capability-a")}
	target, err := NewTarget(resolved, "capability-a")
	if err != nil {
		t.Fatalf("NewTarget = %v", err)
	}
	if target.AgentID != "agent-a" || target.Version != "1.0.0" || target.Endpoint != "http://127.0.0.1:4101/a2a" || target.AuthType != "none" {
		t.Fatalf("target = %#v", target)
	}
}

func TestNewTargetClampsLargeCardLimitsToTransportIntegerRange(t *testing.T) {
	card := targetCard("http://127.0.0.1:4101/a2a", "none", "capability-a")
	card.Limits.MaxInputBytes = "999999999999999999999999999999999999999999"
	card.Limits.MaxOutputBytes = "999999999999999999999999999999999999999999"
	target, err := NewTarget(contracts.ResolveAgentResponse{Card: card}, "capability-a")
	if err != nil {
		t.Fatalf("NewTarget = %v", err)
	}
	if target.MaxInputBytes <= 0 || target.MaxOutputBytes <= 0 {
		t.Fatalf("target limits = %#v", target)
	}
}

func TestNewTargetRejectsUnsupportedStates(t *testing.T) {
	tests := []struct {
		name string
		card contracts.AgentCard
		cap  string
		code contracts.PlatformErrorCode
	}{
		{name: "missing capability", card: targetCard("http://127.0.0.1:4101/a2a", "none", "capability-a"), cap: "", code: contracts.ErrorCodeA2AProtocol},
		{name: "unsupported scheme", card: targetCard("ftp://agent.example/a2a", "none", "capability-a"), cap: "capability-a", code: contracts.ErrorCodeA2AProtocol},
		{name: "userinfo", card: targetCard("http://user@example.test/a2a", "none", "capability-a"), cap: "capability-a", code: contracts.ErrorCodeA2AProtocol},
		{name: "unsupported auth", card: targetCard("http://127.0.0.1:4101/a2a", "bearer", "capability-a"), cap: "capability-a", code: contracts.ErrorCodeAgentAuthUnsupported},
		{name: "missing skill", card: targetCard("http://127.0.0.1:4101/a2a", "none", "capability-b"), cap: "capability-a", code: contracts.ErrorCodeA2AProtocol},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewTarget(contracts.ResolveAgentResponse{Card: test.card}, test.cap); err == nil {
				t.Fatal("NewTarget succeeded, want error")
			} else if code := errorCode(err); code != test.code {
				t.Fatalf("error code = %q, want %q", code, test.code)
			}
		})
	}
}

func errorCode(err error) contracts.PlatformErrorCode {
	var coded interface {
		PlatformErrorCode() contracts.PlatformErrorCode
	}
	if errors.As(err, &coded) {
		return coded.PlatformErrorCode()
	}
	return ""
}

func targetCard(endpoint, authType, capability string) contracts.AgentCard {
	return contracts.AgentCard{
		AgentID: "agent-a", Version: "1.0.0",
		Protocol:       contracts.AgentProtocol{Type: "a2a", Version: contracts.A2AProtocolVersion, Transport: "JSONRPC", Endpoint: endpoint},
		Authentication: contracts.AgentAuthentication{Type: authType},
		Skills:         []contracts.AgentSkill{{ID: capability}},
		Limits:         contracts.AgentLimits{MaxInputBytes: "4096", MaxOutputBytes: "4096"},
	}
}
