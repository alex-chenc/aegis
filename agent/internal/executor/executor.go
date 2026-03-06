package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Executor 命令执行器
type Executor struct {
	sem chan struct{}
}

// ExecuteResult 执行结果
type ExecuteResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
}

// NewExecutor 创建执行器，maxConcurrency 为最大并发数
func NewExecutor(maxConcurrency int) *Executor {
	return &Executor{
		sem: make(chan struct{}, maxConcurrency),
	}
}

// ExecuteCommand 执行命令
func (e *Executor) ExecuteCommand(ctx context.Context, taskID, scriptContent string, timeoutSeconds int32) *ExecuteResult {
	// 获取信号量
	e.sem <- struct{}{}
	defer func() { <-e.sem }()

	// 创建临时目录
	tmpDir := filepath.Join("/tmp/baseline-agent", taskID)
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return &ExecuteResult{ExitCode: 1, Stderr: fmt.Sprintf("failed to create temp dir: %v", err)}
	}
	defer os.RemoveAll(tmpDir)

	// 创建脚本文件
	scriptPath := filepath.Join(tmpDir, "script.sh")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0700); err != nil {
		return &ExecuteResult{ExitCode: 1, Stderr: fmt.Sprintf("failed to write script: %v", err)}
	}

	// 创建带超时的 context
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// 创建命令
	cmd := exec.CommandContext(execCtx, "bash", scriptPath)

	// 创建管道捕获输出
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	var stdout, stderr string
	var wg sync.WaitGroup

	// 读取 stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		out, _ := io.ReadAll(stdoutPipe)
		stdout = string(out)
	}()

	// 读取 stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		errOut, _ := io.ReadAll(stderrPipe)
		stderr = string(errOut)
	}()

	// 执行命令
	err := cmd.Start()
	if err != nil {
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
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
	}

	return result
}
