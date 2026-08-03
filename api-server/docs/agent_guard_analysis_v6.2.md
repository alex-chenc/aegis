# Agent Guard V6.2 P2 analysis design

## Scope and acceptance

P2 adds read-only, asynchronous analysis of an existing deterministic
`AgentSecurityFinding`. It does not add an action dependency, enforcement
route, tool call, policy mutation, or automatic freeze path.

The implementation is accepted when:

1. `POST /api/v1/agent-guard/findings/:finding_id/analyze` requires
   `agent_guard:analysis:run`, binds the exact finding and any supplied signed
   Agent scope, and returns a pending run with HTTP 202.
2. Analysis is unavailable unless both Agent Guard and its analysis flag are
   enabled. The default configuration remains disabled.
3. The Evidence Window is host and finding scoped, chronological, bounded to
   64 events and 128 KiB, retains direct rule evidence, applies a second
   redaction pass, and records loss/truncation/counter-evidence metadata.
4. Event JSON is placed only in a marked untrusted user-data boundary. The
   system instruction grants no tools, network, policy, or action capability.
5. The LLM uses a strict JSON Schema. The server independently rejects
   unknown fields, enum/range/size violations, foreign event IDs, and action
   recommendations above the P2 review/alert ceiling.
6. Provider errors, timeout, or invalid output update only the analysis run.
   Successful output links `latest_analysis_id` but never rewrites the
   deterministic finding's verdict, severity, confidence, summary, or
   recommendation.
7. Logs contain IDs, status, counts, digest, provider/model, latency, and safe
   error codes; evidence, credentials, prompts, and raw model output are not
   logged.

## Failure and rollback behavior

The analysis worker has a 60-second parent deadline. Invalid output becomes
`invalid_output`, inconclusive output becomes `inconclusive`, and provider or
timeout failures become `failed`. A failure is never interpreted as benign.

Operational rollback is the `agent_guard.analysis_enabled` feature flag. Read
APIs and deterministic findings continue to work while workers and the run
endpoint reject new analysis requests. P3 enforcement remains out of scope.

## P3 manual action design

P3 adds only authenticated manual actions. AI analysis has no reference to the
action service and cannot choose the action source; every HTTP-created row is
stored as `source=manual` with the authenticated username and a bounded reason.

The API accepts one UUID path target and resolves its host/instance ownership
from PostgreSQL. Optional UI scope parameters must match that resolved target.
The gRPC `BlockCommand.target` is the single resolved execution-unit or
instance UUID string. Host IDs, wildcards, PIDs and JSON target expansions are
never accepted as action targets.

Acceptance and state rules:

1. Freeze/resume/kill use separate permissions and remain unavailable unless
   both Agent Guard and `action_enabled` are true (default false).
2. The target must be confirmed, local, non-terminal, observable and backed by
   a current capability snapshot; the host Agent must be connected.
3. Manual action bodies contain only a required bounded `reason` and, for the
   freeze endpoint, optional `hold`. `hold=true` is persisted and dispatched as
   the atomic `hold_execution_unit` action; normal freeze timeout comes from the
   applied bundle/Agent LKG. Unknown/trailing JSON fields are rejected.
4. Concurrent freeze or hold requests for one unit return the existing active action.
   Repository row locks serialize creation on PostgreSQL.
5. A new row starts `pending`. A successful Server RPC means only
   `dispatching`; Agent `agent_guard_action_status` events establish
   `running/success/failed/expired/cancelled`. Terminal states never regress.
6. Action rows are the durable audit timeline and contain operator, exact
   resolved target, reason, request/dispatch/completion times and safe result or
   error codes. Operational logs omit reason and raw Agent payloads.
7. The realtime bridge projects valid status events through the same state
   machine and broadcasts a bounded typed `agent_guard.action_updated` summary.

Rollback is `agent_guard.action_enabled=false`, which stops new mutations while
GET timelines and late terminal status projection remain available. Automated
correlation actions and P4 rollout are outside this api-server stage.
