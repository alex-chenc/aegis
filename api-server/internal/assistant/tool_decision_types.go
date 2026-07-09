package assistant

import (
	"bytes"
	"encoding/json"
	"strings"
)

// IntentObject describes a business object extracted from a user request.
type IntentObject struct {
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	Selector string `json:"selector,omitempty"`
	Category string `json:"category,omitempty"`
	Source   string `json:"source,omitempty"`
}

// UnmarshalJSON accepts both the structured object form and the bare string
// form (e.g. "cve" or "CVE-2024-1234") that LLMs frequently emit for intent
// objects. Without this tolerance a string element aborts the whole intent
// decomposition with a json unmarshal error.
func (o *IntentObject) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		o.Source = "llm"
		o.Type = s
		return nil
	}
	type intentObjectAlias IntentObject
	var alias intentObjectAlias
	if err := json.Unmarshal(trimmed, &alias); err != nil {
		return err
	}
	*o = IntentObject(alias)
	return nil
}

// IntentScope describes the execution or query scope of a request.
type IntentScope struct {
	Kind      string   `json:"kind,omitempty"`
	ObjectIDs []string `json:"object_ids,omitempty"`
	Source    string   `json:"source,omitempty"`
}

// MissingInfo describes missing information that blocks a safe tool decision.
type MissingInfo struct {
	Field    string `json:"field"`
	Reason   string `json:"reason"`
	Question string `json:"question,omitempty"`
}

// UnmarshalJSON accepts both the structured object form and the bare string
// form that LLMs frequently emit for missing_info entries, keeping intent
// decomposition resilient to loosely-typed model output.
func (m *MissingInfo) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		m.Field = strings.TrimSpace(s)
		return nil
	}
	type missingInfoAlias MissingInfo
	var alias missingInfoAlias
	if err := json.Unmarshal(trimmed, &alias); err != nil {
		return err
	}
	*m = MissingInfo(alias)
	return nil
}

// IntentBreakdown is the stable intermediate intent model used before tool decisions.
type IntentBreakdown struct {
	Goal                  string                 `json:"goal"`
	Domains               []string               `json:"domains"`
	Actions               []string               `json:"actions"`
	Objects               []IntentObject         `json:"objects"`
	Scope                 IntentScope            `json:"scope"`
	Parameters            IntentParameters       `json:"parameters,omitempty"`
	Constraints           []string               `json:"constraints,omitempty"`
	MissingInfo           []MissingInfo          `json:"missing_info,omitempty"`
	RequiresWrite         bool                   `json:"requires_write"`
	RiskHint              string                 `json:"risk_hint"`
	CandidateCapabilities []string               `json:"candidate_capabilities"`
	NeedClarification     bool                   `json:"need_clarification"`
	ClarifyingQuestion    string                 `json:"clarifying_question,omitempty"`
	Reason                string                 `json:"reason"`
	Confidence            float64                `json:"confidence"`
	Raw                   map[string]interface{} `json:"raw,omitempty"`
}

// IntentParameters preserves arbitrary parameters explicitly extracted by the
// LLM. Business-specific schemas belong to tool descriptors, not the shared
// intent model.
type IntentParameters map[string]interface{}

// ToolUseContract captures the backend constraints for using a tool.
type ToolUseContract struct {
	ToolName                   string                `json:"tool_name"`
	Capability                 string                `json:"capability"`
	Domain                     string                `json:"domain"`
	AllowedIntents             []string              `json:"allowed_intents,omitempty"`
	DeniedIntents              []string              `json:"denied_intents,omitempty"`
	Actions                    []string              `json:"actions,omitempty"`
	ObjectTypes                []string              `json:"object_types,omitempty"`
	RequiredEntities           []string              `json:"required_entities,omitempty"`
	OptionalEntities           []string              `json:"optional_entities,omitempty"`
	Preconditions              []string              `json:"preconditions,omitempty"`
	ArgBindings                []ArgBindingRule      `json:"arg_bindings,omitempty"`
	NegativeCases              []string              `json:"negative_cases,omitempty"`
	StateTransitions           []ToolStateTransition `json:"state_transitions,omitempty"`
	Postconditions             []string              `json:"postconditions,omitempty"`
	ResultValidators           []string              `json:"result_validators,omitempty"`
	NextCapabilities           []string              `json:"next_capabilities,omitempty"`
	WorkflowHints              []string              `json:"workflow_hints,omitempty"`
	Risk                       string                `json:"risk"`
	RequiresExplicitUserIntent bool                  `json:"requires_explicit_user_intent"`
	RequiresApproval           bool                  `json:"requires_approval"`
	DecisionExamples           []ToolDecisionExample `json:"decision_examples,omitempty"`
}

type ArgBindingRule struct {
	ArgName       string   `json:"arg_name"`
	Entity        string   `json:"entity"`
	SourceOrder   []string `json:"source_order"`
	Required      bool     `json:"required"`
	DefaultPolicy string   `json:"default_policy,omitempty"`
}

type ToolStateTransition struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition"`
}

type ToolDecisionExample struct {
	UserInput string `json:"user_input"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason,omitempty"`
}

type HardGateResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason,omitempty"`
}

type ArgSource struct {
	SourceType string  `json:"source_type"`
	SourceRef  string  `json:"source_ref"`
	Confidence float64 `json:"confidence"`
}

type ToolDecisionRecord struct {
	TraceID         string                 `json:"trace_id"`
	ToolName        string                 `json:"tool_name"`
	Capability      string                 `json:"capability"`
	Decision        string                 `json:"decision"`
	Score           float64                `json:"score"`
	RequiresWrite   bool                   `json:"requires_write"`
	HardGateResults []HardGateResult       `json:"hard_gate_results"`
	ArgSources      map[string]ArgSource   `json:"arg_sources,omitempty"`
	ApprovalState   string                 `json:"approval_state,omitempty"`
	Reason          string                 `json:"reason"`
	Evidence        map[string]interface{} `json:"evidence,omitempty"`
}

type EvidencePolicy struct {
	RequireToolEvidence     bool   `json:"require_tool_evidence"`
	RequirePostcondition    bool   `json:"require_postcondition"`
	MissingEvidenceBehavior string `json:"missing_evidence_behavior"`
}

// ToolExecutionPlan is a legacy type name. In the Assistant pure-agent flow it
// is an authorization artifact only; its steps describe allowed tools and are
// never forwarded as agent-runtime execution steps.
type ToolExecutionPlan struct {
	Goal                string               `json:"goal"`
	NeedClarification   bool                 `json:"need_clarification"`
	ClarifyingQuestion  string               `json:"clarifying_question,omitempty"`
	Steps               []ToolPlanStep       `json:"steps"`
	EvidencePolicy      EvidencePolicy       `json:"evidence_policy"`
	DecisionTraceID     string               `json:"decision_trace_id"`
	DecisionRecords     []ToolDecisionRecord `json:"decision_records,omitempty"`
	RejectedToolRecords []ToolDecisionRecord `json:"rejected_tool_records,omitempty"`
}

type ToolPlanStep struct {
	StepID           string                 `json:"step_id"`
	ToolName         string                 `json:"tool_name"`
	Capability       string                 `json:"capability"`
	Args             map[string]interface{} `json:"args,omitempty"`
	Risk             string                 `json:"risk"`
	RequiresApproval bool                   `json:"requires_approval"`
	Reason           string                 `json:"reason"`
	ArgSources       map[string]ArgSource   `json:"arg_sources,omitempty"`
	Preconditions    []string               `json:"preconditions,omitempty"`
	Postconditions   []string               `json:"postconditions,omitempty"`
	OnSuccess        []string               `json:"on_success,omitempty"`
	Condition        string                 `json:"condition,omitempty"`
}

func (p ToolExecutionPlan) ToolNames() []string {
	names := make([]string, 0, len(p.Steps))
	seen := make(map[string]bool, len(p.Steps))
	for _, step := range p.Steps {
		if step.ToolName == "" || seen[step.ToolName] {
			continue
		}
		seen[step.ToolName] = true
		names = append(names, step.ToolName)
	}
	return names
}
