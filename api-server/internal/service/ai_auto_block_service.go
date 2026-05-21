package service

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	grpcclient "api-server/internal/grpc"
	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AIAutoBlockPayload is the top-level result returned after AI auto block execution.
type AIAutoBlockPayload struct {
	Triggered bool                    `json:"triggered"`
	Summary   AIAutoBlockSummary      `json:"summary"`
	Results   []AIAutoBlockResultItem `json:"results"`
}

// AIAutoBlockSummary aggregates counts of block outcomes.
type AIAutoBlockSummary struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// AIAutoBlockResultItem describes the outcome for a single alert.
type AIAutoBlockResultItem struct {
	AlertID          string `json:"alert_id"`
	MitreID          string `json:"mitre_id,omitempty"`
	Action           string `json:"action,omitempty"`
	Target           string `json:"target,omitempty"`
	Status           string `json:"status"` // success | failed | skipped
	Message          string `json:"message"`
	BlockID          string `json:"block_id,omitempty"`
	IssuedBy         string `json:"issued_by,omitempty"`
	ExistingBlockID  string `json:"existing_block_id,omitempty"`
	ExistingIssuedBy string `json:"existing_issued_by,omitempty"`
}

// AlertConclusion mirrors the structured conclusion from the LLM.
type AlertConclusion struct {
	AlertID string `json:"alert_id"`
	Action  string `json:"action"`  // confirm_threat, mark_false_positive, generate_rule
	Summary string `json:"summary"`
}

// AIAutoBlockService executes blocking actions based on AI analysis conclusions.
type AIAutoBlockService struct {
	alertRepo       *repository.AlertRepository
	blockPolicyRepo *repository.BlockPolicyRepository
	blockRepo       *repository.BlockRepository
	serverClient    *grpcclient.ServerClient
}

func NewAIAutoBlockService(
	alertRepo *repository.AlertRepository,
	blockPolicyRepo *repository.BlockPolicyRepository,
	blockRepo *repository.BlockRepository,
	serverClient *grpcclient.ServerClient,
) *AIAutoBlockService {
	return &AIAutoBlockService{
		alertRepo:       alertRepo,
		blockPolicyRepo: blockPolicyRepo,
		blockRepo:       blockRepo,
		serverClient:    serverClient,
	}
}

// Execute processes AI conclusions and blocks alerts where applicable.
// It filters for confirm_threat actions, checks policies, checks idempotency,
// and executes block commands. Returns a payload describing all outcomes.
func (s *AIAutoBlockService) Execute(conclusions []AlertConclusion, alertIDToUUID map[string]uuid.UUID) AIAutoBlockPayload {
	payload := AIAutoBlockPayload{
		Triggered: false,
		Results:   make([]AIAutoBlockResultItem, 0),
	}

	for _, conclusion := range conclusions {
		if conclusion.Action != "confirm_threat" {
			continue
		}

		alertUUID, ok := alertIDToUUID[conclusion.AlertID]
		if !ok {
			payload.Results = append(payload.Results, AIAutoBlockResultItem{
				AlertID: conclusion.AlertID,
				Status:  "skipped",
				Message: "alert not found in session snapshots",
			})
			payload.Summary.Skipped++
			payload.Summary.Total++
			continue
		}

		result := s.processAlert(conclusion.AlertID, alertUUID)
		payload.Results = append(payload.Results, result)
		payload.Summary.Total++

		switch result.Status {
		case "success":
			payload.Summary.Success++
		case "failed":
			payload.Summary.Failed++
		case "skipped":
			payload.Summary.Skipped++
		}
	}

	payload.Triggered = payload.Summary.Total > 0
	return payload
}

func (s *AIAutoBlockService) processAlert(alertID string, alertUUID uuid.UUID) AIAutoBlockResultItem {
	// 1. Load alert
	alert, err := s.alertRepo.FindByID(alertID)
	if err != nil {
		return AIAutoBlockResultItem{
			AlertID: alertID,
			Status:  "skipped",
			Message: fmt.Sprintf("alert not found: %v", err),
		}
	}

	// 2. Check policy
	policy, err := s.blockPolicyRepo.FindByMitreID(alert.MitreID)
	if err != nil {
		return AIAutoBlockResultItem{
			AlertID: alertID,
			MitreID: alert.MitreID,
			Status:  "skipped",
			Message: fmt.Sprintf("policy not found for %s: %v", alert.MitreID, err),
		}
	}

	if !policy.Enabled {
		return AIAutoBlockResultItem{
			AlertID: alertID,
			MitreID: alert.MitreID,
			Status:  "skipped",
			Message: "policy is disabled",
		}
	}
	if !policy.AIAutoBlock {
		return AIAutoBlockResultItem{
			AlertID: alertID,
			MitreID: alert.MitreID,
			Status:  "skipped",
			Message: "ai_auto_block is not enabled for this policy",
		}
	}
	if policy.AutoBlock {
		return AIAutoBlockResultItem{
			AlertID: alertID,
			MitreID: alert.MitreID,
			Status:  "skipped",
			Message: "auto_block is enabled (mutually exclusive with ai_auto_block)",
		}
	}

	// 3. Check idempotency: skip if any block record exists
	exists, existingRecord, err := s.blockRepo.ExistsByAlertID(alertUUID)
	if err != nil {
		return AIAutoBlockResultItem{
			AlertID: alertID,
			MitreID: alert.MitreID,
			Status:  "failed",
			Message: fmt.Sprintf("failed to check existing block records: %v", err),
		}
	}
	if exists {
		return AIAutoBlockResultItem{
			AlertID:          alertID,
			MitreID:          alert.MitreID,
			Status:           "skipped",
			Message:          "block record already exists",
			ExistingBlockID:  existingRecord.BlockID,
			ExistingIssuedBy: existingRecord.IssuedBy,
		}
	}

	// 4. Resolve target
	target, targetErr := aiAutoBlockTargetForAlert(alert, policy.Action)

	// 5. Create block record
	record := &model.BlockRecord{
		BlockID:  "BLK-" + uuid.New().String()[:8],
		AlertID:  &alertUUID,
		HostID:   alert.HostID,
		Action:   policy.Action,
		Target:   target,
		IssuedBy: "ai_auto",
	}

	// Update alert status to blocking
	blockStatus := BlockBlocking
	alert.BlockStatus = &blockStatus
	_ = s.alertRepo.Update(alert)

	if targetErr != nil {
		record.Success = false
		record.Message = targetErr.Error()
		_ = s.blockRepo.Create(record)
		_ = s.alertRepo.UpdateBlockStatus(alertID, BlockFailed, record.Message)
		return AIAutoBlockResultItem{
			AlertID:  alertID,
			MitreID:  alert.MitreID,
			Action:   policy.Action,
			Target:   target,
			Status:   "failed",
			Message:  record.Message,
			BlockID:  record.BlockID,
			IssuedBy: "ai_auto",
		}
	}

	// 6. Execute block command via gRPC
	if s.serverClient == nil {
		record.Success = false
		record.Message = "Server gRPC client not configured"
		_ = s.blockRepo.Create(record)
		_ = s.alertRepo.UpdateBlockStatus(alertID, BlockFailed, record.Message)
		return AIAutoBlockResultItem{
			AlertID:  alertID,
			MitreID:  alert.MitreID,
			Action:   policy.Action,
			Target:   target,
			Status:   "failed",
			Message:  record.Message,
			BlockID:  record.BlockID,
			IssuedBy: "ai_auto",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := s.serverClient.ExecuteBlockCommand(ctx, &pb.ExecuteBlockCommandRequest{
		CommandId: record.BlockID,
		HostId:    alert.HostID.String(),
		Action:    policy.Action,
		Target:    target,
		Reason:    "ai_auto block",
		AlertId:   alertID,
	})

	if err != nil {
		record.Success = false
		record.Message = fmt.Sprintf("阻断指令发送失败: %v", err)
		_ = s.blockRepo.Create(record)
		_ = s.alertRepo.UpdateBlockStatus(alertID, BlockFailed, record.Message)
		return AIAutoBlockResultItem{
			AlertID:  alertID,
			MitreID:  alert.MitreID,
			Action:   policy.Action,
			Target:   target,
			Status:   "failed",
			Message:  record.Message,
			BlockID:  record.BlockID,
			IssuedBy: "ai_auto",
		}
	}

	if !resp.Success {
		record.Success = false
		record.Message = resp.Error
		if record.Message == "" {
			record.Message = "阻断失败，原因未知"
		}
		_ = s.blockRepo.Create(record)
		_ = s.alertRepo.UpdateBlockStatus(alertID, BlockFailed, record.Message)
		return AIAutoBlockResultItem{
			AlertID:  alertID,
			MitreID:  alert.MitreID,
			Action:   policy.Action,
			Target:   target,
			Status:   "failed",
			Message:  record.Message,
			BlockID:  record.BlockID,
			IssuedBy: "ai_auto",
		}
	}

	// Success
	record.Success = true
	record.Message = "阻断成功"
	_ = s.blockRepo.Create(record)
	_ = s.alertRepo.UpdateBlockStatus(alertID, BlockSuccess, "AI自动阻断执行成功")

	logger.Info("ai_auto_block: block executed successfully",
		zap.String("alert_id", alertID),
		zap.String("block_id", record.BlockID),
		zap.String("action", policy.Action),
		zap.String("target", target),
	)

	return AIAutoBlockResultItem{
		AlertID:  alertID,
		MitreID:  alert.MitreID,
		Action:   policy.Action,
		Target:   target,
		Status:   "success",
		Message:  "阻断成功",
		BlockID:  record.BlockID,
		IssuedBy: "ai_auto",
	}
}

// aiAutoBlockTargetForAlert resolves the block target from alert data.
func aiAutoBlockTargetForAlert(alert *model.Alert, action string) (string, error) {
	switch action {
	case "kill_process":
		if alert.PID <= 0 {
			return "", fmt.Errorf("missing process pid for kill_process")
		}
		return fmt.Sprintf("%d", alert.PID), nil
	case "quarantine_file":
		target := strings.TrimSpace(alert.CommandLine)
		if target == "" {
			return "", fmt.Errorf("missing file path for quarantine_file")
		}
		return target, nil
	case "block_connection":
		target := strings.TrimSpace(alert.CommandLine)
		if target == "" {
			return "", fmt.Errorf("missing remote address for block_connection")
		}
		if net.ParseIP(target) == nil {
			return "", fmt.Errorf("invalid remote address for block_connection: %s", target)
		}
		return target, nil
	default:
		if alert.PID <= 0 {
			return "", fmt.Errorf("missing process pid for %s", action)
		}
		return fmt.Sprintf("%d", alert.PID), nil
	}
}
