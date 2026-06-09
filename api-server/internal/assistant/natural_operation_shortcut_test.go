package assistant

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestDetectNaturalOperationShortcut(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    naturalOperationKind
	}{
		{name: "asset collection command", message: "进行资产采集", want: naturalOperationAssetCollection},
		{name: "bare asset collection", message: "资产采集", want: naturalOperationAssetCollection},
		{name: "vulnerability scan command", message: "进行漏洞扫描", want: naturalOperationVulnerabilityScan},
		{name: "baseline scan command", message: "进行基线扫描", want: naturalOperationBaselineScan},
		{name: "detection command", message: "异常检测", want: naturalOperationDetectionCheck},
		{name: "how to question", message: "如何进行资产采集？", want: naturalOperationNone},
		{name: "explicit tool prompt", message: "请调用 Asset.Collection.Trigger 参数 scope=all_hosts", want: naturalOperationNone},
		{name: "collection status", message: "查看资产采集进度", want: naturalOperationNone},
		{name: "composite asset software vulnerability analysis", message: "进行资产采集任务，并分析那个主机上有 MySQL 软件，并分析此 MySql 软件是否有漏洞", want: naturalOperationNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectNaturalOperationShortcut(tt.message).Kind; got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNaturalAssetCollectionShortcutTriggersAllHostsCollection(t *testing.T) {
	registry := NewToolRegistry()
	calls := map[string]int{}
	var triggerArgs map[string]interface{}
	for _, spec := range []*ToolSpec{
		{
			Name:               "Asset.Collection.Trigger",
			Risk:               ToolRiskMedium,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				calls["Asset.Collection.Trigger"]++
				triggerArgs = args
				return map[string]interface{}{"task_id": "collection-shortcut-1", "status": "collecting"}, nil
			},
		},
		{
			Name:               "Asset.Collection.Get",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				calls["Asset.Collection.Get"]++
				return map[string]interface{}{"task": map[string]interface{}{"id": args["task_id"], "status": "completed"}}, nil
			},
		},
		{
			Name:               "Asset.Application.List",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				calls["Asset.Application.List"]++
				return map[string]interface{}{"items": []map[string]interface{}{{"name": "claude-code"}}, "total": 1}, nil
			},
		},
		{
			Name:               "Asset.Summary.Get",
			Risk:               ToolRiskReadonly,
			DefaultWhitelisted: true,
			Enabled:            true,
			Handler: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
				calls["Asset.Summary.Get"]++
				return map[string]interface{}{"summary": map[string]interface{}{"ai_agent_count": 1}}, nil
			},
		},
	} {
		if err := registry.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}

	dispatcher, _ := newTestToolDispatcher(t, registry)
	manager := NewRunManager()
	run := manager.Start("shortcut-session")
	o := &Orchestrator{
		toolRegistry:   registry,
		toolDispatcher: dispatcher,
		runManager:     manager,
		logger:         zap.NewNop(),
	}

	handled, response, err := o.runNaturalOperationShortcut(context.Background(), RunInput{
		RunID:       run.RunID,
		SessionID:   "shortcut-session",
		UserID:      "admin",
		UserMessage: "进行资产采集",
	}, "msg_"+run.RunID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected shortcut to handle asset collection")
	}
	if triggerArgs["scope"] != "all_hosts" || triggerArgs["force"] != true {
		t.Fatalf("unexpected trigger args: %#v", triggerArgs)
	}
	types, ok := triggerArgs["types"].([]string)
	if !ok || len(types) != 1 || types[0] != "process" {
		t.Fatalf("unexpected types arg: %#v", triggerArgs["types"])
	}
	for _, toolName := range []string{"Asset.Collection.Trigger", "Asset.Collection.Get", "Asset.Application.List", "Asset.Summary.Get"} {
		if calls[toolName] == 0 {
			t.Fatalf("expected %s to be called, calls=%v", toolName, calls)
		}
	}
	if !strings.Contains(response, "全部在线主机") || !strings.Contains(response, "collection-shortcut-1") {
		t.Fatalf("unexpected shortcut response: %s", response)
	}
}

func TestNaturalOperationShortcutClarifiesUnderspecifiedScans(t *testing.T) {
	o := &Orchestrator{}
	for _, tt := range []struct {
		message string
		want    string
	}{
		{message: "进行漏洞扫描", want: "扫描范围"},
		{message: "进行基线扫描", want: "基线模板"},
		{message: "异常检测", want: "检测范围"},
	} {
		handled, response, err := o.runNaturalOperationShortcut(context.Background(), RunInput{UserMessage: tt.message}, "msg-test", nil)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tt.message, err)
		}
		if !handled || !strings.Contains(response, tt.want) {
			t.Fatalf("%s: expected clarification containing %q, handled=%v response=%q", tt.message, tt.want, handled, response)
		}
	}
}
