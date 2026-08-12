package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrMCPPlatformNotFound = errors.New("mcp platform object not found")

type MCPServerQuery struct {
	Keyword     string
	Environment string
	Status      string
	RiskTier    string
	Page        int
	PageSize    int
}

type MCPOnboardingJobQuery struct {
	Status   string
	Page     int
	PageSize int
}

// MCPInvocationAuditRow contains only invocation metadata plus the service and
// Client identities needed by the governance UI. It deliberately excludes raw
// request and response payloads.
type MCPInvocationAuditRow struct {
	ID             uuid.UUID  `gorm:"column:id"`
	ClientID       *uuid.UUID `gorm:"column:client_id"`
	ToolRevisionID *uuid.UUID `gorm:"column:tool_revision_id"`
	ToolAlias      string     `gorm:"column:tool_alias"`
	Status         string     `gorm:"column:status"`
	PolicyDecision string     `gorm:"column:policy_decision"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	CompletedAt    *time.Time `gorm:"column:completed_at"`
	ClientKey      string     `gorm:"column:client_key"`
	ClientName     string     `gorm:"column:client_name"`
	ServerID       *uuid.UUID `gorm:"column:server_id"`
	ServerName     string     `gorm:"column:server_name"`
}

type MCPToolAuditRow struct {
	model.MCPToolRevision
	ServerID   uuid.UUID `gorm:"column:server_id"`
	ServerName string    `gorm:"column:server_name"`
}

type MCPSecurityVerdictAuditRow struct {
	model.MCPSecurityVerdict
	ClientID   *uuid.UUID `gorm:"column:client_id"`
	ClientKey  string     `gorm:"column:client_key"`
	ClientName string     `gorm:"column:client_name"`
	ServerID   *uuid.UUID `gorm:"column:server_id"`
	ServerName string     `gorm:"column:server_name"`
	ToolAlias  string     `gorm:"column:tool_alias"`
	Status     string     `gorm:"column:invocation_status"`
	CreatedAt  time.Time  `gorm:"column:invocation_created_at"`
}

type MCPRuleMatchRow struct {
	InvocationID uuid.UUID `gorm:"column:invocation_id"`
	RuleName     string    `gorm:"column:rule_name"`
}

type MCPPlatformRepository struct{ db *gorm.DB }

func NewMCPPlatformRepository(db *gorm.DB) *MCPPlatformRepository {
	return &MCPPlatformRepository{db: db}
}

func (r *MCPPlatformRepository) CreateOnboardingJob(ctx context.Context, job *model.MCPOnboardingJob) error {
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *MCPPlatformRepository) FindOnboardingByIdempotency(ctx context.Context, key string) (*model.MCPOnboardingJob, error) {
	var job model.MCPOnboardingJob
	err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *MCPPlatformRepository) GetOnboardingJob(ctx context.Context, id uuid.UUID) (*model.MCPOnboardingJob, error) {
	var job model.MCPOnboardingJob
	err := r.db.WithContext(ctx).First(&job, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *MCPPlatformRepository) UpdateOnboardingJob(ctx context.Context, job *model.MCPOnboardingJob) error {
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		err = r.db.WithContext(ctx).Save(job).Error
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "locked") {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
	return err
}

func (r *MCPPlatformRepository) ListOnboardingJobs(ctx context.Context, q MCPOnboardingJobQuery) ([]model.MCPOnboardingJob, int64, error) {
	var items []model.MCPOnboardingJob
	var total int64
	tx := r.db.WithContext(ctx).Model(&model.MCPOnboardingJob{})
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, size := normalizePage(q.Page, q.PageSize)
	err := tx.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}

func (r *MCPPlatformRepository) CreateServer(ctx context.Context, server *model.MCPServer) error {
	return r.db.WithContext(ctx).Create(server).Error
}

func (r *MCPPlatformRepository) GetServer(ctx context.Context, id uuid.UUID) (*model.MCPServer, error) {
	var server model.MCPServer
	err := r.db.WithContext(ctx).First(&server, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &server, nil
}

func (r *MCPPlatformRepository) ListServers(ctx context.Context, q MCPServerQuery) ([]model.MCPServer, int64, error) {
	var items []model.MCPServer
	var total int64
	tx := r.db.WithContext(ctx).Model(&model.MCPServer{})
	if q.Keyword != "" {
		keyword := "%" + strings.TrimSpace(q.Keyword) + "%"
		tx = tx.Where("display_name ILIKE ? OR server_key ILIKE ? OR endpoint_display ILIKE ?", keyword, keyword, keyword)
	}
	if q.Environment != "" {
		tx = tx.Where("environment = ?", q.Environment)
	}
	if q.Status != "" {
		tx = tx.Where("lifecycle_status = ?", q.Status)
	} else {
		// Retired servers remain queryable by ID and by an explicit status
		// filter for audit/history, but must not reappear in the default
		// operational list after a reversible delete.
		tx = tx.Where("lifecycle_status <> ?", model.MCPPlatformServerRetired)
	}
	if q.RiskTier != "" {
		tx = tx.Where("risk_tier = ?", q.RiskTier)
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, size := normalizePage(q.Page, q.PageSize)
	err := tx.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}

func (r *MCPPlatformRepository) UpdateServer(ctx context.Context, server *model.MCPServer) error {
	server.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Save(server).Error
}

// RetireServer disables a remote service without deleting its immutable
// revisions, catalog releases, grants, credentials, or invocation history.
// Client endpoints bound to the service are revoked in the same transaction so
// no credential remains usable after the service is retired.
func (r *MCPPlatformRepository) RetireServer(ctx context.Context, serverID uuid.UUID, operator string) (int64, error) {
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	rollback := func(err error) (int64, error) {
		_ = tx.Rollback()
		return 0, err
	}

	result := tx.Model(&model.MCPServer{}).
		Where("id = ?", serverID).
		Updates(map[string]interface{}{"lifecycle_status": model.MCPPlatformServerRetired, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return rollback(result.Error)
	}
	if result.RowsAffected == 0 {
		return rollback(ErrMCPPlatformNotFound)
	}

	// A retired server can no longer be approved. Cancel its pending admission
	// reviews in the same transaction so a stale approval cannot publish the
	// retired server again.
	approvalResult := tx.Model(&model.MCPApprovalRequest{}).
		Where("subject_type = ?", "server_revision").
		Where("subject_id IN (?)", tx.Table("mcp_server_revisions").Select("id").Where("server_id = ?", serverID)).
		Where("status = ?", model.MCPPlatformApprovalPending).
		Updates(map[string]interface{}{
			"status":          model.MCPPlatformApprovalCancelled,
			"decided_by":      operator,
			"decision_reason": "remote MCP server retired",
			"decided_at":      time.Now().UTC(),
		})
	if approvalResult.Error != nil {
		return rollback(approvalResult.Error)
	}

	clientIDs := tx.Table("mcp_client_grants AS grant_row").
		Select("DISTINCT grant_row.client_id").
		Joins("JOIN mcp_catalog_releases AS release ON release.catalog_id = grant_row.catalog_id").
		Joins("JOIN mcp_server_revisions AS revision ON revision.id = release.server_revision_id").
		Where("revision.server_id = ?", serverID)
	if err := tx.Model(&model.MCPClientGrant{}).
		Where("client_id IN (?)", clientIDs).
		Where("status <> ?", "revoked").
		Updates(map[string]interface{}{"status": "revoked", "updated_at": time.Now().UTC()}).Error; err != nil {
		return rollback(err)
	}
	if err := tx.Model(&model.MCPClientCredential{}).
		Where("client_id IN (?)", clientIDs).
		Where("status <> ?", "revoked").
		Updates(map[string]interface{}{"status": "revoked", "updated_at": time.Now().UTC()}).Error; err != nil {
		return rollback(err)
	}
	var revokedClients int64
	clientResult := tx.Model(&model.MCPClient{}).
		Where("id IN (?)", clientIDs).
		Where("status <> ?", "revoked").
		Updates(map[string]interface{}{"status": "revoked", "updated_at": time.Now().UTC()})
	if clientResult.Error != nil {
		return rollback(clientResult.Error)
	}
	revokedClients = clientResult.RowsAffected
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return revokedClients, nil
}

func (r *MCPPlatformRepository) CreateServerRevision(ctx context.Context, revision *model.MCPServerRevision) error {
	return r.db.WithContext(ctx).Create(revision).Error
}

func (r *MCPPlatformRepository) GetServerRevision(ctx context.Context, id uuid.UUID) (*model.MCPServerRevision, error) {
	var item model.MCPServerRevision
	err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) ListToolRevisions(ctx context.Context, serverRevisionID *uuid.UUID) ([]model.MCPToolRevision, error) {
	tx := r.db.WithContext(ctx).Model(&model.MCPToolRevision{}).Order("alias ASC")
	if serverRevisionID != nil {
		tx = tx.Where("server_revision_id = ?", *serverRevisionID)
	}
	var items []model.MCPToolRevision
	return items, tx.Find(&items).Error
}

func (r *MCPPlatformRepository) ListToolAuditRows(ctx context.Context, serverRevisionID *uuid.UUID, page, pageSize int) ([]MCPToolAuditRow, int64, error) {
	var items []MCPToolAuditRow
	var total int64
	tx := r.db.WithContext(ctx).Table("mcp_tool_revisions AS tool").
		Joins("JOIN mcp_server_revisions AS revision ON revision.id = tool.server_revision_id").
		Joins("JOIN mcp_servers AS server ON server.id = revision.server_id")
	// Tool revisions are immutable history, but the operational tool catalog
	// must follow the service lifecycle and hide tools of retired services.
	tx = tx.Where("server.lifecycle_status <> ?", model.MCPPlatformServerRetired)
	if serverRevisionID != nil {
		tx = tx.Where("tool.server_revision_id = ?", *serverRevisionID)
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	p, s := normalizePage(page, pageSize)
	err := tx.Select("tool.*, server.id AS server_id, server.display_name AS server_name").
		Order("server.display_name ASC, tool.alias ASC").Offset((p - 1) * s).Limit(s).Scan(&items).Error
	return items, total, err
}

func (r *MCPPlatformRepository) GetToolRevision(ctx context.Context, id uuid.UUID) (*model.MCPToolRevision, error) {
	var item model.MCPToolRevision
	err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) GetCatalog(ctx context.Context, id uuid.UUID) (*model.MCPCatalog, error) {
	var item model.MCPCatalog
	err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) CreateCatalog(ctx context.Context, item *model.MCPCatalog) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *MCPPlatformRepository) CreateCatalogRelease(ctx context.Context, item *model.MCPCatalogRelease) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *MCPPlatformRepository) GetCatalogReleaseByServerRevision(ctx context.Context, revisionID uuid.UUID) (*model.MCPCatalogRelease, error) {
	var item model.MCPCatalogRelease
	err := r.db.WithContext(ctx).Where("server_revision_id = ?", revisionID).Order("release_no DESC").First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) GetActiveCatalogRelease(ctx context.Context, catalogID uuid.UUID) (*model.MCPCatalogRelease, error) {
	var item model.MCPCatalogRelease
	err := r.db.WithContext(ctx).Where("catalog_id = ? AND status = ?", catalogID, "active").Order("release_no DESC").First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) ActivateRelease(ctx context.Context, id uuid.UUID, expectedDigest string) error {
	now := time.Now().UTC()
	tx := r.db.WithContext(ctx).Model(&model.MCPCatalogRelease{}).Where("id = ? AND status = ?", id, "staged")
	if expectedDigest != "" {
		tx = tx.Where("manifest_digest = ?", expectedDigest)
	}
	result := tx.Updates(map[string]interface{}{"status": "active", "published_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrMCPPlatformNotFound
	}
	return r.db.WithContext(ctx).Model(&model.MCPCatalogReleaseTool{}).Where("release_id = ?", id).Update("status", "active").Error
}

func (r *MCPPlatformRepository) CreateCatalogReleaseTools(ctx context.Context, items []model.MCPCatalogReleaseTool) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&items).Error
}

func (r *MCPPlatformRepository) ListCatalogReleaseTools(ctx context.Context, releaseID uuid.UUID) ([]model.MCPCatalogReleaseTool, error) {
	var items []model.MCPCatalogReleaseTool
	err := r.db.WithContext(ctx).Where("release_id = ?", releaseID).Order("display_order ASC, exposed_name ASC").Find(&items).Error
	return items, err
}

func (r *MCPPlatformRepository) NextCatalogReleaseNo(ctx context.Context, catalogID uuid.UUID) (int64, error) {
	var maxNo *int64
	if err := r.db.WithContext(ctx).Model(&model.MCPCatalogRelease{}).Where("catalog_id = ?", catalogID).Select("MAX(release_no)").Scan(&maxNo).Error; err != nil {
		return 0, err
	}
	if maxNo == nil {
		return 1, nil
	}
	return *maxNo + 1, nil
}

func (r *MCPPlatformRepository) CreateApproval(ctx context.Context, item *model.MCPApprovalRequest) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *MCPPlatformRepository) GetApproval(ctx context.Context, id uuid.UUID) (*model.MCPApprovalRequest, error) {
	var item model.MCPApprovalRequest
	err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) UpdateApprovalDecision(ctx context.Context, id uuid.UUID, expectedDigest, status, decidedBy, reason string) (*model.MCPApprovalRequest, error) {
	var item model.MCPApprovalRequest
	tx := r.db.WithContext(ctx).Model(&model.MCPApprovalRequest{}).Where("id = ? AND status = ?", id, model.MCPPlatformApprovalPending)
	if expectedDigest != "" {
		tx = tx.Where("request_digest = ?", expectedDigest)
	}
	now := time.Now().UTC()
	updates := map[string]interface{}{"status": status, "decided_by": decidedBy, "decision_reason": reason, "decided_at": now}
	if result := tx.Updates(updates); result.Error != nil {
		return nil, result.Error
	} else if result.RowsAffected != 1 {
		return nil, ErrMCPPlatformNotFound
	}
	if err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) CreateToolRevisions(ctx context.Context, items []model.MCPToolRevision) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&items).Error
}

func (r *MCPPlatformRepository) CountByTable(ctx context.Context, table string, conditions ...interface{}) (int64, error) {
	var count int64
	query := r.db.WithContext(ctx).Table(table)
	if len(conditions) > 0 {
		query = query.Where(conditions[0], conditions[1:]...)
	}
	err := query.Count(&count).Error
	return count, err
}

func (r *MCPPlatformRepository) CountOperationalServers(ctx context.Context) (int64, error) {
	return r.CountByTable(ctx, "mcp_servers", "lifecycle_status <> ?", model.MCPPlatformServerRetired)
}

func (r *MCPPlatformRepository) CountOperationalPublishedTools(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("mcp_catalog_release_tools AS release_tool").
		Joins("JOIN mcp_catalog_releases AS release ON release.id = release_tool.release_id").
		Joins("JOIN mcp_server_revisions AS revision ON revision.id = release.server_revision_id").
		Joins("JOIN mcp_servers AS server ON server.id = revision.server_id").
		Where("release_tool.status = ? AND server.lifecycle_status <> ?", "active", model.MCPPlatformServerRetired).
		Count(&count).Error
	return count, err
}

func (r *MCPPlatformRepository) CountRecentOperationalHighRiskCalls(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("mcp_security_verdicts AS verdict").
		Joins("JOIN mcp_invocations AS invocation ON invocation.id = verdict.invocation_id").
		Joins("JOIN mcp_tool_revisions AS tool ON tool.id = invocation.tool_revision_id").
		Joins("JOIN mcp_server_revisions AS revision ON revision.id = tool.server_revision_id").
		Joins("JOIN mcp_servers AS server ON server.id = revision.server_id").
		Where("verdict.overall_risk IN ? AND invocation.created_at >= ? AND server.lifecycle_status <> ?", []string{"high", "critical"}, since, model.MCPPlatformServerRetired).
		Count(&count).Error
	return count, err
}

func (r *MCPPlatformRepository) ListCatalogs(ctx context.Context, page, pageSize int) ([]model.MCPCatalog, int64, error) {
	var items []model.MCPCatalog
	var total int64
	tx := r.db.WithContext(ctx).Model(&model.MCPCatalog{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	p, s := normalizePage(page, pageSize)
	err := tx.Order("created_at DESC").Offset((p - 1) * s).Limit(s).Find(&items).Error
	return items, total, err
}

func (r *MCPPlatformRepository) ListClients(ctx context.Context, page, pageSize int) ([]model.MCPClient, int64, error) {
	var items []model.MCPClient
	var total int64
	tx := r.db.WithContext(ctx).Model(&model.MCPClient{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	p, s := normalizePage(page, pageSize)
	err := tx.Order("created_at DESC").Offset((p - 1) * s).Limit(s).Find(&items).Error
	return items, total, err
}

func (r *MCPPlatformRepository) CreateClient(ctx context.Context, item *model.MCPClient) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *MCPPlatformRepository) GetClientByKey(ctx context.Context, key string) (*model.MCPClient, error) {
	var item model.MCPClient
	err := r.db.WithContext(ctx).Where("client_key = ?", key).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) GetClient(ctx context.Context, id uuid.UUID) (*model.MCPClient, error) {
	var item model.MCPClient
	err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) CreateGrant(ctx context.Context, item *model.MCPClientGrant) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *MCPPlatformRepository) GetGrant(ctx context.Context, id uuid.UUID) (*model.MCPClientGrant, error) {
	var item model.MCPClientGrant
	err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) GetActiveGrantByClientID(ctx context.Context, clientID uuid.UUID) (*model.MCPClientGrant, error) {
	var item model.MCPClientGrant
	err := r.db.WithContext(ctx).Where("client_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", clientID, "active", time.Now().UTC()).Order("created_at DESC").First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) UpdateGrant(ctx context.Context, item *model.MCPClientGrant) error {
	item.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Save(item).Error
}

// RevokeClientEndpoint revokes every grant and credential for one Client while
// retaining the records for audit and historical invocation joins.
func (r *MCPPlatformRepository) RevokeClientEndpoint(ctx context.Context, clientID uuid.UUID) (int64, error) {
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	rollback := func(err error) (int64, error) {
		_ = tx.Rollback()
		return 0, err
	}
	var client model.MCPClient
	if err := tx.First(&client, "id = ?", clientID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return rollback(ErrMCPPlatformNotFound)
		}
		return rollback(err)
	}
	now := time.Now().UTC()
	if err := tx.Model(&model.MCPClientGrant{}).Where("client_id = ? AND status <> ?", clientID, "revoked").Updates(map[string]interface{}{"status": "revoked", "updated_at": now}).Error; err != nil {
		return rollback(err)
	}
	if err := tx.Model(&model.MCPClientCredential{}).Where("client_id = ? AND status <> ?", clientID, "revoked").Updates(map[string]interface{}{"status": "revoked", "updated_at": now}).Error; err != nil {
		return rollback(err)
	}
	clientResult := tx.Model(&model.MCPClient{}).Where("id = ? AND status <> ?", clientID, "revoked").Updates(map[string]interface{}{"status": "revoked", "updated_at": now})
	if clientResult.Error != nil {
		return rollback(clientResult.Error)
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return clientResult.RowsAffected, nil
}

func (r *MCPPlatformRepository) CreateClientCredential(ctx context.Context, item *model.MCPClientCredential) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *MCPPlatformRepository) GetActiveClientCredentialByHash(ctx context.Context, tokenHash string) (*model.MCPClientCredential, error) {
	var item model.MCPClientCredential
	err := r.db.WithContext(ctx).Where("token_hash = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", tokenHash, "active", time.Now().UTC()).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) TouchClientCredential(ctx context.Context, id uuid.UUID, at time.Time) error {
	return r.db.WithContext(ctx).Model(&model.MCPClientCredential{}).Where("id = ?", id).Updates(map[string]interface{}{"last_used_at": at, "updated_at": at}).Error
}

func (r *MCPPlatformRepository) CreateInvocation(ctx context.Context, item *model.MCPInvocation) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *MCPPlatformRepository) GetInvocation(ctx context.Context, id uuid.UUID) (*model.MCPInvocation, error) {
	var item model.MCPInvocation
	err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPPlatformNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) UpdateInvocation(ctx context.Context, id uuid.UUID, status, resultDigest string, completedAt *time.Time) error {
	updates := map[string]interface{}{"status": status, "result_digest": resultDigest}
	if completedAt != nil {
		updates["completed_at"] = *completedAt
	}
	return r.db.WithContext(ctx).Model(&model.MCPInvocation{}).Where("id = ?", id).Updates(updates).Error
}

func (r *MCPPlatformRepository) ListApprovals(ctx context.Context, status string, page, pageSize int) ([]model.MCPApprovalRequest, int64, error) {
	var items []model.MCPApprovalRequest
	var total int64
	tx := r.db.WithContext(ctx).Table("mcp_approval_requests AS approval").
		Joins("LEFT JOIN mcp_server_revisions AS approval_revision ON approval_revision.id = approval.subject_id AND approval.subject_type = ?", "server_revision").
		Joins("LEFT JOIN mcp_servers AS approval_server ON approval_server.id = approval_revision.server_id").
		Where("(approval.subject_type <> ? OR approval_server.id IS NULL OR approval_server.lifecycle_status <> ?)", "server_revision", model.MCPPlatformServerRetired)
	if status != "" {
		tx = tx.Where("approval.status = ?", status)
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	p, s := normalizePage(page, pageSize)
	err := tx.Select("approval.*").Order("approval.created_at DESC").Offset((p - 1) * s).Limit(s).Find(&items).Error
	return items, total, err
}

// CountPendingApprovals returns actionable approvals only. Reviews belonging
// to retired services are historical records and must not affect the live
// dashboard counter.
func (r *MCPPlatformRepository) CountPendingApprovals(ctx context.Context) (int64, error) {
	var total int64
	tx := r.db.WithContext(ctx).Table("mcp_approval_requests AS approval").
		Joins("LEFT JOIN mcp_server_revisions AS approval_revision ON approval_revision.id = approval.subject_id AND approval.subject_type = ?", "server_revision").
		Joins("LEFT JOIN mcp_servers AS approval_server ON approval_server.id = approval_revision.server_id").
		Where("approval.status = ?", model.MCPPlatformApprovalPending).
		Where("(approval.subject_type <> ? OR approval_server.id IS NULL OR approval_server.lifecycle_status <> ?)", "server_revision", model.MCPPlatformServerRetired)
	return total, tx.Count(&total).Error
}

func (r *MCPPlatformRepository) ListInvocations(ctx context.Context, page, pageSize int) ([]MCPInvocationAuditRow, int64, error) {
	var items []MCPInvocationAuditRow
	var total int64
	tx := r.db.WithContext(ctx).Table("mcp_invocations AS invocation")
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	p, s := normalizePage(page, pageSize)
	err := tx.Select(`invocation.id, invocation.client_id, invocation.tool_revision_id,
		invocation.tool_alias, invocation.status, invocation.policy_decision,
		invocation.created_at, invocation.completed_at,
		COALESCE(client.client_key, '') AS client_key,
		COALESCE(client.display_name, '') AS client_name,
		server.id AS server_id,
		COALESCE(server.display_name, '') AS server_name`).
		Joins("LEFT JOIN mcp_clients AS client ON client.id = invocation.client_id").
		Joins("LEFT JOIN mcp_tool_revisions AS tool ON tool.id = invocation.tool_revision_id").
		Joins("LEFT JOIN mcp_server_revisions AS revision ON revision.id = tool.server_revision_id").
		Joins("LEFT JOIN mcp_servers AS server ON server.id = revision.server_id").
		Order("invocation.created_at DESC").Offset((p - 1) * s).Limit(s).Scan(&items).Error
	return items, total, err
}

func (r *MCPPlatformRepository) ListSecurityVerdicts(ctx context.Context, page, pageSize int) ([]model.MCPSecurityVerdict, int64, error) {
	var items []model.MCPSecurityVerdict
	var total int64
	tx := r.db.WithContext(ctx).Model(&model.MCPSecurityVerdict{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	p, s := normalizePage(page, pageSize)
	err := tx.Order("updated_at DESC").Offset((p - 1) * s).Limit(s).Find(&items).Error
	return items, total, err
}

func (r *MCPPlatformRepository) ListSecurityVerdictAuditRows(ctx context.Context, page, pageSize int) ([]MCPSecurityVerdictAuditRow, int64, error) {
	var items []MCPSecurityVerdictAuditRow
	var total int64
	tx := r.db.WithContext(ctx).Table("mcp_security_verdicts AS verdict")
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	p, s := normalizePage(page, pageSize)
	err := tx.Select(`verdict.*,
		invocation.client_id, invocation.tool_alias, invocation.status AS invocation_status,
		invocation.created_at AS invocation_created_at,
		COALESCE(client.client_key, '') AS client_key,
		COALESCE(client.display_name, '') AS client_name,
		server.id AS server_id, COALESCE(server.display_name, '') AS server_name`).
		Joins("JOIN mcp_invocations AS invocation ON invocation.id = verdict.invocation_id").
		Joins("LEFT JOIN mcp_clients AS client ON client.id = invocation.client_id").
		Joins("LEFT JOIN mcp_tool_revisions AS tool ON tool.id = invocation.tool_revision_id").
		Joins("LEFT JOIN mcp_server_revisions AS revision ON revision.id = tool.server_revision_id").
		Joins("LEFT JOIN mcp_servers AS server ON server.id = revision.server_id").
		Order("verdict.updated_at DESC").Offset((p - 1) * s).Limit(s).Scan(&items).Error
	return items, total, err
}

func (r *MCPPlatformRepository) ListSecurityRules(ctx context.Context, page, pageSize int) ([]model.MCPRuleDefinition, int64, error) {
	var items []model.MCPRuleDefinition
	var total int64
	tx := r.db.WithContext(ctx).Model(&model.MCPRuleDefinition{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	p, s := normalizePage(page, pageSize)
	err := tx.Order("phase ASC, severity DESC, rule_key ASC").Offset((p - 1) * s).Limit(s).Find(&items).Error
	return items, total, err
}

func (r *MCPPlatformRepository) ListSecurityRuleMatchNames(ctx context.Context, invocationIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	result := make(map[uuid.UUID][]string)
	if len(invocationIDs) == 0 {
		return result, nil
	}
	var rows []MCPRuleMatchRow
	err := r.db.WithContext(ctx).Table("mcp_rule_hits AS hit").
		Select("hit.invocation_id, rule.name AS rule_name").
		Joins("JOIN mcp_rule_definitions AS rule ON rule.id = hit.rule_definition_id").
		Where("hit.invocation_id IN ?", invocationIDs).
		Order("hit.created_at ASC, rule.name ASC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	seen := make(map[uuid.UUID]map[string]struct{})
	for _, row := range rows {
		if _, ok := seen[row.InvocationID]; !ok {
			seen[row.InvocationID] = make(map[string]struct{})
		}
		if _, ok := seen[row.InvocationID][row.RuleName]; ok {
			continue
		}
		seen[row.InvocationID][row.RuleName] = struct{}{}
		result[row.InvocationID] = append(result[row.InvocationID], row.RuleName)
	}
	return result, nil
}

func (r *MCPPlatformRepository) ListEnabledSecurityRules(ctx context.Context, phase string) ([]model.MCPRuleDefinition, error) {
	var items []model.MCPRuleDefinition
	err := r.db.WithContext(ctx).Where("enabled = ? AND phase = ?", true, phase).Order("severity DESC, rule_key ASC").Find(&items).Error
	return items, err
}

func (r *MCPPlatformRepository) SetSecurityRuleEnabled(ctx context.Context, id uuid.UUID, enabled bool) (*model.MCPRuleDefinition, error) {
	result := r.db.WithContext(ctx).Model(&model.MCPRuleDefinition{}).Where("id = ?", id).Update("enabled", enabled)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrMCPPlatformNotFound
	}
	var item model.MCPRuleDefinition
	if err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MCPPlatformRepository) SaveSecurityEvaluation(ctx context.Context, invocationID uuid.UUID, invocationStatus, ruleStatus, aiStatus, resultDigest string, completedAt time.Time, hits []model.MCPRuleHit, verdict *model.MCPSecurityVerdict) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{"status": invocationStatus, "rule_status": ruleStatus, "ai_status": aiStatus, "result_digest": resultDigest, "completed_at": completedAt}
		if err := tx.Model(&model.MCPInvocation{}).Where("id = ?", invocationID).Updates(updates).Error; err != nil {
			return err
		}
		if len(hits) > 0 {
			if err := tx.Create(&hits).Error; err != nil {
				return err
			}
		}
		return tx.Where("invocation_id = ?", invocationID).Assign(*verdict).FirstOrCreate(verdict).Error
	})
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
