# Offline Release API Startup Fix

## Symptom

The offline release `start.sh` repeatedly printed connection-refused errors for
`127.0.0.1:8082` and then exited. The API Server container log showed:

```text
failed to ensure detection enhancement schema: ERROR: relation "alerts" does not exist
```

## Root Cause

`scripts/build_release_package.sh` generated
`backend/scripts/init.sql` using the caller's default file mode. The release
environment used `umask 0077`, which produced an `0600` root-owned file.
PostgreSQL runs `/docker-entrypoint-initdb.d/01-init.sql` as the unprivileged
`postgres` user, so it could not read the bind-mounted file. PostgreSQL created
the database but skipped the schema initialization. When API Server later
started, its runtime schema enhancement attempted to alter `alerts`, which had
never been created, and the process exited before listening on port 8082.

The clean-database smoke test also found an invalid PostgreSQL statement in
`003_v5.1_enhancements.sql`: PostgreSQL does not support `ADD CONSTRAINT IF
NOT EXISTS`. That statement stopped the concatenated init script before later
migrations had been applied.

The startup script also had an 80-second fixed wait budget. Kafka and the agent
hub have serial health checks, so a normal cold start can take longer than that
and was reported as an API failure without useful container diagnostics.

The release MinIO initialization additionally lacked the `aegis-builds` and
`aegis-releases` buckets and did not configure `MINIO_ARTIFACT_BASE_URL` in the
release service environment.

## Fix

- Set the generated `backend/scripts/init.sql` mode to `0644` explicitly.
- Replace the unsupported constraint syntax with a `pg_constraint` catalog
  check and a guarded `ALTER TABLE` statement.
- Replace the attempt-count loop with a five-minute, configurable,
  time-based API readiness wait (`AEGIS_API_HEALTH_TIMEOUT_SECONDS`).
- Silence expected transient probe failures and report progress every ten
  seconds. On timeout, print `docker compose ps -a` and the relevant service
  log tails.
- Create and publish the MinIO buckets used for build logs and released
  detection-package artifacts.
- Pass `MINIO_ARTIFACT_BASE_URL` to API Server and Server in generated Compose.

## Verification

1. Generate the release directory and verify `init.sql` has mode `0644`.
2. Start a clean PostgreSQL container with the generated script mounted at
   `/docker-entrypoint-initdb.d/01-init.sql`; verify the `alerts` table exists.
3. Run `bash -n start.sh` and `docker compose config` against the release.
4. Build the full release archive, validate its zip and gzip archives, and in
   an isolated environment run `./start.sh` followed by
   `curl -fsS http://localhost:8082/health`.

## Operational Recovery

For an already failed fresh deployment, the PostgreSQL data volume contains the
partially initialized database and will not rerun init scripts automatically.
After confirming it contains no required data, stop the release stack and
remove only that release's `postgres_data` volume, then start again with the
corrected archive. Do not remove a volume that contains production data.
