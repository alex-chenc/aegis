package weakpass

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func (c *Collector) ProbePath(ctx context.Context, params map[string]interface{}) (*PathProbeResult, error) {
	_ = ctx
	if err := validateToolArgsNoShell(params); err != nil {
		return nil, err
	}
	var req PathProbeRequest
	if err := mapToStruct(params, &req); err != nil {
		return nil, err
	}
	if err := validatePath(req.Path); err != nil {
		return nil, err
	}
	info, err := os.Stat(req.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PathProbeResult{Path: req.Path, Exists: false}, nil
		}
		return nil, err
	}
	result := &PathProbeResult{
		Path:   req.Path,
		Exists: true,
		Type:   "file",
		Size:   info.Size(),
		Mode:   fmt.Sprintf("%04o", info.Mode().Perm()),
		Owner:  lookupOwner(info),
	}
	if info.IsDir() {
		result.Type = "directory"
	}
	return result, nil
}

func (c *Collector) ListConfigDir(ctx context.Context, params map[string]interface{}) (*ConfigDirListResult, error) {
	_ = ctx
	if err := validateToolArgsNoShell(params); err != nil {
		return nil, err
	}
	var req ConfigDirListRequest
	if err := mapToStruct(params, &req); err != nil {
		return nil, err
	}
	if req.Recursive {
		return nil, fmt.Errorf("%s: recursive listing is forbidden", ErrRecursiveNotAllowed)
	}
	if err := validatePath(req.Dir); err != nil {
		return nil, err
	}
	if req.MaxEntries <= 0 || req.MaxEntries > 200 {
		req.MaxEntries = 200
	}
	entries, err := os.ReadDir(req.Dir)
	if err != nil {
		return nil, err
	}
	result := &ConfigDirListResult{Dir: req.Dir, Entries: []ConfigDirEntry{}}
	for _, entry := range entries {
		if len(result.Entries) >= req.MaxEntries {
			break
		}
		fullPath := filepath.Join(req.Dir, entry.Name())
		if entry.IsDir() || !hasAllowedSuffix(fullPath, req.SuffixAllowlist) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		result.Entries = append(result.Entries, ConfigDirEntry{
			Name: entry.Name(),
			Path: fullPath,
			Type: "file",
			Size: info.Size(),
		})
	}
	return result, nil
}

func (c *Collector) ReadConfigSlice(ctx context.Context, params map[string]interface{}) (*ConfigSliceResult, error) {
	_ = ctx
	if err := validateToolArgsNoShell(params); err != nil {
		return nil, err
	}
	var req ConfigSliceRequest
	if err := mapToStruct(params, &req); err != nil {
		return nil, err
	}
	if err := validatePath(req.Path); err != nil {
		return nil, err
	}
	if req.MaxBytes <= 0 || req.MaxBytes > maxConfigSliceBytes {
		req.MaxBytes = maxConfigSliceBytes
	}
	content, err := readAllowedFile(req.Path, req.MaxBytes)
	if err != nil {
		if errorsCode, _ := fileErrorCode(err); errorsCode == ErrFileTooLarge {
			content, err = readPrefix(req.Path, req.MaxBytes)
		}
		if err != nil {
			return nil, err
		}
	}
	if req.StartLine <= 0 {
		req.StartLine = 1
	}
	if req.EndLine <= 0 || req.EndLine < req.StartLine {
		req.EndLine = req.StartLine + 80
	}
	if req.EndLine-req.StartLine > 160 {
		req.EndLine = req.StartLine + 160
	}
	allLines := splitLines(content)
	var lines []string
	for i := req.StartLine; i <= req.EndLine && i <= len(allLines); i++ {
		line := allLines[i-1]
		if req.RedactValues {
			line = redactConfigLine(line)
		}
		lines = append(lines, line)
	}
	return &ConfigSliceResult{
		Path:      req.Path,
		StartLine: req.StartLine,
		EndLine:   req.EndLine,
		Lines:     lines,
		Truncated: int64(len(content)) >= req.MaxBytes,
		Redacted:  req.RedactValues,
	}, nil
}

func (c *Collector) ServiceUnitInspect(ctx context.Context, params map[string]interface{}) (*ServiceUnitInspectResult, error) {
	_ = ctx
	if err := validateToolArgsNoShell(params); err != nil {
		return nil, err
	}
	var req ServiceUnitInspectRequest
	if err := mapToStruct(params, &req); err != nil {
		return nil, err
	}
	paths := req.Paths
	if len(paths) == 0 && req.Service != "" {
		serviceName := req.Service
		if !strings.HasSuffix(serviceName, ".service") {
			serviceName += ".service"
		}
		paths = []string{
			filepath.Join("/etc/systemd/system", serviceName),
			filepath.Join("/lib/systemd/system", serviceName),
			filepath.Join("/usr/lib/systemd/system", serviceName),
		}
	}
	for _, path := range paths {
		if err := validatePath(path); err != nil {
			continue
		}
		content, err := readAllowedFile(path, maxConfigSliceBytes)
		if err != nil {
			continue
		}
		result := parseUnitFile(req.Service, path, content)
		return result, nil
	}
	return nil, fmt.Errorf("%s: service unit not found", ErrFileNotFound)
}

func (c *Collector) ProcessConfigHints(ctx context.Context, params map[string]interface{}) (*ProcessConfigHintsResult, error) {
	_ = ctx
	if err := validateToolArgsNoShell(params); err != nil {
		return nil, err
	}
	var req ProcessConfigHintsRequest
	if err := mapToStruct(params, &req); err != nil {
		return nil, err
	}
	if req.PID <= 0 {
		return nil, fmt.Errorf("%s: pid is required", ErrInvalidRequest)
	}
	if req.MaxFiles <= 0 || req.MaxFiles > 20 {
		req.MaxFiles = 20
	}
	result := &ProcessConfigHintsResult{PID: req.PID}
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", req.PID)
	if content, err := os.ReadFile(cmdlinePath); err == nil {
		for _, part := range strings.Split(string(content), "\x00") {
			if part != "" {
				result.Cmdline = append(result.Cmdline, part)
			}
		}
	}
	if cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", req.PID)); err == nil {
		result.CWD = cwd
	}
	if req.IncludeOpenFiles {
		fdDir := fmt.Sprintf("/proc/%d/fd", req.PID)
		if entries, err := os.ReadDir(fdDir); err == nil {
			for _, entry := range entries {
				if len(result.OpenConfigFiles) >= req.MaxFiles {
					break
				}
				target, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
				if err != nil || !filepath.IsAbs(target) || !hasAllowedSuffix(target, req.FileSuffixAllowlist) {
					continue
				}
				if validatePath(target) == nil {
					result.OpenConfigFiles = append(result.OpenConfigFiles, target)
				}
			}
		}
	}
	return result, nil
}

func (c *Collector) PurgeCredentialCache(ctx context.Context, params map[string]interface{}) (*PurgeResult, error) {
	_ = ctx
	var req PurgeCredentialCacheRequest
	if err := mapToStruct(params, &req); err != nil {
		return nil, err
	}
	return &PurgeResult{TaskID: req.TaskID, Purged: true}, nil
}

func lookupOwner(info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	uid := strconv.FormatUint(uint64(stat.Uid), 10)
	if u, err := user.LookupId(uid); err == nil {
		return u.Username
	}
	return uid
}

func readPrefix(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, maxBytes)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return nil, err
	}
	return buf[:n], nil
}

func redactConfigLine(line string) string {
	key, _, ok := splitKV(line)
	if !ok {
		return line
	}
	lower := strings.ToLower(key)
	for _, marker := range []string{"password", "passwd", "secret", "token", "api_key", "apikey", "key"} {
		if strings.Contains(lower, marker) {
			return key + " = ******"
		}
	}
	return line
}

func parseUnitFile(service, path string, content []byte) *ServiceUnitInspectResult {
	result := &ServiceUnitInspectResult{Service: service, UnitPath: path}
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := splitKV(line)
		if !ok {
			continue
		}
		switch key {
		case "ExecStart":
			result.ExecStart = append(result.ExecStart, value)
		case "EnvironmentFile":
			result.EnvironmentFiles = append(result.EnvironmentFiles, strings.TrimPrefix(value, "-"))
		case "WorkingDirectory":
			result.WorkingDirectory = value
		case "User":
			result.User = value
		}
	}
	return result
}
