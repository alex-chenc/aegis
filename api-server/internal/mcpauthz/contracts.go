package mcpauthz

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	InputContractVersion    = "aegis.mcp.authz.input.v1"
	DecisionContractVersion = "aegis.mcp.authz.decision.v1"
	DecisionPath            = "data.aegis.mcp.authz.decision"
	PolicyAllowed           = "POLICY_ALLOWED"
	PolicyDenied            = "POLICY_DENIED"
	NoMatchingPermit        = "NO_MATCHING_PERMIT"
	PolicyUnavailable       = "POLICY_UNAVAILABLE"
	DecisionContractError   = "DECISION_CONTRACT_ERROR"
)

var (
	ErrInvalidInput       = errors.New("invalid MCP authorization input")
	ErrInvalidDecision    = errors.New("invalid MCP authorization decision")
	ErrPolicyUnavailable  = errors.New("MCP authorization policy unavailable")
	ErrEvaluationCanceled = errors.New("MCP authorization evaluation canceled")
)

// Input is constructed by the trusted service boundary. Arguments are the
// only request supplied value and must have passed schema validation first.
type Input struct {
	ContractVersion string    `json:"contract_version"`
	Operation       string    `json:"operation"`
	Request         Request   `json:"request"`
	Principal       Principal `json:"principal"`
	Grant           Grant     `json:"grant"`
	Tool            Tool      `json:"tool"`
	Arguments       any       `json:"arguments"`
	Snapshot        Snapshot  `json:"snapshot"`
}

type Request struct {
	AttemptID        string `json:"attempt_id"`
	DecisionID       string `json:"decision_id"`
	ReceivedAtUnixMS int64  `json:"received_at_unix_ms"`
}

type Principal struct {
	ClientID     string `json:"client_id"`
	CredentialID string `json:"credential_id"`
	User         User   `json:"user"`
}

type User struct {
	Verified bool     `json:"verified"`
	ID       string   `json:"id"`
	Roles    []string `json:"roles"`
}

type Grant struct {
	ID            string         `json:"id"`
	Version       int64          `json:"version"`
	ResourceScope map[string]any `json:"resource_scope"`
}

type Tool struct {
	CatalogReleaseID  string `json:"catalog_release_id"`
	ReleaseToolID     string `json:"release_tool_id"`
	ToolRevisionID    string `json:"tool_revision_id"`
	ServerRevisionID  string `json:"server_revision_id"`
	ExposedName       string `json:"exposed_name"`
	UpstreamName      string `json:"upstream_name"`
	InputSchemaDigest string `json:"input_schema_digest"`
	RiskTier          string `json:"risk_tier"`
}

type Snapshot struct {
	DeploymentGeneration int64  `json:"deployment_generation"`
	PolicyRevision       string `json:"policy_revision"`
}

type Decision struct {
	ContractVersion string   `json:"contract_version"`
	Allow           bool     `json:"allow"`
	ReasonCode      string   `json:"reason_code"`
	DenyRuleIDs     []string `json:"deny_rule_ids"`
	AuditRuleIDs    []string `json:"audit_rule_ids"`
	PolicyRevision  string   `json:"policy_revision"`
}

type Evaluator interface {
	Evaluate(context.Context, Input) (Decision, error)
}

func (i Input) Validate() error {
	if i.ContractVersion != InputContractVersion || i.Operation != "tools/call" {
		return fmt.Errorf("%w: contract or operation", ErrInvalidInput)
	}
	if !nonEmpty(i.Request.AttemptID) || !nonEmpty(i.Request.DecisionID) || i.Request.ReceivedAtUnixMS <= 0 {
		return fmt.Errorf("%w: request", ErrInvalidInput)
	}
	if !nonEmpty(i.Principal.ClientID) || !nonEmpty(i.Principal.CredentialID) || !nonEmpty(i.Grant.ID) || i.Grant.Version < 1 {
		return fmt.Errorf("%w: principal or grant", ErrInvalidInput)
	}
	if !nonEmpty(i.Tool.CatalogReleaseID) || !nonEmpty(i.Tool.ReleaseToolID) || !nonEmpty(i.Tool.ToolRevisionID) || !nonEmpty(i.Tool.ServerRevisionID) || !nonEmpty(i.Tool.ExposedName) || !nonEmpty(i.Tool.UpstreamName) || !nonEmpty(i.Tool.InputSchemaDigest) || !validRisk(i.Tool.RiskTier) {
		return fmt.Errorf("%w: tool", ErrInvalidInput)
	}
	if !nonEmpty(i.Snapshot.PolicyRevision) || i.Snapshot.DeploymentGeneration < 1 {
		return fmt.Errorf("%w: snapshot", ErrInvalidInput)
	}
	if i.Arguments == nil {
		return fmt.Errorf("%w: arguments", ErrInvalidInput)
	}
	return nil
}

func (d Decision) Validate(expectedRevision string) error {
	if d.ContractVersion != DecisionContractVersion || d.ReasonCode != PolicyAllowed && d.ReasonCode != PolicyDenied || strings.TrimSpace(d.PolicyRevision) == "" || d.PolicyRevision != expectedRevision {
		return ErrInvalidDecision
	}
	if d.Allow != (d.ReasonCode == PolicyAllowed) || len(d.DenyRuleIDs) > 64 || len(d.AuditRuleIDs) > 64 {
		return ErrInvalidDecision
	}
	if d.Allow && len(d.DenyRuleIDs) != 0 || !d.Allow && len(d.DenyRuleIDs) == 0 {
		return ErrInvalidDecision
	}
	if !validRuleIDs(d.DenyRuleIDs) || !validRuleIDs(d.AuditRuleIDs) {
		return ErrInvalidDecision
	}
	return nil
}

func validRuleIDs(ids []string) bool {
	for n, id := range ids {
		if id == "" || len(id) > 128 || (n > 0 && ids[n-1] >= id) {
			return false
		}
		if n > 0 && ids[n-1] == id {
			return false
		}
	}
	return true
}

func nonEmpty(value string) bool { return strings.TrimSpace(value) != "" }

func validRisk(value string) bool {
	switch value {
	case "l1", "l2", "l3", "l4":
		return true
	}
	return false
}

// DecodeDecision rejects missing fields, duplicate keys, nulls, extra fields
// and type coercion before converting the untrusted OPA result.
func DecodeDecision(raw []byte) (Decision, error) {
	if len(raw) == 0 || len(raw) > 16<<10 || hasDuplicateJSONKeys(raw) != nil {
		return Decision{}, ErrInvalidDecision
	}
	var fields map[string]json.RawMessage
	if err := decodeStrictObject(raw, &fields); err != nil {
		return Decision{}, fmt.Errorf("%w: %v", ErrInvalidDecision, err)
	}
	expected := map[string]struct{}{"contract_version": {}, "allow": {}, "reason_code": {}, "deny_rule_ids": {}, "audit_rule_ids": {}, "policy_revision": {}}
	if len(fields) != len(expected) {
		return Decision{}, ErrInvalidDecision
	}
	for key := range fields {
		if _, ok := expected[key]; !ok {
			return Decision{}, ErrInvalidDecision
		}
	}
	var d Decision
	if err := json.Unmarshal(fields["contract_version"], &d.ContractVersion); err != nil || string(fields["contract_version"]) == "null" {
		return Decision{}, ErrInvalidDecision
	}
	if err := json.Unmarshal(fields["allow"], &d.Allow); err != nil || string(fields["allow"]) == "null" {
		return Decision{}, ErrInvalidDecision
	}
	if err := json.Unmarshal(fields["reason_code"], &d.ReasonCode); err != nil || string(fields["reason_code"]) == "null" {
		return Decision{}, ErrInvalidDecision
	}
	if err := json.Unmarshal(fields["deny_rule_ids"], &d.DenyRuleIDs); err != nil || d.DenyRuleIDs == nil {
		return Decision{}, ErrInvalidDecision
	}
	if err := json.Unmarshal(fields["audit_rule_ids"], &d.AuditRuleIDs); err != nil || d.AuditRuleIDs == nil {
		return Decision{}, ErrInvalidDecision
	}
	if err := json.Unmarshal(fields["policy_revision"], &d.PolicyRevision); err != nil || string(fields["policy_revision"]) == "null" {
		return Decision{}, ErrInvalidDecision
	}
	return d, nil
}

// hasDuplicateJSONKeys walks the JSON token stream before unmarshalling into
// maps. json.Unmarshal intentionally keeps the last duplicate key, which
// would make an attacker able to make OPA's decision mean something different
// to the application parser.
func hasDuplicateJSONKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := scanJSONValue(dec); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON")
		}
		return err
	}
	return nil
}

func scanJSONValue(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			seen := make(map[string]struct{})
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok {
					return errors.New("object key is not a string")
				}
				if _, exists := seen[name]; exists {
					return fmt.Errorf("duplicate key %q", name)
				}
				seen[name] = struct{}{}
				if err := scanJSONValue(dec); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil {
				return err
			}
			if end != json.Delim('}') {
				return errors.New("invalid object")
			}
		case '[':
			for dec.More() {
				if err := scanJSONValue(dec); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil {
				return err
			}
			if end != json.Delim(']') {
				return errors.New("invalid array")
			}
		default:
			return errors.New("unexpected delimiter")
		}
	}
	return nil
}

func decodeStrictObject(raw []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON")
		}
		return err
	}
	return nil
}

func CanonicalDigest(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func SortRuleIDs(ids []string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}

func NowRequest(attemptID, decisionID string, now time.Time) Request {
	return Request{AttemptID: attemptID, DecisionID: decisionID, ReceivedAtUnixMS: now.UnixMilli()}
}
