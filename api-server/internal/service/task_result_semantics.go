package service

import "strings"

func NormalizeTaskResultStatus(taskType, status string, exitCode int, stderr string) string {
	normalizedStatus := strings.ToUpper(strings.TrimSpace(status))
	normalizedType := strings.ToUpper(strings.TrimSpace(taskType))

	switch normalizedStatus {
	case "PENDING", "RUNNING", "TIMEOUT", "AUDIT_BLOCKED":
		return normalizedStatus
	}

	if normalizedType == "CHECK" || normalizedType == "POC_VERIFY" {
		if isCheckExecutionError(exitCode, stderr) {
			return "FAILED"
		}
		return "SUCCESS"
	}

	if (normalizedType == "FIX" || normalizedType == "VULNERABILITY_FIX") && exitCode != 0 {
		return "FAILED"
	}

	if normalizedStatus == "" {
		return "SUCCESS"
	}
	return normalizedStatus
}

func IsLLMRepairableTask(taskType, status string, exitCode *int, stderr string) bool {
	normalizedStatus := strings.ToUpper(strings.TrimSpace(status))
	normalizedType := strings.ToUpper(strings.TrimSpace(taskType))

	switch normalizedStatus {
	case "TIMEOUT", "AUDIT_BLOCKED":
		return true
	case "FAILED":
		// CHECK exit_code=1 means the baseline item is not compliant. It is a
		// valid detection result, not a broken task that needs script repair.
		if normalizedType == "CHECK" && exitCode != nil && *exitCode == 1 && !looksLikeExecutionError(stderr) {
			return false
		}
		// POC exit_code=1 means the script ran successfully and confirmed the
		// vulnerability. It is a business result, not a broken script.
		if normalizedType == "POC_VERIFY" && exitCode != nil && *exitCode == 1 && !looksLikeExecutionError(stderr) {
			return false
		}
		return true
	default:
		return false
	}
}

func IsTerminalTaskStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS", "FAILED", "TIMEOUT", "AUDIT_BLOCKED":
		return true
	default:
		return false
	}
}

func IsTaskExecutionSuccessful(taskType, status string, exitCode *int, stderr string) bool {
	if strings.ToUpper(strings.TrimSpace(status)) != "SUCCESS" {
		return false
	}

	normalizedType := strings.ToUpper(strings.TrimSpace(taskType))
	if normalizedType == "CHECK" || normalizedType == "POC_VERIFY" {
		if exitCode == nil {
			return true
		}
		return !isCheckExecutionError(*exitCode, stderr)
	}

	if exitCode == nil {
		return true
	}
	return *exitCode == 0
}

func isCheckExecutionError(exitCode int, stderr string) bool {
	if exitCode < 0 || exitCode >= 2 {
		return true
	}
	return looksLikeExecutionError(stderr)
}

func looksLikeExecutionError(stderr string) bool {
	lower := strings.ToLower(stderr)
	if strings.TrimSpace(lower) == "" {
		return false
	}

	patterns := []string{
		"failed to start",
		"failed to write script",
		"failed to create temp dir",
		"[timeout]",
		"syntax error",
		"command not found",
		"permission denied",
		"no such file or directory",
		"unexpected end of file",
	}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
