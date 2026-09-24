package mcpauthz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
)

const (
	SecurityInputContractVersion    = "aegis.mcp.security.input.v2"
	SecurityDecisionContractVersion = "aegis.mcp.security.decision.v2"
	SecurityDecisionPath            = "data.aegis.mcp.security.decision"
	SecurityPolicyRevision          = "mcp-security-rego-v1"
)

var (
	ErrInvalidSecurityInput    = errors.New("invalid MCP security input")
	ErrInvalidSecurityDecision = errors.New("invalid MCP security decision")
)

type SecurityInput struct {
	ContractVersion string           `json:"contract_version"`
	Phase           string           `json:"phase"`
	Operation       string           `json:"operation"`
	Request         SecurityRequest  `json:"request"`
	Tool            SecurityTool     `json:"tool"`
	Arguments       any              `json:"arguments"`
	Result          any              `json:"result,omitempty"`
	Upstream        SecurityUpstream `json:"upstream"`
	PayloadDigest   string           `json:"payload_digest"`
	Snapshot        SecuritySnapshot `json:"snapshot"`
}

type SecurityRequest struct {
	InvocationID     string `json:"invocation_id"`
	DecisionID       string `json:"decision_id"`
	ReceivedAtUnixMS int64  `json:"received_at_unix_ms"`
}

type SecurityTool struct {
	ExposedName string `json:"exposed_name"`
	RiskTier    string `json:"risk_tier"`
}

type SecurityUpstream struct {
	Started    bool   `json:"started"`
	Status     string `json:"status"`
	ErrorClass string `json:"error_class,omitempty"`
	SizeBytes  int64  `json:"size_bytes"`
}

type SecuritySnapshot struct {
	DeploymentGeneration int64  `json:"deployment_generation"`
	PolicyRevision       string `json:"policy_revision"`
}

type SecurityDecision struct {
	ContractVersion string        `json:"contract_version"`
	Phase           string        `json:"phase"`
	Action          string        `json:"action"`
	ReasonCode      string        `json:"reason_code"`
	RuleIDs         []string      `json:"rule_ids"`
	AuditRuleIDs    []string      `json:"audit_rule_ids"`
	Severity        string        `json:"severity"`
	EvidenceRefs    []EvidenceRef `json:"evidence_refs"`
	PolicyRevision  string        `json:"policy_revision"`
}

type EvidenceRef struct {
	Stage  string `json:"stage"`
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type SecurityEvaluator interface {
	EvaluateSecurity(context.Context, SecurityInput) (SecurityDecision, error)
}

// DefaultSecurityPolicy is intentionally small and bounded. It is the only
// source of MCP security actions; Go constructs typed input and persists the
// resulting decision, but does not implement matchers.
const DefaultSecurityPolicy = `package aegis.mcp.security
import rego.v1

sensitive_keys := {"password", "secret", "token", "authorization", "private_key", "access_key", "credential"}
injection_patterns := {"../", "..\\", "\r\n", "$(", "; rm ", " union select ", " or 1=1", "drop table", "ignore previous instructions", "ignore all previous instructions", "reveal the system prompt", "system message override", "developer message override"}

input_sensitive_paths contains path if {
  input.phase == "pre"
  walk(input.arguments, [path, value])
  is_string(path[count(path)-1])
  some key in sensitive_keys
  contains(lower(path[count(path)-1]), key)
}
input_injection_paths contains path if {
  input.phase == "pre"
  walk(input.arguments, [path, value])
  is_string(value)
  some pattern in injection_patterns
  contains(lower(value), pattern)
}
output_sensitive_paths contains path if {
  input.phase == "post"
  walk(input.result, [path, value])
  is_string(path[count(path)-1])
  some key in sensitive_keys
  contains(lower(path[count(path)-1]), key)
}
output_injection_paths contains path if {
  input.phase == "post"
  walk(input.result, [path, value])
  is_string(value)
  some pattern in injection_patterns
  contains(lower(value), pattern)
}

deny_ids contains "mcp.security.risk.l4.v1" if { input.phase == "pre"; input.tool.risk_tier == "l4" }
deny_ids contains "mcp.security.input.sensitive-key.v1" if { count(input_sensitive_paths) > 0 }
deny_ids contains "mcp.security.input.injection.v1" if { count(input_injection_paths) > 0 }
deny_ids contains "mcp.security.output.sensitive-key.v1" if { count(output_sensitive_paths) > 0 }
deny_ids contains "mcp.security.output.injection.v1" if { count(output_injection_paths) > 0 }
audit_ids contains "mcp.security.output.size.v1" if { input.phase == "post"; input.upstream.size_bytes > 524288 }
audit_ids contains "mcp.security.upstream.failure.v1" if { input.phase == "post"; input.upstream.status == "failed" }

action := "deny" if count(deny_ids) > 0
action := "audit" if { count(deny_ids) == 0; count(audit_ids) > 0 }
action := "allow" if { count(deny_ids) == 0; count(audit_ids) == 0 }
reason_code := "POLICY_DENIED" if action == "deny"
reason_code := "AUDIT_ONLY" if action == "audit"
reason_code := "POLICY_ALLOWED" if action == "allow"
severity := "critical" if count(deny_ids) > 0
severity := "medium" if { count(deny_ids) == 0; count(audit_ids) > 0 }
severity := "low" if { count(deny_ids) == 0; count(audit_ids) == 0 }
evidence_refs := [{"stage": input.phase, "path": "$", "digest": input.payload_digest}] if count(deny_ids) + count(audit_ids) > 0
evidence_refs := [] if count(deny_ids) + count(audit_ids) == 0

decision := {
  "contract_version": "aegis.mcp.security.decision.v2",
  "phase": input.phase,
  "action": action,
  "reason_code": reason_code,
  "rule_ids": sort(deny_ids),
  "audit_rule_ids": sort(audit_ids),
  "severity": severity,
  "evidence_refs": evidence_refs,
  "policy_revision": input.snapshot.policy_revision,
}
`

type PreparedSecurity struct {
	query    rego.PreparedEvalQuery
	revision string
	closed   atomic.Bool
}

func PrepareSecurity(ctx context.Context, revision, source string) (*PreparedSecurity, error) {
	revision = strings.TrimSpace(revision)
	if revision == "" || len(revision) > 128 || len(source) == 0 || len(source) > MaxRegoSourceBytes {
		return nil, fmt.Errorf("%w: invalid security policy", ErrInvalidSecurityDecision)
	}
	if !strings.Contains(source, "package aegis.mcp.security") {
		return nil, fmt.Errorf("%w: invalid security package", ErrInvalidSecurityDecision)
	}
	caps := ast.CapabilitiesForThisVersion()
	caps.AllowNet = []string{}
	prepared, err := rego.New(rego.Query(SecurityDecisionPath), rego.Module("aegis_mcp_security.rego", source), rego.Store(inmem.New()), rego.Capabilities(caps), rego.StrictBuiltinErrors(true)).PrepareForEval(ctx)
	if err != nil {
		return nil, err
	}
	s := &PreparedSecurity{query: prepared, revision: revision}
	probe := SecurityInput{ContractVersion: SecurityInputContractVersion, Phase: "pre", Operation: "tools/call", Request: SecurityRequest{InvocationID: "probe", DecisionID: "probe", ReceivedAtUnixMS: time.Now().UnixMilli()}, Tool: SecurityTool{ExposedName: "probe", RiskTier: "l1"}, Arguments: map[string]any{}, Upstream: SecurityUpstream{Status: "not_started"}, PayloadDigest: "sha256:probe", Snapshot: SecuritySnapshot{DeploymentGeneration: 1, PolicyRevision: revision}}
	if _, err := s.EvaluateSecurity(ctx, probe); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *PreparedSecurity) EvaluateSecurity(ctx context.Context, input SecurityInput) (SecurityDecision, error) {
	if s == nil || s.closed.Load() {
		return SecurityDecision{}, ErrPolicyUnavailable
	}
	if err := input.Validate(); err != nil {
		return SecurityDecision{}, err
	}
	if input.Snapshot.PolicyRevision != s.revision {
		return SecurityDecision{}, ErrPolicyUnavailable
	}
	result, err := s.query.Eval(ctx, rego.EvalInput(input))
	if err != nil || len(result) != 1 || len(result[0].Expressions) != 1 {
		return SecurityDecision{}, fmt.Errorf("%w: evaluation", ErrInvalidSecurityDecision)
	}
	raw, err := json.Marshal(result[0].Expressions[0].Value)
	if err != nil {
		return SecurityDecision{}, err
	}
	decision, err := DecodeSecurityDecision(raw, s.revision)
	if err != nil {
		return SecurityDecision{}, err
	}
	if decision.Phase != input.Phase {
		return SecurityDecision{}, ErrInvalidSecurityDecision
	}
	return decision, nil
}

func (i SecurityInput) Validate() error {
	if i.ContractVersion != SecurityInputContractVersion || i.Operation != "tools/call" || (i.Phase != "pre" && i.Phase != "post") || i.Request.InvocationID == "" || i.Request.DecisionID == "" || i.Request.ReceivedAtUnixMS <= 0 || i.Tool.ExposedName == "" || i.Snapshot.PolicyRevision == "" || i.Snapshot.DeploymentGeneration < 1 || i.Arguments == nil {
		return ErrInvalidSecurityInput
	}
	if i.PayloadDigest == "" || !strings.HasPrefix(i.PayloadDigest, "sha256:") || len(i.PayloadDigest) > 80 || i.Upstream.SizeBytes < 0 || i.Upstream.SizeBytes > 16<<20 {
		return ErrInvalidSecurityInput
	}
	if i.Phase == "pre" && (i.Upstream.Started || i.Upstream.Status != "not_started" || i.Result != nil) {
		return ErrInvalidSecurityInput
	}
	if i.Phase == "post" && (!i.Upstream.Started || (i.Upstream.Status != "succeeded" && i.Upstream.Status != "failed" && i.Upstream.Status != "timeout")) {
		return ErrInvalidSecurityInput
	}
	if i.Phase == "post" && i.Result != nil {
		encoded, err := json.Marshal(i.Result)
		if err != nil || len(encoded) > 16<<20 {
			return ErrInvalidSecurityInput
		}
	}
	return nil
}

func DecodeSecurityDecision(raw []byte, revision string) (SecurityDecision, error) {
	if len(raw) == 0 || len(raw) > 16<<10 || hasDuplicateJSONKeys(raw) != nil {
		return SecurityDecision{}, ErrInvalidSecurityDecision
	}
	var fields map[string]json.RawMessage
	if err := decodeStrictObject(raw, &fields); err != nil || len(fields) != 9 {
		return SecurityDecision{}, ErrInvalidSecurityDecision
	}
	for _, key := range []string{"contract_version", "phase", "action", "reason_code", "rule_ids", "audit_rule_ids", "severity", "evidence_refs", "policy_revision"} {
		if _, ok := fields[key]; !ok {
			return SecurityDecision{}, ErrInvalidSecurityDecision
		}
	}
	var d SecurityDecision
	if err := json.Unmarshal(raw, &d); err != nil || d.ContractVersion != SecurityDecisionContractVersion || (d.Phase != "pre" && d.Phase != "post") || d.PolicyRevision != revision || d.Action == "" || d.ReasonCode == "" || d.RuleIDs == nil || d.AuditRuleIDs == nil || d.EvidenceRefs == nil {
		return SecurityDecision{}, ErrInvalidSecurityDecision
	}
	if d.Action != "allow" && d.Action != "deny" && d.Action != "audit" && d.Action != "redact" && d.Action != "quarantine" {
		return SecurityDecision{}, ErrInvalidSecurityDecision
	}
	if d.Severity != "low" && d.Severity != "medium" && d.Severity != "high" && d.Severity != "critical" {
		return SecurityDecision{}, ErrInvalidSecurityDecision
	}
	if (d.Action == "allow" && (d.ReasonCode != "POLICY_ALLOWED" || d.Severity != "low" || len(d.RuleIDs) != 0 || len(d.AuditRuleIDs) != 0)) ||
		(d.Action == "deny" && (d.ReasonCode != "POLICY_DENIED" || d.Severity != "critical")) ||
		(d.Action == "audit" && (d.ReasonCode != "AUDIT_ONLY" || d.Severity != "medium")) ||
		((d.Action == "redact" || d.Action == "quarantine") && (d.ReasonCode != "OUTPUT_QUARANTINED" || d.Severity != "critical")) || len(d.EvidenceRefs) > 64 {
		return SecurityDecision{}, ErrInvalidSecurityDecision
	}
	if d.Action == "deny" && len(d.RuleIDs) == 0 || d.Action == "audit" && len(d.AuditRuleIDs) == 0 {
		return SecurityDecision{}, ErrInvalidSecurityDecision
	}
	if !validRuleIDs(d.RuleIDs) || !validRuleIDs(d.AuditRuleIDs) {
		return SecurityDecision{}, ErrInvalidSecurityDecision
	}
	for _, ref := range d.EvidenceRefs {
		if ref.Stage != d.Phase || ref.Path == "" || len(ref.Path) > 512 || len(ref.Digest) > 80 {
			return SecurityDecision{}, ErrInvalidSecurityDecision
		}
	}
	return d, nil
}

func (s *PreparedSecurity) Revision() string {
	if s == nil {
		return ""
	}
	return s.revision
}
func (s *PreparedSecurity) Close() {
	if s != nil {
		s.closed.Store(true)
	}
}
