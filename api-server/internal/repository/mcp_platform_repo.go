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
	tx := r.db.WithContext(ctx).Model(&model.MCPApprovalRequest{})
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	p, s := normalizePage(page, pageSize)
	err := tx.Order("created_at DESC").Offset((p - 1) * s).Limit(s).Find(&items).Error
	return items, total, err
}

func (r *MCPPlatformRepository) ListInvocations(ctx context.Context, page, pageSize int) ([]model.MCPInvocation, int64, error) {
	var items []model.MCPInvocation
	var total int64
	tx := r.db.WithContext(ctx).Model(&model.MCPInvocation{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	p, s := normalizePage(page, pageSize)
	err := tx.Order("created_at DESC").Offset((p - 1) * s).Limit(s).Find(&items).Error
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
