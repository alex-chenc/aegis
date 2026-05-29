package repository

import (
	"api-server/internal/model"

	"gorm.io/gorm"
)

type RoleRepo struct {
	db *gorm.DB
}

func NewRoleRepo(db *gorm.DB) *RoleRepo {
	return &RoleRepo{db: db}
}

func (r *RoleRepo) GetRole(userID string) (string, error) {
	var rp model.RolePermission
	err := r.db.Where("user_id = ?", userID).First(&rp).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return model.RoleSecurityAnalyst, nil
		}
		return "", err
	}
	return rp.Role, nil
}

// HasRoleRecord returns true if the user has a role record in the database.
func (r *RoleRepo) HasRoleRecord(userID string) (bool, error) {
	var count int64
	err := r.db.Model(&model.RolePermission{}).Where("user_id = ?", userID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// HasAnyRoles returns true if the role_permissions table has any records at all.
func (r *RoleRepo) HasAnyRoles() (bool, error) {
	var count int64
	err := r.db.Model(&model.RolePermission{}).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *RoleRepo) SetRole(userID, role string) error {
	var rp model.RolePermission
	err := r.db.Where("user_id = ?", userID).First(&rp).Error
	if err == gorm.ErrRecordNotFound {
		return r.db.Create(&model.RolePermission{UserID: userID, Role: role}).Error
	}
	if err != nil {
		return err
	}
	rp.Role = role
	return r.db.Save(&rp).Error
}

func (r *RoleRepo) HasPermission(role, operation string) bool {
	permissions := map[string][]string{
		model.RoleSecurityAnalyst:   {"view", "draft", "ai_generate"},
		model.RoleSecurityDeveloper: {"view", "draft", "ai_generate", "build", "review", "sign"},
		model.RoleAdmin:             {"view", "draft", "ai_generate", "build", "review", "sign", "enable", "disable", "uninstall", "rollback", "allowlist"},
	}
	ops, ok := permissions[role]
	if !ok {
		return false
	}
	for _, op := range ops {
		if op == operation {
			return true
		}
	}
	return false
}
