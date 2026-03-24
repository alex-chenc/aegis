package tools

import (
	"fmt"
)

type ToolManager struct {
	allowedCommands []string
}

func NewToolManager() *ToolManager {
	return &ToolManager{
		allowedCommands: []string{
			"ps", "ls", "cat", "netstat", "ss", "lsof",
			"whoami", "id", "groups", "find", "stat",
			"file", "strings", "md5sum", "sha256sum",
		},
	}
}

func (m *ToolManager) Execute(tool string, params map[string]interface{}) (interface{}, error) {
	switch tool {
	case "get_process_tree":
		pid, err := toInt(params["pid"])
		if err != nil {
			return nil, err
		}
		return m.GetProcessTree(pid)
	case "get_network_connections":
		pid, err := toInt(params["pid"])
		if err != nil {
			return nil, err
		}
		return m.GetNetworkConnections(pid)
	case "get_file_info":
		filePath, ok := params["file_path"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid file_path parameter")
		}
		return m.GetFileInfo(filePath)
	case "read_file_content":
		filePath, ok := params["file_path"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid file_path parameter")
		}
		maxSize := int64(1024 * 1024)
		if ms, ok := params["max_size"]; ok {
			if msInt, err := toInt64(ms); err == nil {
				maxSize = msInt
			}
		}
		return m.ReadFileContent(filePath, maxSize)
	case "get_user_info":
		username, ok := params["username"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid username parameter")
		}
		return m.GetUserInfo(username)
	case "execute_command":
		command, ok := params["command"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid command parameter")
		}
		return m.ExecuteCommand(command)
	default:
		return nil, fmt.Errorf("unknown tool: %s", tool)
	}
}

func toInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case float64:
		return int64(v), nil
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	default:
		return 0, fmt.Errorf("invalid numeric parameter")
	}
}

func toInt(value interface{}) (int, error) {
	switch v := value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("invalid numeric parameter")
	}
}
