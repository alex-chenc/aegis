package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"api-server/internal/llm"
	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const customCVEQueryTimeout = 5 * time.Minute

type CustomCVEService struct {
	vulnRepo           *repository.VulnerabilityRepo
	customCVEQueryRepo *repository.CustomCVEQueryRepository
	configRepo         *repository.ConfigRepository
	llmTimeout         int
	llmRetries         int
}

func NewCustomCVEService(
	vulnRepo *repository.VulnerabilityRepo,
	customCVEQueryRepo *repository.CustomCVEQueryRepository,
	configRepo *repository.ConfigRepository,
	llmTimeout int,
	llmRetries int,
) *CustomCVEService {
	return &CustomCVEService{
		vulnRepo:           vulnRepo,
		customCVEQueryRepo: customCVEQueryRepo,
		configRepo:         configRepo,
		llmTimeout:         llmTimeout,
		llmRetries:         llmRetries,
	}
}

func (s *CustomCVEService) StartCustomQuery(ctx context.Context, cveID string) (*model.CustomCVEQuery, error) {
	cveID = strings.TrimSpace(strings.ToUpper(cveID))
	if cveID == "" {
		return nil, fmt.Errorf("cve_id is required")
	}

	if err := s.customCVEQueryRepo.MarkExpiredQueryingAsFailed(); err != nil {
		return nil, fmt.Errorf("failed to cleanup expired queries: %w", err)
	}

	querying, err := s.customCVEQueryRepo.FindQuerying()
	if err != nil {
		return nil, fmt.Errorf("failed to check querying status: %w", err)
	}
	if querying != nil {
		return nil, fmt.Errorf("another custom cve query is in progress")
	}

	if _, err := s.vulnRepo.FindByCveID(cveID); err == nil {
		return nil, fmt.Errorf("cve %s already exists", cveID)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check cve existence: %w", err)
	}

	query := &model.CustomCVEQuery{
		CveID:     cveID,
		Status:    model.QueryStatusQuerying,
		StartedAt: time.Now(),
	}
	if err := s.customCVEQueryRepo.Create(query); err != nil {
		return nil, fmt.Errorf("failed to create custom cve query: %w", err)
	}

	go s.executeQuery(context.Background(), query)

	return query, nil
}

func (s *CustomCVEService) GetQueryStatus(queryID string) (*model.CustomCVEQuery, error) {
	id, err := uuid.Parse(queryID)
	if err != nil {
		return nil, fmt.Errorf("invalid query id: %w", err)
	}

	query, err := s.customCVEQueryRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get query status: %w", err)
	}
	if query == nil {
		return nil, fmt.Errorf("query not found")
	}

	return query, nil
}

func (s *CustomCVEService) GetCurrentQuery() (*model.CustomCVEQuery, bool) {
	query, err := s.customCVEQueryRepo.FindQuerying()
	if err != nil || query == nil {
		return nil, false
	}
	return query, true
}

func (s *CustomCVEService) executeQuery(ctx context.Context, query *model.CustomCVEQuery) {
	llmClient, err := s.getLLMClient(ctx)
	if err != nil {
		s.markQueryFailed(query.ID, "LLM客户端初始化失败", err.Error())
		return
	}

	queryCtx, cancel := context.WithTimeout(ctx, customCVEQueryTimeout)
	defer cancel()

	userPrompt := fmt.Sprintf(`Return authoritative vulnerability intelligence for this CVE: %s

Requirements:
1. Return exactly one JSON object.
2. If authoritative information is available, set found=true and populate every supported field.
3. If it is unavailable, set found=false and leave unsupported fields empty.
4. Never invent references, affected versions, or remediation.

JSON schema:
{
  "cve_id": "CVE-2024-XXXX",
  "severity": "Critical|High|Medium|Low",
  "cvss_score": 0.0,
  "description": "vulnerability description in Simplified Chinese",
  "affected_products": [
    {
      "product": "product",
      "vendor": "vendor",
      "versions": ["affected version"],
      "fixed_versions": ["fixed version"]
    }
  ],
  "solution": "remediation in Simplified Chinese",
  "references": ["https://..."],
  "cwe_id": "CWE-XX",
  "found": true
}`, query.CveID)

	response, err := llmClient.ChatCompletion(queryCtx, "You are a senior vulnerability-intelligence analyst. Use only authoritative information and return strict JSON.", userPrompt, 0.1)
	if err != nil {
		s.markQueryFailed(query.ID, "LLM查询失败", err.Error())
		return
	}

	result, err := s.parseCveQueryResult(response)
	if err != nil {
		s.markQueryFailed(query.ID, "解析查询结果失败", err.Error())
		return
	}

	if !result.Found {
		s.markQueryFailed(query.ID, "未查询到该CVE", "LLM returned found=false")
		return
	}

	vuln := s.convertToVulnerability(result)
	if err := s.vulnRepo.CreateVulnerability(vuln); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			existing, findErr := s.vulnRepo.FindByCveID(vuln.CveID)
			if findErr == nil && existing != nil {
				if markErr := s.customCVEQueryRepo.MarkSuccess(query.ID, existing.ID); markErr != nil {
					logger.Error("failed to mark custom cve query success",
						zap.Error(markErr),
						zap.String("query_id", query.ID.String()),
					)
				} else {
					logger.Info("custom cve query completed",
						zap.String("query_id", query.ID.String()),
						zap.String("cve_id", query.CveID),
						zap.String("vulnerability_id", existing.ID.String()),
					)
				}
				return
			}
		}
		s.markQueryFailed(query.ID, "漏洞入库失败", err.Error())
		return
	}

	if err := s.customCVEQueryRepo.MarkSuccess(query.ID, vuln.ID); err != nil {
		logger.Error("failed to mark custom cve query success",
			zap.Error(err),
			zap.String("query_id", query.ID.String()),
			zap.String("vulnerability_id", vuln.ID.String()),
		)
		_ = s.customCVEQueryRepo.MarkFailed(query.ID, "状态更新失败", err.Error())
		return
	}
	logger.Info("custom cve query completed",
		zap.String("query_id", query.ID.String()),
		zap.String("cve_id", query.CveID),
		zap.String("vulnerability_id", vuln.ID.String()),
	)
}

func (s *CustomCVEService) parseCveQueryResult(response string) (*model.CveQueryResult, error) {
	clean := strings.TrimSpace(response)
	if clean == "" {
		return nil, fmt.Errorf("empty response")
	}

	if trimmed, ok := strings.CutPrefix(clean, "```json"); ok {
		clean = trimmed
		clean = strings.TrimSuffix(clean, "```")
		clean = strings.TrimSpace(clean)
	} else if trimmed, ok := strings.CutPrefix(clean, "```"); ok {
		clean = trimmed
		clean = strings.TrimSuffix(clean, "```")
		clean = strings.TrimSpace(clean)
	}

	jsonStart := strings.Index(clean, "{")
	jsonEnd := strings.LastIndex(clean, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, fmt.Errorf("invalid json object in response")
	}

	clean = clean[jsonStart : jsonEnd+1]

	var result model.CveQueryResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cve query result: %w", err)
	}

	return &result, nil
}

func (s *CustomCVEService) convertToVulnerability(result *model.CveQueryResult) *model.Vulnerability {
	if result == nil {
		return nil
	}

	cvss := result.CvssScore
	v := &model.Vulnerability{
		CveID:       strings.TrimSpace(strings.ToUpper(result.CveID)),
		Severity:    strings.TrimSpace(result.Severity),
		CvssScore:   &cvss,
		Description: strings.TrimSpace(result.Description),
		Source:      "custom_query",
	}

	if strings.TrimSpace(result.Solution) != "" {
		solution := strings.TrimSpace(result.Solution)
		v.Solution = &solution
	}

	if strings.TrimSpace(result.CweID) != "" {
		cweID := strings.TrimSpace(result.CweID)
		v.CweID = &cweID
	}

	if len(result.References) > 0 {
		v.RefLinks = model.JSONB{"references": result.References}
	}

	if len(result.AffectedProducts) > 0 {
		affectedProducts := make([]map[string]any, 0, len(result.AffectedProducts))
		for _, p := range result.AffectedProducts {
			affectedProducts = append(affectedProducts, map[string]any{
				"product":        p.Product,
				"vendor":         p.Vendor,
				"versions":       p.Versions,
				"fixed_versions": p.FixedVersions,
			})
		}
		v.AffectedProducts = model.JSONB{"items": affectedProducts}
	}

	return v
}

func (s *CustomCVEService) getLLMClient(ctx context.Context) (*llm.LLMClient, error) {
	_ = ctx
	if s.configRepo == nil {
		return nil, fmt.Errorf("config repository not configured")
	}

	config, err := s.configRepo.GetActive()
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM config: %w", err)
	}

	if config.APIKeyEncrypted == "" {
		return nil, fmt.Errorf("LLM API key not configured")
	}

	apiKey, err := s.configRepo.DecryptAPIKey(config.APIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt API key: %w", err)
	}

	return llm.NewLLMClient(apiKey, config.BaseURL, config.ModelName, s.llmTimeout, s.llmRetries), nil
}

func (s *CustomCVEService) markQueryFailed(queryID uuid.UUID, errMsg string, errDetail string) {
	logger.Warn("custom cve query failed",
		zap.String("query_id", queryID.String()),
		zap.String("error_message", errMsg),
	)
	if err := s.customCVEQueryRepo.MarkFailed(queryID, errMsg, errDetail); err != nil {
		logger.Error("failed to mark custom cve query as failed",
			zap.Error(err),
			zap.String("query_id", queryID.String()),
			zap.String("error_message", errMsg),
		)
	}
}
