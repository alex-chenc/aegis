package assets

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

// VersionTool 版本识别工具
type VersionTool struct {
	logger *zap.Logger
}

// NewVersionTool 创建版本工具
func NewVersionTool(logger *zap.Logger) *VersionTool {
	return &VersionTool{logger: logger}
}

// versionCommandTemplate 版本命令模板
// key: 进程名或 exe 路径关键字
// value: 命令参数列表
var versionCommandTemplate = map[string][]string{
	"nginx":          {"/usr/sbin/nginx", "-v"},
	"httpd":          {"/usr/sbin/httpd", "-v"},
	"apache2":        {"/usr/sbin/apache2", "-v"},
	"postgres":       {"/usr/bin/postgres", "--version"},
	"mysql":          {"/usr/bin/mysql", "--version"},
	"mariadb":        {"/usr/bin/mariadb", "--version"},
	"mysqld":         {"/usr/bin/mysqld", "--version"},
	"redis-server":   {"/usr/bin/redis-server", "--version"},
	"redis-sentinel": {"/usr/bin/redis-sentinel", "--version"},
	"mongod":         {"/usr/bin/mongod", "--version"},
	"java":           {"/usr/bin/java", "-version"},
	"node":           {"/usr/bin/node", "--version"},
	"python3":        {"/usr/bin/python3", "--version"},
	"python":         {"/usr/bin/python", "--version"},
	"dotnet":         {"/usr/bin/dotnet", "--version"},
}

var versionPattern = regexp.MustCompile(`(?i)\bv?(\d+(?:\.\d+)+(?:[-_+~][0-9A-Za-z.]+)?)\b`)

// AssetGetProcessResult 进程版本获取结果
type AssetGetProcessResult struct {
	Success bool   `json:"success"`
	Version string `json:"version,omitempty"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// AssetGetProcessVersion 获取进程版本
func (v *VersionTool) AssetGetProcessVersion(ctx context.Context, pid int, exePath, hint string) AssetGetProcessResult {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	v.logger.Info("Getting process version",
		zap.Int("pid", pid),
		zap.String("exe_path", exePath),
		zap.String("hint", hint))

	// 尝试匹配命令模板
	argv := v.matchVersionCommand(exePath, hint)
	if argv == nil {
		return AssetGetProcessResult{
			Success: false,
			Error:   "no matching version command template found",
		}
	}

	// 执行命令（不通过 shell）
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	output, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(output))

	if err != nil {
		return AssetGetProcessResult{
			Success: false,
			Output:  outputStr,
			Error:   err.Error(),
		}
	}

	// 解析版本号
	version := v.extractVersion(outputStr)

	return AssetGetProcessResult{
		Success: true,
		Version: version,
		Output:  outputStr,
	}
}

// matchVersionCommand 匹配版本命令模板
func (v *VersionTool) matchVersionCommand(exePath, hint string) []string {
	// 优先使用 hint 匹配
	if hint != "" {
		hintLower := strings.ToLower(hint)
		for key, argv := range versionCommandTemplate {
			if strings.Contains(hintLower, key) {
				// 检查命令是否存在
				if _, err := os.Stat(argv[0]); err == nil {
					return argv
				}
			}
		}
	}

	// 使用 exePath 匹配
	if exePath != "" {
		exeName := filepath.Base(exePath)
		exeNameLower := strings.ToLower(exeName)

		for key, argv := range versionCommandTemplate {
			if strings.Contains(exeNameLower, key) {
				// 检查命令是否存在
				if _, err := os.Stat(argv[0]); err == nil {
					return argv
				}
				// 尝试使用实际 exe 路径
				return append([]string{exePath}, argv[1:]...)
			}
		}
	}

	return nil
}

// extractVersion 从输出中提取版本号
func (v *VersionTool) extractVersion(output string) string {
	// 常见版本格式：x.y.z, x.y, vx.y.z
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if match := versionPattern.FindString(line); match != "" {
			return strings.TrimPrefix(strings.TrimPrefix(match, "v"), "V")
		}
	}

	return strings.TrimSpace(output)
}

// AssetReadConfigSummaryResult 配置摘要结果
type AssetReadConfigSummaryResult struct {
	Success bool   `json:"success"`
	Summary string `json:"summary,omitempty"`
	Error   string `json:"error,omitempty"`
}

// AssetReadConfigSummary 读取配置文件摘要
func (v *VersionTool) AssetReadConfigSummary(ctx context.Context, configPath string, maxSize int) AssetReadConfigSummaryResult {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if maxSize <= 0 {
		maxSize = 64 * 1024 // 默认 64KB
	}

	// 检查文件是否存在
	info, err := os.Stat(configPath)
	if err != nil {
		return AssetReadConfigSummaryResult{
			Success: false,
			Error:   fmt.Sprintf("file not found: %s", configPath),
		}
	}

	// 检查文件大小
	if info.Size() > int64(maxSize) {
		return AssetReadConfigSummaryResult{
			Success: false,
			Error:   fmt.Sprintf("file too large: %d bytes (max %d)", info.Size(), maxSize),
		}
	}

	// 读取文件内容
	content, err := os.ReadFile(configPath)
	if err != nil {
		return AssetReadConfigSummaryResult{
			Success: false,
			Error:   fmt.Sprintf("failed to read file: %s", err.Error()),
		}
	}

	// 脱敏处理
	summary := RedactConfigSummary(string(content))

	return AssetReadConfigSummaryResult{
		Success: true,
		Summary: summary,
	}
}

// AssetListDirectoryHintsResult 目录提示结果
type AssetListDirectoryHintsResult struct {
	Success bool     `json:"success"`
	Entries []string `json:"entries,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// AssetListDirectoryHints 列出目录关键文件
func (v *VersionTool) AssetListDirectoryHints(ctx context.Context, dirPath string, maxEntries int) AssetListDirectoryHintsResult {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if maxEntries <= 0 {
		maxEntries = 50
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return AssetListDirectoryHintsResult{
			Success: false,
			Error:   fmt.Sprintf("failed to read directory: %s", err.Error()),
		}
	}

	var result []string
	for _, entry := range entries {
		if len(result) >= maxEntries {
			break
		}

		name := entry.Name()
		// 跳过隐藏文件
		if strings.HasPrefix(name, ".") {
			continue
		}

		if entry.IsDir() {
			result = append(result, name+"/")
		} else {
			result = append(result, name)
		}
	}

	return AssetListDirectoryHintsResult{
		Success: true,
		Entries: result,
	}
}

// AssetReadProcFileResult 读取 /proc 文件结果
type AssetReadProcFileResult struct {
	Success   bool   `json:"success"`
	FileName  string `json:"file_name"`
	Content   string `json:"content,omitempty"`
	Size      int    `json:"size"`
	Truncated bool   `json:"truncated,omitempty"`
	Error     string `json:"error,omitempty"`
}

const maxProcFileSize = 10 * 1024 // 10KB

// 禁止读取的 /proc 文件（安全风险）
var forbiddenProcFiles = map[string]bool{
	"environ": true, // 环境变量，可能含密码/token
	"mem":     true, // 进程内存
}

// AssetReadProcFile 读取 /proc/{pid}/ 下的文件
// 安全约束：
// - 只读，不允许写操作
// - 最大读取 10KB
// - 禁止读取 environ 和 mem
// - 只能读取 /proc/{pid}/ 下的文件
func (v *VersionTool) AssetReadProcFile(ctx context.Context, pid int, fileName string) AssetReadProcFileResult {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	v.logger.Info("Reading proc file",
		zap.Int("pid", pid),
		zap.String("file_name", fileName))

	if pid <= 0 {
		return AssetReadProcFileResult{
			Success:  false,
			FileName: fileName,
			Error:    "pid must be positive",
		}
	}

	cleanFileName, err := cleanProcFileName(fileName)
	if err != nil {
		return AssetReadProcFileResult{
			Success:  false,
			FileName: fileName,
			Error:    err.Error(),
		}
	}

	// 构建路径并校验
	procDir := fmt.Sprintf("/proc/%d", pid)
	procPath := filepath.Join(procDir, cleanFileName)
	if procPath != procDir && !strings.HasPrefix(procPath, procDir+"/") {
		return AssetReadProcFileResult{
			Success:  false,
			FileName: fileName,
			Error:    "file path escapes process directory",
		}
	}

	// 解析 symlink 确保路径在 /proc/{pid}/ 下
	resolved, err := filepath.EvalSymlinks(procPath)
	if err != nil {
		return AssetReadProcFileResult{
			Success:  false,
			FileName: fileName,
			Error:    fmt.Sprintf("failed to resolve path: %v", err),
		}
	}

	// 确保解析后的路径仍在 /proc/{pid}/ 下
	expectedPrefix := fmt.Sprintf("/proc/%d/", pid)
	if !strings.HasPrefix(resolved, expectedPrefix) && resolved != fmt.Sprintf("/proc/%d", pid) {
		return AssetReadProcFileResult{
			Success:  false,
			FileName: fileName,
			Error:    "resolved path escapes process directory",
		}
	}

	select {
	case <-ctx.Done():
		return AssetReadProcFileResult{
			Success:  false,
			FileName: cleanFileName,
			Error:    ctx.Err().Error(),
		}
	default:
	}

	file, err := os.Open(resolved)
	if err != nil {
		return AssetReadProcFileResult{
			Success:  false,
			FileName: cleanFileName,
			Error:    fmt.Sprintf("failed to open file: %v", err),
		}
	}
	defer file.Close()

	// 读取文件（最大 10KB），避免先将大型 /proc 文件完整读入内存。
	data, err := io.ReadAll(io.LimitReader(file, maxProcFileSize+1))
	if err != nil {
		return AssetReadProcFileResult{
			Success:  false,
			FileName: cleanFileName,
			Error:    fmt.Sprintf("failed to read file: %v", err),
		}
	}
	size := len(data)

	truncated := false
	if len(data) > maxProcFileSize {
		data = data[:maxProcFileSize]
		truncated = true
	}

	return AssetReadProcFileResult{
		Success:   true,
		FileName:  cleanFileName,
		Content:   string(data),
		Size:      size,
		Truncated: truncated,
	}
}

func cleanProcFileName(fileName string) (string, error) {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "", fmt.Errorf("file_name is required")
	}
	if filepath.IsAbs(fileName) {
		return "", fmt.Errorf("file_name must be relative to /proc/{pid}")
	}

	cleaned := filepath.Clean(fileName)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("file_name escapes process directory")
	}

	for _, part := range strings.Split(filepath.ToSlash(cleaned), "/") {
		if forbiddenProcFiles[part] {
			return "", fmt.Errorf("reading %s is forbidden for security reasons", part)
		}
	}

	return cleaned, nil
}

// AssetResolvePackageByFileResult 包解析结果
type AssetResolvePackageByFileResult struct {
	Success bool   `json:"success"`
	Package string `json:"package,omitempty"`
	Error   string `json:"error,omitempty"`
}

// AssetResolvePackageByFile 通过文件路径查找所属软件包
func (v *VersionTool) AssetResolvePackageByFile(ctx context.Context, filePath string) AssetResolvePackageByFileResult {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	v.logger.Info("Resolving package by file", zap.String("file_path", filePath))

	// 尝试 RPM
	if pkg := v.resolveByRPM(ctx, filePath); pkg != "" {
		return AssetResolvePackageByFileResult{
			Success: true,
			Package: pkg,
		}
	}

	// 尝试 DPKG
	if pkg := v.resolveByDPKG(ctx, filePath); pkg != "" {
		return AssetResolvePackageByFileResult{
			Success: true,
			Package: pkg,
		}
	}

	// 尝试 APK
	if pkg := v.resolveByAPK(ctx, filePath); pkg != "" {
		return AssetResolvePackageByFileResult{
			Success: true,
			Package: pkg,
		}
	}

	return AssetResolvePackageByFileResult{
		Success: false,
		Error:   "package not found for file",
	}
}

// resolveByRPM 通过 RPM 查找包
func (v *VersionTool) resolveByRPM(ctx context.Context, filePath string) string {
	cmd := exec.CommandContext(ctx, "rpm", "-qf", "--queryformat", "%{NAME}-%{VERSION}-%{RELEASE}.%{ARCH}", filePath)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// resolveByDPKG 通过 DPKG 查找包
func (v *VersionTool) resolveByDPKG(ctx context.Context, filePath string) string {
	cmd := exec.CommandContext(ctx, "dpkg", "-S", filePath)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// 格式: package: /path/to/file
	parts := strings.SplitN(string(output), ":", 2)
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

// resolveByAPK 通过 APK 查找包
func (v *VersionTool) resolveByAPK(ctx context.Context, filePath string) string {
	cmd := exec.CommandContext(ctx, "apk", "info", "--who-owns", filePath)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// 解析输出获取包名
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "owned by") {
			parts := strings.Split(line, "owned by")
			if len(parts) >= 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}
