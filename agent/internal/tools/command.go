package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type CommandResult struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration int    `json:"duration_ms"`
}

func (m *ToolManager) ExecuteCommand(command string) (*CommandResult, error) {
	cmdName := strings.Split(command, " ")[0]
	if !m.isAllowed(cmdName) {
		return nil, fmt.Errorf("command not allowed: %s", cmdName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	duration := time.Since(start).Milliseconds()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	result := &CommandResult{
		Command:  command,
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: int(duration),
	}

	if err != nil {
		return result, nil
	}

	return result, nil
}

func (m *ToolManager) isAllowed(cmdName string) bool {
	for _, allowed := range m.allowedCommands {
		if allowed == cmdName {
			return true
		}
	}
	return false
}
