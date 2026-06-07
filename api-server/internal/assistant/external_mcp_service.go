package assistant

import (
	"context"
	"fmt"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ExternalMCPSourceService manages external MCP data sources
type ExternalMCPSourceService struct {
	sourceRepo   repository.ExternalMCPSourceRepository
	queryLogRepo repository.ExternalMCPQueryLogRepository
	logger       *zap.Logger
}

// ExternalMCPSourceServiceDeps service dependencies
type ExternalMCPSourceServiceDeps struct {
	SourceRepo   repository.ExternalMCPSourceRepository
	QueryLogRepo repository.ExternalMCPQueryLogRepository
	Logger       *zap.Logger
}

// NewExternalMCPSourceService creates a new ExternalMCPSourceService
func NewExternalMCPSourceService(deps ExternalMCPSourceServiceDeps) *ExternalMCPSourceService {
	return &ExternalMCPSourceService{
		sourceRepo:   deps.SourceRepo,
		queryLogRepo: deps.QueryLogRepo,
		logger:       deps.Logger,
	}
}

// CreateSource creates a new external MCP data source
func (s *ExternalMCPSourceService) CreateSource(ctx context.Context, source *model.ExternalMCPSource) error {
	if source.SourceID == "" {
		source.SourceID = "mcp_" + uuid.New().String()[:8]
	}
	if source.ID == uuid.Nil {
		source.ID = uuid.New()
	}

	if err := s.sourceRepo.Create(ctx, source); err != nil {
		s.logger.Error("failed to create external MCP source",
			zap.String("source_id", source.SourceID),
			zap.String("name", source.Name),
			zap.Error(err),
		)
		return fmt.Errorf("failed to create external MCP source: %w", err)
	}

	s.logger.Info("external MCP source created",
		zap.String("source_id", source.SourceID),
		zap.String("name", source.Name),
		zap.String("source_type", source.SourceType),
	)
	return nil
}

// GetSource retrieves an external MCP source by source_id
func (s *ExternalMCPSourceService) GetSource(ctx context.Context, sourceID string) (*model.ExternalMCPSource, error) {
	source, err := s.sourceRepo.FindBySourceID(ctx, sourceID)
	if err != nil {
		s.logger.Error("failed to find external MCP source",
			zap.String("source_id", sourceID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("external MCP source not found: %w", err)
	}

	s.logger.Debug("external MCP source retrieved", zap.String("source_id", sourceID))
	return source, nil
}

// ListSources lists external MCP sources with filtering and pagination
func (s *ExternalMCPSourceService) ListSources(ctx context.Context, query repository.MCPSourceQuery) ([]model.ExternalMCPSource, int64, error) {
	sources, total, err := s.sourceRepo.List(ctx, query)
	if err != nil {
		s.logger.Error("failed to list external MCP sources", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to list external MCP sources: %w", err)
	}

	// Mask credential_ref in list responses
	for i := range sources {
		sources[i].CredentialRef = ""
	}

	s.logger.Debug("external MCP sources listed",
		zap.Int("count", len(sources)),
		zap.Int64("total", total),
	)
	return sources, total, nil
}

// UpdateSource updates an existing external MCP source
func (s *ExternalMCPSourceService) UpdateSource(ctx context.Context, source *model.ExternalMCPSource) error {
	existing, err := s.sourceRepo.FindBySourceID(ctx, source.SourceID)
	if err != nil {
		s.logger.Error("failed to find external MCP source for update",
			zap.String("source_id", source.SourceID),
			zap.Error(err),
		)
		return fmt.Errorf("external MCP source not found: %w", err)
	}

	// Preserve fields that should not be overwritten
	source.ID = existing.ID
	source.CreatedBy = existing.CreatedBy
	source.CreatedAt = existing.CreatedAt
	source.UpdatedAt = time.Now()

	if err := s.sourceRepo.Update(ctx, source); err != nil {
		s.logger.Error("failed to update external MCP source",
			zap.String("source_id", source.SourceID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to update external MCP source: %w", err)
	}

	s.logger.Info("external MCP source updated",
		zap.String("source_id", source.SourceID),
		zap.String("name", source.Name),
	)
	return nil
}

// DeleteSource deletes an external MCP source by source_id
func (s *ExternalMCPSourceService) DeleteSource(ctx context.Context, sourceID string) error {
	if err := s.sourceRepo.Delete(ctx, sourceID); err != nil {
		s.logger.Error("failed to delete external MCP source",
			zap.String("source_id", sourceID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete external MCP source: %w", err)
	}

	s.logger.Info("external MCP source deleted", zap.String("source_id", sourceID))
	return nil
}

// TestConnection tests the connection to an external MCP source (placeholder)
func (s *ExternalMCPSourceService) TestConnection(ctx context.Context, sourceID string) (*model.MCPConnectionTestResult, error) {
	start := time.Now()

	_, err := s.sourceRepo.FindBySourceID(ctx, sourceID)
	if err != nil {
		_ = s.sourceRepo.UpdateTestStatus(ctx, sourceID, "failed", err.Error())
		s.logger.Error("external MCP source not found for connection test",
			zap.String("source_id", sourceID),
			zap.Error(err),
		)
		return &model.MCPConnectionTestResult{
			SourceID:  sourceID,
			Success:   false,
			LatencyMs: int(time.Since(start).Milliseconds()),
			Message:   "source not found",
		}, nil
	}

	// Placeholder: update test status to success
	_ = s.sourceRepo.UpdateTestStatus(ctx, sourceID, "success", "")

	result := &model.MCPConnectionTestResult{
		SourceID:  sourceID,
		Success:   true,
		LatencyMs: int(time.Since(start).Milliseconds()),
		ToolCount: 0,
		Message:   "connection successful (placeholder)",
	}

	s.logger.Info("external MCP connection test completed (placeholder)",
		zap.String("source_id", sourceID),
		zap.Bool("success", result.Success),
		zap.Int("latency_ms", result.LatencyMs),
	)
	return result, nil
}

// SyncSchema synchronizes the schema from an external MCP source (placeholder)
func (s *ExternalMCPSourceService) SyncSchema(ctx context.Context, sourceID string) (*model.MCPSchemaSyncResult, error) {
	_, err := s.sourceRepo.FindBySourceID(ctx, sourceID)
	if err != nil {
		s.logger.Error("failed to find external MCP source for schema sync",
			zap.String("source_id", sourceID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("external MCP source not found: %w", err)
	}

	result := &model.MCPSchemaSyncResult{
		SourceID:      sourceID,
		SchemaVersion: "v1",
		ToolCount:     0,
		Fields:        []string{},
	}

	s.logger.Info("external MCP schema sync completed (placeholder)",
		zap.String("source_id", sourceID),
		zap.String("schema_version", result.SchemaVersion),
		zap.Int("tool_count", result.ToolCount),
	)
	return result, nil
}

// ListQueryLogs lists query logs for an external MCP source
func (s *ExternalMCPSourceService) ListQueryLogs(ctx context.Context, sourceID string, page, pageSize int) ([]model.ExternalMCPQueryLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	logs, total, err := s.queryLogRepo.ListBySource(ctx, sourceID, page, pageSize)
	if err != nil {
		s.logger.Error("failed to list external MCP query logs",
			zap.String("source_id", sourceID),
			zap.Error(err),
		)
		return nil, 0, fmt.Errorf("failed to list query logs: %w", err)
	}

	s.logger.Debug("external MCP query logs listed",
		zap.String("source_id", sourceID),
		zap.Int("count", len(logs)),
		zap.Int64("total", total),
	)
	return logs, total, nil
}
