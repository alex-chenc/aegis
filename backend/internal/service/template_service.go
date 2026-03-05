package service

import (
	"log"
	"sync"

	"github.com/google/uuid"

	"ai-benchmark/backend/internal/models"
	"ai-benchmark/backend/internal/repository"
	"ai-benchmark/backend/pkg/llm"
)

type TemplateService struct {
	repo    *repository.Repository
	llm     *llm.Client
	parsing map[uuid.UUID]bool
	mu      sync.RWMutex
}

func NewTemplateService(repo *repository.Repository, llmClient *llm.Client) *TemplateService {
	return &TemplateService{
		repo:    repo,
		llm:     llmClient,
		parsing: make(map[uuid.UUID]bool),
	}
}

func (s *TemplateService) CreateTemplate(name, fileType, minioObjectName string) (*models.Template, error) {
	template := &models.Template{
		Name:            name,
		FileType:        fileType,
		MinioObjectName: minioObjectName,
	}
	if err := s.repo.CreateTemplate(template); err != nil {
		return nil, err
	}
	return template, nil
}

func (s *TemplateService) GetTemplate(id uuid.UUID) (*models.Template, error) {
	return s.repo.GetTemplateByID(id)
}

func (s *TemplateService) ListTemplates(page, pageSize int) ([]models.Template, int64, error) {
	return s.repo.GetTemplates(page, pageSize)
}

func (s *TemplateService) DeleteTemplate(id uuid.UUID) error {
	return s.repo.DeleteTemplate(id)
}

func (s *TemplateService) IsParsing(templateID uuid.UUID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.parsing[templateID]
}

func (s *TemplateService) ParseTemplateAsync(templateID uuid.UUID, content string, callback func(error)) {
	s.mu.Lock()
	s.parsing[templateID] = true
	s.mu.Unlock()

	go func() {
		err := s.parseTemplate(templateID, content)

		s.mu.Lock()
		delete(s.parsing, templateID)
		s.mu.Unlock()

		if callback != nil {
			callback(err)
		}
	}()
}

func (s *TemplateService) parseTemplate(templateID uuid.UUID, content string) error {
	log.Printf("[TemplateService] Starting to parse template %s", templateID)

	rules, err := s.llm.ParseBaselineDocument(content)
	if err != nil {
		log.Printf("[TemplateService] Failed to parse document: %v", err)
		return err
	}

	for _, rule := range rules {
		baselineRule := &models.BaselineRule{
			TemplateID:   templateID,
			Title:        rule.Title,
			CheckContent: rule.CheckContent,
			FixContent:   rule.FixContent,
		}

		if err := s.repo.CreateBaselineRule(baselineRule); err != nil {
			log.Printf("[TemplateService] Failed to create rule: %v", err)
			continue
		}

		go s.generateScripts(baselineRule)
	}

	log.Printf("[TemplateService] Parsed %d rules for template %s", len(rules), templateID)
	return nil
}

func (s *TemplateService) generateScripts(rule *models.BaselineRule) {
	checkScript, err := s.llm.GenerateCheckScript(rule.Title, rule.CheckContent)
	if err != nil {
		log.Printf("[TemplateService] Failed to generate check script for rule %s: %v", rule.ID, err)
	} else {
		if err := s.repo.UpdateBaselineRule(rule.ID, map[string]interface{}{
			"generated_check_script": checkScript,
		}); err != nil {
			log.Printf("[TemplateService] Failed to update check script: %v", err)
		}
	}

	fixScript, err := s.llm.GenerateFixScript(rule.Title, rule.FixContent)
	if err != nil {
		log.Printf("[TemplateService] Failed to generate fix script for rule %s: %v", rule.ID, err)
	} else {
		if err := s.repo.UpdateBaselineRule(rule.ID, map[string]interface{}{
			"generated_fix_script": fixScript,
		}); err != nil {
			log.Printf("[TemplateService] Failed to update fix script: %v", err)
		}
	}
}

func (s *TemplateService) GetRules(templateID uuid.UUID) ([]models.BaselineRule, error) {
	return s.repo.GetBaselineRulesByTemplateID(templateID)
}

func (s *TemplateService) GetRule(ruleID uuid.UUID) (*models.BaselineRule, error) {
	return s.repo.GetBaselineRuleByID(ruleID)
}
