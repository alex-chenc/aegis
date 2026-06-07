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

// parseUUID parses a UUID string from args, returns error if missing or invalid.
func parseUUID(args map[string]interface{}, key string) (uuid.UUID, error) {
	s := getStringArg(args, key, "")
	if s == "" {
		return uuid.Nil, fmt.Errorf("%s is required", key)
	}
	return uuid.Parse(s)
}
