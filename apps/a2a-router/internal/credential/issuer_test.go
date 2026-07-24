package credential

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/golang-jwt/jwt/v5"
)

func TestIssuerCreatesFreshExactEd25519Credentials(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	now := time.Unix(1784700030, 987654321)
	random := bytes.NewReader(append(make([]byte, 16), bytes.Repeat([]byte{1}, 16)...))
	issuer, err := NewIssuer(Config{Issuer: "https://a2a-router.nekiro.test", KeyID: "router-key-1", PrivateKey: privateKey, TTL: 30 * time.Second}, func() time.Time { return now }, random)
	if err != nil {
		t.Fatal(err)
	}
	context := contracts.RouterInvocationCredentialContextV1{
		Audience: "http://runtime-b:8092", WorkspaceID: "workspace-a", AgentID: "runtime-b", AgentVersion: "1.0.0",
		ReleaseID: "release-b", CardDigest: strings.Repeat("b", 64), Capability: "runtime.echo", InvocationID: "inv_0123456789abcdef0123456789abcdef",
		RootTaskID: "task-a", ParentInvocationID: "inv_1123456789abcdef0123456789abcdef", TraceID: "trace-a",
	}
	first := parseIssuedCredential(t, issuer, privateKey.Public().(ed25519.PublicKey), context)
	second := parseIssuedCredential(t, issuer, privateKey.Public().(ed25519.PublicKey), context)
	if first["jti"] == second["jti"] {
		t.Fatal("two issued credentials reused jti")
	}
	if first["iat"] != float64(now.Unix()) || first["exp"] != float64(now.Unix()+30) {
		t.Fatalf("numeric dates = iat %#v exp %#v", first["iat"], first["exp"])
	}
	if first["parentInvocationId"] != context.ParentInvocationID || first["releaseId"] != context.ReleaseID || first["cardDigest"] != context.CardDigest {
		t.Fatalf("claims = %#v", first)
	}
	audience, ok := first["aud"].([]any)
	if !ok || len(audience) != 1 || audience[0] != context.Audience {
		t.Fatalf("audience claim = %#v", first["aud"])
	}
}

func parseIssuedCredential(t *testing.T, issuer *Issuer, publicKey ed25519.PublicKey, context contracts.RouterInvocationCredentialContextV1) map[string]any {
	t.Helper()
	serialized, err := issuer.Issue(context)
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.Parse(serialized, func(token *jwt.Token) (any, error) { return publicKey, nil }, jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithoutClaimsValidation())
	if err != nil || !token.Valid {
		t.Fatalf("parse issued credential: %v", err)
	}
	if len(token.Header) != 3 || token.Header["alg"] != "EdDSA" || token.Header["typ"] != contracts.RouterAgentCredentialType || token.Header["kid"] != "router-key-1" {
		t.Fatalf("header = %#v", token.Header)
	}
	parts := strings.Split(serialized, ".")
	claimsJSON, err := jwt.NewParser().DecodeSegment(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}
