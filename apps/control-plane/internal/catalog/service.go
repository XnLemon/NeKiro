package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	semver "github.com/Masterminds/semver/v3"
	"github.com/Nene7ko/NeKiro/contracts"
)

type Clock func() time.Time

type Service struct {
	store     Store
	validator *contracts.Validator
	clock     Clock
}

func NewService(store Store, validator *contracts.Validator, clock Clock) (*Service, error) {
	if store == nil || validator == nil || clock == nil {
		return nil, errors.New("catalog service dependencies are required")
	}
	return &Service{store: store, validator: validator, clock: clock}, nil
}

func (service *Service) Register(ctx context.Context, caller AuthenticatedCaller, requestJSON []byte) (contracts.CatalogEntry, error) {
	request, err := service.validator.DecodeRegisterAgentRequest(requestJSON)
	if err != nil {
		return contracts.CatalogEntry{}, ErrInvalid
	}
	if caller.ID != request.Card.Owner.ID {
		return contracts.CatalogEntry{}, ErrForbidden
	}
	cardJSON, err := json.Marshal(request.Card)
	if err != nil {
		return contracts.CatalogEntry{}, fmt.Errorf("canonicalize Agent Card: %w", ErrDependency)
	}
	digest := sha256.Sum256(cardJSON)
	registered, err := service.store.Register(ctx, AgentVersion{
		Card:         request.Card,
		CardJSON:     cardJSON,
		CardDigest:   digest,
		Status:       PublicationDraft,
		RegisteredAt: service.clock().UTC(),
	})
	if err != nil {
		return contracts.CatalogEntry{}, err
	}
	return registered.CatalogEntry(), nil
}

func (service *Service) Get(ctx context.Context, caller AuthenticatedCaller, agentID, version string) (contracts.CatalogEntry, error) {
	if !validAgentVersionIdentity(agentID, version) {
		return contracts.CatalogEntry{}, ErrInvalid
	}
	entry, err := service.store.Get(ctx, agentID, version)
	if err != nil {
		return contracts.CatalogEntry{}, err
	}
	if entry.Status != PublicationPublished && caller.ID != entry.Card.Owner.ID {
		return contracts.CatalogEntry{}, ErrForbidden
	}
	return entry.CatalogEntry(), nil
}

func (service *Service) Publish(ctx context.Context, caller AuthenticatedCaller, agentID, version string) (contracts.CatalogEntry, error) {
	if !validAgentVersionIdentity(agentID, version) {
		return contracts.CatalogEntry{}, ErrInvalid
	}
	entry, err := service.store.Publish(ctx, agentID, version, caller.ID, service.clock().UTC())
	if err != nil {
		return contracts.CatalogEntry{}, err
	}
	return entry.CatalogEntry(), nil
}

func (service *Service) Disable(ctx context.Context, caller AuthenticatedCaller, agentID, version string) (contracts.CatalogEntry, error) {
	if !validAgentVersionIdentity(agentID, version) {
		return contracts.CatalogEntry{}, ErrInvalid
	}
	entry, err := service.store.Disable(ctx, agentID, version, caller.ID, service.clock().UTC())
	if err != nil {
		return contracts.CatalogEntry{}, err
	}
	return entry.CatalogEntry(), nil
}

func (service *Service) Search(ctx context.Context, query contracts.SearchAgentsQuery) (SearchResult, error) {
	filter, err := NormalizeDiscoveryFilter(query)
	if err != nil {
		return SearchResult{}, err
	}
	var snapshot int64
	var after *DiscoveryPosition
	if query.Cursor == nil {
		var firstPage DiscoveryResult
		snapshot, firstPage, err = service.store.DiscoverFirstPage(ctx, filter)
		if err != nil {
			return SearchResult{}, err
		}
		return buildSearchResult(filter, snapshot, firstPage)
	} else {
		var position DiscoveryPosition
		snapshot, position, err = DecodeCursor(*query.Cursor, filter)
		if err != nil {
			return SearchResult{}, err
		}
		after = &position
	}
	result, err := service.store.Discover(ctx, DiscoveryQuery{
		Filter: filter, SnapshotPublicationSequence: snapshot, After: after,
	})
	if err != nil {
		return SearchResult{}, err
	}
	return buildSearchResult(filter, snapshot, result)
}

func buildSearchResult(filter DiscoveryFilter, snapshot int64, result DiscoveryResult) (SearchResult, error) {
	entries := make([]contracts.CatalogEntry, 0, len(result.Versions))
	for _, version := range result.Versions {
		entries = append(entries, version.CatalogEntry())
	}
	response := SearchResult{Entries: entries}
	if result.HasMore && len(result.Versions) > 0 {
		last := result.Versions[len(result.Versions)-1]
		if last.PublishedAt == nil {
			return SearchResult{}, fmt.Errorf("published discovery row has no timestamp: %w", ErrDependency)
		}
		cursor, err := EncodeCursor(filter, snapshot, DiscoveryPosition{
			PublishedAt: *last.PublishedAt,
			AgentID:     last.Card.AgentID,
			Version:     last.Card.Version,
		})
		if err != nil {
			return SearchResult{}, fmt.Errorf("encode next cursor: %w", ErrDependency)
		}
		response.NextCursor = &cursor
	}
	return response, nil
}

func validAgentVersionIdentity(agentID, version string) bool {
	if !ValidIdentifier(agentID) {
		return false
	}
	_, err := semver.StrictNewVersion(version)
	return err == nil
}
