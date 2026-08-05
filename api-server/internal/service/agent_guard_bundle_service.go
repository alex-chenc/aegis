package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"api-server/internal/model"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

const (
	AgentGuardBundleConfigType = "agent_guard_bundle"
	AgentGuardBundleSchema     = "aegis.agent_guard.bundle.v1"
)

var (
	ErrAgentGuardPolicyPublishDisabled = errors.New("agent guard policy publishing is disabled")
	ErrAgentGuardPolicyNotDraft        = errors.New("agent guard policy is not a draft")
	ErrAgentGuardPolicyNoTargetHosts   = errors.New("agent guard policy resolves to no target hosts")
)

type agentGuardBundleStore interface {
	GetByID(context.Context, uuid.UUID) (*model.AgentGuardPolicy, error)
	List(context.Context, model.AgentGuardPolicyQuery) ([]model.AgentGuardPolicy, int64, error)
	ListDeliveries(context.Context, string, int64, model.AgentGuardDeliveryQuery) ([]model.AgentGuardPolicyDelivery, int64, error)
	ResolveTargetHostIDs(context.Context, model.AgentGuardPolicyTargets) ([]uuid.UUID, error)
	MaxBundleVersion(context.Context, []uuid.UUID) (int64, error)
	PublishDraftWithDeliveries(context.Context, uuid.UUID, string, []model.AgentGuardPolicyDelivery) (*model.AgentGuardPolicy, []model.AgentGuardPolicyDelivery, error)
	UpdateDeliveryDispatchStatus(context.Context, uuid.UUID, string, string, string) error
}

type agentGuardBundleCatalog interface {
	ListProfiles(context.Context, model.AgentGuardProfileQuery) ([]model.AgentGuardAdapterProfile, int64, error)
	ListRules(context.Context, model.AgentBehaviorRuleQuery) ([]model.AgentBehaviorRuleDefinition, int64, error)
}

type agentGuardBundleDispatcher interface {
	SyncAgentConfig(context.Context, string, []*pb.AgentConfig) (int32, error)
}

type AgentGuardBundleDefaults struct {
	Mode                     string `json:"mode"`
	BehaviorMonitorEnabled   bool   `json:"behavior_monitor_enabled"`
	BehaviorPolicyEnabled    bool   `json:"behavior_policy_enabled"`
	EscapePolicyEnabled      bool   `json:"escape_policy_enabled"`
	BehaviorHookEnabled      bool   `json:"behavior_hook_enabled"`
	EscapeHookEnabled        bool   `json:"escape_hook_enabled"`
	ToolAdapterEnabled       bool   `json:"tool_adapter_enabled"`
	EnforcementEnabled       bool   `json:"enforcement_enabled"`
	FreezeEnabled            bool   `json:"freeze_enabled"`
	FreezeTimeoutSeconds     int    `json:"freeze_timeout_seconds"`
	ReconcileIntervalSeconds int    `json:"reconcile_interval_seconds"`
}

type AgentGuardBundleProfile struct {
	ProfileKey           string         `json:"profile_key"`
	ProfileVersion       int64          `json:"profile_version"`
	AgentType            string         `json:"agent_type"`
	DisplayName          string         `json:"display_name"`
	SandboxFamily        string         `json:"sandbox_family"`
	ControllerMatch      datatypes.JSON `json:"controller_match"`
	WorkerMatch          datatypes.JSON `json:"worker_match"`
	BackendDetectors     datatypes.JSON `json:"backend_detectors"`
	IsolationExpectation datatypes.JSON `json:"isolation_expectation"`
	DefaultEscapeRules   datatypes.JSON `json:"default_escape_rules"`
	Digest               string         `json:"digest"`
}

type AgentGuardBundleRule struct {
	RuleKey            string         `json:"rule_key"`
	RuleVersion        int64          `json:"rule_version"`
	Enabled            bool           `json:"enabled"`
	Severity           string         `json:"severity"`
	Action             string         `json:"action"`
	CompiledParameters datatypes.JSON `json:"compiled_parameters"`
	Digest             string         `json:"digest"`
}

type AgentGuardBundlePolicy struct {
	PolicyKey            string         `json:"policy_key"`
	Version              int64          `json:"version"`
	Priority             int            `json:"priority"`
	Targets              datatypes.JSON `json:"targets"`
	CollectionPolicy     datatypes.JSON `json:"collection_policy"`
	BuiltinRuleOverrides datatypes.JSON `json:"builtin_rule_overrides"`
	AtomicRules          datatypes.JSON `json:"atomic_rules"`
	CorrelationRules     datatypes.JSON `json:"correlation_rules"`
	AnalysisPolicy       datatypes.JSON `json:"analysis_policy"`
	EscapeRules          datatypes.JSON `json:"escape_rules"`
	CompiledPreview      datatypes.JSON `json:"compiled_preview"`
	Digest               string         `json:"digest"`
}

type AgentGuardBundle struct {
	Schema        string                    `json:"schema"`
	BundleVersion int64                     `json:"bundle_version"`
	GeneratedAt   time.Time                 `json:"generated_at"`
	HostID        string                    `json:"host_id"`
	Profiles      []AgentGuardBundleProfile `json:"profiles"`
	BuiltinRules  []AgentGuardBundleRule    `json:"builtin_rules"`
	EscapeRules   []AgentGuardBundleRule    `json:"escape_rules"`
	Policies      []AgentGuardBundlePolicy  `json:"policies"`
	Defaults      AgentGuardBundleDefaults  `json:"defaults"`
	Digest        string                    `json:"digest"`
}

type AgentGuardBundlePublishResult struct {
	Policy     *model.AgentGuardPolicy          `json:"policy"`
	Deliveries []model.AgentGuardPolicyDelivery `json:"deliveries"`
}

type AgentGuardBundleService struct {
	store              agentGuardBundleStore
	catalog            agentGuardBundleCatalog
	dispatcher         agentGuardBundleDispatcher
	publishEnabled     bool
	toolAdapterEnabled bool
	logger             *zap.Logger
}

func NewAgentGuardBundleService(
	store agentGuardBundleStore,
	catalog agentGuardBundleCatalog,
	dispatcher agentGuardBundleDispatcher,
	publishEnabled bool,
	toolAdapterEnabled bool,
	logger *zap.Logger,
) *AgentGuardBundleService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentGuardBundleService{
		store:              store,
		catalog:            catalog,
		dispatcher:         dispatcher,
		publishEnabled:     publishEnabled,
		toolAdapterEnabled: toolAdapterEnabled,
		logger:             logger,
	}
}

func (s *AgentGuardBundleService) Publish(
	ctx context.Context,
	id uuid.UUID,
	publishedBy string,
) (*AgentGuardBundlePublishResult, error) {
	if !s.publishEnabled {
		return nil, ErrAgentGuardPolicyPublishDisabled
	}
	draft, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if draft.Status == "published" {
		deliveries, _, err := s.store.ListDeliveries(ctx, draft.PolicyKey, draft.Version, model.AgentGuardDeliveryQuery{
			AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 200},
		})
		if err != nil {
			return nil, err
		}
		return &AgentGuardBundlePublishResult{Policy: draft, Deliveries: deliveries}, nil
	}
	if draft.Status != "draft" {
		return nil, ErrAgentGuardPolicyNotDraft
	}

	var targets model.AgentGuardPolicyTargets
	if err := json.Unmarshal(draft.Targets, &targets); err != nil {
		return nil, fmt.Errorf("decode agent guard policy targets: %w", err)
	}
	hostIDs, err := s.store.ResolveTargetHostIDs(ctx, targets)
	if err != nil {
		return nil, err
	}
	if len(hostIDs) == 0 {
		return nil, ErrAgentGuardPolicyNoTargetHosts
	}

	profiles, _, err := s.catalog.ListProfiles(ctx, model.AgentGuardProfileQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 200},
	})
	if err != nil {
		return nil, err
	}
	rules, _, err := s.catalog.ListRules(ctx, model.AgentBehaviorRuleQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 200},
		Source:              "builtin",
	})
	if err != nil {
		return nil, err
	}
	publishedPolicies, _, err := s.store.List(ctx, model.AgentGuardPolicyQuery{
		AgentGuardPageQuery: model.AgentGuardPageQuery{Page: 1, PageSize: 200},
		Status:              "published",
	})
	if err != nil {
		return nil, err
	}
	publishedPolicies = append(publishedPolicies, *draft)

	maxVersion, err := s.store.MaxBundleVersion(ctx, hostIDs)
	if err != nil {
		return nil, err
	}
	bundleVersion := time.Now().UTC().UnixMilli()
	if bundleVersion <= maxVersion {
		bundleVersion = maxVersion + 1
	}
	generatedAt := time.Now().UTC()
	payloadByHost := make(map[uuid.UUID]string, len(hostIDs))
	deliveries := make([]model.AgentGuardPolicyDelivery, 0, len(hostIDs))
	for _, hostID := range hostIDs {
		bundle, payload, err := buildAgentGuardBundle(
			hostID,
			bundleVersion,
			generatedAt,
			profiles,
			rules,
			publishedPolicies,
			s.toolAdapterEnabled,
		)
		if err != nil {
			return nil, err
		}
		payloadByHost[hostID] = payload
		policyVersions, _ := json.Marshal(bundlePolicyVersions(bundle.Policies))
		profileVersions, _ := json.Marshal(bundleProfileVersions(bundle.Profiles))
		ruleVersions, _ := json.Marshal(bundleRuleVersions(bundle.BuiltinRules))
		deliveries = append(deliveries, model.AgentGuardPolicyDelivery{
			ID:                  uuid.New(),
			HostID:              hostID,
			BundleVersion:       bundle.BundleVersion,
			BundleDigest:        bundle.Digest,
			PolicyVersions:      datatypes.JSON(policyVersions),
			ProfileVersions:     datatypes.JSON(profileVersions),
			BuiltinRuleVersions: datatypes.JSON(ruleVersions),
			Status:              "pending",
			CoverageLevel:       "monitor_only",
			CapabilitySnapshot:  datatypes.JSON([]byte(`{}`)),
			GeneratedAt:         generatedAt,
		})
	}

	policy, storedDeliveries, err := s.store.PublishDraftWithDeliveries(
		ctx,
		id,
		strings.TrimSpace(publishedBy),
		deliveries,
	)
	if err != nil {
		return nil, err
	}
	for index := range storedDeliveries {
		delivery := &storedDeliveries[index]
		status, errorCode, errorMessage := s.dispatchBundle(ctx, *delivery, payloadByHost[delivery.HostID])
		delivery.Status = status
		delivery.ErrorCode = errorCode
		delivery.ErrorMessage = errorMessage
		if err := s.store.UpdateDeliveryDispatchStatus(ctx, delivery.ID, status, errorCode, errorMessage); err != nil {
			s.logger.Error("agent_guard_delivery_status_update_failed",
				zap.String("host_id", delivery.HostID.String()),
				zap.Int64("bundle_version", delivery.BundleVersion),
				zap.String("bundle_digest", delivery.BundleDigest),
				zap.String("status", status),
				zap.Error(err),
			)
		}
	}
	s.logger.Info("agent_guard_policy_published",
		zap.String("policy_id", policy.ID.String()),
		zap.String("policy_key", policy.PolicyKey),
		zap.Int64("policy_version", policy.Version),
		zap.Int("delivery_count", len(storedDeliveries)),
		zap.Bool("tool_adapter_global_gate_enabled", s.toolAdapterEnabled),
	)
	return &AgentGuardBundlePublishResult{Policy: policy, Deliveries: storedDeliveries}, nil
}

func (s *AgentGuardBundleService) dispatchBundle(
	ctx context.Context,
	delivery model.AgentGuardPolicyDelivery,
	payload string,
) (string, string, string) {
	if s.dispatcher == nil {
		return "failed", "agent_guard_dispatcher_unavailable", "agent config dispatcher is unavailable"
	}
	affected, err := s.dispatcher.SyncAgentConfig(ctx, delivery.HostID.String(), []*pb.AgentConfig{{
		ConfigType: AgentGuardBundleConfigType,
		ConfigJson: payload,
	}})
	if err != nil {
		s.logger.Warn("agent_guard_bundle_dispatch_failed",
			zap.String("host_id", delivery.HostID.String()),
			zap.Int64("bundle_version", delivery.BundleVersion),
			zap.String("bundle_digest", delivery.BundleDigest),
			zap.Error(err),
		)
		return "failed", "agent_guard_delivery_failed", truncateAgentGuardError(err.Error())
	}
	if affected < 1 {
		return "failed", "agent_guard_agent_offline", "target agent is not connected"
	}
	s.logger.Info("agent_guard_bundle_dispatched",
		zap.String("host_id", delivery.HostID.String()),
		zap.Int64("bundle_version", delivery.BundleVersion),
		zap.String("bundle_digest", delivery.BundleDigest),
	)
	return "dispatching", "", ""
}

func buildAgentGuardBundle(
	hostID uuid.UUID,
	bundleVersion int64,
	generatedAt time.Time,
	profiles []model.AgentGuardAdapterProfile,
	rules []model.AgentBehaviorRuleDefinition,
	policies []model.AgentGuardPolicy,
	toolAdapterRolloutEnabled bool,
) (AgentGuardBundle, string, error) {
	toolAdapterRequested, err := agentGuardPoliciesRequestToolAdapter(policies)
	if err != nil {
		return AgentGuardBundle{}, "", err
	}
	bundle := AgentGuardBundle{
		Schema:        AgentGuardBundleSchema,
		BundleVersion: bundleVersion,
		GeneratedAt:   generatedAt,
		HostID:        hostID.String(),
		Profiles:      make([]AgentGuardBundleProfile, 0, len(profiles)),
		BuiltinRules:  make([]AgentGuardBundleRule, 0, len(rules)),
		EscapeRules:   make([]AgentGuardBundleRule, 0, len(model.BuiltinAgentEscapeRuleManifest())),
		Policies:      make([]AgentGuardBundlePolicy, 0, len(policies)),
		Defaults: AgentGuardBundleDefaults{
			Mode: "monitor_only", BehaviorMonitorEnabled: true,
			BehaviorPolicyEnabled: true, EscapePolicyEnabled: true,
			BehaviorHookEnabled: toolAdapterRolloutEnabled && toolAdapterRequested,
			EscapeHookEnabled:   true,
			ToolAdapterEnabled:  toolAdapterRolloutEnabled && toolAdapterRequested,
			EnforcementEnabled:  false, FreezeEnabled: false,
			FreezeTimeoutSeconds: 300, ReconcileIntervalSeconds: 30,
		},
	}
	for _, rule := range model.BuiltinAgentEscapeRuleManifest() {
		bundle.EscapeRules = append(bundle.EscapeRules, AgentGuardBundleRule{
			RuleKey: rule.RuleKey, RuleVersion: rule.RuleVersion,
			Enabled: rule.DefaultEnabled, Severity: rule.DefaultSeverity,
			Action: rule.DefaultAction, CompiledParameters: datatypes.JSON([]byte(`{}`)),
			Digest: rule.Digest,
		})
	}
	for _, profile := range profiles {
		if !profile.Enabled {
			continue
		}
		bundle.Profiles = append(bundle.Profiles, AgentGuardBundleProfile{
			ProfileKey: profile.ProfileKey, ProfileVersion: profile.ProfileVersion,
			AgentType: profile.AgentType, DisplayName: profile.DisplayName,
			SandboxFamily: profile.SandboxFamily, ControllerMatch: profile.ControllerMatch,
			WorkerMatch: profile.WorkerMatch, BackendDetectors: profile.BackendDetectors,
			IsolationExpectation: profile.IsolationExpectation,
			DefaultEscapeRules:   profile.DefaultEscapeRules, Digest: profile.Digest,
		})
	}
	for _, rule := range rules {
		bundle.BuiltinRules = append(bundle.BuiltinRules, AgentGuardBundleRule{
			RuleKey: rule.RuleKey, RuleVersion: rule.RuleVersion,
			Enabled: rule.DefaultEnabled, Severity: rule.DefaultSeverity,
			Action: rule.DefaultAction, CompiledParameters: rule.DefaultParameters,
			Digest: rule.Digest,
		})
	}
	for _, policy := range policies {
		bundle.Policies = append(bundle.Policies, AgentGuardBundlePolicy{
			PolicyKey: policy.PolicyKey, Version: policy.Version, Priority: policy.Priority,
			Targets: policy.Targets, CollectionPolicy: policy.CollectionPolicy,
			BuiltinRuleOverrides: policy.BuiltinRuleOverrides, AtomicRules: policy.AtomicRules,
			CorrelationRules: policy.CorrelationRules, AnalysisPolicy: policy.AnalysisPolicy,
			EscapeRules: policy.EscapeRules, CompiledPreview: policy.CompiledPreview,
			Digest: policy.Digest,
		})
	}

	unsigned, err := json.Marshal(bundle)
	if err != nil {
		return AgentGuardBundle{}, "", fmt.Errorf("encode agent guard bundle: %w", err)
	}
	var canonical map[string]any
	if err := json.Unmarshal(unsigned, &canonical); err != nil {
		return AgentGuardBundle{}, "", fmt.Errorf("canonicalize agent guard bundle: %w", err)
	}
	delete(canonical, "digest")
	canonicalJSON, err := json.Marshal(canonical)
	if err != nil {
		return AgentGuardBundle{}, "", fmt.Errorf("canonicalize agent guard bundle: %w", err)
	}
	sum := sha256.Sum256(canonicalJSON)
	bundle.Digest = "sha256:" + hex.EncodeToString(sum[:])
	payload, err := json.Marshal(bundle)
	if err != nil {
		return AgentGuardBundle{}, "", fmt.Errorf("encode signed agent guard bundle: %w", err)
	}
	return bundle, string(payload), nil
}

func agentGuardPoliciesRequestToolAdapter(policies []model.AgentGuardPolicy) (bool, error) {
	requested := false
	for _, policy := range policies {
		if strings.TrimSpace(string(policy.CollectionPolicy)) == "" {
			continue
		}
		var collection model.AgentGuardCollectionPolicy
		if err := json.Unmarshal(policy.CollectionPolicy, &collection); err != nil {
			return false, fmt.Errorf("decode agent guard collection policy %s@%d: %w", policy.PolicyKey, policy.Version, err)
		}
		if !collection.ToolAdapterEnabled {
			continue
		}
		if !containsStringValue(collection.Categories, "tool") {
			return false, fmt.Errorf("agent guard collection policy %s@%d enables tool adapter without tool category", policy.PolicyKey, policy.Version)
		}
		requested = true
	}
	return requested, nil
}

func bundlePolicyVersions(policies []AgentGuardBundlePolicy) []map[string]any {
	result := make([]map[string]any, 0, len(policies))
	for _, policy := range policies {
		result = append(result, map[string]any{"policy_key": policy.PolicyKey, "version": policy.Version})
	}
	return result
}

func bundleProfileVersions(profiles []AgentGuardBundleProfile) []map[string]any {
	result := make([]map[string]any, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, map[string]any{"profile_key": profile.ProfileKey, "version": profile.ProfileVersion})
	}
	return result
}

func bundleRuleVersions(rules []AgentGuardBundleRule) map[string]int64 {
	result := make(map[string]int64, len(rules))
	for _, rule := range rules {
		result[rule.RuleKey] = rule.RuleVersion
	}
	return result
}

func truncateAgentGuardError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		return value[:512]
	}
	return value
}
