package sessionaudit

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const Topic = "aegis.agent.sessions.v1"

type Batch struct {
	Schema                     string          `json:"schema"`
	HostID                     string          `json:"host_id"`
	AgentType                  string          `json:"agent_type"`
	SourceSubjectUID           int64           `json:"source_subject_uid"`
	SourceSessionID            string          `json:"source_session_id"`
	SourceMode                 string          `json:"source_mode"`
	SourceStorageNamespaceHash string          `json:"source_storage_namespace_hash"`
	Model                      string          `json:"model"`
	SessionMetadataJSON        json.RawMessage `json:"session_metadata_json"`
	Items                      []Item          `json:"items"`
}
type Item struct {
	ItemID             string          `json:"item_id"`
	SourceSequence     uint64          `json:"source_sequence"`
	ItemType           string          `json:"item_type"`
	Role               string          `json:"role"`
	OccurredAtUnixNano int64           `json:"occurred_at_unix_nano"`
	NormalizedJSON     json.RawMessage `json:"normalized_json"`
	ContentDigest      string          `json:"content_digest"`
	SourceUsage        Usage           `json:"source_usage"`
}
type Usage struct {
	InputTokens  *int64 `json:"input_tokens,omitempty"`
	OutputTokens *int64 `json:"output_tokens,omitempty"`
}

type Projector struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewProjector(db *gorm.DB, logger *zap.Logger) *Projector {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Projector{db: db, logger: logger}
}

func (p *Projector) Project(ctx context.Context, value []byte) error {
	var batch Batch
	if err := json.Unmarshal(value, &batch); err != nil {
		return fmt.Errorf("decode session batch: %w", err)
	}
	if batch.Schema != "aegis.agent_session_batch.v1" || batch.SourceSessionID == "" || batch.HostID == "" || len(batch.Items) == 0 {
		return fmt.Errorf("invalid session batch envelope")
	}
	hostID, err := uuid.Parse(batch.HostID)
	if err != nil {
		return fmt.Errorf("host id: %w", err)
	}
	if batch.AgentType != "claude-code" && batch.AgentType != "codex" {
		return fmt.Errorf("unsupported agent type")
	}
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sessionID := uuid.New()
		metadata := map[string]any{}
		metadataRaw := batch.SessionMetadataJSON
		if decoded, decodeErr := decodeBytes(metadataRaw); decodeErr == nil {
			metadataRaw = decoded
		}
		_ = json.Unmarshal(metadataRaw, &metadata)
		modelName, _ := metadata["model"].(string)
		if modelName == "" {
			modelName = batch.Model
		}
		projectDigest := batch.SourceStorageNamespaceHash
		if projectDigest == "" {
			if v, ok := metadata["project_root_hash"].(string); ok {
				projectDigest = v
			}
		}
		if err := tx.Exec(`INSERT INTO agent_conversation_sessions (id,host_id,agent_type,source_mode,source_subject_uid,external_session_id,project_digest,model,state,last_seen_at,last_collected_at,last_sequence) VALUES (?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT (host_id,agent_type,source_subject_uid,external_session_id) DO UPDATE SET model=COALESCE(NULLIF(EXCLUDED.model,''),agent_conversation_sessions.model), project_digest=COALESCE(NULLIF(EXCLUDED.project_digest,''),agent_conversation_sessions.project_digest), last_seen_at=GREATEST(agent_conversation_sessions.last_seen_at,EXCLUDED.last_seen_at), last_collected_at=EXCLUDED.last_collected_at, last_sequence=GREATEST(agent_conversation_sessions.last_sequence,EXCLUDED.last_sequence), updated_at=now()`, sessionID, hostID, batch.AgentType, batch.SourceMode, batch.SourceSubjectUID, batch.SourceSessionID, projectDigest, modelName, "active_inferred", time.Now().UTC(), time.Now().UTC(), batch.Items[len(batch.Items)-1].SourceSequence).Error; err != nil {
			return err
		}
		var stored struct{ ID uuid.UUID }
		if err := tx.Raw(`SELECT id FROM agent_conversation_sessions WHERE host_id=? AND agent_type=? AND source_subject_uid=? AND external_session_id=?`, hostID, batch.AgentType, batch.SourceSubjectUID, batch.SourceSessionID).Scan(&stored).Error; err != nil {
			return err
		}
		for _, item := range batch.Items {
			normalized := item.NormalizedJSON
			if decoded, decodeErr := decodeBytes(normalized); decodeErr == nil {
				normalized = decoded
			}
			var payload map[string]any
			if json.Unmarshal(normalized, &payload) != nil {
				continue
			}
			content, _ := payload["content"].(string)
			total := int64((len([]byte(content)) + 3) / 4)
			input, output := total, int64(0)
			if item.SourceUsage.InputTokens != nil {
				input = *item.SourceUsage.InputTokens
			}
			if item.SourceUsage.OutputTokens != nil {
				output = *item.SourceUsage.OutputTokens
				if item.SourceUsage.InputTokens != nil {
					total = input + output
				}
			}
			occurred := (*time.Time)(nil)
			if item.OccurredAtUnixNano != 0 {
				t := time.Unix(0, item.OccurredAtUnixNano).UTC()
				occurred = &t
			}
			itemUUID := uuid.New()
			if err := tx.Exec(`INSERT INTO agent_conversation_items (id,session_id,item_id,sequence,item_type,role,occurred_at,content_digest,content_redacted,normalized_json,visibility,redaction_applied,input_tokens,output_tokens,total_tokens) VALUES (?,?,?,?,?,?,?,?,?,?::jsonb,?,?,?,?,?) ON CONFLICT (session_id,item_id) DO NOTHING`, itemUUID, stored.ID, item.ItemID, item.SourceSequence, item.ItemType, item.Role, occurred, item.ContentDigest, content, string(normalized), visibility(payload), redactionApplied(payload), input, output, total).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`UPDATE agent_conversation_sessions s SET item_count=(SELECT count(*) FROM agent_conversation_items i WHERE i.session_id=s.id), prompt_count=(SELECT count(*) FROM agent_conversation_items i WHERE i.session_id=s.id AND i.item_type='user_message'), assistant_count=(SELECT count(*) FROM agent_conversation_items i WHERE i.session_id=s.id AND i.item_type='assistant_message'), tool_call_count=(SELECT count(*) FROM agent_conversation_items i WHERE i.session_id=s.id AND i.item_type='tool_call'), estimated_input_tokens=COALESCE((SELECT sum(COALESCE(i.input_tokens,0)) FROM agent_conversation_items i WHERE i.session_id=s.id),0), estimated_output_tokens=COALESCE((SELECT sum(COALESCE(i.output_tokens,0)) FROM agent_conversation_items i WHERE i.session_id=s.id),0), estimated_total_tokens=COALESCE((SELECT sum(COALESCE(i.total_tokens,0)) FROM agent_conversation_items i WHERE i.session_id=s.id),0), updated_at=now() WHERE s.id=?`, stored.ID).Error; err != nil {
			return err
		}
		return nil
	})
}

func decodeBytes(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return []byte("{}"), nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return base64.StdEncoding.DecodeString(text)
	}
	return raw, nil
}
func visibility(v map[string]any) string {
	if x, ok := v["visibility"].(string); ok && x != "" {
		return x
	}
	return "normal"
}
func redactionApplied(v map[string]any) bool {
	x, ok := v["redaction_state"].(string)
	return ok && x != "" && x != "none"
}
func Digest(s string) string {
	d := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(d[:])
}
func sanitizeForTest(s string) string { return strings.ReplaceAll(s, "\n", " ") }
