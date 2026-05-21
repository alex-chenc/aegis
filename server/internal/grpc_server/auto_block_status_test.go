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

type successAgentClient struct {
	pb.UnimplementedAgentServiceServer
}

func (c *successAgentClient) Register(context.Context, *pb.RegisterRequest, ...grpc.CallOption) (*pb.RegisterResponse, error) {
	return nil, nil
}

func (c *successAgentClient) Heartbeat(context.Context, *pb.HeartbeatRequest, ...grpc.CallOption) (*pb.HeartbeatResponse, error) {
	return nil, nil
}

func (c *successAgentClient) ExecuteCommand(context.Context, ...grpc.CallOption) (grpc.BidiStreamingClient[pb.CommandRequest, pb.CommandRequest], error) {
	return nil, nil
}

func (c *successAgentClient) CollectSoftwareList(context.Context, *pb.SoftwareListRequest, ...grpc.CallOption) (*pb.SoftwareListResponse, error) {
	return nil, nil
}

func (c *successAgentClient) ReportEvent(context.Context, *pb.ReportEventRequest, ...grpc.CallOption) (*pb.ReportEventResponse, error) {
	return nil, nil
}

func (c *successAgentClient) ExecuteTool(context.Context, *pb.ToolRequest, ...grpc.CallOption) (*pb.ToolResponse, error) {
	return nil, nil
}

func (c *successAgentClient) UpdateRules(context.Context, *pb.RuleUpdateRequest, ...grpc.CallOption) (*pb.RuleUpdateResponse, error) {
	return nil, nil
}

func (c *successAgentClient) ExecuteBlockCommand(context.Context, *pb.BlockCommand, ...grpc.CallOption) (*pb.BlockResponse, error) {
	return &pb.BlockResponse{Success: true}, nil
}

func (c *successAgentClient) SyncConfig(context.Context, *pb.ConfigSyncRequest, ...grpc.CallOption) (*pb.ConfigSyncResponse, error) {
	return &pb.ConfigSyncResponse{Success: true}, nil
}

func createTestAlertForStatusUpdate(t *testing.T, db *gorm.DB, mitreID string) *model.Alert {
	t.Helper()
	alert := &model.Alert{
		ID:        uuid.New(),
		AlertID:   "ALT-" + uuid.New().String()[:8],
		HostID:    uuid.New(),
		PID:       1234,
		MitreID:   mitreID,
		Severity:  "high",
		DedupeKey: "test-dedupe-" + uuid.New().String()[:8],
		HitCount:  1,
		Status:    "pending",
	}
	if err := db.Create(alert).Error; err != nil {
		t.Fatalf("failed to create alert: %v", err)
	}
	return alert
}

func TestCheckAutoActions_BlockStatusSuccessOnCallbackSuccess(t *testing.T) {
	db := setupAutoActionTestDB(t)
	seedPolicyForTest(t, db, "T1059.004", true, true, false)
	alert := createTestAlertForStatusUpdate(t, db, "T1059.004")

	s := &GRPCServer{
		blockPolicyRepo: repository.NewBlockPolicyRepository(db),
		alertRepo:       repository.NewAlertRepository(db),
	}
	s.agentConnections.Store(alert.HostID, &AgentConnection{
		HostID:         alert.HostID,
		CallbackClient: &successAgentClient{},
		Ctx:            context.Background(),
	})

	s.checkAutoActions(alert)

	// Verify alert auto-block fields
	if !alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=true")
	}

	// Verify block_status was updated to "success" in the database
	var updated model.Alert
	if err := db.Where("alert_id = ?", alert.AlertID).First(&updated).Error; err != nil {
		t.Fatalf("failed to fetch updated alert: %v", err)
	}
	if updated.BlockStatus == nil || *updated.BlockStatus != "success" {
		blockStatusStr := "<nil>"
		if updated.BlockStatus != nil {
			blockStatusStr = *updated.BlockStatus
		}
		t.Fatalf("expected BlockStatus=success in DB, got %s", blockStatusStr)
	}
	if updated.Status != "resolved" {
		t.Fatalf("expected Status=resolved in DB, got %s", updated.Status)
	}
}

func TestCheckAutoActions_BlockStatusFailedOnCallbackFailure(t *testing.T) {
	db := setupAutoActionTestDB(t)
	seedPolicyForTest(t, db, "T1059.004", true, true, false)
	alert := createTestAlertForStatusUpdate(t, db, "T1059.004")

	s := &GRPCServer{
		blockPolicyRepo: repository.NewBlockPolicyRepository(db),
		alertRepo:       repository.NewAlertRepository(db),
	}
	s.agentConnections.Store(alert.HostID, &AgentConnection{
		HostID:         alert.HostID,
		CallbackClient: &failingAgentClient{reason: "process not found"},
		Ctx:            context.Background(),
	})

	s.checkAutoActions(alert)

	if !alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=true")
	}

	// Verify block_status was updated to "failed" in the database
	var updated model.Alert
	if err := db.Where("alert_id = ?", alert.AlertID).First(&updated).Error; err != nil {
		t.Fatalf("failed to fetch updated alert: %v", err)
	}
	if updated.BlockStatus == nil || *updated.BlockStatus != "failed" {
		blockStatusStr := "<nil>"
		if updated.BlockStatus != nil {
			blockStatusStr = *updated.BlockStatus
		}
		t.Fatalf("expected BlockStatus=failed in DB, got %s", blockStatusStr)
	}
	if updated.Status != "pending" {
		t.Fatalf("expected Status=pending in DB (not resolved on failure), got %s", updated.Status)
	}
}

func TestCheckAutoActions_BlockStatusSuccessOnStreamSend(t *testing.T) {
	db := setupAutoActionTestDB(t)
	seedPolicyForTest(t, db, "T1059.004", true, true, false)
	alert := createTestAlertForStatusUpdate(t, db, "T1059.004")

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

	if !alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=true")
	}
	if len(stream.sent) != 1 || stream.sent[0].GetBlock() == nil {
		t.Fatal("expected auto-block command to be sent via stream")
	}

	// Stream path: SendBlockCommand returns nil on successful send,
	// so block_status should be updated to "success"
	var updated model.Alert
	if err := db.Where("alert_id = ?", alert.AlertID).First(&updated).Error; err != nil {
		t.Fatalf("failed to fetch updated alert: %v", err)
	}
	if updated.BlockStatus == nil || *updated.BlockStatus != "success" {
		blockStatusStr := "<nil>"
		if updated.BlockStatus != nil {
			blockStatusStr = *updated.BlockStatus
		}
		t.Fatalf("expected BlockStatus=success in DB after stream send, got %s", blockStatusStr)
	}
}

func TestCheckAutoActions_BlockStatusSuccessWithAutoDispose(t *testing.T) {
	db := setupAutoActionTestDB(t)
	seedPolicyForTest(t, db, "T1059.004", true, true, true)
	alert := createTestAlertForStatusUpdate(t, db, "T1059.004")

	s := &GRPCServer{
		blockPolicyRepo: repository.NewBlockPolicyRepository(db),
		alertRepo:       repository.NewAlertRepository(db),
	}
	s.agentConnections.Store(alert.HostID, &AgentConnection{
		HostID:         alert.HostID,
		CallbackClient: &successAgentClient{},
		Ctx:            context.Background(),
	})

	s.checkAutoActions(alert)

	if !alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=true")
	}
	if !alert.AutoDispose {
		t.Fatal("expected AutoDispose=true")
	}

	// Verify block_status is "success" even when AutoDispose also runs
	// Previously, AutoDispose's Update(alert) would overwrite block_status
	// back to "blocking" because the in-memory struct wasn't updated
	var updated model.Alert
	if err := db.Where("alert_id = ?", alert.AlertID).First(&updated).Error; err != nil {
		t.Fatalf("failed to fetch updated alert: %v", err)
	}
	if updated.BlockStatus == nil || *updated.BlockStatus != "success" {
		blockStatusStr := "<nil>"
		if updated.BlockStatus != nil {
			blockStatusStr = *updated.BlockStatus
		}
		t.Fatalf("expected BlockStatus=success in DB with AutoDispose, got %s", blockStatusStr)
	}
	if updated.Status != "resolved" {
		t.Fatalf("expected Status=resolved in DB, got %s", updated.Status)
	}
}
