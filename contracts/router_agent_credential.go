package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	semver "github.com/Masterminds/semver/v3"
)

const (
	RouterAgentCredentialSchemaVersion = "1"
	RouterAgentCredentialAlgorithm     = "EdDSA"
	RouterAgentCredentialType          = "nekiro-router+jwt"
	RouterAgentCredentialMaximumTTL    = 300 * time.Second

	RouterAgentAuthorizationHeader    = "Authorization"
	RouterAgentTraceHeader            = "x-nek-trace-id"
	RouterAgentInvocationHeader       = "x-nek-invocation-id"
	RouterAgentRootTaskHeader         = "x-nek-root-task-id"
	RouterAgentParentInvocationHeader = "x-nek-parent-invocation-id"
	RouterAgentWorkspaceHeader        = "x-nek-workspace-id"
	RouterAgentTargetAgentHeader      = "x-nek-target-agent-id"
	RouterAgentCardVersionHeader      = "x-nek-agent-card-version"
	RouterAgentReleaseHeader          = "x-nek-agent-release-id"
	RouterAgentCardDigestHeader       = "x-nek-agent-card-digest"
	RouterAgentCapabilityHeader       = "x-nek-capability"
)

type RouterInvocationCredentialContextV1 struct {
	Audience           string
	WorkspaceID        string
	AgentID            string
	AgentVersion       string
	ReleaseID          string
	CardDigest         string
	Capability         string
	InvocationID       string
	RootTaskID         string
	ParentInvocationID string
	TraceID            TraceID
}

type RouterInvocationCredentialClaimsV1 struct {
	Issuer             string   `json:"iss"`
	Audience           []string `json:"aud"`
	ExpiresAt          int64    `json:"exp"`
	IssuedAt           int64    `json:"iat"`
	JWTID              string   `json:"jti"`
	WorkspaceID        string   `json:"workspaceId"`
	AgentID            string   `json:"agentId"`
	AgentVersion       string   `json:"agentVersion"`
	ReleaseID          string   `json:"releaseId"`
	CardDigest         string   `json:"cardDigest"`
	Capability         string   `json:"capability"`
	InvocationID       string   `json:"invocationId"`
	RootTaskID         string   `json:"rootTaskId"`
	ParentInvocationID string   `json:"parentInvocationId,omitempty"`
	TraceID            TraceID  `json:"traceId"`
}

type RouterAgentAuthenticationErrorV1 struct {
	Code    PlatformErrorCode `json:"code"`
	Message string            `json:"message"`
}

type RouterAgentCredentialConformanceManifestV1 struct {
	SchemaVersion string                                 `json:"schemaVersion"`
	Cases         []RouterAgentCredentialConformanceCase `json:"cases"`
}

type RouterAgentCredentialConformanceCase struct {
	ID            string `json:"id"`
	File          string `json:"file"`
	Kind          string `json:"kind"`
	ExpectedValid bool   `json:"expectedValid"`
}

func LoadRouterAgentCredentialConformanceManifestV1() (RouterAgentCredentialConformanceManifestV1, error) {
	data, err := fs.ReadFile(contractFiles, "router-agent-credential/v1/conformance/manifest.json")
	if err != nil {
		return RouterAgentCredentialConformanceManifestV1{}, err
	}
	if err := rejectDuplicateJSONMemberNames(data); err != nil {
		return RouterAgentCredentialConformanceManifestV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest RouterAgentCredentialConformanceManifestV1
	if err := decoder.Decode(&manifest); err != nil {
		return RouterAgentCredentialConformanceManifestV1{}, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return RouterAgentCredentialConformanceManifestV1{}, err
	}
	if manifest.SchemaVersion != RouterAgentCredentialSchemaVersion || len(manifest.Cases) == 0 {
		return RouterAgentCredentialConformanceManifestV1{}, errors.New("router credential conformance manifest is invalid")
	}
	return manifest, nil
}

func NewRouterAgentAuthenticationErrorV1(code PlatformErrorCode) (RouterAgentAuthenticationErrorV1, error) {
	switch code {
	case ErrorCodeUnauthenticated:
		return RouterAgentAuthenticationErrorV1{Code: code, Message: "Authentication is required."}, nil
	case ErrorCodeForbidden:
		return RouterAgentAuthenticationErrorV1{Code: code, Message: "The managed invocation context is not allowed."}, nil
	default:
		return RouterAgentAuthenticationErrorV1{}, fmt.Errorf("unsupported Router Agent authentication error code %q", code)
	}
}

func ValidateRouterInvocationCredentialClaimsV1(claims RouterInvocationCredentialClaimsV1, now time.Time) error {
	if err := ValidateRouterAgentIssuer(claims.Issuer); err != nil {
		return err
	}
	if len(claims.Audience) != 1 {
		return errors.New("router credential must contain exactly one audience")
	}
	if err := ValidateRouterAgentAudience(claims.Audience[0]); err != nil {
		return err
	}
	if claims.IssuedAt < 1 || claims.ExpiresAt <= claims.IssuedAt {
		return errors.New("router credential time range is invalid")
	}
	if claims.ExpiresAt-claims.IssuedAt > int64(RouterAgentCredentialMaximumTTL/time.Second) {
		return errors.New("router credential lifetime exceeds the profile maximum")
	}
	nowUnix := now.UTC().Unix()
	if claims.IssuedAt > nowUnix {
		return errors.New("router credential issuance time is in the future")
	}
	if nowUnix >= claims.ExpiresAt {
		return errors.New("router credential is expired")
	}
	for name, value := range map[string]string{
		"jti": claims.JWTID, "workspaceId": claims.WorkspaceID,
		"agentId": claims.AgentID, "releaseId": claims.ReleaseID,
		"capability": claims.Capability, "invocationId": claims.InvocationID,
		"rootTaskId": claims.RootTaskID, "traceId": string(claims.TraceID),
	} {
		if !safeIdentifierPattern.MatchString(value) {
			return fmt.Errorf("router credential %s is invalid", name)
		}
	}
	if claims.ParentInvocationID != "" && !safeIdentifierPattern.MatchString(claims.ParentInvocationID) {
		return errors.New("router credential parentInvocationId is invalid")
	}
	if _, err := semver.StrictNewVersion(claims.AgentVersion); err != nil {
		return errors.New("router credential agentVersion is invalid")
	}
	if !isLowerHexDigest(claims.CardDigest) {
		return errors.New("router credential cardDigest is invalid")
	}
	return nil
}

func ValidateRouterAgentIssuer(value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 2048 {
		return errors.New("router credential issuer is invalid")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" {
		return errors.New("router credential issuer must be an HTTPS origin URI")
	}
	canonical, err := canonicalHTTPOrigin(value, false)
	if err != nil || canonical != value {
		return errors.New("router credential issuer must be canonical")
	}
	return nil
}

func ValidateRouterAgentAudience(value string) error {
	canonical, err := canonicalHTTPOrigin(value, false)
	if err != nil || canonical != value {
		return errors.New("router credential audience must be a canonical HTTP(S) origin")
	}
	return nil
}

func CanonicalRouterAgentAudience(endpoint string) (string, error) {
	return canonicalHTTPOrigin(endpoint, true)
}

func ValidateRouterAgentKeyID(value string) error {
	if !safeIdentifierPattern.MatchString(value) {
		return errors.New("router credential key ID is invalid")
	}
	return nil
}

func RouterAgentContextHeadersV1(context RouterInvocationCredentialContextV1) map[string]string {
	headers := map[string]string{
		RouterAgentTraceHeader:       string(context.TraceID),
		RouterAgentInvocationHeader:  context.InvocationID,
		RouterAgentRootTaskHeader:    context.RootTaskID,
		RouterAgentWorkspaceHeader:   context.WorkspaceID,
		RouterAgentTargetAgentHeader: context.AgentID,
		RouterAgentCardVersionHeader: context.AgentVersion,
		RouterAgentReleaseHeader:     context.ReleaseID,
		RouterAgentCardDigestHeader:  context.CardDigest,
		RouterAgentCapabilityHeader:  context.Capability,
	}
	if context.ParentInvocationID != "" {
		headers[RouterAgentParentInvocationHeader] = context.ParentInvocationID
	}
	return headers
}

func canonicalHTTPOrigin(value string, allowPath bool) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") || len(value) > 2048 {
		return "", errors.New("http origin value is invalid")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" || parsed.RawPath != "" {
		return "", errors.New("http origin value is invalid")
	}
	if !allowPath && parsed.Path != "" {
		return "", errors.New("http origin must not contain a path")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("http origin scheme is unsupported")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || strings.Contains(host, "%") {
		return "", errors.New("http origin host is invalid")
	}
	portText := parsed.Port()
	if portText == "" && strings.HasSuffix(parsed.Host, ":") {
		return "", errors.New("http origin port is invalid")
	}
	port := 0
	if portText != "" {
		port, err = strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return "", errors.New("http origin port is invalid")
		}
	}
	hostPort := host
	if net.ParseIP(host) != nil {
		if port != 0 {
			hostPort = net.JoinHostPort(host, strconv.Itoa(port))
		} else if strings.Contains(host, ":") {
			hostPort = "[" + host + "]"
		}
	} else if port != 0 {
		hostPort = host + ":" + strconv.Itoa(port)
	}
	if scheme == "http" && port == 80 || scheme == "https" && port == 443 {
		if strings.Contains(host, ":") {
			hostPort = "[" + host + "]"
		} else {
			hostPort = host
		}
	}
	return scheme + "://" + hostPort, nil
}

func isLowerHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}
