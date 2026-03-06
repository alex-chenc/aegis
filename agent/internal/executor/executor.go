package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Executor struct {
	sem chan struct{}
}

type ExecuteResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func NewExecutor(maxConcurrency int) *Executor {
	return &Executor{
		sem: make(chan struct{}, maxConcurrency),
	}
}

func (e *Executor) ExecuteCommand(ctx context.Context, taskID, scriptContent string, timeoutSeconds int32) (*ExecuteResult, error) {
	e.sem <- struct{}{}
	defer func() { <-e.sem }()

	// Create temporary script file
	tmpDir := "/tmp/baseline-agent"
	os.MkdirAll(tmpDir, 0755)

	scriptPath := filepath.Join(tmpDir, fmt.Sprintf("%s.sh", taskID))
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return nil, err
	}
	defer os.Remove(scriptPath)

	// Execute with timeout
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", scriptPath)

	stdout, err := cmd.Output()
	stderr := ""
	exitCode := 0

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
			exitCode = exitErr.ExitCode()
		} else {
			return nil, err
		}
	}

	return &ExecuteResult{
		ExitCode: exitCode,
		Stdout:   string(stdout),
		Stderr:   stderr,
	}, nil
}
