package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AIConfig AI规则生成配置
type AIConfig struct {
	ID                         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name                       string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Description                string    `gorm:"type:text" json:"description"`
	Enabled                    bool      `gorm:"type:boolean;not null;default:false" json:"enabled"`
	Mode                       string    `gorm:"type:varchar(20);not null;default:'suggest'" json:"mode"` // suggest, auto
	Thresholds                 string    `gorm:"type:jsonb;not null;default:'{\"high_frequency_count\":10,\"high_frequency_hours\":1}'" json:"thresholds"`
	Conservatism               float64   `gorm:"type:decimal(3,2);not null;default:0.5" json:"conservatism"` // 0.0-1.0
	RequireApproval            bool      `gorm:"type:boolean;not null;default:true" json:"require_approval"`
	AutoActivateAfterApproval  bool      `gorm:"type:boolean;not null;default:false" json:"auto_activate_after_approval"`
	ActivationDelayHours       int       `gorm:"type:integer;not null;default:24" json:"activation_delay_hours"`
	NotifyOnGeneration         bool      `gorm:"type:boolean;not null;default:true" json:"notify_on_generation"`
	NotifyOnApproval           bool      `gorm:"type:boolean;not null;default:true" json:"notify_on_approval"`
	NotificationTargets        string    `gorm:"type:jsonb;not null;default:'[]'" json:"notification_targets"` // ["email:xxx", "webhook:xxx"]
	RulesGeneratedCount        int       `gorm:"type:integer;not null;default:0" json:"rules_generated_count"`
	RulesApprovedCount         int       `gorm:"type:integer;not null;default:0" json:"rules_approved_count"`
	CreatedAt                  time.Time `gorm:"default:now()" json:"created_at"`
	UpdatedAt                  time.Time `gorm:"default:now()" json:"updated_at"`
	CreatedBy                  string    `gorm:"type:varchar(100)" json:"created_by"`
	UpdatedBy                  string    `gorm:"type:varchar(100)" json:"updated_by"`
}

func (AIConfig) TableName() string { return "ai_rule_config" }

// Thresholds 触发阈值
type Thresholds struct {
	HighFrequencyCount int `json:"high_frequency_count"`
	HighFrequencyHours int `json:"high_frequency_hours"`
}

// GetThresholds parses and returns the thresholds struct
func (c *AIConfig) GetThresholds() (*Thresholds, error) {
	var t Thresholds
	if err := json.Unmarshal([]byte(c.Thresholds), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// SetThresholds sets the thresholds from a struct
func (c *AIConfig) SetThresholds(t *Thresholds) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	c.Thresholds = string(data)
	return nil
}

// GetNotificationTargets parses and returns notification targets
func (c *AIConfig) GetNotificationTargets() ([]string, error) {
	var targets []string
	if err := json.Unmarshal([]byte(c.NotificationTargets), &targets); err != nil {
		return nil, err
	}
	return targets, nil
}

// SetNotificationTargets sets notification targets
func (c *AIConfig) SetNotificationTargets(targets []string) error {
	data, err := json.Marshal(targets)
	if err != nil {
		return err
	}
	c.NotificationTargets = string(data)
	return nil
}

// AIConfigResponse API响应结构
type AIConfigResponse struct {
	ID                        string     `json:"id"`
	Name                      string     `json:"name"`
	Enabled                   bool       `json:"enabled"`
	Mode                      string     `json:"mode"`
	Thresholds                Thresholds `json:"thresholds"`
	Conservatism              float64    `json:"conservatism"`
	RequireApproval           bool       `json:"require_approval"`
	AutoActivateAfterApproval bool       `json:"auto_activate_after_approval"`
	ActivationDelayHours      int        `json:"activation_delay_hours"`
	NotifyOnGeneration       bool       `json:"notify_on_generation"`
	NotifyOnApproval         bool       `json:"notify_on_approval"`
	NotificationTargets       []string   `json:"notification_targets"`
	RulesGeneratedCount       int        `json:"rules_generated_count"`
	RulesApprovedCount       int        `json:"rules_approved_count"`
}

// ToResponse converts AIConfig to API response
func (c *AIConfig) ToResponse() (*AIConfigResponse, error) {
	thresholds, err := c.GetThresholds()
	if err != nil {
		return nil, err
	}

	notificationTargets, err := c.GetNotificationTargets()
	if err != nil {
		return nil, err
	}

	return &AIConfigResponse{
		ID:                        c.ID.String(),
		Name:                      c.Name,
		Enabled:                   c.Enabled,
		Mode:                      c.Mode,
		Thresholds:                *thresholds,
		Conservatism:              c.Conservatism,
		RequireApproval:           c.RequireApproval,
		AutoActivateAfterApproval: c.AutoActivateAfterApproval,
		ActivationDelayHours:      c.ActivationDelayHours,
		NotifyOnGeneration:        c.NotifyOnGeneration,
		NotifyOnApproval:          c.NotifyOnApproval,
		NotificationTargets:       notificationTargets,
		RulesGeneratedCount:        c.RulesGeneratedCount,
		RulesApprovedCount:         c.RulesApprovedCount,
	}, nil
}

// UpdateAIConfigRequest 更新配置请求
type UpdateAIConfigRequest struct {
	Enabled                   *bool       `json:"enabled"`
	Mode                      *string     `json:"mode"`
	Thresholds                *Thresholds `json:"thresholds"`
	Conservatism              *float64    `json:"conservatism"`
	RequireApproval           *bool       `json:"require_approval"`
	AutoActivateAfterApproval *bool       `json:"auto_activate_after_approval"`
	ActivationDelayHours      *int        `json:"activation_delay_hours"`
	NotifyOnGeneration        *bool       `json:"notify_on_generation"`
	NotifyOnApproval          *bool       `json:"notify_on_approval"`
	NotificationTargets       []string    `json:"notification_targets"`
}
