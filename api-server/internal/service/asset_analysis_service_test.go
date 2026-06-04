package service

import (
	"testing"

	"go.uber.org/zap"
)

func TestParseAnalysisResultSupportsMarkdownJSON(t *testing.T) {
	svc := NewAssetAnalysisService(nil, nil, zap.NewNop())

	result, err := svc.parseAnalysisResult("```json\n{\"applications\":[{\"name\":\"nginx\",\"category\":\"web_service\",\"confidence\":0.9}]}\n```")
	if err != nil {
		t.Fatalf("expected markdown JSON to parse: %v", err)
	}
	if len(result.Applications) != 1 || result.Applications[0].Name != "nginx" {
		t.Fatalf("unexpected applications: %#v", result.Applications)
	}
}

func TestParseAnalysisResultSupportsBareApplicationArray(t *testing.T) {
	svc := NewAssetAnalysisService(nil, nil, zap.NewNop())

	result, err := svc.parseAnalysisResult(`[{"name":"redis","category":"database","confidence":0.95}]`)
	if err != nil {
		t.Fatalf("expected bare array to parse: %v", err)
	}
	if len(result.Applications) != 1 || result.Applications[0].Name != "redis" {
		t.Fatalf("unexpected applications: %#v", result.Applications)
	}
}

func TestSplitProcessBatches(t *testing.T) {
	processes := []ProcessAsset{{PID: 1}, {PID: 2}, {PID: 3}, {PID: 4}, {PID: 5}}

	batches := splitProcessBatches(processes, 2)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if len(batches[0]) != 2 || len(batches[1]) != 2 || len(batches[2]) != 1 {
		t.Fatalf("unexpected batch sizes: %d %d %d", len(batches[0]), len(batches[1]), len(batches[2]))
	}
}
