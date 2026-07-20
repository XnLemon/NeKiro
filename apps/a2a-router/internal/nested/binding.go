// Package nested provides the Router-owned agent binding authenticator and
// child context derivation helpers for nested Agent invocations. It does not
// contain Agent Runtime, model, tool, workflow, or memory behavior.
package nested

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
)

var (
	// ErrUnauthenticated indicates a missing or invalid Agent credential.
	ErrUnauthenticated = errors.New("nested: agent authentication is required")
	// ErrForbidden indicates the authenticated Agent does not match the parent target.
	ErrForbidden = errors.New("nested: agent is forbidden")
)

// AgentPrincipal maps one opaque bearer credential digest to one exact Agent ID.
type AgentPrincipal struct {
	AgentID     string
	TokenSHA256 string
}

// AgentBinding authenticates Agent-facing nested invocation requests by
// matching one opaque bearer credential to one exact Agent ID. Duplicate
// Agent IDs or token digests are rejected at construction.
type AgentBinding struct {
	digests map[string]string
}

// NewAgentBinding creates an Agent binding authenticator from explicit
// principals. It rejects empty, duplicate, or invalid bindings.
func NewAgentBinding(principals []AgentPrincipal) (*AgentBinding, error) {
	if len(principals) == 0 {
		return nil, errors.New("nested: at least one agent principal is required")
	}
	digests := make(map[string]string, len(principals))
	ids := make(map[string]struct{}, len(principals))
	for _, principal := range principals {
		if !validIdentifier(principal.AgentID) {
			return nil, errors.New("nested: agent principal id is invalid")
		}
		decoded, err := hex.DecodeString(principal.TokenSHA256)
		if err != nil || len(decoded) != sha256.Size || principal.TokenSHA256 != strings.ToLower(principal.TokenSHA256) {
			return nil, errors.New("nested: agent principal tokenSha256 is invalid")
		}
		if _, exists := ids[principal.AgentID]; exists {
			return nil, errors.New("nested: agent principal id is duplicated")
		}
		if _, exists := digests[principal.TokenSHA256]; exists {
			return nil, errors.New("nested: agent principal tokenSha256 is duplicated")
		}
		ids[principal.AgentID] = struct{}{}
		digests[principal.TokenSHA256] = principal.AgentID
	}
	return &AgentBinding{digests: digests}, nil
}

// Authenticate extracts and validates the bearer credential from the request.
// It returns the bound Agent ID on success. Missing, malformed, unknown, or
// duplicate Authorization headers fail without defaults.
func (binding *AgentBinding) Authenticate(request *http.Request) (string, error) {
	values := request.Header.Values("Authorization")
	if len(values) != 1 {
		return "", ErrUnauthenticated
	}
	value := values[0]
	if value == "" {
		return "", ErrUnauthenticated
	}
	token, ok := strings.CutPrefix(value, "Bearer ")
	if !ok || token == "" || strings.TrimSpace(token) != token {
		return "", ErrUnauthenticated
	}
	sum := sha256.Sum256([]byte(token))
	digest := hex.EncodeToString(sum[:])
	for configured, agentID := range binding.digests {
		if subtle.ConstantTimeCompare([]byte(configured), []byte(digest)) == 1 {
			return agentID, nil
		}
	}
	return "", ErrForbidden
}

func validIdentifier(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for index, character := range []byte(value) {
		if character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == ':' || character == '-' {
			if index > 0 || character != '.' && character != '_' && character != ':' && character != '-' {
				continue
			}
		}
		return false
	}
	return true
}
