package blocker

import (
	"fmt"
	"os"
	"strings"

	"aegis-agent/internal/logger"
	"go.uber.org/zap"
)

func (b *Blocker) KillProcess(pid int) error {
	comm, _ := getProcessComm(pid)
	for _, p := range b.protectedProcs {
		if p == comm {
			return fmt.Errorf("protected process: %s (pid: %d)", comm, pid)
		}
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %d", pid)
	}

	if err := process.Kill(); err != nil {
		return fmt.Errorf("failed to kill process %d: %w", pid, err)
	}

	b.recordAudit("kill_process", fmt.Sprintf("%d", pid), comm, "success")
	logger.Info("Process killed", zap.Int("pid", pid), zap.String("comm", comm))
	return nil
}

func getProcessComm(pid int) (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
