CREATE SCHEMA IF NOT EXISTS workspace;

CREATE TABLE workspace.workspaces (
    workspace_id varchar(128) COLLATE "C" PRIMARY KEY,
    owner_id varchar(128) COLLATE "C" NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT workspaces_id_format CHECK (workspace_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT workspaces_owner_format CHECK (owner_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT workspaces_timestamp_order CHECK (created_at <= updated_at)
);

CREATE TABLE workspace.installations (
    installation_id varchar(128) COLLATE "C" PRIMARY KEY,
    workspace_id varchar(128) COLLATE "C" NOT NULL,
    agent_id varchar(128) COLLATE "C" NOT NULL,
    version_constraint text COLLATE "C" NOT NULL,
    installed_version text COLLATE "C" NOT NULL,
    accepted_permissions text[] NOT NULL,
    status varchar(16) COLLATE "C" NOT NULL,
    installed_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    uninstalled_at timestamptz,
    CONSTRAINT installations_workspace_fk
        FOREIGN KEY (workspace_id) REFERENCES workspace.workspaces (workspace_id),
    CONSTRAINT installations_id_format CHECK (installation_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT installations_workspace_id_format CHECK (workspace_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT installations_agent_id_format CHECK (agent_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT installations_status CHECK (status IN ('enabled', 'disabled', 'uninstalled')),
    CONSTRAINT installations_timestamp_order CHECK (installed_at <= updated_at),
    CONSTRAINT installations_state_timestamps CHECK (
        (status IN ('enabled', 'disabled') AND uninstalled_at IS NULL)
        OR (status = 'uninstalled' AND uninstalled_at IS NOT NULL AND uninstalled_at = updated_at)
    )
);

CREATE UNIQUE INDEX installations_current_agent_idx
    ON workspace.installations (workspace_id, agent_id)
    WHERE status <> 'uninstalled';

CREATE INDEX installations_workspace_order_idx
    ON workspace.installations (workspace_id, installed_at ASC, installation_id ASC);

---- create above / drop below ----

DROP TABLE workspace.installations;
DROP TABLE workspace.workspaces;
