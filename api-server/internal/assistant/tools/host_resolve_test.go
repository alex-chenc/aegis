package tools

import (
	"context"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveHostSelectorsHandlesCompactIPLabel(t *testing.T) {
	logger.Logger = zap.NewNop()
	db, err := gorm.Open(sqlite.Open("file:host_resolve?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := createSQLiteHostTable(db); err != nil {
		t.Fatal(err)
	}
	host := model.Host{
		ID:              uuid.New(),
		IPAddress:       "192.168.152.159",
		Hostname:        "baseline-target",
		OSType:          "linux",
		AgentVersion:    "6.1.0",
		LastHeartbeatAt: time.Now(),
	}
	if err := db.Create(&host).Error; err != nil {
		t.Fatal(err)
	}

	result, err := resolveHostSelectors(context.Background(), repository.NewHostRepository(db), nil, []string{"159IP"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resolved) != 1 || len(result.Unresolved) != 0 || len(result.Ambiguous) != 0 || len(result.Offline) != 0 {
		t.Fatalf("unexpected resolution: %#v", result)
	}
	if got := result.Resolved[0]["host_id"]; got != host.ID.String() {
		t.Fatalf("host_id = %v, want %s", got, host.ID)
	}
	if got := result.Resolved[0]["matched_by"]; got != "ip_token_unique" {
		t.Fatalf("matched_by = %v", got)
	}
}

func TestResolveHostSelectorsRejectsAmbiguousCompactIPLabel(t *testing.T) {
	logger.Logger = zap.NewNop()
	db, err := gorm.Open(sqlite.Open("file:host_resolve_ambiguous?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := createSQLiteHostTable(db); err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"10.0.0.159", "10.1.0.159"} {
		host := model.Host{ID: uuid.New(), IPAddress: ip, Hostname: "host-" + ip, OSType: "linux", AgentVersion: "6.1.0", LastHeartbeatAt: time.Now()}
		if err := db.Create(&host).Error; err != nil {
			t.Fatal(err)
		}
	}

	result, err := resolveHostSelectors(context.Background(), repository.NewHostRepository(db), nil, []string{"159IP"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resolved) != 0 || len(result.Ambiguous) != 1 {
		t.Fatalf("ambiguous selector was not rejected: %#v", result)
	}
}

func TestHostResolveSelectsAllOnlineHostsWithoutNaturalLanguageSelector(t *testing.T) {
	logger.Logger = zap.NewNop()
	db, err := gorm.Open(sqlite.Open("file:host_resolve_all_online?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := createSQLiteHostTable(db); err != nil {
		t.Fatal(err)
	}
	online := model.Host{ID: uuid.New(), IPAddress: "192.168.152.159", Hostname: "online-host", OSType: "linux", AgentVersion: "6.1.0", LastHeartbeatAt: time.Now()}
	offline := model.Host{ID: uuid.New(), IPAddress: "192.168.152.160", Hostname: "offline-host", OSType: "linux", AgentVersion: "6.1.0", LastHeartbeatAt: time.Now().Add(-10 * time.Minute)}
	for _, host := range []model.Host{online, offline} {
		if err := db.Create(&host).Error; err != nil {
			t.Fatal(err)
		}
	}

	result, err := makeHostResolveHandler(repository.NewHostRepository(db), nil)(context.Background(), map[string]interface{}{
		"target_scope":   "all_online_hosts",
		"require_online": true,
	})
	if err != nil {
		t.Fatalf("resolve all online hosts: %v", err)
	}
	resolution := result.(*hostResolution)
	if resolution.Coverage["resolved"] != 1 || len(resolution.Resolved) != 1 {
		t.Fatalf("expected one online host, got %#v", resolution)
	}
	if resolution.Resolved[0]["host_id"] != online.ID.String() || resolution.Resolved[0]["matched_by"] != "target_scope" {
		t.Fatalf("unexpected online host resolution: %#v", resolution.Resolved[0])
	}
}

func TestHostResolveReportsBusinessFailureForEmptyOnlineScope(t *testing.T) {
	logger.Logger = zap.NewNop()
	db, err := gorm.Open(sqlite.Open("file:host_resolve_empty_online?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := createSQLiteHostTable(db); err != nil {
		t.Fatal(err)
	}

	result, err := makeHostResolveHandler(repository.NewHostRepository(db), nil)(context.Background(), map[string]interface{}{
		"target_scope": "all_online_hosts",
	})
	if err != nil {
		t.Fatalf("resolve empty online scope: %v", err)
	}
	resolution := result.(*hostResolution)
	if resolution.OperationStatus != "failed" || resolution.Coverage["resolved"] != 0 {
		t.Fatalf("empty online scope must be a business failure: %#v", resolution)
	}
}

func TestHostResolvePreflightRejectsMissingTarget(t *testing.T) {
	if err := validateHostResolveArgs(context.Background(), map[string]interface{}{}); err == nil {
		t.Fatal("expected missing host scope to be rejected before handler execution")
	}
}

func createSQLiteHostTable(db *gorm.DB) error {
	return db.Exec(`CREATE TABLE hosts (
		id TEXT PRIMARY KEY,
		ip_address TEXT NOT NULL UNIQUE,
		hostname TEXT NOT NULL,
		os_type TEXT NOT NULL,
		agent_version TEXT NOT NULL,
		last_heartbeat_at DATETIME NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error
}
