package assets

import "time"

// HostAssetSnapshot 主机资产快照
type HostAssetSnapshot struct {
	HostID      string         `json:"host_id"`
	Hostname    string         `json:"hostname"`
	IPAddress   string         `json:"ip_address"`
	OSType      string         `json:"os_type"`
	OSVersion   string         `json:"os_version"`
	Arch        string         `json:"arch"`
	Packages    []PackageAsset `json:"packages"`
	Processes   []ProcessAsset `json:"processes"`
	CollectedAt time.Time      `json:"collected_at"`
	Errors      []CollectError `json:"errors"`
}

// ProcessSnapshotChunk 进程快照分片
type ProcessSnapshotChunk struct {
	HostID        string         `json:"host_id"`
	Hostname      string         `json:"hostname"`
	IPAddress     string         `json:"ip_address"`
	OSType        string         `json:"os_type"`
	OSVersion     string         `json:"os_version"`
	Arch          string         `json:"arch"`
	ProcessOffset int            `json:"process_offset"`
	ProcessLimit  int            `json:"process_limit"`
	ProcessTotal  int            `json:"process_total"`
	HasMore       bool           `json:"has_more"`
	Processes     []ProcessAsset `json:"processes"`
	CollectedAt   time.Time      `json:"collected_at"`
}

// PackageAsset 软件包资产
type PackageAsset struct {
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	Release        string            `json:"release,omitempty"`
	Epoch          string            `json:"epoch,omitempty"`
	Architecture   string            `json:"architecture"`
	PackageManager string            `json:"package_manager"`
	SourceName     string            `json:"source_name,omitempty"`
	Vendor         string            `json:"vendor,omitempty"`
	License        string            `json:"license,omitempty"`
	InstallTime    time.Time         `json:"install_time,omitempty"`
	InstallPaths   []string          `json:"install_paths"`
	FileCount      int               `json:"file_count"`
	Metadata       map[string]string `json:"metadata"`
}

// ProcessAsset 进程资产
type ProcessAsset struct {
	PID         int       `json:"pid"`
	PPID        int       `json:"ppid"`
	Comm        string    `json:"comm"`
	Cmdline     string    `json:"cmdline"`
	ExePath     string    `json:"exe_path"`
	Cwd         string    `json:"cwd"`
	UID         int       `json:"uid"`
	Username    string    `json:"username"`
	ListenPorts []int     `json:"listen_ports"`
	StartTime   time.Time `json:"start_time,omitempty"`
	ContainerID string    `json:"container_id,omitempty"`
	PackageName string    `json:"package_name,omitempty"`
}

// CollectError 采集错误
type CollectError struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

// CollectOptions 采集选项
type CollectOptions struct {
	IncludePackageFiles bool `json:"include_package_files"`
	IncludeListenPorts  bool `json:"include_listen_ports"`
	MaxProcessCount     int  `json:"max_process_count"`
}
