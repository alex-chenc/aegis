package assistant

import (
	"context"

	"api-server/internal/repository"
	"go.uber.org/zap"
)

// ToolDecisionRecorder persists tool decision records to session metadata.
// Phase 1: records are stored in the AssistantSession.Metadata JSONB column.
// Phase 2: may migrate to a dedicated assistant_tool_decision_records table.
type ToolDecisionRecorder struct {
	sessionRepo repository.AssistantSessionRepository
	logger      *zap.Logger
}

// NewToolDecisionRecorder creates a recorder. If sessionRepo is nil, all
// operations are silently skipped (degraded mode).
func NewToolDecisionRecorder(sessionRepo repository.AssistantSessionRepository, logger *zap.Logger) *ToolDecisionRecorder {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ToolDecisionRecorder{
		sessionRepo: sessionRepo,
		logger:      logger,
	}
}

// Record persists the decision records from a ToolExecutionPlan into the
// session's metadata. It merges with existing metadata so prior keys are
// preserved.
func (r *ToolDecisionRecorder) Record(ctx context.Context, sessionID string, plan *ToolExecutionPlan) error {
	if r == nil || r.sessionRepo == nil || sessionID == "" || plan == nil {
		return nil
	}

	session, err := r.sessionRepo.FindBySessionID(ctx, sessionID)
	if err != nil {
		r.logger.Warn("tool decision recorder: failed to load session",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		return err
	}

	metadata := unmarshalJSON(session.Metadata)
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	metadata["decision_trace_id"] = plan.DecisionTraceID
	metadata["tool_decision_records"] = plan.DecisionRecords
	metadata["rejected_tool_records"] = plan.RejectedToolRecords
	if plan.NeedClarification {
		metadata["current_run_status"] = "clarification_required"
	}

	session.Metadata = mustMarshalJSON(metadata)
	if err := r.sessionRepo.Update(ctx, session); err != nil {
		r.logger.Warn("tool decision recorder: failed to save records",
			zap.String("session_id", sessionID),
			zap.String("trace_id", plan.DecisionTraceID),
			zap.Error(err),
		)
		return err
	}

	r.logger.Debug("tool decision recorder: records saved",
		zap.String("session_id", sessionID),
		zap.String("trace_id", plan.DecisionTraceID),
		zap.Int("accepted_records", len(plan.DecisionRecords)),
		zap.Int("rejected_records", len(plan.RejectedToolRecords)),
	)
	return nil
}
