package mcp

import (
	"encoding/json"
	"regexp"
	"strings"
)

// InvocationEvent is the metadata-only Kafka contract. Raw arguments and
// results never belong on this topic; summaries are bounded and treated as
// untrusted text by the deterministic rules.
type InvocationEvent struct {
	Schema         string          `json:"schema"`
	InvocationID   string          `json:"invocation_id"`
	ToolAlias      string          `json:"tool_alias"`
	Status         string          `json:"status"`
	PolicyDecision string          `json:"policy_decision"`
	RequestDigest  string          `json:"request_digest"`
	ResultDigest   string          `json:"result_digest"`
	Classification string          `json:"classification"`
	RuleHints      []string        `json:"rule_hints"`
	Metadata       json.RawMessage `json:"metadata"`
}

type RuleHit struct {
	RuleKey  string `json:"rule_key"`
	Severity string `json:"severity"`
	Phase    string `json:"phase"`
	Reason   string `json:"reason"`
	Evidence string `json:"evidence"`
}

type Verdict struct {
	DeterministicSeverity string    `json:"deterministic_severity"`
	OverallRisk           string    `json:"overall_risk"`
	Hits                  []RuleHit `json:"hits"`
}

var (
	secretPattern    = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|bearer\s+[a-z0-9._-]+|password|private[_-]?key|client[_-]?secret)`)
	injectionPattern = regexp.MustCompile(`(?i)(ignore\s+(all\s+)?(previous\s+|prior\s+)?instructions|system\s+prompt|<script|javascript:|\bunion\s+select\b|;\s*(drop|delete|insert|update)\b|\.\./)`)
)

func Analyze(event InvocationEvent) Verdict {
	text := strings.ToLower(strings.TrimSpace(event.ToolAlias + " " + event.Classification + " " + strings.Join(event.RuleHints, " ")))
	hits := make([]RuleHit, 0, 2)
	if secretPattern.MatchString(text) || strings.Contains(text, "secret_marker") {
		hits = append(hits, RuleHit{RuleKey: "mcp.secret_or_credential", Severity: "high", Phase: "pre_post", Reason: "secret-like marker in invocation evidence", Evidence: "redacted"})
	}
	if injectionPattern.MatchString(text) || strings.Contains(text, "prompt_injection_marker") {
		hits = append(hits, RuleHit{RuleKey: "mcp.untrusted_injection", Severity: "high", Phase: "pre_post", Reason: "untrusted injection or dangerous syntax marker", Evidence: "redacted"})
	}
	if event.Status == "denied" || event.PolicyDecision == "deny" {
		hits = append(hits, RuleHit{RuleKey: "mcp.policy_denied", Severity: "medium", Phase: "pre", Reason: "deterministic policy denial", Evidence: "metadata"})
	}
	severity := "low"
	risk := "low"
	for _, hit := range hits {
		if hit.Severity == "high" {
			severity, risk = "high", "high"
			break
		}
		if hit.Severity == "medium" {
			severity, risk = "medium", "medium"
		}
	}
	return Verdict{DeterministicSeverity: severity, OverallRisk: risk, Hits: hits}
}
