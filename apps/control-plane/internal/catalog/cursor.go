package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	semver "github.com/Masterminds/semver/v3"
	"github.com/Nene7ko/NeKiro/contracts"
)

const cursorVersion = 1

type cursorPayload struct {
	Version                     int       `json:"v"`
	FilterHash                  string    `json:"filter_hash"`
	SnapshotPublicationSequence int64     `json:"snapshot_publication_sequence"`
	LastPublishedAt             time.Time `json:"last_published_at"`
	LastAgentID                 string    `json:"last_agent_id"`
	LastVersion                 string    `json:"last_version"`
}

type canonicalFilter struct {
	Query      *string `json:"query"`
	Capability *string `json:"capability"`
	OwnerID    *string `json:"owner_id"`
	Limit      int     `json:"limit"`
}

func NormalizeDiscoveryFilter(query contracts.SearchAgentsQuery) (DiscoveryFilter, error) {
	filter := DiscoveryFilter{
		Query:      query.Query,
		Capability: query.Capability,
		OwnerID:    query.OwnerID,
		Limit:      contracts.DiscoveryDefaultLimit,
	}
	if query.Limit != nil {
		if *query.Limit < contracts.DiscoveryMinimumLimit || *query.Limit > contracts.DiscoveryMaximumLimit {
			return DiscoveryFilter{}, ErrInvalid
		}
		filter.Limit = *query.Limit
	}
	if filter.Query != nil {
		if strings.TrimSpace(*filter.Query) == "" || utf8.RuneCountInString(*filter.Query) > 256 {
			return DiscoveryFilter{}, ErrInvalid
		}
	}
	if filter.Capability != nil && !ValidIdentifier(*filter.Capability) {
		return DiscoveryFilter{}, ErrInvalid
	}
	if filter.OwnerID != nil && !ValidIdentifier(*filter.OwnerID) {
		return DiscoveryFilter{}, ErrInvalid
	}
	return filter, nil
}

func EncodeCursor(filter DiscoveryFilter, snapshot int64, position DiscoveryPosition) (string, error) {
	filterHash, err := hashFilter(filter)
	if err != nil {
		return "", err
	}
	payload := cursorPayload{
		Version:                     cursorVersion,
		FilterHash:                  filterHash,
		SnapshotPublicationSequence: snapshot,
		LastPublishedAt:             position.PublishedAt.UTC(),
		LastAgentID:                 position.AgentID,
		LastVersion:                 position.Version,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode discovery cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func DecodeCursor(value string, filter DiscoveryFilter) (int64, DiscoveryPosition, error) {
	data, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(data) == 0 {
		return 0, DiscoveryPosition{}, ErrInvalid
	}
	if err := rejectDuplicateCursorMembers(data); err != nil {
		return 0, DiscoveryPosition{}, ErrInvalid
	}
	var payload cursorPayload
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return 0, DiscoveryPosition{}, ErrInvalid
	}
	if err := cursorJSONEOF(decoder); err != nil {
		return 0, DiscoveryPosition{}, ErrInvalid
	}
	expectedHash, err := hashFilter(filter)
	if err != nil {
		return 0, DiscoveryPosition{}, err
	}
	decodedHash, hashErr := hex.DecodeString(payload.FilterHash)
	expectedBytes, expectedErr := hex.DecodeString(expectedHash)
	if hashErr != nil || expectedErr != nil || len(decodedHash) != sha256.Size || !bytes.Equal(decodedHash, expectedBytes) {
		return 0, DiscoveryPosition{}, ErrInvalid
	}
	if payload.Version != cursorVersion || payload.SnapshotPublicationSequence < 0 || payload.LastPublishedAt.IsZero() || !ValidIdentifier(payload.LastAgentID) {
		return 0, DiscoveryPosition{}, ErrInvalid
	}
	if _, err := semver.StrictNewVersion(payload.LastVersion); err != nil {
		return 0, DiscoveryPosition{}, ErrInvalid
	}
	return payload.SnapshotPublicationSequence, DiscoveryPosition{
		PublishedAt: payload.LastPublishedAt,
		AgentID:     payload.LastAgentID,
		Version:     payload.LastVersion,
	}, nil
}

func hashFilter(filter DiscoveryFilter) (string, error) {
	data, err := json.Marshal(canonicalFilter{
		Query: filter.Query, Capability: filter.Capability, OwnerID: filter.OwnerID, Limit: filter.Limit,
	})
	if err != nil {
		return "", fmt.Errorf("encode discovery filter: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func cursorJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON")
}

func rejectDuplicateCursorMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("cursor must be an object")
	}
	members := make(map[string]struct{})
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := nameToken.(string)
		if !ok {
			return errors.New("cursor member name is invalid")
		}
		if _, exists := members[name]; exists {
			return errors.New("cursor member is duplicated")
		}
		members[name] = struct{}{}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
