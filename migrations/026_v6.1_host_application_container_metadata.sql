-- Keep host_application_assets aligned with the V6.1 container-aware model.
-- Existing host application assets are host processes unless container
-- evidence is collected later.

ALTER TABLE host_application_assets
    ADD COLUMN IF NOT EXISTS is_container BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS container_id VARCHAR(128),
    ADD COLUMN IF NOT EXISTS container_runtime VARCHAR(64);

COMMENT ON COLUMN host_application_assets.is_container IS
    'Whether the application asset runs inside a container';
COMMENT ON COLUMN host_application_assets.container_id IS
    'Container identifier derived from process cgroup evidence';
COMMENT ON COLUMN host_application_assets.container_runtime IS
    'Container runtime such as docker, containerd, cri-o, or podman';
