ALTER TABLE ledger.invocations
    ADD COLUMN agent_release_id varchar(128) COLLATE "C",
    ADD COLUMN agent_card_digest bytea,
    ADD CONSTRAINT invocations_release_id_format
        CHECK (agent_release_id IS NULL OR agent_release_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    ADD CONSTRAINT invocations_release_digest_length
        CHECK (agent_card_digest IS NULL OR octet_length(agent_card_digest) = 32),
    ADD CONSTRAINT invocations_release_provenance_pair
        CHECK ((agent_release_id IS NULL) = (agent_card_digest IS NULL));

ALTER TABLE ledger.invocation_events
    ADD COLUMN agent_release_id varchar(128) COLLATE "C",
    ADD COLUMN agent_card_digest bytea,
    ADD CONSTRAINT invocation_events_release_id_format
        CHECK (agent_release_id IS NULL OR agent_release_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
    ADD CONSTRAINT invocation_events_release_digest_length
        CHECK (agent_card_digest IS NULL OR octet_length(agent_card_digest) = 32),
    ADD CONSTRAINT invocation_events_release_provenance_pair
        CHECK ((agent_release_id IS NULL) = (agent_card_digest IS NULL));

---- create above / drop below ----

ALTER TABLE ledger.invocation_events
    DROP CONSTRAINT invocation_events_release_provenance_pair,
    DROP CONSTRAINT invocation_events_release_digest_length,
    DROP CONSTRAINT invocation_events_release_id_format,
    DROP COLUMN agent_card_digest,
    DROP COLUMN agent_release_id;

ALTER TABLE ledger.invocations
    DROP CONSTRAINT invocations_release_provenance_pair,
    DROP CONSTRAINT invocations_release_digest_length,
    DROP CONSTRAINT invocations_release_id_format,
    DROP COLUMN agent_card_digest,
    DROP COLUMN agent_release_id;
