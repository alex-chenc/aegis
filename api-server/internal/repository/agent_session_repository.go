package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"api-server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AgentSessionRepository struct{ db *gorm.DB }

func NewAgentSessionRepository(db *gorm.DB) *AgentSessionRepository {
	return &AgentSessionRepository{db: db}
}

func (r *AgentSessionRepository) UpsertBatch(ctx context.Context, hostID uuid.UUID, sourceUID int64, agentType string, items []AgentSessionItemInput) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		byExternal := make(map[string]*model.AgentConversationSession)
		for _, input := range items {
			if strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.ItemID) == "" {
				continue
			}
			key := agentType + "\x00" + input.SessionID
			session := byExternal[key]
			if session == nil {
				created := false
				session = &model.AgentConversationSession{}
				err := tx.Where("host_id = ? AND agent_type = ? AND source_subject_uid = ? AND external_session_id = ?", hostID, agentType, sourceUID, input.SessionID).First(session).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					session = &model.AgentConversationSession{ID: uuid.New(), HostID: hostID, AgentType: agentType, SourceMode: model.AgentSessionModeStatic, SourceSubjectUID: sourceUID, ExternalSessionID: input.SessionID, State: model.AgentSessionStateUnknown, RiskLevel: "unknown", LastSequence: -1}
					created = true
				} else if err != nil {
					return fmt.Errorf("load agent conversation session: %w", err)
				}
				byExternal[key] = session
				if created {
					if err := tx.Create(session).Error; err != nil {
						return fmt.Errorf("create agent conversation session: %w", err)
					}
				}
			}
			if input.ProjectDigest != "" {
				session.ProjectDigest = input.ProjectDigest
			}
			if input.Model != "" {
				session.Model = input.Model
			}
			if input.OccurredAt != nil {
				if session.FirstSeenAt == nil || input.OccurredAt.Before(*session.FirstSeenAt) {
					v := *input.OccurredAt
					session.FirstSeenAt = &v
				}
				if session.LastSeenAt == nil || input.OccurredAt.After(*session.LastSeenAt) {
					v := *input.OccurredAt
					session.LastSeenAt = &v
					session.LastItemAt = &v
				}
			}
			if input.Sequence <= session.LastSequence {
				continue
			}
			item := &model.AgentConversationItem{ID: uuid.New(), SessionID: session.ID, ItemID: input.ItemID, Sequence: input.Sequence, ItemType: input.ItemType, Role: input.Role, OccurredAt: input.OccurredAt, ContentDigest: input.ContentDigest, ContentRedacted: input.ContentRedacted, NormalizedJSON: input.NormalizedJSON, Visibility: input.Visibility, RedactionApplied: input.RedactionApplied, InputTokens: input.InputTokens, OutputTokens: input.OutputTokens, TotalTokens: input.TotalTokens}
			if err := tx.Create(item).Error; err != nil {
				// Retries are expected: an item ID is the idempotency key.
				if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
					continue
				}
				return fmt.Errorf("persist agent conversation item: %w", err)
			}
			session.ItemCount++
			session.LastSequence = input.Sequence
			switch input.ItemType {
			case "user_message":
				session.PromptCount++
			case "assistant_message":
				session.AssistantCount++
			case "tool_call":
				session.ToolCallCount++
			}
			session.EstimatedInputTokens += input.InputTokensValue
			session.EstimatedOutputTokens += input.OutputTokensValue
			session.EstimatedTotalTokens += input.TotalTokensValue
		}
		for _, session := range byExternal {
			session.LastCollectedAt = ptrTime(time.Now().UTC())
			if session.LastSeenAt != nil && time.Since(*session.LastSeenAt) <= 2*time.Minute {
				session.State = model.AgentSessionStateActive
			} else if session.LastSeenAt != nil {
				session.State = model.AgentSessionStateIdle
			}
			if err := tx.Save(session).Error; err != nil {
				return fmt.Errorf("save agent conversation session: %w", err)
			}
		}
		return nil
	})
}

type AgentSessionItemInput struct {
	SessionID, ItemID, ItemType, Role, ProjectDigest, Model string
	Sequence                                                int64
	OccurredAt                                              *time.Time
	ContentDigest, ContentRedacted, Visibility              string
	NormalizedJSON                                          []byte
	RedactionApplied                                        bool
	InputTokens, OutputTokens, TotalTokens                  *int64
	InputTokensValue, OutputTokensValue, TotalTokensValue   int64
}

func ptrTime(t time.Time) *time.Time { return &t }

func (r *AgentSessionRepository) List(ctx context.Context, hostID *uuid.UUID, agentType string, risk string, page, pageSize int) ([]model.AgentConversationSession, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.AgentConversationSession{})
	if hostID != nil {
		q = q.Where("host_id = ?", *hostID)
	}
	if agentType != "" {
		q = q.Where("agent_type = ?", agentType)
	}
	if risk != "" {
		q = q.Where("risk_level = ?", risk)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.AgentConversationSession
	if err := q.Order("COALESCE(last_seen_at, created_at) DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *AgentSessionRepository) Get(ctx context.Context, id uuid.UUID) (*model.AgentConversationSession, error) {
	var row model.AgentConversationSession
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *AgentSessionRepository) ListItems(ctx context.Context, sessionID uuid.UUID, limit int) ([]model.AgentConversationItem, error) {
	if limit < 1 || limit > 1000 {
		limit = 500
	}
	var rows []model.AgentConversationItem
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("sequence ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *AgentSessionRepository) CreateAIRun(ctx context.Context, run *model.AgentSessionAIRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}
func (r *AgentSessionRepository) UpdateAIRun(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	return r.db.WithContext(ctx).Model(&model.AgentSessionAIRun{}).Where("id = ?", id).Updates(updates).Error
}
func (r *AgentSessionRepository) CreateAIChunk(ctx context.Context, chunk *model.AgentSessionAIChunk) error {
	return r.db.WithContext(ctx).Create(chunk).Error
}

func (r *AgentSessionRepository) SaveRuleHits(ctx context.Context, hits []model.AgentSessionRuleHit) error {
	if len(hits) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range hits {
			if err := tx.Where("session_id = ? AND item_id IS NOT DISTINCT FROM ? AND rule_key = ? AND evidence_digest = ?", hits[i].SessionID, hits[i].ItemID, hits[i].RuleKey, hits[i].EvidenceDigest).FirstOrCreate(&hits[i]).Error; err != nil {
				return err
			}
		}
		if len(hits) > 0 {
			var count int64
			if err := tx.Model(&model.AgentSessionRuleHit{}).Where("session_id = ?", hits[0].SessionID).Count(&count).Error; err != nil {
				return err
			}
			level := model.AgentSessionRiskLow
			for _, hit := range hits {
				if hit.Severity == "critical" {
					level = model.AgentSessionRiskCritical
					break
				}
				if hit.Severity == "high" {
					level = model.AgentSessionRiskHigh
				} else if hit.Severity == "medium" && level == model.AgentSessionRiskLow {
					level = model.AgentSessionRiskMedium
				}
			}
			if err := tx.Model(&model.AgentConversationSession{}).Where("id = ?", hits[0].SessionID).Updates(map[string]any{"rule_hit_count": count, "risk_level": level}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *AgentSessionRepository) ListRuleHits(ctx context.Context, sessionID uuid.UUID) ([]model.AgentSessionRuleHit, error) {
	var hits []model.AgentSessionRuleHit
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("created_at DESC").Limit(500).Find(&hits).Error; err != nil {
		return nil, err
	}
	return hits, nil
}

func (r *AgentSessionRepository) GetLatestAIRun(ctx context.Context, sessionID uuid.UUID) (*model.AgentSessionAIRun, error) {
	var run model.AgentSessionAIRun
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("created_at DESC").First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *AgentSessionRepository) ListAIChunks(ctx context.Context, runID uuid.UUID) ([]model.AgentSessionAIChunk, error) {
	var chunks []model.AgentSessionAIChunk
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).Order("chunk_index ASC").Limit(100).Find(&chunks).Error; err != nil {
		return nil, err
	}
	return chunks, nil
}

func (r *AgentSessionRepository) GetByNaturalKey(ctx context.Context, hostID uuid.UUID, externalSessionID string) (*model.AgentConversationSession, error) {
	var row model.AgentConversationSession
	if err := r.db.WithContext(ctx).Where("host_id = ? AND external_session_id = ?", hostID, externalSessionID).Order("updated_at DESC").First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func normalizeJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return []byte("{}")
	}
	b, _ := json.Marshal(v)
	return b
}
