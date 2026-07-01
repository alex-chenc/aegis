package weakpass

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	defaultMaxFileBytes = int64(1024 * 1024)
	defaultMaxRecords   = 500
	maxConfigSliceBytes = int64(16 * 1024)
)

var defaultAuxiliaryTools = []string{
	"WeakPassword.ProbePath",
	"WeakPassword.ListConfigDir",
	"WeakPassword.ReadConfigSlice",
	"WeakPassword.ServiceUnitInspect",
	"WeakPassword.ProcessConfigHints",
}

func normalizePolicy(policy CollectionPolicy) CollectionPolicy {
	if policy.MaxFileBytes <= 0 || policy.MaxFileBytes > defaultMaxFileBytes {
		policy.MaxFileBytes = defaultMaxFileBytes
	}
	if policy.MaxRecords <= 0 || policy.MaxRecords > defaultMaxRecords {
		policy.MaxRecords = defaultMaxRecords
	}
	return policy
}

func validateCollectionPolicy(policy CollectionPolicy) error {
	if !policy.ForbidFindCommand {
		return fmt.Errorf("forbid_find_command must be true")
	}
	if !policy.ForbidRecursiveSearch {
		return fmt.Errorf("forbid_recursive_search must be true")
	}
	return nil
}

func validatePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute")
	}
	clean := filepath.Clean(path)
	if clean != path {
		return fmt.Errorf("path must be clean")
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal is not allowed")
	}
	for _, token := range []string{";", "|", "&", "`", "$(", "\n", "\r"} {
		if strings.Contains(path, token) {
			return fmt.Errorf("shell metacharacter %q is not allowed", token)
		}
	}
	if strings.ContainsAny(path, "*?[]") {
		return fmt.Errorf("wildcard path is not allowed")
	}
	return nil
}

func validateToolArgsNoShell(params map[string]interface{}) error {
	for key, value := range params {
		if err := validateValueNoShell(key, value); err != nil {
			return err
		}
	}
	return nil
}

func validateValueNoShell(key string, value interface{}) error {
	switch v := value.(type) {
	case string:
		lower := strings.ToLower(v)
		if strings.Contains(lower, "find ") || strings.Contains(lower, "locate ") || strings.Contains(lower, "grep -r") || strings.Contains(lower, "grep -R") {
			return fmt.Errorf("%s contains forbidden command text", key)
		}
		for _, token := range []string{";", "|", "`", "$("} {
			if strings.Contains(v, token) {
				return fmt.Errorf("%s contains shell metacharacter", key)
			}
		}
	case []interface{}:
		for i, item := range v {
			if err := validateValueNoShell(fmt.Sprintf("%s[%d]", key, i), item); err != nil {
				return err
			}
		}
	case []string:
		for i, item := range v {
			if err := validateValueNoShell(fmt.Sprintf("%s[%d]", key, i), item); err != nil {
				return err
			}
		}
	case []map[string]interface{}:
		for i, item := range v {
			if err := validateValueNoShell(fmt.Sprintf("%s[%d]", key, i), item); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		for childKey, childValue := range v {
			if err := validateValueNoShell(key+"."+childKey, childValue); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasAllowedSuffix(path string, suffixes []string) bool {
	if len(suffixes) == 0 {
		return true
	}
	lower := strings.ToLower(path)
	for _, suffix := range suffixes {
		if suffix != "" && strings.HasSuffix(lower, strings.ToLower(suffix)) {
			return true
		}
	}
	return false
}

func collectionError(app, sourcePath, code, message string, retryable bool) CredentialCollectionError {
	return CredentialCollectionError{
		Application:             app,
		SourcePath:              sourcePath,
		ErrorCode:               code,
		Message:                 message,
		Retryable:               retryable,
		SuggestedAuxiliaryTools: defaultAuxiliaryTools,
	}
}
