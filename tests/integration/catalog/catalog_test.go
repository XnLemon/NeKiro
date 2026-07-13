//go:build integration

package catalog_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/tern/v2/migrate"
)

const (
	ownerAToken = "catalog-owner-a-integration-token"
	ownerBToken = "catalog-owner-b-integration-token"
	userToken   = "catalog-user-integration-token"
)

type testServer struct {
	command *exec.Cmd
	logs    bytes.Buffer
	baseURL string
}

type httpResult struct {
	status int
	header http.Header
	body   []byte
}

func TestCatalogPostgreSQLAndHTTPAcceptance(t *testing.T) {
	ctx := context.Background()
	databaseURL := guardedDatabaseURL(t)
	root := repositoryRoot(t)
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create test database pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect dedicated test database: %v", err)
	}
	resetCatalog(t, pool)

	binary := buildControlPlane(t, root)
	assertV1ToV2Migration(t, databaseURL, root, pool)
	runCommand(t, root, databaseURL, binary, "migrate", "up")
	assertCatalogSchemaV2(t, pool)
	runCommand(t, root, databaseURL, binary, "migrate", "up")
	assertUnsupportedMigrationLeavesPopulatedCatalog(t, root, databaseURL, binary, pool)

	server := startServer(t, root, databaseURL, binary)
	defer func() {
		server.stop(t)
		assertLogsAreSecretSafe(t, server.logs.String())
	}()

	runtimeA := readFixture(t, root, "runtime-a-card.json")
	runtimeB := readFixture(t, root, "runtime-b-card.json")

	t.Run("fixed authentication and registration semantics", func(t *testing.T) {
		missing := request(t, http.MethodGet, server.baseURL+"/v2/agents", "", nil)
		assertPlatformError(t, missing, http.StatusUnauthorized, contracts.ErrorCodeUnauthenticated)
		if bytes.Contains(missing.body, []byte(ownerAToken)) || bytes.Contains(missing.body, []byte(digest(ownerAToken))) {
			t.Fatal("authentication material appeared in public error")
		}

		draft := request(t, http.MethodPost, server.baseURL+"/v2/agents", ownerAToken, registrationEnvelope(t, runtimeA))
		draftEntry := decodeEntry(t, draft)
		if draft.status != http.StatusCreated || draftEntry.PublicationStatus != "draft" {
			t.Fatalf("Runtime A registration = %d %s", draft.status, draft.body)
		}
		if !sameJSONNumber(draftEntry.Card.Limits.MaxInputBytes.String(), "1e400") {
			t.Fatalf("large maxInputBytes changed value to %s", draftEntry.Card.Limits.MaxInputBytes)
		}
		var storedRegisteredAt time.Time
		if err := pool.QueryRow(ctx, `SELECT registered_at FROM catalog.agent_versions WHERE agent_id = 'runtime-a' AND version = '1.0.0'`).Scan(&storedRegisteredAt); err != nil || !draftEntry.RegisteredAt.Equal(storedRegisteredAt) {
			t.Fatalf("registration response time = %s, stored = %s, err = %v", draftEntry.RegisteredAt, storedRegisteredAt, err)
		}
		assertPlatformError(t, request(t, http.MethodPost, server.baseURL+"/v2/agents", ownerAToken, registrationEnvelope(t, runtimeA)), http.StatusConflict, contracts.ErrorCodeConflict)

		boundaryCard := decodeCard(t, runtimeA)
		boundaryCard.AgentID = "unbounded-number-agent"
		boundaryCard.Name = "Unbounded Number Agent"
		boundaryCard.Description = "Preserves JSON numbers beyond PostgreSQL numeric range."
		boundaryCard.Skills[0].ID = "number.boundary"
		boundaryCard.Skills[0].Name = "Number boundary"
		boundaryCard.Limits.MaxInputBytes = json.Number("1e131072")
		boundary := request(t, http.MethodPost, server.baseURL+"/v2/agents", ownerAToken, registrationEnvelope(t, mustJSON(t, boundaryCard)))
		boundaryEntry := decodeEntry(t, boundary)
		if boundary.status != http.StatusCreated {
			t.Fatalf("unbounded number registration = %d %s", boundary.status, boundary.body)
		}
		if got := boundaryEntry.Card.Limits.MaxInputBytes.String(); got != "1e131072" {
			t.Fatalf("unbounded response number = %s, want 1e131072", got)
		}
		var storedCard, cardType, storedName, storedDescription string
		if err := pool.QueryRow(ctx, `
SELECT card, pg_typeof(card)::text, card_name, card_description
FROM catalog.agent_versions
WHERE agent_id = 'unbounded-number-agent' AND version = '1.0.0'`).Scan(
			&storedCard,
			&cardType,
			&storedName,
			&storedDescription,
		); err != nil {
			t.Fatal(err)
		}
		storedBoundaryCard := decodeCard(t, []byte(storedCard))
		if cardType != "text" || storedBoundaryCard.Limits.MaxInputBytes.String() != "1e131072" {
			t.Fatalf("stored Card type/number = %s/%s", cardType, storedBoundaryCard.Limits.MaxInputBytes)
		}
		if storedName != boundaryCard.Name || storedDescription != boundaryCard.Description {
			t.Fatalf("derived text = %q/%q", storedName, storedDescription)
		}
		if result := request(t, http.MethodPost, server.baseURL+"/v2/agents/unbounded-number-agent/versions/1.0.0/publish", ownerAToken, nil); result.status != http.StatusOK {
			t.Fatalf("publish unbounded number Card = %d %s", result.status, result.body)
		}
		boundaryDiscovery := decodeSearch(t, request(t, http.MethodGet, server.baseURL+"/v2/agents?query=PostgreSQL&capability=number.boundary", userToken, nil))
		if len(boundaryDiscovery.Items) != 1 || boundaryDiscovery.Items[0].Card.Limits.MaxInputBytes.String() != "1e131072" {
			t.Fatalf("unbounded number Discovery = %#v", boundaryDiscovery)
		}

		invalid := append([]byte(nil), runtimeA...)
		invalid = bytes.Replace(invalid, []byte(`"schemaVersion": "0.2"`), []byte(`"schemaVersion": "0.1"`), 1)
		assertPlatformError(t, request(t, http.MethodPost, server.baseURL+"/v2/agents", ownerAToken, registrationEnvelope(t, invalid)), http.StatusBadRequest, contracts.ErrorCodeValidationError)

		crossOwner := decodeCard(t, runtimeA)
		crossOwner.Version = "2.0.0"
		crossOwner.Owner.ID = "catalog-owner-b"
		assertPlatformError(t, request(t, http.MethodPost, server.baseURL+"/v2/agents", ownerBToken, registrationEnvelope(t, mustJSON(t, crossOwner))), http.StatusForbidden, contracts.ErrorCodeForbidden)

		assertPlatformError(t, request(t, http.MethodGet, server.baseURL+"/v2/agents/runtime-a/versions/1.0.0", userToken, nil), http.StatusForbidden, contracts.ErrorCodeForbidden)
		if result := request(t, http.MethodGet, server.baseURL+"/v2/agents/runtime-a/versions/1.0.0", ownerAToken, nil); result.status != http.StatusOK {
			t.Fatalf("owner draft read = %d %s", result.status, result.body)
		}
		var versionRows int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM catalog.agent_versions WHERE agent_id = 'runtime-a'`).Scan(&versionRows); err != nil || versionRows != 1 {
			t.Fatalf("rollback evidence rows = %d, err = %v", versionRows, err)
		}
	})

	t.Run("publication discovery disablement and cross-runtime metadata", func(t *testing.T) {
		publishedA := request(t, http.MethodPost, server.baseURL+"/v2/agents/runtime-a/versions/1.0.0/publish", ownerAToken, nil)
		publishedEntry := decodeEntry(t, publishedA)
		if publishedA.status != http.StatusOK || publishedEntry.PublicationStatus != "published" {
			t.Fatalf("publish Runtime A = %d %s", publishedA.status, publishedA.body)
		}
		var storedPublishedAt time.Time
		if err := pool.QueryRow(ctx, `SELECT published_at FROM catalog.agent_versions WHERE agent_id = 'runtime-a' AND version = '1.0.0'`).Scan(&storedPublishedAt); err != nil || publishedEntry.PublishedAt == nil || !publishedEntry.PublishedAt.Equal(storedPublishedAt) {
			t.Fatalf("publication response time = %v, stored = %s, err = %v", publishedEntry.PublishedAt, storedPublishedAt, err)
		}
		assertPlatformError(t, request(t, http.MethodPost, server.baseURL+"/v2/agents/runtime-a/versions/1.0.0/publish", ownerAToken, nil), http.StatusConflict, contracts.ErrorCodeConflict)
		if result := request(t, http.MethodGet, server.baseURL+"/v2/agents/runtime-a/versions/1.0.0", userToken, nil); result.status != http.StatusOK {
			t.Fatalf("published public read = %d %s", result.status, result.body)
		}

		if result := request(t, http.MethodPost, server.baseURL+"/v2/agents", ownerBToken, registrationEnvelope(t, runtimeB)); result.status != http.StatusCreated {
			t.Fatalf("register Runtime B = %d %s", result.status, result.body)
		}
		if result := request(t, http.MethodPost, server.baseURL+"/v2/agents/runtime-b/versions/1.0.0/publish", ownerBToken, nil); result.status != http.StatusOK {
			t.Fatalf("publish Runtime B = %d %s", result.status, result.body)
		}
		search := decodeSearch(t, request(t, http.MethodGet, server.baseURL+"/v2/agents?capability=runtime.echo", userToken, nil))
		if len(search.Items) != 2 {
			t.Fatalf("cross-runtime discovery count = %d, want 2", len(search.Items))
		}
		filtered := decodeSearch(t, request(t, http.MethodGet, server.baseURL+"/v2/agents?query=translation&capability=runtime.translate&ownerId=catalog-owner-b", userToken, nil))
		if len(filtered.Items) != 1 || filtered.Items[0].Card.AgentID != "runtime-b" {
			t.Fatalf("combined discovery = %#v", filtered)
		}
		for _, literal := range []string{"%", "_"} {
			literalResult := decodeSearch(t, request(t, http.MethodGet, server.baseURL+"/v2/agents?query="+url.QueryEscape(literal), userToken, nil))
			if len(literalResult.Items) != 0 {
				t.Fatalf("literal wildcard query %q matched %d rows", literal, len(literalResult.Items))
			}
		}

		assertPlatformError(t, request(t, http.MethodPost, server.baseURL+"/v2/agents/runtime-a/versions/1.0.0/disable", userToken, nil), http.StatusForbidden, contracts.ErrorCodeForbidden)
		firstDisable := request(t, http.MethodPost, server.baseURL+"/v2/agents/runtime-a/versions/1.0.0/disable", ownerAToken, nil)
		secondDisable := request(t, http.MethodPost, server.baseURL+"/v2/agents/runtime-a/versions/1.0.0/disable", ownerAToken, nil)
		firstEntry, secondEntry := decodeEntry(t, firstDisable), decodeEntry(t, secondDisable)
		if firstDisable.status != http.StatusOK || secondDisable.status != http.StatusOK || firstEntry.PublicationStatus != "disabled" || firstEntry.PublishedAt == nil || secondEntry.PublishedAt == nil || !firstEntry.PublishedAt.Equal(*secondEntry.PublishedAt) {
			t.Fatalf("idempotent disable = %#v / %#v", firstEntry, secondEntry)
		}
		afterDisable := decodeSearch(t, request(t, http.MethodGet, server.baseURL+"/v2/agents?capability=runtime.echo", userToken, nil))
		if len(afterDisable.Items) != 1 || afterDisable.Items[0].Card.AgentID != "runtime-b" {
			t.Fatalf("discovery after disable = %#v", afterDisable)
		}
	})

	t.Run("concurrent lifecycle has one legal final state", func(t *testing.T) {
		card := decodeCard(t, runtimeA)
		card.AgentID = "race-agent"
		if result := request(t, http.MethodPost, server.baseURL+"/v2/agents", ownerAToken, registrationEnvelope(t, mustJSON(t, card))); result.status != http.StatusCreated {
			t.Fatalf("register race Card = %d %s", result.status, result.body)
		}
		var wait sync.WaitGroup
		start := make(chan struct{})
		wait.Add(2)
		statuses := make(chan int, 2)
		go func() {
			defer wait.Done()
			<-start
			statuses <- request(t, http.MethodPost, server.baseURL+"/v2/agents/race-agent/versions/1.0.0/publish", ownerAToken, nil).status
		}()
		go func() {
			defer wait.Done()
			<-start
			statuses <- request(t, http.MethodPost, server.baseURL+"/v2/agents/race-agent/versions/1.0.0/disable", ownerAToken, nil).status
		}()
		close(start)
		wait.Wait()
		close(statuses)
		for status := range statuses {
			if status != http.StatusOK && status != http.StatusConflict {
				t.Fatalf("race status = %d", status)
			}
		}
		final := decodeEntry(t, request(t, http.MethodGet, server.baseURL+"/v2/agents/race-agent/versions/1.0.0", ownerAToken, nil))
		if final.PublicationStatus != "disabled" {
			t.Fatalf("race final state = %q", final.PublicationStatus)
		}
	})

	t.Run("concurrent duplicate registration is atomic", func(t *testing.T) {
		card := decodeCard(t, runtimeA)
		card.AgentID = "registration-race-agent"
		body := registrationEnvelope(t, mustJSON(t, card))
		const callers = 12
		start := make(chan struct{})
		statuses := make(chan int, callers)
		var wait sync.WaitGroup
		wait.Add(callers)
		for range callers {
			go func() {
				defer wait.Done()
				<-start
				statuses <- request(t, http.MethodPost, server.baseURL+"/v2/agents", ownerAToken, body).status
			}()
		}
		close(start)
		wait.Wait()
		close(statuses)
		created, conflicted := 0, 0
		for status := range statuses {
			switch status {
			case http.StatusCreated:
				created++
			case http.StatusConflict:
				conflicted++
			default:
				t.Fatalf("concurrent registration status = %d", status)
			}
		}
		if created != 1 || conflicted != callers-1 {
			t.Fatalf("concurrent registration outcomes = %d created, %d conflicted", created, conflicted)
		}
	})

	t.Run("restart and explicit dependency failure", func(t *testing.T) {
		previousServer := server
		previousServer.stop(t)
		assertLogsAreSecretSafe(t, previousServer.logs.String())
		server = startServer(t, root, databaseURL, binary)
		if result := request(t, http.MethodGet, server.baseURL+"/v2/agents/runtime-b/versions/1.0.0", userToken, nil); result.status != http.StatusOK {
			t.Fatalf("durable read after restart = %d %s", result.status, result.body)
		}
		boundaryRead := decodeEntry(t, request(t, http.MethodGet, server.baseURL+"/v2/agents/unbounded-number-agent/versions/1.0.0", userToken, nil))
		if got := boundaryRead.Card.Limits.MaxInputBytes.String(); got != "1e131072" {
			t.Fatalf("unbounded number after restart = %s", got)
		}
		boundaryDiscovery := decodeSearch(t, request(t, http.MethodGet, server.baseURL+"/v2/agents?capability=number.boundary", userToken, nil))
		if len(boundaryDiscovery.Items) != 1 || boundaryDiscovery.Items[0].Card.AgentID != "unbounded-number-agent" || boundaryDiscovery.Items[0].Card.Limits.MaxInputBytes.String() != "1e131072" {
			t.Fatalf("unbounded Discovery after restart = %#v", boundaryDiscovery)
		}

		if _, err := pool.Exec(ctx, `ALTER SCHEMA catalog RENAME TO catalog_unavailable`); err != nil {
			t.Fatal(err)
		}
		failure := request(t, http.MethodGet, server.baseURL+"/v2/agents?capability=runtime.echo", userToken, nil)
		assertPlatformError(t, failure, http.StatusServiceUnavailable, contracts.ErrorCodeDependency)
		if _, err := pool.Exec(ctx, `ALTER SCHEMA catalog_unavailable RENAME TO catalog`); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE catalog.schema_version SET version = 3`); err != nil {
			t.Fatal(err)
		}
		if ready := request(t, http.MethodGet, server.baseURL+"/readyz", "", nil); ready.status != http.StatusServiceUnavailable {
			t.Fatalf("schema mismatch readiness = %d", ready.status)
		}
		if _, err := pool.Exec(ctx, `UPDATE catalog.schema_version SET version = 2`); err != nil {
			t.Fatal(err)
		}
		var lastSequence int64
		if err := pool.QueryRow(ctx, `SELECT last_sequence FROM catalog.publication_clock WHERE singleton = true`).Scan(&lastSequence); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM catalog.publication_clock WHERE singleton = true`); err != nil {
			t.Fatal(err)
		}
		if ready := request(t, http.MethodGet, server.baseURL+"/readyz", "", nil); ready.status != http.StatusServiceUnavailable {
			t.Fatalf("missing clock readiness = %d", ready.status)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO catalog.publication_clock (singleton, last_sequence) VALUES (true, $1)`, lastSequence); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("stable cursor traversal and scale latency", func(t *testing.T) {
		seedPublishedVersions(t, pool, 0, 1000)
		registerClockRaceCards(t, server)
		clockTransaction, firstSequence := beginDelayedPublication(t, pool, "clock-race-a")
		secondPublication := make(chan httpResult, 1)
		go func() {
			secondPublication <- request(t, http.MethodPost, server.baseURL+"/v2/agents/clock-race-b/versions/1.0.0/publish", ownerAToken, nil)
		}()
		select {
		case result := <-secondPublication:
			t.Fatalf("competing publication bypassed transactional clock with status %d", result.status)
		case <-time.After(100 * time.Millisecond):
		}
		first := decodeSearch(t, request(t, http.MethodGet, server.baseURL+"/v2/agents?capability=scale.test&limit=100", userToken, nil))
		if len(first.Items) != 100 || first.NextCursor == nil {
			t.Fatalf("first scale page = %d items, cursor %v", len(first.Items), first.NextCursor)
		}
		mismatched := request(t, http.MethodGet, server.baseURL+"/v2/agents?capability=other.test&limit=100&cursor="+*first.NextCursor, userToken, nil)
		assertPlatformError(t, mismatched, http.StatusBadRequest, contracts.ErrorCodeValidationError)
		firstVersions := make([]string, len(first.Items))
		for index, item := range first.Items {
			firstVersions[index] = item.Card.Version
		}
		if !sort.StringsAreSorted(firstVersions) {
			t.Fatalf("tie ordering is not exact-string ascending: %v", firstVersions)
		}
		if err := clockTransaction.Commit(ctx); err != nil {
			t.Fatalf("commit delayed publication: %v", err)
		}
		second := <-secondPublication
		if second.status != http.StatusOK {
			t.Fatalf("competing publication after clock release = %d %s", second.status, second.body)
		}
		var secondSequence int64
		if err := pool.QueryRow(ctx, `SELECT publication_sequence FROM catalog.agent_versions WHERE agent_id = 'clock-race-b' AND version = '1.0.0'`).Scan(&secondSequence); err != nil {
			t.Fatal(err)
		}
		if secondSequence <= firstSequence {
			t.Fatalf("publication sequence order = delayed %d, competing %d", firstSequence, secondSequence)
		}
		seedLatePublication(t, pool)
		if _, err := pool.Exec(ctx, `UPDATE catalog.agent_versions SET publication_status = 'disabled', disabled_at = now() WHERE agent_id = 'scale-agent' AND version = '1.0.999'`); err != nil {
			t.Fatal(err)
		}
		seen := make(map[string]struct{}, 1000)
		for _, item := range first.Items {
			seen[item.Card.AgentID+"@"+item.Card.Version] = struct{}{}
		}
		cursor := first.NextCursor
		for cursor != nil {
			page := decodeSearch(t, request(t, http.MethodGet, server.baseURL+"/v2/agents?capability=scale.test&limit=100&cursor="+*cursor, userToken, nil))
			for _, item := range page.Items {
				key := item.Card.AgentID + "@" + item.Card.Version
				if _, exists := seen[key]; exists {
					t.Fatalf("duplicate traversal result %s", key)
				}
				seen[key] = struct{}{}
			}
			cursor = page.NextCursor
		}
		if len(seen) != 999 {
			t.Fatalf("stable traversal count = %d, want 999", len(seen))
		}
		if _, exists := seen["scale-agent@2.0.0"]; exists {
			t.Fatal("publication after first page entered traversal")
		}
		if _, exists := seen["scale-agent@1.0.999"]; exists {
			t.Fatal("between-page disablement remained in traversal")
		}
		if _, exists := seen["clock-race-a@1.0.0"]; exists {
			t.Fatal("delayed lower-sequence publication entered traversal")
		}
		if _, exists := seen["clock-race-b@1.0.0"]; exists {
			t.Fatal("publication serialized after delayed commit entered traversal")
		}

		seedPublishedVersions(t, pool, 1000, 9000)
		started := time.Now()
		page := request(t, http.MethodGet, server.baseURL+"/v2/agents?capability=scale.test&limit=100", userToken, nil)
		elapsed := time.Since(started)
		if page.status != http.StatusOK {
			t.Fatalf("10,000-version first page = %d %s", page.status, page.body)
		}
		if elapsed >= 500*time.Millisecond {
			t.Fatalf("10,000-version first page latency = %s, want <500ms", elapsed)
		}
	})
}

func guardedDatabaseURL(t *testing.T) string {
	t.Helper()
	value := os.Getenv("NEKIRO_TEST_DATABASE_URL")
	if strings.TrimSpace(value) == "" {
		t.Fatal("NEKIRO_TEST_DATABASE_URL is required for integration tests")
	}
	configuration, err := pgxpool.ParseConfig(value)
	if err != nil {
		t.Fatal("NEKIRO_TEST_DATABASE_URL is invalid")
	}
	if !strings.HasSuffix(configuration.ConnConfig.Database, "_test") {
		t.Fatalf("integration database %q must end in _test", configuration.ConnConfig.Database)
	}
	return value
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func resetCatalog(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS catalog CASCADE`); err != nil {
		t.Fatalf("reset dedicated Catalog schema: %v", err)
	}
}

func assertCatalogSchemaV2(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var version int
	var cardType string
	var nameRequired, descriptionRequired bool
	err := pool.QueryRow(context.Background(), `
SELECT sv.version,
       format_type(card.atttypid, card.atttypmod),
       name.attnotnull,
       description.attnotnull
FROM catalog.schema_version sv
JOIN pg_attribute card
  ON card.attrelid = 'catalog.agent_versions'::regclass
 AND card.attname = 'card'
 AND NOT card.attisdropped
JOIN pg_attribute name
  ON name.attrelid = card.attrelid
 AND name.attname = 'card_name'
 AND NOT name.attisdropped
JOIN pg_attribute description
  ON description.attrelid = card.attrelid
 AND description.attname = 'card_description'
 AND NOT description.attisdropped`).Scan(&version, &cardType, &nameRequired, &descriptionRequired)
	if err != nil {
		t.Fatalf("inspect Catalog schema v2: %v", err)
	}
	if version != 2 || cardType != "text" || !nameRequired || !descriptionRequired {
		t.Fatalf("Catalog schema = version %d, Card %s, name required %t, description required %t", version, cardType, nameRequired, descriptionRequired)
	}
}

func assertV1ToV2Migration(t *testing.T, databaseURL, root string, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect migration assertion database: %v", err)
	}
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, `CREATE SCHEMA catalog`); err != nil {
		t.Fatalf("create migration assertion schema: %v", err)
	}
	migrationFS := fstest.MapFS{}
	for _, filename := range []string{"001_catalog.sql", "002_card_text.sql"} {
		data, err := os.ReadFile(filepath.Join(root, "apps", "control-plane", "migrations", filename))
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		migrationFS[filename] = &fstest.MapFile{Data: data, Mode: 0o444}
	}
	migrator, err := migrate.NewMigrator(ctx, connection, "catalog.schema_version")
	if err != nil {
		t.Fatalf("initialize migration assertion: %v", err)
	}
	if err := migrator.LoadMigrations(migrationFS); err != nil {
		t.Fatalf("load migration assertion files: %v", err)
	}
	if err := migrator.MigrateTo(ctx, 1); err != nil {
		t.Fatalf("migrate assertion schema to v1: %v", err)
	}
	registeredAt := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
INSERT INTO catalog.agent_identities (agent_id, owner_id, created_at)
VALUES ('migration-agent', 'catalog-owner-a', $1)`, registeredAt); err != nil {
		t.Fatalf("seed v1 identity: %v", err)
	}
	card := `{"schemaVersion":"0.2","agentId":"migration-agent","name":"Migration Agent","description":"Existing v1 Card","owner":{"id":"catalog-owner-a"},"version":"1.0.0"}`
	cardDigest := sha256.Sum256([]byte(card))
	if _, err := pool.Exec(ctx, `
INSERT INTO catalog.agent_versions (
    agent_id, version, schema_version, card, card_digest,
    publication_status, registered_at
) VALUES ('migration-agent', '1.0.0', '0.2', $1, $2, 'draft', $3)`, card, cardDigest[:], registeredAt); err != nil {
		t.Fatalf("seed v1 Card: %v", err)
	}
	if err := migrator.MigrateTo(ctx, 2); err != nil {
		t.Fatalf("migrate assertion schema to v2: %v", err)
	}
	assertCatalogSchemaV2(t, pool)
	var storedCard, name, description string
	if err := pool.QueryRow(ctx, `
SELECT card, card_name, card_description
FROM catalog.agent_versions
WHERE agent_id = 'migration-agent' AND version = '1.0.0'`).Scan(&storedCard, &name, &description); err != nil {
		t.Fatalf("read migrated v1 Card: %v", err)
	}
	if name != "Migration Agent" || description != "Existing v1 Card" || !strings.Contains(storedCard, `"agentId": "migration-agent"`) {
		t.Fatalf("migrated v1 Card = %q, %q, %s", name, description, storedCard)
	}
	if err := migrator.MigrateTo(ctx, 0); err != nil {
		t.Fatalf("roll back migration assertion schema: %v", err)
	}
}

type migrationGuardSnapshot struct {
	schemaVersion       int
	ownerID             string
	identityCreatedAt   time.Time
	card                string
	cardName            string
	cardDescription     string
	cardDigest          string
	publicationStatus   string
	registeredAt        time.Time
	publishedAt         time.Time
	publicationSequence int64
	capabilityID        string
	clockSequence       int64
}

func assertUnsupportedMigrationLeavesPopulatedCatalog(t *testing.T, root, databaseURL, binary string, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	card := scaleCard("1.0.0")
	card.AgentID = "migration-guard-agent"
	card.Name = "Migration Guard Agent"
	card.Description = "Ordinary populated Catalog migration guard."
	card.Skills[0].ID = "migration.guard"
	card.Skills[0].Name = "Migration guard"
	cardJSON := mustJSON(t, card)
	cardDigest := sha256.Sum256(cardJSON)
	createdAt := time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE catalog.publication_clock SET last_sequence = 1 WHERE singleton = true`); err != nil {
		t.Fatalf("seed migration guard clock: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO catalog.agent_identities (agent_id, owner_id, created_at)
VALUES ($1, $2, $3)`, card.AgentID, card.Owner.ID, createdAt); err != nil {
		t.Fatalf("seed migration guard identity: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO catalog.agent_versions (
    agent_id, version, schema_version, card, card_name, card_description,
    card_digest, publication_status, registered_at, published_at, publication_sequence
) VALUES ($1, $2, $3, $4, $5, $6, $7, 'published', $8, $8, 1)`,
		card.AgentID, card.Version, card.SchemaVersion, string(cardJSON), card.Name,
		card.Description, cardDigest[:], createdAt); err != nil {
		t.Fatalf("seed migration guard version: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO catalog.agent_version_capabilities (agent_id, version, capability_id)
VALUES ($1, $2, $3)`, card.AgentID, card.Version, card.Skills[0].ID); err != nil {
		t.Fatalf("seed migration guard capability: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit migration guard data: %v", err)
	}

	before := readMigrationGuardSnapshot(t, pool)
	command := exec.Command(binary, "migrate", "down")
	command.Dir = root
	command.Env = environmentWith(map[string]string{"NEKIRO_DATABASE_URL": databaseURL})
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("unsupported migrate down succeeded: %s", output)
	}
	assertCatalogSchemaV2(t, pool)
	after := readMigrationGuardSnapshot(t, pool)
	if after != before {
		t.Fatalf("unsupported migrate down changed Catalog\nbefore: %#v\nafter:  %#v", before, after)
	}

	cleanup, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup.Rollback(ctx)
	if _, err := cleanup.Exec(ctx, `DELETE FROM catalog.agent_version_capabilities WHERE agent_id = $1`, card.AgentID); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanup.Exec(ctx, `DELETE FROM catalog.agent_versions WHERE agent_id = $1`, card.AgentID); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanup.Exec(ctx, `DELETE FROM catalog.agent_identities WHERE agent_id = $1`, card.AgentID); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanup.Exec(ctx, `UPDATE catalog.publication_clock SET last_sequence = 0 WHERE singleton = true`); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func readMigrationGuardSnapshot(t *testing.T, pool *pgxpool.Pool) migrationGuardSnapshot {
	t.Helper()
	var snapshot migrationGuardSnapshot
	err := pool.QueryRow(context.Background(), `
SELECT sv.version,
       i.owner_id,
       i.created_at,
       v.card,
       v.card_name,
       v.card_description,
       encode(v.card_digest, 'hex'),
       v.publication_status,
       v.registered_at,
       v.published_at,
       v.publication_sequence,
       c.capability_id,
       p.last_sequence
FROM catalog.schema_version sv
CROSS JOIN catalog.agent_identities i
JOIN catalog.agent_versions v ON v.agent_id = i.agent_id
JOIN catalog.agent_version_capabilities c
  ON c.agent_id = v.agent_id AND c.version = v.version
CROSS JOIN catalog.publication_clock p
WHERE i.agent_id = 'migration-guard-agent' AND p.singleton = true`).Scan(
		&snapshot.schemaVersion,
		&snapshot.ownerID,
		&snapshot.identityCreatedAt,
		&snapshot.card,
		&snapshot.cardName,
		&snapshot.cardDescription,
		&snapshot.cardDigest,
		&snapshot.publicationStatus,
		&snapshot.registeredAt,
		&snapshot.publishedAt,
		&snapshot.publicationSequence,
		&snapshot.capabilityID,
		&snapshot.clockSequence,
	)
	if err != nil {
		t.Fatalf("read migration guard snapshot: %v", err)
	}
	return snapshot
}

func buildControlPlane(t *testing.T, root string) string {
	t.Helper()
	name := "control-plane"
	if filepath.Separator == '\\' {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	command := exec.Command("go", "build", "-o", binary, "./apps/control-plane/cmd/control-plane")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Control Plane: %v\n%s", err, output)
	}
	return binary
}

func runCommand(t *testing.T, root, databaseURL, binary string, arguments ...string) {
	t.Helper()
	command := exec.Command(binary, arguments...)
	command.Dir = root
	command.Env = environmentWith(map[string]string{"NEKIRO_DATABASE_URL": databaseURL})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run %v: %v\n%s", arguments, err, output)
	}
}

func startServer(t *testing.T, root, databaseURL, binary string) *testServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	principals := []map[string]string{
		{"id": "catalog-owner-a", "tokenSha256": digest(ownerAToken)},
		{"id": "catalog-owner-b", "tokenSha256": digest(ownerBToken)},
		{"id": "catalog-user", "tokenSha256": digest(userToken)},
	}
	principalsJSON, err := json.Marshal(principals)
	if err != nil {
		t.Fatal(err)
	}
	server := &testServer{baseURL: "http://" + address}
	server.command = exec.Command(binary, "serve")
	server.command.Dir = root
	server.command.Env = environmentWith(map[string]string{
		"NEKIRO_DATABASE_URL":             databaseURL,
		"NEKIRO_LISTEN_ADDRESS":           address,
		"NEKIRO_AUTH_MODE":                "development-static",
		"NEKIRO_DEV_AUTH_PRINCIPALS_JSON": string(principalsJSON),
	})
	server.command.Stdout = &server.logs
	server.command.Stderr = &server.logs
	if err := server.command.Start(); err != nil {
		t.Fatalf("start Control Plane: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(server.baseURL + "/readyz")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusNoContent {
				return server
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	server.stop(t)
	t.Fatalf("Control Plane did not become ready: %s", server.logs.String())
	return nil
}

func (server *testServer) stop(t *testing.T) {
	t.Helper()
	if server == nil || server.command == nil || server.command.Process == nil || server.command.ProcessState != nil {
		return
	}
	if err := server.command.Process.Signal(os.Interrupt); err != nil {
		_ = server.command.Process.Kill()
	}
	done := make(chan error, 1)
	go func() { done <- server.command.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = server.command.Process.Kill()
		<-done
	}
	server.command = nil
}

func environmentWith(values map[string]string) []string {
	environment := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := values[strings.ToUpper(name)]; !replaced {
			keep := true
			for replacement := range values {
				if strings.EqualFold(name, replacement) {
					keep = false
					break
				}
			}
			if keep {
				environment = append(environment, entry)
			}
		}
	}
	for name, value := range values {
		environment = append(environment, name+"="+value)
	}
	return environment
}

func request(t *testing.T, method, url, token string, body []byte) httpResult {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return httpResult{status: response.StatusCode, header: response.Header.Clone(), body: data}
}

func assertPlatformError(t *testing.T, result httpResult, status int, code contracts.PlatformErrorCode) {
	t.Helper()
	if result.status != status {
		t.Fatalf("status = %d, want %d: %s", result.status, status, result.body)
	}
	var platformError contracts.PlatformError
	if err := json.Unmarshal(result.body, &platformError); err != nil {
		t.Fatalf("decode Platform Error: %v: %s", err, result.body)
	}
	if platformError.Code != code || string(platformError.TraceID) != result.header.Get("x-nek-trace-id") {
		t.Fatalf("Platform Error = %#v, header trace = %q", platformError, result.header.Get("x-nek-trace-id"))
	}
}

func decodeEntry(t *testing.T, result httpResult) contracts.CatalogEntry {
	t.Helper()
	var entry contracts.CatalogEntry
	decoder := json.NewDecoder(bytes.NewReader(result.body))
	decoder.UseNumber()
	if err := decoder.Decode(&entry); err != nil {
		t.Fatalf("decode Catalog entry: %v: %s", err, result.body)
	}
	return entry
}

func decodeSearch(t *testing.T, result httpResult) contracts.SearchAgentsResponse {
	t.Helper()
	if result.status != http.StatusOK {
		t.Fatalf("search status = %d: %s", result.status, result.body)
	}
	var response contracts.SearchAgentsResponse
	decoder := json.NewDecoder(bytes.NewReader(result.body))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode search: %v: %s", err, result.body)
	}
	return response
}

func decodeCard(t *testing.T, data []byte) contracts.AgentCard {
	t.Helper()
	var card contracts.AgentCard
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&card); err != nil {
		t.Fatal(err)
	}
	return card
}

func assertLogsAreSecretSafe(t *testing.T, logs string) {
	t.Helper()
	for _, forbidden := range []string{
		ownerAToken,
		ownerBToken,
		userToken,
		digest(ownerAToken),
		digest(ownerBToken),
		digest(userToken),
		"runtime-a.example.test",
		"inputSchema",
		"NEKIRO_DATABASE_URL",
	} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("Control Plane logs contain forbidden request or credential material %q", forbidden)
		}
	}
}

func readFixture(t *testing.T, root, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "tests", "fixtures", "catalog", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func registrationEnvelope(t *testing.T, card []byte) []byte {
	t.Helper()
	var document any
	decoder := json.NewDecoder(bytes.NewReader(card))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		t.Fatal(err)
	}
	return mustJSON(t, map[string]any{"card": document})
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func digest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func sameJSONNumber(left, right string) bool {
	leftNumber, _, leftErr := big.ParseFloat(left, 10, 2048, big.ToNearestEven)
	rightNumber, _, rightErr := big.ParseFloat(right, 10, 2048, big.ToNearestEven)
	return leftErr == nil && rightErr == nil && leftNumber.Cmp(rightNumber) == 0
}

func seedPublishedVersions(t *testing.T, pool *pgxpool.Pool, start, count int) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	registeredAt := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	if _, err := tx.Exec(ctx, `INSERT INTO catalog.agent_identities (agent_id, owner_id, created_at) VALUES ('scale-agent', 'catalog-owner-a', $1) ON CONFLICT DO NOTHING`, registeredAt); err != nil {
		t.Fatal(err)
	}
	var sequence int64
	if err := tx.QueryRow(ctx, `SELECT last_sequence FROM catalog.publication_clock WHERE singleton = true FOR UPDATE`).Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	versionRows := make([][]any, 0, count)
	capabilityRows := make([][]any, 0, count)
	for index := start; index < start+count; index++ {
		version := fmt.Sprintf("1.0.%d", index)
		card := scaleCard(version)
		cardJSON := mustJSON(t, card)
		cardDigest := sha256.Sum256(cardJSON)
		sequence++
		versionRows = append(versionRows, []any{"scale-agent", version, "0.2", string(cardJSON), card.Name, card.Description, cardDigest[:], "published", registeredAt, registeredAt, sequence, nil})
		capabilityRows = append(capabilityRows, []any{"scale-agent", version, "scale.test"})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"catalog", "agent_versions"}, []string{"agent_id", "version", "schema_version", "card", "card_name", "card_description", "card_digest", "publication_status", "registered_at", "published_at", "publication_sequence", "disabled_at"}, pgx.CopyFromRows(versionRows)); err != nil {
		t.Fatalf("seed scale versions: %v", err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"catalog", "agent_version_capabilities"}, []string{"agent_id", "version", "capability_id"}, pgx.CopyFromRows(capabilityRows)); err != nil {
		t.Fatalf("seed scale capabilities: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE catalog.publication_clock SET last_sequence = $1 WHERE singleton = true`, sequence); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func seedLatePublication(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	card := scaleCard("2.0.0")
	cardJSON := mustJSON(t, card)
	cardDigest := sha256.Sum256(cardJSON)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var sequence int64
	if err := tx.QueryRow(ctx, `UPDATE catalog.publication_clock SET last_sequence = last_sequence + 1 WHERE singleton = true RETURNING last_sequence`).Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO catalog.agent_versions (agent_id, version, schema_version, card, card_name, card_description, card_digest, publication_status, registered_at, published_at, publication_sequence)
VALUES ('scale-agent', '2.0.0', '0.2', $1, $2, $3, $4, 'published', now(), now(), $5)`, string(cardJSON), card.Name, card.Description, cardDigest[:], sequence); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO catalog.agent_version_capabilities (agent_id, version, capability_id) VALUES ('scale-agent', '2.0.0', 'scale.test')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func registerClockRaceCards(t *testing.T, server *testServer) {
	t.Helper()
	for _, agentID := range []string{"clock-race-a", "clock-race-b"} {
		card := scaleCard("1.0.0")
		card.AgentID = agentID
		result := request(t, http.MethodPost, server.baseURL+"/v2/agents", ownerAToken, registrationEnvelope(t, mustJSON(t, card)))
		if result.status != http.StatusCreated {
			t.Fatalf("register %s = %d %s", agentID, result.status, result.body)
		}
	}
}

func beginDelayedPublication(t *testing.T, pool *pgxpool.Pool, agentID string) (pgx.Tx, int64) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM catalog.agent_versions WHERE agent_id = $1 AND version = '1.0.0' FOR UPDATE`, agentID); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	var sequence int64
	if err := tx.QueryRow(ctx, `UPDATE catalog.publication_clock SET last_sequence = last_sequence + 1 WHERE singleton = true RETURNING last_sequence`).Scan(&sequence); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE catalog.agent_versions SET publication_status = 'published', published_at = now(), publication_sequence = $3 WHERE agent_id = $1 AND version = $2`, agentID, "1.0.0", sequence); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	return tx, sequence
}

func scaleCard(version string) contracts.AgentCard {
	return contracts.AgentCard{
		SchemaVersion: "0.2", AgentID: "scale-agent", Name: "Scale Agent", Description: "Cursor scale acceptance fixture.",
		Owner: contracts.AgentOwner{ID: "catalog-owner-a", DisplayName: "Catalog Owner A"}, Version: version,
		Protocol:       contracts.AgentProtocol{Type: "a2a", Version: "0.3.0", Transport: "JSONRPC", Endpoint: "https://scale.example.test/a2a"},
		Skills:         []contracts.AgentSkill{{ID: "scale.test", Name: "Scale", Description: "Scale test.", InputSchema: contracts.JSONSchema{"type": "object"}, OutputSchema: contracts.JSONSchema{"type": "object"}, RequiredPermissions: []string{}}},
		Authentication: contracts.AgentAuthentication{Type: "none"}, Permissions: []contracts.PermissionDeclaration{},
		Limits: contracts.AgentLimits{TimeoutMS: 1000, MaxInputBytes: json.Number("1024"), MaxOutputBytes: json.Number("1024"), Streaming: false},
	}
}
