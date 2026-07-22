ALTER TABLE workspace.installations
    ADD COLUMN installed_release_id varchar(128) COLLATE "C",
    ADD CONSTRAINT installations_release_id_format
        CHECK (installed_release_id IS NULL OR installed_release_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$');

---- create above / drop below ----

ALTER TABLE workspace.installations
    DROP CONSTRAINT installations_release_id_format,
    DROP COLUMN installed_release_id;
