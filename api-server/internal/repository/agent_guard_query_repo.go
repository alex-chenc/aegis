package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrAgentGuardHostNotFound          = errors.New("agent guard host not found")
	ErrAgentGuardInstanceNotFound      = errors.New("agent runtime instance not found")
	ErrAgentGuardSessionNotFound       = errors.New("agent behavior session not found")
	ErrAgentGuardExecutionUnitNotFound = errors.New("agent execution unit not found")
	ErrAgentGuardBehaviorNotFound      = errors.New("agent behavior event not found")
	ErrAgentGuardFindingNotFound       = errors.New("agent security finding not found")
	ErrAgentGuardAnalysisNotFound      = errors.New("agent security analysis not found")
	ErrAgentGuardActionNotFound        = errors.New("agent guard action not found")
)

const (
	agentGuardProcessFactRepositoryLimit = 5000
	// Runtime controllers heartbeat every 30 seconds. Three intervals avoid a
	// transient false stop while removing dead PID epochs from the live view
	// quickly enough for the selector to reflect the host's current process set.
	agentGuardRuntimeFreshnessWindow = 90 * time.Second
)

type AgentGuardQueryRepository struct {
	db *gorm.DB
}

type agentGuardScopeRow struct {
	AssetID     *uuid.UUID
	HostID      uuid.UUID
	AgentType   string
	ProfileKey  string
	DisplayName string
	Hostname    string
	IPAddress   string
}

func NewAgentGuardQueryRepository(db *gorm.DB) *AgentGuardQueryRepository {
	return &AgentGuardQueryRepository{db: db}
}

func (r *AgentGuardQueryRepository) ListAgents(
	ctx context.Context,
	query model.AgentGuardAgentQuery,
) ([]model.AgentGuardAgentSummary, int64, error) {
	page, pageSize := normalizeAgentGuardPage(query.Page, query.PageSize)
	offset := (page - 1) * pageSize

	assets := r.db.WithContext(ctx).
		Table("host_application_assets AS a").
		Joins("JOIN hosts AS h ON h.id = a.host_id").
		Where("a.category = ? AND a.status <> ?", "ai_agent", "deleted")
	assets = applyAgentGuardAssetFilters(assets, query)
	assetTypeExpr := agentGuardCanonicalTypeSQL("COALESCE(NULLIF(a.runtime_name, ''), NULLIF(a.name, ''), 'unknown')")
	assetScopes := assets.Session(&gorm.Session{}).
		Select(fmt.Sprintf(`a.host_id, %s AS agent_type,
			MAX(COALESCE(NULLIF(a.display_name, ''), NULLIF(a.name, ''), 'Unknown Agent')) AS display_name,
			h.hostname, h.ip_address`, assetTypeExpr)).
		Group("a.host_id, " + assetTypeExpr + ", h.hostname, h.ip_address")
	var assetCount int64
	if err := r.db.WithContext(ctx).Table("(?) AS logical_agent_assets", assetScopes).
		Count(&assetCount).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent assets: %w", err)
	}

	runtimeTypeExpr := agentGuardCanonicalTypeSQL("i.agent_type")
	orphans := r.db.WithContext(ctx).
		Table("agent_runtime_instances AS i").
		Select(fmt.Sprintf(`i.host_id, %s AS agent_type, MAX(i.profile_key) AS profile_key,
			MAX(COALESCE(NULLIF(i.display_name, ''), i.agent_type)) AS display_name,
			h.hostname, h.ip_address`, runtimeTypeExpr)).
		Joins("JOIN hosts AS h ON h.id = i.host_id").
		Where("i.detection_confidence = ?", "confirmed").
		Where(fmt.Sprintf(`NOT EXISTS (
			SELECT 1 FROM host_application_assets a
			WHERE a.host_id = i.host_id AND a.category = 'ai_agent' AND a.status <> 'deleted'
			  AND %s = %s
		)`, agentGuardCanonicalTypeSQL("COALESCE(NULLIF(a.runtime_name, ''), NULLIF(a.name, ''), 'unknown')"), runtimeTypeExpr))
	orphans = applyAgentGuardOrphanFilters(orphans, query).
		Group("i.host_id, " + runtimeTypeExpr + ", h.hostname, h.ip_address")
	var orphanCount int64
	if err := r.db.WithContext(ctx).Table("(?) AS orphan_agent_scopes", orphans).
		Count(&orphanCount).Error; err != nil {
		return nil, 0, fmt.Errorf("count assetless agent scopes: %w", err)
	}

	total := assetCount + orphanCount
	if int64(offset) >= total {
		return []model.AgentGuardAgentSummary{}, total, nil
	}

	rows := make([]agentGuardScopeRow, 0, pageSize)
	remaining := pageSize
	if int64(offset) < assetCount {
		var assetRows []agentGuardScopeRow
		assetOffset := offset
		assetLimit := remaining
		if available := int(assetCount) - assetOffset; assetLimit > available {
			assetLimit = available
		}
		if assetLimit > 0 {
			err := assetScopes.
				Order("h.hostname ASC, display_name ASC, agent_type ASC").
				Offset(assetOffset).
				Limit(assetLimit).
				Scan(&assetRows).Error
			if err != nil {
				return nil, 0, fmt.Errorf("list agent assets: %w", err)
			}
			for index := range assetRows {
				asset, selectErr := r.selectLogicalAgentAsset(ctx, assets, assetRows[index].HostID, assetRows[index].AgentType)
				if selectErr != nil {
					return nil, 0, selectErr
				}
				assetRows[index].AssetID = &asset.ID
				assetRows[index].DisplayName = asset.DisplayName
			}
			rows = append(rows, assetRows...)
			remaining -= len(assetRows)
		}
	}
	if remaining > 0 {
		orphanOffset := offset - int(assetCount)
		if orphanOffset < 0 {
			orphanOffset = 0
		}
		var orphanRows []agentGuardScopeRow
		if err := orphans.
			Order("h.hostname ASC, display_name ASC, agent_type ASC").
			Offset(orphanOffset).
			Limit(remaining).
			Scan(&orphanRows).Error; err != nil {
			return nil, 0, fmt.Errorf("list assetless agent scopes: %w", err)
		}
		rows = append(rows, orphanRows...)
	}

	items := make([]model.AgentGuardAgentSummary, 0, len(rows))
	for _, row := range rows {
		item, err := r.enrichAgentGuardScope(ctx, row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

func (r *AgentGuardQueryRepository) GetOverview(ctx context.Context) (*model.AgentGuardOverview, error) {
	since := time.Now().UTC().Add(-24 * time.Hour)
	overview := &model.AgentGuardOverview{
		Coverage:           map[string]int64{},
		Behaviors24h:       map[string]int64{},
		Findings24h:        map[string]int64{},
		BuiltinRuleHits24h: map[string]int64{},
		PolicyHosts:        map[string]int64{},
	}
	if err := r.db.WithContext(ctx).Model(&model.AgentRuntimeInstance{}).
		Where("status = ? AND last_seen_at >= ?", "running", agentGuardRuntimeFreshnessCutoff()).
		Count(&overview.RunningInstances).Error; err != nil {
		return nil, fmt.Errorf("count running agent instances: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.AgentExecutionUnit{}).
		Where("status NOT IN ?", []string{"stopped", "terminated"}).
		Count(&overview.ExecutionUnits).Error; err != nil {
		return nil, fmt.Errorf("count agent execution units: %w", err)
	}
	if err := r.groupAgentGuardCounts(
		ctx,
		&model.AgentRuntimeInstance{},
		"coverage_level",
		"",
		nil,
		overview.Coverage,
	); err != nil {
		return nil, fmt.Errorf("group agent coverage: %w", err)
	}
	if err := r.groupAgentGuardCounts(
		ctx,
		&model.AgentBehaviorEvent{},
		"category",
		"occurred_at >= ?",
		[]any{since},
		overview.Behaviors24h,
	); err != nil {
		return nil, fmt.Errorf("group agent behaviors: %w", err)
	}
	var denied int64
	if err := r.db.WithContext(ctx).Model(&model.AgentBehaviorEvent{}).
		Where("occurred_at >= ? AND decision = ?", since, "deny").
		Count(&denied).Error; err != nil {
		return nil, fmt.Errorf("count denied agent behaviors: %w", err)
	}
	overview.Behaviors24h["denied"] = denied
	var frozen int64
	if err := r.db.WithContext(ctx).Model(&model.AgentGuardAction{}).
		Where(
			"requested_at >= ? AND action IN ? AND status = ?",
			since,
			[]string{model.AgentGuardActionFreezeExecutionUnit, model.AgentGuardActionHoldExecutionUnit},
			"success",
		).
		Count(&frozen).Error; err != nil {
		return nil, fmt.Errorf("count frozen agent actions: %w", err)
	}
	overview.Behaviors24h["frozen"] = frozen
	if err := r.groupAgentGuardCounts(
		ctx,
		&model.AgentSecurityFinding{},
		"verdict",
		"last_observed_at >= ?",
		[]any{since},
		overview.Findings24h,
	); err != nil {
		return nil, fmt.Errorf("group agent findings: %w", err)
	}
	var analysisPending int64
	if err := r.db.WithContext(ctx).Model(&model.AgentSecurityAnalysisRun{}).
		Where(
			"queued_at >= ? AND status IN ?",
			since,
			[]string{model.AgentGuardAnalysisStatusPending, model.AgentGuardAnalysisStatusRunning},
		).
		Count(&analysisPending).Error; err != nil {
		return nil, fmt.Errorf("count pending agent analyses: %w", err)
	}
	overview.Findings24h["analysis_pending"] = analysisPending
	if err := r.groupAgentGuardCounts(
		ctx,
		&model.AgentBehaviorEvent{},
		"rule_id",
		"occurred_at >= ? AND rule_id LIKE ?",
		[]any{since, "AGB-BUILTIN-%"},
		overview.BuiltinRuleHits24h,
	); err != nil {
		return nil, fmt.Errorf("group builtin agent rule hits: %w", err)
	}
	type deliveryStatusCount struct {
		Status string
		Count  int64
	}
	var deliveryCounts []deliveryStatusCount
	latestDelivery := r.db.WithContext(ctx).
		Model(&model.AgentGuardPolicyDelivery{}).
		Select("host_id, MAX(bundle_version) AS bundle_version").
		Group("host_id")
	if err := r.db.WithContext(ctx).
		Table("agent_guard_policy_deliveries AS d").
		Select("d.status, COUNT(*) AS count").
		Joins("JOIN (?) AS latest ON latest.host_id = d.host_id AND latest.bundle_version = d.bundle_version", latestDelivery).
		Group("d.status").
		Scan(&deliveryCounts).Error; err != nil {
		return nil, fmt.Errorf("group latest agent policy deliveries: %w", err)
	}
	for _, row := range deliveryCounts {
		overview.PolicyHosts[row.Status] = row.Count
	}
	return overview, nil
}

func (r *AgentGuardQueryRepository) GetCoverage(
	ctx context.Context,
	query model.AgentRuntimeInstanceQuery,
) ([]model.AgentGuardCoverageSummary, int64, error) {
	type coverageRow struct {
		HostID          uuid.UUID
		AgentType       string
		ProfileKey      string
		IsolationType   string
		CoverageLevel   string
		CoverageReasons datatypes.JSON
		InstanceCount   int64
		UnitCount       int64
	}
	base := r.db.WithContext(ctx).
		Table("agent_runtime_instances AS i").
		Select(`i.host_id, i.agent_type, i.profile_key,
			COALESCE(NULLIF(u.unit_type, ''), 'none') AS isolation_type,
			i.coverage_level, i.coverage_reasons,
			COUNT(DISTINCT i.id) AS instance_count,
			COUNT(DISTINCT u.id) AS unit_count`).
		Joins("LEFT JOIN agent_execution_units AS u ON u.instance_id = i.id")
	base = applyAgentRuntimeInstanceFilters(base, query)
	if query.IsolationType != "" {
		base = base.Where("u.unit_type = ?", query.IsolationType)
	}
	base = base.Group(
		"i.host_id, i.agent_type, i.profile_key, COALESCE(NULLIF(u.unit_type, ''), 'none'), i.coverage_level, i.coverage_reasons",
	)
	var total int64
	if err := r.db.WithContext(ctx).Table("(?) AS coverage_groups", base).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent coverage groups: %w", err)
	}
	page, pageSize := normalizeAgentGuardPage(query.Page, query.PageSize)
	var rows []coverageRow
	if err := base.Order("i.host_id ASC, i.agent_type ASC, isolation_type ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list agent coverage: %w", err)
	}
	items := make([]model.AgentGuardCoverageSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, model.AgentGuardCoverageSummary{
			HostID:          row.HostID,
			AgentType:       row.AgentType,
			ProfileKey:      row.ProfileKey,
			IsolationType:   row.IsolationType,
			CoverageLevel:   row.CoverageLevel,
			CoverageReasons: decodeAgentGuardStringArray(row.CoverageReasons),
			InstanceCount:   row.InstanceCount,
			UnitCount:       row.UnitCount,
		})
	}
	return items, total, nil
}

func (r *AgentGuardQueryRepository) GetHostStatus(
	ctx context.Context,
	hostID uuid.UUID,
) (*model.AgentGuardHostStatus, error) {
	var hostCount int64
	if err := r.db.WithContext(ctx).Model(&model.Host{}).
		Where("id = ?", hostID).
		Count(&hostCount).Error; err != nil {
		return nil, fmt.Errorf("check agent guard host: %w", err)
	}
	if hostCount == 0 {
		return nil, ErrAgentGuardHostNotFound
	}
	status := &model.AgentGuardHostStatus{
		HostID:          hostID,
		CoverageLevel:   model.AgentGuardCoverageUnsupportedProfile,
		CoverageReasons: []string{"bundle_not_reported"},
	}
	var delivery model.AgentGuardPolicyDelivery
	err := r.db.WithContext(ctx).
		Where("host_id = ?", hostID).
		Order("bundle_version DESC").
		First(&delivery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return status, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest agent guard delivery: %w", err)
	}
	status.LatestDelivery = &delivery
	status.CapabilitySnapshot = append(json.RawMessage(nil), delivery.CapabilitySnapshot...)
	status.CoverageLevel = delivery.CoverageLevel
	if status.CoverageLevel == "" {
		status.CoverageLevel = model.AgentGuardCoverageDegraded
	}
	status.CoverageReasons = nil
	if delivery.Status != "applied" {
		status.CoverageReasons = []string{"bundle_" + delivery.Status}
	}
	status.ErrorCode = delivery.ErrorCode
	status.ErrorMessage = delivery.ErrorMessage
	return status, nil
}

func (r *AgentGuardQueryRepository) ListInstances(
	ctx context.Context,
	query model.AgentRuntimeInstanceQuery,
) ([]model.AgentRuntimeInstance, int64, error) {
	var (
		items []model.AgentRuntimeInstance
		total int64
	)
	db := applyAgentRuntimeInstanceFilters(
		r.db.WithContext(ctx).Model(&model.AgentRuntimeInstance{}),
		query,
	)
	if query.IsolationType != "" {
		db = db.Where(
			"EXISTS (SELECT 1 FROM agent_execution_units u WHERE u.instance_id = agent_runtime_instances.id AND u.unit_type = ?)",
			query.IsolationType,
		)
	}
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent runtime instances: %w", err)
	}
	if err := findAgentGuardPage(
		db.Order("last_seen_at DESC, id ASC"),
		query.Page,
		query.PageSize,
		&items,
	); err != nil {
		return nil, 0, fmt.Errorf("list agent runtime instances: %w", err)
	}
	return items, total, nil
}

func (r *AgentGuardQueryRepository) GetInstance(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentRuntimeInstance, error) {
	var item model.AgentRuntimeInstance
	return &item, r.getAgentGuardByID(ctx, &item, id, ErrAgentGuardInstanceNotFound)
}

func (r *AgentGuardQueryRepository) ListSessions(
	ctx context.Context,
	query model.AgentBehaviorSessionQuery,
) ([]model.AgentBehaviorSession, int64, error) {
	var (
		items []model.AgentBehaviorSession
		total int64
	)
	db := r.db.WithContext(ctx).Model(&model.AgentBehaviorSession{})
	db = applyAgentGuardEqual(db, "instance_id", query.InstanceID)
	db = applyAgentGuardEqual(db, "execution_unit_id", query.ExecutionUnitID)
	db = applyAgentGuardEqual(db, "status", query.Status)
	db = applyAgentGuardEqual(db, "source", query.Source)
	if query.TrustedOnly {
		db = db.Where(
			"source IN ? AND confidence = ? AND COALESCE(external_session_id, '') <> ''",
			[]string{
				model.AgentGuardSessionSourceAgentOfficial,
				model.AgentGuardSessionSourceAdapterHook,
				model.AgentGuardSessionSourceAegisWrapper,
			},
			"confirmed",
		)
	}
	if query.PreferTrusted {
		db = db.Where(`source <> 'activity_window' OR NOT EXISTS (
			SELECT 1 FROM agent_behavior_sessions trusted
			WHERE trusted.instance_id = agent_behavior_sessions.instance_id
			AND trusted.source IN ('agent_official', 'adapter_hook', 'aegis_wrapper')
		)`)
	}
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent behavior sessions: %w", err)
	}
	if err := findAgentGuardPage(
		db.Order("last_seen_at DESC, id ASC"),
		query.Page,
		query.PageSize,
		&items,
	); err != nil {
		return nil, 0, fmt.Errorf("list agent behavior sessions: %w", err)
	}
	return items, total, nil
}

func (r *AgentGuardQueryRepository) GetSession(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentBehaviorSession, error) {
	var item model.AgentBehaviorSession
	return &item, r.getAgentGuardByID(ctx, &item, id, ErrAgentGuardSessionNotFound)
}

// DeleteSessions removes the selected trusted sessions and the session-owned
// behavior/finding records in one transaction. A finding without its session
// would otherwise reappear in the instance-wide analysis view, which is the
// exact data leak the session selector is intended to prevent.
func (r *AgentGuardQueryRepository) DeleteSessions(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			DELETE FROM agent_security_analysis_runs
			WHERE finding_id IN (
				SELECT id FROM agent_security_findings WHERE session_id IN ?
			)`, ids).Error; err != nil {
			return fmt.Errorf("delete agent security analyses for sessions: %w", err)
		}
		if err := tx.Model(&model.AgentSecurityFinding{}).
			Where("session_id IN ?", ids).Delete(&model.AgentSecurityFinding{}).Error; err != nil {
			return fmt.Errorf("delete agent security findings for sessions: %w", err)
		}
		if err := tx.Model(&model.AgentBehaviorEvent{}).
			Where("session_id IN ?", ids).Delete(&model.AgentBehaviorEvent{}).Error; err != nil {
			return fmt.Errorf("delete agent behavior events for sessions: %w", err)
		}
		if err := tx.Model(&model.AgentBehaviorSession{}).
			Where("id IN ?", ids).Delete(&model.AgentBehaviorSession{}).Error; err != nil {
			return fmt.Errorf("delete agent behavior sessions: %w", err)
		}
		return nil
	})
}

func (r *AgentGuardQueryRepository) ListExecutionUnits(
	ctx context.Context,
	query model.AgentExecutionUnitQuery,
) ([]model.AgentExecutionUnit, int64, error) {
	var (
		items []model.AgentExecutionUnit
		total int64
	)
	db := r.db.WithContext(ctx).Model(&model.AgentExecutionUnit{})
	db = applyAgentGuardEqual(db, "host_id", query.HostID)
	db = applyAgentGuardEqual(db, "instance_id", query.InstanceID)
	db = applyAgentGuardEqual(db, "unit_type", query.UnitType)
	db = applyAgentGuardEqual(db, "status", query.Status)
	db = applyAgentGuardEqual(db, "coverage_level", query.Coverage)
	db = applyAgentGuardEqual(db, "container_id", query.ContainerID)
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent execution units: %w", err)
	}
	if err := findAgentGuardPage(
		db.Order("last_seen_at DESC, id ASC"),
		query.Page,
		query.PageSize,
		&items,
	); err != nil {
		return nil, 0, fmt.Errorf("list agent execution units: %w", err)
	}
	return items, total, nil
}

func (r *AgentGuardQueryRepository) GetExecutionUnit(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentExecutionUnit, error) {
	var item model.AgentExecutionUnit
	return &item, r.getAgentGuardByID(ctx, &item, id, ErrAgentGuardExecutionUnitNotFound)
}

func (r *AgentGuardQueryRepository) ListBehaviors(
	ctx context.Context,
	query model.AgentBehaviorEventQuery,
) ([]model.AgentBehaviorEvent, int64, error) {
	var (
		items []model.AgentBehaviorEvent
		total int64
	)
	db := r.db.WithContext(ctx).Model(&model.AgentBehaviorEvent{})
	for _, item := range []struct{ column, value string }{
		{"host_id", query.HostID},
		{"agent_type", query.AgentType},
		{"instance_id", query.InstanceID},
		{"session_id", query.SessionID},
		{"execution_unit_id", query.ExecutionUnitID},
		{"category", query.Category},
		{"operation", query.Operation},
		{"outcome", query.Outcome},
		{"resource_type", query.ResourceType},
		{"resource_classification", query.ResourceClassification},
		{"decision", query.Decision},
		{"severity", query.Severity},
		{"rule_id", query.RuleID},
		{"policy_id", query.PolicyID},
	} {
		db = applyAgentGuardEqual(db, item.column, item.value)
	}
	if query.ResourceKeyword != "" {
		db = db.Where("LOWER(resource_identity) LIKE ?", "%"+strings.ToLower(query.ResourceKeyword)+"%")
	}
	if query.PID != nil {
		db = db.Where("pid = ?", *query.PID)
	}
	if query.ProcessStartTicks != "" {
		db = db.Where("process_start_ticks = ?", query.ProcessStartTicks)
	}
	db = applyAgentGuardTimeRange(db, "occurred_at", query.StartTime, query.EndTime)
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent behavior events: %w", err)
	}
	if err := findAgentGuardPage(
		db.Order("occurred_at DESC, agent_sequence DESC, id ASC"),
		query.Page,
		query.PageSize,
		&items,
	); err != nil {
		return nil, 0, fmt.Errorf("list agent behavior events: %w", err)
	}
	return items, total, nil
}

func (r *AgentGuardQueryRepository) ListProcessFacts(
	ctx context.Context,
	query model.AgentBehaviorEventQuery,
	limit int,
) ([]model.AgentBehaviorEvent, int64, error) {
	query.Category = "process"
	query.Page, query.PageSize = 1, 100
	db := r.db.WithContext(ctx).Model(&model.AgentBehaviorEvent{}).
		Where("category = 'process' AND pid IS NOT NULL AND process_start_ticks IS NOT NULL")
	for _, item := range []struct{ column, value string }{
		{"host_id", query.HostID}, {"instance_id", query.InstanceID},
		{"session_id", query.SessionID}, {"execution_unit_id", query.ExecutionUnitID},
	} {
		db = applyAgentGuardEqual(db, item.column, item.value)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent process facts: %w", err)
	}
	if limit <= 0 || limit > agentGuardProcessFactRepositoryLimit {
		limit = agentGuardProcessFactRepositoryLimit
	}
	var items []model.AgentBehaviorEvent
	if err := db.Order("occurred_at ASC, agent_sequence ASC, id ASC").Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list agent process facts: %w", err)
	}
	return items, total, nil
}

func (r *AgentGuardQueryRepository) GetBehavior(
	ctx context.Context,
	eventID string,
) (*model.AgentBehaviorEvent, error) {
	var item model.AgentBehaviorEvent
	db := r.db.WithContext(ctx).Where("raw_event_id = ?", eventID)
	if parsed, err := uuid.Parse(eventID); err == nil {
		db = db.Or("id = ?", parsed)
	}
	err := db.First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentGuardBehaviorNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get agent behavior event: %w", err)
	}
	return &item, nil
}

func (r *AgentGuardQueryRepository) GetRuntimeEvent(
	ctx context.Context,
	eventID string,
) (*model.RuntimeEvent, error) {
	var item model.RuntimeEvent
	db := r.db.WithContext(ctx).Where("event_id = ?", eventID)
	if parsed, err := uuid.Parse(eventID); err == nil {
		db = db.Or("id = ?", parsed)
	}
	err := db.First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentGuardBehaviorNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get runtime event: %w", err)
	}
	return &item, nil
}

func (r *AgentGuardQueryRepository) GetRawBehavior(
	ctx context.Context,
	eventID string,
) (*model.RuntimeEvent, error) {
	behavior, err := r.GetBehavior(ctx, eventID)
	if err != nil {
		return nil, err
	}
	var raw model.RuntimeEvent
	err = r.db.WithContext(ctx).
		Where("event_id = ?", behavior.RawEventID).
		First(&raw).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentGuardBehaviorNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get raw agent behavior event: %w", err)
	}
	return &raw, nil
}

func (r *AgentGuardQueryRepository) ListFindings(
	ctx context.Context,
	query model.AgentSecurityFindingQuery,
) ([]model.AgentSecurityFinding, int64, error) {
	var (
		items []model.AgentSecurityFinding
		total int64
	)
	db := r.db.WithContext(ctx).Model(&model.AgentSecurityFinding{})
	for _, item := range []struct{ column, value string }{
		{"host_id", query.HostID},
		{"instance_id", query.InstanceID},
		{"session_id", query.SessionID},
		{"execution_unit_id", query.ExecutionUnitID},
		{"severity", query.Severity},
		{"verdict", query.Verdict},
		{"status", query.Status},
	} {
		db = applyAgentGuardEqual(db, item.column, item.value)
	}
	if len(query.InstanceIDs) > 0 {
		db = db.Where("instance_id IN ?", query.InstanceIDs)
	}
	switch query.FindingDomain {
	case model.AgentSecurityFindingDomainTool:
		// Tool findings are owned by api-server and use a stable key prefix so
		// behavior-session analysis cannot include OS escape findings or the
		// historical DC single-event findings.
		db = db.Where("finding_key LIKE ?", "tool-command:v1:%")
	case model.AgentSecurityFindingDomainEscape:
		// Escape findings are permission-scoped v2 records. Legacy v1 rows are
		// removed by the v6.2 migration and must never leak into the new detail.
		db = db.Where("finding_key LIKE ?", "escape:v2:%")
	}
	if query.AssetID != "" || query.AgentType != "" || query.ProfileKey != "" {
		instanceScope := r.db.WithContext(ctx).
			Model(&model.AgentRuntimeInstance{}).
			Select("id")
		instanceScope = applyAgentGuardEqual(instanceScope, "asset_id", query.AssetID)
		instanceScope = applyAgentGuardEqual(instanceScope, "agent_type", query.AgentType)
		instanceScope = applyAgentGuardEqual(instanceScope, "profile_key", query.ProfileKey)
		db = db.Where("instance_id IN (?)", instanceScope)
	}
	if query.ConfidenceMin != nil {
		db = db.Where("confidence >= ?", *query.ConfidenceMin)
	}
	if query.RuleID != "" {
		if r.db.Dialector.Name() == "postgres" {
			db = db.Where("rule_hits @> ?::jsonb", fmt.Sprintf("[%q]", query.RuleID))
		} else {
			db = db.Where(
				"EXISTS (SELECT 1 FROM json_each(agent_security_findings.rule_hits) WHERE json_each.value = ?)",
				query.RuleID,
			)
		}
	}
	if query.AnalysisStatus != "" {
		db = db.Where(
			`EXISTS (
				SELECT 1 FROM agent_security_analysis_runs ar
				WHERE ar.finding_id = agent_security_findings.id AND ar.status = ?
			)`,
			query.AnalysisStatus,
		)
	}
	if query.Handled != nil {
		if *query.Handled {
			db = db.Where("handled_at IS NOT NULL")
		} else {
			db = db.Where("handled_at IS NULL")
		}
	}
	db = applyAgentGuardTimeRange(db, "last_observed_at", query.StartTime, query.EndTime)
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent security findings: %w", err)
	}
	if err := findAgentGuardPage(
		db.Order("last_observed_at DESC, id ASC"),
		query.Page,
		query.PageSize,
		&items,
	); err != nil {
		return nil, 0, fmt.Errorf("list agent security findings: %w", err)
	}
	return items, total, nil
}

func (r *AgentGuardQueryRepository) GetFinding(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentSecurityFinding, error) {
	var item model.AgentSecurityFinding
	return &item, r.getAgentGuardByID(ctx, &item, id, ErrAgentGuardFindingNotFound)
}

func (r *AgentGuardQueryRepository) ListAnalyses(
	ctx context.Context,
	query model.AgentSecurityAnalysisQuery,
) ([]model.AgentSecurityAnalysisRun, int64, error) {
	var (
		items []model.AgentSecurityAnalysisRun
		total int64
	)
	db := r.db.WithContext(ctx).Model(&model.AgentSecurityAnalysisRun{})
	db = applyAgentGuardEqual(db, "finding_id", query.FindingID)
	db = applyAgentGuardEqual(db, "status", query.Status)
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent security analyses: %w", err)
	}
	if err := findAgentGuardPage(
		db.Order("attempt DESC, id ASC"),
		query.Page,
		query.PageSize,
		&items,
	); err != nil {
		return nil, 0, fmt.Errorf("list agent security analyses: %w", err)
	}
	return items, total, nil
}

func (r *AgentGuardQueryRepository) GetAnalysis(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentSecurityAnalysisRun, error) {
	var item model.AgentSecurityAnalysisRun
	return &item, r.getAgentGuardByID(ctx, &item, id, ErrAgentGuardAnalysisNotFound)
}

func (r *AgentGuardQueryRepository) ListActions(
	ctx context.Context,
	query model.AgentGuardActionQuery,
) ([]model.AgentGuardAction, int64, error) {
	var (
		items []model.AgentGuardAction
		total int64
	)
	db := r.db.WithContext(ctx).Model(&model.AgentGuardAction{})
	for _, item := range []struct{ column, value string }{
		{"host_id", query.HostID},
		{"instance_id", query.InstanceID},
		{"execution_unit_id", query.ExecutionUnitID},
		{"action", query.Action},
		{"status", query.Status},
		{"source", query.Source},
	} {
		db = applyAgentGuardEqual(db, item.column, item.value)
	}
	db = applyAgentGuardTimeRange(db, "requested_at", query.StartTime, query.EndTime)
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent guard actions: %w", err)
	}
	if err := findAgentGuardPage(
		db.Order("requested_at DESC, id ASC"),
		query.Page,
		query.PageSize,
		&items,
	); err != nil {
		return nil, 0, fmt.Errorf("list agent guard actions: %w", err)
	}
	return items, total, nil
}

func (r *AgentGuardQueryRepository) GetAction(
	ctx context.Context,
	id uuid.UUID,
) (*model.AgentGuardAction, error) {
	var item model.AgentGuardAction
	return &item, r.getAgentGuardByID(ctx, &item, id, ErrAgentGuardActionNotFound)
}

func (r *AgentGuardQueryRepository) groupAgentGuardCounts(
	ctx context.Context,
	table any,
	column string,
	where string,
	args []any,
	target map[string]int64,
) error {
	type row struct {
		Key   string
		Count int64
	}
	var rows []row
	db := r.db.WithContext(ctx).
		Model(table).
		Select(column + " AS key, COUNT(*) AS count").
		Where(column + " IS NOT NULL AND " + column + " <> ''")
	if where != "" {
		db = db.Where(where, args...)
	}
	if err := db.Group(column).Scan(&rows).Error; err != nil {
		return err
	}
	for _, item := range rows {
		target[item.Key] = item.Count
	}
	return nil
}

func (r *AgentGuardQueryRepository) getAgentGuardByID(
	ctx context.Context,
	target any,
	id uuid.UUID,
	notFound error,
) error {
	err := r.db.WithContext(ctx).First(target, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound
	}
	if err != nil {
		return fmt.Errorf("get agent guard record: %w", err)
	}
	return nil
}

func applyAgentGuardAssetFilters(db *gorm.DB, query model.AgentGuardAgentQuery) *gorm.DB {
	assetTypeExpression := agentGuardCanonicalTypeSQL("COALESCE(NULLIF(a.runtime_name, ''), NULLIF(a.name, ''), 'unknown')")
	runtimeTypeExpression := agentGuardCanonicalTypeSQL("ai.agent_type")
	logicalRuntimeMatch := fmt.Sprintf(
		"ai.host_id = a.host_id AND %s = %s AND ai.detection_confidence = 'confirmed'",
		runtimeTypeExpression,
		assetTypeExpression,
	)
	if len(query.HostIDs) > 0 {
		db = db.Where("a.host_id IN ?", query.HostIDs)
	}
	if len(query.AgentTypes) > 0 {
		canonical := make([]string, 0, len(query.AgentTypes))
		for _, value := range query.AgentTypes {
			canonical = append(canonical, normalizeAgentGuardType(value))
		}
		db = db.Where(
			agentGuardCanonicalTypeSQL("COALESCE(NULLIF(a.runtime_name, ''), NULLIF(a.name, ''), 'unknown')")+" IN ?",
			canonical,
		)
	}
	if query.RuntimeStatus != "" {
		statusPredicate, statusArgs := agentGuardRuntimeStatusPredicate("ai.", query.RuntimeStatus)
		db = db.Where(
			"EXISTS (SELECT 1 FROM agent_runtime_instances ai WHERE "+logicalRuntimeMatch+" AND "+statusPredicate+")",
			statusArgs...,
		)
	}
	if query.Coverage != "" {
		db = db.Where(
			"EXISTS (SELECT 1 FROM agent_runtime_instances ai WHERE "+logicalRuntimeMatch+" AND ai.coverage_level = ?)",
			query.Coverage,
		)
	}
	if query.IsolationType != "" {
		db = db.Where(
			"EXISTS (SELECT 1 FROM agent_runtime_instances ai JOIN agent_execution_units au ON au.instance_id = ai.id WHERE "+logicalRuntimeMatch+" AND au.unit_type = ?)",
			query.IsolationType,
		)
	}
	db = applyAgentGuardRiskFilters(
		db,
		query,
		"EXISTS (SELECT 1 FROM agent_runtime_instances ai JOIN agent_security_findings af ON af.instance_id = ai.id WHERE "+logicalRuntimeMatch+" AND af.severity IN ('high','critical'))",
		"EXISTS (SELECT 1 FROM agent_runtime_instances ai JOIN agent_security_findings af ON af.instance_id = ai.id WHERE "+logicalRuntimeMatch+" AND LOWER(CAST(af.attack_stages AS TEXT)) LIKE '%escape%')",
	)
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		pattern := "%" + strings.ToLower(keyword) + "%"
		db = db.Where(
			`LOWER(COALESCE(a.display_name, '')) LIKE ?
			 OR LOWER(COALESCE(a.name, '')) LIKE ?
			 OR LOWER(h.hostname) LIKE ?
			 OR LOWER(h.ip_address) LIKE ?
			 OR EXISTS (SELECT 1 FROM agent_runtime_instances ai WHERE `+logicalRuntimeMatch+` AND CAST(ai.controller_pid AS TEXT) LIKE ?)`,
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
		)
	}
	return db
}

func applyAgentGuardOrphanFilters(db *gorm.DB, query model.AgentGuardAgentQuery) *gorm.DB {
	if len(query.HostIDs) > 0 {
		db = db.Where("i.host_id IN ?", query.HostIDs)
	}
	if len(query.AgentTypes) > 0 {
		canonical := make([]string, 0, len(query.AgentTypes))
		for _, value := range query.AgentTypes {
			canonical = append(canonical, normalizeAgentGuardType(value))
		}
		db = db.Where(agentGuardCanonicalTypeSQL("i.agent_type")+" IN ?", canonical)
	}
	db = applyAgentGuardRuntimeStatus(db, "i.", query.RuntimeStatus)
	db = applyAgentGuardEqual(db, "i.coverage_level", query.Coverage)
	if query.IsolationType != "" {
		db = db.Where(
			"EXISTS (SELECT 1 FROM agent_execution_units au WHERE au.instance_id = i.id AND au.unit_type = ?)",
			query.IsolationType,
		)
	}
	db = applyAgentGuardRiskFilters(
		db,
		query,
		"EXISTS (SELECT 1 FROM agent_security_findings af WHERE af.instance_id = i.id AND af.severity IN ('high','critical'))",
		"EXISTS (SELECT 1 FROM agent_security_findings af WHERE af.instance_id = i.id AND LOWER(CAST(af.attack_stages AS TEXT)) LIKE '%escape%')",
	)
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		pattern := "%" + strings.ToLower(keyword) + "%"
		db = db.Where(
			`LOWER(COALESCE(i.display_name, i.agent_type)) LIKE ?
			 OR LOWER(i.agent_type) LIKE ?
			 OR LOWER(h.hostname) LIKE ?
			 OR LOWER(h.ip_address) LIKE ?
			 OR CAST(i.controller_pid AS TEXT) LIKE ?`,
			pattern,
			pattern,
			pattern,
			pattern,
			pattern,
		)
	}
	return db
}

func applyAgentGuardRiskFilters(
	db *gorm.DB,
	query model.AgentGuardAgentQuery,
	highRiskExpression string,
	escapeExpression string,
) *gorm.DB {
	if query.HasHighRisk != nil {
		if *query.HasHighRisk {
			db = db.Where(highRiskExpression)
		} else {
			db = db.Where("NOT (" + highRiskExpression + ")")
		}
	}
	if query.HasEscapeFinding != nil {
		if *query.HasEscapeFinding {
			db = db.Where(escapeExpression)
		} else {
			db = db.Where("NOT (" + escapeExpression + ")")
		}
	}
	return db
}

func (r *AgentGuardQueryRepository) enrichAgentGuardScope(
	ctx context.Context,
	row agentGuardScopeRow,
) (model.AgentGuardAgentSummary, error) {
	item := model.AgentGuardAgentSummary{
		AssetID:         row.AssetID,
		Host:            model.AgentGuardHostSummary{ID: row.HostID, Hostname: row.Hostname, IP: row.IPAddress},
		AgentType:       normalizeAgentGuardType(row.AgentType),
		DisplayName:     row.DisplayName,
		ProfileKey:      row.ProfileKey,
		ControllerPIDs:  []int{},
		IsolationTypes:  []string{},
		CoverageLevel:   model.AgentGuardCoverageUnsupportedProfile,
		CoverageReasons: []string{"no_confirmed_runtime"},
		RuntimeStatus:   "stopped",
		ActionStatus:    "none",
	}
	if profileKey := builtinAgentGuardProfileKey(item.AgentType); profileKey != "" {
		item.ProfileKey = profileKey
		item.CoverageLevel = "unknown"
	}
	item.ScopeIdentity = strings.Join([]string{"logical", row.HostID.String(), item.AgentType}, ":")
	var hostInstances []model.AgentRuntimeInstance
	if err := r.db.WithContext(ctx).
		Where("host_id = ? AND detection_confidence = ?", row.HostID, "confirmed").
		Order("last_seen_at DESC").Find(&hostInstances).Error; err != nil {
		return item, fmt.Errorf("load agent scope instances: %w", err)
	}
	instances := make([]model.AgentRuntimeInstance, 0, len(hostInstances))
	for _, instance := range hostInstances {
		if normalizeAgentGuardType(instance.AgentType) == item.AgentType {
			instances = append(instances, instance)
		}
	}
	if len(instances) == 0 {
		return item, nil
	}
	if item.AgentType == "" || item.AgentType == "unknown" {
		item.AgentType = instances[0].AgentType
	}
	if item.DisplayName == "" {
		item.DisplayName = instances[0].DisplayName
	}
	item.ProfileKey = instances[0].ProfileKey
	item.CoverageReasons = []string{}
	instanceIDs := make([]uuid.UUID, 0, len(instances))
	reasons := map[string]struct{}{}
	coverage := ""
	for _, instance := range instances {
		instanceIDs = append(instanceIDs, instance.ID)
		effectiveStatus := agentGuardEffectiveRuntimeStatus(instance.Status, instance.LastSeenAt, time.Now().UTC())
		if effectiveStatus == "running" {
			item.RunningInstanceCount++
			item.ControllerPIDs = append(item.ControllerPIDs, instance.ControllerPID)
			item.RuntimeStatus = "running"
		} else if agentGuardRuntimeStatusRank(effectiveStatus) > agentGuardRuntimeStatusRank(item.RuntimeStatus) {
			item.RuntimeStatus = effectiveStatus
		}
		if item.LastSeenAt == nil || instance.LastSeenAt.After(*item.LastSeenAt) {
			lastSeen := instance.LastSeenAt
			item.LastSeenAt = &lastSeen
		}
		if agentGuardCoverageRank(instance.CoverageLevel) > agentGuardCoverageRank(coverage) {
			coverage = instance.CoverageLevel
		}
		for _, reason := range decodeAgentGuardStringArray(instance.CoverageReasons) {
			reasons[reason] = struct{}{}
		}
	}
	sort.Ints(item.ControllerPIDs)
	item.CoverageLevel = coverage
	for reason := range reasons {
		item.CoverageReasons = append(item.CoverageReasons, reason)
	}
	sort.Strings(item.CoverageReasons)

	var units []model.AgentExecutionUnit
	if err := r.db.WithContext(ctx).
		Where("instance_id IN ?", instanceIDs).
		Find(&units).Error; err != nil {
		return item, fmt.Errorf("load agent scope execution units: %w", err)
	}
	isolationTypes := map[string]struct{}{}
	for _, unit := range units {
		if unit.UnitType != "" {
			isolationTypes[unit.UnitType] = struct{}{}
		}
	}
	for isolationType := range isolationTypes {
		item.IsolationTypes = append(item.IsolationTypes, isolationType)
	}
	sort.Strings(item.IsolationTypes)

	findings := r.db.WithContext(ctx).
		Model(&model.AgentSecurityFinding{}).
		Where("instance_id IN ?", instanceIDs)
	if err := findings.Where("severity IN ?", []string{"high", "critical"}).
		Count(&item.HighRiskFindingCount).Error; err != nil {
		return item, fmt.Errorf("count agent scope high risk findings: %w", err)
	}
	if err := findings.Where("LOWER(CAST(attack_stages AS TEXT)) LIKE ?", "%escape%").
		Count(&item.EscapeFindingCount).Error; err != nil {
		return item, fmt.Errorf("count agent scope escape findings: %w", err)
	}
	var action model.AgentGuardAction
	err := r.db.WithContext(ctx).
		Where("instance_id IN ?", instanceIDs).
		Order("requested_at DESC").
		First(&action).Error
	if err == nil {
		item.ActionStatus = action.Status
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return item, fmt.Errorf("load agent scope latest action: %w", err)
	}
	return item, nil
}

func normalizeAgentGuardType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	aliases := []struct {
		canonical string
		markers   []string
	}{
		{canonical: "claude-code", markers: []string{"claude-code", "claude_code", "claude"}},
		{canonical: "zcode", markers: []string{"zcode", "z-code", "z_code"}},
		{canonical: "gemini-cli", markers: []string{"gemini-cli", "gemini_cli", "gemini"}},
		{canonical: "opencode", markers: []string{"opencode", "open-code", "open_code"}},
		{canonical: "openclaw", markers: []string{"openclaw", "open-claw", "open_claw"}},
		{canonical: "hermes", markers: []string{"hermes"}},
		{canonical: "codex", markers: []string{"openai_codex", "openai-codex", "codex"}},
	}
	for _, alias := range aliases {
		for _, marker := range alias.markers {
			if strings.Contains(value, marker) {
				return alias.canonical
			}
		}
	}
	if value == "" {
		return "unknown"
	}
	return value
}

func builtinAgentGuardProfileKey(agentType string) string {
	return map[string]string{
		"claude-code": "claude-code-linux",
		"codex":       "codex-linux",
		"gemini-cli":  "gemini-cli-linux",
		"hermes":      "hermes-linux",
		"openclaw":    "openclaw-linux",
		"opencode":    "opencode-linux",
		"zcode":       "zcode-linux",
	}[normalizeAgentGuardType(agentType)]
}

func agentGuardCanonicalTypeSQL(expression string) string {
	value := "LOWER(" + expression + ")"
	return fmt.Sprintf(`CASE
		WHEN %s LIKE '%%claude%%' THEN 'claude-code'
		WHEN %s LIKE '%%zcode%%' OR %s LIKE '%%z-code%%' OR %s LIKE '%%z_code%%' THEN 'zcode'
		WHEN %s LIKE '%%gemini%%' THEN 'gemini-cli'
		WHEN %s LIKE '%%opencode%%' OR %s LIKE '%%open-code%%' OR %s LIKE '%%open_code%%' THEN 'opencode'
		WHEN %s LIKE '%%openclaw%%' OR %s LIKE '%%open-claw%%' OR %s LIKE '%%open_claw%%' THEN 'openclaw'
		WHEN %s LIKE '%%hermes%%' THEN 'hermes'
		WHEN %s LIKE '%%codex%%' THEN 'codex'
		ELSE %s END`, value, value, value, value, value, value, value, value, value, value, value, value, value, value)
}

func (r *AgentGuardQueryRepository) selectLogicalAgentAsset(
	ctx context.Context,
	base *gorm.DB,
	hostID uuid.UUID,
	agentType string,
) (model.HostApplicationAsset, error) {
	var asset model.HostApplicationAsset
	expression := agentGuardCanonicalTypeSQL("COALESCE(NULLIF(a.runtime_name, ''), NULLIF(a.name, ''), 'unknown')")
	err := base.Session(&gorm.Session{}).
		Select("a.*").
		Where("a.host_id = ?", hostID).
		Where(expression+" = ?", normalizeAgentGuardType(agentType)).
		Order("CASE a.status WHEN 'active' THEN 0 WHEN 'needs_review' THEN 1 ELSE 2 END ASC").
		Order("a.last_seen_at DESC, a.id ASC").
		First(&asset).Error
	if err != nil {
		return asset, fmt.Errorf("select logical agent asset: %w", err)
	}
	return asset, nil
}

func agentGuardCoverageRank(value string) int {
	return map[string]int{
		model.AgentGuardCoverageFullEnforcement:              1,
		model.AgentGuardCoverageBehaviorMonitorEscapeEnforce: 2,
		model.AgentGuardCoverageMonitorOnly:                  3,
		model.AgentGuardCoverageNoIsolation:                  4,
		model.AgentGuardCoverageRemoteUnobservable:           5,
		model.AgentGuardCoverageUnsupportedProfile:           6,
		model.AgentGuardCoverageDegraded:                     7,
	}[value]
}

func applyAgentRuntimeInstanceFilters(
	db *gorm.DB,
	query model.AgentRuntimeInstanceQuery,
) *gorm.DB {
	prefix := ""
	if db.Statement != nil && strings.Contains(db.Statement.Table, " AS i") {
		prefix = "i."
	}
	db = applyAgentGuardEqual(db, prefix+"host_id", query.HostID)
	if len(query.AssetIDs) > 0 {
		db = db.Where(prefix+"asset_id IN ?", query.AssetIDs)
	}
	if len(query.AgentTypes) > 0 {
		db = db.Where(prefix+"agent_type IN ?", query.AgentTypes)
	}
	if len(query.InstanceIDs) > 0 {
		db = db.Where(prefix+"id IN ?", query.InstanceIDs)
	}
	db = applyAgentGuardEqual(db, prefix+"profile_key", query.ProfileKey)
	db = applyAgentGuardRuntimeStatus(db, prefix, query.Status)
	db = applyAgentGuardEqual(db, prefix+"coverage_level", query.Coverage)
	db = applyAgentGuardTimeRange(db, prefix+"last_seen_at", query.StartTime, query.EndTime)
	if query.ContainerID != "" {
		db = db.Where(
			"EXISTS (SELECT 1 FROM agent_execution_units u2 WHERE u2.instance_id = "+
				prefix+"id AND u2.container_id = ?)",
			query.ContainerID,
		)
	}
	return db
}

func applyAgentGuardRuntimeStatus(db *gorm.DB, prefix, status string) *gorm.DB {
	if status == "" {
		return db
	}
	predicate, args := agentGuardRuntimeStatusPredicate(prefix, status)
	return db.Where(predicate, args...)
}

func agentGuardRuntimeStatusPredicate(prefix, status string) (string, []any) {
	switch status {
	case "running":
		return prefix + "status = ? AND " + prefix + "last_seen_at >= ?", []any{"running", agentGuardRuntimeFreshnessCutoff()}
	case "stale":
		return "(" + prefix + "status = ? OR (" + prefix + "status = ? AND " + prefix + "last_seen_at < ?))",
			[]any{"stale", "running", agentGuardRuntimeFreshnessCutoff()}
	default:
		return prefix + "status = ?", []any{status}
	}
}

func agentGuardRuntimeFreshnessCutoff() time.Time {
	return time.Now().UTC().Add(-agentGuardRuntimeFreshnessWindow)
}

func agentGuardEffectiveRuntimeStatus(status string, lastSeenAt, now time.Time) string {
	if status == "running" && lastSeenAt.Before(now.Add(-agentGuardRuntimeFreshnessWindow)) {
		return "stale"
	}
	return status
}

func agentGuardRuntimeStatusRank(status string) int {
	switch status {
	case "running":
		return 4
	case "stale":
		return 3
	case "unknown":
		return 2
	case "stopped":
		return 1
	default:
		return 0
	}
}

func applyAgentGuardEqual(db *gorm.DB, column string, value string) *gorm.DB {
	if value == "" {
		return db
	}
	return db.Where(column+" = ?", value)
}

func applyAgentGuardTimeRange(
	db *gorm.DB,
	column string,
	start *time.Time,
	end *time.Time,
) *gorm.DB {
	if start != nil {
		db = db.Where(column+" >= ?", *start)
	}
	if end != nil {
		db = db.Where(column+" <= ?", *end)
	}
	return db
}

func findAgentGuardPage(db *gorm.DB, page int, pageSize int, target any) error {
	page, pageSize = normalizeAgentGuardPage(page, pageSize)
	return db.Offset((page - 1) * pageSize).Limit(pageSize).Find(target).Error
}

func decodeAgentGuardStringArray(value []byte) []string {
	var items []string
	if len(value) == 0 || json.Unmarshal(value, &items) != nil {
		return []string{}
	}
	sort.Strings(items)
	return items
}
