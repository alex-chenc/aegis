package weakpass

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Collector struct {
	logger *zap.Logger
}

func NewCollector(logger *zap.Logger) *Collector {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Collector{logger: logger}
}

func (c *Collector) CollectCredentials(ctx context.Context, params map[string]interface{}) (*CredentialCollectionResult, error) {
	_ = ctx
	if err := validateToolArgsNoShell(params); err != nil {
		return nil, err
	}

	var req CredentialCollectionRequest
	if err := mapToStruct(params, &req); err != nil {
		return nil, err
	}
	req.CollectionPolicy = normalizePolicy(req.CollectionPolicy)
	if req.TaskID == "" || req.PlanID == "" || req.HostID == "" {
		return nil, fmt.Errorf("%s: task_id, plan_id and host_id are required", ErrInvalidRequest)
	}
	if err := validateCollectionPolicy(req.CollectionPolicy); err != nil {
		return nil, fmt.Errorf("%s: %w", ErrInvalidPolicy, err)
	}

	result := &CredentialCollectionResult{
		TaskID:  req.TaskID,
		PlanID:  req.PlanID,
		HostID:  req.HostID,
		Records: []CredentialRecord{},
		Errors:  []CredentialCollectionError{},
	}

	for _, app := range req.Applications {
		if len(app.Extractors) == 0 {
			app.Extractors = defaultExtractors(app.Application)
		}
		appRecordStart := len(result.Records)
		if len(result.Records) < req.CollectionPolicy.MaxRecords {
			remaining := req.CollectionPolicy.MaxRecords - len(result.Records)
			result.Records = append(result.Records, collectContainerEnvironmentCredentialRecords(app, remaining)...)
		}
		if len(result.Records) > appRecordStart {
			continue
		}
		for _, path := range app.Paths {
			if len(result.Records) >= req.CollectionPolicy.MaxRecords {
				result.Errors = append(result.Errors, collectionError(app.Application, path, ErrRecordLimitReached, "record limit reached", false))
				return result, nil
			}
			if err := validatePath(path); err != nil {
				result.Errors = append(result.Errors, collectionError(app.Application, path, ErrInvalidPath, err.Error(), false))
				continue
			}

			content, resolved, statErr := readAllowedCredentialFile(path, app.RelatedPIDs, app.IsContainer, req.CollectionPolicy.MaxFileBytes)
			if statErr != nil {
				code, retryable := fileErrorCode(statErr)
				result.Errors = append(result.Errors, collectionError(app.Application, resolved.SourcePath, code, safeFileErrorMessage(code), retryable))
				continue
			}

			for _, extractor := range app.Extractors {
				if len(result.Records) >= req.CollectionPolicy.MaxRecords {
					result.Errors = append(result.Errors, collectionError(app.Application, path, ErrRecordLimitReached, "record limit reached", false))
					return result, nil
				}
				records, parseErr := parseCredentialFile(app, resolved.SourcePath, content, extractor)
				if parseErr != nil {
					code := ErrUnsupportedFormat
					if errors.Is(parseErr, errFieldNotFound) {
						code = ErrFieldNotFound
					}
					result.Errors = append(result.Errors, collectionError(app.Application, resolved.SourcePath, code, parseErr.Error(), true))
					continue
				}
				for idx := range records {
					records[idx].ProcessPID = resolved.ProcessPID
				}
				result.Records = append(result.Records, records...)
			}
		}
		if len(result.Records) == appRecordStart && len(result.Records) < req.CollectionPolicy.MaxRecords {
			remaining := req.CollectionPolicy.MaxRecords - len(result.Records)
			result.Records = append(result.Records, collectProcessCredentialRecords(app, remaining)...)
		}
	}

	c.logger.Info("weak password credential collection completed",
		zap.String("task_id", req.TaskID),
		zap.String("plan_id", req.PlanID),
		zap.String("host_id", req.HostID),
		zap.Int("record_count", len(result.Records)),
		zap.Int("error_count", len(result.Errors)))

	return result, nil
}

func mapToStruct(params map[string]interface{}, out interface{}) error {
	data, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	return nil
}

func readAllowedFile(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory")
	}
	if info.Size() > maxBytes {
		return nil, errFileTooLarge
	}
	return os.ReadFile(path)
}

type resolvedCredentialPath struct {
	ReadPath   string
	SourcePath string
	ProcessPID int
}

func readAllowedCredentialFile(path string, relatedPIDs []int, containerOnly bool, maxBytes int64) ([]byte, resolvedCredentialPath, error) {
	candidates := credentialPathCandidates(path, relatedPIDs, containerOnly)
	var lastErr error
	lastCandidate := resolvedCredentialPath{ReadPath: path, SourcePath: path}
	for _, candidate := range candidates {
		content, err := readAllowedFile(candidate.ReadPath, maxBytes)
		if err == nil {
			return content, candidate, nil
		}
		lastErr = err
		lastCandidate = candidate
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return nil, lastCandidate, lastErr
}

func credentialPathCandidates(path string, relatedPIDs []int, containerOnly bool) []resolvedCredentialPath {
	return credentialPathCandidatesWithResolver(path, relatedPIDs, containerOnly, detectContainerRootForPID)
}

func credentialPathCandidatesWithResolver(path string, relatedPIDs []int, containerOnly bool, rootForPID func(int) (string, bool)) []resolvedCredentialPath {
	candidates := []resolvedCredentialPath{}
	clean := filepath.Clean(path)
	relative := strings.TrimPrefix(clean, string(filepath.Separator))
	seen := map[string]struct{}{}
	for _, pid := range relatedPIDs {
		if pid <= 0 {
			continue
		}
		root, ok := rootForPID(pid)
		if !ok {
			continue
		}
		readPath := filepath.Join(root, relative)
		if _, ok := seen[readPath]; ok {
			continue
		}
		seen[readPath] = struct{}{}
		candidates = append(candidates, resolvedCredentialPath{
			ReadPath:   readPath,
			SourcePath: readPath,
			ProcessPID: pid,
		})
	}
	if !containerOnly {
		if _, ok := seen[path]; ok {
			return candidates
		}
		candidates = append(candidates, resolvedCredentialPath{ReadPath: path, SourcePath: path})
	}
	return candidates
}

func collectProcessCredentialRecords(app ApplicationCollectPlan, maxRecords int) []CredentialRecord {
	if maxRecords <= 0 || !isRedisApplication(app.Application) {
		return nil
	}
	var records []CredentialRecord
	for _, pid := range uniquePositivePIDs(app.RelatedPIDs) {
		if len(records) >= maxRecords {
			break
		}
		pidRecordStart := len(records)
		if parts, err := readProcessCmdlineParts(pid); err == nil {
			for _, record := range redisCredentialRecordsFromCmdline(app, pid, parts) {
				records = append(records, record)
				if len(records) >= maxRecords {
					break
				}
			}
		}
		if len(records) == pidRecordStart && len(records) < maxRecords {
			for _, record := range redisCredentialRecordsFromDockerConfig(app, pid) {
				records = append(records, record)
				if len(records) >= maxRecords {
					break
				}
			}
		}
	}
	runtime := strings.TrimSpace(app.ContainerRuntime)
	if len(records) == 0 && (runtime == "" || strings.EqualFold(runtime, "docker")) && strings.TrimSpace(app.ContainerID) != "" {
		env, sourcePath, err := readDockerContainerEnvForID(app.ContainerID)
		if err == nil && len(env) > 0 {
			for _, record := range credentialRecordsFromEnv(app, 0, env, sourcePath) {
				records = append(records, record)
				if len(records) >= maxRecords {
					break
				}
			}
		}
	}
	return records
}

func collectContainerEnvironmentCredentialRecords(app ApplicationCollectPlan, maxRecords int) []CredentialRecord {
	if maxRecords <= 0 {
		return nil
	}
	var records []CredentialRecord
	for _, pid := range uniquePositivePIDs(app.RelatedPIDs) {
		if len(records) >= maxRecords {
			break
		}
		env, sourcePath, err := readDockerContainerEnvForPID(pid)
		if err != nil || len(env) == 0 {
			continue
		}
		for _, record := range credentialRecordsFromEnv(app, pid, env, sourcePath) {
			records = append(records, record)
			if len(records) >= maxRecords {
				break
			}
		}
	}
	return records
}

func redisCredentialRecordsFromCmdline(app ApplicationCollectPlan, pid int, parts []string) []CredentialRecord {
	return redisCredentialRecordsFromArgs(app, pid, parts, fmt.Sprintf("/proc/%d/cmdline", pid), "process_cmdline", "process_cmdline", "cmdline", 0.88)
}

func redisCredentialRecordsFromDockerConfig(app ApplicationCollectPlan, pid int) []CredentialRecord {
	parts, sourcePath, err := readDockerContainerCommandPartsForPID(pid)
	if err != nil || len(parts) == 0 {
		return nil
	}
	return redisCredentialRecordsFromArgs(app, pid, parts, sourcePath, "container_runtime_config", "docker_config_cmd", "docker_config", 0.9)
}

func redisCredentialRecordsFromArgs(app ApplicationCollectPlan, pid int, parts []string, sourcePath, sourceKind, parser, fieldPrefix string, confidence float64) []CredentialRecord {
	var records []CredentialRecord
	for idx := 0; idx < len(parts); idx++ {
		key, value, ok := redisCredentialArg(parts, idx)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		extractor := CredentialExtractor{
			Type:             parser,
			PasswordSelector: key,
			FormatHint:       CredentialTypePlaintext,
			SourceKind:       sourceKind,
		}
		record := newRecord(app, sourcePath, extractor, "", value, fieldPrefix+"."+key)
		record.Parser = parser
		record.ProcessPID = pid
		record.Confidence = confidence
		records = append(records, record)
		if !strings.Contains(parts[idx], "=") {
			idx++
		}
	}
	return records
}

func redisCredentialArg(parts []string, idx int) (string, string, bool) {
	part := strings.TrimSpace(parts[idx])
	lower := strings.ToLower(strings.TrimLeft(part, "-"))
	for _, key := range []string{"requirepass", "masterauth"} {
		if lower == key {
			if idx+1 >= len(parts) {
				return "", "", false
			}
			return key, parts[idx+1], true
		}
		if strings.HasPrefix(lower, key+"=") {
			return key, part[strings.Index(part, "=")+1:], true
		}
	}
	return "", "", false
}

func readProcessCmdlineParts(pid int) ([]string, error) {
	content, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return nil, err
	}
	var parts []string
	for _, part := range strings.Split(string(content), "\x00") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts, nil
}

const dockerContainerConfigMaxBytes int64 = 1024 * 1024

type dockerContainerRuntimeConfig struct {
	Path   string   `json:"Path"`
	Args   []string `json:"Args"`
	Config struct {
		Entrypoint []string `json:"Entrypoint"`
		Cmd        []string `json:"Cmd"`
		Env        []string `json:"Env"`
	} `json:"Config"`
}

func readDockerContainerCommandPartsForPID(pid int) ([]string, string, error) {
	configContent, configPath, err := readDockerContainerConfigForPID(pid)
	if err != nil {
		return nil, "", err
	}
	parts, err := parseDockerContainerCommandParts(configContent)
	if err != nil {
		return nil, "", err
	}
	return parts, configPath, nil
}

func readDockerContainerEnvForPID(pid int) ([]string, string, error) {
	configContent, configPath, err := readDockerContainerConfigForPID(pid)
	if err != nil {
		return nil, "", err
	}
	env, err := parseDockerContainerEnv(configContent)
	if err != nil {
		return nil, "", err
	}
	return env, configPath, nil
}

func readDockerContainerEnvForID(containerID string) ([]string, string, error) {
	configContent, configPath, err := readDockerContainerConfigForID(containerID)
	if err != nil {
		return nil, "", err
	}
	env, err := parseDockerContainerEnv(configContent)
	if err != nil {
		return nil, "", err
	}
	return env, configPath, nil
}

func readDockerContainerConfigForPID(pid int) ([]byte, string, error) {
	content, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return nil, "", err
	}
	identity := parseContainerIdentityFromCgroup(string(content))
	if identity.Runtime != "docker" || identity.ID == "" {
		return nil, "", os.ErrNotExist
	}
	configPath, ok := dockerContainerConfigPath(identity.ID)
	if !ok {
		return nil, "", os.ErrNotExist
	}
	configContent, err := readAllowedFile(configPath, dockerContainerConfigMaxBytes)
	if err != nil {
		return nil, "", err
	}
	return configContent, configPath, nil
}

func readDockerContainerConfigForID(containerID string) ([]byte, string, error) {
	configPath, ok := dockerContainerConfigPath(containerID)
	if !ok {
		return nil, "", os.ErrNotExist
	}
	configContent, err := readAllowedFile(configPath, dockerContainerConfigMaxBytes)
	if err != nil {
		return nil, "", err
	}
	return configContent, configPath, nil
}

func parseDockerContainerCommandParts(content []byte) ([]string, error) {
	var cfg dockerContainerRuntimeConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.Args) > 0 {
		return compactCommandParts(cfg.Args), nil
	}
	parts := make([]string, 0, len(cfg.Config.Entrypoint)+len(cfg.Config.Cmd))
	parts = append(parts, cfg.Config.Entrypoint...)
	parts = append(parts, cfg.Config.Cmd...)
	return compactCommandParts(parts), nil
}

func parseDockerContainerEnv(content []byte) ([]string, error) {
	var cfg dockerContainerRuntimeConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	return compactCommandParts(cfg.Config.Env), nil
}

func credentialRecordsFromEnv(app ApplicationCollectPlan, pid int, env []string, sourcePath string) []CredentialRecord {
	var records []CredentialRecord
	for _, item := range env {
		key, value, ok := splitEnvAssignment(item)
		if !ok || strings.TrimSpace(value) == "" || !isCredentialEnvKey(key) {
			continue
		}
		extractor := CredentialExtractor{
			Type:             "docker_config_env",
			PasswordSelector: key,
			FormatHint:       CredentialTypePlaintext,
			SourceKind:       "container_env",
		}
		record := newRecord(app, sourcePath, extractor, "", value, "Env."+key)
		record.Parser = "docker_config_env"
		record.ProcessPID = pid
		record.Confidence = 0.86
		records = append(records, record)
	}
	return records
}

func splitEnvAssignment(value string) (string, string, bool) {
	key, val, ok := strings.Cut(value, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return "", "", false
	}
	return key, val, true
}

func isCredentialEnvKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{"password", "passwd", "pass", "pwd", "secret", "token", "auth", "credential"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	switch normalized {
	case "rediscli_auth", "requirepass", "masterauth":
		return true
	}
	return false
}

func compactCommandParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func dockerContainerConfigPath(id string) (string, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	if !isHexContainerID(id) {
		return "", false
	}
	if len(id) == 64 {
		return filepath.Join("/var/lib/docker/containers", id, "config.v2.json"), true
	}
	entries, err := os.ReadDir("/var/lib/docker/containers")
	if err != nil {
		return "", false
	}
	var match string
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if !entry.IsDir() || len(name) != 64 || !strings.HasPrefix(name, id) || !isHexContainerID(name) {
			continue
		}
		if match != "" {
			return "", false
		}
		match = name
	}
	if match == "" {
		return "", false
	}
	return filepath.Join("/var/lib/docker/containers", match, "config.v2.json"), true
}

func isHexContainerID(id string) bool {
	if len(id) < 12 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func isRedisApplication(application string) bool {
	lower := strings.ToLower(strings.TrimSpace(application))
	return lower == "redis" || lower == "redis-server" || strings.Contains(lower, "redis")
}

func uniquePositivePIDs(values []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

var errFileTooLarge = errors.New("file too large")

func fileErrorCode(err error) (string, bool) {
	if errors.Is(err, os.ErrNotExist) {
		return ErrFileNotFound, true
	}
	if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) {
		return ErrPermissionDenied, true
	}
	if errors.Is(err, errFileTooLarge) {
		return ErrFileTooLarge, false
	}
	return ErrUnsupportedFormat, true
}

func safeFileErrorMessage(code string) string {
	switch code {
	case ErrFileNotFound:
		return "configured file does not exist"
	case ErrPermissionDenied:
		return "permission denied when reading config file"
	case ErrFileTooLarge:
		return "configured file exceeds max_file_bytes"
	default:
		return "failed to read configured file"
	}
}

func newRecord(app ApplicationCollectPlan, path string, extractor CredentialExtractor, account, value, fieldPath string) CredentialRecord {
	credType, salt, algorithm := classifyCredential(value, extractor.FormatHint)
	sourceKind := extractor.SourceKind
	if sourceKind == "" {
		sourceKind = "config_file"
	}
	return CredentialRecord{
		RecordID:        uuid.NewString(),
		Application:     app.Application,
		AssetID:         app.AssetID,
		SourcePath:      path,
		SourceKind:      sourceKind,
		Account:         account,
		CredentialType:  credType,
		CredentialValue: value,
		Salt:            salt,
		AlgorithmHint:   algorithm,
		FieldPath:       fieldPath,
		Parser:          extractor.Type,
		Confidence:      0.94,
	}
}
