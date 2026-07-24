package tools

import (
	"context"
	"io"
	"strings"
	"testing"

	"api-server/internal/assistant"
	"api-server/internal/model"
	"api-server/internal/service"
)

type fakeSigmaContextReader struct {
	refs []model.AssistantContextRef
}

func (f *fakeSigmaContextReader) ListBySession(context.Context, string) ([]model.AssistantContextRef, error) {
	return f.refs, nil
}

type fakeSigmaLifecycleService struct {
	content       string
	filename      string
	approvedID    string
	approvedHosts []string
}

func (f *fakeSigmaLifecycleService) UploadRules(reader io.Reader, filename string, _ int64) (*service.UploadResult, error) {
	content, _ := io.ReadAll(reader)
	f.content = string(content)
	f.filename = filename
	return &service.UploadResult{
		Success:     true,
		ParsedCount: 1,
		Rules: []service.ParsedRule{{
			RuleID: "rule-123",
			Title:  "SSH shell",
			Status: "pending",
		}},
	}, nil
}

func (f *fakeSigmaLifecycleService) ApproveRule(ruleID string, hostIDs []string) error {
	f.approvedID = ruleID
	f.approvedHosts = append([]string{}, hostIDs...)
	return nil
}

func TestSigmaRuleImportReadsBoundSessionAttachment(t *testing.T) {
	reader := &fakeSigmaContextReader{refs: []model.AssistantContextRef{{
		ObjectType: "file",
		ObjectID:   "file-1",
		Title:      "rule.yml",
		Summary:    "title: SSH shell\nid: rule-123\ntags:\n  - attack.t1059\ndetection:\n  selection: test",
	}}}
	svc := &fakeSigmaLifecycleService{}
	handler := makeSigmaRuleImportHandler(reader, svc)
	ctx := assistant.WithToolInvocationContext(context.Background(), assistant.ToolInvocationContext{SessionID: "session-1"})

	result, err := handler(ctx, map[string]interface{}{"file_id": "file-1"})
	if err != nil {
		t.Fatalf("import attached Sigma rule: %v", err)
	}
	payload := result.(map[string]interface{})
	if payload["rule_id"] != "rule-123" || payload["filename"] != "rule.yml" {
		t.Fatalf("unexpected import result: %#v", payload)
	}
	if svc.filename != "rule.yml" || !strings.Contains(svc.content, "id: rule-123") {
		t.Fatalf("attachment content was not passed to Sigma parser: filename=%q content=%q", svc.filename, svc.content)
	}
}

func TestSigmaRuleEnableUsesImportedRuleID(t *testing.T) {
	svc := &fakeSigmaLifecycleService{}
	handler := makeSigmaRuleEnableHandler(svc)
	result, err := handler(context.Background(), map[string]interface{}{
		"rule_id":         "rule-123",
		"target_host_ids": []string{"host-1"},
	})
	if err != nil {
		t.Fatalf("enable imported Sigma rule: %v", err)
	}
	payload := result.(map[string]interface{})
	if svc.approvedID != "rule-123" || len(svc.approvedHosts) != 1 || svc.approvedHosts[0] != "host-1" {
		t.Fatalf("unexpected approval call: id=%q hosts=%v", svc.approvedID, svc.approvedHosts)
	}
	if payload["operation_status"] != "completed" {
		t.Fatalf("unexpected enable result: %#v", payload)
	}
}

func TestSigmaRuleLifecycleToolsAreExposedForAttachedRuleFile(t *testing.T) {
	registry := assistant.NewToolRegistry()
	if err := RegisterSigmaRuleTools(registry, SigmaRuleToolDeps{}); err != nil {
		t.Fatal(err)
	}
	catalog := assistant.NewToolExposureResolver(registry).IntentCatalog(assistant.ToolExposureContext{
		Domains:     []string{"analysis", "detection"},
		ObjectTypes: []string{"file", "rule"},
	})
	names := make(map[string]bool, len(catalog))
	for _, tool := range catalog {
		names[tool.Name] = true
	}
	for _, want := range []string{"SigmaRule.Import", "SigmaRule.Enable"} {
		if !names[want] {
			t.Fatalf("%s not exposed for attached rule file; catalog=%v", want, names)
		}
	}
}
