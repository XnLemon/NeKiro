package contracts

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTrustedPublicationOpenAPIAndSchemaMappings(t *testing.T) {
	document := loadOpenAPIDocument(t, filepath.Join("openapi", "trusted-publication.v1.yaml"))
	if document.Info.Version != "1.0.0" {
		t.Fatalf("trusted publication contract version=%q", document.Info.Version)
	}
	now := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	binding := EndpointBindingResponse{BindingID: "binding-1", ProviderID: "provider-1", AgentID: "agent-1", AgentCardVersion: "1.0.0-beta.1+build.7", Endpoint: "https://agent.example/a2a", VerificationMethod: "http_well_known", VerificationStatus: "verified", VerificationEvidenceDigest: &digest, CreatedAt: now, UpdatedAt: now, VerifiedAt: &now}
	challenge := VerificationChallengeResponse{ChallengeID: "challenge-1", BindingID: "binding-1", ChallengeURL: "https://agent.example/.well-known/nekiro/challenges/challenge-1", Proof: "proof", ExpiresAt: now}
	validateOpenAPIValue(t, document.Paths.Find("/v4/providers/{providerId}/agents/{agentId}/endpoint-bindings").Post.Responses.Status(201).Value.Content["application/json"].Schema, binding)
	validateOpenAPIValue(t, document.Paths.Find("/v4/providers/{providerId}/agents/{agentId}/endpoint-bindings").Post.RequestBody.Value.Content["application/json"].Schema, map[string]any{"endpoint": "https://agent.example/a2a", "method": "http_well_known", "version": "1.0.0-beta.1+build.7"})
	validateOpenAPIValue(t, document.Paths.Find("/v4/providers/{providerId}/endpoint-bindings/{bindingId}/challenges").Post.Responses.Status(201).Value.Content["application/json"].Schema, challenge)
	release := AgentReleaseResponse{ReleaseID: "release-1", ProviderID: "provider-1", AgentID: "agent-1", AgentCardVersion: "1.0.0-beta.1+build.7", CardDigest: digest, EndpointBindingID: "binding-1", EndpointOrigin: "https://agent.example", EndpointPath: "/a2a", VerificationMethod: "http_well_known", VerificationEvidenceDigest: &digest, State: ReleaseStatePublished, CreatedAt: now, UpdatedAt: now, VerifiedAt: &now, PublishedAt: &now}
	validateOpenAPIValue(t, document.Paths.Find("/v4/providers/{providerId}/agents/{agentId}/releases").Post.RequestBody.Value.Content["application/json"].Schema, CreateAgentReleaseRequest{Version: "1.0.0-beta.1+build.7", EndpointBindingID: "binding-1"})
	assertOpenAPIValueRejected(t, document.Paths.Find("/v4/providers/{providerId}/agents/{agentId}/releases").Post.RequestBody.Value.Content["application/json"].Schema, CreateAgentReleaseRequest{Version: "not-semver", EndpointBindingID: "binding-1"})
	assertOpenAPIValueRejected(t, document.Paths.Find("/v4/providers/{providerId}/agents/{agentId}/releases").Post.RequestBody.Value.Content["application/json"].Schema, CreateAgentReleaseRequest{Version: "1.0.0", EndpointBindingID: "bad binding"})
	assertOpenAPIValueRejected(t, document.Components.Parameters["releaseId"].Value.Schema, "bad release")
	validateOpenAPIValue(t, document.Paths.Find("/v4/providers/{providerId}/agents/{agentId}/releases").Post.Responses.Status(201).Value.Content["application/json"].Schema, release)
	for _, path := range []string{"/v4/releases/{releaseId}/verify", "/v4/releases/{releaseId}/publish", "/v4/releases/{releaseId}/suspend", "/v4/releases/{releaseId}/revoke"} {
		validateOpenAPIValue(t, document.Paths.Find(path).Post.Responses.Status(200).Value.Content["application/json"].Schema, release)
	}
	publicError, err := NewTrustedPublicationError(TrustedErrorRedirectNotAllowed, "trace-trusted-publication")
	if err != nil {
		t.Fatal(err)
	}
	validateOpenAPIValue(t, document.Components.Schemas["TrustedPublicationError"], publicError)
}
