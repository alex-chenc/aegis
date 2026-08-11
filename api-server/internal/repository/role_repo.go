package repository

import (
	"api-server/internal/model"

	"gorm.io/gorm"
)

const (
	PermissionAgentGuardRead          = "agent_guard:read"
	PermissionAgentGuardEvidenceRead  = "agent_guard:evidence:read"
	PermissionAgentGuardAnalysisRead  = "agent_guard:analysis:read"
	PermissionAgentGuardPolicyWrite   = "agent_guard:policy:write"
	PermissionAgentGuardPolicyPublish = "agent_guard:policy:publish"
	PermissionAgentGuardAnalysisRun   = "agent_guard:analysis:run"
	PermissionAgentGuardSessionDelete = "agent_guard:session:delete"
	PermissionAgentGuardActionFreeze  = "agent_guard:action:freeze"
	PermissionAgentGuardActionResume  = "agent_guard:action:resume"
	PermissionAgentGuardActionKill    = "agent_guard:action:kill"
	PermissionAgentGuardSettings      = "agent_guard:settings"
	PermissionMCPOnboardingRead       = "mcp:onboarding:read"
	PermissionMCPOnboardingCreate     = "mcp:onboarding:create"
	PermissionMCPOnboardingOperate    = "mcp:onboarding:operate"
	PermissionMCPServerRead           = "mcp:server:read"
	PermissionMCPServerWrite          = "mcp:server:write"
	PermissionMCPServerDiscover       = "mcp:server:discover"
	PermissionMCPServerReview         = "mcp:server:review"
	PermissionMCPCatalogRead          = "mcp:catalog:read"
	PermissionMCPCatalogWrite         = "mcp:catalog:write"
	PermissionMCPCatalogPublish       = "mcp:catalog:publish"
	PermissionMCPClientRead           = "mcp:client:read"
	PermissionMCPClientWrite          = "mcp:client:write"
	PermissionMCPGrantWrite           = "mcp:grant:write"
	PermissionMCPApprovalRead         = "mcp:approval:read"
	PermissionMCPApprovalDecide       = "mcp:approval:decide"
	PermissionMCPInvocationRead       = "mcp:invocation:read"
	PermissionMCPAuditPayloadRead     = "mcp:audit:payload:read"
	PermissionMCPSecurityRead         = "mcp:security:read"
	PermissionMCPSecurityAIRetry      = "mcp:security:ai:retry"
	PermissionMCPPolicyRead           = "mcp:policy:read"
	PermissionMCPPolicyWrite          = "mcp:policy:write"
	PermissionMCPPolicyPublish        = "mcp:policy:publish"
	PermissionMCPBreakGlass           = "mcp:break_glass"
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
	permissions := permissionMap()
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

// ListPermissions returns a copy so authentication responses cannot mutate
// the process-wide role policy.
func (r *RoleRepo) ListPermissions(role string) []string {
	items := permissionMap()[role]
	return append([]string(nil), items...)
}

func permissionMap() map[string][]string {
	return map[string][]string{
		model.RoleSecurityAnalyst: {
			"view",
			"draft",
			"ai_generate",
			PermissionAgentGuardRead,
			PermissionAgentGuardAnalysisRead,
			PermissionMCPOnboardingRead,
			PermissionMCPServerRead,
			PermissionMCPCatalogRead,
			PermissionMCPClientRead,
			PermissionMCPApprovalRead,
			PermissionMCPInvocationRead,
			PermissionMCPSecurityRead,
		},
		model.RoleSecurityDeveloper: {
			"view",
			"draft",
			"ai_generate",
			"build",
			"review",
			"sign",
			PermissionAgentGuardRead,
			PermissionAgentGuardEvidenceRead,
			PermissionAgentGuardAnalysisRead,
			PermissionAgentGuardPolicyWrite,
			PermissionAgentGuardAnalysisRun,
			PermissionMCPOnboardingRead,
			PermissionMCPOnboardingCreate,
			PermissionMCPOnboardingOperate,
			PermissionMCPServerRead,
			PermissionMCPServerDiscover,
			PermissionMCPServerReview,
			PermissionMCPCatalogRead,
			PermissionMCPClientRead,
			PermissionMCPApprovalRead,
			PermissionMCPInvocationRead,
			PermissionMCPSecurityRead,
		},
		model.RoleAdmin: {
			"view",
			"draft",
			"ai_generate",
			"build",
			"review",
			"sign",
			"enable",
			"disable",
			"uninstall",
			"rollback",
			"allowlist",
			PermissionAgentGuardRead,
			PermissionAgentGuardEvidenceRead,
			PermissionAgentGuardAnalysisRead,
			PermissionAgentGuardPolicyWrite,
			PermissionAgentGuardPolicyPublish,
			PermissionAgentGuardAnalysisRun,
			PermissionAgentGuardSessionDelete,
			PermissionAgentGuardActionFreeze,
			PermissionAgentGuardActionResume,
			PermissionAgentGuardActionKill,
			PermissionAgentGuardSettings,
			PermissionMCPOnboardingRead,
			PermissionMCPOnboardingCreate,
			PermissionMCPOnboardingOperate,
			PermissionMCPServerRead,
			PermissionMCPServerWrite,
			PermissionMCPServerDiscover,
			PermissionMCPServerReview,
			PermissionMCPCatalogRead,
			PermissionMCPCatalogWrite,
			PermissionMCPCatalogPublish,
			PermissionMCPClientRead,
			PermissionMCPClientWrite,
			PermissionMCPGrantWrite,
			PermissionMCPApprovalRead,
			PermissionMCPApprovalDecide,
			PermissionMCPInvocationRead,
			PermissionMCPAuditPayloadRead,
			PermissionMCPSecurityRead,
			PermissionMCPSecurityAIRetry,
			PermissionMCPPolicyRead,
			PermissionMCPPolicyWrite,
			PermissionMCPPolicyPublish,
			PermissionMCPBreakGlass,
		},
	}
}
