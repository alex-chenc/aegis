# Assistant Baseline Tool Contract and Full-Access Fix

## Problem and scope

The assistant baseline workflow can fall back to model-generated arguments when
the intent model uses valid English synonyms instead of the backend's canonical
identifiers. The model then retries strict tool schemas with legacy fields.
Separately, the UI exposes a full-access mode while authorization metadata,
prompts, and dispatch can observe different approval decisions.

This fix is limited to:

- canonicalizing the `baseline_compliance` workflow intent;
- making capability mapping the only tool-election authority for every
  tool-enabled Assistant run;
- making fully bound fixed-plan arguments authoritative;
- filtering and normalizing candidate tool arguments before runtime validation;
- revalidating final arguments before durable call creation or approval;
- snapshotting the effective approval mode for one assistant run;
- preventing reuse of volatile or asynchronous operation status;
- adding structured logs and regression tests for those decisions.

It does not change RBAC, tool enablement, explicit write-intent checks, target
validation, or command audit enforcement.

## Non-negotiable tool-election invariant

The Assistant has exactly one tool-election boundary:

```text
LLM intent -> capability identifiers -> exact backend mapping
           -> authorization hard gates -> immutable mapped execution plan
```

The runtime must never receive a free tool catalog and then perform a second
tool election using model-authored `tool_name`. When mapped tools exist, the
backend authorization artifact is always converted to the runtime initial
plan. A synchronous runtime step is bound to exactly one mapped tool. An
asynchronous runtime step may additionally contain only the producer's
registered completion tool when that completion tool was independently
accepted by the same Mapping artifact. The gateway rejects every name outside
that immutable per-step set before durable execution.

The ReAct `tool_name` field may remain as an agent-runtime wire-format field,
but it is not an election input: the model may copy only a name exposed for the
current step. Tools from later steps are hidden. It cannot add, replace, or
reorder tools outside the mapped plan. Runs without mapped tools receive no
tool descriptors and may only produce a direct answer.

This invariant is fail-closed and must be protected by code comments and
regression tests. Future prompt, planner, router, or agent-runtime changes may
not restore dynamic model tool election. A new tool can become callable only by
registering a capability mapping and passing the existing authorization gates.

## Target behavior

For the request to run all checks on every online host with automatic
remediation for five rounds, the backend must compile:

```json
{
  "target_scope": "all_online_hosts",
  "template_selector": "CIS_Ubuntu_Linux_24.04_LTS_Benchmark_v2.0.0-12-971.pdf",
  "scope": "all_rules",
  "remediation": {
    "enabled": true,
    "max_rounds": 5
  }
}
```

The model may select and reason about the workflow, but it must not replace
these compiled arguments. Unknown model fields are discarded for a fully bound
fixed step.

The production intent aliases `baseline_template.selector` and
`parameters.retry_rounds` are canonicalized to `template_selector` and
`remediation_rounds`. Missing governed workflow parameters must never cause a
fallback to dynamic tool election.

## Pre-invocation filter chain

A candidate model action is not yet a durable tool call. The gateway runs an
ordered filter chain before agent-runtime schema validation:

1. apply caller-authorized fixed-plan arguments;
2. apply workflow-scoped, allowlisted canonicalization;
3. validate the normalized arguments against the registered tool schema.

The dispatcher runs the same schema boundary again before generating a call ID,
creating an `assistant_tool_calls` row, evaluating approval, or invoking the
handler. Registry validation remains the final defense in depth.

The first implementation permits only semantics-preserving normalization.
`Host.Resolve` may translate `selector=live|alive|online` to
`target_scope=all_online_hosts` only when the authorized baseline workflow
already carries an online-host target. It never guesses IDs, broadens a generic
scope, enables remediation, drops unknown fields, or changes approved
arguments.

Pre-gateway validation attempts are internal recovery evidence. They are not
published as user-visible tool errors and are not persisted as durable tool
calls. If runtime cannot repair the candidate within its bounded retry policy,
the step fails once with an evidence gap.

For an approval resume, the dispatcher uses the already persisted prepared
arguments and runs validation-only filters. No parameter mutation is permitted
after approval.

The run snapshots `request_approval`, `whitelist`, or `full_access` before
runtime construction. Tool dispatch, prompt guidance, metadata, and audit logs
use that snapshot. `full_access` skips interactive approval but retains all
other authorization and validation gates.

Asynchronous polling always reaches the real status handler. Volatile operation
tools are never served from same-message result reuse. A background worker
advances durable operations after the conversation finishes. Operations with
no persisted progress for more than 24 hours fail as `operation_stale` instead
of unexpectedly dispatching historical work after a service restart.

## Compatibility and failure behavior

The generic intent model remains open. Canonical aliases are applied only when
the selected workflow or capability is `baseline_compliance`. Supported aliases
include:

- `machine` to `host`;
- `baseline` to `baseline_template`;
- `auto_repair` to `auto_remediate`;
- `remediation_enabled` to `auto_remediate`;
- `repair_rounds` to `remediation_rounds`;
- live, alive, or online host selectors to `all_online_hosts`.

If a governed baseline workflow still cannot bind a target or template, it
must stop with clarification or an evidence gap. It must not silently degrade
to unconstrained model-authored arguments. Conflicting or unsafe targets are
not guessed.

## Acceptance tests

- The captured production intent shape maps the fixed three-tool sequence
  `Host.Resolve -> Baseline.Compliance.Run -> Operation.Get`, then compiles it
  into two business runtime steps: host resolution and baseline execution plus
  its mapped completion query.
- Every non-empty mapped authorization plan is supplied as the runtime initial
  plan; arbitrary non-baseline tools are not replanned by the model.
- Runtime construction fails closed if tool descriptors exist without a mapped
  execution plan.
- A runtime step cannot invoke a tool outside its exact Mapping-bound primary
  and optional registered completion pair.
- If a mapped dependency fails, transitively blocked steps are persisted as
  `skipped` instead of remaining misleadingly `pending`.
- Fixed plan preparation removes legacy model fields and preserves compiled
  values.
- The captured `selector=live` production intent binds
  `target_scope=all_online_hosts` and activates the fixed workflow.
- The captured `baseline_template.selector` and `retry_rounds=5` shape binds the
  template and preserves five remediation rounds.
- A structured final model response without `final_answer` is rejected and
  replaced with an evidence-grounded response.
- A pre-gateway argument validation failure creates no visible tool-error event
  and no durable tool-call row.
- The dispatcher validates final arguments before creating a durable call row;
  invalid arguments invoke no handler and create no approval.
- Approval resume validates but does not mutate the persisted prepared args.
- Full access directly executes a high-risk tool without an approval row.
- Whitelist mode still requires approval.
- An approval-mode change after the run snapshot does not change that run.
- `Operation.Get` and every runtime asynchronous poll bypass result reuse.
- The affected assistant tests and `api-server` build pass.

## Rollback

The changes do not require a migration. Rolling back the API server binary
restores the previous behavior. Existing operation and approval records remain
audit evidence and are not rewritten.
