package model

import (
	"time"

	"github.com/google/uuid"
)

type SigmaRule struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	RuleID      string     `gorm:"type:varchar(128);uniqueIndex;not null" json:"rule_id"`
	Title       string     `gorm:"type:varchar(256)" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	Content     string     `gorm:"type:text;not null" json:"content"`
	Status      string     `gorm:"type:varchar(20);not null;default:'pending';index" json:"status"`
	MitreID     string     `gorm:"type:varchar(20);index" json:"mitre_id"`
	Severity    string     `gorm:"type:varchar(20)" json:"severity"`
	GeneratedBy string     `gorm:"type:varchar(20);not null;default:'llm'" json:"generated_by"`
	Version     string     `gorm:"type:varchar(20);not null;default:'1.0'" json:"version"`
	CreatedAt   time.Time  `gorm:"default:now()" json:"created_at"`
	ActivatedAt *time.Time `json:"activated_at"`
	UpdatedAt   time.Time  `gorm:"default:now()" json:"updated_at"`

	// V5.6 新增字段
	Source           string    `gorm:"type:varchar(50);default:'upload'" json:"source"`            // manual, upload, ai_generated, converted
	FileName         string    `gorm:"type:varchar(255)" json:"file_name"`                          // 上传的文件名
	FileHash         string    `gorm:"type:varchar(64);index" json:"file_hash"`                     // SHA256哈希，用于去重
	FileSize         int       `gorm:"type:integer" json:"file_size"`                              // 文件大小
	ParsedAt         *time.Time `gorm:"type:timestamp with time zone" json:"parsed_at"`             // 解析时间
	ParseError       string    `gorm:"type:text" json:"parse_error"`                                // 解析错误信息
	AIGenerated      bool      `gorm:"type:boolean;default:false" json:"ai_generated"`              // 是否AI生成
	ParentRuleID     string    `gorm:"type:varchar(100)" json:"parent_rule_id"`                    // 父规则ID
	GenerationPrompt  string    `gorm:"type:text" json:"generation_prompt"`                         // 生成提示词
	GenerationContext string    `gorm:"type:text" json:"generation_context"`                      // 生成上下文
	ApprovedBy       string    `gorm:"type:varchar(100)" json:"approved_by"`                       // 审批人
	ApprovedAt       *time.Time `gorm:"type:timestamp with time zone" json:"approved_at"`          // 审批时间
	DispatchHosts    string    `gorm:"type:text" json:"dispatch_hosts"`               // 已下发的主机ID列表
	DispatchStatus   string    `gorm:"type:varchar(20);default:'pending'" json:"dispatch_status"`  // pending, dispatched, partial_failed
}

func (SigmaRule) TableName() string { return "sigma_rules" }
