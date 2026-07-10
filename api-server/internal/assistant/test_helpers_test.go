package assistant

import "context"

func noopToolHandler(context.Context, map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"ok": true}, nil
}
