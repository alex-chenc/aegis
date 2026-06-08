package tools

import (
	"testing"
	"time"

	"api-server/internal/model"
)

func TestNormalizeHostStatusFilterSupportsCommonModelArgs(t *testing.T) {
	args := map[string]interface{}{
		"filters": []interface{}{
			map[string]interface{}{
				"field":    "status",
				"operator": "eq",
				"value":    "online",
			},
		},
	}

	if got := normalizeHostStatusFilter(args); got != "online" {
		t.Fatalf("status filter = %q, want online", got)
	}

	args = map[string]interface{}{"agent_status": "离线"}
	if got := normalizeHostStatusFilter(args); got != "offline" {
		t.Fatalf("agent status filter = %q, want offline", got)
	}
}

func TestFilterHostsByAgentStatusUsesHeartbeatFreshness(t *testing.T) {
	hosts := []model.Host{{
		Hostname:        "online-host",
		LastHeartbeatAt: time.Now(),
	}, {
		Hostname:        "offline-host",
		LastHeartbeatAt: time.Now().Add(-10 * time.Minute),
	}}

	online := filterHostsByAgentStatus(hosts, "online")
	if len(online) != 1 || online[0].Hostname != "online-host" {
		t.Fatalf("online hosts = %#v", online)
	}

	offline := filterHostsByAgentStatus(hosts, "offline")
	if len(offline) != 1 || offline[0].Hostname != "offline-host" {
		t.Fatalf("offline hosts = %#v", offline)
	}
}
