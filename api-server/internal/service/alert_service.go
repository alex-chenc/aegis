package service

import (
	"context"
	"fmt"
	"time"

	grpcclient "api-server/internal/grpc"
	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	StatusPending  = "pending"
	StatusResolved = "resolved"
	BlockPending   = "pending"
	BlockBlocking  = "blocking"
	BlockSuccess   = "success"
	BlockFailed    = "failed"
	JudgmentSystem = "system"
	JudgmentAI     = "ai"
)

type AlertService struct {
	alertRepo       *repository.AlertRepository
	blockPolicyRepo *repository.BlockPolicyRepository
	blockRepo       *repository.BlockRepository
	serverClient    *grpcclient.ServerClient
}

func NewAlertService(
	alertRepo *repository.AlertRepository,
	blockPolicyRepo *repository.BlockPolicyRepository,
	blockRepo *repository.BlockRepository,
	serverClient *grpcclient.ServerClient,
) *AlertService {
	return &AlertService{
		alertRepo:       alertRepo,
		blockPolicyRepo: blockPolicyRepo,
		blockRepo:       blockRepo,
		serverClient:    serverClient,
	}
}

func (s *AlertService) UpsertByDedupe(hostID uuid.UUID, pid int, ruleID, ruleTitle, mitreID, mitreName, severity, description string) (*model.Alert, error) {
	dedupeKey := fmt.Sprintf("%s:%d:%s", hostID.String(), pid, ruleID)

	existing, err := s.alertRepo.FindByDedupeKey(dedupeKey)
	if err == nil && existing != nil {
		existing.HitCount++
		existing.LastSeenAt = time.Now()
		if updateErr := s.alertRepo.Update(existing); updateErr != nil {
			return nil, updateErr
		}
		return existing, nil
	}

	alert := &model.Alert{
		ID:             uuid.New(),
		AlertID:        "ALT-" + uuid.New().String()[:8],
		HostID:         hostID,
		PID:            pid,
		MitreID:        mitreID,
		MitreName:      mitreName,
		Severity:       severity,
		Description:    description,
		DedupeKey:      dedupeKey,
		HitCount:       1,
		Status:         StatusPending,
		JudgmentSource: JudgmentSystem,
		RuleID:         ruleID,
		RuleTitle:      ruleTitle,
	}

	if err := s.alertRepo.Create(alert); err != nil {
		return nil, err
	}

	logger.Info("alert created",
		zap.String("alert_id", alert.AlertID),
		zap.String("mitre_id", mitreID),
		zap.String("rule_id", ruleID),
		zap.Int("pid", pid),
	)

	return alert, nil
}

func (s *AlertService) CheckAndAutoBlock(alert *model.Alert) error {
	policy, err := s.blockPolicyRepo.FindByMitreID(alert.MitreID)
	if err != nil || !policy.Enabled || !policy.AutoBlock {
		return nil
	}

	logger.Info("auto-blocking",
		zap.String("alert_id", alert.AlertID),
		zap.String("mitre_id", alert.MitreID),
		zap.Int("pid", alert.PID),
	)

	alert.AutoBlocked = true
	blockStatus := BlockBlocking
	alert.BlockStatus = &blockStatus
	if err := s.alertRepo.Update(alert); err != nil {
		return err
	}

	record := &model.BlockRecord{
		BlockID:  "BLK-" + uuid.New().String()[:8],
		AlertID:  &alert.ID,
		HostID:   alert.HostID,
		Action:   policy.Action,
		Target:   fmt.Sprintf("%d", alert.PID),
		IssuedBy: "auto",
	}

	return s.blockRepo.Create(record)
}

// CheckAndAutoDispose checks if auto-dispose is enabled for the alert's MITRE ID
// and automatically resolves the alert if so
func (s *AlertService) CheckAndAutoDispose(alert *model.Alert) error {
	policy, err := s.blockPolicyRepo.FindByMitreID(alert.MitreID)
	if err != nil || !policy.Enabled || !policy.AutoDispose {
		return nil
	}

	logger.Info("auto-disposing alert",
		zap.String("alert_id", alert.AlertID),
		zap.String("mitre_id", alert.MitreID),
		zap.Int("pid", alert.PID),
	)

	alert.AutoDispose = true
	alert.Status = StatusResolved
	if err := s.alertRepo.Update(alert); err != nil {
		return err
	}

	logger.Info("alert auto-disposed successfully",
		zap.String("alert_id", alert.AlertID),
		zap.String("status", StatusResolved),
	)

	return nil
}

func (s *AlertService) ManualBlock(alertID string, action string) (*model.BlockRecord, error) {
	alert, err := s.alertRepo.FindByID(alertID)
	if err != nil {
		return nil, err
	}

	if action == "" {
		action = "kill_process"
	}

	var target string
	switch action {
	case "kill_process":
		target = fmt.Sprintf("%d", alert.PID)
	case "quarantine_file":
		target = alert.CommandLine
	case "block_connection", "disable_user":
		target = fmt.Sprintf("%d", alert.PID)
	default:
		target = fmt.Sprintf("%d", alert.PID)
	}

	record := &model.BlockRecord{
		BlockID:  "BLK-" + uuid.New().String()[:8],
		AlertID:  &alert.ID,
		HostID:   alert.HostID,
		Action:   action,
		Target:   target,
		IssuedBy: "manual",
	}

	alert.ManualBlocked = true
	blockStatus := BlockBlocking
	alert.BlockStatus = &blockStatus
	s.alertRepo.Update(alert)

	// Check if server client is available
	if s.serverClient == nil {
		record.Success = false
		record.Message = "Server gRPC client not configured"
		s.blockRepo.Create(record)
		s.alertRepo.UpdateBlockStatus(alertID, BlockFailed, "Server gRPC client not configured")
		return record, nil
	}

	// Check agent connectivity via server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	statusResp, err := s.serverClient.GetAgentStatus(ctx, alert.HostID.String())
	if err != nil || !statusResp.Connected {
		record.Success = false
		record.Message = "Agent未连接"
		s.blockRepo.Create(record)
		s.alertRepo.UpdateBlockStatus(alertID, BlockFailed, "Agent未连接")
		return record, nil
	}

	// Execute block command via server
	blockCtx, blockCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer blockCancel()

	resp, err := s.serverClient.ExecuteBlockCommand(blockCtx, &pb.ExecuteBlockCommandRequest{
		CommandId: record.BlockID,
		HostId:    alert.HostID.String(),
		Action:    action,
		Target:    target,
		Reason:    "manual block",
		AlertId:   alertID,
	})

	if err != nil {
		record.Success = false
		record.Message = fmt.Sprintf("阻断指令发送失败: %v", err)
		s.blockRepo.Create(record)
		s.alertRepo.UpdateBlockStatus(alertID, BlockFailed, record.Message)
		return record, nil
	}

	if resp.Success {
		record.Success = true
		record.Message = "阻断成功"
		s.blockRepo.Create(record)
		s.alertRepo.UpdateBlockStatus(alertID, BlockSuccess, "阻断执行成功")
	} else {
		record.Success = false
		record.Message = resp.Error
		if record.Message == "" {
			record.Message = "阻断失败，原因未知"
		}
		s.blockRepo.Create(record)
		s.alertRepo.UpdateBlockStatus(alertID, BlockFailed, record.Message)
	}

	return record, nil
}

func (s *AlertService) Resolve(alertID string) error {
	return s.alertRepo.Resolve(alertID)
}

func (s *AlertService) UpdateBlockStatus(alertID string, status string, message string) error {
	return s.alertRepo.UpdateBlockStatus(alertID, status, message)
}

func (s *AlertService) MarkAIJudged(alertID string, disposalStrategy string) error {
	return s.alertRepo.MarkAIJudged(alertID, disposalStrategy)
}
