package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"api-server/internal/model"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
)

type collectionDispatcherStub struct {
	affected int32
	err      error
}

func (s collectionDispatcherStub) SyncAgentConfig(context.Context, string, []*pb.AgentConfig) (int32, error) {
	return s.affected, s.err
}

func TestRequestCollectionKeepsDisconnectedAgentPending(t *testing.T) {
	svc := NewAgentSessionService(nil, nil, nil)
	svc.SetCollectionDispatcher(collectionDispatcherStub{affected: 0})
	result, err := svc.RequestCollection(context.Background(), uuid.New(), model.AgentSessionSourceClaude)
	if err != nil {
		t.Fatalf("RequestCollection returned error: %v", err)
	}
	if result.Accepted || result.Status != "pending_reconnect" {
		t.Fatalf("unexpected result: %+v", result)
	}

	svc.SetCollectionDispatcher(collectionDispatcherStub{err: errors.New("agent not connected")})
	result, err = svc.RequestCollection(context.Background(), uuid.New(), model.AgentSessionSourceCodex)
	if err != nil {
		t.Fatalf("disconnected dispatch returned error: %v", err)
	}
	if result.Status != "pending_reconnect" {
		t.Fatalf("expected pending_reconnect, got %+v", result)
	}
}

func TestBuiltinRulesExposeReadOnlyRuleCatalog(t *testing.T) {
	rules := NewAgentSessionService(nil, nil, nil).BuiltinRules()
	if len(rules) != 5 {
		t.Fatalf("expected five builtin rules, got %d", len(rules))
	}
	for _, rule := range rules {
		if rule.RuleKey == "" || rule.Name == "" || rule.Description == "" || rule.Source != "builtin" || !rule.DefaultEnabled || rule.RuleVersion != 1 || rule.Digest == "" {
			t.Fatalf("invalid builtin rule summary: %+v", rule)
		}
	}
}

func TestBuildAIChunksBoundsInput(t *testing.T) {
	items := make([]model.AgentConversationItem, 0, 21)
	for i := 0; i < 20; i++ {
		items = append(items, model.AgentConversationItem{Sequence: int64(i), Role: "user", ContentRedacted: strings.Repeat("x", 1200)})
	}
	items = append(items, model.AgentConversationItem{Sequence: 21, Role: "user", ContentRedacted: strings.Repeat("超", 50000)})
	chunks := BuildAIChunks(items)
	if len(chunks) < 2 {
		t.Fatalf("expected bounded chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		tokens := 0
		bytes := 0
		for _, item := range chunk {
			tokens += int(estimateTokens(item.ContentRedacted))
			bytes += len(item.ContentRedacted) + 128
		}
		if tokens > AgentSessionMaxChunkTokens {
			t.Fatalf("chunk token bound exceeded: %d", tokens)
		}
		if bytes > AgentSessionMaxChunkBytes {
			t.Fatalf("chunk byte bound exceeded: %d", bytes)
		}
	}
}

func TestEstimateTokensIsDeterministic(t *testing.T) {
	if got := estimateTokens("abcd中文"); got != 3 {
		t.Fatalf("unexpected chars_div_4 estimate: %d", got)
	}
}
