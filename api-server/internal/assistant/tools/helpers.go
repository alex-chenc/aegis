package tools

import (
	"fmt"

	"github.com/google/uuid"
)

// getStringArg extracts a string argument from the args map with a default value.
func getStringArg(args map[string]interface{}, key, defaultVal string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

// getIntArg extracts an integer argument from the args map with a default value.
// Handles both int and float64 (JSON numbers decode as float64).
func getIntArg(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

// getBoolArg extracts a boolean argument from the args map with a default value.
func getBoolArg(args map[string]interface{}, key string, defaultVal bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultVal
}

// parseUUID parses a UUID string from args, returns error if missing or invalid.
func parseUUID(args map[string]interface{}, key string) (uuid.UUID, error) {
	s := getStringArg(args, key, "")
	if s == "" {
		return uuid.Nil, fmt.Errorf("%s is required", key)
	}
	return uuid.Parse(s)
}

func buildTaskRef(kind, id, taskGroupID, statusURL, routePath string) map[string]interface{} {
	ref := map[string]interface{}{
		"kind":       kind,
		"id":         id,
		"status_url": statusURL,
		"route_path": routePath,
	}
	if taskGroupID != "" {
		ref["task_group_id"] = taskGroupID
	}
	return ref
}
