package service

import (
	"fmt"

	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"aegis-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BlockService handles threat blocking operations
type BlockService struct {
	blockRepo *repository.BlockRepository
	logger    *zap.Logger
}

// NewBlockService creates a new block service
func NewBlockService(blockRepo *repository.BlockRepository) *BlockService {
	return &BlockService{
		blockRepo: blockRepo,
		logger:    logger.Logger,
	}
}

// ExecuteBlock creates a block record and dispatches the command
func (s *BlockService) ExecuteBlock(hostID string, action string, target string, alertID *uuid.UUID, issuedBy string) (*model.BlockRecord, error) {
	// Create block record
	record := &model.BlockRecord{
		BlockID:  "BLK-" + uuid.New().String()[:8],
		AlertID:  alertID,
		HostID:   uuid.MustParse(hostID),
		Action:   action,
		Target:   target,
		IssuedBy: issuedBy,
	}

	// Dispatch block command
	err := s.dispatchBlockCommand(hostID, action, target)
	if err != nil {
		record.Success = false
		record.Message = err.Error()
		s.logger.Error("block command failed",
			zap.String("host_id", hostID),
			zap.String("action", action),
			zap.Error(err),
		)
	} else {
		record.Success = true
		record.Message = "阻断指令已下发"
	}

	// Save record
	if err := s.blockRepo.Create(record); err != nil {
		return nil, fmt.Errorf("failed to create block record: %w", err)
	}

	s.logger.Info("block executed",
		zap.String("block_id", record.BlockID),
		zap.String("host_id", hostID),
		zap.String("action", action),
		zap.Bool("success", record.Success),
	)

	return record, nil
}

// dispatchBlockCommand sends the block command to the agent via gRPC
func (s *BlockService) dispatchBlockCommand(hostID, action, target string) error {
	// TODO: Implement actual gRPC call to agent
	// For now, just log the command
	s.logger.Info("dispatching block command",
		zap.String("host_id", hostID),
		zap.String("action", action),
		zap.String("target", target),
	)
	return nil
}
