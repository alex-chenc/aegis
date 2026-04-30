# AI Conservative Rule Effectiveness Design V5.6

## Problem

The operator can configure AI rule update with the most conservative strategy,
but alert volume can remain high. Investigation found that the system repeatedly
tightens rules while the agent-side matcher still triggers broadly.

Observed production data:

- AI rule config is enabled with `mode=auto`, `conservatism=0.00`, and
  `high_frequency_count=10` within `high_frequency_hours=1`.
- Three rules still produced tens of thousands of pending alerts in the last
  24 hours.
- The same rules had been updated more than 120 times and their YAML content had
  grown to tens of KB, showing repeated ineffective tightening.

## Root Causes

1. Conservative strategy is not applied to automatic tightening.
   - `conservatism` only changes manual/test rule generation prompts.
   - The automatic `analyzeRule` path uses a fixed false-positive confidence
     threshold.
2. Rule tightening edits `detection.condition`, but the agent matcher currently
   ignores Sigma condition semantics and matches if any compiled field pattern
   matches.
3. LLM tightening can append natural-language fragments or undefined selector
   names to `condition`; these fragments are persisted without a quality gate.
4. The scheduler can repeatedly tighten the same high-frequency rule without a
   cooldown or evidence that the last update reduced alert volume.

## Design

### 1. Conservative Policy Mapping

`conservatism` is a risk slider where lower means more conservative:

| Range | Confidence Required | Cooldown | Behavior |
| --- | ---: | ---: | --- |
| `0.0-0.2` | `0.90` | `24h` | Extremely low trigger rate; reject weak updates |
| `0.2-0.4` | `0.85` | `12h` | Conservative |
| `0.4-0.6` | `0.80` | `6h` | Balanced |
| `0.6-1.0` | `0.70` | `1h` | Aggressive |

Automatic tightening must use this mapping before applying an LLM result.

### 2. Agent Condition Semantics

The agent Sigma matcher must evaluate the rule `detection.condition` instead of
treating all selection fields as a single broad OR set.

Required first-stage condition support:

- Single selector: `selection`
- Boolean operators: `and`, `or`, `not`
- Parentheses
- Selector glob expressions:
  - `all of selection_*`
  - `1 of selection_*`
  - `not 1 of filter_*`

Unsupported condition syntax must fail closed and return no match.

### 3. Rule Quality Gate

Before persisting a tightened rule:

1. Parse the generated YAML.
2. Validate that every selector referenced by `condition` exists, except
   supported glob expressions.
3. Reject natural-language field fragments in `condition`, for example
   `CommandLine contains ...` or Chinese prose.
4. Reject duplicate tightening that does not change the rule content.

### 4. Cooldown

If a rule was updated within the configured cooldown window, skip automatic
tightening for that rule. This prevents version/content growth loops while alert
counts are still being measured.

## Verification

Tests first:

1. Agent matcher returns false when a rule condition requires two selectors and
   only one selector matches.
2. Agent matcher supports `not 1 of filter_*`.
3. Automatic tightening skips rules inside conservative cooldown.
4. Conservative mode requires higher false-positive confidence than aggressive
   mode.
5. Tightened rules with undefined selectors or natural-language condition
   fragments are rejected.

Runtime verification after implementation:

1. Run focused Go tests through `aegis-build-test`.
2. Build changed Go components through `aegis-build-test`.
3. Use read-only SQL/curl checks to compare alert counts before and after a short
   monitoring window.
