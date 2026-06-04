package service

import (
	"fmt"

	"api-server/internal/model"
	"api-server/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AssetQueryService 资产查询服务
type AssetQueryService struct {
	repo   *repository.AssetCollectionRepository
	logger *zap.Logger
}

// NewAssetQueryService 创建资产查询服务
func NewAssetQueryService(repo *repository.AssetCollectionRepository, logger *zap.Logger) *AssetQueryService {
	return &AssetQueryService{
		repo:   repo,
		logger: logger,
	}
}

// GetSummary 获取资产概览
func (s *AssetQueryService) GetSummary() (*model.AssetSummary, error) {
	return s.repo.GetSummary()
}

// ListSoftwareAssets 列出软件资产
func (s *AssetQueryService) ListSoftwareAssets(query model.SoftwareAssetQuery) ([]model.HostSoftwareAsset, int64, error) {
	return s.repo.GetSoftwareAssets(query)
}

// ListApplicationAssets 列出应用资产
func (s *AssetQueryService) ListApplicationAssets(query model.ApplicationAssetQuery) ([]model.HostApplicationAsset, int64, error) {
	return s.repo.GetApplicationAssets(query)
}

// GetApplicationDetail 获取应用详情
func (s *AssetQueryService) GetApplicationDetail(id uuid.UUID) (*model.HostApplicationAsset, []model.HostApplicationToolCall, error) {
	app, err := s.repo.GetApplicationAsset(id)
	if err != nil {
		return nil, nil, fmt.Errorf("application not found: %w", err)
	}

	toolCalls, err := s.repo.GetToolCallsByApplication(id)
	if err != nil {
		s.logger.Warn("Failed to get tool calls", zap.Error(err))
		toolCalls = []model.HostApplicationToolCall{}
	}

	return app, toolCalls, nil
}

// ReviewApplication 人工复核
func (s *AssetQueryService) ReviewApplication(id uuid.UUID, payload model.ApplicationReviewPayload) error {
	app, err := s.repo.GetApplicationAsset(id)
	if err != nil {
		return fmt.Errorf("application not found: %w", err)
	}

	// 更新人工复核字段
	if payload.Name != "" {
		app.Name = payload.Name
	}
	if payload.Category != "" {
		app.Category = payload.Category
	}
	if payload.Version != "" {
		app.Version = payload.Version
		app.VersionSource = "manual"
	}
	if payload.InstallPath != "" {
		app.InstallPath = payload.InstallPath
	}
	if payload.ConfigPaths != nil {
		app.ConfigPaths = mustMarshalJSON(payload.ConfigPaths)
	}

	app.ReviewStatus = payload.ReviewStatus
	if app.ReviewStatus == "" {
		app.ReviewStatus = "confirmed"
	}

	return s.repo.UpdateApplicationAsset(app)
}

// GetSoftwareByHost 获取主机的软件资产
func (s *AssetQueryService) GetSoftwareByHost(hostID uuid.UUID) ([]model.HostSoftwareAsset, error) {
	return s.repo.GetSoftwareAssetsByHost(hostID)
}

// GetApplicationsByHost 获取主机的应用资产
func (s *AssetQueryService) GetApplicationsByHost(hostID uuid.UUID) ([]model.HostApplicationAsset, error) {
	return s.repo.GetApplicationAssetsByHost(hostID)
}
