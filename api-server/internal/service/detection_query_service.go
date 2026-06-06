package service

import (
	"context"
	"fmt"

	"api-server/internal/model"
	"api-server/internal/repository"
)

// DetectionQueryService 检测查询服务（对齐设计文档第 13 节）
type DetectionQueryService struct {
	alertRepo     *repository.AlertRepository
	blockRepo     *repository.BlockRepository
	sigmaRuleRepo *repository.SigmaRuleRepository
	toolCallRepo  repository.AssistantToolCallRepository
}

// NewDetectionQueryService 创建检测查询服务
func NewDetectionQueryService(
	alertRepo *repository.AlertRepository,
	blockRepo *repository.BlockRepository,
	sigmaRuleRepo *repository.SigmaRuleRepository,
	toolCallRepo repository.AssistantToolCallRepository,
) *DetectionQueryService {
	return &DetectionQueryService{
		alertRepo:     alertRepo,
		blockRepo:     blockRepo,
		sigmaRuleRepo: sigmaRuleRepo,
		toolCallRepo:  toolCallRepo,
	}
}

// DetectionStatistics 检测统计信息
type DetectionStatistics struct {
	TodayAlertCount      int64 `json:"today_alert_count"`
	TodayBlockCount      int64 `json:"today_block_count"`
	AffectedHostCount    int64 `json:"affected_host_count"`
	ActiveSigmaRuleCount int64 `json:"active_sigma_rule_count"`
}

// TrendPoint 趋势数据点
type TrendPoint struct {
	Time  string `json:"time"`
	Count int64  `json:"count"`
}

// GetStatistics 获取检测统计（对齐 Detection.Statistics.Get 工具）
func (s *DetectionQueryService) GetStatistics(ctx context.Context) (*DetectionStatistics, error) {
	stats := &DetectionStatistics{}

	// TODO: 调用各 repository 获取统计数据
	// alertCount, err := s.alertRepo.GetTodayCount(ctx)
	// blockCount, err := s.blockRepo.GetTodayCount(ctx)
	// hostCount, err := s.alertRepo.GetAffectedHostCount(ctx)
	// ruleCount, err := s.sigmaRuleRepo.GetActiveCount(ctx)

	return stats, nil
}

// GetTrend 获取告警趋势（对齐 Detection.Trend.Get 工具）
func (s *DetectionQueryService) GetTrend(ctx context.Context, hours int) ([]TrendPoint, error) {
	// TODO: 调用 alertRepo.GetTrend
	return nil, fmt.Errorf("not implemented")
}

// ListAlerts 查询告警列表（对齐 Detection.Alert.List 工具）
func (s *DetectionQueryService) ListAlerts(ctx context.Context, page, pageSize int, filters map[string]interface{}) ([]model.Alert, int64, error) {
	// TODO: 调用 alertRepo.ListPaginated
	return nil, 0, fmt.Errorf("not implemented")
}

// GetAlert 获取告警详情（对齐 Detection.Alert.Get 工具）
func (s *DetectionQueryService) GetAlert(ctx context.Context, id string) (*model.Alert, error) {
	return s.alertRepo.FindByID(id)
}
