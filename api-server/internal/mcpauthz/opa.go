package mcpauthz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
)

const (
	MaxRegoSourceBytes = 256 << 10
)

// Permission is the typed policy data accepted by the controlled template.
// It remains available for compatibility while RegoSource provides the
// explicit source-policy path under the same fixed decision contract.
type Permission struct {
	CatalogReleaseID string   `json:"catalog_release_id"`
	ToolRevisionID   string   `json:"tool_revision_id"`
	RequireUser      bool     `json:"require_user"`
	UserIDs          []string `json:"user_ids"`
	ProjectIDs       []string `json:"project_ids"`
	MaxLimit         int64    `json:"max_limit"`
}

type Policy struct {
	Revision         string                           `json:"revision"`
	Permissions      map[string]map[string]Permission `json:"permissions"`
	BlockRiskAtLeast string                           `json:"block_risk_at_least,omitempty"`
	// RegoSource is an optional, constrained policy module. When present it is
	// the executable policy; Permissions remains for backwards-compatible
	// typed policies and is ignored by the Rego evaluator.
	RegoSource string `json:"rego_source,omitempty"`
}

type PreparedSnapshot struct {
	query         rego.PreparedEvalQuery
	revision      string
	digest        string
	bindings      map[string]map[string]struct{}
	visibilityAll bool
	closed        atomic.Bool
}

const controlledPolicy = `package aegis.mcp.authz
import rego.v1

default permit := false

subject_ok(permission) if {
  permission.require_user == false
}
subject_ok(permission) if {
  permission.require_user == true
  input.principal.user.verified == true
  input.principal.user.id in permission.user_ids
}

permit if {
  input.contract_version == "aegis.mcp.authz.input.v1"
  input.operation == "tools/call"
  input.snapshot.policy_revision == data.aegis_config.meta.revision
  permission := data.aegis_config.permissions[input.principal.client_id][input.tool.release_tool_id]
  input.tool.catalog_release_id == permission.catalog_release_id
  input.tool.tool_revision_id == permission.tool_revision_id
  subject_ok(permission)
  project_permitted(permission)
  limit_permitted(permission)
}

project_permitted(permission) if {
  count(permission.project_ids) == 0
}
project_permitted(permission) if {
  count(permission.project_ids) > 0
  input.arguments.project_id in input.grant.resource_scope.project_ids
  input.arguments.project_id in permission.project_ids
}

limit_permitted(permission) if {
  not input.arguments.limit
}
limit_permitted(permission) if {
  is_number(input.arguments.limit)
  input.arguments.limit == floor(input.arguments.limit)
  input.arguments.limit >= 1
  input.arguments.limit <= permission.max_limit
}

deny_ids contains "baseline.block_risk" if {
  data.aegis_config.meta.block_risk_at_least != ""
  risk_rank(input.tool.risk_tier) >= risk_rank(data.aegis_config.meta.block_risk_at_least)
}
deny_ids contains "NO_MATCHING_PERMIT" if { not permit }

risk_rank("l1") := 1
risk_rank("l2") := 2
risk_rank("l3") := 3
risk_rank("l4") := 4

default allow := false
allow if { permit; count(deny_ids) == 0 }
reason_code := "POLICY_ALLOWED" if allow
reason_code := "POLICY_DENIED" if not allow

decision := {
  "contract_version": "aegis.mcp.authz.decision.v1",
  "allow": allow,
  "reason_code": reason_code,
  "deny_rule_ids": sort(deny_ids),
  "audit_rule_ids": [],
  "policy_revision": data.aegis_config.meta.revision,
}
`

func Prepare(ctx context.Context, policy Policy) (*PreparedSnapshot, error) {
	if strings.TrimSpace(policy.RegoSource) != "" {
		return PrepareRego(ctx, policy.Revision, policy.RegoSource)
	}
	if err := ValidatePolicy(policy); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPolicyUnavailable, err)
	}
	if policy.BlockRiskAtLeast != "" && !validRisk(policy.BlockRiskAtLeast) {
		return nil, fmt.Errorf("%w: invalid risk threshold", ErrPolicyUnavailable)
	}
	policy = normalizePolicy(policy)
	data := map[string]any{"aegis_config": map[string]any{"meta": map[string]any{"revision": policy.Revision, "block_risk_at_least": policy.BlockRiskAtLeast}, "permissions": policy.Permissions}}
	digest, err := CanonicalDigest(data)
	if err != nil {
		return nil, fmt.Errorf("%w: policy digest: %v", ErrPolicyUnavailable, err)
	}
	bindings := make(map[string]map[string]struct{}, len(policy.Permissions))
	for clientID, permissions := range policy.Permissions {
		bindings[clientID] = make(map[string]struct{}, len(permissions))
		for releaseToolID := range permissions {
			bindings[clientID][releaseToolID] = struct{}{}
		}
	}
	prepared, err := rego.New(rego.Query(DecisionPath), rego.Module("aegis_mcp_authz.rego", controlledPolicy), rego.Store(inmem.NewFromObject(data)), rego.StrictBuiltinErrors(true)).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: policy compile: %v", ErrPolicyUnavailable, err)
	}
	return &PreparedSnapshot{query: prepared, revision: policy.Revision, digest: digest, bindings: bindings}, nil
}

// PrepareRego compiles one bounded Rego module against the fixed MCP input
// contract. The module must expose data.aegis.mcp.authz.decision; evaluation
// validates the complete decision contract before the snapshot is accepted.
func PrepareRego(ctx context.Context, revision, source string) (*PreparedSnapshot, error) {
	revision = strings.TrimSpace(revision)
	source = strings.TrimSpace(source)
	if revision == "" || len(revision) > 128 {
		return nil, fmt.Errorf("%w: missing or oversized revision", ErrPolicyUnavailable)
	}
	if len([]byte(source)) == 0 || len([]byte(source)) > MaxRegoSourceBytes {
		return nil, fmt.Errorf("%w: Rego source must be between 1 and %d bytes", ErrPolicyUnavailable, MaxRegoSourceBytes)
	}
	if err := validateRegoSource(source); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPolicyUnavailable, err)
	}
	data := map[string]any{"aegis_config": map[string]any{"meta": map[string]any{"revision": revision}}}
	digest, err := CanonicalDigest(map[string]any{"revision": revision, "rego_source": source})
	if err != nil {
		return nil, fmt.Errorf("%w: policy digest: %v", ErrPolicyUnavailable, err)
	}
	caps := ast.CapabilitiesForThisVersion()
	caps.AllowNet = []string{}
	banned := map[string]struct{}{"http.send": {}, "opa.runtime": {}, "opa.eval": {}, "io.jwt.decode": {}, "io.jwt.decode_verify": {}}
	allowed := make([]*ast.Builtin, 0, len(caps.Builtins))
	for _, builtin := range caps.Builtins {
		if _, blocked := banned[builtin.Name]; !blocked {
			allowed = append(allowed, builtin)
		}
	}
	caps.Builtins = allowed
	prepared, err := rego.New(rego.Query(DecisionPath), rego.Module("aegis_mcp_authz.rego", source), rego.Store(inmem.NewFromObject(data)), rego.Capabilities(caps), rego.StrictBuiltinErrors(true)).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: Rego compile: %v", ErrPolicyUnavailable, err)
	}
	snapshot := &PreparedSnapshot{query: prepared, revision: revision, digest: digest, visibilityAll: true}
	if err := snapshot.validateContract(ctx); err != nil {
		snapshot.Close()
		return nil, fmt.Errorf("%w: Rego decision contract: %v", ErrPolicyUnavailable, err)
	}
	return snapshot, nil
}

func validateRegoSource(source string) error {
	lower := strings.ToLower(source)
	for _, banned := range []string{"import net/http", "import opa.runtime", "import io.jwt", "http.send", "opa.runtime", "opa.eval"} {
		if strings.Contains(lower, banned) {
			return fmt.Errorf("forbidden Rego import or builtin %q", banned)
		}
	}
	if !strings.Contains(lower, "package aegis.mcp.authz") {
		return errors.New("module must declare package aegis.mcp.authz")
	}
	return nil
}

func (s *PreparedSnapshot) validateContract(ctx context.Context) error {
	input := Input{ContractVersion: InputContractVersion, Operation: "tools/call", Request: NowRequest("validation-attempt", "validation-decision", time.Now().UTC()), Principal: Principal{ClientID: "validation-client", CredentialID: "validation-credential"}, Grant: Grant{ID: "validation-grant", Version: 1, ResourceScope: map[string]any{}}, Tool: Tool{CatalogReleaseID: "validation-catalog-release", ReleaseToolID: "validation-release-tool", ToolRevisionID: "validation-tool-revision", ServerRevisionID: "validation-server-revision", ExposedName: "validation", UpstreamName: "validation", InputSchemaDigest: "validation-schema", RiskTier: "l1"}, Arguments: map[string]any{}, Snapshot: Snapshot{DeploymentGeneration: 1, PolicyRevision: s.revision}}
	_, err := s.Evaluate(ctx, input)
	return err
}

func normalizePolicy(policy Policy) Policy {
	normalized := policy
	normalized.Permissions = make(map[string]map[string]Permission, len(policy.Permissions))
	for clientID, permissions := range policy.Permissions {
		normalized.Permissions[clientID] = make(map[string]Permission, len(permissions))
		for releaseToolID, permission := range permissions {
			permission.UserIDs = append([]string{}, permission.UserIDs...)
			permission.ProjectIDs = append([]string{}, permission.ProjectIDs...)
			normalized.Permissions[clientID][releaseToolID] = permission
		}
	}
	return normalized
}

func ValidatePolicy(policy Policy) error {
	if policy.Revision == "" || len(policy.Revision) > 128 {
		return errors.New("missing or oversized revision")
	}
	if policy.Permissions == nil {
		return errors.New("missing permissions")
	}
	if policy.BlockRiskAtLeast != "" && !validRisk(policy.BlockRiskAtLeast) {
		return errors.New("invalid risk threshold")
	}
	for clientID, permissions := range policy.Permissions {
		if clientID == "" || len(clientID) > 128 || len(permissions) == 0 {
			return errors.New("invalid client permission set")
		}
		for releaseToolID, permission := range permissions {
			if releaseToolID == "" || permission.CatalogReleaseID == "" || permission.ToolRevisionID == "" || permission.MaxLimit < 0 {
				return errors.New("invalid permission identity or limit")
			}
			if permission.RequireUser && len(permission.UserIDs) == 0 {
				return errors.New("user permission has no users")
			}
			if !uniqueStrings(permission.UserIDs) || !uniqueStrings(permission.ProjectIDs) {
				return errors.New("permission contains duplicate values")
			}
		}
	}
	return nil
}

func uniqueStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" || len(value) > 128 {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func (s *PreparedSnapshot) Evaluate(ctx context.Context, input Input) (Decision, error) {
	if s == nil || s.closed.Load() {
		return Decision{}, ErrPolicyUnavailable
	}
	// OPA may return an empty result for an already-cancelled context instead
	// of surfacing context.Canceled. Check before validating or evaluating so
	// cancellation and deadline paths can never be interpreted as a decision.
	if err := ctx.Err(); err != nil {
		return Decision{}, ErrEvaluationCanceled
	}
	if err := input.Validate(); err != nil {
		return Decision{}, err
	}
	if input.Snapshot.PolicyRevision != s.revision {
		return Decision{}, ErrPolicyUnavailable
	}
	result, err := s.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Decision{}, ErrEvaluationCanceled
		}
		return Decision{}, fmt.Errorf("%w: evaluation: %v", ErrPolicyUnavailable, err)
	}
	if len(result) != 1 || len(result[0].Expressions) != 1 {
		return Decision{}, fmt.Errorf("%w: decision result count", ErrInvalidDecision)
	}
	raw, err := json.Marshal(result[0].Expressions[0].Value)
	if err != nil {
		return Decision{}, fmt.Errorf("%w: encode result", ErrInvalidDecision)
	}
	decision, err := DecodeDecision(raw)
	if err != nil {
		return Decision{}, err
	}
	if err := decision.Validate(s.revision); err != nil {
		return Decision{}, err
	}
	return decision, nil
}

func (s *PreparedSnapshot) Revision() string {
	if s == nil {
		return ""
	}
	return s.revision
}
func (s *PreparedSnapshot) Digest() string {
	if s == nil {
		return ""
	}
	return s.digest
}
func (s *PreparedSnapshot) AllowsTool(clientID, releaseToolID string) bool {
	if s == nil || s.closed.Load() {
		return false
	}
	if s.visibilityAll {
		return true
	}
	_, ok := s.bindings[clientID][releaseToolID]
	return ok
}
func (s *PreparedSnapshot) Close() {
	if s != nil {
		s.closed.Store(true)
	}
}

// CompatibilityEvaluator is the explicit migration permit for existing
// grants. Legacy Go pre rules continue to enforce their existing blocks while
// this adapter is installed; it must be replaced by a prepared policy before
// new grants are used.
type CompatibilityEvaluator struct {
	mu       sync.RWMutex
	permits  map[string]map[string]struct{}
	bindings map[string]map[string]Permission
	snapshot *PreparedSnapshot
	useOPA   bool
}

func NewCompatibilityEvaluator() *CompatibilityEvaluator {
	return &CompatibilityEvaluator{permits: make(map[string]map[string]struct{}), bindings: make(map[string]map[string]Permission)}
}

// PrepareEmpty installs the fail-closed OPA snapshot used during service
// startup before any explicit compatibility bindings are published.
func (e *CompatibilityEvaluator) PrepareEmpty() error {
	prepared, err := Prepare(context.Background(), Policy{Revision: "compatibility-v1", Permissions: map[string]map[string]Permission{}})
	if err != nil {
		return err
	}
	e.mu.Lock()
	if e.snapshot != nil {
		e.snapshot.Close()
	}
	e.snapshot = prepared
	e.useOPA = true
	e.mu.Unlock()
	return nil
}
func (e *CompatibilityEvaluator) Allow(clientID, releaseToolID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.permits[clientID] == nil {
		e.permits[clientID] = make(map[string]struct{})
	}
	e.permits[clientID][releaseToolID] = struct{}{}
}
func (e *CompatibilityEvaluator) AllowWithTool(clientID, releaseToolID, catalogReleaseID, toolRevisionID string) error {
	e.mu.Lock()
	policy := Policy{Revision: "compatibility-v1", Permissions: make(map[string]map[string]Permission, len(e.bindings))}
	for client, permissions := range e.bindings {
		policy.Permissions[client] = make(map[string]Permission, len(permissions))
		for releaseTool, permission := range permissions {
			policy.Permissions[client][releaseTool] = permission
		}
	}
	if policy.Permissions[clientID] == nil {
		policy.Permissions[clientID] = make(map[string]Permission)
	}
	policy.Permissions[clientID][releaseToolID] = Permission{CatalogReleaseID: catalogReleaseID, ToolRevisionID: toolRevisionID}
	e.mu.Unlock()
	prepared, err := Prepare(context.Background(), policy)
	if err != nil {
		return err
	}
	e.mu.Lock()
	if e.permits[clientID] == nil {
		e.permits[clientID] = make(map[string]struct{})
	}
	e.permits[clientID][releaseToolID] = struct{}{}
	if e.bindings[clientID] == nil {
		e.bindings[clientID] = make(map[string]Permission)
	}
	e.bindings[clientID][releaseToolID] = Permission{CatalogReleaseID: catalogReleaseID, ToolRevisionID: toolRevisionID}
	if e.snapshot != nil {
		e.snapshot.Close()
	}
	e.snapshot = prepared
	e.useOPA = true
	e.mu.Unlock()
	return nil
}
func (e *CompatibilityEvaluator) AllowsTool(clientID, releaseToolID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.useOPA && e.snapshot != nil {
		return e.snapshot.AllowsTool(clientID, releaseToolID)
	}
	_, ok := e.permits[clientID][releaseToolID]
	return ok
}
func (e *CompatibilityEvaluator) Evaluate(ctx context.Context, input Input) (Decision, error) {
	e.mu.RLock()
	snapshot, useOPA := e.snapshot, e.useOPA
	e.mu.RUnlock()
	if useOPA && snapshot != nil {
		return snapshot.Evaluate(ctx, input)
	}
	if err := input.Validate(); err != nil {
		return Decision{}, err
	}
	e.mu.RLock()
	_, permitted := e.permits[input.Principal.ClientID][input.Tool.ReleaseToolID]
	e.mu.RUnlock()
	decision := Decision{ContractVersion: DecisionContractVersion, Allow: permitted, ReasonCode: PolicyDenied, DenyRuleIDs: []string{NoMatchingPermit}, AuditRuleIDs: []string{}, PolicyRevision: "compatibility-v1"}
	if permitted {
		decision.Allow = true
		decision.ReasonCode = PolicyAllowed
		decision.DenyRuleIDs = []string{}
	}
	return decision, nil
}

func EvaluateWithTimeout(ctx context.Context, evaluator Evaluator, input Input, timeout time.Duration) (Decision, error) {
	if timeout <= 0 {
		timeout = 20 * time.Millisecond
	}
	evalCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return evaluator.Evaluate(evalCtx, input)
}
