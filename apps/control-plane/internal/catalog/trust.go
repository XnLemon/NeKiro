package catalog

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

type VerificationStatus string

const (
	VerificationUnverified VerificationStatus = "unverified"
	VerificationPending    VerificationStatus = "pending"
	VerificationVerified   VerificationStatus = "verified"
	VerificationFailed     VerificationStatus = "failed"
	VerificationRevoked    VerificationStatus = "revoked"
	VerificationSuspended  VerificationStatus = "suspended"
)

const VerificationMethodHTTPWellKnown = "http_well_known"

var (
	ErrProviderNotFound    = errors.New("provider not found")
	ErrBindingNotFound     = errors.New("endpoint binding not found")
	ErrChallengeNotFound   = errors.New("verification challenge not found")
	ErrChallengeExpired    = errors.New("verification challenge expired")
	ErrChallengeReused     = errors.New("verification challenge already used")
	ErrWrongProof          = errors.New("endpoint returned the wrong verification proof")
	ErrEndpointUnavailable = errors.New("declared endpoint is unavailable")
	ErrRedirectNotAllowed  = errors.New("endpoint ownership verification redirect is not allowed")
	ErrDisallowedNetwork   = errors.New("declared endpoint resolves to a disallowed network")
	ErrEndpointInvalid     = errors.New("declared endpoint is invalid")
	ErrTrustConflict       = errors.New("trusted publication state conflict")
	ErrTrustDependency     = errors.New("trusted publication dependency failed")
)

type Provider struct {
	ProviderID         string
	OwnerIdentity      string
	VerificationStatus VerificationStatus
	VerificationMethod string
	VerifiedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type EndpointBinding struct {
	BindingID                  string
	ProviderID                 string
	AgentID                    string
	AgentCardVersion           string
	Endpoint                   string
	Origin                     string
	Path                       string
	VerificationMethod         string
	VerificationStatus         VerificationStatus
	VerificationEvidenceDigest *[32]byte
	VerificationFailureCode    *string
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	VerifiedAt                 *time.Time
	RevokedAt                  *time.Time
}

type VerificationChallenge struct {
	ChallengeID string
	BindingID   string
	ProofDigest [32]byte
	ExpiresAt   time.Time
	UsedAt      *time.Time
	CreatedAt   time.Time
}

type TrustStore interface {
	CreateBinding(context.Context, Provider, EndpointBinding) (EndpointBinding, error)
	GetProvider(context.Context, string) (Provider, error)
	GetBinding(context.Context, string, string) (EndpointBinding, error)
	CreateChallenge(context.Context, VerificationChallenge) error
	ReserveChallenge(context.Context, string, string, time.Time) (VerificationChallenge, EndpointBinding, error)
	SetBindingVerification(context.Context, string, VerificationStatus, *string, *[32]byte, time.Time) (EndpointBinding, error)
}

// AgentVersionReader is deliberately narrower than catalog.Store so Trust
// reads the exact Card fact without gaining ownership of Card persistence.
type AgentVersionReader interface {
	Get(context.Context, string, string) (AgentVersion, error)
}

type EndpointPolicy struct {
	LookupIP            func(context.Context, string) ([]net.IP, error)
	AllowedPrivateHosts map[string]struct{}
}

func NewEndpointPolicy(allowedPrivateHosts []string) EndpointPolicy {
	allowed := make(map[string]struct{}, len(allowedPrivateHosts))
	for _, host := range allowedPrivateHosts {
		if host != "" {
			allowed[strings.TrimSuffix(strings.ToLower(host), ".")] = struct{}{}
		}
	}
	return EndpointPolicy{LookupIP: func(ctx context.Context, host string) ([]net.IP, error) {
		addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		ips := make([]net.IP, 0, len(addresses))
		ips = append(ips, addresses...)
		return ips, nil
	}, AllowedPrivateHosts: allowed}
}

type Endpoint struct {
	Canonical string
	Origin    string
	Path      string
	Host      string
}

func ParseEndpoint(raw string) (Endpoint, error) {
	if strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n") {
		return Endpoint{}, ErrEndpointInvalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" {
		return Endpoint{}, ErrEndpointInvalid
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return Endpoint{}, ErrEndpointInvalid
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || strings.Contains(host, "%") || strings.ContainsAny(host, "\r\n") {
		return Endpoint{}, ErrEndpointInvalid
	}
	portText := parsed.Port()
	if portText == "" && strings.HasSuffix(parsed.Host, ":") {
		return Endpoint{}, ErrEndpointInvalid
	}
	port := 0
	if portText != "" {
		port, err = strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return Endpoint{}, ErrEndpointInvalid
		}
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if parsed.RawPath != "" || strings.Contains(path, "%") || strings.ContainsAny(parsed.Path, "\\\r\n") {
		return Endpoint{}, ErrEndpointInvalid
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return Endpoint{}, ErrEndpointInvalid
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
	origin := scheme + "://" + hostPort
	return Endpoint{Canonical: origin + path, Origin: origin, Path: path, Host: host}, nil
}

func (policy EndpointPolicy) ValidateDestination(ctx context.Context, endpoint Endpoint) error {
	_, err := policy.ResolveDestination(ctx, endpoint)
	return err
}

func (policy EndpointPolicy) ResolveDestination(ctx context.Context, endpoint Endpoint) ([]net.IP, error) {
	if endpoint.Host == "" {
		return nil, ErrEndpointInvalid
	}
	lookup := policy.LookupIP
	if lookup == nil {
		return nil, ErrTrustDependency
	}
	ips, err := lookup(ctx, endpoint.Host)
	if err != nil {
		return nil, fmt.Errorf("lookup endpoint destination: %w: %v", ErrTrustDependency, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("endpoint has no addresses: %w", ErrEndpointUnavailable)
	}
	if _, explicitlyAllowed := policy.AllowedPrivateHosts[strings.TrimSuffix(strings.ToLower(endpoint.Host), ".")]; explicitlyAllowed {
		hasDisallowed, hasPublic := false, false
		for _, ip := range ips {
			if isAlwaysDisallowedIP(ip) {
				return nil, ErrDisallowedNetwork
			}
			if isDisallowedIP(ip) {
				hasDisallowed = true
			} else {
				hasPublic = true
			}
		}
		if hasDisallowed && hasPublic {
			return nil, ErrDisallowedNetwork
		}
		return ips, nil
	}
	for _, ip := range ips {
		if isDisallowedIP(ip) {
			return nil, ErrDisallowedNetwork
		}
	}
	return ips, nil
}

func isDisallowedIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	return isReservedIP(ip)
}

func isAlwaysDisallowedIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalMulticast() || isReservedIP(ip) {
		return true
	}
	return !ip.IsGlobalUnicast() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}

var reservedTrustNetworks = func() []*net.IPNet {
	values := []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "192.31.196.0/24", "192.52.193.0/24", "192.88.99.0/24", "192.175.48.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "100::/64", "2001:1::/48", "2001:2::/48", "2001:10::/28", "2001:20::/28", "2001:db8::/32"}
	networks := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			panic(err)
		}
		networks = append(networks, network)
	}
	return networks
}()

func isReservedIP(ip net.IP) bool {
	for _, network := range reservedTrustNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

type TrustService struct {
	store               TrustStore
	versions            AgentVersionReader
	clock               Clock
	newID               func(string) (string, error)
	challengeTTL        time.Duration
	verificationTimeout time.Duration
	policy              EndpointPolicy
	httpClient          *http.Client
}

func NewTrustService(store TrustStore, versions AgentVersionReader, clock Clock, policy EndpointPolicy, httpClient *http.Client, challengeTTL, verificationTimeout time.Duration) (*TrustService, error) {
	if store == nil || versions == nil || clock == nil || httpClient == nil {
		return nil, errors.New("trusted publication dependencies are required")
	}
	if challengeTTL <= 0 {
		return nil, errors.New("trusted publication challenge TTL must be positive")
	}
	if verificationTimeout <= 0 || verificationTimeout >= challengeTTL {
		return nil, errors.New("trusted publication verification timeout must be positive and shorter than challenge TTL")
	}
	client := *httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &TrustService{store: store, versions: versions, clock: clock, newID: newTrustID, challengeTTL: challengeTTL, verificationTimeout: verificationTimeout, policy: policy, httpClient: &client}, nil
}

func (service *TrustService) CreateBinding(ctx context.Context, caller catalogCaller, providerID, agentID, version, endpoint, method string) (EndpointBinding, error) {
	if !ValidIdentifier(providerID) || !validAgentVersionIdentity(agentID, version) || !ValidIdentifier(caller.ID) || method != VerificationMethodHTTPWellKnown {
		return EndpointBinding{}, ErrInvalid
	}
	parsed, err := ParseEndpoint(endpoint)
	if err != nil {
		return EndpointBinding{}, err
	}
	agentVersion, err := service.versions.Get(ctx, agentID, version)
	if err != nil {
		return EndpointBinding{}, err
	}
	cardEndpoint, err := ParseEndpoint(agentVersion.Card.Protocol.Endpoint)
	if err != nil {
		return EndpointBinding{}, fmt.Errorf("read stored Agent endpoint: %w", ErrTrustDependency)
	}
	if cardEndpoint.Canonical != parsed.Canonical {
		return EndpointBinding{}, ErrEndpointInvalid
	}
	now := service.clock().UTC()
	bindingID, err := service.newID("binding")
	if err != nil {
		return EndpointBinding{}, fmt.Errorf("generate endpoint binding id: %w", ErrTrustDependency)
	}
	return service.store.CreateBinding(ctx, Provider{ProviderID: providerID, OwnerIdentity: caller.ID, VerificationStatus: VerificationUnverified, VerificationMethod: method, CreatedAt: now, UpdatedAt: now}, EndpointBinding{BindingID: bindingID, ProviderID: providerID, AgentID: agentID, AgentCardVersion: version, Endpoint: parsed.Canonical, Origin: parsed.Origin, Path: parsed.Path, VerificationMethod: method, VerificationStatus: VerificationPending, CreatedAt: now, UpdatedAt: now})
}

// catalogCaller is kept local so the trust service does not import Gateway.
type catalogCaller struct{ ID string }

func (service *TrustService) CreateChallenge(ctx context.Context, caller catalogCaller, providerID, bindingID string) (contracts.VerificationChallengeResponse, error) {
	if !ValidIdentifier(providerID) || !ValidIdentifier(bindingID) || !ValidIdentifier(caller.ID) {
		return contracts.VerificationChallengeResponse{}, ErrInvalid
	}
	if err := service.authorizeProvider(ctx, caller, providerID); err != nil {
		return contracts.VerificationChallengeResponse{}, err
	}
	binding, err := service.store.GetBinding(ctx, providerID, bindingID)
	if err != nil {
		return contracts.VerificationChallengeResponse{}, err
	}
	if binding.VerificationStatus == VerificationVerified || binding.VerificationStatus == VerificationRevoked {
		return contracts.VerificationChallengeResponse{}, ErrTrustConflict
	}
	now := service.clock().UTC()
	challengeID, err := service.newID("challenge")
	if err != nil {
		return contracts.VerificationChallengeResponse{}, fmt.Errorf("generate verification challenge id: %w", ErrTrustDependency)
	}
	proofBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, proofBytes); err != nil {
		return contracts.VerificationChallengeResponse{}, fmt.Errorf("generate verification proof: %w", ErrTrustDependency)
	}
	proof := hex.EncodeToString(proofBytes)
	challenge := VerificationChallenge{ChallengeID: challengeID, BindingID: binding.BindingID, ProofDigest: sha256.Sum256([]byte(proof)), ExpiresAt: now.Add(service.challengeTTL), CreatedAt: now}
	if err := service.store.CreateChallenge(ctx, challenge); err != nil {
		return contracts.VerificationChallengeResponse{}, err
	}
	return contracts.VerificationChallengeResponse{ChallengeID: challengeID, BindingID: binding.BindingID, ChallengeURL: binding.Origin + "/.well-known/nekiro/challenges/" + challengeID, Proof: proof, ExpiresAt: challenge.ExpiresAt}, nil
}

func (service *TrustService) CompleteChallenge(ctx context.Context, caller catalogCaller, providerID, bindingID, challengeID string) (EndpointBinding, error) {
	if !ValidIdentifier(providerID) || !ValidIdentifier(bindingID) || !ValidIdentifier(challengeID) || !ValidIdentifier(caller.ID) {
		return EndpointBinding{}, ErrInvalid
	}
	if err := service.authorizeProvider(ctx, caller, providerID); err != nil {
		return EndpointBinding{}, err
	}
	endpointBinding, err := service.store.GetBinding(ctx, providerID, bindingID)
	if err != nil {
		return EndpointBinding{}, err
	}
	if endpointBinding.VerificationStatus == VerificationRevoked || endpointBinding.VerificationStatus == VerificationVerified {
		return EndpointBinding{}, ErrTrustConflict
	}
	endpoint, err := ParseEndpoint(endpointBinding.Endpoint)
	if err != nil {
		return EndpointBinding{}, err
	}
	challenge, reservedBinding, err := service.store.ReserveChallenge(ctx, bindingID, challengeID, service.clock().UTC())
	if err != nil {
		if errors.Is(err, ErrChallengeExpired) {
			_, recordErr := service.failVerification(ctx, bindingID, ErrChallengeExpired)
			if recordErr != nil && !errors.Is(recordErr, ErrTrustConflict) {
				return EndpointBinding{}, recordErr
			}
		}
		return EndpointBinding{}, err
	}
	endpointBinding = reservedBinding
	verificationDuration := service.verificationTimeout
	remainingChallenge := challenge.ExpiresAt.Sub(service.clock().UTC())
	if remainingChallenge < verificationDuration {
		verificationDuration = remainingChallenge
	}
	verificationContext, cancel := context.WithTimeout(ctx, verificationDuration)
	defer cancel()
	destinationIPs, err := service.policy.ResolveDestination(verificationContext, endpoint)
	if err != nil {
		if !service.clock().UTC().Before(challenge.ExpiresAt) {
			return service.failVerification(ctx, bindingID, ErrChallengeExpired)
		}
		return service.failVerification(ctx, bindingID, err)
	}
	proof, err := fetchVerificationProof(verificationContext, service.httpClient, endpoint, challengeID, destinationIPs)
	if err != nil {
		if !service.clock().UTC().Before(challenge.ExpiresAt) {
			return service.failVerification(ctx, bindingID, ErrChallengeExpired)
		}
		return service.failVerification(ctx, bindingID, err)
	}
	if !service.clock().UTC().Before(challenge.ExpiresAt) {
		return service.failVerification(ctx, bindingID, ErrChallengeExpired)
	}
	digest := sha256.Sum256([]byte(proof))
	if subtle.ConstantTimeCompare(digest[:], challenge.ProofDigest[:]) != 1 {
		return service.failVerification(ctx, bindingID, ErrWrongProof)
	}
	verifiedAt := service.clock().UTC()
	return service.store.SetBindingVerification(ctx, bindingID, VerificationVerified, nil, &digest, verifiedAt)
}

func (service *TrustService) failVerification(ctx context.Context, bindingID string, cause error) (EndpointBinding, error) {
	code, classified := verificationFailureCode(cause)
	if !classified {
		return EndpointBinding{}, cause
	}
	binding, err := service.store.SetBindingVerification(ctx, bindingID, VerificationFailed, &code, nil, service.clock().UTC())
	if err != nil {
		if errors.Is(err, ErrTrustConflict) {
			return EndpointBinding{}, err
		}
		return EndpointBinding{}, fmt.Errorf("record verification failure: %w", ErrTrustDependency)
	}
	return binding, cause
}

func (service *TrustService) GetBinding(ctx context.Context, caller catalogCaller, providerID, bindingID string) (EndpointBinding, error) {
	if !ValidIdentifier(providerID) || !ValidIdentifier(bindingID) || !ValidIdentifier(caller.ID) {
		return EndpointBinding{}, ErrInvalid
	}
	if err := service.authorizeProvider(ctx, caller, providerID); err != nil {
		return EndpointBinding{}, err
	}
	return service.store.GetBinding(ctx, providerID, bindingID)
}

func (service *TrustService) authorizeProvider(ctx context.Context, caller catalogCaller, providerID string) error {
	provider, err := service.store.GetProvider(ctx, providerID)
	if err != nil {
		return err
	}
	if provider.OwnerIdentity != caller.ID {
		return ErrForbidden
	}
	if provider.VerificationStatus == VerificationSuspended {
		return ErrForbidden
	}
	return nil
}

func fetchVerificationProof(ctx context.Context, client *http.Client, endpoint Endpoint, challengeID string, destinationIPs []net.IP) (proof string, returnErr error) {
	if len(destinationIPs) == 0 {
		return "", ErrEndpointUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.Origin+"/.well-known/nekiro/challenges/"+challengeID, nil)
	if err != nil {
		return "", ErrEndpointInvalid
	}
	request.Header.Set("Accept", "text/plain")
	pinnedClient, err := clientForPinnedDestination(client, endpoint, destinationIPs[0])
	if err != nil {
		return "", err
	}
	response, err := pinnedClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("request ownership challenge: %w: %v", ErrEndpointUnavailable, err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil && returnErr == nil {
			proof = ""
			returnErr = fmt.Errorf("close ownership challenge response: %w: %v", ErrEndpointUnavailable, closeErr)
		}
	}()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
			return "", ErrRedirectNotAllowed
		}
		return "", ErrWrongProof
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(body) > 4096 {
		return "", ErrWrongProof
	}
	return string(body), nil
}

func clientForPinnedDestination(client *http.Client, endpoint Endpoint, destination net.IP) (*http.Client, error) {
	var base *http.Transport
	switch transport := client.Transport.(type) {
	case nil:
		defaultTransport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, ErrTrustDependency
		}
		base = defaultTransport.Clone()
	case *http.Transport:
		base = transport.Clone()
	default:
		return nil, ErrTrustDependency
	}
	parsed, err := url.Parse(endpoint.Origin)
	if err != nil {
		return nil, ErrEndpointInvalid
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	expectedAddress := net.JoinHostPort(strings.ToLower(endpoint.Host), port)
	base.Proxy = nil
	//nolint:staticcheck // clearing the deprecated hook is required to prevent an injected TLS dialer bypassing the pinned DialContext.
	base.DialTLS = nil
	base.DialTLSContext = nil
	if base.TLSClientConfig == nil {
		base.TLSClientConfig = &tls.Config{ServerName: endpoint.Host}
	} else {
		base.TLSClientConfig = base.TLSClientConfig.Clone()
		base.TLSClientConfig.ServerName = endpoint.Host
	}
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		requestedHost, requestedPort, err := net.SplitHostPort(address)
		if err != nil || net.JoinHostPort(strings.ToLower(requestedHost), requestedPort) != expectedAddress {
			return nil, ErrDisallowedNetwork
		}
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, net.JoinHostPort(destination.String(), port))
	}
	pinned := *client
	pinned.Transport = base
	return &pinned, nil
}

func verificationFailureCode(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrWrongProof):
		return "wrong_proof", true
	case errors.Is(err, ErrRedirectNotAllowed):
		return "redirect_not_allowed", true
	case errors.Is(err, ErrChallengeExpired):
		return "challenge_expired", true
	case errors.Is(err, ErrEndpointUnavailable):
		return "endpoint_unavailable", true
	case errors.Is(err, ErrDisallowedNetwork):
		return "disallowed_network", true
	case errors.Is(err, ErrEndpointInvalid):
		return "invalid_endpoint", true
	case errors.Is(err, ErrTrustDependency):
		return "dependency_failure", true
	default:
		return "", false
	}
}

func newTrustID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(raw), nil
}

// TrustCaller converts the Gateway caller without introducing a Gateway
// dependency into the Catalog package.
func TrustCaller(id string) catalogCaller { return catalogCaller{ID: id} }

func (service *TrustService) CreateBindingForCaller(ctx context.Context, caller AuthenticatedCaller, providerID, agentID, version, endpoint, method string) (EndpointBinding, error) {
	return service.CreateBinding(ctx, TrustCaller(caller.ID), providerID, agentID, version, endpoint, method)
}

func (service *TrustService) CreateChallengeForCaller(ctx context.Context, caller AuthenticatedCaller, providerID, bindingID string) (contracts.VerificationChallengeResponse, error) {
	return service.CreateChallenge(ctx, TrustCaller(caller.ID), providerID, bindingID)
}

func (service *TrustService) CompleteChallengeForCaller(ctx context.Context, caller AuthenticatedCaller, providerID, bindingID, challengeID string) (EndpointBinding, error) {
	return service.CompleteChallenge(ctx, TrustCaller(caller.ID), providerID, bindingID, challengeID)
}

func (service *TrustService) GetBindingForCaller(ctx context.Context, caller AuthenticatedCaller, providerID, bindingID string) (EndpointBinding, error) {
	return service.GetBinding(ctx, TrustCaller(caller.ID), providerID, bindingID)
}
