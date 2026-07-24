package routerauth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

var verifierTime = time.Unix(1784700030, 0)

func testKey() ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
}

func testVerifierConfig() Config {
	privateKey := testKey()
	return Config{Issuer: "https://a2a-router.nekiro.test", Audience: "http://runtime-b:8092", KeyID: "router-key-1", PublicKey: privateKey.Public().(ed25519.PublicKey)}
}

func validClaims(jwtID string) map[string]any {
	return map[string]any{
		"iss": "https://a2a-router.nekiro.test", "aud": []string{"http://runtime-b:8092"}, "exp": verifierTime.Unix() + 30, "iat": verifierTime.Unix(), "jti": jwtID,
		"workspaceId": "workspace-a", "agentId": "runtime-b", "agentVersion": "1.0.0", "releaseId": "release-b", "cardDigest": strings.Repeat("b", 64),
		"capability": "runtime.echo", "invocationId": "inv_0123456789abcdef0123456789abcdef", "rootTaskId": "task-a", "traceId": "trace-a",
	}
}

func signedRequest(t *testing.T, privateKey ed25519.PrivateKey, headerJSON, claimsJSON []byte) *http.Request {
	t.Helper()
	headerSegment := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsSegment := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerSegment + "." + claimsSegment
	signature := ed25519.Sign(privateKey, []byte(signingInput))
	credential := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
	request := httptest.NewRequest(http.MethodPost, "http://runtime-b:8092/", nil)
	request.Header.Set("Authorization", "Bearer "+credential)
	return request
}

func validSignedRequest(t *testing.T, jwtID string) *http.Request {
	t.Helper()
	headerJSON, _ := json.Marshal(map[string]any{"alg": "EdDSA", "typ": contracts.RouterAgentCredentialType, "kid": "router-key-1"})
	claims := validClaims(jwtID)
	claimsJSON, _ := json.Marshal(claims)
	request := signedRequest(t, testKey(), headerJSON, claimsJSON)
	context := contracts.RouterInvocationCredentialContextV1{
		WorkspaceID: claims["workspaceId"].(string), AgentID: claims["agentId"].(string), AgentVersion: claims["agentVersion"].(string), ReleaseID: claims["releaseId"].(string),
		CardDigest: claims["cardDigest"].(string), Capability: claims["capability"].(string), InvocationID: claims["invocationId"].(string), RootTaskID: claims["rootTaskId"].(string), TraceID: contracts.TraceID(claims["traceId"].(string)),
	}
	for name, value := range contracts.RouterAgentContextHeadersV1(context) {
		request.Header.Set(name, value)
	}
	return request
}

func TestVerifierAcceptsExactCredentialAndContext(t *testing.T) {
	verifier, err := NewVerifier(testVerifierConfig(), func() time.Time { return verifierTime })
	if err != nil {
		t.Fatal(err)
	}
	request := validSignedRequest(t, "rtj_exact")
	claims, err := verifier.Verify(request)
	if err != nil {
		t.Fatal(err)
	}
	if claims.AgentID != "runtime-b" || claims.JWTID != "rtj_exact" || claims.TraceID != "trace-a" {
		t.Fatalf("claims = %#v", claims)
	}
}

func TestVerifierRejectsInvalidCredentialMatrix(t *testing.T) {
	validHeader := `{"alg":"EdDSA","typ":"nekiro-router+jwt","kid":"router-key-1"}`
	baseClaims, _ := json.Marshal(validClaims("rtj_matrix"))
	tests := []struct {
		name       string
		headerJSON string
		claimsJSON string
		key        ed25519.PrivateKey
		mutate     func(*http.Request)
		status     int
	}{
		{name: "unsupported algorithm", headerJSON: strings.Replace(validHeader, "EdDSA", "HS256", 1), claimsJSON: string(baseClaims), key: testKey(), status: http.StatusUnauthorized},
		{name: "wrong type", headerJSON: strings.Replace(validHeader, "nekiro-router+jwt", "JWT", 1), claimsJSON: string(baseClaims), key: testKey(), status: http.StatusUnauthorized},
		{name: "unknown header", headerJSON: strings.TrimSuffix(validHeader, "}") + `,"extra":"x"}`, claimsJSON: string(baseClaims), key: testKey(), status: http.StatusUnauthorized},
		{name: "duplicate header", headerJSON: strings.Replace(validHeader, `"alg":"EdDSA"`, `"alg":"EdDSA","alg":"EdDSA"`, 1), claimsJSON: string(baseClaims), key: testKey(), status: http.StatusUnauthorized},
		{name: "wrong key id", headerJSON: strings.Replace(validHeader, "router-key-1", "router-key-2", 1), claimsJSON: string(baseClaims), key: testKey(), status: http.StatusUnauthorized},
		{name: "forged signature", headerJSON: validHeader, claimsJSON: string(baseClaims), key: ed25519.NewKeyFromSeed([]byte(strings.Repeat("x", ed25519.SeedSize))), status: http.StatusUnauthorized},
		{name: "unknown claim", headerJSON: validHeader, claimsJSON: strings.TrimSuffix(string(baseClaims), "}") + `,"extra":"x"}`, key: testKey(), status: http.StatusUnauthorized},
		{name: "duplicate claim", headerJSON: validHeader, claimsJSON: strings.Replace(string(baseClaims), `"iss":"https://a2a-router.nekiro.test"`, `"iss":"https://a2a-router.nekiro.test","iss":"https://a2a-router.nekiro.test"`, 1), key: testKey(), status: http.StatusUnauthorized},
		{name: "missing claim", headerJSON: validHeader, claimsJSON: strings.Replace(string(baseClaims), `"traceId":"trace-a",`, "", 1), key: testKey(), status: http.StatusUnauthorized},
		{name: "wrong issuer", headerJSON: validHeader, claimsJSON: strings.Replace(string(baseClaims), "https://a2a-router.nekiro.test", "https://other-router.nekiro.test", 1), key: testKey(), status: http.StatusUnauthorized},
		{name: "wrong audience", headerJSON: validHeader, claimsJSON: strings.Replace(string(baseClaims), "http://runtime-b:8092", "http://runtime-a:8091", 1), key: testKey(), status: http.StatusForbidden},
		{name: "expired", headerJSON: validHeader, claimsJSON: strings.Replace(string(baseClaims), `"exp":1784700060`, `"exp":1784700030`, 1), key: testKey(), status: http.StatusUnauthorized},
		{name: "future issued", headerJSON: validHeader, claimsJSON: strings.Replace(string(baseClaims), `"iat":1784700030`, `"iat":1784700031`, 1), key: testKey(), status: http.StatusUnauthorized},
		{name: "excessive lifetime", headerJSON: validHeader, claimsJSON: strings.Replace(string(baseClaims), `"exp":1784700060`, `"exp":1784700331`, 1), key: testKey(), status: http.StatusUnauthorized},
		{name: "fractional expiry", headerJSON: validHeader, claimsJSON: strings.Replace(string(baseClaims), `"exp":1784700060`, `"exp":1784700060.5`, 1), key: testKey(), status: http.StatusUnauthorized},
		{name: "context mismatch", headerJSON: validHeader, claimsJSON: string(baseClaims), key: testKey(), mutate: func(request *http.Request) {
			request.Header.Set(contracts.RouterAgentCapabilityHeader, "runtime.other")
		}, status: http.StatusForbidden},
		{name: "duplicate context header", headerJSON: validHeader, claimsJSON: string(baseClaims), key: testKey(), mutate: func(request *http.Request) {
			request.Header.Add(contracts.RouterAgentCapabilityHeader, "runtime.echo")
		}, status: http.StatusForbidden},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier, err := NewVerifier(testVerifierConfig(), func() time.Time { return verifierTime })
			if err != nil {
				t.Fatal(err)
			}
			request := signedRequest(t, test.key, []byte(test.headerJSON), []byte(test.claimsJSON))
			context := contracts.RouterInvocationCredentialContextV1{WorkspaceID: "workspace-a", AgentID: "runtime-b", AgentVersion: "1.0.0", ReleaseID: "release-b", CardDigest: strings.Repeat("b", 64), Capability: "runtime.echo", InvocationID: "inv_0123456789abcdef0123456789abcdef", RootTaskID: "task-a", TraceID: "trace-a"}
			for name, value := range contracts.RouterAgentContextHeadersV1(context) {
				request.Header.Set(name, value)
			}
			if test.mutate != nil {
				test.mutate(request)
			}
			_, err = verifier.Verify(request)
			var failure *verificationError
			if err == nil || !errors.As(err, &failure) || failure.status != test.status {
				t.Fatalf("case %d error = %#v, want status %d", index, err, test.status)
			}
		})
	}
}

func TestVerifierRejectsAuthorizationHeaderAmbiguity(t *testing.T) {
	cases := []struct {
		name   string
		values []string
	}{
		{name: "missing", values: nil},
		{name: "empty bearer", values: []string{"Bearer "}},
		{name: "non bearer", values: []string{"Basic abc"}},
		{name: "multiple values", values: []string{"Bearer one", "Bearer two"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			verifier, err := NewVerifier(testVerifierConfig(), func() time.Time { return verifierTime })
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "http://runtime-b:8092/", nil)
			for _, value := range test.values {
				request.Header.Add(contracts.RouterAgentAuthorizationHeader, value)
			}
			_, err = verifier.Verify(request)
			var failure *verificationError
			if err == nil || !errors.As(err, &failure) || failure.status != http.StatusUnauthorized {
				t.Fatalf("error = %#v, want 401", err)
			}
		})
	}
}

func TestVerifierRejectsPresentButEmptyParentClaim(t *testing.T) {
	headerJSON := []byte(`{"alg":"EdDSA","typ":"nekiro-router+jwt","kid":"router-key-1"}`)
	claims := validClaims("rtj_empty_parent")
	claims["parentInvocationId"] = ""
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(testVerifierConfig(), func() time.Time { return verifierTime })
	if err != nil {
		t.Fatal(err)
	}
	request := signedRequest(t, testKey(), headerJSON, claimsJSON)
	for name, value := range contracts.RouterAgentContextHeadersV1(contracts.RouterInvocationCredentialContextV1{
		WorkspaceID: "workspace-a", AgentID: "runtime-b", AgentVersion: "1.0.0", ReleaseID: "release-b", CardDigest: strings.Repeat("b", 64), Capability: "runtime.echo", InvocationID: "inv_0123456789abcdef0123456789abcdef", RootTaskID: "task-a", TraceID: "trace-a",
	}) {
		request.Header.Set(name, value)
	}
	_, err = verifier.Verify(request)
	var failure *verificationError
	if err == nil || !errors.As(err, &failure) || failure.status != http.StatusUnauthorized {
		t.Fatalf("error = %#v, want 401", err)
	}
}

func TestVerifierRejectsCredentialThatExpiresBetweenValidationAndReplay(t *testing.T) {
	headerJSON := []byte(`{"alg":"EdDSA","typ":"nekiro-router+jwt","kid":"router-key-1"}`)
	claims := validClaims("rtj_clock_boundary")
	claims["exp"] = verifierTime.Unix() + 1
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	times := []time.Time{verifierTime, time.Unix(verifierTime.Unix()+1, 0)}
	var calls int
	verifier, err := NewVerifier(testVerifierConfig(), func() time.Time {
		value := times[calls]
		calls++
		return value
	})
	if err != nil {
		t.Fatal(err)
	}
	request := signedRequest(t, testKey(), headerJSON, claimsJSON)
	for name, value := range contracts.RouterAgentContextHeadersV1(contracts.RouterInvocationCredentialContextV1{
		WorkspaceID: "workspace-a", AgentID: "runtime-b", AgentVersion: "1.0.0", ReleaseID: "release-b", CardDigest: strings.Repeat("b", 64), Capability: "runtime.echo", InvocationID: "inv_0123456789abcdef0123456789abcdef", RootTaskID: "task-a", TraceID: "trace-a",
	}) {
		request.Header.Set(name, value)
	}
	_, err = verifier.Verify(request)
	var failure *verificationError
	if err == nil || !errors.As(err, &failure) || failure.status != http.StatusUnauthorized {
		t.Fatalf("error = %#v, want 401", err)
	}
}

func TestVerifierRejectsMalformedCompactSerialization(t *testing.T) {
	verifier, _ := NewVerifier(testVerifierConfig(), func() time.Time { return verifierTime })
	for _, credential := range []string{"", "one.two", "one.two.three.four", "=.two.three", "a.b.c=", "e30.e30._x"} {
		request := httptest.NewRequest(http.MethodPost, "http://runtime-b:8092", nil)
		request.Header.Set("Authorization", "Bearer "+credential)
		if _, err := verifier.Verify(request); err == nil {
			t.Errorf("credential %q was accepted", credential)
		}
	}
}
