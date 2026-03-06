package asset

import (
	"net"
	"os"
	"runtime"
)

// AssetInfo 主机资产信息
type AssetInfo struct {
	IPAddress    string `json:"ip_address"`
	Hostname     string `json:"hostname"`
	OSType       string `json:"os_type"`
	OSVersion    string `json:"os_version"`
	Arch         string `json:"arch"`
	AgentVersion string `json:"agent_version"`
}

const AgentVersion = "v2.2.0"

// Collect 采集主机资产信息
func Collect() (*AssetInfo, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	ipAddress := getOutboundIP()

	return &AssetInfo{
		IPAddress:    ipAddress,
		Hostname:     hostname,
		OSType:       runtime.GOOS,
		OSVersion:    runtime.GOOS,
		Arch:         runtime.GOARCH,
		AgentVersion: AgentVersion,
	}, nil
}

// getOutboundIP 获取出站 IP 地址
func getOutboundIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback == 0 {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
					return ipNet.IP.String()
				}
			}
		}
	}

	return "127.0.0.1"
}
