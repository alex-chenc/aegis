package adapters

import (
	"context"
	"fmt"

	agentruntime "github.com/alex-chenc/agent-runtime"

	"api-server/internal/repository"
	"api-server/internal/service"
)

// AgentReflectionSummary is a lightweight summary of a failed-agent reflection.
type AgentReflectionSummary struct {
	ReflectionID   string
	RootCause      string
	Impact         string
	ReusableLesson string
}

// AgentExecutionSummary is a lightweight summary of a successful agent execution.
type AgentExecutionSummary struct {
	TaskID      string
	FinalAnswer string
}

// AgentAuditSummary is a lightweight summary of an audit correction record.
type AgentAuditSummary struct {
	AuditID        string
	Decision       string
	CorrectionHint string
}

// AgentModelErrorSummary is a lightweight summary of a recoverable model error.
type AgentModelErrorSummary struct {
	CallID    string
	Purpose   string
	ErrorKind string
	Message   string
}

// ReflectionQuerier abstracts the repository that supplies reflection and
// execution summaries, so the adapter does not depend on a concrete model package.
type ReflectionQuerier interface {
	FindFailedReflections(ctx context.Context, limit int) ([]AgentReflectionSummary, error)
	FindSuccessfulSummaries(ctx context.Context, limit int) ([]AgentExecutionSummary, error)
	FindRecentAudits(ctx context.Context, limit int) ([]AgentAuditSummary, error)
	FindRecentModelErrors(ctx context.Context, limit int) ([]AgentModelErrorSummary, error)
}

// defaultMaxItems is the fallback when the caller does not specify a limit.
const defaultMaxItems = 5

// ExperienceProviderAdapter bridges the existing VectorService and a
// ReflectionQuerier to the agent-runtime ExperienceProvider interface.
type ExperienceProviderAdapter struct {
	vectorSvc         *service.VectorService
	reflectionQuerier ReflectionQuerier
	maxItems          int
}

// NewExperienceProviderAdapter creates a new adapter. If maxItems <= 0 the
// default value (5) is used.
func NewExperienceProviderAdapter(
	vectorSvc *service.VectorService,
	reflectionQuerier ReflectionQuerier,
	maxItems int,
) *ExperienceProviderAdapter {
	if maxItems <= 0 {
		maxItems = defaultMaxItems
	}
	return &ExperienceProviderAdapter{
		vectorSvc:         vectorSvc,
		reflectionQuerier: reflectionQuerier,
		maxItems:          maxItems,
	}
}

// Fetch implements the agent-runtime ExperienceProvider interface. It combines
// vector-similar historical analyses with reflection lessons, falling back to
// recent successful executions when the vector store is unavailable.
func (p *ExperienceProviderAdapter) Fetch(ctx context.Context, req agentruntime.ExperienceRequest) (agentruntime.ExperienceResponse, error) {
	limit := req.MaxItems
	if limit <= 0 {
		limit = p.maxItems
	}

	var items []agentruntime.ExperienceItem

	// --- 1. Vector similarity search ---
	similarItems, vectorErr := p.fetchSimilarCases(ctx, req.Query, limit)
	if vectorErr == nil {
		items = append(items, similarItems...)
	}

	// --- 2. Reflection lessons ---
	reflectionItems, reflectionErr := p.fetchReflectionLessons(ctx)
	if reflectionErr == nil {
		items = append(items, reflectionItems...)
	}

	// --- 3. Audit correction patterns ---
	auditItems, auditErr := p.fetchAuditLessons(ctx)
	if auditErr == nil {
		items = append(items, auditItems...)
	}

	// --- 4. Model error degradation strategies ---
	modelErrItems, modelErrErr := p.fetchModelErrorLessons(ctx)
	if modelErrErr == nil {
		items = append(items, modelErrItems...)
	}

	// --- 5. Fallback: recent successes when vector store failed ---
	if vectorErr != nil {
		recentItems, recentErr := p.fetchRecentSuccesses(ctx, limit)
		if recentErr == nil {
			items = append(items, recentItems...)
		}
	}

	_ = reflectionErr // reflection errors are non-fatal; return whatever we collected
	_ = auditErr
	_ = modelErrErr

	return agentruntime.ExperienceResponse{Items: items}, nil
}

// fetchSimilarCases queries VectorService for semantically similar analyses.
func (p *ExperienceProviderAdapter) fetchSimilarCases(ctx context.Context, query string, limit int) ([]agentruntime.ExperienceItem, error) {
	if p.vectorSvc == nil {
		return nil, fmt.Errorf("vector service not configured")
	}

	results, err := p.vectorSvc.FindSimilarAnalysis(ctx, query, "", 0.7, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	items := make([]agentruntime.ExperienceItem, 0, len(results))
	for _, r := range results {
		items = append(items, agentruntime.ExperienceItem{
			ID:      r.SessionID,
			Summary: r.Summary,
			Content: formatSimilarCase(r),
			Tags:    []string{"similar_case", "rag"},
			Metadata: map[string]any{
				"session_id": r.SessionID,
				"similarity": r.Similarity,
				"alert_ids":  r.AlertIDs,
			},
		})
	}
	return items, nil
}

// fetchReflectionLessons retrieves recent failed-agent reflections that carry
// reusable lessons.
func (p *ExperienceProviderAdapter) fetchReflectionLessons(ctx context.Context) ([]agentruntime.ExperienceItem, error) {
	if p.reflectionQuerier == nil {
		return nil, fmt.Errorf("reflection querier not configured")
	}

	reflections, err := p.reflectionQuerier.FindFailedReflections(ctx, 3)
	if err != nil {
		return nil, fmt.Errorf("reflection query failed: %w", err)
	}

	items := make([]agentruntime.ExperienceItem, 0, len(reflections))
	for _, r := range reflections {
		content := fmt.Sprintf("Root Cause: %s\nImpact: %s\nLesson: %s",
			r.RootCause, r.Impact, r.ReusableLesson)
		items = append(items, agentruntime.ExperienceItem{
			ID:      r.ReflectionID,
			Summary: r.ReusableLesson,
			Content: content,
			Tags:    []string{"reflection", "lesson"},
			Metadata: map[string]any{
				"root_cause": r.RootCause,
				"impact":     r.Impact,
			},
		})
	}
	return items, nil
}

// fetchAuditLessons retrieves recent audit correction patterns where
// decision='correct_plan', providing insights into plan drift corrections.
func (p *ExperienceProviderAdapter) fetchAuditLessons(ctx context.Context) ([]agentruntime.ExperienceItem, error) {
	if p.reflectionQuerier == nil {
		return nil, fmt.Errorf("reflection querier not configured")
	}

	audits, err := p.reflectionQuerier.FindRecentAudits(ctx, 3)
	if err != nil {
		return nil, fmt.Errorf("audit query failed: %w", err)
	}

	items := make([]agentruntime.ExperienceItem, 0, len(audits))
	for _, a := range audits {
		content := fmt.Sprintf("Decision: %s\nCorrection Hint: %s", a.Decision, a.CorrectionHint)
		items = append(items, agentruntime.ExperienceItem{
			ID:      a.AuditID,
			Summary: a.CorrectionHint,
			Content: content,
			Tags:    []string{"audit", "correction_pattern"},
			Metadata: map[string]any{
				"decision": a.Decision,
			},
		})
	}
	return items, nil
}

// fetchModelErrorLessons retrieves recent recoverable model errors for
// learning degradation strategies when LLM calls fail.
func (p *ExperienceProviderAdapter) fetchModelErrorLessons(ctx context.Context) ([]agentruntime.ExperienceItem, error) {
	if p.reflectionQuerier == nil {
		return nil, fmt.Errorf("reflection querier not configured")
	}

	modelErrors, err := p.reflectionQuerier.FindRecentModelErrors(ctx, 3)
	if err != nil {
		return nil, fmt.Errorf("model error query failed: %w", err)
	}

	items := make([]agentruntime.ExperienceItem, 0, len(modelErrors))
	for _, me := range modelErrors {
		content := fmt.Sprintf("Purpose: %s\nError Kind: %s\nMessage: %s", me.Purpose, me.ErrorKind, me.Message)
		items = append(items, agentruntime.ExperienceItem{
			ID:      me.CallID,
			Summary: fmt.Sprintf("[%s] %s: %s", me.Purpose, me.ErrorKind, me.Message),
			Content: content,
			Tags:    []string{"model_error", "degradation"},
			Metadata: map[string]any{
				"purpose":    me.Purpose,
				"error_kind": me.ErrorKind,
			},
		})
	}
	return items, nil
}

// fetchRecentSuccesses is the degradation path when the vector store is
// unavailable. It returns recently successful agent executions.
func (p *ExperienceProviderAdapter) fetchRecentSuccesses(ctx context.Context, limit int) ([]agentruntime.ExperienceItem, error) {
	if p.reflectionQuerier == nil {
		return nil, fmt.Errorf("reflection querier not configured")
	}

	summaries, err := p.reflectionQuerier.FindSuccessfulSummaries(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("recent success query failed: %w", err)
	}

	items := make([]agentruntime.ExperienceItem, 0, len(summaries))
	for _, s := range summaries {
		items = append(items, agentruntime.ExperienceItem{
			ID:      s.TaskID,
			Summary: s.FinalAnswer,
			Content: s.FinalAnswer,
			Tags:    []string{"recent_success"},
			Metadata: map[string]any{
				"task_id": s.TaskID,
			},
		})
	}
	return items, nil
}

// formatSimilarCase renders a SimilarAnalysis into a human-readable content
// string suitable for LLM consumption.
func formatSimilarCase(r service.SimilarAnalysis) string {
	return fmt.Sprintf(
		"Session: %s\nQuery: %s\nSummary: %s\nSimilarity: %.2f%%",
		r.SessionID, r.InitialQuery, r.Summary, r.Similarity*100,
	)
}

// ReflectionQuerierAdapter wraps an AgentExecutionRepository to satisfy
// the ReflectionQuerier interface by converting model types to summary types.
type ReflectionQuerierAdapter struct {
	repo *repository.AgentExecutionRepository
}

func NewReflectionQuerierAdapter(repo *repository.AgentExecutionRepository) *ReflectionQuerierAdapter {
	return &ReflectionQuerierAdapter{repo: repo}
}

func (a *ReflectionQuerierAdapter) FindFailedReflections(ctx context.Context, limit int) ([]AgentReflectionSummary, error) {
	reflections, err := a.repo.FindFailedReflections(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]AgentReflectionSummary, 0, len(reflections))
	for _, r := range reflections {
		result = append(result, AgentReflectionSummary{
			ReflectionID:   r.ReflectionID,
			RootCause:      r.RootCause,
			Impact:         r.Impact,
			ReusableLesson: r.ReusableLesson,
		})
	}
	return result, nil
}

func (a *ReflectionQuerierAdapter) FindSuccessfulSummaries(ctx context.Context, limit int) ([]AgentExecutionSummary, error) {
	execs, err := a.repo.FindSuccessfulSummaries(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]AgentExecutionSummary, 0, len(execs))
	for _, e := range execs {
		result = append(result, AgentExecutionSummary{
			TaskID:      e.TaskID,
			FinalAnswer: e.FinalAnswer,
		})
	}
	return result, nil
}

func (a *ReflectionQuerierAdapter) FindRecentAudits(ctx context.Context, limit int) ([]AgentAuditSummary, error) {
	audits, err := a.repo.FindRecentAudits(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]AgentAuditSummary, 0, len(audits))
	for _, au := range audits {
		result = append(result, AgentAuditSummary{
			AuditID:        au.AuditID,
			Decision:       au.Decision,
			CorrectionHint: au.CorrectionHint,
		})
	}
	return result, nil
}

func (a *ReflectionQuerierAdapter) FindRecentModelErrors(ctx context.Context, limit int) ([]AgentModelErrorSummary, error) {
	modelErrors, err := a.repo.FindRecentModelErrors(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]AgentModelErrorSummary, 0, len(modelErrors))
	for _, me := range modelErrors {
		result = append(result, AgentModelErrorSummary{
			CallID:    me.CallID,
			Purpose:   me.Purpose,
			ErrorKind: me.ErrorKind,
			Message:   me.Message,
		})
	}
	return result, nil
}
