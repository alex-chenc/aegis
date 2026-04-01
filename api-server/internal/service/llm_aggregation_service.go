package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"api-server/internal/model"
	"api-server/internal/pipeline"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type LLMAggregationService struct {
	aggRepo         *repository.LLMAggregationRepository
	eventRepo       *repository.RuntimeEventRepository
	alertRepo       *repository.AlertRepository
	llmService      *LLMAnalysisService
	blockPolicyRepo *repository.BlockPolicyRepository
	blockRepo       *repository.BlockRepository
}

func NewLLMAggregationService(
	aggRepo *repository.LLMAggregationRepository,
	eventRepo *repository.RuntimeEventRepository,
	alertRepo *repository.AlertRepository,
	llmService *LLMAnalysisService,
	blockPolicyRepo *repository.BlockPolicyRepository,
	blockRepo *repository.BlockRepository,
) *LLMAggregationService {
	return &LLMAggregationService{
		aggRepo:         aggRepo,
		eventRepo:       eventRepo,
		alertRepo:       alertRepo,
		llmService:      llmService,
		blockPolicyRepo: blockPolicyRepo,
		blockRepo:       blockRepo,
	}
}

type AggregateRequest struct {
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	HostIDs     []string  `json:"host_ids"`
	AutoDispose bool      `json:"auto_dispose"`
}

type AggregateResult struct {
	AggregationID    string `json:"aggregation_id"`
	Status           string `json:"status"`
	EventCount       int    `json:"event_count"`
	AlertCount       int    `json:"alert_count"`
	AIJudgedCount    int    `json:"ai_judged_count"`
	AutoDisposeCount int    `json:"auto_dispose_count"`
}

func (s *LLMAggregationService) StartAggregation(ctx context.Context, req AggregateRequest) (*AggregateResult, error) {
	maxDuration := 24 * time.Hour
	if req.EndTime.Sub(req.StartTime) > maxDuration {
		return nil, fmt.Errorf("time range exceeds maximum of 24 hours")
	}

	aggID := "AGG-" + uuid.New().String()[:8]
	agg := &model.LLMAggregation{
		AggregationID: aggID,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		HostIDs:       req.HostIDs,
		Status:        "pending",
	}

	if err := s.aggRepo.Create(agg); err != nil {
		return nil, err
	}

	go s.executeAggregation(context.Background(), agg, req.AutoDispose)

	return &AggregateResult{
		AggregationID: aggID,
		Status:        "pending",
	}, nil
}

func (s *LLMAggregationService) executeAggregation(ctx context.Context, agg *model.LLMAggregation, autoDispose bool) {
	s.aggRepo.UpdateStatus(agg.ID, "processing", "")

	startTs := agg.StartTime.UnixMilli()
	endTs := agg.EndTime.UnixMilli()

	events, err := s.eventRepo.FindUnaggregated(startTs, endTs, agg.HostIDs)
	if err != nil {
		s.aggRepo.UpdateStatus(agg.ID, "failed", err.Error())
		return
	}

	agg.EventCount = len(events)
	s.aggRepo.Update(agg)

	if len(events) == 0 {
		s.aggRepo.UpdateStatus(agg.ID, "completed", "")
		return
	}

	window := &pipeline.HostWindow{
		HostID: "aggregated",
		Events: s.convertEvents(events),
	}

	result, err := s.llmService.Analyze(ctx, window)
	if err != nil {
		s.aggRepo.UpdateStatus(agg.ID, "failed", err.Error())
		return
	}

	aiJudgedCount := 0
	autoDisposeCount := 0

	for _, alertPayload := range result.Alerts {
		if alertPayload.JudgmentSource == JudgmentAI {
			aiJudgedCount++
		}

		alert, err := s.createAlertFromPayload(alertPayload, agg.HostIDs)
		if err != nil {
			logger.Error("failed to create alert", zap.Error(err))
			continue
		}

		agg.AlertCount++

		if alertPayload.JudgmentSource == JudgmentAI && autoDispose {
			policy, err := s.blockPolicyRepo.FindByMitreID(alert.MitreID)
			if err == nil && policy.Enabled && policy.AutoDispose {
				if err := s.executeAutoDispose(alert, policy); err != nil {
					logger.Error("auto-dispose failed", zap.Error(err))
				} else {
					autoDisposeCount++
				}
			}
		}
	}

	eventIDs := make([]uuid.UUID, len(events))
	for i, e := range events {
		eventIDs[i] = e.ID
	}
	s.eventRepo.MarkAggregated(eventIDs)

	agg.AIJudgedCount = aiJudgedCount
	agg.AutoDisposeCount = autoDisposeCount
	agg.Status = "completed"
	now := time.Now()
	agg.CompletedAt = &now

	respJSON, _ := json.Marshal(result)
	agg.LLMResponse = string(respJSON)

	s.aggRepo.Update(agg)
}

func (s *LLMAggregationService) createAlertFromPayload(payload pipeline.AlertPayload, hostIDs []string) (*model.Alert, error) {
	var hostID uuid.UUID
	if len(hostIDs) > 0 {
		hostID, _ = uuid.Parse(hostIDs[0])
	} else {
		hostID = uuid.New()
	}

	judgmentSource := payload.JudgmentSource
	if judgmentSource == "" {
		judgmentSource = JudgmentAI
	}

	description := payload.Description
	if description == "" {
		description = fmt.Sprintf("AI检测到潜在威胁行为，MITRE技术: %s", payload.MitreID)
	}

	llmSummary := payload.LLMSummary
	if llmSummary == "" {
		llmSummary = fmt.Sprintf("AI降噪判定: %s级别威胁", payload.Severity)
	}

	alert := &model.Alert{
		AlertID:             "ALT-" + uuid.New().String()[:8],
		HostID:              hostID,
		PID:                 payload.PID,
		RuleID:              payload.RuleID,
		RuleTitle:           payload.RuleTitle,
		MitreID:             payload.MitreID,
		MitreName:           payload.MitreName,
		Severity:            payload.Severity,
		Description:         description,
		LLMSummary:          llmSummary,
		LLMDisposalStrategy: payload.DisposalStrategy,
		JudgmentSource:      judgmentSource,
		Status:              StatusPending,
		HitCount:            1,
		DedupeKey:           fmt.Sprintf("%s:%d:%s", hostID.String(), payload.PID, payload.RuleID),
	}

	return alert, s.alertRepo.Create(alert)
}

func (s *LLMAggregationService) executeAutoDispose(alert *model.Alert, policy *model.BlockPolicy) error {
	alert.AutoDispose = true
	blockStatus := BlockBlocking
	alert.BlockStatus = &blockStatus
	s.alertRepo.Update(alert)

	record := &model.BlockRecord{
		BlockID:  "BLK-" + uuid.New().String()[:8],
		AlertID:  &alert.ID,
		HostID:   alert.HostID,
		Action:   policy.Action,
		Target:   fmt.Sprintf("%d", alert.PID),
		IssuedBy: "auto_dispose",
	}

	return s.blockRepo.Create(record)
}

func (s *LLMAggregationService) convertEvents(events []model.RuntimeEvent) []pipeline.RuntimeEvent {
	var result []pipeline.RuntimeEvent
	for _, e := range events {
		result = append(result, pipeline.RuntimeEvent{
			EventType:     e.EventType,
			PID:           e.PID,
			CommandLine:   e.CommandLine,
			MatchedRuleID: e.MatchedRuleID,
			MitreID:       e.MitreID,
			Severity:      e.Severity,
			Timestamp:     e.Timestamp,
		})
	}
	return result
}

func (s *LLMAggregationService) GetStatus(aggregationID string) (*model.LLMAggregation, error) {
	return s.aggRepo.FindByID(aggregationID)
}
