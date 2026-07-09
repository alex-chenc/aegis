package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

// HostAttackInvestigationService manages attack investigations
type HostAttackInvestigationService struct {
	reportRepo   repository.AssistantInvestigationReportRepository
	evidenceRepo repository.AssistantInvestigationEvidenceRepository
	hostRepo     *repository.HostRepository
	alertRepo    *repository.AlertRepository
	taskRepo     *repository.TaskLogRepository
	vulnRepo     *repository.VulnerabilityRepo
	blockRepo    *repository.BlockRepository
	logger       *zap.Logger
}

// HostAttackInvestigationServiceDeps service dependencies
type HostAttackInvestigationServiceDeps struct {
	ReportRepo   repository.AssistantInvestigationReportRepository
	EvidenceRepo repository.AssistantInvestigationEvidenceRepository
	HostRepo     *repository.HostRepository
	AlertRepo    *repository.AlertRepository
	TaskRepo     *repository.TaskLogRepository
	VulnRepo     *repository.VulnerabilityRepo
	BlockRepo    *repository.BlockRepository
	Logger       *zap.Logger
}

// NewHostAttackInvestigationService creates a new HostAttackInvestigationService
func NewHostAttackInvestigationService(deps HostAttackInvestigationServiceDeps) *HostAttackInvestigationService {
	return &HostAttackInvestigationService{
		reportRepo:   deps.ReportRepo,
		evidenceRepo: deps.EvidenceRepo,
		hostRepo:     deps.HostRepo,
		alertRepo:    deps.AlertRepo,
		taskRepo:     deps.TaskRepo,
		vulnRepo:     deps.VulnRepo,
		blockRepo:    deps.BlockRepo,
		logger:       deps.Logger,
	}
}

// CreateInvestigation creates a new host attack investigation
func (s *HostAttackInvestigationService) CreateInvestigation(ctx context.Context, input model.HostAttackInvestigationInput, operator string) (*model.HostAttackInvestigationResult, error) {
	investigationID := "inv_" + uuid.New().String()[:8]

	s.logger.Info("creating host attack investigation",
		zap.String("investigation_id", investigationID),
		zap.String("host_id", input.HostID),
		zap.String("operator", operator),
	)

	// Build host snapshot
	hostSnapshot, err := s.buildHostSnapshot(ctx, input)
	if err != nil {
		s.logger.Warn("failed to build host snapshot, continuing with partial data",
			zap.String("investigation_id", investigationID),
			zap.Error(err),
		)
		hostSnapshot = model.HostSnapshot{
			HostID:   input.HostID,
			Hostname: input.Hostname,
			IPs:      input.IPs,
		}
	}

	// Collect evidence from multiple sources
	var evidences []model.AssistantInvestigationEvidence
	evidenceBySource := make(map[string][]string)
	evidenceByPhase := make(map[string][]string)
	evidenceByMITRE := make(map[string][]string)

	// 1. Collect alert evidence
	alertEvidence := s.collectAlertEvidence(ctx, investigationID, input)
	evidences = append(evidences, alertEvidence...)

	// 2. Collect vulnerability evidence
	vulnEvidence := s.collectVulnerabilityEvidence(ctx, investigationID, input)
	evidences = append(evidences, vulnEvidence...)

	// 3. Collect baseline task evidence
	taskEvidence := s.collectTaskEvidence(ctx, investigationID, input)
	evidences = append(evidences, taskEvidence...)

	// 4. Collect block evidence
	blockEvidence := s.collectBlockEvidence(ctx, investigationID, input)
	evidences = append(evidences, blockEvidence...)

	// Build evidence index maps
	for _, ev := range evidences {
		evidenceBySource[ev.SourceType] = append(evidenceBySource[ev.SourceType], ev.EvidenceID)
		evidenceByPhase["detection"] = append(evidenceByPhase["detection"], ev.EvidenceID)
		if ev.MITREID != "" {
			evidenceByMITRE[ev.MITREID] = append(evidenceByMITRE[ev.MITREID], ev.EvidenceID)
		}
	}

	// Build compromise assessment
	assessment := s.buildCompromiseAssessment(evidences)

	// Determine verdict and score for the report
	verdict := assessment.Verdict
	score := assessment.Score
	confidence := assessment.Confidence

	// Save evidence to database
	if len(evidences) > 0 {
		if err := s.evidenceRepo.BatchSave(ctx, evidences); err != nil {
			s.logger.Error("failed to save investigation evidence",
				zap.String("investigation_id", investigationID),
				zap.Error(err),
			)
		}
	}

	// Build evidence matrix
	evidenceItems := make([]model.EvidenceItem, 0, len(evidences))
	for _, ev := range evidences {
		item := model.EvidenceItem{
			EvidenceID: ev.EvidenceID,
			SourceType: ev.SourceType,
			SourceName: ev.SourceName,
			ObjectType: ev.ObjectType,
			ObjectID:   ev.ObjectID,
			HostID:     ev.HostID,
			Timestamp:  ev.EventTime,
			Severity:   ev.Severity,
			MITREID:    ev.MITREID,
			Title:      ev.Title,
			Summary:    ev.Summary,
			Confidence: ev.Confidence,
			IsExternal: ev.IsExternal,
		}
		evidenceItems = append(evidenceItems, item)
	}

	// Determine key evidence
	keyEvidence := s.selectKeyEvidence(evidences)

	// Build MITRE techniques from evidence
	mitreTechniques := s.buildMITRETechniques(evidences)

	// Build impact scope
	impactScope := s.buildImpactScope(evidences, input)

	// Build source coverage
	sourceCoverage := model.SourceCoverage{
		AegisInternal: true,
		AgentLive:     input.IncludeAgentLive,
		ExternalMCP:   input.IncludeExternalMCP,
	}

	// Build attack timeline
	attackTimeline := s.buildAttackTimeline(evidences)

	// Build attack path
	attackPath := s.buildAttackPath(evidences)

	// Build entry point candidates
	entryCandidates := s.buildEntryPointCandidates(evidences)

	// Build recommended actions
	recommendedActions := s.buildRecommendedActions(assessment, evidences)

	// Build missing evidence
	missingEvidence := s.buildMissingEvidence(input, evidences)

	// Build report markdown
	reportMarkdown := s.buildReportMarkdown(investigationID, hostSnapshot, assessment, evidences)

	// Save report to database
	report := &model.AssistantInvestigationReport{
		ID:              uuid.New(),
		InvestigationID: investigationID,
		SessionID:       input.SessionID,
		RunID:           input.RunID,
		HostID:          input.HostID,
		TaskType:        "host_attack_investigation",
		Verdict:         verdict,
		Score:           score,
		Confidence:      confidence,
		SourceCoverage:  toJSON(sourceCoverage),
		MissingEvidence: toJSON(missingEvidence),
		ReportMarkdown:  reportMarkdown,
		CreatedBy:       operator,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := s.reportRepo.Save(ctx, report); err != nil {
		s.logger.Error("failed to save investigation report",
			zap.String("investigation_id", investigationID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to save investigation report: %w", err)
	}

	result := &model.HostAttackInvestigationResult{
		InvestigationID:      investigationID,
		Host:                 hostSnapshot,
		TimeRange:            input.TimeRange,
		CompromiseAssessment: assessment,
		EntryPointCandidates: entryCandidates,
		AttackTimeline:       attackTimeline,
		AttackPath:           attackPath,
		EvidenceMatrix: model.EvidenceMatrix{
			Items:       evidenceItems,
			ByPhase:     evidenceByPhase,
			BySource:    evidenceBySource,
			ByMITRE:     evidenceByMITRE,
			KeyEvidence: keyEvidence,
		},
		MITRETechniques:    mitreTechniques,
		ImpactScope:        impactScope,
		RecommendedActions: recommendedActions,
		MissingEvidence:    missingEvidence,
		SourceCoverage:     sourceCoverage,
		ReportMarkdown:     reportMarkdown,
		CreatedAt:          time.Now(),
	}

	s.logger.Info("host attack investigation created",
		zap.String("investigation_id", investigationID),
		zap.String("verdict", verdict),
		zap.Int("score", score),
		zap.Int("evidence_count", len(evidences)),
	)
	return result, nil
}

// GetInvestigation retrieves an investigation report by investigation_id
func (s *HostAttackInvestigationService) GetInvestigation(ctx context.Context, investigationID string) (*model.AssistantInvestigationReport, error) {
	report, err := s.reportRepo.FindByInvestigationID(ctx, investigationID)
	if err != nil {
		s.logger.Error("failed to find investigation report",
			zap.String("investigation_id", investigationID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("investigation report not found: %w", err)
	}

	s.logger.Debug("investigation report retrieved",
		zap.String("investigation_id", investigationID),
	)
	return report, nil
}

// ListEvidence lists evidence for an investigation
func (s *HostAttackInvestigationService) ListEvidence(ctx context.Context, investigationID string, query repository.EvidenceQuery) ([]model.AssistantInvestigationEvidence, int64, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 50
	}

	evidences, total, err := s.evidenceRepo.ListByInvestigation(ctx, investigationID, query)
	if err != nil {
		s.logger.Error("failed to list investigation evidence",
			zap.String("investigation_id", investigationID),
			zap.Error(err),
		)
		return nil, 0, fmt.Errorf("failed to list evidence: %w", err)
	}

	s.logger.Debug("investigation evidence listed",
		zap.String("investigation_id", investigationID),
		zap.Int("count", len(evidences)),
		zap.Int64("total", total),
	)
	return evidences, total, nil
}

// --- Private helper methods ---

// buildHostSnapshot builds a host snapshot from available data
func (s *HostAttackInvestigationService) buildHostSnapshot(ctx context.Context, input model.HostAttackInvestigationInput) (model.HostSnapshot, error) {
	snapshot := model.HostSnapshot{
		HostID:   input.HostID,
		Hostname: input.Hostname,
		IPs:      input.IPs,
	}

	// Try to find host by UUID
	hostUUID, err := uuid.Parse(input.HostID)
	if err == nil {
		host, err := s.hostRepo.FindByID(hostUUID)
		if err == nil && host != nil {
			snapshot.Hostname = host.Hostname
			snapshot.OS = host.OSType
			snapshot.AgentStatus = "online"
			if len(snapshot.IPs) == 0 {
				snapshot.IPs = []string{host.IPAddress}
			}
			return snapshot, nil
		}
	}

	return snapshot, fmt.Errorf("host not found: %s", input.HostID)
}

// collectAlertEvidence collects evidence from alerts
func (s *HostAttackInvestigationService) collectAlertEvidence(ctx context.Context, investigationID string, input model.HostAttackInvestigationInput) []model.AssistantInvestigationEvidence {
	var evidences []model.AssistantInvestigationEvidence

	// Collect by specific alert IDs if provided
	if len(input.AlertIDs) > 0 {
		alerts, err := s.alertRepo.FindByAlertIDs(input.AlertIDs)
		if err != nil {
			s.logger.Warn("failed to find alerts by IDs", zap.Error(err))
			return evidences
		}
		for _, alert := range alerts {
			ev := model.AssistantInvestigationEvidence{
				ID:              uuid.New(),
				InvestigationID: investigationID,
				EvidenceID:      "ev_alert_" + alert.AlertID,
				SourceType:      "alert",
				SourceName:      "aegis_alerts",
				ObjectType:      "alert",
				ObjectID:        alert.AlertID,
				HostID:          input.HostID,
				EventTime:       &alert.CreatedAt,
				Severity:        alert.Severity,
				MITREID:         alert.MitreID,
				Title:           alert.RuleTitle,
				Summary:         alert.Description,
				Confidence:      0.8,
				IsExternal:      false,
			}
			evidences = append(evidences, ev)
		}
		return evidences
	}

	// Otherwise collect by time range and host
	if input.HostID != "" {
		hostUUID, err := uuid.Parse(input.HostID)
		if err == nil {
			alerts, err := s.alertRepo.FindPendingByTimeRange(
				input.TimeRange.From,
				input.TimeRange.To,
				[]string{hostUUID.String()},
			)
			if err != nil {
				s.logger.Warn("failed to find alerts by time range", zap.Error(err))
				return evidences
			}
			for _, alert := range alerts {
				ev := model.AssistantInvestigationEvidence{
					ID:              uuid.New(),
					InvestigationID: investigationID,
					EvidenceID:      "ev_alert_" + alert.AlertID,
					SourceType:      "alert",
					SourceName:      "aegis_alerts",
					ObjectType:      "alert",
					ObjectID:        alert.AlertID,
					HostID:          input.HostID,
					EventTime:       &alert.CreatedAt,
					Severity:        alert.Severity,
					MITREID:         alert.MitreID,
					Title:           alert.RuleTitle,
					Summary:         alert.Description,
					Confidence:      0.8,
					IsExternal:      false,
				}
				evidences = append(evidences, ev)
			}
		}
	}

	return evidences
}

// collectVulnerabilityEvidence collects evidence from vulnerabilities
func (s *HostAttackInvestigationService) collectVulnerabilityEvidence(ctx context.Context, investigationID string, input model.HostAttackInvestigationInput) []model.AssistantInvestigationEvidence {
	var evidences []model.AssistantInvestigationEvidence

	if len(input.CVEIDs) > 0 {
		for _, cveID := range input.CVEIDs {
			vuln, err := s.vulnRepo.FindByCveID(cveID)
			if err != nil {
				s.logger.Warn("failed to find vulnerability", zap.String("cve_id", cveID), zap.Error(err))
				continue
			}

			ev := model.AssistantInvestigationEvidence{
				ID:              uuid.New(),
				InvestigationID: investigationID,
				EvidenceID:      "ev_vuln_" + cveID,
				SourceType:      "vulnerability",
				SourceName:      "aegis_vulnerabilities",
				ObjectType:      "vulnerability",
				ObjectID:        cveID,
				HostID:          input.HostID,
				Severity:        vuln.Severity,
				Title:           cveID,
				Summary:         vuln.Description,
				Confidence:      0.7,
				IsExternal:      false,
			}
			evidences = append(evidences, ev)
		}
	}

	return evidences
}

// collectTaskEvidence collects evidence from baseline tasks
func (s *HostAttackInvestigationService) collectTaskEvidence(ctx context.Context, investigationID string, input model.HostAttackInvestigationInput) []model.AssistantInvestigationEvidence {
	var evidences []model.AssistantInvestigationEvidence

	if input.HostID == "" {
		return evidences
	}

	// Look for recent task groups (ListTaskGroupsParams doesn't have HostID filter)
	taskGroups, err := s.taskRepo.ListTaskGroups(repository.ListTaskGroupsParams{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		s.logger.Warn("failed to list task groups", zap.Error(err))
		return evidences
	}

	for _, tg := range taskGroups {
		ev := model.AssistantInvestigationEvidence{
			ID:              uuid.New(),
			InvestigationID: investigationID,
			EvidenceID:      "ev_task_" + tg.TaskGroupID.String(),
			SourceType:      "baseline_task",
			SourceName:      "aegis_tasks",
			ObjectType:      "task_group",
			ObjectID:        tg.TaskGroupID.String(),
			HostID:          input.HostID,
			Severity:        "info",
			Title:           fmt.Sprintf("Baseline task: %s", tg.TaskType),
			Summary:         fmt.Sprintf("Task type: %s, status: %s", tg.TaskType, tg.Status),
			Confidence:      0.6,
			IsExternal:      false,
		}
		evidences = append(evidences, ev)
	}

	return evidences
}

// collectBlockEvidence collects evidence from block records
func (s *HostAttackInvestigationService) collectBlockEvidence(ctx context.Context, investigationID string, input model.HostAttackInvestigationInput) []model.AssistantInvestigationEvidence {
	var evidences []model.AssistantInvestigationEvidence

	if input.HostID == "" {
		return evidences
	}

	hostUUID, err := uuid.Parse(input.HostID)
	if err != nil {
		return evidences
	}

	blocks, _, err := s.blockRepo.List(1, 20, map[string]interface{}{
		"host_id": hostUUID,
	})
	if err != nil {
		s.logger.Warn("failed to list block records for host", zap.Error(err))
		return evidences
	}

	for _, block := range blocks {
		ev := model.AssistantInvestigationEvidence{
			ID:              uuid.New(),
			InvestigationID: investigationID,
			EvidenceID:      "ev_block_" + block.BlockID,
			SourceType:      "block",
			SourceName:      "aegis_blocks",
			ObjectType:      "block_record",
			ObjectID:        block.BlockID,
			HostID:          input.HostID,
			EventTime:       &block.CreatedAt,
			Severity:        "high",
			Title:           fmt.Sprintf("Block action: %s", block.Action),
			Summary:         block.Message,
			Confidence:      0.9,
			IsExternal:      false,
		}
		evidences = append(evidences, ev)
	}

	return evidences
}

// buildCompromiseAssessment builds a compromise assessment based on evidence
func (s *HostAttackInvestigationService) buildCompromiseAssessment(evidences []model.AssistantInvestigationEvidence) model.CompromiseAssessment {
	if len(evidences) == 0 {
		return model.CompromiseAssessment{
			Verdict:    model.VerdictInsufficientEvidence,
			Score:      0,
			Confidence: 0,
			Summary:    "No evidence found to assess compromise status.",
			KeyReasons: []string{"No alerts, vulnerabilities, tasks, or block records found for the specified host and time range."},
		}
	}

	// Count evidence by severity
	var criticalCount, highCount, mediumCount int
	var alertCount, blockCount, vulnCount int
	for _, ev := range evidences {
		switch ev.Severity {
		case "critical":
			criticalCount++
		case "high":
			highCount++
		case "medium":
			mediumCount++
		}
		switch ev.SourceType {
		case "alert":
			alertCount++
		case "block":
			blockCount++
		case "vulnerability":
			vulnCount++
		}
	}

	// Calculate score based on evidence
	score := 0
	var reasons []string

	if criticalCount > 0 {
		score += criticalCount * 30
		reasons = append(reasons, fmt.Sprintf("Found %d critical severity evidence items", criticalCount))
	}
	if highCount > 0 {
		score += highCount * 20
		reasons = append(reasons, fmt.Sprintf("Found %d high severity evidence items", highCount))
	}
	if mediumCount > 0 {
		score += mediumCount * 10
		reasons = append(reasons, fmt.Sprintf("Found %d medium severity evidence items", mediumCount))
	}
	if blockCount > 0 {
		score += blockCount * 15
		reasons = append(reasons, fmt.Sprintf("Found %d block actions indicating active threats", blockCount))
	}
	if alertCount > 0 {
		score += alertCount * 5
		reasons = append(reasons, fmt.Sprintf("Found %d security alerts", alertCount))
	}
	if vulnCount > 0 {
		score += vulnCount * 5
		reasons = append(reasons, fmt.Sprintf("Found %d associated vulnerabilities", vulnCount))
	}

	// Cap score at 100
	if score > 100 {
		score = 100
	}

	// Determine verdict
	var verdict string
	var confidence float64
	switch {
	case score >= 70:
		verdict = model.VerdictConfirmedCompromised
		confidence = 0.8
	case score >= 40:
		verdict = model.VerdictSuspicious
		confidence = 0.6
	case score > 0:
		verdict = model.VerdictLikelyBenign
		confidence = 0.5
	default:
		verdict = model.VerdictInsufficientEvidence
		confidence = 0.1
		reasons = []string{"Evidence present but no strong indicators of compromise found."}
	}

	summary := fmt.Sprintf("Assessment based on %d evidence items. Score: %d/100.", len(evidences), score)

	return model.CompromiseAssessment{
		Verdict:    verdict,
		Score:      score,
		Confidence: confidence,
		Summary:    summary,
		KeyReasons: reasons,
	}
}

// selectKeyEvidence selects the most important evidence items
func (s *HostAttackInvestigationService) selectKeyEvidence(evidences []model.AssistantInvestigationEvidence) []string {
	var keyEvidence []string
	// Select critical and high severity evidence as key
	for _, ev := range evidences {
		if ev.Severity == "critical" || ev.Severity == "high" {
			keyEvidence = append(keyEvidence, ev.EvidenceID)
		}
	}
	// If no critical/high, select up to 5 most recent
	if len(keyEvidence) == 0 && len(evidences) > 0 {
		limit := 5
		if len(evidences) < limit {
			limit = len(evidences)
		}
		for i := 0; i < limit; i++ {
			keyEvidence = append(keyEvidence, evidences[i].EvidenceID)
		}
	}
	return keyEvidence
}

// buildMITRETechniques builds MITRE technique evidence from evidence items
func (s *HostAttackInvestigationService) buildMITRETechniques(evidences []model.AssistantInvestigationEvidence) []model.MITRETechniqueEvidence {
	techniqueMap := make(map[string][]string)
	for _, ev := range evidences {
		if ev.MITREID != "" {
			techniqueMap[ev.MITREID] = append(techniqueMap[ev.MITREID], ev.EvidenceID)
		}
	}

	var techniques []model.MITRETechniqueEvidence
	for mitreID, evIDs := range techniqueMap {
		techniques = append(techniques, model.MITRETechniqueEvidence{
			TechniqueID: mitreID,
			Name:        mitreID,
			EvidenceIDs: evIDs,
		})
	}
	return techniques
}

// buildImpactScope builds the impact scope from evidence
func (s *HostAttackInvestigationService) buildImpactScope(_ []model.AssistantInvestigationEvidence, input model.HostAttackInvestigationInput) model.ImpactScope {
	return model.ImpactScope{
		AffectedHosts: []string{input.HostID},
	}
}

// buildAttackTimeline builds an attack timeline from evidence
func (s *HostAttackInvestigationService) buildAttackTimeline(evidences []model.AssistantInvestigationEvidence) model.AttackTimeline {
	var events []model.AttackTimelineEvent
	for _, ev := range evidences {
		if ev.EventTime != nil {
			events = append(events, model.AttackTimelineEvent{
				EventID:     ev.EvidenceID,
				Time:        *ev.EventTime,
				Phase:       "detection",
				Title:       ev.Title,
				Summary:     ev.Summary,
				EvidenceIDs: []string{ev.EvidenceID},
				Confidence:  ev.Confidence,
			})
		}
	}
	return model.AttackTimeline{Events: events}
}

// buildAttackPath builds an attack path graph from evidence
func (s *HostAttackInvestigationService) buildAttackPath(evidences []model.AssistantInvestigationEvidence) model.AttackPathGraph {
	nodes := []model.AttackPathNode{
		{
			NodeID:    "host",
			NodeType:  "host",
			Label:     "Target Host",
			RiskLevel: "medium",
		},
	}

	var edges []model.AttackPathEdge
	for i, ev := range evidences {
		nodeID := fmt.Sprintf("evidence_%d", i)
		nodes = append(nodes, model.AttackPathNode{
			NodeID:      nodeID,
			NodeType:    ev.SourceType,
			Label:       ev.Title,
			RiskLevel:   ev.Severity,
			EvidenceIDs: []string{ev.EvidenceID},
		})
		edges = append(edges, model.AttackPathEdge{
			From:        "host",
			To:          nodeID,
			Relation:    "detected_by",
			EvidenceIDs: []string{ev.EvidenceID},
			Confidence:  ev.Confidence,
		})
	}

	return model.AttackPathGraph{
		Nodes: nodes,
		Edges: edges,
	}
}

// buildEntryPointCandidates builds entry point candidates from evidence
func (s *HostAttackInvestigationService) buildEntryPointCandidates(evidences []model.AssistantInvestigationEvidence) []model.EntryPointCandidate {
	var candidates []model.EntryPointCandidate
	for _, ev := range evidences {
		if ev.Severity == "critical" || ev.Severity == "high" {
			candidates = append(candidates, model.EntryPointCandidate{
				CandidateID: "ep_" + uuid.New().String()[:8],
				EntryType:   ev.SourceType,
				Title:       ev.Title,
				Score:       severityToScore(ev.Severity),
				Confidence:  ev.Confidence,
				FirstSeenAt: ev.EventTime,
				EvidenceIDs: []string{ev.EvidenceID},
				Explanation: ev.Summary,
			})
		}
	}
	return candidates
}

// buildRecommendedActions builds recommended actions based on assessment
func (s *HostAttackInvestigationService) buildRecommendedActions(assessment model.CompromiseAssessment, evidences []model.AssistantInvestigationEvidence) []model.RecommendedAction {
	var actions []model.RecommendedAction

	switch assessment.Verdict {
	case model.VerdictConfirmedCompromised:
		actions = append(actions, model.RecommendedAction{
			ActionType:  "isolate",
			Title:       "Isolate Host",
			Description: "Immediately isolate the host from the network to prevent lateral movement.",
			RiskLevel:   "high",
			ToolName:    "network_isolate",
		})
		actions = append(actions, model.RecommendedAction{
			ActionType:  "investigate",
			Title:       "Deep Investigation",
			Description: "Perform a deep forensic investigation of the compromised host.",
			RiskLevel:   "high",
		})
	case model.VerdictSuspicious:
		actions = append(actions, model.RecommendedAction{
			ActionType:  "monitor",
			Title:       "Enhanced Monitoring",
			Description: "Enable enhanced monitoring on the host and related systems.",
			RiskLevel:   "medium",
		})
		actions = append(actions, model.RecommendedAction{
			ActionType:  "collect",
			Title:       "Collect Forensic Data",
			Description: "Collect volatile forensic data before it is lost.",
			RiskLevel:   "medium",
		})
	case model.VerdictLikelyBenign:
		actions = append(actions, model.RecommendedAction{
			ActionType:  "review",
			Title:       "Manual Review",
			Description: "Review the evidence items manually to confirm benign status.",
			RiskLevel:   "low",
		})
	default:
		actions = append(actions, model.RecommendedAction{
			ActionType:  "collect",
			Title:       "Collect More Evidence",
			Description: "Gather additional evidence from the host and related systems.",
			RiskLevel:   "low",
		})
	}

	return actions
}

// buildMissingEvidence identifies what evidence sources are missing
func (s *HostAttackInvestigationService) buildMissingEvidence(input model.HostAttackInvestigationInput, evidences []model.AssistantInvestigationEvidence) []model.MissingEvidence {
	var missing []model.MissingEvidence

	hasAlerts := false
	hasVulns := false
	hasTasks := false
	hasBlocks := false
	for _, ev := range evidences {
		switch ev.SourceType {
		case "alert":
			hasAlerts = true
		case "vulnerability":
			hasVulns = true
		case "baseline_task":
			hasTasks = true
		case "block":
			hasBlocks = true
		}
	}

	if !hasAlerts {
		missing = append(missing, model.MissingEvidence{
			SourceType:      "alert",
			Description:     "No security alerts found for the specified host and time range.",
			SuggestedAction: "Check if the agent is running and reporting events properly.",
		})
	}
	if !hasVulns {
		missing = append(missing, model.MissingEvidence{
			SourceType:      "vulnerability",
			Description:     "No vulnerability data associated with this investigation.",
			SuggestedAction: "Run a vulnerability scan on the host.",
		})
	}
	if !hasTasks {
		missing = append(missing, model.MissingEvidence{
			SourceType:      "baseline_task",
			Description:     "No baseline task results found for this host.",
			SuggestedAction: "Run baseline audit tasks to gather system state information.",
		})
	}
	if !hasBlocks {
		missing = append(missing, model.MissingEvidence{
			SourceType:      "block",
			Description:     "No block actions recorded for this host.",
			SuggestedAction: "Review block policies and auto-block configuration.",
		})
	}

	return missing
}

// buildReportMarkdown builds a markdown report
func (s *HostAttackInvestigationService) buildReportMarkdown(investigationID string, host model.HostSnapshot, assessment model.CompromiseAssessment, evidences []model.AssistantInvestigationEvidence) string {
	report := fmt.Sprintf("# Attack Investigation Report: %s\n\n", investigationID)
	report += fmt.Sprintf("## Host Information\n- **Host ID**: %s\n- **Hostname**: %s\n- **OS**: %s\n- **Agent Status**: %s\n\n",
		host.HostID, host.Hostname, host.OS, host.AgentStatus)

	report += fmt.Sprintf("## Compromise Assessment\n- **Verdict**: %s\n- **Score**: %d/100\n- **Confidence**: %.2f\n- **Summary**: %s\n\n",
		assessment.Verdict, assessment.Score, assessment.Confidence, assessment.Summary)

	if len(assessment.KeyReasons) > 0 {
		report += "### Key Reasons\n"
		for _, reason := range assessment.KeyReasons {
			report += fmt.Sprintf("- %s\n", reason)
		}
		report += "\n"
	}

	report += fmt.Sprintf("## Evidence Summary\nTotal evidence items: %d\n\n", len(evidences))
	if len(evidences) > 0 {
		report += "| Evidence ID | Source | Severity | Title |\n|---|---|---|---|\n"
		for _, ev := range evidences {
			report += fmt.Sprintf("| %s | %s | %s | %s |\n", ev.EvidenceID, ev.SourceType, ev.Severity, ev.Title)
		}
	}

	return report
}

// severityToScore converts severity string to numeric score
func severityToScore(severity string) int {
	switch severity {
	case "critical":
		return 90
	case "high":
		return 70
	case "medium":
		return 50
	case "low":
		return 30
	default:
		return 10
	}
}

// toJSON marshals a value to datatypes.JSON
func toJSON(v interface{}) datatypes.JSON {
	data, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(data)
}
