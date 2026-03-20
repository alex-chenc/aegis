package service

import (
	"fmt"
	"time"

	"aegis-system/internal/model"
	"aegis-system/internal/repository"
	"aegis-system/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AlertService struct {
	alertRepo       *repository.AlertRepository
	blockPolicyRepo *repository.BlockPolicyRepository
	blockRepo       *repository.BlockRepository
}

func NewAlertService(
	alertRepo *repository.AlertRepository,
	blockPolicyRepo *repository.BlockPolicyRepository,
	blockRepo *repository.BlockRepository,
) *AlertService {
	return &AlertService{
		alertRepo:       alertRepo,
		blockPolicyRepo: blockPolicyRepo,
		blockRepo:       blockRepo,
	}
}

func (s *AlertService) UpsertByDedupe(hostID uuid.UUID, pid int, mitreID, mitreName, severity, description string) (*model.Alert, error) {
	dedupeKey := fmt.Sprintf("%s:%d:%s", hostID.String(), pid, mitreID)

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
		AlertID:     "ALT-" + uuid.New().String()[:8],
		HostID:      hostID,
		PID:         pid,
		MitreID:     mitreID,
		MitreName:   mitreName,
		Severity:    severity,
		Description: description,
		DedupeKey:   dedupeKey,
		HitCount:    1,
		Status:      "active",
	}

	if err := s.alertRepo.Create(alert); err != nil {
		return nil, err
	}

	logger.Info("alert created",
		zap.String("alert_id", alert.AlertID),
		zap.String("mitre_id", mitreID),
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
	if err := s.alertRepo.Update(alert); err != nil {
		return err
	}

	record := &model.BlockRecord{
		BlockID:  "BLK-" + uuid.New().String()[:8],
		AlertID:  &alert.ID,
		HostID:   alert.HostID,
		Action:   "kill_process",
		Target:   fmt.Sprintf("%d", alert.PID),
		IssuedBy: "llm",
	}

	return s.blockRepo.Create(record)
}

func (s *AlertService) ManualBlock(alertID string) (*model.BlockRecord, error) {
	alert, err := s.alertRepo.FindByID(alertID)
	if err != nil {
		return nil, err
	}

	alert.ManualBlocked = true
	if err := s.alertRepo.Update(alert); err != nil {
		return nil, err
	}

	record := &model.BlockRecord{
		BlockID:  "BLK-" + uuid.New().String()[:8],
		AlertID:  &alert.ID,
		HostID:   alert.HostID,
		Action:   "kill_process",
		Target:   fmt.Sprintf("%d", alert.PID),
		IssuedBy: "manual",
	}

	if err := s.blockRepo.Create(record); err != nil {
		return nil, err
	}

	return record, nil
}
