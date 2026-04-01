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

type BlockService struct {
	blockRepo    *repository.BlockRepository
	alertRepo    *repository.AlertRepository
	serverClient *grpcclient.ServerClient
	logger       *zap.Logger
}

func NewBlockService(blockRepo *repository.BlockRepository, alertRepo *repository.AlertRepository) *BlockService {
	return &BlockService{
		blockRepo: blockRepo,
		alertRepo: alertRepo,
		logger:    logger.Logger,
	}
}

func (s *BlockService) SetServerClient(client *grpcclient.ServerClient) {
	s.serverClient = client
}

type BlockResult struct {
	Record   *model.BlockRecord
	Success  bool
	Message  string
	AlertMsg string
}

func (s *BlockService) ExecuteBlock(hostID string, action string, target string, alertID string, issuedBy string) (*BlockResult, error) {
	hostUUID, err := uuid.Parse(hostID)
	if err != nil {
		return nil, fmt.Errorf("invalid host_id: %w", err)
	}

	var alertUUID *uuid.UUID
	if alertID != "" {
		id, err := uuid.Parse(alertID)
		if err == nil {
			alertUUID = &id
		}
	}

	record := &model.BlockRecord{
		BlockID:  "BLK-" + uuid.New().String()[:8],
		AlertID:  alertUUID,
		HostID:   hostUUID,
		Action:   action,
		Target:   target,
		IssuedBy: issuedBy,
	}

	if s.serverClient == nil {
		record.Success = false
		record.Message = "gRPC服务不可用"
		s.blockRepo.Create(record)
		return &BlockResult{Record: record, Success: false, Message: record.Message}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := s.serverClient.ExecuteBlockCommand(ctx, &pb.ExecuteBlockCommandRequest{
		CommandId: record.BlockID,
		HostId:    hostID,
		Action:    action,
		Target:    target,
		Reason:    "manual block",
	})

	if err != nil {
		record.Success = false
		record.Message = fmt.Sprintf("阻断指令发送失败: %v", err)
		s.blockRepo.Create(record)
		if alertUUID != nil {
			s.alertRepo.UpdateBlockStatus(alertID, "failed", record.Message)
		}
		return &BlockResult{Record: record, Success: false, Message: record.Message}, nil
	}

	if resp.Success {
		record.Success = true
		record.Message = "阻断成功"
		s.blockRepo.Create(record)
		if alertUUID != nil {
			s.alertRepo.UpdateBlockStatus(alertID, "success", "阻断执行成功")
		}
		return &BlockResult{Record: record, Success: true, Message: "阻断成功"}, nil
	} else {
		record.Success = false
		record.Message = resp.Error
		if record.Message == "" {
			record.Message = "阻断失败，原因未知"
		}
		s.blockRepo.Create(record)
		if alertUUID != nil {
			s.alertRepo.UpdateBlockStatus(alertID, "failed", record.Message)
		}
		return &BlockResult{Record: record, Success: false, Message: record.Message}, nil
	}
}
