package blocker

import (
	"os/exec"

	"aegis-agent/internal/logger"
	"go.uber.org/zap"
)

func (b *Blocker) BlockConnection(remoteAddr string) error {
	cmd := exec.Command("iptables", "-A", "OUTPUT", "-d", remoteAddr, "-j", "DROP")
	if err := cmd.Run(); err != nil {
		return err
	}

	b.recordAudit("block_connection", remoteAddr, "", "success")
	logger.Info("Connection blocked", zap.String("addr", remoteAddr))
	return nil
}
