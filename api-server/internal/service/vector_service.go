package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"gorm.io/gorm"
)

// VectorService provides vector similarity search functionality
type VectorService struct {
	db           *gorm.DB
	embeddingSvc *EmbeddingService
}

// EmbeddingService wraps the embedding service
type EmbeddingService struct {
	openaiAPIKey string
	baseURL      string
}

// SimilarAnalysis represents a similar analysis result
type SimilarAnalysis struct {
	ID               string                 `json:"id"`
	SessionID        string                 `json:"session_id"`
	AlertIDs         []string               `json:"alert_ids"`
	HostFilter       []string               `json:"host_filter"`
	InitialQuery     string                 `json:"initial_query"`
	FinalConclusion  map[string]interface{} `json:"final_conclusion"`
	Summary          string                 `json:"summary"`
	Similarity       float64                `json:"similarity"`
}

// NewEmbeddingService creates a new embedding service
func NewEmbeddingService(openaiAPIKey, baseURL string) *EmbeddingService {
	return &EmbeddingService{
		openaiAPIKey: openaiAPIKey,
		baseURL:      baseURL,
	}
}

// Generate generates an embedding for the given text
func (s *EmbeddingService) Generate(ctx context.Context, text string) ([]float32, error) {
	reqBody := map[string]interface{}{
		"model": "text-embedding-3-small",
		"input": text,
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", s.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.openaiAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return result.Data[0].Embedding, nil
}

// NewVectorService creates a new vector service
func NewVectorService(db *gorm.DB, embeddingSvc *EmbeddingService) *VectorService {
	return &VectorService{
		db:           db,
		embeddingSvc: embeddingSvc,
	}
}

// AIAnalysisRecord represents the AI analysis record model
type AIAnalysisRecord struct {
	ID              string `gorm:"column:id;primaryKey"`
	SessionID       string `gorm:"column:session_id"`
	AlertIDs        string `gorm:"column:alert_ids"`
	HostFilter      string `gorm:"column:host_filter"`
	InitialQuery    string `gorm:"column:initial_query"`
	FinalConclusion string `gorm:"column:final_conclusion"`
	Summary         string `gorm:"column:summary"`
	SummaryVector   string `gorm:"column:summary_vector"`
}

// TableName returns the table name
func (AIAnalysisRecord) TableName() string {
	return "ai_analysis_record"
}

// FindSimilarAnalysis finds similar analysis records based on vector similarity
func (s *VectorService) FindSimilarAnalysis(ctx context.Context, query string, alertType string, threshold float64, limit int) ([]SimilarAnalysis, error) {
	if s.embeddingSvc == nil {
		return nil, fmt.Errorf("embedding service not initialized")
	}

	// Generate query embedding
	queryEmbedding, err := s.embeddingSvc.Generate(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	vectorJSON, err := json.Marshal(queryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding: %w", err)
	}

	var results []SimilarAnalysis
	var sql string

	if alertType != "" {
		sql = `
			SELECT
				id::text,
				session_id,
				alert_ids,
				host_filter,
				initial_query,
				final_conclusion,
				summary,
				1 - (summary_vector <=> ?) as similarity
			FROM ai_analysis_record
			WHERE summary_vector IS NOT NULL
				AND alert_ids::text LIKE ?
				AND 1 - (summary_vector <=> ?) > ?
			ORDER BY summary_vector <=> ?
			LIMIT ?`
		rows, err := s.db.Raw(sql, vectorJSON, "%"+alertType+"%", vectorJSON, threshold, vectorJSON, limit).Rows()
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var r SimilarAnalysis
			var alertIDsJSON, hostFilterJSON, conclusionJSON []byte

			err := rows.Scan(&r.ID, &r.SessionID, &alertIDsJSON, &hostFilterJSON, &r.InitialQuery, &conclusionJSON, &r.Summary, &r.Similarity)
			if err != nil {
				continue
			}

			json.Unmarshal(alertIDsJSON, &r.AlertIDs)
			json.Unmarshal(hostFilterJSON, &r.HostFilter)
			json.Unmarshal(conclusionJSON, &r.FinalConclusion)

			results = append(results, r)
		}
	} else {
		sql = `
			SELECT
				id::text,
				session_id,
				alert_ids,
				host_filter,
				initial_query,
				final_conclusion,
				summary,
				1 - (summary_vector <=> ?) as similarity
			FROM ai_analysis_record
			WHERE summary_vector IS NOT NULL
				AND 1 - (summary_vector <=> ?) > ?
			ORDER BY summary_vector <=> ?
			LIMIT ?`
		rows, err := s.db.Raw(sql, vectorJSON, vectorJSON, threshold, vectorJSON, limit).Rows()
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var r SimilarAnalysis
			var alertIDsJSON, hostFilterJSON, conclusionJSON []byte

			err := rows.Scan(&r.ID, &r.SessionID, &alertIDsJSON, &hostFilterJSON, &r.InitialQuery, &conclusionJSON, &r.Summary, &r.Similarity)
			if err != nil {
				continue
			}

			json.Unmarshal(alertIDsJSON, &r.AlertIDs)
			json.Unmarshal(hostFilterJSON, &r.HostFilter)
			json.Unmarshal(conclusionJSON, &r.FinalConclusion)

			results = append(results, r)
		}
	}

	return results, nil
}

// BuildRAGContext builds a RAG context string from similar analysis records
func (s *VectorService) BuildRAGContext(ctx context.Context, query string, alertType string) (string, error) {
	similar, err := s.FindSimilarAnalysis(ctx, query, alertType, 0.75, 3)
	if err != nil {
		return "", err
	}

	if len(similar) == 0 {
		return "", nil
	}

	var ragContext strings.Builder
	ragContext.WriteString("参考历史分析案例:\n\n")

	for i, r := range similar {
		ragContext.WriteString(fmt.Sprintf("案例 %d (相似度: %.2f%%):\n", i+1, r.Similarity*100))
		ragContext.WriteString(fmt.Sprintf("初始问题: %s\n", r.InitialQuery))
		ragContext.WriteString(fmt.Sprintf("分析摘要: %s\n", r.Summary))

		if len(r.FinalConclusion) > 0 {
			conclusionJSON, _ := json.Marshal(r.FinalConclusion)
			ragContext.WriteString(fmt.Sprintf("最终结论: %s\n", string(conclusionJSON)))
		}
		ragContext.WriteString("---\n\n")
	}

	return ragContext.String(), nil
}

// GenerateAndSaveEmbedding generates and saves an embedding for an analysis record
func (s *VectorService) GenerateAndSaveEmbedding(ctx context.Context, record *AIAnalysisRecord) error {
	if record.Summary == "" {
		return fmt.Errorf("record summary is empty")
	}

	if s.embeddingSvc == nil {
		return fmt.Errorf("embedding service not initialized")
	}

	embedding, err := s.embeddingSvc.Generate(ctx, record.Summary)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	vectorJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}

	return s.db.Model(record).Update("summary_vector", vectorJSON).Error
}
