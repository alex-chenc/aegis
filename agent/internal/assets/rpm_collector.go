package assets

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"
)

// RPMCollector RPM 包管理器采集器
type RPMCollector struct {
	logger *zap.Logger
}

// NewRPMCollector 创建 RPM 采集器
func NewRPMCollector(logger *zap.Logger) *RPMCollector {
	return &RPMCollector{logger: logger}
}

// Collect 采集 RPM 软件包
func (c *RPMCollector) Collect(ctx context.Context, includeFiles bool) ([]PackageAsset, error) {
	// 优先使用 rpm 命令采集
	packages, err := c.collectViaCommand(ctx, includeFiles)
	if err != nil {
		c.logger.Warn("RPM command collection failed, trying database", zap.Error(err))
		// 降级到数据库解析（如果实现的话）
		return nil, fmt.Errorf("rpm collection failed: %w", err)
	}
	return packages, nil
}

// collectViaCommand 通过 rpm 命令采集
func (c *RPMCollector) collectViaCommand(ctx context.Context, includeFiles bool) ([]PackageAsset, error) {
	// 设置超时
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// 查询格式: name|epoch|version|release|arch|installtime|sourcerpm|license|vendor
	queryFormat := "%{NAME}|%{EPOCH}|%{VERSION}|%{RELEASE}|%{ARCH}|%{INSTALLTIME}|%{SOURCERPM}|%{LICENSE}|%{VENDOR}\n"
	cmd := exec.CommandContext(ctx, "rpm", "-qa", "--queryformat", queryFormat)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start rpm command: %w", err)
	}

	var packages []PackageAsset
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		pkg, err := c.parseRPMLine(line)
		if err != nil {
			c.logger.Warn("Failed to parse RPM line", zap.String("line", line), zap.Error(err))
			continue
		}

		// 如果需要采集文件列表
		if includeFiles {
			files := c.collectFiles(ctx, pkg.Name)
			pkg.InstallPaths = files
			pkg.FileCount = len(files)
		}

		packages = append(packages, pkg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read rpm output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("rpm command failed: %w", err)
	}

	return packages, nil
}

// parseRPMLine 解析 RPM 输出行
func (c *RPMCollector) parseRPMLine(line string) (PackageAsset, error) {
	parts := strings.Split(line, "|")
	if len(parts) < 9 {
		return PackageAsset{}, fmt.Errorf("invalid RPM output format: %s", line)
	}

	epoch := parts[1]
	if epoch == "(none)" {
		epoch = ""
	}

	var installTime time.Time
	if ts := parts[5]; ts != "0" && ts != "(none)" {
		if t, err := time.Parse("2006-01-02 15:04:05 -0700", ts); err == nil {
			installTime = t
		}
	}

	return PackageAsset{
		Name:           parts[0],
		Version:        parts[2],
		Release:        parts[3],
		Epoch:          epoch,
		Architecture:   parts[4],
		PackageManager: "rpm",
		InstallTime:    installTime,
		SourceName:     parts[6],
		License:        parts[7],
		Vendor:         parts[8],
		Metadata:       make(map[string]string),
	}, nil
}

// collectFiles 采集软件包安装的文件列表
func (c *RPMCollector) collectFiles(ctx context.Context, packageName string) []string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rpm", "-ql", packageName)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var files []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "(contains no files)") {
			files = append(files, line)
		}
	}

	// 限制文件数量
	maxFiles := 200
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}

	return files
}
