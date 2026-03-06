package asset

import (
	"net"
	"os"
	"runtime"
)

type AssetInfo struct {
	IPAddress    string `json:"ip_address"`
	Hostname     string `json:"hostname"`
	OSType       string `json:"os_type"`
	OSVersion    string `json:"os_version"`
	Arch         string `json:"arch"`
	AgentVersion string `json:"agent_version"`
}

const AgentVersion = "v2.2.0"

func Collect() (*AssetInfo, error) {
	hostname, _ := os.Hostname()

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

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
