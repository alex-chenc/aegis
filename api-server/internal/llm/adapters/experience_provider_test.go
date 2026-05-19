package adapters

import (
	"context"
	"fmt"
	"testing"

	agentruntime "github.com/alex-chenc/agent-runtime"
)

// mockReflectionQuerier implements ReflectionQuerier for testing.
type mockReflectionQuerier struct {
	reflections []AgentReflectionSummary
	audits      []AgentAuditSummary
	modelErrors []AgentModelErrorSummary
	successes   []AgentExecutionSummary

	reflectionErr error
	auditErr      error
	modelErrErr   error
	successErr    error
}

func (m *mockReflectionQuerier) FindFailedReflections(ctx context.Context, limit int) ([]AgentReflectionSummary, error) {
	if m.reflectionErr != nil {
		return nil, m.reflectionErr
	}
	if limit > len(m.reflections) {
		return m.reflections, nil
	}
	return m.reflections[:limit], nil
}

func (m *mockReflectionQuerier) FindSuccessfulSummaries(ctx context.Context, limit int) ([]AgentExecutionSummary, error) {
	if m.successErr != nil {
		return nil, m.successErr
	}
	if limit > len(m.successes) {
		return m.successes, nil
	}
	return m.successes[:limit], nil
}

func (m *mockReflectionQuerier) FindRecentAudits(ctx context.Context, limit int) ([]AgentAuditSummary, error) {
	if m.auditErr != nil {
		return nil, m.auditErr
	}
	if limit > len(m.audits) {
		return m.audits, nil
	}
	return m.audits[:limit], nil
}

func (m *mockReflectionQuerier) FindRecentModelErrors(ctx context.Context, limit int) ([]AgentModelErrorSummary, error) {
	if m.modelErrErr != nil {
		return nil, m.modelErrErr
	}
	if limit > len(m.modelErrors) {
		return m.modelErrors, nil
	}
	return m.modelErrors[:limit], nil
}

func TestFetch_IncludesReflectionLessons(t *testing.T) {
	querier := &mockReflectionQuerier{
		reflections: []AgentReflectionSummary{
			{
				ReflectionID:   "refl-1",
				RootCause:      "tool timeout",
				Impact:         "step failed",
				ReusableLesson: "always set timeout",
			},
		},
	}

	adapter := NewExperienceProviderAdapter(nil, querier, 5)
	resp, err := adapter.Fetch(context.Background(), agentruntime.ExperienceRequest{
		TaskID: "task-1",
		Query:  "test query",
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	found := false
	for _, item := range resp.Items {
		for _, tag := range item.Tags {
			if tag == "reflection" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected reflection lesson in response items")
	}
}

func TestFetch_IncludesAuditLessons(t *testing.T) {
	querier := &mockReflectionQuerier{
		audits: []AgentAuditSummary{
			{
				AuditID:        "audit-1",
				Decision:       "correct_plan",
				CorrectionHint: "skip irrelevant steps",
			},
		},
	}

	adapter := NewExperienceProviderAdapter(nil, querier, 5)
	resp, err := adapter.Fetch(context.Background(), agentruntime.ExperienceRequest{
		TaskID: "task-1",
		Query:  "test query",
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	found := false
	for _, item := range resp.Items {
		for _, tag := range item.Tags {
			if tag == "audit" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected audit lesson in response items")
	}
}

func TestFetch_IncludesModelErrorLessons(t *testing.T) {
	querier := &mockReflectionQuerier{
		modelErrors: []AgentModelErrorSummary{
			{
				CallID:    "call-1",
				Purpose:   "react",
				ErrorKind: "timeout",
				Message:   "model call timed out",
			},
		},
	}

	adapter := NewExperienceProviderAdapter(nil, querier, 5)
	resp, err := adapter.Fetch(context.Background(), agentruntime.ExperienceRequest{
		TaskID: "task-1",
		Query:  "test query",
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	found := false
	for _, item := range resp.Items {
		for _, tag := range item.Tags {
			if tag == "model_error" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected model error lesson in response items")
	}
}

func TestFetch_FallbackToRecentSuccesses(t *testing.T) {
	querier := &mockReflectionQuerier{
		successes: []AgentExecutionSummary{
			{
				TaskID:      "task-success-1",
				FinalAnswer: "threat confirmed",
			},
		},
	}

	// No vector service — triggers fallback
	adapter := NewExperienceProviderAdapter(nil, querier, 5)
	resp, err := adapter.Fetch(context.Background(), agentruntime.ExperienceRequest{
		TaskID: "task-1",
		Query:  "test query",
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	found := false
	for _, item := range resp.Items {
		for _, tag := range item.Tags {
			if tag == "recent_success" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected recent success in response items when vector store is unavailable")
	}
}

func TestFetch_CombinesAllSources(t *testing.T) {
	querier := &mockReflectionQuerier{
		reflections: []AgentReflectionSummary{
			{ReflectionID: "refl-1", RootCause: "rc", Impact: "imp", ReusableLesson: "lesson"},
		},
		audits: []AgentAuditSummary{
			{AuditID: "audit-1", Decision: "correct_plan", CorrectionHint: "hint"},
		},
		modelErrors: []AgentModelErrorSummary{
			{CallID: "call-1", Purpose: "react", ErrorKind: "timeout", Message: "msg"},
		},
	}

	adapter := NewExperienceProviderAdapter(nil, querier, 5)
	resp, err := adapter.Fetch(context.Background(), agentruntime.ExperienceRequest{
		TaskID: "task-1",
		Query:  "test query",
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	tags := make(map[string]bool)
	for _, item := range resp.Items {
		for _, tag := range item.Tags {
			tags[tag] = true
		}
	}

	for _, expected := range []string{"reflection", "audit", "model_error"} {
		if !tags[expected] {
			t.Errorf("expected tag %q in response items", expected)
		}
	}
}

func TestFetch_NilQuerierReturnsEmpty(t *testing.T) {
	adapter := NewExperienceProviderAdapter(nil, nil, 5)
	resp, err := adapter.Fetch(context.Background(), agentruntime.ExperienceRequest{
		TaskID: "task-1",
		Query:  "test query",
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Fatalf("expected 0 items with nil querier, got %d", len(resp.Items))
	}
}

func TestFetch_QuerierErrorsAreNonFatal(t *testing.T) {
	querier := &mockReflectionQuerier{
		reflectionErr: fmt.Errorf("db error"),
		auditErr:      fmt.Errorf("db error"),
		modelErrErr:   fmt.Errorf("db error"),
		successErr:    fmt.Errorf("db error"),
	}

	adapter := NewExperienceProviderAdapter(nil, querier, 5)
	resp, err := adapter.Fetch(context.Background(), agentruntime.ExperienceRequest{
		TaskID: "task-1",
		Query:  "test query",
	})
	if err != nil {
		t.Fatalf("Fetch should not return error for querier failures: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Fatalf("expected 0 items when all queriers fail, got %d", len(resp.Items))
	}
}
