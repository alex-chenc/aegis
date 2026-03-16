package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aegis-agent/internal/logger"

	"go.uber.org/zap"
)

type Executor struct {
	sem chan struct{}
}

type ExecuteResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
}

func NewExecutor(maxConcurrency int) *Executor {
	return &Executor{
		sem: make(chan struct{}, maxConcurrency),
	}
}

func (e *Executor) ExecuteCommand(ctx context.Context, taskID, scriptContent string, timeoutSeconds int32) *ExecuteResult {
	e.sem <- struct{}{}
	defer func() { <-e.sem }()

	logger.Info("Executing command",
		zap.String("task_id", taskID),
		zap.Int32("timeout_seconds", timeoutSeconds))

	if strings.TrimSpace(scriptContent) == "#SOFTWARE_COLLECT#" {
		return e.collectSoftwareList(ctx, taskID, timeoutSeconds)
	}

	tmpDir := filepath.Join("/tmp/aegis-agent", taskID)
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		logger.Error("Failed to create temp dir",
			zap.String("task_id", taskID),
			zap.Error(err))
		return &ExecuteResult{ExitCode: 1, Stderr: fmt.Sprintf("failed to create temp dir: %v", err)}
	}
	defer os.RemoveAll(tmpDir)

	scriptPath := filepath.Join(tmpDir, "script.sh")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0700); err != nil {
		logger.Error("Failed to write script",
			zap.String("task_id", taskID),
			zap.Error(err))
		return &ExecuteResult{ExitCode: 1, Stderr: fmt.Sprintf("failed to write script: %v", err)}
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "bash", scriptPath)

	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	var stdout, stderr string
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		out, _ := io.ReadAll(stdoutPipe)
		stdout = string(out)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errOut, _ := io.ReadAll(stderrPipe)
		stderr = string(errOut)
	}()

	err := cmd.Start()
	if err != nil {
		logger.Error("Failed to start command",
			zap.String("task_id", taskID),
			zap.Error(err))
		return &ExecuteResult{ExitCode: 1, Stderr: fmt.Sprintf("failed to start: %v", err)}
	}

	wg.Wait()
	err = cmd.Wait()

	result := &ExecuteResult{
		ExitCode: 0,
		Stdout:   stdout,
		Stderr:   stderr,
		TimedOut: false,
	}

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.Stderr = stderr + "\n[TIMEOUT]"
			logger.Warn("Command timed out", zap.String("task_id", taskID))
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
	}

	logger.Info("Command execution completed",
		zap.String("task_id", taskID),
		zap.Int("exit_code", result.ExitCode),
		zap.Bool("timed_out", result.TimedOut))

	return result
}

func (e *Executor) collectSoftwareList(ctx context.Context, taskID string, timeoutSeconds int32) *ExecuteResult {
	logger.Info("Collecting software list", zap.String("task_id", taskID))

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	var shellCmd string

	if _, err := exec.LookPath("dpkg-query"); err == nil {
		logger.Info("using dpkg-query for software collection", zap.String("task_id", taskID))
		shellCmd = `dpkg-query -W -f='${Package}\t${Version}\t${Architecture}\n' | awk -F'\t' '{printf "{\"name\":\"%s\",\"version\":\"%s\"}\n", $1, $2}'`
		cmd = exec.CommandContext(execCtx, "bash", "-c", shellCmd)
	} else if _, err := exec.LookPath("rpm"); err == nil {
		logger.Info("using rpm for software collection", zap.String("task_id", taskID))
		shellCmd = `rpm -qa --qf '%{NAME}\t%{VERSION}\n' | awk -F'\t' '{printf "{\"name\":\"%s\",\"version\":\"%s\"}\n", $1, $2}'`
		cmd = exec.CommandContext(execCtx, "bash", "-c", shellCmd)
	} else {
		logger.Error("no package manager found", zap.String("task_id", taskID))
		return &ExecuteResult{ExitCode: 1, Stderr: "no supported package manager found (dpkg-query or rpm)"}
	}

	out, err := cmd.Output()
	logger.Info("rpm/dpkg command completed",
		zap.String("task_id", taskID),
		zap.Error(err),
		zap.Int("output_bytes", len(out)))

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			logger.Warn("Software collection timed out", zap.String("task_id", taskID))
			return &ExecuteResult{ExitCode: 1, Stderr: "[TIMEOUT]", TimedOut: true}
		}
		exitCode := 1
		stderr := fmt.Sprintf("failed to collect software list: %v", err)
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			stderr = string(exitErr.Stderr)
		}
		logger.Error("Software collection failed", zap.String("task_id", taskID), zap.Error(err))
		return &ExecuteResult{ExitCode: exitCode, Stderr: stderr}
	}

	logger.Info("Software collection completed",
		zap.String("task_id", taskID),
		zap.Int("packages", strings.Count(string(out), "\n")))

	return &ExecuteResult{ExitCode: 0, Stdout: strings.TrimSpace(string(out))}
}
