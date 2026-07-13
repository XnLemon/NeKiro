package gateway

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/config"
)

var ErrUnauthenticated = errors.New("authentication failed")

type Authenticator interface {
	Authenticate(*http.Request) (catalog.AuthenticatedCaller, error)
}

type staticPrincipal struct {
	id     string
	digest [sha256.Size]byte
}

type DevelopmentStaticAuthenticator struct {
	principals []staticPrincipal
}

func NewDevelopmentStaticAuthenticator(principals []config.StaticPrincipal) (*DevelopmentStaticAuthenticator, error) {
	if len(principals) == 0 {
		return nil, errors.New("development-static principals are required")
	}
	configured := make([]staticPrincipal, 0, len(principals))
	ids := make(map[string]struct{}, len(principals))
	digests := make(map[string]struct{}, len(principals))
	for _, principal := range principals {
		if !catalog.ValidIdentifier(principal.ID) {
			return nil, errors.New("development-static principal id is invalid")
		}
		digest, err := hex.DecodeString(principal.TokenSHA256)
		if err != nil || len(digest) != sha256.Size || principal.TokenSHA256 != strings.ToLower(principal.TokenSHA256) {
			return nil, errors.New("development-static token digest is invalid")
		}
		if _, exists := ids[principal.ID]; exists {
			return nil, errors.New("development-static principal id is duplicated")
		}
		if _, exists := digests[principal.TokenSHA256]; exists {
			return nil, errors.New("development-static token digest is duplicated")
		}
		var fixedDigest [sha256.Size]byte
		copy(fixedDigest[:], digest)
		configured = append(configured, staticPrincipal{id: principal.ID, digest: fixedDigest})
		ids[principal.ID] = struct{}{}
		digests[principal.TokenSHA256] = struct{}{}
	}
	return &DevelopmentStaticAuthenticator{principals: configured}, nil
}

func (authenticator *DevelopmentStaticAuthenticator) Authenticate(request *http.Request) (catalog.AuthenticatedCaller, error) {
	values := request.Header.Values("Authorization")
	if len(values) != 1 {
		return catalog.AuthenticatedCaller{}, ErrUnauthenticated
	}
	scheme, token, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t\r\n") {
		return catalog.AuthenticatedCaller{}, ErrUnauthenticated
	}

	digest := sha256.Sum256([]byte(token))
	matched := 0
	matchedID := ""
	for _, principal := range authenticator.principals {
		equal := subtle.ConstantTimeCompare(digest[:], principal.digest[:])
		if equal == 1 {
			matchedID = principal.id
		}
		matched |= equal
	}
	if matched != 1 {
		return catalog.AuthenticatedCaller{}, ErrUnauthenticated
	}
	return catalog.AuthenticatedCaller{ID: matchedID, AuthenticationKind: config.DevelopmentStaticAuthMode}, nil
}
