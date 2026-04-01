package model

import (
	"time"

	"github.com/google/uuid"
)

type CustomCVEQuery struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CveID                 string     `gorm:"type:varchar(20);not null" json:"cve_id"`
	Status                string     `gorm:"type:varchar(20);not null;default:'querying'" json:"status"`
	ResultVulnerabilityID *uuid.UUID `gorm:"type:uuid" json:"result_vulnerability_id"`
	ErrorMessage          *string    `gorm:"type:text" json:"error_message"`
	ErrorDetail           *string    `gorm:"type:text" json:"error_detail"`
	StartedAt             time.Time  `gorm:"autoCreateTime" json:"started_at"`
	CompletedAt           *time.Time `json:"completed_at"`
	CreatedAt             time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

func (CustomCVEQuery) TableName() string {
	return "custom_cve_queries"
}

const (
	QueryStatusQuerying = "querying"
	QueryStatusSuccess  = "success"
	QueryStatusFailed   = "failed"
)

type CveQueryResult struct {
	CveID            string  `json:"cve_id"`
	Severity         string  `json:"severity"`
	CvssScore        float64 `json:"cvss_score"`
	Description      string  `json:"description"`
	AffectedProducts []struct {
		Product       string   `json:"product"`
		Vendor        string   `json:"vendor"`
		Versions      []string `json:"versions"`
		FixedVersions []string `json:"fixed_versions"`
	} `json:"affected_products"`
	Solution   string   `json:"solution"`
	References []string `json:"references"`
	CweID      string   `json:"cwe_id"`
	Found      bool     `json:"found"`
}
