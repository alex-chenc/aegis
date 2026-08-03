package repository

import (
	"testing"

	"api-server/internal/model"
)

func TestAgentGuardRolePermissions(t *testing.T) {
	repo := &RoleRepo{}

	if !repo.HasPermission(model.RoleSecurityAnalyst, PermissionAgentGuardRead) {
		t.Fatal("security analyst must be able to read Agent Guard summaries")
	}
	if repo.HasPermission(model.RoleSecurityAnalyst, PermissionAgentGuardEvidenceRead) {
		t.Fatal("security analyst must not receive detailed evidence permission")
	}
	if !repo.HasPermission(model.RoleSecurityDeveloper, PermissionAgentGuardPolicyWrite) {
		t.Fatal("security developer must be able to edit policy drafts")
	}
	if repo.HasPermission(model.RoleSecurityDeveloper, PermissionAgentGuardPolicyPublish) {
		t.Fatal("security developer must not publish Agent Guard policies")
	}

	adminPermissions := []string{
		PermissionAgentGuardRead,
		PermissionAgentGuardEvidenceRead,
		PermissionAgentGuardPolicyWrite,
		PermissionAgentGuardPolicyPublish,
		PermissionAgentGuardAnalysisRun,
		PermissionAgentGuardActionFreeze,
		PermissionAgentGuardActionResume,
		PermissionAgentGuardActionKill,
	}
	for _, permission := range adminPermissions {
		if !repo.HasPermission(model.RoleAdmin, permission) {
			t.Fatalf("admin missing permission %q", permission)
		}
	}
}
