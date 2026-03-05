package ipdetect

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

var publicIPServices = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
	"https://api4.my-ip.io/ip",
}

func DetectServerIP(configuredIP string) string {
	if configuredIP != "" {
		return configuredIP
	}

	if ip := queryPublicIP(); ip != "" {
		return ip
	}

	if ip := getOutboundIP(); ip != "" {
		return ip
	}

	if ip := getLocalIP(); ip != "" {
		return ip
	}

	return "127.0.0.1"
}

func queryPublicIP() string {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	for _, serviceURL := range publicIPServices {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		req, _ := http.NewRequestWithContext(ctx, "GET", serviceURL, nil)
		resp, err := client.Do(req)
		cancel()
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}
		ip := strings.TrimSpace(string(body))
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	return ""
}

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func getLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			ipStr := ip.String()
			if strings.HasPrefix(ipStr, "172.17.") {
				continue
			}
			return ipStr
		}
	}
	return ""
}
