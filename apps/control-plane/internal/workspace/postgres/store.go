package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace"
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
		return nil, errors.New("workspace database pool is required")
	}
	return &Store{pool: pool}, nil
}

func (store *Store) CreateWorkspace(ctx context.Context, value contracts.Workspace) (result contracts.Workspace, returnErr error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return contracts.Workspace{}, dependencyError("begin workspace creation", err)
	}
	defer rollback(ctx, tx, &returnErr, "workspace creation")
	if err = tx.QueryRow(ctx, `
INSERT INTO workspace.workspaces (workspace_id, owner_id, created_at, updated_at)
VALUES ($1, $2, $3, $4)
RETURNING workspace_id, owner_id, created_at, updated_at`, value.WorkspaceID, value.OwnerID, value.CreatedAt, value.UpdatedAt).Scan(
		&result.WorkspaceID, &result.OwnerID, &result.CreatedAt, &result.UpdatedAt,
	); err != nil {
		if isUniqueViolation(err) {
			return contracts.Workspace{}, workspace.ErrConflict
		}
		return contracts.Workspace{}, dependencyError("insert Workspace", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return contracts.Workspace{}, dependencyError("commit Workspace", err)
	}
	return result, nil
}

func (store *Store) GetWorkspace(ctx context.Context, workspaceID string) (contracts.Workspace, error) {
	var value contracts.Workspace
	err := store.pool.QueryRow(ctx, `
SELECT workspace_id, owner_id, created_at, updated_at
FROM workspace.workspaces WHERE workspace_id = $1`, workspaceID).Scan(
		&value.WorkspaceID, &value.OwnerID, &value.CreatedAt, &value.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Workspace{}, workspace.ErrNotFound
	}
	if err != nil {
		return contracts.Workspace{}, dependencyError("read Workspace", err)
	}
	return value, nil
}

func (store *Store) HasCurrentInstallation(ctx context.Context, workspaceID, agentID string) (bool, error) {
	var exists bool
	if err := store.pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM workspace.installations
  WHERE workspace_id = $1 AND agent_id = $2 AND status <> 'uninstalled'
)`, workspaceID, agentID).Scan(&exists); err != nil {
		return false, dependencyError("check current Installation", err)
	}
	return exists, nil
}

func (store *Store) CreateInstallation(ctx context.Context, callerID string, value contracts.Installation) (result contracts.Installation, returnErr error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return contracts.Installation{}, dependencyError("begin Installation creation", err)
	}
	defer rollback(ctx, tx, &returnErr, "Installation creation")
	var ownerID string
	if err = tx.QueryRow(ctx, `
SELECT owner_id FROM workspace.workspaces WHERE workspace_id = $1 FOR UPDATE`, value.WorkspaceID).Scan(&ownerID); errors.Is(err, pgx.ErrNoRows) {
		return contracts.Installation{}, workspace.ErrNotFound
	} else if err != nil {
		return contracts.Installation{}, dependencyError("lock Workspace for Installation", err)
	}
	if ownerID != callerID {
		return contracts.Installation{}, workspace.ErrForbidden
	}
	var current bool
	if err = tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM workspace.installations
  WHERE workspace_id = $1 AND agent_id = $2 AND status <> 'uninstalled'
)`, value.WorkspaceID, value.AgentID).Scan(&current); err != nil {
		return contracts.Installation{}, dependencyError("check Installation uniqueness", err)
	}
	if current {
		return contracts.Installation{}, workspace.ErrConflict
	}
	if err = tx.QueryRow(ctx, `
INSERT INTO workspace.installations (
  installation_id, workspace_id, agent_id, version_constraint, installed_version,
  accepted_permissions, status, installed_at, updated_at, uninstalled_at,
  installed_release_id
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING installation_id, workspace_id, agent_id, version_constraint, installed_version,
          accepted_permissions, status, installed_at, updated_at, uninstalled_at,
          COALESCE(installed_release_id, '')`,
		value.InstallationID, value.WorkspaceID, value.AgentID, value.VersionConstraint,
		value.InstalledVersion, value.AcceptedPermissions, value.Status, value.InstalledAt,
		value.UpdatedAt, value.UninstalledAt, nullableReleaseID(value.InstalledReleaseID)).Scan(
		&result.InstallationID, &result.WorkspaceID, &result.AgentID, &result.VersionConstraint,
		&result.InstalledVersion, &result.AcceptedPermissions, &result.Status, &result.InstalledAt,
		&result.UpdatedAt, &result.UninstalledAt, &result.InstalledReleaseID); err != nil {
		if isUniqueViolation(err) {
			return contracts.Installation{}, workspace.ErrConflict
		}
		return contracts.Installation{}, dependencyError("insert Installation", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return contracts.Installation{}, dependencyError("commit Installation", err)
	}
	return result, nil
}

func (store *Store) GetInstallation(ctx context.Context, workspaceID, installationID string) (contracts.Installation, error) {
	return store.getInstallation(ctx, `WHERE workspace_id = $1 AND installation_id = $2`, workspaceID, installationID)
}

func (store *Store) GetCurrentInstallation(ctx context.Context, workspaceID, agentID string) (contracts.Installation, error) {
	return store.getInstallation(ctx, `WHERE workspace_id = $1 AND agent_id = $2 AND status <> 'uninstalled'`, workspaceID, agentID)
}

func (store *Store) getInstallation(ctx context.Context, where string, args ...any) (contracts.Installation, error) {
	var value contracts.Installation
	err := store.pool.QueryRow(ctx, `
SELECT installation_id, workspace_id, agent_id, version_constraint, installed_version,
       accepted_permissions, status, installed_at, updated_at, uninstalled_at,
       COALESCE(installed_release_id, '')
FROM workspace.installations `+where, args...).Scan(
		&value.InstallationID, &value.WorkspaceID, &value.AgentID, &value.VersionConstraint,
		&value.InstalledVersion, &value.AcceptedPermissions, &value.Status, &value.InstalledAt,
		&value.UpdatedAt, &value.UninstalledAt, &value.InstalledReleaseID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Installation{}, workspace.ErrNotFound
	}
	if err != nil {
		return contracts.Installation{}, dependencyError("read Installation", err)
	}
	return value, nil
}

func (store *Store) ListInstallations(ctx context.Context, workspaceID string, limit int, after *workspace.InstallationPosition) ([]contracts.Installation, bool, error) {
	var afterTime any
	var afterID any
	if after != nil {
		afterTime, afterID = after.InstalledAt, after.InstallationID
	}
	rows, err := store.pool.Query(ctx, `
SELECT installation_id, workspace_id, agent_id, version_constraint, installed_version,
       accepted_permissions, status, installed_at, updated_at, uninstalled_at,
       COALESCE(installed_release_id, '')
FROM workspace.installations
WHERE workspace_id = $1
  AND ($2::timestamptz IS NULL OR installed_at > $2 OR (installed_at = $2 AND installation_id > $3))
ORDER BY installed_at ASC, installation_id ASC
LIMIT $4`, workspaceID, afterTime, afterID, limit+1)
	if err != nil {
		return nil, false, dependencyError("list Installations", err)
	}
	defer rows.Close()
	items := make([]contracts.Installation, 0, limit+1)
	for rows.Next() {
		value, err := scanInstallation(rows)
		if err != nil {
			return nil, false, dependencyError("scan Installation list", err)
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		return nil, false, dependencyError("read Installation list", err)
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

func (store *Store) ChangeInstallationStatus(ctx context.Context, workspaceID, installationID, status string, at time.Time) (result contracts.Installation, returnErr error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return contracts.Installation{}, dependencyError("begin Installation transition", err)
	}
	defer rollback(ctx, tx, &returnErr, "Installation transition")
	before, err := scanInstallation(tx.QueryRow(ctx, `
SELECT installation_id, workspace_id, agent_id, version_constraint, installed_version,
       accepted_permissions, status, installed_at, updated_at, uninstalled_at,
       COALESCE(installed_release_id, '')
FROM workspace.installations
WHERE workspace_id = $1 AND installation_id = $2 FOR UPDATE`, workspaceID, installationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Installation{}, workspace.ErrNotFound
	}
	if err != nil {
		return contracts.Installation{}, dependencyError("lock Installation transition", err)
	}
	if before.Status == "uninstalled" || before.Status == status {
		return contracts.Installation{}, workspace.ErrConflict
	}
	if (before.Status != "enabled" || status != "disabled") && (before.Status != "disabled" || status != "enabled") {
		return contracts.Installation{}, workspace.ErrConflict
	}
	committedAt := nextLifecycleTime(before.UpdatedAt, at)
	after := before
	after.Status = status
	after.UpdatedAt = committedAt
	if err := contracts.ValidateInstallationImmutablePin(before, after); err != nil {
		return contracts.Installation{}, dependencyError("validate immutable Installation pin", err)
	}
	if err = tx.QueryRow(ctx, `
UPDATE workspace.installations SET status = $3, updated_at = $4
WHERE workspace_id = $1 AND installation_id = $2
RETURNING installation_id, workspace_id, agent_id, version_constraint, installed_version,
          accepted_permissions, status, installed_at, updated_at, uninstalled_at,
          COALESCE(installed_release_id, '')`,
		workspaceID, installationID, status, committedAt).Scan(
		&after.InstallationID, &after.WorkspaceID, &after.AgentID, &after.VersionConstraint,
		&after.InstalledVersion, &after.AcceptedPermissions, &after.Status, &after.InstalledAt,
		&after.UpdatedAt, &after.UninstalledAt, &after.InstalledReleaseID); err != nil {
		return contracts.Installation{}, dependencyError("update Installation status", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return contracts.Installation{}, dependencyError("commit Installation transition", err)
	}
	return after, nil
}

func (store *Store) UninstallInstallation(ctx context.Context, workspaceID, installationID string, at time.Time) (result contracts.Installation, returnErr error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return contracts.Installation{}, dependencyError("begin Installation uninstall", err)
	}
	defer rollback(ctx, tx, &returnErr, "Installation uninstall")
	before, err := scanInstallation(tx.QueryRow(ctx, `
SELECT installation_id, workspace_id, agent_id, version_constraint, installed_version,
       accepted_permissions, status, installed_at, updated_at, uninstalled_at,
       COALESCE(installed_release_id, '')
FROM workspace.installations
WHERE workspace_id = $1 AND installation_id = $2 FOR UPDATE`, workspaceID, installationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return contracts.Installation{}, workspace.ErrNotFound
	}
	if err != nil {
		return contracts.Installation{}, dependencyError("lock Installation uninstall", err)
	}
	if before.Status != "disabled" {
		return contracts.Installation{}, workspace.ErrConflict
	}
	committedAt := nextLifecycleTime(before.UpdatedAt, at)
	after := before
	after.Status = "uninstalled"
	after.UpdatedAt = committedAt
	after.UninstalledAt = &committedAt
	if err := contracts.ValidateInstallationImmutablePin(before, after); err != nil {
		return contracts.Installation{}, dependencyError("validate immutable Installation pin", err)
	}
	if err = tx.QueryRow(ctx, `
UPDATE workspace.installations SET status = 'uninstalled', updated_at = $3, uninstalled_at = $3
WHERE workspace_id = $1 AND installation_id = $2
RETURNING installation_id, workspace_id, agent_id, version_constraint, installed_version,
          accepted_permissions, status, installed_at, updated_at, uninstalled_at,
          COALESCE(installed_release_id, '')`,
		workspaceID, installationID, committedAt).Scan(
		&after.InstallationID, &after.WorkspaceID, &after.AgentID, &after.VersionConstraint,
		&after.InstalledVersion, &after.AcceptedPermissions, &after.Status, &after.InstalledAt,
		&after.UpdatedAt, &after.UninstalledAt, &after.InstalledReleaseID); err != nil {
		return contracts.Installation{}, dependencyError("uninstall Installation", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return contracts.Installation{}, dependencyError("commit Installation uninstall", err)
	}
	return after, nil
}

func nextLifecycleTime(previous, candidate time.Time) time.Time {
	previous = previous.UTC().Truncate(time.Microsecond)
	candidate = candidate.UTC().Truncate(time.Microsecond)
	if !candidate.After(previous) {
		return previous.Add(time.Microsecond)
	}
	return candidate
}

func scanInstallation(row interface{ Scan(...any) error }) (contracts.Installation, error) {
	var value contracts.Installation
	err := row.Scan(&value.InstallationID, &value.WorkspaceID, &value.AgentID, &value.VersionConstraint,
		&value.InstalledVersion, &value.AcceptedPermissions, &value.Status, &value.InstalledAt,
		&value.UpdatedAt, &value.UninstalledAt, &value.InstalledReleaseID)
	return value, err
}

func nullableReleaseID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (store *Store) Check(ctx context.Context) error { return CheckSchema(ctx, store.pool) }

func rollback(ctx context.Context, tx pgx.Tx, returnErr *error, operation string) {
	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
		*returnErr = errors.Join(*returnErr, dependencyError("rollback "+operation, rollbackErr))
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func dependencyError(operation string, err error) error {
	return fmt.Errorf("%s: %w: %v", operation, workspace.ErrDependency, err)
}
