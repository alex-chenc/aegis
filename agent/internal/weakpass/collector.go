package weakpass

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
		for _, path := range app.Paths {
			if len(result.Records) >= req.CollectionPolicy.MaxRecords {
				result.Errors = append(result.Errors, collectionError(app.Application, path, ErrRecordLimitReached, "record limit reached", false))
				return result, nil
			}
			if err := validatePath(path); err != nil {
				result.Errors = append(result.Errors, collectionError(app.Application, path, ErrInvalidPath, err.Error(), false))
				continue
			}

			content, resolved, statErr := readAllowedCredentialFile(path, app.RelatedPIDs, req.CollectionPolicy.MaxFileBytes)
			if statErr != nil {
				code, retryable := fileErrorCode(statErr)
				result.Errors = append(result.Errors, collectionError(app.Application, path, code, safeFileErrorMessage(code), retryable))
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
					result.Errors = append(result.Errors, collectionError(app.Application, path, code, parseErr.Error(), true))
					continue
				}
				for idx := range records {
					records[idx].ProcessPID = resolved.ProcessPID
				}
				result.Records = append(result.Records, records...)
			}
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

func readAllowedCredentialFile(path string, relatedPIDs []int, maxBytes int64) ([]byte, resolvedCredentialPath, error) {
	candidates := credentialPathCandidates(path, relatedPIDs)
	var lastErr error
	for _, candidate := range candidates {
		content, err := readAllowedFile(candidate.ReadPath, maxBytes)
		if err == nil {
			return content, candidate, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return nil, resolvedCredentialPath{ReadPath: path, SourcePath: path}, lastErr
}

func credentialPathCandidates(path string, relatedPIDs []int) []resolvedCredentialPath {
	candidates := []resolvedCredentialPath{}
	clean := filepath.Clean(path)
	relative := strings.TrimPrefix(clean, string(filepath.Separator))
	seen := map[string]struct{}{}
	for _, pid := range relatedPIDs {
		if pid <= 0 {
			continue
		}
		readPath := filepath.Join("/proc", strconv.Itoa(pid), "root", relative)
		if _, ok := seen[readPath]; ok {
			continue
		}
		seen[readPath] = struct{}{}
		candidates = append(candidates, resolvedCredentialPath{
			ReadPath:   readPath,
			SourcePath: path,
			ProcessPID: pid,
		})
	}
	if _, ok := seen[path]; !ok {
		candidates = append(candidates, resolvedCredentialPath{ReadPath: path, SourcePath: path})
	}
	return candidates
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
