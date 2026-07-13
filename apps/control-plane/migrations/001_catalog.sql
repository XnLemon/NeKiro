CREATE SCHEMA IF NOT EXISTS catalog;

CREATE TABLE catalog.agent_identities (
    agent_id varchar(128) COLLATE "C" PRIMARY KEY,
    owner_id varchar(128) COLLATE "C" NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT agent_identities_agent_id_format CHECK (agent_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT agent_identities_owner_id_format CHECK (owner_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$')
);

CREATE INDEX agent_identities_owner_idx
    ON catalog.agent_identities (owner_id, agent_id);

CREATE TABLE catalog.publication_clock (
    singleton boolean PRIMARY KEY,
    last_sequence bigint NOT NULL,
    CONSTRAINT publication_clock_singleton CHECK (singleton),
    CONSTRAINT publication_clock_non_negative CHECK (last_sequence >= 0)
);

INSERT INTO catalog.publication_clock (singleton, last_sequence)
VALUES (true, 0);

CREATE TABLE catalog.agent_versions (
    agent_id varchar(128) COLLATE "C" NOT NULL,
    version text COLLATE "C" NOT NULL,
    schema_version varchar(16) NOT NULL,
    card jsonb NOT NULL,
    card_digest bytea NOT NULL,
    publication_status varchar(16) NOT NULL,
    registered_at timestamptz NOT NULL,
    published_at timestamptz,
    publication_sequence bigint,
    disabled_at timestamptz,
    PRIMARY KEY (agent_id, version),
    CONSTRAINT agent_versions_identity_fk
        FOREIGN KEY (agent_id) REFERENCES catalog.agent_identities (agent_id),
    CONSTRAINT agent_versions_agent_id_format CHECK (agent_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT agent_versions_schema_version CHECK (schema_version = '0.2'),
    CONSTRAINT agent_versions_card_digest_length CHECK (octet_length(card_digest) = 32),
    CONSTRAINT agent_versions_publication_status CHECK (publication_status IN ('draft', 'published', 'disabled')),
    CONSTRAINT agent_versions_state_timestamps CHECK (
        (publication_status = 'draft'
            AND published_at IS NULL
            AND publication_sequence IS NULL
            AND disabled_at IS NULL)
        OR
        (publication_status = 'published'
            AND published_at IS NOT NULL
            AND publication_sequence IS NOT NULL
            AND disabled_at IS NULL)
        OR
        (publication_status = 'disabled'
            AND disabled_at IS NOT NULL
            AND ((published_at IS NULL AND publication_sequence IS NULL)
                OR (published_at IS NOT NULL AND publication_sequence IS NOT NULL)))
    )
);

CREATE UNIQUE INDEX agent_versions_publication_sequence_idx
    ON catalog.agent_versions (publication_sequence)
    WHERE publication_sequence IS NOT NULL;

CREATE INDEX agent_versions_published_order_idx
    ON catalog.agent_versions (published_at DESC, agent_id, version)
    WHERE publication_status = 'published';

CREATE TABLE catalog.agent_version_capabilities (
    agent_id varchar(128) COLLATE "C" NOT NULL,
    version text COLLATE "C" NOT NULL,
    capability_id varchar(128) COLLATE "C" NOT NULL,
    PRIMARY KEY (agent_id, version, capability_id),
    CONSTRAINT agent_version_capabilities_version_fk
        FOREIGN KEY (agent_id, version)
        REFERENCES catalog.agent_versions (agent_id, version),
    CONSTRAINT agent_version_capabilities_id_format
        CHECK (capability_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$')
);

CREATE INDEX agent_version_capabilities_lookup_idx
    ON catalog.agent_version_capabilities (capability_id, agent_id, version);

---- create above / drop below ----

DROP TABLE catalog.agent_version_capabilities;
DROP TABLE catalog.agent_versions;
DROP TABLE catalog.agent_identities;
DROP TABLE catalog.publication_clock;
