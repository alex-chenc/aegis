package weakpass

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
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
	req.FileSuffixAllowlist = normalizeConfigSuffixAllowlist(req.FileSuffixAllowlist)
	result := &ProcessConfigHintsResult{PID: req.PID}
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", req.PID)
	var rawCmdline []string
	if content, err := os.ReadFile(cmdlinePath); err == nil {
		for _, part := range strings.Split(string(content), "\x00") {
			if part != "" {
				rawCmdline = append(rawCmdline, part)
				result.Cmdline = append(result.Cmdline, redactCmdlinePart(part))
			}
		}
	}
	if cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", req.PID)); err == nil {
		result.CWD = cwd
	}
	if content, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", req.PID)); err == nil {
		result.Cgroup = nonEmptyLines(content)
		if identity := parseContainerIdentityFromCgroup(string(content)); identity.ID != "" {
			result.ContainerID = identity.ID
			result.ContainerRuntime = identity.Runtime
			result.ContainerRoot = fmt.Sprintf("/proc/%d/root", req.PID)
		}
	}
	if result.ContainerRoot == "" {
		procRoot := fmt.Sprintf("/proc/%d/root", req.PID)
		if _, err := os.Stat(filepath.Join(procRoot, ".dockerenv")); err == nil {
			result.ContainerRuntime = "docker"
			result.ContainerRoot = procRoot
		}
	}

	cmdlineCandidates := configPathsFromCmdline(rawCmdline, result.CWD, req.FileSuffixAllowlist)
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
	if result.ContainerRoot != "" {
		result.ContainerConfigFiles = discoverContainerConfigPaths(result.ContainerRoot, req.Application, req.FileSuffixAllowlist, req.MaxFiles, cmdlineCandidates, result.OpenConfigFiles)
		result.ConfigPathCandidates = uniqueConfigPaths(append(result.ConfigPathCandidates, result.ContainerConfigFiles...), req.MaxFiles)
	} else {
		result.ConfigPathCandidates = uniqueConfigPaths(append(cmdlineCandidates, result.OpenConfigFiles...), req.MaxFiles)
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

type containerIdentity struct {
	ID      string
	Runtime string
}

var cgroupContainerIDPatterns = []struct {
	runtime string
	re      *regexp.Regexp
}{
	{"containerd", regexp.MustCompile(`(?:^|[/:\-])cri-containerd[-/]([0-9a-f]{12,64})(?:\.scope|/|$|\s)`)},
	{"containerd", regexp.MustCompile(`(?:^|[/:\-])containerd[-/]([0-9a-f]{12,64})(?:\.scope|/|$|\s)`)},
	{"cri-o", regexp.MustCompile(`(?:^|[/:\-])crio[-/]([0-9a-f]{12,64})(?:\.scope|/|$|\s)`)},
	{"cri-o", regexp.MustCompile(`(?:^|[/:\-])cri-o[-/]([0-9a-f]{12,64})(?:\.scope|/|$|\s)`)},
	{"podman", regexp.MustCompile(`(?:^|[/:\-])libpod[-/]([0-9a-f]{12,64})(?:\.scope|/|$|\s)`)},
	{"podman", regexp.MustCompile(`(?:^|[/:\-])podman[-/]([0-9a-f]{12,64})(?:\.scope|/|$|\s)`)},
	{"docker", regexp.MustCompile(`(?:^|[/:\-])docker[-/]([0-9a-f]{12,64})(?:\.scope|/|$|\s)`)},
}

var genericFullContainerIDPattern = regexp.MustCompile(`(?:^|[/:\-])([0-9a-f]{64})(?:\.scope|/|$|\s)`)

func parseContainerIdentityFromCgroup(content string) containerIdentity {
	lower := strings.ToLower(content)
	for _, pattern := range cgroupContainerIDPatterns {
		if matches := pattern.re.FindStringSubmatch(lower); len(matches) == 2 {
			return containerIdentity{ID: matches[1], Runtime: pattern.runtime}
		}
	}
	if matches := genericFullContainerIDPattern.FindStringSubmatch(lower); len(matches) == 2 {
		return containerIdentity{ID: matches[1], Runtime: "container"}
	}
	return containerIdentity{}
}

func nonEmptyLines(content []byte) []string {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func normalizeConfigSuffixAllowlist(suffixes []string) []string {
	if len(suffixes) == 0 {
		return []string{".conf", ".cnf", ".ini", ".yaml", ".yml", ".json", ".properties", ".toml", ".env", ".xml", ".db", ".passwd", ".htpasswd"}
	}
	out := make([]string, 0, len(suffixes))
	seen := map[string]struct{}{}
	for _, suffix := range suffixes {
		suffix = strings.ToLower(strings.TrimSpace(suffix))
		if suffix == "" {
			continue
		}
		if _, ok := seen[suffix]; ok {
			continue
		}
		seen[suffix] = struct{}{}
		out = append(out, suffix)
	}
	if len(out) == 0 {
		return normalizeConfigSuffixAllowlist(nil)
	}
	return out
}

func redactCmdlinePart(value string) string {
	lower := strings.ToLower(value)
	for _, key := range []string{"password", "passwd", "pwd", "token", "secret", "api_key", "apikey"} {
		if strings.Contains(lower, key+"=") || strings.Contains(lower, key+":") {
			if idx := strings.IndexAny(value, "=:"); idx >= 0 {
				return value[:idx+1] + "******"
			}
		}
	}
	return value
}

func configPathsFromCmdline(parts []string, cwd string, suffixes []string) []string {
	var candidates []string
	valueFlags := map[string]struct{}{
		"--config": {}, "--conf": {}, "--config-file": {}, "--config.path": {},
		"--defaults-file": {}, "--defaults-extra-file": {}, "--spring.config.location": {},
		"--spring.config.additional-location": {}, "-c": {},
	}
	for i := 0; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])
		if part == "" {
			continue
		}
		if _, ok := valueFlags[part]; ok && i+1 < len(parts) {
			candidates = append(candidates, splitConfigPathValue(parts[i+1])...)
			i++
			continue
		}
		if key, value, ok := strings.Cut(part, "="); ok {
			if _, allowed := valueFlags[key]; allowed {
				candidates = append(candidates, splitConfigPathValue(value)...)
				continue
			}
		}
		if hasAllowedSuffix(part, suffixes) {
			candidates = append(candidates, part)
		}
	}

	resolved := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = cleanConfigPathToken(candidate)
		if candidate == "" {
			continue
		}
		if !filepath.IsAbs(candidate) && cwd != "" {
			candidate = filepath.Join(cwd, candidate)
		}
		if validatePath(candidate) == nil && hasAllowedSuffix(candidate, suffixes) {
			resolved = append(resolved, candidate)
		}
	}
	return uniqueConfigPaths(resolved, 20)
}

func splitConfigPathValue(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var out []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' }) {
		part = cleanConfigPathToken(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func cleanConfigPathToken(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"'`))
	value = strings.TrimPrefix(value, "file://")
	value = strings.TrimPrefix(value, "file:")
	if idx := strings.Index(value, ":"); idx == 1 && len(value) > 2 {
		return ""
	}
	return filepath.Clean(value)
}

func discoverContainerConfigPaths(root, application string, suffixes []string, maxFiles int, cmdlineCandidates, openFiles []string) []string {
	if maxFiles <= 0 || maxFiles > 20 {
		maxFiles = 20
	}
	var candidates []string
	candidates = append(candidates, cmdlineCandidates...)
	candidates = append(candidates, openFiles...)
	candidates = append(candidates, defaultConfigPathsForApplication(application)...)
	candidates = append(candidates, listContainerConfigDirs(root, application, suffixes, maxFiles)...)

	var existing []string
	for _, sourcePath := range uniqueConfigPaths(candidates, maxFiles*3) {
		if len(existing) >= maxFiles {
			break
		}
		if !filepath.IsAbs(sourcePath) || validatePath(sourcePath) != nil || (!isDefaultCredentialPath(application, sourcePath) && !hasAllowedSuffix(sourcePath, suffixes)) {
			continue
		}
		readPath := processRootPath(root, sourcePath)
		info, err := os.Stat(readPath)
		if err != nil || info.IsDir() {
			continue
		}
		existing = append(existing, sourcePath)
	}
	return uniqueConfigPaths(existing, maxFiles)
}

func defaultConfigPathsForApplication(application string) []string {
	switch strings.ToLower(strings.TrimSpace(application)) {
	case "redis":
		return []string{
			"/etc/redis/redis.conf",
			"/etc/redis.conf",
			"/usr/local/etc/redis/redis.conf",
			"/data/redis.conf",
			"/redis.conf",
		}
	case "ssh", "sshd", "openssh", "linux_shadow":
		return []string{"/etc/shadow"}
	case "mysql", "mariadb":
		return []string{
			"/etc/mysql/my.cnf",
			"/etc/my.cnf",
			"/usr/local/etc/my.cnf",
			"/etc/mysql/mysql.conf.d/mysqld.cnf",
			"/etc/mysql/mariadb.conf.d/50-server.cnf",
			"/root/.my.cnf",
		}
	case "tomcat":
		return []string{
			"/usr/local/tomcat/conf/tomcat-users.xml",
			"/opt/tomcat/conf/tomcat-users.xml",
			"/etc/tomcat/tomcat-users.xml",
			"/etc/tomcat8/tomcat-users.xml",
			"/etc/tomcat9/tomcat-users.xml",
			"/etc/tomcat10/tomcat-users.xml",
		}
	case "ftp", "vsftpd", "proftpd":
		return []string{
			"/etc/vsftpd/virtual_users.db",
			"/etc/proftpd/passwd",
			"/etc/proftpd/ftppasswd",
			"/etc/proftpd/ftpd.passwd",
			"/etc/shadow",
		}
	case "nginx":
		return []string{
			"/etc/nginx/nginx.conf",
			"/usr/local/nginx/conf/nginx.conf",
			"/etc/nginx/.htpasswd",
		}
	case "apache", "httpd", "web_service":
		return []string{
			"/etc/apache2/apache2.conf",
			"/etc/httpd/conf/httpd.conf",
			"/etc/apache2/.htpasswd",
			"/etc/httpd/.htpasswd",
		}
	case "postgres", "postgresql":
		return []string{
			"/var/lib/postgresql/data/postgresql.conf",
			"/var/lib/postgresql/data/pg_hba.conf",
			"/etc/postgresql/postgresql.conf",
		}
	default:
		return nil
	}
}

func listContainerConfigDirs(root, application string, suffixes []string, maxFiles int) []string {
	var dirs []string
	switch strings.ToLower(strings.TrimSpace(application)) {
	case "redis":
		dirs = []string{"/etc/redis", "/usr/local/etc/redis", "/data"}
	case "ssh", "sshd", "openssh", "linux_shadow":
		dirs = []string{"/etc"}
	case "mysql", "mariadb":
		dirs = []string{"/etc/mysql", "/etc/mysql/mysql.conf.d", "/etc/mysql/mariadb.conf.d", "/usr/local/etc", "/root"}
	case "tomcat":
		dirs = []string{"/usr/local/tomcat/conf", "/opt/tomcat/conf", "/etc/tomcat", "/etc/tomcat8", "/etc/tomcat9", "/etc/tomcat10"}
	case "ftp", "vsftpd", "proftpd":
		dirs = []string{"/etc/vsftpd", "/etc/proftpd", "/etc"}
	case "nginx":
		dirs = []string{"/etc/nginx", "/usr/local/nginx/conf"}
	case "apache", "httpd", "web_service":
		dirs = []string{"/etc/apache2", "/etc/httpd/conf"}
	case "postgres", "postgresql":
		dirs = []string{"/var/lib/postgresql/data", "/etc/postgresql"}
	default:
		dirs = []string{"/etc", "/usr/local/etc", "/app/config", "/config"}
	}

	var candidates []string
	for _, dir := range dirs {
		if len(candidates) >= maxFiles {
			break
		}
		readDir := processRootPath(root, dir)
		entries, err := os.ReadDir(readDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if len(candidates) >= maxFiles {
				break
			}
			if entry.IsDir() {
				continue
			}
			sourcePath := filepath.Join(dir, entry.Name())
			if (isDefaultCredentialPath(application, sourcePath) || hasAllowedSuffix(sourcePath, suffixes)) && validatePath(sourcePath) == nil {
				candidates = append(candidates, sourcePath)
			}
		}
	}
	return candidates
}

func isDefaultCredentialPath(application, path string) bool {
	clean := filepath.Clean(path)
	for _, candidate := range defaultConfigPathsForApplication(application) {
		if clean == filepath.Clean(candidate) {
			return true
		}
	}
	return false
}

func processRootPath(root, sourcePath string) string {
	return filepath.Join(root, strings.TrimPrefix(filepath.Clean(sourcePath), string(filepath.Separator)))
}

func uniqueConfigPaths(paths []string, limit int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "." || path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	sort.Strings(out)
	return out
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
