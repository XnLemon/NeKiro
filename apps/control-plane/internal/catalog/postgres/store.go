package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, errors.New("catalog database pool is required")
	}
	return &Store{pool: pool}, nil
}

func (store *Store) Register(ctx context.Context, version catalog.AgentVersion) (result catalog.AgentVersion, returnErr error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return catalog.AgentVersion{}, dependencyError("begin registration", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			returnErr = errors.Join(returnErr, dependencyError("rollback registration", rollbackErr))
		}
	}()

	if _, err := tx.Exec(ctx, `
INSERT INTO catalog.agent_identities (agent_id, owner_id, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (agent_id) DO NOTHING`, version.Card.AgentID, version.Card.Owner.ID, version.RegisteredAt); err != nil {
		return catalog.AgentVersion{}, dependencyError("claim Agent identity", err)
	}
	var ownerID string
	if err := tx.QueryRow(ctx, `
SELECT owner_id
FROM catalog.agent_identities
WHERE agent_id = $1
FOR UPDATE`, version.Card.AgentID).Scan(&ownerID); err != nil {
		return catalog.AgentVersion{}, dependencyError("read Agent owner", err)
	}
	var exactVersionExists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM catalog.agent_versions
    WHERE agent_id = $1 AND version = $2
)`, version.Card.AgentID, version.Card.Version).Scan(&exactVersionExists); err != nil {
		return catalog.AgentVersion{}, dependencyError("check exact Agent version", err)
	}
	if exactVersionExists {
		return catalog.AgentVersion{}, catalog.ErrConflict
	}
	if ownerID != version.Card.Owner.ID {
		return catalog.AgentVersion{}, catalog.ErrForbidden
	}

	var storedRegisteredAt time.Time
	err = tx.QueryRow(ctx, `
INSERT INTO catalog.agent_versions (
    agent_id, version, schema_version, card, card_name, card_description,
    card_digest, publication_status, registered_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING registered_at`,
		version.Card.AgentID,
		version.Card.Version,
		version.Card.SchemaVersion,
		string(version.CardJSON),
		version.Card.Name,
		version.Card.Description,
		version.CardDigest[:],
		version.Status,
		version.RegisteredAt,
	).Scan(&storedRegisteredAt)
	if err != nil {
		if constraintName(err) == "agent_versions_pkey" {
			return catalog.AgentVersion{}, catalog.ErrConflict
		}
		return catalog.AgentVersion{}, dependencyError("insert Agent version", err)
	}
	for _, skill := range version.Card.Skills {
		if _, err := tx.Exec(ctx, `
INSERT INTO catalog.agent_version_capabilities (agent_id, version, capability_id)
VALUES ($1, $2, $3)`, version.Card.AgentID, version.Card.Version, skill.ID); err != nil {
			return catalog.AgentVersion{}, dependencyError("insert capability index", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return catalog.AgentVersion{}, dependencyError("commit registration", err)
	}
	version.RegisteredAt = storedRegisteredAt
	return version, nil
}

func (store *Store) Get(ctx context.Context, agentID, version string) (catalog.AgentVersion, error) {
	row := store.pool.QueryRow(ctx, selectVersionSQL+`
WHERE v.agent_id = $1 AND v.version = $2`, agentID, version)
	entry, _, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.AgentVersion{}, catalog.ErrNotFound
	}
	if err != nil {
		return catalog.AgentVersion{}, dependencyError("read Agent version", err)
	}
	return entry, nil
}

func (store *Store) Published(ctx context.Context, agentID string) ([]catalog.AgentVersion, error) {
	rows, err := store.pool.Query(ctx, selectVersionSQL+`
WHERE v.agent_id = $1 AND v.publication_status = 'published'
ORDER BY v.version COLLATE "C" ASC`, agentID)
	if err != nil {
		return nil, dependencyError("list published Agent versions", err)
	}
	defer rows.Close()
	versions := make([]catalog.AgentVersion, 0)
	for rows.Next() {
		version, _, err := scanVersion(rows)
		if err != nil {
			return nil, dependencyError("scan published Agent version", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, dependencyError("read published Agent versions", err)
	}
	return versions, nil
}

func (store *Store) Publish(ctx context.Context, agentID, version, callerID string, at time.Time) (catalog.AgentVersion, error) {
	return store.transition(ctx, agentID, version, callerID, at, true)
}

func (store *Store) Disable(ctx context.Context, agentID, version, callerID string, at time.Time) (catalog.AgentVersion, error) {
	return store.transition(ctx, agentID, version, callerID, at, false)
}

func (store *Store) transition(ctx context.Context, agentID, version, callerID string, at time.Time, publish bool) (result catalog.AgentVersion, returnErr error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return catalog.AgentVersion{}, dependencyError("begin lifecycle transition", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			returnErr = errors.Join(returnErr, dependencyError("rollback lifecycle transition", rollbackErr))
		}
	}()
	row := tx.QueryRow(ctx, selectVersionSQL+`
WHERE v.agent_id = $1 AND v.version = $2
FOR UPDATE OF v`, agentID, version)
	entry, ownerID, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.AgentVersion{}, catalog.ErrNotFound
	}
	if err != nil {
		return catalog.AgentVersion{}, dependencyError("lock Agent version", err)
	}
	if ownerID != callerID {
		return catalog.AgentVersion{}, catalog.ErrForbidden
	}

	if publish {
		if entry.Status != catalog.PublicationDraft {
			return catalog.AgentVersion{}, catalog.ErrConflict
		}
		var sequence int64
		if err := tx.QueryRow(ctx, `
UPDATE catalog.publication_clock
SET last_sequence = last_sequence + 1
WHERE singleton = true
RETURNING last_sequence`).Scan(&sequence); err != nil {
			return catalog.AgentVersion{}, dependencyError("advance publication clock", err)
		}
		var storedPublishedAt time.Time
		if err := tx.QueryRow(ctx, `
UPDATE catalog.agent_versions
SET publication_status = 'published',
    published_at = $3,
    publication_sequence = $4
WHERE agent_id = $1 AND version = $2
RETURNING published_at`, agentID, version, at, sequence).Scan(&storedPublishedAt); err != nil {
			return catalog.AgentVersion{}, dependencyError("publish Agent version", err)
		}
		entry.Status = catalog.PublicationPublished
		entry.PublishedAt = &storedPublishedAt
		entry.PublicationSequence = &sequence
	} else if entry.Status != catalog.PublicationDisabled {
		var storedDisabledAt time.Time
		if err := tx.QueryRow(ctx, `
UPDATE catalog.agent_versions
SET publication_status = 'disabled', disabled_at = $3
WHERE agent_id = $1 AND version = $2
RETURNING disabled_at`, agentID, version, at).Scan(&storedDisabledAt); err != nil {
			return catalog.AgentVersion{}, dependencyError("disable Agent version", err)
		}
		entry.Status = catalog.PublicationDisabled
		entry.DisabledAt = &storedDisabledAt
	}
	if err := tx.Commit(ctx); err != nil {
		return catalog.AgentVersion{}, dependencyError("commit lifecycle transition", err)
	}
	return entry, nil
}

func (store *Store) DiscoverFirstPage(ctx context.Context, filter catalog.DiscoveryFilter) (snapshot int64, result catalog.DiscoveryResult, returnErr error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return 0, catalog.DiscoveryResult{}, dependencyError("begin discovery snapshot", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			returnErr = errors.Join(returnErr, dependencyError("rollback discovery snapshot", rollbackErr))
		}
	}()
	var sequence int64
	if err := tx.QueryRow(ctx, `
SELECT last_sequence
FROM catalog.publication_clock
WHERE singleton = true`).Scan(&sequence); err != nil {
		return 0, catalog.DiscoveryResult{}, dependencyError("read publication boundary", err)
	}
	result, err = discover(ctx, tx, catalog.DiscoveryQuery{Filter: filter, SnapshotPublicationSequence: sequence})
	if err != nil {
		return 0, catalog.DiscoveryResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, catalog.DiscoveryResult{}, dependencyError("commit discovery snapshot", err)
	}
	return sequence, result, nil
}

func (store *Store) Discover(ctx context.Context, query catalog.DiscoveryQuery) (catalog.DiscoveryResult, error) {
	return discover(ctx, store.pool, query)
}

type rowQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func discover(ctx context.Context, database rowQuerier, query catalog.DiscoveryQuery) (catalog.DiscoveryResult, error) {
	var textFilter any
	var capabilityFilter any
	var ownerFilter any
	if query.Filter.Query != nil {
		textFilter = *query.Filter.Query
	}
	if query.Filter.Capability != nil {
		capabilityFilter = *query.Filter.Capability
	}
	if query.Filter.OwnerID != nil {
		ownerFilter = *query.Filter.OwnerID
	}
	var lastPublishedAt any
	var lastAgentID any
	var lastVersion any
	if query.After != nil {
		lastPublishedAt = query.After.PublishedAt
		lastAgentID = query.After.AgentID
		lastVersion = query.After.Version
	}
	rows, err := database.Query(ctx, selectVersionSQL+`
WHERE v.publication_status = 'published'
  AND v.publication_sequence <= $1
  AND ($2::text IS NULL OR strpos(lower(v.card_name), lower($2)) > 0 OR strpos(lower(v.card_description), lower($2)) > 0)
  AND ($3::text IS NULL OR EXISTS (
      SELECT 1 FROM catalog.agent_version_capabilities c
      WHERE c.agent_id = v.agent_id AND c.version = v.version AND c.capability_id = $3
  ))
  AND ($4::text IS NULL OR i.owner_id = $4)
  AND ($5::timestamptz IS NULL OR
      v.published_at < $5 OR
      (v.published_at = $5 AND (v.agent_id > $6 OR (v.agent_id = $6 AND v.version > $7))))
ORDER BY v.published_at DESC, v.agent_id ASC, v.version ASC
LIMIT $8`,
		query.SnapshotPublicationSequence,
		textFilter,
		capabilityFilter,
		ownerFilter,
		lastPublishedAt,
		lastAgentID,
		lastVersion,
		query.Filter.Limit+1,
	)
	if err != nil {
		return catalog.DiscoveryResult{}, dependencyError("query discovery", err)
	}
	defer rows.Close()
	versions := make([]catalog.AgentVersion, 0, query.Filter.Limit+1)
	for rows.Next() {
		entry, _, err := scanVersion(rows)
		if err != nil {
			return catalog.DiscoveryResult{}, dependencyError("scan discovery row", err)
		}
		versions = append(versions, entry)
	}
	if err := rows.Err(); err != nil {
		return catalog.DiscoveryResult{}, dependencyError("read discovery rows", err)
	}
	hasMore := len(versions) > query.Filter.Limit
	if hasMore {
		versions = versions[:query.Filter.Limit]
	}
	return catalog.DiscoveryResult{Versions: versions, HasMore: hasMore}, nil
}

func (store *Store) Check(ctx context.Context) error {
	return CheckSchema(ctx, store.pool)
}

const selectVersionSQL = `
SELECT v.card,
       v.card_digest,
       v.publication_status,
       v.registered_at,
       v.published_at,
       v.publication_sequence,
       v.disabled_at,
       i.owner_id
FROM catalog.agent_versions v
JOIN catalog.agent_identities i ON i.agent_id = v.agent_id
`

type scanner interface {
	Scan(...any) error
}

func scanVersion(row scanner) (catalog.AgentVersion, string, error) {
	var cardJSON []byte
	var digest []byte
	var status catalog.PublicationStatus
	var registeredAt time.Time
	var publishedAt *time.Time
	var publicationSequence *int64
	var disabledAt *time.Time
	var ownerID string
	if err := row.Scan(
		&cardJSON,
		&digest,
		&status,
		&registeredAt,
		&publishedAt,
		&publicationSequence,
		&disabledAt,
		&ownerID,
	); err != nil {
		return catalog.AgentVersion{}, "", err
	}
	if len(digest) != sha256Size {
		return catalog.AgentVersion{}, "", errors.New("stored Card digest has invalid length")
	}
	card, err := decodeStoredCard(cardJSON)
	if err != nil {
		return catalog.AgentVersion{}, "", err
	}
	var fixedDigest [sha256Size]byte
	copy(fixedDigest[:], digest)
	return catalog.AgentVersion{
		Card:                card,
		CardJSON:            cardJSON,
		CardDigest:          fixedDigest,
		Status:              status,
		RegisteredAt:        registeredAt,
		PublishedAt:         publishedAt,
		PublicationSequence: publicationSequence,
		DisabledAt:          disabledAt,
	}, ownerID, nil
}

const sha256Size = 32

func decodeStoredCard(data []byte) (contracts.AgentCard, error) {
	var card contracts.AgentCard
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&card); err != nil {
		return contracts.AgentCard{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return contracts.AgentCard{}, errors.New("stored Card has trailing JSON")
		}
		return contracts.AgentCard{}, err
	}
	return card, nil
}

func constraintName(err error) string {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return postgresError.ConstraintName
	}
	return ""
}

func dependencyError(operation string, err error) error {
	return fmt.Errorf("%s: %w: %v", operation, catalog.ErrDependency, err)
}
