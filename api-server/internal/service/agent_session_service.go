package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	AgentSessionRulePromptVersion = "agent-session-v6.3-p0"
	AgentSessionMaxChunkTokens    = 4096
	AgentSessionMaxChunkBytes     = 16 * 1024
	AgentSessionMaxAIChunks       = 64
)

type AgentSessionAIClient interface {
	Complete(context.Context, []Message, string) (content, provider, modelName string, usage TokenUsage, err error)
}

type AgentSessionCollectionDispatcher interface {
	SyncAgentConfig(context.Context, string, []*pb.AgentConfig) (int32, error)
}

type Message struct{ Role, Content string }
type TokenUsage struct{ Input, Output, Total int }

// AgentSessionService owns the server-side projection and analysis pipeline.
// It only accepts normalized, already-redacted payloads emitted by Agent.
type AgentSessionService struct {
	repo       *repository.AgentSessionRepository
	ai         AgentSessionAIClient
	logger     *zap.Logger
	dispatcher AgentSessionCollectionDispatcher
}

func NewAgentSessionService(repo *repository.AgentSessionRepository, ai AgentSessionAIClient, logger *zap.Logger) *AgentSessionService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentSessionService{repo: repo, ai: ai, logger: logger}
}

func (s *AgentSessionService) SetCollectionDispatcher(dispatcher AgentSessionCollectionDispatcher) {
	if s == nil {
		return
	}
	s.dispatcher = dispatcher
}

type AgentSessionCollectionResult struct {
	HostID    string `json:"host_id"`
	AgentType string `json:"agent_type"`
	Accepted  bool   `json:"accepted"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

// RequestCollection dispatches an additive config-sync command to the
// selected host. The Agent handles this as a bounded static scan; it does not
// install a hook or receive any session content in the command path.
func (s *AgentSessionService) RequestCollection(ctx context.Context, hostID uuid.UUID, agentType string) (AgentSessionCollectionResult, error) {
	result := AgentSessionCollectionResult{HostID: hostID.String(), AgentType: agentType, Status: "rejected"}
	if s == nil || s.dispatcher == nil {
		return result, errors.New("agent session collection dispatcher is not configured")
	}
	if agentType != model.AgentSessionSourceClaude && agentType != model.AgentSessionSourceCodex {
		return result, errors.New("agent session source is unsupported")
	}
	payload, err := json.Marshal(map[string]string{
		"schema":     "aegis.agent_session_collect.v1",
		"agent_type": agentType,
		"mode":       model.AgentSessionModeStatic,
	})
	if err != nil {
		return result, err
	}
	affected, err := s.dispatcher.SyncAgentConfig(ctx, hostID.String(), []*pb.AgentConfig{{
		ConfigType: "agent_session_collect",
		ConfigJson: string(payload),
	}})
	if err != nil {
		// Static collection is independent from the Aegis Agent Guard runtime
		// status. If the host-side collector is temporarily disconnected, keep
		// the request pending so the next reconnect can execute the scan rather
		// than surfacing a misleading runtime error to the user.
		if strings.Contains(strings.ToLower(err.Error()), "agent not connected") {
			result.Status = "pending_reconnect"
			result.Message = "Agent 未连接，重连后将执行静态会话采集"
			return result, nil
		}
		return result, fmt.Errorf("dispatch agent session collection: %w", err)
	}
	if affected == 0 {
		result.Status = "pending_reconnect"
		result.Message = "Agent 未连接，重连后将执行静态会话采集"
		return result, nil
	}
	result.Accepted = true
	result.Status = "accepted"
	result.Message = "static session collection requested"
	return result, nil
}

func (s *AgentSessionService) IngestBatch(ctx context.Context, req *pb.AgentSessionBatchRequest) error {
	if req == nil || req.HostId == "" || req.SourceSessionId == "" {
		return errors.New("agent session batch identity is required")
	}
	hostID, err := uuid.Parse(req.HostId)
	if err != nil {
		return fmt.Errorf("host id: %w", err)
	}
	if req.AgentType != model.AgentSessionSourceClaude && req.AgentType != model.AgentSessionSourceCodex {
		return errors.New("agent session source is unsupported")
	}
	inputs := make([]repositoryInput, 0, len(req.Items))
	for _, item := range req.Items {
		if item == nil || strings.TrimSpace(item.ItemId) == "" {
			continue
		}
		if len(item.NormalizedJson) == 0 || len(item.NormalizedJson) > 256*1024 {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(item.NormalizedJson, &payload); err != nil {
			continue
		}
		content, _ := payload["content"].(string)
		if len(content) > 64*1024 {
			content = content[:64*1024]
		}
		usage := item.GetSourceUsage()
		inTok, outTok, totalTok := int64(usage.GetInputTokens()), int64(usage.GetOutputTokens()), int64(usage.GetInputTokens()+usage.GetOutputTokens())
		if totalTok == 0 {
			totalTok = estimateTokens(content)
			if item.ItemType == "user_message" {
				inTok = totalTok
			} else {
				outTok = totalTok
			}
		}
		redacted := payload["redaction_state"] != nil && payload["redaction_state"] != "none"
		inputs = append(inputs, repositoryInput{SessionID: req.SourceSessionId, ItemID: item.ItemId, ItemType: item.ItemType, Role: item.Role, Sequence: int64(item.SourceSequence), OccurredAt: parseOccurred(item.OccurredAtUnixNano), ProjectDigest: req.SourceStorageNamespaceHash, Model: metadataModel(req.SessionMetadataJson), ContentDigest: item.ContentDigest, ContentRedacted: content, Visibility: metadataVisibility(payload), NormalizedJSON: datatypes.JSON(payloadBytes(payload)), RedactionApplied: redacted, InputTokens: &inTok, OutputTokens: &outTok, TotalTokens: &totalTok, InputTokensValue: inTok, OutputTokensValue: outTok, TotalTokensValue: totalTok})
	}
	if err := s.repo.UpsertBatch(ctx, hostID, req.SourceSubjectUid, req.AgentType, toRepoInputs(inputs)); err != nil {
		return err
	}
	// Rules are deliberately evaluated after persistence so a retry cannot
	// produce an alert for an item which was rejected by the projection.
	if len(inputs) > 0 {
		if err := s.evaluateRules(ctx, hostID, req.SourceSessionId, inputs); err != nil {
			return err
		}
	}
	s.logger.Info("agent_session_batch_projected", zap.String("host_id", hostID.String()), zap.String("agent_type", req.AgentType), zap.Int("item_count", len(inputs)))
	return nil
}

func (s *AgentSessionService) HandleKafkaMessage(ctx context.Context, _key, value []byte) error {
	var req pb.AgentSessionBatchRequest
	if err := proto.Unmarshal(value, &req); err != nil || req.SourceSessionId == "" {
		// Server's generic Kafka producer serializes protobuf messages as JSON;
		// retain protobuf decoding for future binary producers and accept only the
		// documented JSON representation as the compatibility path.
		if jsonErr := json.Unmarshal(value, &req); jsonErr != nil {
			return fmt.Errorf("decode agent session batch: %w", jsonErr)
		}
	}
	return s.IngestBatch(ctx, &req)
}

// repositoryInput mirrors the unexported repository input without leaking it
// through the repository package's public surface.
type repositoryInput = repository.AgentSessionItemInput

func toRepoInputs(in []repositoryInput) []repository.AgentSessionItemInput { return in }

func parseOccurred(ns int64) *time.Time {
	if ns == 0 {
		return nil
	}
	t := time.Unix(0, ns).UTC()
	return &t
}
func metadataModel(raw []byte) string {
	var v map[string]string
	if json.Unmarshal(raw, &v) == nil {
		return v["model"]
	}
	return ""
}
func metadataVisibility(payload map[string]any) string {
	if v, ok := payload["visibility"].(string); ok && v != "" {
		return v
	}
	return "normal"
}
func payloadBytes(v map[string]any) []byte { b, _ := json.Marshal(v); return b }

func estimateTokens(text string) int64 {
	if text == "" {
		return 0
	}
	// This is a deterministic upper-bound estimate, intentionally advertised in
	// the UI as chars_div_4 rather than pretending to be model-token exact.
	return int64((len([]byte(text)) + 3) / 4)
}

type sessionRule struct {
	key, name, category, severity string
	re                            *regexp.Regexp
	description                   string
}

var sessionRules = []sessionRule{
	{key: "ASR-PROMPT-001", name: "提示词越权指令", category: "prompt_injection", severity: "high", re: regexp.MustCompile(`(?is)ignore\s+(all|any|previous|prior)\s+instructions|disregard\s+.*system\s+message`), description: "检测试图覆盖高优先级指令的提示词"},
	{key: "ASR-PROMPT-002", name: "隐藏提示词探测", category: "prompt_injection", severity: "high", re: regexp.MustCompile(`(?is)reveal\s+(the\s+)?(system|developer)\s+prompt|show\s+hidden\s+instructions`), description: "检测索取系统提示词、开发者提示词或隐藏指令的请求"},
	{key: "ASR-PROMPT-003", name: "越狱与安全绕过", category: "jailbreak", severity: "critical", re: regexp.MustCompile(`(?is)\bDAN\b|developer\s+mode|bypass\s+(all\s+)?safety|disable\s+safety`), description: "检测越狱、开发者模式或绕过安全限制的表达"},
	{key: "ASR-SECRET-001", name: "凭据与密钥暴露", category: "secret_exposure", severity: "critical", re: regexp.MustCompile(`(?i)(sk-[a-z0-9]{16,}|ghp_[a-z0-9]{20,}|bearer\s+[a-z0-9._-]{16,}|-----BEGIN [A-Z ]+PRIVATE KEY-----)`), description: "检测会话中出现疑似 API 密钥、令牌或私钥内容"},
	{key: "ASR-TOOL-001", name: "高风险工具与命令", category: "tool_abuse", severity: "high", re: regexp.MustCompile(`(?is)(curl|wget|nc|bash|sh)\s+.*(http|/etc/|/root/)|rm\s+-rf\s+/`), description: "检测高风险 Shell、网络访问或破坏性文件操作"},
}

type AgentSessionRuleSummary struct {
	RuleKey           string   `json:"rule_key"`
	RuleVersion       int64    `json:"rule_version"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Source            string   `json:"source"`
	Engine            string   `json:"engine"`
	Categories        []string `json:"categories"`
	DefaultEnabled    bool     `json:"default_enabled"`
	DefaultSeverity   string   `json:"default_severity"`
	DefaultAction     string   `json:"default_action"`
	RecommendedAction string   `json:"recommended_action"`
	Immutable         bool     `json:"immutable"`
	Digest            string   `json:"digest"`
}

func (s *AgentSessionService) BuiltinRules() []AgentSessionRuleSummary {
	rules := make([]AgentSessionRuleSummary, 0, len(sessionRules))
	for _, rule := range sessionRules {
		digest := sha256.Sum256([]byte(strings.Join([]string{rule.key, rule.name, rule.category, rule.severity, rule.re.String(), rule.description}, "\x00")))
		rules = append(rules, AgentSessionRuleSummary{
			RuleKey:           rule.key,
			RuleVersion:       1,
			Name:              rule.name,
			Description:       rule.description,
			Source:            "builtin",
			Engine:            "api_session_static",
			Categories:        []string{rule.category},
			DefaultEnabled:    true,
			DefaultSeverity:   rule.severity,
			DefaultAction:     "alert",
			RecommendedAction: "alert",
			Immutable:         true,
			Digest:            "sha256:" + hex.EncodeToString(digest[:]),
		})
	}
	return rules
}

func (s *AgentSessionService) evaluateRules(ctx context.Context, hostID uuid.UUID, externalSessionID string, inputs []repositoryInput) error {
	// Rule hits are persisted by session after the ingestion transaction. The
	// session row ID is looked up by the stable natural key.
	session, err := s.repo.GetByNaturalKey(ctx, hostID, inputs[0].SessionID)
	if err != nil {
		return err
	}
	items, err := s.repo.ListItems(ctx, session.ID, 1000)
	if err != nil {
		return err
	}
	itemIDs := make(map[string]uuid.UUID, len(items))
	for _, item := range items {
		itemIDs[item.ItemID] = item.ID
	}
	hits := make([]model.AgentSessionRuleHit, 0)
	for _, input := range inputs {
		if input.ContentRedacted == "" {
			continue
		}
		for _, rule := range sessionRules {
			if !rule.re.MatchString(input.ContentRedacted) {
				continue
			}
			excerpt := rule.re.FindString(input.ContentRedacted)
			if len(excerpt) > 256 {
				excerpt = excerpt[:256]
			}
			digest := sha256.Sum256([]byte(excerpt))
			itemID := itemIDs[input.ItemID]
			hit := model.AgentSessionRuleHit{ID: uuid.New(), SessionID: session.ID, RuleKey: rule.key, Severity: rule.severity, Category: rule.category, EvidenceDigest: "sha256:" + hex.EncodeToString(digest[:]), EvidenceExcerpt: excerpt, Status: "open"}
			if itemID != uuid.Nil {
				hit.ItemID = &itemID
			}
			hits = append(hits, hit)
		}
	}
	return s.repo.SaveRuleHits(ctx, hits)
}

type AgentSessionListResult struct {
	Items    []model.AgentConversationSession `json:"items"`
	Total    int64                            `json:"total"`
	Page     int                              `json:"page"`
	PageSize int                              `json:"page_size"`
}

type AgentSessionAIResult struct {
	RunID         string                      `json:"run_id,omitempty"`
	Status        string                      `json:"status"`
	PromptVersion string                      `json:"prompt_version"`
	Provider      string                      `json:"provider,omitempty"`
	Model         string                      `json:"model,omitempty"`
	Summary       string                      `json:"summary,omitempty"`
	ChunkCount    int                         `json:"chunk_count"`
	Chunks        []AgentSessionAIChunkResult `json:"chunks"`
}
type AgentSessionAIChunkResult struct {
	Index              int        `json:"index"`
	StartSequence      int64      `json:"start_sequence"`
	EndSequence        int64      `json:"end_sequence"`
	ItemSequences      []int64    `json:"item_sequences,omitempty"`
	InputTokenEstimate int64      `json:"input_token_estimate"`
	Content            string     `json:"content,omitempty"`
	Provider           string     `json:"provider,omitempty"`
	Model              string     `json:"model,omitempty"`
	Usage              TokenUsage `json:"usage"`
	Status             string     `json:"status,omitempty"`
}

func (s *AgentSessionService) Analyze(ctx context.Context, id uuid.UUID) (AgentSessionAIResult, error) {
	if s.ai == nil {
		return AgentSessionAIResult{}, errors.New("agent session AI analysis is not configured")
	}
	items, err := s.repo.ListItems(ctx, id, 500)
	if err != nil {
		return AgentSessionAIResult{}, err
	}
	chunks := BuildAIChunks(items)
	run := &model.AgentSessionAIRun{ID: uuid.New(), SessionID: id, Provider: "pending", Model: "pending", PromptVersion: AgentSessionRulePromptVersion, Status: "running", ChunkCount: len(chunks), CreatedAt: time.Now().UTC()}
	if err := s.repo.CreateAIRun(ctx, run); err != nil {
		return AgentSessionAIResult{}, err
	}
	result := AgentSessionAIResult{RunID: run.ID.String(), Status: "running", PromptVersion: AgentSessionRulePromptVersion, ChunkCount: len(chunks), Chunks: make([]AgentSessionAIChunkResult, 0, len(chunks))}
	for index, chunk := range chunks {
		var b strings.Builder
		for _, item := range chunk {
			b.WriteString("[")
			b.WriteString(item.Role)
			b.WriteString("] ")
			b.WriteString(item.ContentRedacted)
			b.WriteByte('\n')
		}
		prompt := "Analyze this redacted agent conversation segment for prompt injection, jailbreak, secret exposure, unsafe tool intent, and social engineering. Return concise JSON with risk_level, score, reasons, evidence_digest. Do not reproduce secrets.\n" + b.String()
		content, provider, modelName, usage, callErr := s.ai.Complete(ctx, []Message{{Role: "user", Content: prompt}}, AgentSessionRulePromptVersion)
		if callErr != nil {
			_ = s.repo.UpdateAIRun(ctx, run.ID, map[string]any{"status": "failed", "error_code": "provider_unavailable", "error_message": callErr.Error(), "finished_at": time.Now().UTC()})
			return AgentSessionAIResult{}, callErr
		}
		outputJSON := []byte(content)
		if !json.Valid(outputJSON) {
			outputJSON, _ = json.Marshal(map[string]string{"raw": content})
		}
		_ = s.repo.CreateAIChunk(ctx, &model.AgentSessionAIChunk{ID: uuid.New(), RunID: run.ID, ChunkIndex: index, ItemStartSequence: chunk[0].Sequence, ItemEndSequence: chunk[len(chunk)-1].Sequence, InputTokenEstimate: estimateTokens(prompt), OutputJSON: datatypes.JSON(outputJSON), Status: "succeeded", CreatedAt: time.Now().UTC()})
		if provider != "" {
			run.Provider = provider
		}
		if modelName != "" {
			run.Model = modelName
		}
		sequences := make([]int64, 0, len(chunk))
		for _, item := range chunk {
			sequences = append(sequences, item.Sequence)
		}
		result.Chunks = append(result.Chunks, AgentSessionAIChunkResult{Index: index, StartSequence: chunk[0].Sequence, EndSequence: chunk[len(chunk)-1].Sequence, ItemSequences: sequences, InputTokenEstimate: estimateTokens(prompt), Content: content, Provider: provider, Model: modelName, Usage: usage, Status: "succeeded"})
	}
	_ = s.repo.UpdateAIRun(ctx, run.ID, map[string]any{"status": "succeeded", "provider": run.Provider, "model": run.Model, "finished_at": time.Now().UTC()})
	result.Status = "succeeded"
	result.Provider = run.Provider
	result.Model = run.Model
	return result, nil
}

func (s *AgentSessionService) List(ctx context.Context, hostID *uuid.UUID, agentType, risk string, page, pageSize int) (AgentSessionListResult, error) {
	items, total, err := s.repo.List(ctx, hostID, agentType, risk, page, pageSize)
	return AgentSessionListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *AgentSessionService) Detail(ctx context.Context, id uuid.UUID, includeItems bool) (*model.AgentConversationSession, []model.AgentConversationItem, error) {
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if !includeItems {
		return session, nil, nil
	}
	items, err := s.repo.ListItems(ctx, id, 500)
	return session, items, err
}

func (s *AgentSessionService) RuleHits(ctx context.Context, id uuid.UUID) ([]model.AgentSessionRuleHit, error) {
	hits, err := s.repo.ListRuleHits(ctx, id)
	if err != nil || len(hits) == 0 {
		return hits, err
	}
	items, err := s.repo.ListItems(ctx, id, 1000)
	if err != nil {
		return nil, err
	}
	sequences := make(map[uuid.UUID]int64, len(items))
	for _, item := range items {
		sequences[item.ID] = item.Sequence
	}
	for i := range hits {
		if hits[i].ItemID != nil {
			if sequence, ok := sequences[*hits[i].ItemID]; ok {
				value := sequence
				hits[i].ItemSequence = &value
				continue
			}
		}
		// Older projections did not persist item_id. Resolve those hits by the
		// redacted evidence excerpt so existing sessions still get a useful UI
		// location without exposing raw source paths or content.
		if hits[i].EvidenceExcerpt != "" {
			for _, item := range items {
				if strings.Contains(item.ContentRedacted, hits[i].EvidenceExcerpt) {
					value := item.Sequence
					hits[i].ItemSequence = &value
					break
				}
			}
		}
	}
	return hits, nil
}

// GetAIAnalysis returns the latest persisted analysis and expands every chunk
// back to the exact item sequences shown in the conversation. A session with
// no previous run returns not_run instead of a 404 so the detail drawer can
// render rule evidence and the analysis state independently.
func (s *AgentSessionService) GetAIAnalysis(ctx context.Context, id uuid.UUID) (AgentSessionAIResult, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return AgentSessionAIResult{}, err
	}
	run, err := s.repo.GetLatestAIRun(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AgentSessionAIResult{Status: "not_run", PromptVersion: AgentSessionRulePromptVersion, Chunks: []AgentSessionAIChunkResult{}}, nil
	}
	if err != nil {
		return AgentSessionAIResult{}, err
	}
	chunks, err := s.repo.ListAIChunks(ctx, run.ID)
	if err != nil {
		return AgentSessionAIResult{}, err
	}
	items, err := s.repo.ListItems(ctx, id, 1000)
	if err != nil {
		return AgentSessionAIResult{}, err
	}
	result := AgentSessionAIResult{RunID: run.ID.String(), Status: run.Status, PromptVersion: run.PromptVersion, Provider: run.Provider, Model: run.Model, Summary: run.Summary, ChunkCount: len(chunks), Chunks: make([]AgentSessionAIChunkResult, 0, len(chunks))}
	for _, chunk := range chunks {
		sequences := make([]int64, 0)
		for _, item := range items {
			if item.Sequence >= chunk.ItemStartSequence && item.Sequence <= chunk.ItemEndSequence {
				sequences = append(sequences, item.Sequence)
			}
		}
		result.Chunks = append(result.Chunks, AgentSessionAIChunkResult{Index: chunk.ChunkIndex, StartSequence: chunk.ItemStartSequence, EndSequence: chunk.ItemEndSequence, ItemSequences: sequences, InputTokenEstimate: chunk.InputTokenEstimate, Content: string(chunk.OutputJSON), Status: chunk.Status, Provider: run.Provider, Model: run.Model})
	}
	return result, nil
}

// BuildAIChunks bounds each prompt by both estimated tokens and bytes. It is
// shared by the HTTP handler and tests; no chunk contains partial UTF-8.
func BuildAIChunks(items []model.AgentConversationItem) [][]model.AgentConversationItem {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
	chunks := make([][]model.AgentConversationItem, 0)
	current := make([]model.AgentConversationItem, 0)
	tokens, bytes := 0, 0
	flush := func() {
		if len(current) > 0 {
			cp := append([]model.AgentConversationItem(nil), current...)
			chunks = append(chunks, cp)
			current = current[:0]
			tokens, bytes = 0, 0
		}
	}
	for _, item := range items {
		text := item.ContentRedacted
		if !utf8.ValidString(text) {
			text = strings.ToValidUTF8(text, "�")
		}
		for _, segment := range boundedTextSegments(text) {
			if len(chunks) >= AgentSessionMaxAIChunks {
				break
			}
			item.ContentRedacted = segment
			cost := int(estimateTokens(segment))
			size := len(segment) + 128
			if len(current) > 0 && (tokens+cost > AgentSessionMaxChunkTokens || bytes+size > AgentSessionMaxChunkBytes) {
				flush()
			}
			if len(chunks) >= AgentSessionMaxAIChunks {
				break
			}
			current = append(current, item)
			tokens += cost
			bytes += size
			if len(chunks) >= AgentSessionMaxAIChunks {
				break
			}
		}
		if len(chunks) >= AgentSessionMaxAIChunks {
			break
		}
	}
	flush()
	return chunks
}

func boundedTextSegments(text string) []string {
	if text == "" {
		return []string{""}
	}
	maxBytes := AgentSessionMaxChunkBytes - 256
	if maxBytes < 1024 {
		maxBytes = 1024
	}
	segments := make([]string, 0, 1)
	for len(text) > 0 {
		cut := len(text)
		if cut > maxBytes {
			cut = maxBytes
		}
		for cut > 0 && cut < len(text) && (text[cut]&0xc0) == 0x80 {
			cut--
		}
		if cut == 0 {
			cut = maxBytes
		}
		segments = append(segments, text[:cut])
		text = text[cut:]
	}
	return segments
}
