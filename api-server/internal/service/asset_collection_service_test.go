package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeAssetCollectionServerClient struct {
	resp            *pb.ListConnectedAgentsResponse
	err             error
	executeResp     *pb.ToolExecuteResponse
	executeErr      error
	executedTool    string
	executedHostID  string
	executedArgs    string
	executedTimeout int32
}

func (f *fakeAssetCollectionServerClient) ListConnectedAgents(ctx context.Context) (*pb.ListConnectedAgentsResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func (f *fakeAssetCollectionServerClient) ExecuteTool(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error) {
	f.executedTool = tool
	f.executedHostID = hostID
	f.executedArgs = arguments
	f.executedTimeout = timeoutSeconds
	if f.executeErr != nil {
		return nil, f.executeErr
	}
	if f.executeResp != nil {
		return f.executeResp, nil
	}
	return &pb.ToolExecuteResponse{CallId: callID, Success: true, Result: `{}`}, nil
}

func TestNormalizeCollectTypesPreservesSoftware(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{name: "software", input: []string{"software", "process"}, want: []string{"process", "software"}},
		{name: "full", input: []string{"full"}, want: []string{"process", "software", "application_analysis"}},
		{name: "process only", input: []string{"process"}, want: []string{"process"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeCollectTypes(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %#v, got %#v", tt.want, got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("expected %#v, got %#v", tt.want, got)
				}
			}
		})
	}
}

func TestCollectSoftwareUsesHostAssetTool(t *testing.T) {
	client := &fakeAssetCollectionServerClient{
		executeResp: &pb.ToolExecuteResponse{
			Success: true,
			Result:  `{"host_id":"host-1","hostname":"node-1","packages":[{"name":"curl","version":"8.0","architecture":"amd64","package_manager":"dpkg"}]}`,
		},
	}
	svc := NewAssetCollectionService(nil, client, zap.NewNop())

	snapshot, err := svc.collectSoftware(context.Background(), "host-1")
	if err != nil {
		t.Fatalf("expected software collection to succeed: %v", err)
	}
	if len(snapshot.Packages) != 1 || snapshot.Packages[0].Name != "curl" {
		t.Fatalf("unexpected software snapshot: %#v", snapshot)
	}
	if client.executedTool != "AssetCollectHostAssets" || client.executedHostID != "host-1" {
		t.Fatalf("unexpected tool call: tool=%s host=%s", client.executedTool, client.executedHostID)
	}
	if client.executedTimeout != 120 {
		t.Fatalf("expected 120 second timeout, got %d", client.executedTimeout)
	}

	var args struct {
		CollectTypes        []string `json:"collect_types"`
		IncludePackageFiles bool     `json:"include_package_files"`
	}
	if err := json.Unmarshal([]byte(client.executedArgs), &args); err != nil {
		t.Fatalf("failed to parse tool arguments: %v", err)
	}
	if len(args.CollectTypes) != 1 || args.CollectTypes[0] != "software" {
		t.Fatalf("unexpected collect types: %#v", args.CollectTypes)
	}
	if args.IncludePackageFiles {
		t.Fatal("package file collection should be disabled")
	}
}

func TestCollectSoftwareRejectsFailedToolResponse(t *testing.T) {
	client := &fakeAssetCollectionServerClient{
		executeResp: &pb.ToolExecuteResponse{Success: false, Error: "agent unavailable"},
	}
	svc := NewAssetCollectionService(nil, client, zap.NewNop())

	if _, err := svc.collectSoftware(context.Background(), "host-1"); err == nil {
		t.Fatal("expected failed tool response to return an error")
	}
}

func TestSaveSoftwareAssetsWritesHostSoftwareAssets(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to access test database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec(`CREATE TABLE host_software_assets (
		id TEXT PRIMARY KEY,
		host_id TEXT NOT NULL,
		hostname TEXT,
		ip_address TEXT,
		group_name TEXT,
		os_type TEXT,
		package_manager TEXT NOT NULL,
		name TEXT,
		version TEXT,
		release TEXT,
		epoch TEXT,
		architecture TEXT,
		source_name TEXT,
		vendor TEXT,
		license TEXT,
		install_paths TEXT,
		file_count INTEGER,
		package_metadata TEXT,
		fingerprint TEXT NOT NULL,
		status TEXT,
		last_modified_at DATETIME,
		first_seen_at DATETIME,
		last_seen_at DATETIME,
		collected_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME,
		UNIQUE(host_id, package_manager, fingerprint)
	)`).Error; err != nil {
		t.Fatalf("failed to create software asset table: %v", err)
	}

	svc := NewAssetCollectionService(repository.NewAssetCollectionRepository(db), nil, zap.NewNop())
	hostID := uuid.New()
	snapshot := HostAssetSnapshot{
		HostID:    hostID.String(),
		Hostname:  "node-1",
		IPAddress: "10.0.0.1",
		OSType:    "linux",
		Packages: []PackageAsset{
			{
				Name:           "curl",
				Version:        "8.0",
				Architecture:   "amd64",
				PackageManager: "dpkg",
				InstallTime:    time.Now(),
			},
		},
	}

	count, err := svc.saveSoftwareAssets(hostID, snapshot)
	if err != nil {
		t.Fatalf("expected software assets to be saved: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one saved software asset, got %d", count)
	}

	var saved model.HostSoftwareAsset
	if err := db.Where("host_id = ?", hostID).First(&saved).Error; err != nil {
		t.Fatalf("expected saved software asset: %v", err)
	}
	if saved.Name != "curl" || saved.Version != "8.0" || saved.Hostname != "node-1" {
		t.Fatalf("unexpected saved software asset: %#v", saved)
	}
}

func TestResolveTargetHostIDsAllHostsUsesConnectedAgents(t *testing.T) {
	svc := NewAssetCollectionService(nil, &fakeAssetCollectionServerClient{
		resp: &pb.ListConnectedAgentsResponse{
			Agents: []*pb.AgentInfo{
				{HostId: "host-online", Connected: true},
				{HostId: "host-offline", Connected: false},
				{HostId: "", Connected: true},
			},
		},
	}, zap.NewNop())

	hostIDs, err := svc.resolveTargetHostIDs(context.Background(), model.TriggerAssetCollectionRequest{
		Scope: "all_hosts",
		Types: []string{"software", "process"},
	})
	if err != nil {
		t.Fatalf("expected connected host resolution to succeed: %v", err)
	}
	if len(hostIDs) != 1 || hostIDs[0] != "host-online" {
		t.Fatalf("expected only connected host IDs, got %#v", hostIDs)
	}
}

func TestResolveTargetHostIDsAllHostsRequiresConnectedAgents(t *testing.T) {
	svc := NewAssetCollectionService(nil, &fakeAssetCollectionServerClient{
		resp: &pb.ListConnectedAgentsResponse{
			Agents: []*pb.AgentInfo{{HostId: "host-offline", Connected: false}},
		},
	}, zap.NewNop())

	_, err := svc.resolveTargetHostIDs(context.Background(), model.TriggerAssetCollectionRequest{
		Scope: "all_hosts",
		Types: []string{"software", "process"},
	})
	if err == nil {
		t.Fatal("expected error when no connected hosts are available")
	}
}
