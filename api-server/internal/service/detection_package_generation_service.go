package service

import (
	"context"
	"fmt"

	"api-server/internal/model"
)

// DetectionPackageGenerationService 检测包生成服务（对齐设计文档第 10.2 节）
// 从 handler 层下沉，供工具 handler 和页面 handler 共同调用
type DetectionPackageGenerationService struct {
	configRepo interface{}
	pkgService interface{}
}

// NewDetectionPackageGenerationService 创建检测包生成服务
func NewDetectionPackageGenerationService(
	configRepo interface{},
	pkgService interface{},
) *DetectionPackageGenerationService {
	return &DetectionPackageGenerationService{
		configRepo: configRepo,
		pkgService: pkgService,
	}
}

// GenerateDetectionPackageDraftRequest 生成检测包草稿请求
type GenerateDetectionPackageDraftRequest struct {
	CVEID                    string `json:"cve_id"`
	VulnerabilityDescription string `json:"vulnerability_description"`
	AttackPrerequisites      string `json:"attack_prerequisites"`
	ExploitationChain        string `json:"exploitation_chain"`
	FalsePositiveConstraints string `json:"false_positive_constraints"`
	Operator                 string `json:"operator"`
}

// GenerateDraft 生成检测包草稿（对齐 Package.Draft.Generate 工具）
func (s *DetectionPackageGenerationService) GenerateDraft(ctx context.Context, req GenerateDetectionPackageDraftRequest) (*model.DetectionPackageDraft, error) {
	if req.CVEID == "" {
		return nil, fmt.Errorf("cve_id is required")
	}

	// TODO: 调用 LLM 生成检测包草稿
	draft := &model.DetectionPackageDraft{
		Status: "draft",
	}

	return draft, nil
}
