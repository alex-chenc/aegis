package model

import (
	"time"

	"github.com/google/uuid"
)

type DetectionPackage struct {
	ID                 uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PackageID          string    `gorm:"type:varchar(160);not null" json:"package_id"`
	Version            string    `gorm:"type:varchar(32);not null" json:"version"`
	Title              string    `gorm:"type:varchar(255)" json:"title"`
	Status             string    `gorm:"type:varchar(32);not null" json:"status"`
	PackageObjectKey   string    `gorm:"type:text" json:"package_object_key"`
	SignatureObjectKey string    `gorm:"type:text" json:"signature_object_key"`
	PackageSize        int64     `json:"package_size"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (DetectionPackage) TableName() string { return "detection_packages" }

type DetectionPackageHostStatus struct {
	ID             uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PackageID      string     `gorm:"type:varchar(160);not null" json:"package_id"`
	Version        string     `gorm:"type:varchar(32);not null" json:"version"`
	HostID         uuid.UUID  `gorm:"type:uuid;not null" json:"host_id"`
	Hostname       string     `gorm:"type:varchar(255)" json:"hostname"`
	Status         string     `gorm:"type:varchar(64);not null" json:"status"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastReportedAt *time.Time `json:"last_reported_at"`
}

func (DetectionPackageHostStatus) TableName() string { return "detection_package_host_status" }
