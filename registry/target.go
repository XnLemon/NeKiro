package registry

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var cardDigestRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ReleaseTargetInput contains the six byte-exact ReleaseTarget v1 fields.
// Construction validates but never trims, normalizes, or resolves them.
type ReleaseTargetInput struct {
	AgentID           string
	AgentCardVersion  string
	ReleaseID         string
	CardDigest        string
	CanonicalEndpoint string
	Audience          string
}

// ReleaseTarget is an exact, immutable Catalog-authorized release identity.
type ReleaseTarget struct {
	agentID           string
	agentCardVersion  string
	releaseID         string
	cardDigest        string
	canonicalEndpoint string
	audience          string
}

// NewReleaseTarget validates and copies an exact ReleaseTarget v1 value.
func NewReleaseTarget(input ReleaseTargetInput) (ReleaseTarget, error) {
	target := ReleaseTarget{
		agentID:           input.AgentID,
		agentCardVersion:  input.AgentCardVersion,
		releaseID:         input.ReleaseID,
		cardDigest:        input.CardDigest,
		canonicalEndpoint: input.CanonicalEndpoint,
		audience:          input.Audience,
	}
	if err := target.Validate(); err != nil {
		return ReleaseTarget{}, err
	}
	return target, nil
}

// Validate verifies that this value is a complete exact ReleaseTarget v1.
// It is useful to directory implementations because a zero value can be
// constructed without NewReleaseTarget.
func (t ReleaseTarget) Validate() error {
	if !validExactText(t.agentID) {
		return newInvalidError("agent_id")
	}
	if !validExactText(t.agentCardVersion) {
		return newInvalidError("agent_card_version")
	}
	if !validExactText(t.releaseID) {
		return newInvalidError("release_id")
	}
	if !cardDigestRE.MatchString(t.cardDigest) {
		return newInvalidError("card_digest")
	}
	if !validCanonicalEndpoint(t.canonicalEndpoint) {
		return newInvalidError("canonical_endpoint")
	}
	if !validAudienceOrigin(t.audience) {
		return newInvalidError("audience")
	}
	expectedAudience, ok := audienceForCanonicalEndpoint(t.canonicalEndpoint)
	if !ok || expectedAudience != t.audience {
		return newInvalidError("audience")
	}
	return nil
}

func validExactText(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validCanonicalEndpoint(raw string) bool {
	canonical, ok := canonicalHTTPValue(raw, true)
	return ok && canonical == raw
}

func validAudienceOrigin(raw string) bool {
	canonical, ok := canonicalHTTPValue(raw, false)
	return ok && canonical == raw
}

func audienceForCanonicalEndpoint(endpoint string) (string, bool) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", false
	}
	return canonicalHTTPValue(parsed.Scheme+"://"+parsed.Host, false)
}

// canonicalHTTPValue mirrors the target form used by Router credential
// validation without importing the contracts package. It returns a canonical
// HTTP(S) origin, plus a canonical path when allowPath is true.
func canonicalHTTPValue(raw string, allowPath bool) (string, bool) {
	if !validExactText(raw) || len(raw) > 2048 {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" || parsed.RawPath != "" {
		return "", false
	}
	if !allowPath && parsed.Path != "" {
		return "", false
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || strings.Contains(host, "%") {
		return "", false
	}
	portText := parsed.Port()
	if portText == "" && strings.HasSuffix(parsed.Host, ":") {
		return "", false
	}
	port := 0
	if portText != "" {
		port, err = strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return "", false
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
	if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
		if strings.Contains(host, ":") {
			hostPort = "[" + host + "]"
		} else {
			hostPort = host
		}
	}

	canonical := scheme + "://" + hostPort
	if !allowPath {
		return canonical, true
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if strings.Contains(path, "%") || strings.ContainsAny(parsed.Path, "\\\r\n") {
		return "", false
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	return canonical + path, true
}

func (t ReleaseTarget) AgentID() string           { return t.agentID }
func (t ReleaseTarget) AgentCardVersion() string  { return t.agentCardVersion }
func (t ReleaseTarget) ReleaseID() string         { return t.releaseID }
func (t ReleaseTarget) CardDigest() string        { return t.cardDigest }
func (t ReleaseTarget) CanonicalEndpoint() string { return t.canonicalEndpoint }
func (t ReleaseTarget) Audience() string          { return t.audience }

// Equal reports byte-exact equality across all six target fields.
func (t ReleaseTarget) Equal(other ReleaseTarget) bool {
	return t.agentID == other.agentID &&
		t.agentCardVersion == other.agentCardVersion &&
		t.releaseID == other.releaseID &&
		t.cardDigest == other.cardDigest &&
		t.canonicalEndpoint == other.canonicalEndpoint &&
		t.audience == other.audience
}
