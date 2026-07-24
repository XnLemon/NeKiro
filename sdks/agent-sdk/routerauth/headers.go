package routerauth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Nene7ko/NeKiro/contracts"
)

func credentialFromHeaders(header http.Header) (string, error) {
	values := headerValues(header, contracts.RouterAgentAuthorizationHeader)
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return "", unauthenticated(errors.New("one Bearer authorization header is required"))
	}
	credential := strings.TrimPrefix(values[0], "Bearer ")
	if credential == "" || strings.ContainsAny(credential, " \t\r\n,") {
		return "", unauthenticated(errors.New("bearer credential is malformed"))
	}
	return credential, nil
}

func validateContextHeaders(header http.Header, claims contracts.RouterInvocationCredentialClaimsV1, parentClaimPresent bool) error {
	expected := map[string]string{
		contracts.RouterAgentWorkspaceHeader:   claims.WorkspaceID,
		contracts.RouterAgentTargetAgentHeader: claims.AgentID,
		contracts.RouterAgentCardVersionHeader: claims.AgentVersion,
		contracts.RouterAgentReleaseHeader:     claims.ReleaseID,
		contracts.RouterAgentCardDigestHeader:  claims.CardDigest,
		contracts.RouterAgentCapabilityHeader:  claims.Capability,
		contracts.RouterAgentInvocationHeader:  claims.InvocationID,
		contracts.RouterAgentRootTaskHeader:    claims.RootTaskID,
		contracts.RouterAgentTraceHeader:       string(claims.TraceID),
	}
	for name, value := range expected {
		values := headerValues(header, name)
		if len(values) != 1 || values[0] != value {
			return forbidden(errors.New("signed context header mismatch"))
		}
	}
	parentValues := headerValues(header, contracts.RouterAgentParentInvocationHeader)
	if parentClaimPresent {
		if len(parentValues) != 1 || parentValues[0] != claims.ParentInvocationID {
			return forbidden(errors.New("signed parent context header mismatch"))
		}
		return nil
	}
	if len(parentValues) != 0 {
		return forbidden(errors.New("unexpected parent context header"))
	}
	return nil
}

func headerValues(header http.Header, name string) []string {
	var values []string
	for candidate, candidateValues := range header {
		if strings.EqualFold(candidate, name) {
			values = append(values, candidateValues...)
		}
	}
	return values
}
