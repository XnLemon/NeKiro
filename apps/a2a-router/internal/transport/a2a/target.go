package a2a

import (
	"errors"
	"math"
	"math/big"
	"net/url"

	"github.com/Nene7ko/NeKiro/contracts"
)

type Target struct {
	AgentID        string
	Version        string
	Capability     string
	Endpoint       string
	Protocol       string
	Transport      string
	AuthType       string
	MaxInputBytes  int64
	MaxOutputBytes int64
}

func NewTarget(resolved contracts.ResolveAgentResponse, capability string) (Target, error) {
	if capability == "" {
		return Target{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("dispatch capability is required"))
	}
	card := resolved.Card
	if card.AgentID == "" || card.Version == "" {
		return Target{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("resolved Agent identity is required"))
	}
	if card.Protocol.Type != "a2a" || card.Protocol.Version != contracts.A2AProtocolVersion || card.Protocol.Transport != "JSONRPC" {
		return Target{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("resolved Agent A2A profile is unsupported"))
	}
	if card.Protocol.Endpoint == "" {
		return Target{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("resolved Agent endpoint is required"))
	}
	endpoint, err := url.Parse(card.Protocol.Endpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" || endpoint.User != nil {
		return Target{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("resolved Agent endpoint is invalid"))
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return Target{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("resolved Agent endpoint scheme is unsupported"))
	}
	if card.Authentication.Type != "none" {
		return Target{}, classify(contracts.ErrorCodeAgentAuthUnsupported, errors.New("resolved Agent authentication is unsupported"))
	}
	if !declaresCapability(card, capability) {
		return Target{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("resolved Agent capability is missing"))
	}
	maxInputBytes, err := parseCardLimit(card.Limits.MaxInputBytes.String())
	if err != nil {
		return Target{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("resolved Agent input limit is invalid"))
	}
	maxOutputBytes, err := parseCardLimit(card.Limits.MaxOutputBytes.String())
	if err != nil {
		return Target{}, classify(contracts.ErrorCodeA2AProtocol, errors.New("resolved Agent output limit is invalid"))
	}
	return Target{
		AgentID: card.AgentID, Version: card.Version, Capability: capability,
		Endpoint: card.Protocol.Endpoint, Protocol: card.Protocol.Type,
		Transport: card.Protocol.Transport, AuthType: card.Authentication.Type,
		MaxInputBytes: maxInputBytes, MaxOutputBytes: maxOutputBytes,
	}, nil
}

func parseCardLimit(value string) (int64, error) {
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok || parsed.Sign() <= 0 {
		return 0, errors.New("Agent Card limit must be a positive base-10 integer")
	}
	if parsed.BitLen() > 63 {
		return math.MaxInt64, nil
	}
	return parsed.Int64(), nil
}

func declaresCapability(card contracts.AgentCard, capability string) bool {
	for _, skill := range card.Skills {
		if skill.ID == capability {
			return true
		}
	}
	return false
}
