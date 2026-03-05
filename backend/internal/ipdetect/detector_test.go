package ipdetect

import (
	"net"
	"testing"
)

func TestDetectServerIP_WithConfiguredIP(t *testing.T) {
	configured := "192.168.1.100"
	result := DetectServerIP(configured)
	if result != configured {
		t.Errorf("Expected %s, got %s", configured, result)
	}
}

func TestDetectServerIP_EmptyConfig(t *testing.T) {
	result := DetectServerIP("")
	if result == "" {
		t.Error("Expected non-empty IP address")
	}
	ip := net.ParseIP(result)
	if ip == nil {
		t.Errorf("Invalid IP address: %s", result)
	}
}

func TestGetLocalIP(t *testing.T) {
	result := getLocalIP()
	if result == "" {
		t.Log("No local IP found (may be expected in some environments)")
		return
	}
	ip := net.ParseIP(result)
	if ip == nil {
		t.Errorf("Invalid IP address: %s", result)
	}
	if ip.IsLoopback() {
		t.Errorf("Should not return loopback address: %s", result)
	}
}

func TestGetOutboundIP(t *testing.T) {
	result := getOutboundIP()
	if result == "" {
		t.Log("No outbound IP found (network may be unavailable)")
		return
	}
	ip := net.ParseIP(result)
	if ip == nil {
		t.Errorf("Invalid IP address: %s", result)
	}
}
