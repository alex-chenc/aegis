#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONTRACT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/aegis-agent-guard-contract.XXXXXX")"
trap 'rm -rf "${CONTRACT_DIR}"' EXIT

BUNDLE_FIXTURE="${CONTRACT_DIR}/bundle.json"
APPLIED_FIXTURE="${CONTRACT_DIR}/config-status-applied.json"
REJECTED_FIXTURE="${CONTRACT_DIR}/config-status-rejected.json"

(
  cd "${ROOT_DIR}/api-server"
  AEGIS_AGENT_GUARD_CONTRACT_BUNDLE_OUT="${BUNDLE_FIXTURE}" \
    go test ./internal/service -run '^TestExportAgentGuardBundleContract$' -count=1
)

(
  cd "${ROOT_DIR}/agent"
  AEGIS_AGENT_GUARD_CONTRACT_BUNDLE="${BUNDLE_FIXTURE}" \
  AEGIS_AGENT_GUARD_APPLIED_STATUS_OUT="${APPLIED_FIXTURE}" \
  AEGIS_AGENT_GUARD_REJECTED_STATUS_OUT="${REJECTED_FIXTURE}" \
    go test ./internal/agentguard -run '^TestAPIServerBundleContractAndExportConfigStatuses$' -count=1
)

(
  cd "${ROOT_DIR}/server"
  AEGIS_AGENT_GUARD_CONTRACT_BUNDLE="${BUNDLE_FIXTURE}" \
  AEGIS_AGENT_GUARD_APPLIED_STATUS="${APPLIED_FIXTURE}" \
  AEGIS_AGENT_GUARD_REJECTED_STATUS="${REJECTED_FIXTURE}" \
    go test ./internal/grpc_server -run '^TestAPIServerBundleAndAgentStatusCrossContract$' -count=1
)

(
  cd "${ROOT_DIR}/dc"
  AEGIS_AGENT_GUARD_APPLIED_STATUS="${APPLIED_FIXTURE}" \
  AEGIS_AGENT_GUARD_REJECTED_STATUS="${REJECTED_FIXTURE}" \
    go test ./internal/pipeline -run '^TestAgentConfigStatusCrossContract$' -count=1
)

printf 'PASS: Agent Guard bundle and config-status cross-service contracts\n'
