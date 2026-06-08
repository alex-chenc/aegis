package assistant

import (
	"context"
	"strings"

	"api-server/internal/repository"
	agentruntime "github.com/alex-chenc/agent-runtime"
)

const assistantReflectionMemoryType = "reflection"

type assistantReflectionExperienceProvider struct {
	memoryRepo repository.AssistantMemoryRepository
	sessionID  string
}

func newAssistantReflectionExperienceProvider(repo repository.AssistantMemoryRepository, sessionID string) agentruntime.ExperienceProvider {
	return &assistantReflectionExperienceProvider{
		memoryRepo: repo,
		sessionID:  sessionID,
	}
}

func (p *assistantReflectionExperienceProvider) Fetch(ctx context.Context, req agentruntime.ExperienceRequest) (agentruntime.ExperienceResponse, error) {
	if p == nil || p.memoryRepo == nil || p.sessionID == "" {
		return agentruntime.ExperienceResponse{}, nil
	}

	memories, err := p.memoryRepo.ListBySession(ctx, p.sessionID, assistantReflectionMemoryType)
	if err != nil {
		return agentruntime.ExperienceResponse{}, err
	}

	maxItems := req.MaxItems
	if maxItems <= 0 {
		maxItems = 3
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	items := make([]agentruntime.ExperienceItem, 0, maxItems)

	appendMemory := func(content string, id string) {
		content = strings.TrimSpace(content)
		if content == "" || len(items) >= maxItems {
			return
		}
		items = append(items, agentruntime.ExperienceItem{
			ID:      id,
			Summary: truncateForExperience(content, 220),
			Content: content,
			Tags:    []string{assistantReflectionMemoryType},
			Metadata: map[string]any{
				"session_id": p.sessionID,
			},
		})
	}

	if query != "" {
		for _, memory := range memories {
			content := strings.TrimSpace(memory.Content)
			if strings.Contains(strings.ToLower(content), query) {
				appendMemory(content, memory.ID.String())
			}
		}
	}
	for _, memory := range memories {
		appendMemory(memory.Content, memory.ID.String())
	}

	return agentruntime.ExperienceResponse{Items: items}, nil
}

func truncateForExperience(value string, max int) string {
	if max <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}
