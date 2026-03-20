package blocker

import (
	"os/exec"

	"aegis-agent/internal/logger"
	"go.uber.org/zap"
)

func (b *Blocker) DisableUser(username string) error {
	cmd := exec.Command("usermod", "-L", username)
	if err := cmd.Run(); err != nil {
		return err
	}

	b.recordAudit("disable_user", username, "", "success")
	logger.Info("User disabled", zap.String("username", username))
	return nil
}

func (b *Blocker) RevokePermission(filePath string) error {
	cmd := exec.Command("chmod", "000", filePath)
	if err := cmd.Run(); err != nil {
		return err
	}

	b.recordAudit("revoke_permission", filePath, "", "success")
	logger.Info("Permission revoked", zap.String("path", filePath))
	return nil
}
