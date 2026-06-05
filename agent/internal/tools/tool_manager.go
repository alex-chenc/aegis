package tools

import (
	"context"
	"fmt"
	"sort"
	"time"

	hostasset "aegis-agent/internal/asset"
	"aegis-agent/internal/assets"
	"go.uber.org/zap"
)

type ToolManager struct {
	allowedCommands []string
	versionTool     *assets.VersionTool
	logger          *zap.Logger
}

func NewToolManager() *ToolManager {
	logger, _ := zap.NewProduction()
	return &ToolManager{
		allowedCommands: []string{
			"ps", "ls", "cat", "netstat", "ss", "lsof",
			"whoami", "id", "groups", "find", "stat",
			"file", "strings", "md5sum", "sha256sum",
		},
		versionTool: assets.NewVersionTool(logger),
		logger:      logger,
	}
}

func (m *ToolManager) Execute(tool string, params map[string]interface{}) (interface{}, error) {
	switch tool {
	case "GetProcessTree":
		pid, err := toInt(params["pid"])
		if err != nil {
			return nil, err
		}
		return m.GetProcessTree(pid)
	case "GetNetworkConnections":
		pid, err := toInt(params["pid"])
		if err != nil {
			return nil, err
		}
		return m.GetNetworkConnections(pid)
	case "GetFileInfo":
		filePath, ok := params["file_path"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid file_path parameter")
		}
		return m.GetFileInfo(filePath)
	case "ReadFileContent":
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
	case "GetUserInfo":
		username, ok := params["username"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid username parameter")
		}
		return m.GetUserInfo(username)
	case "ExecuteCommand":
		command, ok := params["command"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid command parameter")
		}
		return m.ExecuteCommand(command)
	case "QueryHistoricalLogs":
		return m.QueryHistoricalLogs(params)
	case "GetOpenFiles":
		return m.GetOpenFiles(params)
	case "GetRunningProcesses":
		return m.GetRunningProcesses(params)
	case "GetUserSessions":
		return m.GetUserSessions(params)
	case "AssetGetProcessVersion":
		pid, err := toInt(params["pid"])
		if err != nil {
			return nil, err
		}
		exePath, _ := params["exe_path"].(string)
		hint, _ := params["hint"].(string)
		return m.versionTool.AssetGetProcessVersion(context.Background(), pid, exePath, hint), nil
	case "AssetReadConfigSummary":
		path, ok := params["path"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid path parameter")
		}
		maxSize := 64 * 1024
		if ms, ok := params["max_size"]; ok {
			if msInt, err := toInt(ms); err == nil {
				maxSize = msInt
			}
		}
		return m.versionTool.AssetReadConfigSummary(context.Background(), path, maxSize), nil
	case "AssetListDirectoryHints":
		path, ok := params["path"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid path parameter")
		}
		maxEntries := 50
		if me, ok := params["max_entries"]; ok {
			if meInt, err := toInt(me); err == nil {
				maxEntries = meInt
			}
		}
		return m.versionTool.AssetListDirectoryHints(context.Background(), path, maxEntries), nil
	case "AssetResolvePackageByFile":
		path, ok := params["path"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid path parameter")
		}
		return m.versionTool.AssetResolvePackageByFile(context.Background(), path), nil
	case "AssetReadProcFile":
		pid, err := toInt(params["pid"])
		if err != nil {
			return nil, err
		}
		fileName, ok := params["file_name"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid file_name parameter")
		}
		return m.versionTool.AssetReadProcFile(context.Background(), pid, fileName), nil
	case "AssetCollectHostAssets":
		hostID, _ := params["host_id"].(string)
		collectTypes := toStringSlice(params["collect_types"])
		includePackageFiles := toBool(params["include_package_files"])
		includeListenPorts := true
		if value, ok := params["include_listen_ports"]; ok {
			includeListenPorts = toBool(value)
		}
		maxProcessCount := 2000
		if value, ok := params["max_process_count"]; ok {
			if parsed, err := toInt(value); err == nil && parsed > 0 {
				maxProcessCount = parsed
			}
		}
		return m.collectHostAssets(hostID, collectTypes, includePackageFiles, includeListenPorts, maxProcessCount)
	case "AssetCollectProcessSnapshot":
		hostID, _ := params["host_id"].(string)
		offset := 0
		if value, ok := params["offset"]; ok {
			if parsed, err := toInt(value); err == nil && parsed >= 0 {
				offset = parsed
			}
		}
		limit := 100
		if value, ok := params["limit"]; ok {
			if parsed, err := toInt(value); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		includeListenPorts := true
		if value, ok := params["include_listen_ports"]; ok {
			includeListenPorts = toBool(value)
		}
		maxProcessCount := 2000
		if value, ok := params["max_process_count"]; ok {
			if parsed, err := toInt(value); err == nil && parsed > 0 {
				maxProcessCount = parsed
			}
		}
		return m.collectProcessSnapshot(hostID, offset, limit, includeListenPorts, maxProcessCount)
	default:
		return nil, fmt.Errorf("unknown tool: %s", tool)
	}
}

func (m *ToolManager) collectHostAssets(hostID string, collectTypes []string, includePackageFiles, includeListenPorts bool, maxProcessCount int) (*assets.HostAssetSnapshot, error) {
	info, err := hostasset.Collect()
	if err != nil {
		return nil, fmt.Errorf("failed to collect host info: %w", err)
	}

	m.logger.Info("Collecting host assets via whitelisted tool",
		zap.String("host_id", hostID),
		zap.Strings("collect_types", collectTypes),
		zap.Bool("include_package_files", includePackageFiles),
		zap.Bool("include_listen_ports", includeListenPorts),
		zap.Int("max_process_count", maxProcessCount))

	collector := assets.NewAssetCollector(m.logger)
	return collector.Collect(context.Background(), hostID, info.Hostname, info.IPAddress, info.OSType, info.OSVersion, info.Arch, assets.CollectOptions{
		IncludePackageFiles: includePackageFiles,
		IncludeListenPorts:  includeListenPorts,
		MaxProcessCount:     maxProcessCount,
	})
}

func (m *ToolManager) collectProcessSnapshot(hostID string, offset, limit int, includeListenPorts bool, maxProcessCount int) (*assets.ProcessSnapshotChunk, error) {
	info, err := hostasset.Collect()
	if err != nil {
		return nil, fmt.Errorf("failed to collect host info: %w", err)
	}

	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}

	m.logger.Info("Collecting process snapshot chunk via whitelisted tool",
		zap.String("host_id", hostID),
		zap.Int("offset", offset),
		zap.Int("limit", limit),
		zap.Bool("include_listen_ports", includeListenPorts),
		zap.Int("max_process_count", maxProcessCount))

	collector := assets.NewProcessCollector(m.logger, maxProcessCount)
	processes, processTotal, hasMore, err := collector.CollectPage(context.Background(), includeListenPorts, offset, limit)
	if err != nil {
		return nil, err
	}
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].PID < processes[j].PID
	})

	return &assets.ProcessSnapshotChunk{
		HostID:        hostID,
		Hostname:      info.Hostname,
		IPAddress:     info.IPAddress,
		OSType:        info.OSType,
		OSVersion:     info.OSVersion,
		Arch:          info.Arch,
		ProcessOffset: offset,
		ProcessLimit:  limit,
		ProcessTotal:  processTotal,
		HasMore:       hasMore,
		Processes:     processes,
		CollectedAt:   time.Now(),
	}, nil
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

func toBool(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1" || v == "yes"
	default:
		return false
	}
}

func toStringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}
