package credential

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/golang-jwt/jwt/v5"
)

const jwtIDRandomBytes = 16

type Issuer struct {
	issuer     string
	keyID      string
	privateKey ed25519.PrivateKey
	ttl        time.Duration
	now        func() time.Time
	random     io.Reader
}

type signedClaims struct {
	jwt.RegisteredClaims
	WorkspaceID        string            `json:"workspaceId"`
	AgentID            string            `json:"agentId"`
	AgentVersion       string            `json:"agentVersion"`
	ReleaseID          string            `json:"releaseId"`
	CardDigest         string            `json:"cardDigest"`
	Capability         string            `json:"capability"`
	InvocationID       string            `json:"invocationId"`
	RootTaskID         string            `json:"rootTaskId"`
	ParentInvocationID string            `json:"parentInvocationId,omitempty"`
	TraceID            contracts.TraceID `json:"traceId"`
}

func NewIssuer(config Config, now func() time.Time, random io.Reader) (*Issuer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if now == nil {
		return nil, errors.New("router credential clock is required")
	}
	if random == nil {
		return nil, errors.New("router credential random source is required")
	}
	privateKey := make(ed25519.PrivateKey, len(config.PrivateKey))
	copy(privateKey, config.PrivateKey)
	return &Issuer{issuer: config.Issuer, keyID: config.KeyID, privateKey: privateKey, ttl: config.TTL, now: now, random: random}, nil
}

func (issuer *Issuer) Issue(context contracts.RouterInvocationCredentialContextV1) (string, error) {
	now := issuer.now().UTC().Truncate(time.Second)
	jwtID, err := issuer.newJWTID()
	if err != nil {
		return "", err
	}
	expiresAt := now.Add(issuer.ttl)
	claims := contracts.RouterInvocationCredentialClaimsV1{
		Issuer: issuer.issuer, Audience: []string{context.Audience}, ExpiresAt: expiresAt.Unix(), IssuedAt: now.Unix(), JWTID: jwtID,
		WorkspaceID: context.WorkspaceID, AgentID: context.AgentID, AgentVersion: context.AgentVersion, ReleaseID: context.ReleaseID,
		CardDigest: context.CardDigest, Capability: context.Capability, InvocationID: context.InvocationID, RootTaskID: context.RootTaskID,
		ParentInvocationID: context.ParentInvocationID, TraceID: context.TraceID,
	}
	if err := contracts.ValidateRouterInvocationCredentialClaimsV1(claims, now); err != nil {
		return "", fmt.Errorf("router credential context is invalid: %w", err)
	}
	wire := signedClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: claims.Issuer, Audience: jwt.ClaimStrings(claims.Audience), ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt: jwt.NewNumericDate(now), ID: claims.JWTID,
		},
		WorkspaceID: claims.WorkspaceID, AgentID: claims.AgentID, AgentVersion: claims.AgentVersion, ReleaseID: claims.ReleaseID,
		CardDigest: claims.CardDigest, Capability: claims.Capability, InvocationID: claims.InvocationID, RootTaskID: claims.RootTaskID,
		ParentInvocationID: claims.ParentInvocationID, TraceID: claims.TraceID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, wire)
	token.Header["typ"] = contracts.RouterAgentCredentialType
	token.Header["kid"] = issuer.keyID
	serialized, err := token.SignedString(issuer.privateKey)
	if err != nil {
		return "", errors.New("sign router credential")
	}
	return serialized, nil
}

func (issuer *Issuer) newJWTID() (string, error) {
	randomBytes := make([]byte, jwtIDRandomBytes)
	if _, err := io.ReadFull(issuer.random, randomBytes); err != nil {
		return "", errors.New("generate router credential identifier")
	}
	return "rtj_" + hex.EncodeToString(randomBytes), nil
}
