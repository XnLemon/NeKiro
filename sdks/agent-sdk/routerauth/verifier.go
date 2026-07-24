package routerauth

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

var headerMembers = map[string]struct{}{"alg": {}, "typ": {}, "kid": {}}

var claimMembers = map[string]struct{}{
	"iss": {}, "aud": {}, "exp": {}, "iat": {}, "jti": {}, "workspaceId": {}, "agentId": {}, "agentVersion": {},
	"releaseId": {}, "cardDigest": {}, "capability": {}, "invocationId": {}, "rootTaskId": {}, "parentInvocationId": {}, "traceId": {},
}

type Verifier struct {
	config Config
	now    func() time.Time
	replay *ReplayGuard
}

func NewVerifier(config Config, now func() time.Time) (*Verifier, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if now == nil {
		return nil, errors.New("router credential verification clock is required")
	}
	replay, err := NewReplayGuard(now)
	if err != nil {
		return nil, err
	}
	publicKey := make(ed25519.PublicKey, len(config.PublicKey))
	copy(publicKey, config.PublicKey)
	config.PublicKey = publicKey
	return &Verifier{config: config, now: now, replay: replay}, nil
}

func (verifier *Verifier) Verify(request *http.Request) (contracts.RouterInvocationCredentialClaimsV1, error) {
	credential, err := credentialFromHeaders(request.Header)
	if err != nil {
		return contracts.RouterInvocationCredentialClaimsV1{}, err
	}
	parts := strings.Split(credential, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(errors.New("compact credential must contain exactly three non-empty segments"))
	}
	headerJSON, err := decodeSegment(parts[0])
	if err != nil {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(err)
	}
	claimsJSON, err := decodeSegment(parts[1])
	if err != nil {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(err)
	}
	signature, err := decodeSegment(parts[2])
	if err != nil || len(signature) != ed25519.SignatureSize {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(errors.New("credential signature is malformed"))
	}
	header, err := decodeObject(headerJSON, headerMembers)
	if err != nil || len(header) != len(headerMembers) {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(errors.New("credential protected header is invalid"))
	}
	algorithm, algorithmOK := decodeJSONString(header["alg"])
	typeValue, typeOK := decodeJSONString(header["typ"])
	keyID, keyIDOK := decodeJSONString(header["kid"])
	if !algorithmOK || !typeOK || !keyIDOK || algorithm != contracts.RouterAgentCredentialAlgorithm || typeValue != contracts.RouterAgentCredentialType || keyID != verifier.config.KeyID {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(errors.New("credential protected header is unsupported"))
	}
	if !ed25519.Verify(verifier.config.PublicKey, []byte(parts[0]+"."+parts[1]), signature) {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(errors.New("credential signature is invalid"))
	}
	claims, parentPresent, err := decodeClaims(claimsJSON)
	if err != nil {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(err)
	}
	if parentPresent && claims.ParentInvocationID == "" {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(errors.New("credential claim parentInvocationId must not be empty"))
	}
	now := verifier.now().UTC().Truncate(time.Second)
	if err := contracts.ValidateRouterInvocationCredentialClaimsV1(claims, now); err != nil {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(err)
	}
	if claims.Issuer != verifier.config.Issuer {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(errors.New("credential issuer is invalid"))
	}
	if claims.Audience[0] != verifier.config.Audience {
		return contracts.RouterInvocationCredentialClaimsV1{}, forbidden(errors.New("credential audience is invalid"))
	}
	if err := validateContextHeaders(request.Header, claims, parentPresent); err != nil {
		return contracts.RouterInvocationCredentialClaimsV1{}, err
	}
	if !verifier.replay.Accept(claims.JWTID, claims.ExpiresAt) {
		return contracts.RouterInvocationCredentialClaimsV1{}, unauthenticated(errors.New("credential has already been used"))
	}
	return claims, nil
}

func decodeSegment(value string) ([]byte, error) {
	if strings.Contains(value, "=") {
		return nil, errors.New("compact credential segments must be unpadded")
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return nil, errors.New("compact credential segment is not strict Base64url")
	}
	return decoded, nil
}

func decodeObject(data []byte, allowed map[string]struct{}) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, errors.New("credential segment must be a JSON object")
	}
	members := make(map[string]json.RawMessage)
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := nameToken.(string)
		if !ok {
			return nil, errors.New("credential object member name is invalid")
		}
		if _, duplicate := members[name]; duplicate {
			return nil, errors.New("credential object contains a duplicate member")
		}
		if _, known := allowed[name]; !known {
			return nil, errors.New("credential object contains an unknown member")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		members[name] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, errors.New("credential JSON object is incomplete")
	}
	if err := requireEOF(decoder); err != nil {
		return nil, err
	}
	return members, nil
}

func decodeClaims(data []byte) (contracts.RouterInvocationCredentialClaimsV1, bool, error) {
	members, err := decodeObject(data, claimMembers)
	if err != nil {
		return contracts.RouterInvocationCredentialClaimsV1{}, false, err
	}
	required := []string{"iss", "aud", "exp", "iat", "jti", "workspaceId", "agentId", "agentVersion", "releaseId", "cardDigest", "capability", "invocationId", "rootTaskId", "traceId"}
	for _, name := range required {
		if _, exists := members[name]; !exists {
			return contracts.RouterInvocationCredentialClaimsV1{}, false, fmt.Errorf("credential claim %s is required", name)
		}
	}
	claims := contracts.RouterInvocationCredentialClaimsV1{}
	stringClaims := []struct {
		name        string
		destination *string
	}{
		{"iss", &claims.Issuer}, {"jti", &claims.JWTID}, {"workspaceId", &claims.WorkspaceID}, {"agentId", &claims.AgentID},
		{"agentVersion", &claims.AgentVersion}, {"releaseId", &claims.ReleaseID}, {"cardDigest", &claims.CardDigest},
		{"capability", &claims.Capability}, {"invocationId", &claims.InvocationID}, {"rootTaskId", &claims.RootTaskID},
	}
	for _, field := range stringClaims {
		value, ok := decodeJSONString(members[field.name])
		if !ok {
			return contracts.RouterInvocationCredentialClaimsV1{}, false, fmt.Errorf("credential claim %s must be a string", field.name)
		}
		*field.destination = value
	}
	traceID, ok := decodeJSONString(members["traceId"])
	if !ok {
		return contracts.RouterInvocationCredentialClaimsV1{}, false, errors.New("credential claim traceId must be a string")
	}
	claims.TraceID = contracts.TraceID(traceID)
	if err := decodeExactJSON(members["aud"], &claims.Audience); err != nil || claims.Audience == nil {
		return contracts.RouterInvocationCredentialClaimsV1{}, false, errors.New("credential claim aud must be an array of strings")
	}
	if err := decodeExactJSON(members["exp"], &claims.ExpiresAt); err != nil {
		return contracts.RouterInvocationCredentialClaimsV1{}, false, errors.New("credential claim exp must be an integer")
	}
	if err := decodeExactJSON(members["iat"], &claims.IssuedAt); err != nil {
		return contracts.RouterInvocationCredentialClaimsV1{}, false, errors.New("credential claim iat must be an integer")
	}
	parent, parentPresent := members["parentInvocationId"]
	if parentPresent {
		value, ok := decodeJSONString(parent)
		if !ok {
			return contracts.RouterInvocationCredentialClaimsV1{}, false, errors.New("credential claim parentInvocationId must be a string")
		}
		claims.ParentInvocationID = value
	}
	return claims, parentPresent, nil
}

func decodeJSONString(data json.RawMessage) (string, bool) {
	var value string
	if err := decodeExactJSON(data, &value); err != nil {
		return "", false
	}
	return value, true
}

func decodeExactJSON(data json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return requireEOF(decoder)
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("credential JSON contains trailing data")
}
