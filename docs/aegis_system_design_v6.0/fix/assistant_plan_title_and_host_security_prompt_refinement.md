# Assistant Plan Title Display And Host Security Prompt Refinement

## Problem

The assistant execution plan currently exposes step goal, tool names, and result
details in the right rail. For agent-mode conversations this makes the plan area
too dense; operators only need the plan step title and execution state there.

The host security analysis prompt also needs stronger structure so full-host or
multi-host analysis produces clear, evidence-based security results.

## Proposed Behavior

- In the assistant context rail, plan steps show only the step title and status.
- Plan goal, suggested tools, step result summaries, and audit/reflection details
  are hidden in this compact assistant mode.
- The reusable `ExecutionPlan` component keeps its detailed mode for other
  pages unless compact mode is explicitly enabled.
- Host security prompts require:
  - target scope and host identity coverage;
  - asset and exposure profile;
  - baseline and task posture;
  - vulnerability and affected host evidence;
  - alert and detection evidence;
  - live Agent evidence where available;
  - evidence gaps and conservative conclusions;
  - per-host risk level plus overall prioritized remediation.

## Test Cases

- Mount `ExecutionPlan` with title-only mode and assert step detail, tool tags,
  result summaries, and plan goal are not rendered.
- Build `AssistantPromptProvider` prompts and assert the host security analysis
  rubric and multi-host output requirements are present.
- Run focused frontend and backend tests.

## Risk And Rollback

The UI change is opt-in for the assistant rail, so rollback is removing the
`titleOnly` prop usage. Prompt changes only affect future assistant runs; old
conversation text is not rewritten.
