#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RELEASE_SCRIPT="${ROOT_DIR}/scripts/build_release_package.sh"
ROOT_COMPOSE="${ROOT_DIR}/docker-compose.yml"
ENV_EXAMPLE="${ROOT_DIR}/.env.example"
V62_MIGRATION="${ROOT_DIR}/migrations/029_v6.2_agent_guard.sql"
AGENT_INSTALLER="${ROOT_DIR}/server/internal/handler/agent_handler.go"

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

assert_contains() {
  local file="$1"
  local expected="$2"

  grep -F -- "${expected}" "${file}" >/dev/null ||
    fail "${file} does not contain required contract: ${expected}"
}

assert_min_count() {
  local file="$1"
  local expected="$2"
  local minimum="$3"
  local actual

  actual="$(grep -F -c -- "${expected}" "${file}" || true)"
  if [ "${actual}" -lt "${minimum}" ]; then
    fail "${file} contains ${actual} occurrences of ${expected}; expected at least ${minimum}"
  fi
}

assert_service_contains() {
  local file="$1"
  local service="$2"
  local expected="$3"

  awk -v header="  ${service}:" '
    $0 == header {
      in_service = 1
      next
    }
    in_service && $0 ~ /^  [A-Za-z0-9_-]+:$/ {
      exit
    }
    in_service {
      print
    }
  ' "${file}" | grep -F -- "${expected}" >/dev/null ||
    fail "${file} service ${service} does not contain required contract: ${expected}"
}

assert_service_not_contains() {
  local file="$1"
  local service="$2"
  local forbidden="$3"

  if awk -v header="  ${service}:" '
    $0 == header {
      in_service = 1
      next
    }
    in_service && $0 ~ /^  [A-Za-z0-9_-]+:$/ {
      exit
    }
    in_service {
      print
    }
  ' "${file}" | grep -F -- "${forbidden}" >/dev/null; then
    fail "${file} service ${service} contains forbidden contract: ${forbidden}"
  fi
}

assert_mode() {
  local path="$1"
  local expected="$2"
  local actual

  actual="$(stat -c '%a' "${path}")"
  if [ "${actual}" != "${expected}" ]; then
    fail "${path} mode is ${actual}; expected ${expected}"
  fi
}

assert_db_migrate_only_runs_v62() {
  local file="$1"

  if awk '
    $0 == "  db-migrate:" {
      in_service = 1
      next
    }
    in_service && $0 ~ /^  [A-Za-z0-9_-]+:$/ {
      exit
    }
    in_service {
      print
    }
  ' "${file}" |
    grep -F -v -- '029_v6.2_agent_guard.sql' |
    grep -E -- 'migrations/[0-9]{3}[^/]*\.sql' >/dev/null; then
    fail "${file} db-migrate service references a historical migration"
  fi
}

assert_contains "${RELEASE_SCRIPT}" 'VERSION="${1:-}"'
if [ -e "${V62_MIGRATION}" ] && [ -z "$(find "${V62_MIGRATION}" -maxdepth 0 -perm -004 -print -quit)" ]; then
  fail "${V62_MIGRATION} must be readable by the PostgreSQL container user"
fi
assert_contains "${RELEASE_SCRIPT}" 'LC_ALL=C sort'
assert_contains "${RELEASE_SCRIPT}" 'migrations/029_v6.2_agent_guard.sql'
assert_contains "${RELEASE_SCRIPT}" 'backend/migrations/029_v6.2_agent_guard.sql'
assert_min_count "${RELEASE_SCRIPT}" 'copy_release_migration' 2
assert_contains "${RELEASE_SCRIPT}" 'db-migrate:'
assert_contains "${RELEASE_SCRIPT}" 'condition: service_completed_successfully'
assert_min_count "${RELEASE_SCRIPT}" 'condition: service_completed_successfully' 3
assert_contains "${RELEASE_SCRIPT}" 'save_image pgvector/pgvector:pg16 pgvector.tar.gz'

assert_contains "${ROOT_COMPOSE}" 'db-migrate:'
assert_contains "${ROOT_COMPOSE}" './migrations:/docker-entrypoint-initdb.d:ro'
if grep -F -- './migrations/001_init.sql:/docker-entrypoint-initdb.d/01-init.sql:ro' "${ROOT_COMPOSE}" >/dev/null; then
  fail "${ROOT_COMPOSE} fresh database initialization would skip prerequisite migrations"
fi
assert_contains "${ROOT_COMPOSE}" './migrations/029_v6.2_agent_guard.sql:/migrations/029_v6.2_agent_guard.sql:ro'
assert_min_count "${ROOT_COMPOSE}" 'condition: service_completed_successfully' 3

for compose_contract in "${RELEASE_SCRIPT}" "${ROOT_COMPOSE}"; do
  assert_service_contains "${compose_contract}" "db-migrate" 'condition: service_healthy'
  assert_service_contains "${compose_contract}" "db-migrate" 'ON_ERROR_STOP=1'
  assert_service_contains "${compose_contract}" "db-migrate" '/migrations/029_v6.2_agent_guard.sql'
  assert_service_not_contains "${compose_contract}" "db-migrate" '/migrations/001'
  assert_service_not_contains "${compose_contract}" "db-migrate" 'backend/scripts/init.sql'
  assert_db_migrate_only_runs_v62 "${compose_contract}"
  for database_consumer in api-server server dc; do
    assert_service_contains \
      "${compose_contract}" \
      "${database_consumer}" \
      'condition: service_completed_successfully'
  done
done
assert_service_contains "${RELEASE_SCRIPT}" "db-migrate" 'image: pgvector/pgvector:pg16'

rollout_flags=(
  AGENT_GUARD_ENABLED
  AGENT_GUARD_POLICY_WRITE_ENABLED
  AGENT_GUARD_ANALYSIS_ENABLED
  AGENT_GUARD_ACTION_ENABLED
  AGENT_GUARD_TOOL_ADAPTER_ENABLED
  AGENT_GUARD_DENY_ENABLED
  AGENT_GUARD_FREEZE_ENABLED
  AGENT_GUARD_ACTION_PUBLISH_ENABLED
  AGENT_GUARD_ACTION_CONSUMER_ENABLED
  AGENT_GUARD_PROJECTION_ENABLED
  AGENT_BEHAVIOR_RULES_ENABLED
  AGENT_BEHAVIOR_FINDINGS_ENABLED
  AGENT_BEHAVIOR_ANALYSIS_REQUEST_ENABLED
  AGENT_GUARD_ALERT_ENABLED
)

for flag in "${rollout_flags[@]}"; do
  assert_contains "${ENV_EXAMPLE}" "${flag}=false"
  assert_contains "${RELEASE_SCRIPT}" "${flag}: \${${flag}:-false}"
  assert_contains "${ROOT_COMPOSE}" "${flag}: \${${flag}:-false}"
done

assert_contains "${ENV_EXAMPLE}" 'AGENT_GUARD_SCOPE_SIGNING_KEY='
assert_contains "${RELEASE_SCRIPT}" 'AGENT_GUARD_SCOPE_SIGNING_KEY: ${AGENT_GUARD_SCOPE_SIGNING_KEY:-}'
assert_contains "${ROOT_COMPOSE}" 'AGENT_GUARD_SCOPE_SIGNING_KEY: ${AGENT_GUARD_SCOPE_SIGNING_KEY:-}'
assert_contains "${AGENT_INSTALLER}" 'AgentGuardToolAdapterEnabled = false'
assert_contains "${AGENT_INSTALLER}" 'AgentGuardToolSourceManifest = ""'
assert_contains "${AGENT_INSTALLER}" 'AgentGuardToolHookSocket = ""'
assert_contains "${RELEASE_SCRIPT}" 'cp "${ROOT_DIR}/.env.example" "${RELEASE_DIR}/.env.example"'
assert_min_count "${RELEASE_SCRIPT}" 'normalize_release_permissions' 3

explicit_version_probe="${TMPDIR:-/tmp}/aegis-release-contract-explicit-version-$$"
if RELEASE_ROOT="${explicit_version_probe}" "${RELEASE_SCRIPT}" >/dev/null 2>&1; then
  fail "release script unexpectedly succeeded without an explicit version"
fi
if [ -e "${explicit_version_probe}" ]; then
  fail "release script created output before rejecting a missing version"
fi

permissions_probe="$(mktemp -d "${TMPDIR:-/tmp}/aegis-release-contract-permissions.XXXXXX")"
trap 'rm -rf "${permissions_probe}"' EXIT
(
  umask 0077
  GENERATE_ONLY=1 RELEASE_ROOT="${permissions_probe}" "${RELEASE_SCRIPT}" v6.2 >/dev/null
)
assert_mode "${permissions_probe}/v6.2" 755
assert_mode "${permissions_probe}/v6.2/docker-compose.yml" 644
assert_mode "${permissions_probe}/v6.2/.env.example" 644
assert_mode "${permissions_probe}/v6.2/backend/scripts/init.sql" 644
assert_mode "${permissions_probe}/v6.2/backend/migrations/029_v6.2_agent_guard.sql" 644
assert_mode "${permissions_probe}/v6.2/start.sh" 755
assert_mode "${permissions_probe}/v6.2/build-context/minio-entrypoint.sh" 755

printf 'PASS: release package static contracts\n'
