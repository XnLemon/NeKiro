package catalog

import (
	"errors"
	"regexp"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
)

type PublicationStatus string

const (
	PublicationDraft     PublicationStatus = "draft"
	PublicationPublished PublicationStatus = "published"
	PublicationDisabled  PublicationStatus = "disabled"
)

var (
	ErrNotFound      = errors.New("catalog entry not found")
	ErrForbidden     = errors.New("catalog operation forbidden")
	ErrConflict      = errors.New("catalog operation conflict")
	ErrInvalid       = errors.New("catalog input invalid")
	ErrDependency    = errors.New("catalog dependency failed")
	safeIdentifierRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

type AgentVersion struct {
	Card                contracts.AgentCard
	CardJSON            []byte
	CardDigest          [32]byte
	Status              PublicationStatus
	RegisteredAt        time.Time
	PublishedAt         *time.Time
	PublicationSequence *int64
	DisabledAt          *time.Time
}

func (version AgentVersion) CatalogEntry() contracts.CatalogEntry {
	return contracts.CatalogEntry{
		Card:              version.Card,
		PublicationStatus: string(version.Status),
		RegisteredAt:      version.RegisteredAt,
		PublishedAt:       version.PublishedAt,
	}
}

type DiscoveryFilter struct {
	Query      *string
	Capability *string
	OwnerID    *string
	Limit      int
}

type DiscoveryPosition struct {
	PublishedAt time.Time
	AgentID     string
	Version     string
}

type DiscoveryQuery struct {
	Filter                      DiscoveryFilter
	SnapshotPublicationSequence int64
	After                       *DiscoveryPosition
}

type DiscoveryResult struct {
	Versions []AgentVersion
	HasMore  bool
}

type SearchResult struct {
	Entries    []contracts.CatalogEntry
	NextCursor *string
}

type AuthenticatedCaller struct {
	ID                 string
	AuthenticationKind string
}

func ValidIdentifier(value string) bool {
	return safeIdentifierRE.MatchString(value)
}
