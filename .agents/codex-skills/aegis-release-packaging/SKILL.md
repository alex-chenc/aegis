---
name: aegis-release-packaging
description: Use for Aegis offline deployment package creation, Docker image export, release zip assembly, database init SQL packaging, start.sh generation, and MinIO agent artifact preloading.
version: 1.0.0
source: migrated-from-claude
---

# Aegis Release Packaging

Use this skill when building the offline Linux deployment package. The target is a
zip that can be copied to a Docker-capable Linux host and started with one script.

## Package Layout

```text
release/
└── {version}/
    ├── images/
    │   ├── api-server.tar.gz
    │   ├── server.tar.gz
    │   ├── dc.tar.gz
    │   ├── frontend.tar.gz
    │   ├── postgres.tar.gz
    │   ├── redis.tar.gz
    │   ├── kafka.tar.gz
    │   ├── zookeeper.tar.gz
    │   ├── minio-with-agent.tar.gz
    │   └── ...
    ├── build-context/
    │   ├── aegis-agent-linux-amd64
    │   ├── aegis-agent-linux-amd64.tar.gz
    │   ├── bpf/
    │   └── minio-entrypoint.sh
    ├── backend/scripts/init.sql
    ├── docker-compose.yml
    ├── .env.example
    ├── start.sh
    └── README.md
```

## Core Service Order

```text
postgres -> redis -> minio -> kafka/zookeeper -> server -> api-server -> dc -> frontend
```

Agent event flow:

```text
Agent -> Server -> Kafka aegis.security.events -> DC -> PostgreSQL -> WebSocket -> Frontend
```

## Build Directory

```bash
VERSION=v5.5
RELEASE_DIR=/code/ai-benchmark/release/${VERSION}
mkdir -p ${RELEASE_DIR}/{images,build-context/bpf,backend/scripts}
```

## Build And Package Agent

```bash
cd agent
make bpf
make build
```

```bash
cp agent/bin/agent-linux-amd64 ${RELEASE_DIR}/build-context/aegis-agent-linux-amd64
cp agent/internal/ebpf/*.bpf.o ${RELEASE_DIR}/build-context/bpf/
tar -czvf ${RELEASE_DIR}/build-context/aegis-agent-linux-amd64.tar.gz \
  -C ${RELEASE_DIR}/build-context \
  aegis-agent-linux-amd64 bpf/
```

## Build Docker Images

```bash
docker build -f api-server/Dockerfile -t aegis-system/api-server:latest api-server/
docker build -f server/Dockerfile -t aegis-system/server:latest server/
docker build -f dc/Dockerfile -t aegis-system/dc:latest dc/
docker build -f frontend/Dockerfile -t aegis-system/frontend:latest frontend/
docker pull postgres:14-alpine
docker pull redis:7-alpine
docker pull confluentinc/cp-kafka:7.5.0
docker pull confluentinc/cp-zookeeper:7.5.0
docker pull minio/minio:latest
docker pull minio/mc:latest
```

## MinIO With Preloaded Agent

The agent installer downloads from MinIO, so the offline package must preload the
agent artifact. The object name must be `aegis-agent.tar.gz`.

Create `${RELEASE_DIR}/build-context/minio-entrypoint.sh`:

```bash
#!/bin/bash
set -e

mc alias set myminio http://localhost:9000 ${MINIO_ROOT_USER} ${MINIO_ROOT_PASSWORD} 2>/dev/null || true
mc mb myminio/aegis-templates --ignore-existing 2>/dev/null || true
mc mb myminio/agent-artifacts --ignore-existing 2>/dev/null || true
mc mb myminio/generated-scripts --ignore-existing 2>/dev/null || true
mc anonymous set download myminio/agent-artifacts 2>/dev/null || true

if [ -f /agent-artifacts/aegis-agent-linux-amd64.tar.gz ]; then
  mc cp /agent-artifacts/aegis-agent-linux-amd64.tar.gz myminio/agent-artifacts/aegis-agent.tar.gz
fi

exec minio server /data --console-address ":9001" "$@"
```

Create `${RELEASE_DIR}/Dockerfile.minio`:

```dockerfile
FROM minio/minio:latest
COPY build-context/aegis-agent-linux-amd64.tar.gz /agent-artifacts/
COPY build-context/minio-entrypoint.sh /usr/bin/minio-entrypoint.sh
RUN chmod +x /usr/bin/minio-entrypoint.sh
ENTRYPOINT ["/usr/bin/minio-entrypoint.sh"]
CMD ["server", "/data", "--console-address", ":9001"]
```

Build:

```bash
docker build -f ${RELEASE_DIR}/Dockerfile.minio -t aegis-system/minio-with-agent:latest ${RELEASE_DIR}
```

## Export Images

```bash
cd ${RELEASE_DIR}/images
docker save aegis-system/api-server:latest | gzip > api-server.tar.gz
docker save aegis-system/server:latest | gzip > server.tar.gz
docker save aegis-system/dc:latest | gzip > dc.tar.gz
docker save aegis-system/frontend:latest | gzip > frontend.tar.gz
docker save aegis-system/minio-with-agent:latest | gzip > minio-with-agent.tar.gz
docker save postgres:14-alpine | gzip > postgres.tar.gz
docker save redis:7-alpine | gzip > redis.tar.gz
docker save confluentinc/cp-kafka:7.5.0 | gzip > kafka.tar.gz
docker save confluentinc/cp-zookeeper:7.5.0 | gzip > zookeeper.tar.gz
docker save minio/minio:latest | gzip > minio.tar.gz
docker save minio/mc:latest | gzip > minio-mc.tar.gz
```

## Database Init SQL

`backend/scripts/init.sql` must include all tables and fields expected by GORM
models. The `runtime_events.process_name` field is easy to miss and required by
DC models.

```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS runtime_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id VARCHAR(64) NOT NULL UNIQUE,
  host_id UUID NOT NULL,
  event_type VARCHAR(32) NOT NULL,
  event_data JSONB NOT NULL DEFAULT '{}',
  matched_rule_id VARCHAR(128),
  rule_title VARCHAR(255),
  mitre_id VARCHAR(20),
  severity VARCHAR(16),
  pid INTEGER DEFAULT 0,
  command_line TEXT,
  process_name VARCHAR(255),
  timestamp BIGINT NOT NULL,
  created_at TIMESTAMP DEFAULT NOW(),
  aggregated BOOLEAN DEFAULT FALSE
);
```

Cross-check table definitions against `dc/internal/model/` and the database
design docs in `docs/aegis_system_design_v5.5/`.

## docker-compose.yml Requirements

Key points:

- `api-server` must depend on healthy `postgres`, `redis`, `minio`, `server`,
  and `kafka`.
- `minio-init` or the custom MinIO image must upload
  `aegis-agent-linux-amd64.tar.gz` as `agent-artifacts/aegis-agent.tar.gz`.
- `api-server.environment.SERVER_EXTERNAL_IP` should reference `${EXTERNAL_IP:-}`.

Example MinIO init entrypoint fragment:

```yaml
entrypoint: >
  /bin/sh -c "
  mc alias set myminio http://minio:9000 $${MINIO_ACCESS_KEY} $${MINIO_SECRET_KEY};
  mc mb myminio/agent-artifacts --ignore-existing;
  mc anonymous set download myminio/agent-artifacts;
  mc cp /agent-artifacts/aegis-agent-linux-amd64.tar.gz myminio/agent-artifacts/aegis-agent.tar.gz;
  exit 0;
  "
```

## start.sh Requirements

The release `start.sh` should:

1. Check Docker and Docker Compose.
2. Load all `images/*.tar.gz`.
3. Detect a usable external host IP, excluding Docker/Kubernetes/private
   loopback ranges when appropriate.
4. Write `EXTERNAL_IP` into `.env`.
5. Run `docker compose up -d`.
6. Print `docker compose ps` after a short readiness wait.

## Package Zip

Ask before deleting an existing release zip. Then:

```bash
cd /code/ai-benchmark/release
zip -r aegis-${VERSION}-linux-amd64-release.zip ${VERSION}/
```

## Deployment Verification

```bash
unzip aegis-*-linux-amd64-release.zip
cd v5.5
bash start.sh
docker compose ps
curl http://localhost:8082/health
curl -s http://localhost:8082/api/v1/agent/install.sh | grep SERVER_ADDR
curl -s "http://localhost:8082/api/v1/agent/download?os=linux&arch=amd64" -o /tmp/test.tar.gz
ls -lh /tmp/test.tar.gz
docker exec aegis-postgres psql -U aegis_user -d aegis_db -c "SELECT COUNT(*) FROM sigma_rules;"
```

## Troubleshooting

Agent install script uses an internal Docker IP:

```bash
grep SERVER_ADDR <(curl -s http://localhost:8082/api/v1/agent/install.sh)
```

Fix `SERVER_EXTERNAL_IP: ${EXTERNAL_IP:-}` and make sure `start.sh` updates
`.env`.

Agent download is empty or truncated:

```bash
curl -s "http://localhost:8082/api/v1/agent/download?os=linux&arch=amd64" -o /tmp/test.tar.gz
file /tmp/test.tar.gz
docker compose logs minio-init
```

Fix MinIO object naming so `aegis-agent.tar.gz` exists in `agent-artifacts`.

DC reports `column does not exist`:

```bash
docker logs aegis-dc 2>&1 | grep "column.*does not exist"
```

Compare `init.sql` with `dc/internal/model/` and add missing fields.
