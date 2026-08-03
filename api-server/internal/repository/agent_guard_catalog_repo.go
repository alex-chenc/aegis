package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrAgentGuardProfileNotFound       = errors.New("agent guard adapter profile not found")
	ErrAgentGuardRuleNotFound          = errors.New("agent behavior rule not found")
	ErrAgentGuardBuiltinDigestMismatch = errors.New("agent guard builtin manifest digest mismatch")
)

type AgentGuardCatalogRepository struct {
	db *gorm.DB
}

func NewAgentGuardCatalogRepository(db *gorm.DB) *AgentGuardCatalogRepository {
	return &AgentGuardCatalogRepository{db: db}
}

func (r *AgentGuardCatalogRepository) ListProfiles(
	ctx context.Context,
	query model.AgentGuardProfileQuery,
) ([]model.AgentGuardAdapterProfile, int64, error) {
	var (
		profiles []model.AgentGuardAdapterProfile
		total    int64
	)
	db := r.db.WithContext(ctx).Model(&model.AgentGuardAdapterProfile{})
	if query.AgentType != "" {
		db = db.Where("agent_type = ?", query.AgentType)
	}
	if query.Source != "" {
		db = db.Where("source = ?", query.Source)
	}
	if query.Enabled != nil {
		db = db.Where("enabled = ?", *query.Enabled)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		pattern := "%" + strings.ToLower(keyword) + "%"
		db = db.Where(
			"LOWER(profile_key) LIKE ? OR LOWER(display_name) LIKE ? OR LOWER(agent_type) LIKE ?",
			pattern,
			pattern,
			pattern,
		)
	}
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent guard profiles: %w", err)
	}
	page, pageSize := normalizeAgentGuardPage(query.Page, query.PageSize)
	if err := db.Order("profile_key ASC, profile_version DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&profiles).Error; err != nil {
		return nil, 0, fmt.Errorf("list agent guard profiles: %w", err)
	}
	return profiles, total, nil
}

func (r *AgentGuardCatalogRepository) GetProfile(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentGuardAdapterProfile, error) {
	var profile model.AgentGuardAdapterProfile
	err := r.db.WithContext(ctx).First(&profile, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentGuardProfileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get agent guard profile: %w", err)
	}
	return &profile, nil
}

func (r *AgentGuardCatalogRepository) ListRules(
	ctx context.Context,
	query model.AgentBehaviorRuleQuery,
) ([]model.AgentBehaviorRuleDefinition, int64, error) {
	var (
		rules []model.AgentBehaviorRuleDefinition
		total int64
	)
	db := r.db.WithContext(ctx).Model(&model.AgentBehaviorRuleDefinition{})
	if query.Source != "" {
		db = db.Where("source = ?", query.Source)
	}
	if query.Engine != "" {
		db = db.Where("engine = ?", query.Engine)
	}
	if query.Category != "" {
		if r.db.Dialector.Name() == "postgres" {
			db = db.Where("categories @> ?::jsonb", fmt.Sprintf("[%q]", query.Category))
		} else {
			db = db.Where(
				"EXISTS (SELECT 1 FROM json_each(agent_behavior_rule_definitions.categories) WHERE json_each.value = ?)",
				query.Category,
			)
		}
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		pattern := "%" + strings.ToLower(keyword) + "%"
		db = db.Where(
			"LOWER(rule_key) LIKE ? OR LOWER(name) LIKE ? OR LOWER(description) LIKE ?",
			pattern,
			pattern,
			pattern,
		)
	}
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent behavior rules: %w", err)
	}
	page, pageSize := normalizeAgentGuardPage(query.Page, query.PageSize)
	if err := db.Order("rule_key ASC, rule_version DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&rules).Error; err != nil {
		return nil, 0, fmt.Errorf("list agent behavior rules: %w", err)
	}
	return rules, total, nil
}

func (r *AgentGuardCatalogRepository) GetRule(
	ctx context.Context,
	ruleKey string,
	ruleVersion int64,
) (*model.AgentBehaviorRuleDefinition, error) {
	var rule model.AgentBehaviorRuleDefinition
	db := r.db.WithContext(ctx).Where("rule_key = ?", ruleKey)
	if ruleVersion > 0 {
		db = db.Where("rule_version = ?", ruleVersion)
	} else {
		db = db.Order("rule_version DESC")
	}
	err := db.First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentGuardRuleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get agent behavior rule: %w", err)
	}
	return &rule, nil
}

func (r *AgentGuardCatalogRepository) ListRuleVersions(
	ctx context.Context,
	ruleKey string,
	page int,
	pageSize int,
) ([]model.AgentBehaviorRuleDefinition, int64, error) {
	var (
		rules []model.AgentBehaviorRuleDefinition
		total int64
	)
	db := r.db.WithContext(ctx).
		Model(&model.AgentBehaviorRuleDefinition{}).
		Where("rule_key = ?", ruleKey)
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent behavior rule versions: %w", err)
	}
	page, pageSize = normalizeAgentGuardPage(page, pageSize)
	if err := db.Order("rule_version DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&rules).Error; err != nil {
		return nil, 0, fmt.Errorf("list agent behavior rule versions: %w", err)
	}
	return rules, total, nil
}

func (r *AgentGuardCatalogRepository) VerifyBuiltinManifest(ctx context.Context) error {
	if err := model.VerifyBuiltinAgentGuardManifest(); err != nil {
		return fmt.Errorf("%w: %v", ErrAgentGuardBuiltinDigestMismatch, err)
	}
	for _, expected := range model.BuiltinAgentGuardProfileManifest() {
		var actual model.AgentGuardAdapterProfile
		err := r.db.WithContext(ctx).
			Where("profile_key = ? AND profile_version = ?", expected.ProfileKey, expected.ProfileVersion).
			First(&actual).Error
		if err != nil {
			return fmt.Errorf("%w: profile %s@%d: %v",
				ErrAgentGuardBuiltinDigestMismatch, expected.ProfileKey, expected.ProfileVersion, err)
		}
		calculated, err := model.CalculateAgentGuardProfileDigest(actual)
		if err != nil || actual.ID != expected.ID || actual.Digest != expected.Digest || calculated != expected.Digest {
			return fmt.Errorf("%w: profile %s@%d",
				ErrAgentGuardBuiltinDigestMismatch, expected.ProfileKey, expected.ProfileVersion)
		}
	}
	for _, expected := range model.BuiltinAgentBehaviorRuleManifest() {
		var actual model.AgentBehaviorRuleDefinition
		err := r.db.WithContext(ctx).
			Where("rule_key = ? AND rule_version = ?", expected.RuleKey, expected.RuleVersion).
			First(&actual).Error
		if err != nil {
			return fmt.Errorf("%w: rule %s@%d: %v",
				ErrAgentGuardBuiltinDigestMismatch, expected.RuleKey, expected.RuleVersion, err)
		}
		calculated, err := model.CalculateAgentBehaviorRuleDigest(actual)
		if err != nil || actual.ID != expected.ID || actual.Digest != expected.Digest || calculated != expected.Digest {
			return fmt.Errorf("%w: rule %s@%d",
				ErrAgentGuardBuiltinDigestMismatch, expected.RuleKey, expected.RuleVersion)
		}
	}
	return nil
}

func normalizeAgentGuardPage(page int, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
