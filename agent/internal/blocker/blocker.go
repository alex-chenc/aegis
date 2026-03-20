package blocker

import (
	"fmt"
	"strconv"

	"aegis-agent/internal/logger"

	"go.uber.org/zap"
)

type Blocker struct {
	quarantineDir  string
	protectedProcs []string
}

func NewBlocker(quarantineDir string) *Blocker {
	if quarantineDir == "" {
		quarantineDir = "/var/quarantine"
	}
	return &Blocker{
		quarantineDir: quarantineDir,
		protectedProcs: []string{
			"systemd", "sshd", "init", "kthreadd",
			"dockerd", "containerd", "kubelet",
			"postgres", "redis-server", "nginx",
		},
	}
}

func (b *Blocker) Execute(action, target string) error {
	switch action {
	case "kill_process":
		pid, err := strconv.Atoi(target)
		if err != nil {
			return fmt.Errorf("invalid pid: %s", target)
		}
		return b.KillProcess(pid)
	case "quarantine_file":
		return b.QuarantineFile(target)
	case "block_connection":
		return b.BlockConnection(target)
	case "disable_user":
		return b.DisableUser(target)
	case "revoke_permission":
		return b.RevokePermission(target)
	default:
		return fmt.Errorf("unknown block action: %s", action)
	}
}

func (b *Blocker) recordAudit(action, target, details, result string) {
	logger.Info("Blocker audit",
		zap.String("action", action),
		zap.String("target", target),
		zap.String("details", details),
		zap.String("result", result))
}
