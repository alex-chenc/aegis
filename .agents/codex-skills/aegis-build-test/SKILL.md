---
name: aegis-build-test
description: Use for Aegis build, test, Docker Compose service verification, agent packaging, API smoke tests, health checks, and end-to-end runtime data-flow validation across api-server, server, dc, agent, and frontend.
version: 1.1.0
source: migrated-from-claude
---

# Aegis Build And Test

Use this skill before running build, test, deployment, or verification commands in
this repository. Prefer the narrowest check that covers the files changed.

## Service Ports

| Service | HTTP | gRPC | Container |
| --- | --- | --- | --- |
| `api-server` | `8082` | `19093` | `aegis-api-server` |
| `server` | - | `19090`, `19094` | `aegis-server` |
| `dc` | - | `19092` | `aegis-dc` |
| `frontend` | `8081` | - | `aegis-frontend` |
| `postgres` | `5432` | - | `aegis-postgres` |
| `redis` | `6379` | - | `aegis-redis` |
| `minio` | `9000`, `9001` | - | `aegis-minio` |
| `kafka` | `29092` | - | `aegis-kafka` |

## Architecture References

For architecture-sensitive changes, read only the relevant v5.5 design file:

```bash
sed -n '1,220p' docs/aegis_system_design_v5.5/README.md
sed -n '1,220p' docs/aegis_system_design_v5.5/architecture_design_v5.5.md
sed -n '1,220p' docs/aegis_system_design_v5.5/communication_protocol_design_v5.5.md
sed -n '1,220p' docs/aegis_system_design_v5.5/backend_detailed_design_v5.5_complete.md
sed -n '1,220p' docs/aegis_system_design_v5.5/agent_detailed_design_v5.5_complete.md
sed -n '1,220p' docs/aegis_system_design_v5.5/frontend_detailed_design_v5.5_complete.md
sed -n '1,220p' docs/aegis_system_design_v5.5/database_structure_design_v5.5_complete.md
```

## Component Builds

```bash
cd api-server && make build
cd server && make build
cd dc && make build
cd agent && make bpf && make build
cd frontend && npm run build
```

Agent eBPF changes require `make bpf` before `make build`.

## Docker Compose Iteration

```bash
docker compose build <service>
docker compose up -d --build <service>
docker compose logs -f <service>
docker compose ps
```

Available app services: `api-server`, `server`, `dc`, `frontend`.

For the full stack:

```bash
docker compose up -d --build
docker compose ps
```

Ask before running destructive lifecycle commands such as `docker compose down -v`.

## Health Checks

| Service | Check | Expected |
| --- | --- | --- |
| `api-server` | `curl -s http://localhost:8082/health` | `{"status":"ok"}` |
| `server` | `nc -z localhost 19090 && echo OK` | `OK` |
| `dc` | `nc -z localhost 19092 && echo OK` | `OK` |
| `postgres` | `pg_isready -U aegis_user -d aegis_db` | accepting connections |
| `redis` | `redis-cli -a <password> ping` | `PONG` |
| `minio` | `mc ready local` | `OK` |

## Frontend Checks

```bash
cd frontend
npm run test -- --grep "test name"
npm run test -- src/components/FooBar.test.ts
npm run lint
npm run type-check
npm run build
```

## API Smoke Tests

```bash
curl http://localhost:8082/health
curl -X POST http://localhost:8082/api/v1/vulnerability/scan \
  -H "Content-Type: application/json" \
  -d '{"host_id":1,"scan_type":"full"}'
```

## Runtime Data-Flow Validation

Expected path:

```text
Agent -> Server (19090) -> Kafka -> DC (19092) -> PostgreSQL
```

Useful checkpoints:

```bash
docker compose logs server | grep agent
docker compose exec kafka kafka-console-consumer --topic aegis.security.events --from-beginning
docker compose logs dc
docker compose exec postgres psql -U aegis_user -d aegis_db -c "SELECT COUNT(*) FROM alerts;"
```

## Agent Package Upload To MinIO

Local MinIO defaults:

```text
endpoint: http://localhost:9000
console:  http://localhost:9001
user:     minio_admin
password: a_third_strong_secret_password
```

Build and upload:

```bash
export MINIO_ENDPOINT=localhost:9000
export MINIO_ACCESS_KEY=minio_admin
export MINIO_SECRET_KEY=a_third_strong_secret_password
cd agent && make all && make upload
```

Remote MinIO:

```bash
MINIO_ENDPOINT=<server>:9000 \
MINIO_ACCESS_KEY=<your_key> \
MINIO_SECRET_KEY=<your_secret> \
make upload
```

## Agent Reinstall Flow

Target host:

```bash
sudo /opt/aegis-agent/uninstall.sh
curl -sSL http://<API_SERVER_IP>:8082/api/v1/agent/install.sh | sudo bash
sudo systemctl status aegis-agent
sudo journalctl -u aegis-agent -f
```

## Troubleshooting

Service startup:

```bash
docker compose logs <service>
netstat -tlnp | grep <port>
docker compose ps
```

Agent cannot connect:

```bash
docker compose exec server nc -z localhost 19090
sudo journalctl -u aegis-agent | grep -i error
telnet <server_ip> 19090
```

MinIO upload failure:

```bash
docker compose ps minio
mc alias list
mc cp dist/aegis-agent.tar.gz local/agent-artifacts/
```
