package blocker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestBlockerHostStrategies(t *testing.T) {
	if os.Getenv("AEGIS_BLOCKER_HOST_TEST") != "1" {
		t.Skip("set AEGIS_BLOCKER_HOST_TEST=1 to run host-level blocking strategy tests")
	}

	t.Run("kill_process", func(t *testing.T) {
		cmd := exec.Command("sleep", "60")
		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start test process: %v", err)
		}
		defer func() {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}()

		b := NewBlocker(t.TempDir())
		if err := b.Execute("kill_process", strconv.Itoa(cmd.Process.Pid)); err != nil {
			t.Fatalf("kill_process failed: %v", err)
		}

		waitDone := make(chan error, 1)
		go func() { waitDone <- cmd.Wait() }()
		select {
		case <-waitDone:
		case <-time.After(3 * time.Second):
			t.Fatalf("expected process %d to exit after kill_process", cmd.Process.Pid)
		}
	})

	t.Run("quarantine_file", func(t *testing.T) {
		tmp := t.TempDir()
		quarantineDir := filepath.Join(tmp, "quarantine")
		target := filepath.Join(tmp, "aegis-block-test-file")
		if err := os.WriteFile(target, []byte("test"), 0600); err != nil {
			t.Fatalf("failed to create quarantine target: %v", err)
		}

		b := NewBlocker(quarantineDir)
		if err := b.Execute("quarantine_file", target); err != nil {
			t.Fatalf("quarantine_file failed: %v", err)
		}
		if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected original file to be moved, stat err=%v", err)
		}
		matches, err := filepath.Glob(filepath.Join(quarantineDir, "aegis-block-test-file.*"))
		if err != nil {
			t.Fatalf("failed to inspect quarantine dir: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("expected one quarantined file, got %d", len(matches))
		}
	})

	t.Run("block_connection", func(t *testing.T) {
		if _, err := exec.LookPath("iptables"); err != nil {
			t.Fatalf("iptables is required for block_connection host test: %v", err)
		}
		target := "203.0.113.25"
		_ = exec.Command("iptables", "-D", "OUTPUT", "-d", target, "-j", "DROP").Run()
		t.Cleanup(func() {
			_ = exec.Command("iptables", "-D", "OUTPUT", "-d", target, "-j", "DROP").Run()
		})

		b := NewBlocker(t.TempDir())
		if err := b.Execute("block_connection", target); err != nil {
			t.Fatalf("block_connection failed: %v", err)
		}
		if err := exec.Command("iptables", "-C", "OUTPUT", "-d", target, "-j", "DROP").Run(); err != nil {
			t.Fatalf("expected iptables DROP rule for %s: %v", target, err)
		}
	})
}
