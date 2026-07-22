ALTER TABLE catalog.agent_versions
    ADD COLUMN legacy_unverified boolean NOT NULL DEFAULT false;

UPDATE catalog.agent_versions
SET legacy_unverified = true
WHERE publication_status = 'published';

ALTER TABLE catalog.agent_versions
    ALTER COLUMN legacy_unverified DROP DEFAULT;

CREATE TABLE catalog.agent_releases (
    release_id varchar(128) COLLATE "C" PRIMARY KEY,
    provider_id varchar(128) COLLATE "C" NOT NULL,
    agent_id varchar(128) COLLATE "C" NOT NULL,
    agent_card_version text COLLATE "C" NOT NULL,
    card_digest bytea NOT NULL,
    endpoint_binding_id varchar(128) COLLATE "C" NOT NULL,
    endpoint_origin text NOT NULL,
    endpoint_path text NOT NULL,
    verification_method varchar(64) NOT NULL,
    verification_evidence_digest bytea,
    state varchar(32) COLLATE "C" NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    verified_at timestamptz,
    published_at timestamptz,
    suspended_at timestamptz,
    revoked_at timestamptz,
    CONSTRAINT agent_releases_provider_fk FOREIGN KEY (provider_id) REFERENCES catalog.providers(provider_id),
    CONSTRAINT agent_releases_card_fk FOREIGN KEY (agent_id, agent_card_version) REFERENCES catalog.agent_versions(agent_id, version),
    CONSTRAINT agent_releases_binding_fk FOREIGN KEY (endpoint_binding_id) REFERENCES catalog.endpoint_bindings(binding_id),
    CONSTRAINT agent_releases_release_id_format CHECK (release_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT agent_releases_provider_id_format CHECK (provider_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT agent_releases_agent_id_format CHECK (agent_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    CONSTRAINT agent_releases_card_digest_length CHECK (octet_length(card_digest) = 32),
    CONSTRAINT agent_releases_evidence_digest_length CHECK (verification_evidence_digest IS NULL OR octet_length(verification_evidence_digest) = 32),
    CONSTRAINT agent_releases_state CHECK (state IN ('draft', 'pending_verification', 'verified', 'published', 'suspended', 'revoked')),
    CONSTRAINT agent_releases_method CHECK (verification_method = 'http_well_known'),
    CONSTRAINT agent_releases_timestamp_order CHECK (created_at <= updated_at),
    CONSTRAINT agent_releases_state_timestamps CHECK (
        (state IN ('draft', 'pending_verification') AND verification_evidence_digest IS NULL AND verified_at IS NULL AND published_at IS NULL AND suspended_at IS NULL AND revoked_at IS NULL)
        OR (state = 'verified' AND verification_evidence_digest IS NOT NULL AND verified_at IS NOT NULL AND published_at IS NULL AND suspended_at IS NULL AND revoked_at IS NULL)
        OR (state = 'published' AND verification_evidence_digest IS NOT NULL AND verified_at IS NOT NULL AND published_at IS NOT NULL AND suspended_at IS NULL AND revoked_at IS NULL)
        OR (state = 'suspended' AND verification_evidence_digest IS NOT NULL AND verified_at IS NOT NULL AND suspended_at IS NOT NULL AND revoked_at IS NULL)
        OR (state = 'revoked' AND verification_evidence_digest IS NOT NULL AND verified_at IS NOT NULL AND revoked_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX agent_releases_agent_version_idx
    ON catalog.agent_releases (agent_id, agent_card_version);

CREATE INDEX agent_releases_provider_state_idx
    ON catalog.agent_releases (provider_id, state, created_at DESC);

CREATE FUNCTION catalog.reject_agent_release_bound_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.release_id <> OLD.release_id
       OR NEW.provider_id <> OLD.provider_id
       OR NEW.agent_id <> OLD.agent_id
       OR NEW.agent_card_version <> OLD.agent_card_version
       OR NEW.card_digest <> OLD.card_digest
       OR NEW.endpoint_binding_id <> OLD.endpoint_binding_id
       OR NEW.endpoint_origin <> OLD.endpoint_origin
       OR NEW.endpoint_path <> OLD.endpoint_path
       OR NEW.verification_method <> OLD.verification_method
       OR (NEW.verification_evidence_digest IS DISTINCT FROM OLD.verification_evidence_digest
           AND NOT (OLD.state = 'pending_verification'
                    AND NEW.state = 'verified'
                    AND OLD.verification_evidence_digest IS NULL
                    AND NEW.verification_evidence_digest IS NOT NULL))
       OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'Agent Release bound facts are immutable' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER agent_releases_bound_immutable
BEFORE UPDATE ON catalog.agent_releases
FOR EACH ROW EXECUTE FUNCTION catalog.reject_agent_release_bound_mutation();

---- create above / drop below ----

DROP TRIGGER agent_releases_bound_immutable ON catalog.agent_releases;
DROP FUNCTION catalog.reject_agent_release_bound_mutation();
DROP INDEX catalog.agent_releases_provider_state_idx;
DROP INDEX catalog.agent_releases_agent_version_idx;
DROP TABLE catalog.agent_releases;

ALTER TABLE catalog.agent_versions
    DROP COLUMN legacy_unverified;
