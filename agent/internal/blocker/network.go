package blocker

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strings"

	"aegis-agent/internal/logger"
	"go.uber.org/zap"
)

func (b *Blocker) BlockConnection(remoteAddr string) error {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return fmt.Errorf("missing target for block_connection")
	}
	if ip := net.ParseIP(remoteAddr); ip == nil {
		if _, _, err := net.ParseCIDR(remoteAddr); err != nil {
			return fmt.Errorf("invalid remote address for block_connection: %s", remoteAddr)
		}
	}

	cmd := exec.Command("iptables", "-A", "OUTPUT", "-d", remoteAddr, "-j", "DROP")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return fmt.Errorf("iptables block failed for %s: %s: %w", remoteAddr, detail, err)
		}
		return fmt.Errorf("iptables block failed for %s: %w", remoteAddr, err)
	}

	b.recordAudit("block_connection", remoteAddr, "", "success")
	logger.Info("Connection blocked", zap.String("addr", remoteAddr))
	return nil
}
