package grpc_server

import (
	"context"
	"testing"

	"server/internal/model"
	"server/internal/repository"
	pb "server/pkg/api/v1"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

type captureAgentStream struct {
	grpc.ServerStream
	sent []*pb.CommandRequest
}

func (s *captureAgentStream) Send(req *pb.CommandRequest) error {
	s.sent = append(s.sent, req)
	return nil
}

func (s *captureAgentStream) Recv() (*pb.CommandRequest, error) {
	return nil, context.Canceled
}

func seedPolicyWithActionForTest(t *testing.T, db *gorm.DB, mitreID, action string) {
	t.Helper()
	if err := db.Exec(
		"INSERT INTO block_policies (id, mitre_id, enabled, auto_block, auto_dispose, action, created_at, updated_at) VALUES (?, ?, 1, 1, 0, ?, datetime('now'), datetime('now'))",
		uuid.New().String(), mitreID, action,
	).Error; err != nil {
		t.Fatalf("failed to seed policy: %v", err)
	}
}

func createTestAlertWithCommandLine(t *testing.T, db *gorm.DB, mitreID string, pid int, commandLine string) *model.Alert {
	t.Helper()
	alert := &model.Alert{
		ID:          uuid.New(),
		AlertID:     "ALT-" + uuid.New().String()[:8],
		HostID:      uuid.New(),
		PID:         pid,
		CommandLine: commandLine,
		MitreID:     mitreID,
		Severity:    "high",
		DedupeKey:   "test-dedupe-" + uuid.New().String()[:8],
		HitCount:    1,
		Status:      "pending",
	}
	if err := db.Create(alert).Error; err != nil {
		t.Fatalf("failed to create alert: %v", err)
	}
	return alert
}

func TestCheckAutoActionsSendsBlockCommandForSupportedStrategies(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		pid         int
		commandLine string
		wantTarget  string
	}{
		{name: "kill_process", action: "kill_process", pid: 4321, wantTarget: "4321"},
		{name: "quarantine_file", action: "quarantine_file", pid: 4322, commandLine: "/tmp/aegis-block-test-file", wantTarget: "/tmp/aegis-block-test-file"},
		{name: "block_connection", action: "block_connection", pid: 4323, commandLine: "203.0.113.25", wantTarget: "203.0.113.25"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupAutoActionTestDB(t)
			seedPolicyWithActionForTest(t, db, "T1059.004", tt.action)
			alert := createTestAlertWithCommandLine(t, db, "T1059.004", tt.pid, tt.commandLine)

			stream := &captureAgentStream{}
			s := &GRPCServer{
				blockPolicyRepo: repository.NewBlockPolicyRepository(db),
				alertRepo:       repository.NewAlertRepository(db),
			}
			s.agentConnections.Store(alert.HostID, &AgentConnection{
				HostID: alert.HostID,
				Stream: stream,
				Ctx:    context.Background(),
				Inbox:  make(chan *pb.CommandExecute, 1),
			})

			s.checkAutoActions(alert)

			if len(stream.sent) != 1 {
				t.Fatalf("expected one block command, got %d", len(stream.sent))
			}
			block := stream.sent[0].GetBlock()
			if block == nil {
				t.Fatal("expected block command request")
			}
			if block.Action != tt.action {
				t.Fatalf("expected action %s, got %s", tt.action, block.Action)
			}
			if block.Target != tt.wantTarget {
				t.Fatalf("expected target %s, got %s", tt.wantTarget, block.Target)
			}
		})
	}
}
