-- V6.1 container metadata for application assets.

ALTER TABLE host_application_assets
    ADD COLUMN IF NOT EXISTS is_container BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS container_id VARCHAR(128),
    ADD COLUMN IF NOT EXISTS container_runtime VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_host_application_assets_container
    ON host_application_assets(is_container, container_runtime);
