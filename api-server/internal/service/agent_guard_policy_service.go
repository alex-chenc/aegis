package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path"
	"regexp"
	"sort"
	"strings"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

var (
	ErrAgentGuardPolicyWriteDisabled = errors.New("agent guard policy writes are disabled")
	ErrAgentGuardPolicyInvalid       = errors.New("agent guard policy is invalid")
)

var (
	agentGuardPolicyKeyPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]{0,126}[a-z0-9])?$`)
	agentGuardDNSLabelPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	agentGuardRuleIDPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
	agentGuardShellMetaPattern = regexp.MustCompile(`[;&|` + "`" + `$><\r\n]`)
)

type agentGuardPolicyCatalog interface {
	GetRule(context.Context, string, int64) (*model.AgentBehaviorRuleDefinition, error)
}

type agentGuardPolicyStore interface {
	CreateDraft(context.Context, *model.AgentGuardPolicy) error
	GetByID(context.Context, uuid.UUID) (*model.AgentGuardPolicy, error)
	UpdateDraft(context.Context, uuid.UUID, model.AgentGuardPolicyDraftUpdate) (*model.AgentGuardPolicy, error)
}

// AgentGuardPolicyService is the P0 control-plane boundary for policy drafts.
// It deliberately contains no publish path: P0 keeps publishing and enforcement
// disabled while still exercising the complete server-side validation contract.
type AgentGuardPolicyService struct {
	catalog      agentGuardPolicyCatalog
	store        agentGuardPolicyStore
	writeEnabled bool
}

func NewAgentGuardPolicyService(
	catalog agentGuardPolicyCatalog,
	store agentGuardPolicyStore,
	writeEnabled bool,
) *AgentGuardPolicyService {
	return &AgentGuardPolicyService{
		catalog:      catalog,
		store:        store,
		writeEnabled: writeEnabled,
	}
}

func (s *AgentGuardPolicyService) Validate(
	ctx context.Context,
	request model.AgentGuardPolicyDraftRequest,
) model.AgentGuardPolicyValidationPreview {
	normalizeAgentGuardPolicyRequest(&request)
	preview := model.AgentGuardPolicyValidationPreview{
		Errors:            []model.AgentGuardPolicyValidationIssue{},
		Warnings:          []model.AgentGuardPolicyValidationIssue{},
		DefinitionDigests: map[string]string{},
	}
	addError := func(field, code, message string) {
		preview.Errors = append(preview.Errors, model.AgentGuardPolicyValidationIssue{
			Field: field, Code: code, Message: message,
		})
	}

	if !agentGuardPolicyKeyPattern.MatchString(request.PolicyKey) {
		addError("policy_key", "invalid_format", "must be a stable lowercase identifier")
	}
	if request.Name == "" || len(request.Name) > 255 {
		addError("name", "invalid_length", "must contain 1 to 255 characters")
	}
	if request.Priority < 0 || request.Priority > 10000 {
		addError("priority", "out_of_range", "must be between 0 and 10000")
	}

	validateAgentGuardTargets(request.Targets, addError)
	validateAgentGuardCollection(request.Collection, addError)

	seenRuleIDs := make(map[string]string)
	for index, override := range request.BuiltinRuleOverrides {
		field := fmt.Sprintf("builtin_rule_overrides[%d]", index)
		if _, duplicate := seenRuleIDs["builtin:"+override.RuleKey]; duplicate {
			addError(field+".rule_key", "duplicate", "builtin rule override is duplicated")
		}
		seenRuleIDs["builtin:"+override.RuleKey] = field
		if override.RuleKey == "" || override.RuleVersion < 1 {
			addError(field, "invalid_reference", "rule_key and positive rule_version are required")
			continue
		}
		rule, err := s.catalog.GetRule(ctx, override.RuleKey, override.RuleVersion)
		if err != nil {
			addError(field, "rule_not_found", "referenced builtin rule version does not exist")
			continue
		}
		preview.DefinitionDigests[fmt.Sprintf("%s@%d", override.RuleKey, override.RuleVersion)] = rule.Digest
		validateAgentGuardSeverityAction(field, override.SeverityOverride, override.ActionOverride, addError)
		if override.ActionOverride == "deny_and_freeze" &&
			override.SeverityOverride != "high" && override.SeverityOverride != "critical" {
			addError(field+".action_override", "unsafe_action", "deny_and_freeze requires high or critical severity")
		}
		validateAgentGuardParameters(field+".parameters", override.Parameters, addError)
		validateAgentGuardParametersSchema(field+".parameters", override.Parameters, rule.ParametersSchema, addError)
	}

	allowedAtomicRules := stringSet("protected_resource_access", "process_execution", "network_connect", "identity_change")
	allowedOperations := stringSet(
		"read", "write", "create", "delete", "rename", "execute", "connect",
		"setuid", "setgid", "capability_change", "namespace_change",
	)
	for index, rule := range request.AtomicRules {
		field := fmt.Sprintf("atomic_rules[%d]", index)
		validateAgentGuardUniqueRuleID(field, rule.RuleID, seenRuleIDs, addError)
		if !allowedAtomicRules[rule.Rule] {
			addError(field+".rule", "unknown_enum", "unsupported atomic rule")
		}
		if len(rule.Operations) == 0 {
			addError(field+".operations", "required", "at least one operation is required")
		}
		for operationIndex, operation := range rule.Operations {
			if !allowedOperations[operation] {
				addError(fmt.Sprintf("%s.operations[%d]", field, operationIndex), "unknown_enum", "unsupported operation")
			}
		}
		validateAgentGuardSeverityAction(field, rule.Severity, rule.Action, addError)
		if rule.Action == "deny_and_freeze" && rule.Severity != "high" && rule.Severity != "critical" {
			addError(field+".action", "unsafe_action", "deny_and_freeze requires high or critical severity")
		}
		validateAgentGuardResource(field+".resource", rule.Resource, addError)
		validateAgentGuardParameters(field+".parameters", rule.Parameters, addError)
	}

	allowedGroupKeys := stringSet("host_id", "instance_id", "session_id", "execution_unit_id", "process_chain")
	allowedEvidence := stringSet("process", "file", "network", "identity", "isolation", "rule_hit")
	for index, rule := range request.CorrelationRules {
		field := fmt.Sprintf("correlation_rules[%d]", index)
		validateAgentGuardUniqueRuleID(field, rule.RuleID, seenRuleIDs, addError)
		if rule.WindowSeconds < 1 || rule.WindowSeconds > 3600 {
			addError(field+".window_seconds", "out_of_range", "must be between 1 and 3600")
		}
		validateAgentGuardSeverityAction(field, rule.Severity, rule.Action, addError)
		for groupIndex, group := range rule.GroupKeys {
			if !allowedGroupKeys[group] {
				addError(fmt.Sprintf("%s.group_keys[%d]", field, groupIndex), "unknown_enum", "unsupported group key")
			}
		}
		for evidenceIndex, evidence := range rule.RequiredEvidence {
			if !allowedEvidence[evidence] {
				addError(fmt.Sprintf("%s.required_evidence[%d]", field, evidenceIndex), "unknown_enum", "unsupported evidence category")
			}
		}
	}

	allowedEscapeRules := stringSet(
		"join_external_namespace", "escape_cgroup", "write_host_namespace",
		"mount_host_filesystem", "unexpected_privilege_gain",
	)
	for index, rule := range request.EscapeRules {
		field := fmt.Sprintf("escape_rules[%d]", index)
		validateAgentGuardUniqueRuleID(field, rule.RuleID, seenRuleIDs, addError)
		if !allowedEscapeRules[rule.Rule] {
			addError(field+".rule", "unknown_enum", "unsupported escape rule")
		}
		validateAgentGuardSeverityAction(field, rule.Severity, rule.Action, addError)
		if rule.Action == "deny_and_freeze" && rule.Severity != "high" && rule.Severity != "critical" {
			addError(field+".action", "unsafe_action", "deny_and_freeze requires high or critical severity")
		}
		validateAgentGuardParameters(field+".parameters", rule.Parameters, addError)
	}

	if request.Analysis.AIOnlyActionCeiling != "audit" && request.Analysis.AIOnlyActionCeiling != "alert" {
		addError("analysis.ai_only_action_ceiling", "unsafe_action", "must be audit or alert")
	}
	if request.Analysis.EvidenceWindowSeconds < 30 || request.Analysis.EvidenceWindowSeconds > 3600 {
		addError("analysis.evidence_window_seconds", "out_of_range", "must be between 30 and 3600")
	}
	for index, severity := range request.Analysis.TriggerSeverities {
		if !agentGuardSeverities()[severity] {
			addError(fmt.Sprintf("analysis.trigger_severities[%d]", index), "unknown_enum", "unsupported severity")
		}
	}
	if request.FreezeTimeoutSeconds < 30 || request.FreezeTimeoutSeconds > 900 {
		addError("freeze_timeout_seconds", "out_of_range", "must be between 30 and 900")
	}

	preview.Valid = len(preview.Errors) == 0
	if !preview.Valid {
		return preview
	}

	preview.CompiledPreview = compileAgentGuardPolicyPreview(request, preview.DefinitionDigests)
	digest, err := canonicalAgentGuardDigest(request, preview.CompiledPreview, preview.DefinitionDigests)
	if err != nil {
		addError("", "canonical_encoding_failed", "policy could not be canonically encoded")
		preview.Valid = false
		return preview
	}
	preview.Digest = digest
	return preview
}

func (s *AgentGuardPolicyService) CreateDraft(
	ctx context.Context,
	request model.AgentGuardPolicyDraftRequest,
	createdBy string,
) (*model.AgentGuardPolicy, model.AgentGuardPolicyValidationPreview, error) {
	preview := s.Validate(ctx, request)
	if !preview.Valid {
		return nil, preview, ErrAgentGuardPolicyInvalid
	}
	if !s.writeEnabled {
		return nil, preview, ErrAgentGuardPolicyWriteDisabled
	}
	normalizeAgentGuardPolicyRequest(&request)
	policy, err := buildAgentGuardPolicy(request, preview, createdBy)
	if err != nil {
		return nil, preview, err
	}
	if err := s.store.CreateDraft(ctx, policy); err != nil {
		return nil, preview, err
	}
	return policy, preview, nil
}

func (s *AgentGuardPolicyService) UpdateDraft(
	ctx context.Context,
	id uuid.UUID,
	request model.AgentGuardPolicyDraftRequest,
) (*model.AgentGuardPolicy, model.AgentGuardPolicyValidationPreview, error) {
	preview := s.Validate(ctx, request)
	if !preview.Valid {
		return nil, preview, ErrAgentGuardPolicyInvalid
	}
	if !s.writeEnabled {
		return nil, preview, ErrAgentGuardPolicyWriteDisabled
	}
	existing, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, preview, err
	}
	if request.PolicyKey != existing.PolicyKey {
		preview.Valid = false
		preview.Errors = append(preview.Errors, model.AgentGuardPolicyValidationIssue{
			Field: "policy_key", Code: "immutable", Message: "policy_key cannot change",
		})
		return nil, preview, ErrAgentGuardPolicyInvalid
	}
	normalizeAgentGuardPolicyRequest(&request)
	update, err := buildAgentGuardPolicyUpdate(request, preview)
	if err != nil {
		return nil, preview, err
	}
	policy, err := s.store.UpdateDraft(ctx, id, update)
	return policy, preview, err
}

func buildAgentGuardPolicy(
	request model.AgentGuardPolicyDraftRequest,
	preview model.AgentGuardPolicyValidationPreview,
	createdBy string,
) (*model.AgentGuardPolicy, error) {
	update, err := buildAgentGuardPolicyUpdate(request, preview)
	if err != nil {
		return nil, err
	}
	return &model.AgentGuardPolicy{
		ID:        uuid.New(),
		PolicyKey: request.PolicyKey,
		// The repository allocates the next immutable version transactionally.
		Version:              0,
		Name:                 update.Name,
		Description:          update.Description,
		Status:               "draft",
		Priority:             update.Priority,
		Targets:              update.Targets,
		CollectionPolicy:     update.CollectionPolicy,
		BuiltinRuleOverrides: update.BuiltinRuleOverrides,
		AtomicRules:          update.AtomicRules,
		CorrelationRules:     update.CorrelationRules,
		AnalysisPolicy:       update.AnalysisPolicy,
		EscapeRules:          update.EscapeRules,
		FreezeTimeoutSeconds: update.FreezeTimeoutSeconds,
		CompiledPreview:      update.CompiledPreview,
		Digest:               update.Digest,
		CreatedBy:            strings.TrimSpace(createdBy),
	}, nil
}

func buildAgentGuardPolicyUpdate(
	request model.AgentGuardPolicyDraftRequest,
	preview model.AgentGuardPolicyValidationPreview,
) (model.AgentGuardPolicyDraftUpdate, error) {
	marshal := func(value any) (datatypes.JSON, error) {
		encoded, err := json.Marshal(value)
		return datatypes.JSON(encoded), err
	}
	targets, err := marshal(request.Targets)
	if err != nil {
		return model.AgentGuardPolicyDraftUpdate{}, err
	}
	collection, err := marshal(request.Collection)
	if err != nil {
		return model.AgentGuardPolicyDraftUpdate{}, err
	}
	overrides, err := marshal(request.BuiltinRuleOverrides)
	if err != nil {
		return model.AgentGuardPolicyDraftUpdate{}, err
	}
	atomic, err := marshal(request.AtomicRules)
	if err != nil {
		return model.AgentGuardPolicyDraftUpdate{}, err
	}
	correlation, err := marshal(request.CorrelationRules)
	if err != nil {
		return model.AgentGuardPolicyDraftUpdate{}, err
	}
	analysis, err := marshal(request.Analysis)
	if err != nil {
		return model.AgentGuardPolicyDraftUpdate{}, err
	}
	escapeRules, err := marshal(request.EscapeRules)
	if err != nil {
		return model.AgentGuardPolicyDraftUpdate{}, err
	}
	compiled, err := marshal(preview.CompiledPreview)
	if err != nil {
		return model.AgentGuardPolicyDraftUpdate{}, err
	}
	return model.AgentGuardPolicyDraftUpdate{
		Name:                 request.Name,
		Description:          request.Description,
		Priority:             request.Priority,
		Targets:              targets,
		CollectionPolicy:     collection,
		BuiltinRuleOverrides: overrides,
		AtomicRules:          atomic,
		CorrelationRules:     correlation,
		AnalysisPolicy:       analysis,
		EscapeRules:          escapeRules,
		FreezeTimeoutSeconds: request.FreezeTimeoutSeconds,
		CompiledPreview:      compiled,
		Digest:               preview.Digest,
	}, nil
}

func validateAgentGuardTargets(targets model.AgentGuardPolicyTargets, addError func(string, string, string)) {
	if len(targets.AgentTypes) == 0 {
		addError("targets.agent_types", "required", "at least one agent type or * is required")
	}
	allowed := stringSet("codex", "openclaw", "hermes", "claude-code", "opencode", "gemini-cli", "*")
	for index, agentType := range targets.AgentTypes {
		if !allowed[agentType] {
			addError(fmt.Sprintf("targets.agent_types[%d]", index), "unknown_enum", "unsupported agent type")
		}
	}
	for index, value := range append(append([]string{}, targets.HostIDs...), targets.HostGroupIDs...) {
		if _, err := uuid.Parse(value); err != nil {
			addError(fmt.Sprintf("targets.ids[%d]", index), "invalid_uuid", "host and host group IDs must be UUIDs")
		}
	}
}

func validateAgentGuardCollection(collection model.AgentGuardCollectionPolicy, addError func(string, string, string)) {
	allowedCategories := stringSet(
		"process", "file", "network", "identity", "persistence",
		"isolation", "kernel", "ipc", "tool", "control",
	)
	if len(collection.Categories) == 0 {
		addError("collection.categories", "required", "at least one category is required")
	}
	for index, category := range collection.Categories {
		if !allowedCategories[category] {
			addError(fmt.Sprintf("collection.categories[%d]", index), "unknown_enum", "unsupported collection category")
		}
	}
	if collection.ToolAdapterEnabled && !containsStringValue(collection.Categories, "tool") {
		addError("collection.tool_adapter_enabled", "missing_dependency", "tool category is required when the tool adapter is enabled")
	}
	if collection.CommandArgv != "redacted" && collection.CommandArgv != "disabled" {
		addError("collection.command_argv", "unsafe_collection", "must be redacted or disabled")
	}
	if collection.FileContent != "disabled" {
		addError("collection.file_content", "unsafe_collection", "file content collection is forbidden")
	}
	if collection.NetworkContent != "disabled" {
		addError("collection.network_content", "unsafe_collection", "network content collection is forbidden")
	}
	for key, seconds := range collection.Aggregation {
		if strings.TrimSpace(key) == "" || seconds < 1 || seconds > 3600 {
			addError("collection.aggregation."+key, "out_of_range", "aggregation windows must be between 1 and 3600 seconds")
		}
	}
}

func containsStringValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validateAgentGuardUniqueRuleID(
	field string,
	ruleID string,
	seen map[string]string,
	addError func(string, string, string),
) {
	if !agentGuardRuleIDPattern.MatchString(ruleID) {
		addError(field+".rule_id", "invalid_format", "a stable rule_id is required")
		return
	}
	key := "custom:" + ruleID
	if _, duplicate := seen[key]; duplicate {
		addError(field+".rule_id", "duplicate", "rule_id must be unique within the policy")
	}
	seen[key] = field
}

func validateAgentGuardSeverityAction(
	field string,
	severity string,
	action string,
	addError func(string, string, string),
) {
	if severity != "" && !agentGuardSeverities()[severity] {
		addError(field+".severity", "unknown_enum", "unsupported severity")
	}
	if action != "" && !agentGuardActions()[action] {
		addError(field+".action", "unknown_enum", "unsupported action")
	}
}

func validateAgentGuardResource(field string, resource map[string]any, addError func(string, string, string)) {
	resourceType, _ := resource["type"].(string)
	if resourceType == "" {
		addError(field+".type", "required", "resource type is required")
	}
	if resourceType == "file" {
		value, _ := resource["path"].(string)
		if value == "" || !path.IsAbs(value) {
			addError(field+".path", "invalid_path", "file resource path must be absolute")
		} else {
			validateAgentGuardPath(field+".path", value, addError)
		}
	}
	match, _ := resource["match"].(string)
	if match != "" && match != "exact" && match != "glob" && match != "prefix" {
		addError(field+".match", "unknown_enum", "resource match must be exact, prefix, or glob")
	}
	if match == "glob" {
		value, _ := resource["path"].(string)
		if !validAgentGuardGlob(value) {
			addError(field+".path", "invalid_glob", "glob uses unsupported syntax")
		}
	}
}

func validateAgentGuardParameters(field string, parameters map[string]any, addError func(string, string, string)) {
	for key, value := range parameters {
		switch key {
		case "trusted_cidrs":
			for index, item := range stringSlice(value) {
				if _, _, err := net.ParseCIDR(item); err != nil {
					addError(fmt.Sprintf("%s.trusted_cidrs[%d]", field, index), "invalid_cidr", "trusted CIDR is invalid")
				}
			}
		case "trusted_domains":
			for index, item := range stringSlice(value) {
				if !validAgentGuardDomain(item) {
					addError(fmt.Sprintf("%s.trusted_domains[%d]", field, index), "invalid_domain", "trusted domain must use DNS label boundaries")
				}
			}
		default:
			if strings.Contains(key, "path") || strings.Contains(key, "executable") {
				for index, item := range stringSlice(value) {
					validateAgentGuardPath(fmt.Sprintf("%s.%s[%d]", field, key, index), item, addError)
				}
			}
		}
	}
}

func validateAgentGuardParametersSchema(
	field string,
	parameters map[string]any,
	rawSchema datatypes.JSON,
	addError func(string, string, string),
) {
	var schema map[string]any
	if len(rawSchema) == 0 || json.Unmarshal(rawSchema, &schema) != nil {
		addError(field, "schema_invalid", "builtin rule parameter schema is invalid")
		return
	}
	properties, _ := schema["properties"].(map[string]any)
	additionalAllowed, hasAdditional := schema["additionalProperties"].(bool)
	for key, value := range parameters {
		propertySchema, exists := properties[key].(map[string]any)
		if !exists {
			if hasAdditional && !additionalAllowed {
				addError(field+"."+key, "unknown_parameter", "parameter is not declared by the builtin rule schema")
			}
			continue
		}
		validateAgentGuardSchemaValue(field+"."+key, value, propertySchema, addError)
	}
	for _, required := range stringSlice(schema["required"]) {
		if _, exists := parameters[required]; !exists {
			addError(field+"."+required, "required", "required builtin rule parameter is missing")
		}
	}
}

func validateAgentGuardSchemaValue(
	field string,
	value any,
	schema map[string]any,
	addError func(string, string, string),
) {
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, allowed := range enum {
			if fmt.Sprint(allowed) == fmt.Sprint(value) {
				found = true
				break
			}
		}
		if !found {
			addError(field, "unknown_enum", "value is not permitted by the builtin rule schema")
			return
		}
	}

	switch schema["type"] {
	case "boolean":
		if _, ok := value.(bool); !ok {
			addError(field, "type_mismatch", "value must be a boolean")
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			addError(field, "type_mismatch", "value must be a string")
			return
		}
		if maximum, ok := numberAsInt(schema["maxLength"]); ok && len(text) > maximum {
			addError(field, "out_of_range", "string exceeds the allowed length")
		}
	case "integer":
		number, ok := numberAsInt(value)
		if !ok {
			addError(field, "type_mismatch", "value must be an integer")
			return
		}
		if minimum, ok := numberAsInt(schema["minimum"]); ok && number < minimum {
			addError(field, "out_of_range", "value is below the allowed minimum")
		}
		if maximum, ok := numberAsInt(schema["maximum"]); ok && number > maximum {
			addError(field, "out_of_range", "value exceeds the allowed maximum")
		}
	case "array":
		items, ok := anySlice(value)
		if !ok {
			addError(field, "type_mismatch", "value must be an array")
			return
		}
		itemSchema, _ := schema["items"].(map[string]any)
		seen := make(map[string]struct{}, len(items))
		for index, item := range items {
			validateAgentGuardSchemaValue(fmt.Sprintf("%s[%d]", field, index), item, itemSchema, addError)
			if unique, _ := schema["uniqueItems"].(bool); unique {
				encoded, _ := json.Marshal(item)
				key := string(encoded)
				if _, exists := seen[key]; exists {
					addError(field, "duplicate", "array items must be unique")
				}
				seen[key] = struct{}{}
			}
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			addError(field, "type_mismatch", "value must be an object")
		}
	}
}

func validateAgentGuardPath(field, value string, addError func(string, string, string)) {
	if !path.IsAbs(value) || strings.Contains(value, "..") || agentGuardShellMetaPattern.MatchString(value) {
		addError(field, "invalid_path", "path must be absolute and cannot contain .. or shell metacharacters")
	}
}

func validAgentGuardGlob(value string) bool {
	if strings.ContainsAny(value, "?[\\{}") || strings.Contains(value, "***") {
		return false
	}
	if strings.Count(value, "**") > 1 {
		return false
	}
	return !strings.Contains(value, "**") || strings.HasSuffix(value, "/**")
}

func validAgentGuardDomain(value string) bool {
	value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if value == "" || strings.Contains(value, "*") || net.ParseIP(value) != nil {
		return false
	}
	labels := strings.Split(value, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !agentGuardDNSLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

func compileAgentGuardPolicyPreview(
	request model.AgentGuardPolicyDraftRequest,
	definitionDigests map[string]string,
) map[string]any {
	positions := make([]map[string]any, 0,
		len(request.BuiltinRuleOverrides)+len(request.AtomicRules)+len(request.CorrelationRules)+len(request.EscapeRules)+1)
	for _, rule := range request.BuiltinRuleOverrides {
		positions = append(positions, map[string]any{
			"rule_id": rule.RuleKey, "rule_version": rule.RuleVersion, "position": "dc_builtin_evaluator",
		})
	}
	for _, rule := range request.AtomicRules {
		positions = append(positions, map[string]any{"rule_id": rule.RuleID, "position": "agent_local_atomic"})
	}
	for _, rule := range request.CorrelationRules {
		positions = append(positions, map[string]any{"rule_id": rule.RuleID, "position": "dc_correlation"})
	}
	for _, rule := range request.EscapeRules {
		positions = append(positions, map[string]any{"rule_id": rule.RuleID, "position": "agent_local_escape"})
	}
	if request.Analysis.Enabled {
		positions = append(positions, map[string]any{"rule_id": "analysis", "position": "dc_async_analysis"})
	}
	sort.Slice(positions, func(i, j int) bool {
		return fmt.Sprint(positions[i]["rule_id"]) < fmt.Sprint(positions[j]["rule_id"])
	})
	return map[string]any{
		"schema":              "aegis.agent_guard.policy_preview.v1",
		"mode":                "monitor_only",
		"execution_positions": positions,
		"definition_digests":  definitionDigests,
		"conflict_resolution": map[string]any{
			"policy_priority": request.Priority,
			"tie_breaker":     "policy_key_then_rule_id",
		},
	}
}

func canonicalAgentGuardDigest(
	request model.AgentGuardPolicyDraftRequest,
	compiled map[string]any,
	definitionDigests map[string]string,
) (string, error) {
	document := struct {
		Request           model.AgentGuardPolicyDraftRequest `json:"request"`
		Compiled          map[string]any                     `json:"compiled"`
		DefinitionDigests map[string]string                  `json:"definition_digests"`
	}{
		Request: request, Compiled: compiled, DefinitionDigests: definitionDigests,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func normalizeAgentGuardPolicyRequest(request *model.AgentGuardPolicyDraftRequest) {
	request.PolicyKey = strings.ToLower(strings.TrimSpace(request.PolicyKey))
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	for index := range request.Targets.AgentTypes {
		request.Targets.AgentTypes[index] = strings.ToLower(strings.TrimSpace(request.Targets.AgentTypes[index]))
	}
	sort.Strings(request.Targets.AgentTypes)
	sort.Strings(request.Targets.HostIDs)
	sort.Strings(request.Targets.HostGroupIDs)
	sort.Strings(request.Targets.ProfileKeys)
	sort.Strings(request.Collection.Categories)
	for index := range request.Analysis.TriggerSeverities {
		request.Analysis.TriggerSeverities[index] = strings.ToLower(strings.TrimSpace(request.Analysis.TriggerSeverities[index]))
	}
	sort.Strings(request.Analysis.TriggerSeverities)
	if request.Analysis.AIOnlyActionCeiling == "" {
		request.Analysis.AIOnlyActionCeiling = "audit"
	}
	if request.Analysis.EvidenceWindowSeconds == 0 {
		request.Analysis.EvidenceWindowSeconds = 300
	}
	if request.FreezeTimeoutSeconds == 0 {
		request.FreezeTimeoutSeconds = 300
	}
}

func stringSet(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func agentGuardSeverities() map[string]bool {
	return stringSet("info", "low", "medium", "high", "critical")
}

func agentGuardActions() map[string]bool {
	return stringSet("audit", "alert", "would_deny", "deny", "deny_and_freeze")
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func anySlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []string:
		result := make([]any, len(typed))
		for index := range typed {
			result[index] = typed[index]
		}
		return result, true
	case []int:
		result := make([]any, len(typed))
		for index := range typed {
			result[index] = typed[index]
		}
		return result, true
	default:
		return nil, false
	}
}

func numberAsInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		converted := int(typed)
		return converted, float64(converted) == typed
	default:
		return 0, false
	}
}
