package workspace

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
)

var (
	ErrNotFound             = errors.New("workspace resource not found")
	ErrForbidden            = errors.New("workspace operation forbidden")
	ErrConflict             = errors.New("workspace operation conflict")
	ErrInvalid              = errors.New("workspace input invalid")
	ErrDependency           = errors.New("workspace dependency failed")
	ErrAgentNotInstalled    = errors.New("agent is not installed")
	ErrInstallationDisabled = errors.New("installation is disabled")
	ErrAgentDisabled        = errors.New("agent version is disabled")
	ErrReleaseUnpublished   = errors.New("agent release is not published")
	ErrReleaseSuspended     = errors.New("agent release is suspended")
	ErrReleaseRevoked       = errors.New("agent release is revoked")
	ErrCapabilityNotAllowed = errors.New("capability is not allowed")
	safeIdentifierRE        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

type AuthenticatedCaller struct {
	ID                 string
	AuthenticationKind string
}

type AuthorizedInvocation struct {
	AgentCardVersion string
	AgentReleaseID   string
	AgentCardDigest  string
}

type Clock func() time.Time
type IDGenerator func() (string, error)

type Authorizer interface {
	Authorize(ownerID, callerID string) error
}

type OwnerPolicy struct{}

func (OwnerPolicy) Authorize(ownerID, callerID string) error {
	if ownerID == "" || callerID == "" || ownerID != callerID {
		return ErrForbidden
	}
	return nil
}

// CatalogReader is the only Catalog dependency exposed to Workspace.
type CatalogReader interface {
	SelectInstallable(context.Context, string, string) (catalog.AgentVersion, error)
	GetVersion(context.Context, string, string) (catalog.AgentVersion, error)
}

type InstallationPosition struct {
	InstalledAt    time.Time
	InstallationID string
}

func ValidIdentifier(value string) bool {
	return safeIdentifierRE.MatchString(value)
}

func currentInstallationStatus(value string) bool {
	return value == "enabled" || value == "disabled"
}
