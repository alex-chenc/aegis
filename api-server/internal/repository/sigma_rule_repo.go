package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"api-server/internal/model"

	"gorm.io/gorm"
)

type SigmaRuleRepository struct {
	db *gorm.DB
}

func NewSigmaRuleRepository(db *gorm.DB) *SigmaRuleRepository {
	return &SigmaRuleRepository{db: db}
}

func (r *SigmaRuleRepository) Create(rule *model.SigmaRule) error {
	return r.db.Create(rule).Error
}

func (r *SigmaRuleRepository) Update(rule *model.SigmaRule) error {
	return r.db.Save(rule).Error
}

func (r *SigmaRuleRepository) ExistsByMitreID(mitreID string) (bool, error) {
	var count int64
	err := r.db.Model(&model.SigmaRule{}).Where("mitre_id = ?", mitreID).Count(&count).Error
	return count > 0, err
}

func (r *SigmaRuleRepository) FindByID(ruleID string) (*model.SigmaRule, error) {
	var rule model.SigmaRule
	err := r.db.Where("rule_id = ?", ruleID).First(&rule).Error
	if err != nil {
		return nil, err
	}

	return &rule, nil
}

func (r *SigmaRuleRepository) List(page, pageSize int, filters map[string]interface{}) ([]model.SigmaRule, int64, error) {
	var (
		rules []model.SigmaRule
		total int64
	)

	query := r.db.Model(&model.SigmaRule{})

	if searchQuery, ok := filters["query"].(string); ok && searchQuery != "" {
		searchPattern := "%" + searchQuery + "%"
		query = query.Where(
			"title ILIKE ? OR rule_id ILIKE ? OR mitre_id ILIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	for key, val := range filters {
		if key == "query" {
			continue
		}
		if val != nil && val != "" {
			query = query.Where(key+" = ?", val)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rules).Error; err != nil {
		return nil, 0, err
	}

	return rules, total, nil
}

func (r *SigmaRuleRepository) UpdateStatus(ruleID, status string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": gorm.Expr("NOW()"),
	}
	// Set activated_at for both active and experimental status.
	// Experimental rules are dispatched immediately; activated_at is used for
	// observing how long they have been in trial before promotion to active.
	if status == "active" || status == "experimental" {
		updates["activated_at"] = gorm.Expr("NOW()")
	}

	return r.db.Model(&model.SigmaRule{}).Where("rule_id = ?", ruleID).Updates(updates).Error
}

func (r *SigmaRuleRepository) GetActiveAndExperimental() ([]model.SigmaRule, error) {
	var rules []model.SigmaRule
	err := r.db.Where("status IN ?", []string{"active", "experimental"}).Find(&rules).Error
	return rules, err
}

func (r *SigmaRuleRepository) GetActiveCount() (int64, error) {
	var count int64
	err := r.db.Model(&model.SigmaRule{}).Where("status IN ?", []string{"active", "experimental"}).Count(&count).Error
	return count, err
}

func (r *SigmaRuleRepository) GetExperimentalRules() ([]model.SigmaRule, error) {
	var rules []model.SigmaRule
	err := r.db.Where("status = ?", "experimental").Find(&rules).Error
	return rules, err
}

func (r *SigmaRuleRepository) FindByRuleID(ruleID string) (*model.SigmaRule, error) {
	var rule model.SigmaRule
	err := r.db.Where("rule_id = ?", ruleID).First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *SigmaRuleRepository) DeleteByRuleID(ruleID string) error {
	return r.db.Where("rule_id = ?", ruleID).Delete(&model.SigmaRule{}).Error
}

func (r *SigmaRuleRepository) NormalizeMitreIDs(ctx context.Context) (int, error) {
	var rules []model.SigmaRule
	if err := r.db.Where("mitre_id IS NOT NULL AND mitre_id != ''").Find(&rules).Error; err != nil {
		return 0, err
	}

	updated := 0
	for _, rule := range rules {
		upperID := strings.ToUpper(rule.MitreID)
		if !strings.HasPrefix(upperID, "T") {
			upperID = "T" + upperID
		}
		if upperID != rule.MitreID {
			if err := r.db.Model(&model.SigmaRule{}).Where("id = ?", rule.ID).Update("mitre_id", upperID).Error; err != nil {
				continue
			}
			updated++
		}
	}
	return updated, nil
}

// ListByGeneratedBy 按生成来源和状态获取规则列表
func (r *SigmaRuleRepository) ListByGeneratedBy(generatedBy, status string) ([]model.SigmaRule, error) {
	var rules []model.SigmaRule
	query := r.db.Where("generated_by = ?", generatedBy)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Order("created_at DESC").Find(&rules).Error
	return rules, err
}

// CountPendingByMitreID 统计同一MITRE ID的待审核规则数
func (r *SigmaRuleRepository) CountPendingByMitreID(mitreID string) (int64, error) {
	var count int64
	err := r.db.Model(&model.SigmaRule{}).
		Where("mitre_id = ? AND status = ?", mitreID, "pending").
		Count(&count).Error
	return count, err
}

// FindByMitreID 根据MITRE ID查找规则
func (r *SigmaRuleRepository) FindByMitreID(mitreID string) (*model.SigmaRule, error) {
	var rule model.SigmaRule
	err := r.db.Where("mitre_id = ?", mitreID).First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// DeleteByMitreID 根据MITRE ID删除规则
func (r *SigmaRuleRepository) DeleteByMitreID(mitreID string) (int64, error) {
	result := r.db.Where("mitre_id = ?", mitreID).Delete(&model.SigmaRule{})
	return result.RowsAffected, result.Error
}

// FindByFileHash 根据文件hash查找规则（用于去重）
func (r *SigmaRuleRepository) FindByFileHash(fileHash string) (*model.SigmaRule, error) {
	var rule model.SigmaRule
	err := r.db.Where("file_hash = ?", fileHash).First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// UpdateDispatchStatus 更新规则下发状态
func (r *SigmaRuleRepository) UpdateDispatchStatus(ruleID string, hosts []string, status string) error {
	// 将hosts数组转换为JSON字符串
	hostsJSON := "[]"
	if len(hosts) > 0 {
		imported, _ := json.Marshal(hosts)
		hostsJSON = string(imported)
	}

	updates := map[string]interface{}{
		"dispatch_hosts":  hostsJSON,
		"dispatch_status": status,
		"updated_at":      gorm.Expr("NOW()"),
	}

	return r.db.Model(&model.SigmaRule{}).Where("rule_id = ?", ruleID).Updates(updates).Error
}

// UpdateStatusWithApproval 更新规则状态并记录审批信息
func (r *SigmaRuleRepository) UpdateStatusWithApproval(ruleID, status, approvedBy string) error {
	updates := map[string]interface{}{
		"status":      status,
		"approved_by": approvedBy,
		"updated_at":  gorm.Expr("NOW()"),
	}
	// 设置activated_at和approved_at时间戳
	now := time.Now()
	if status == "active" || status == "experimental" {
		updates["activated_at"] = now
	}
	if approvedBy != "" {
		updates["approved_at"] = now
	}

	return r.db.Model(&model.SigmaRule{}).Where("rule_id = ?", ruleID).Updates(updates).Error
}
