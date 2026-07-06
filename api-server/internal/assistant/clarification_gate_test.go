package assistant

import (
	"testing"
)

func TestClarificationGateVagueRepair(t *testing.T) {
	gate := NewClarificationGate(ToolDecisionConfig{ClarificationRequiredWrite: true}, nil)
	breakdown := &IntentBreakdown{
		Goal:              "帮我修复一下",
		Actions:           []string{"execute"},
		RequiresWrite:     true,
		NeedClarification: true,
		ClarifyingQuestion: "请确认要修复的对象，是基线规则、漏洞 CVE、弱密码任务还是检测包？",
	}
	decision := gate.Evaluate(breakdown, nil, nil)
	if !decision.Required {
		t.Fatal("expected clarification required for vague repair")
	}
	if decision.Source != "intent_breakdown" {
		t.Fatalf("expected source intent_breakdown, got %q", decision.Source)
	}
	if decision.Question == "" {
		t.Fatal("expected non-empty question")
	}
}

func TestClarificationGateBlockWithoutAlert(t *testing.T) {
	gate := NewClarificationGate(ToolDecisionConfig{ClarificationRequiredWrite: true}, nil)
	breakdown := &IntentBreakdown{
		Goal:              "阻断这个告警",
		Actions:           []string{"block"},
		RequiresWrite:     true,
		NeedClarification: true,
		ClarifyingQuestion: "请确认要阻断的告警 ID。",
	}
	decision := gate.Evaluate(breakdown, nil, nil)
	if !decision.Required {
		t.Fatal("expected clarification required for block without alert")
	}
}

func TestClarificationGateWriteWithMissingScope(t *testing.T) {
	gate := NewClarificationGate(ToolDecisionConfig{ClarificationRequiredWrite: true}, nil)
	breakdown := &IntentBreakdown{
		Goal:              "采集资产",
		Actions:           []string{"collect"},
		RequiresWrite:     true,
		NeedClarification: true,
		ClarifyingQuestion: "请补充要操作的对象和范围。",
	}
	decision := gate.Evaluate(breakdown, nil, nil)
	if !decision.Required {
		t.Fatal("expected clarification required for missing scope")
	}
}

func TestClarificationGateReadOnlyNoClarification(t *testing.T) {
	gate := NewClarificationGate(ToolDecisionConfig{}, nil)
	breakdown := &IntentBreakdown{
		Goal:              "查询主机列表",
		Actions:           []string{"query"},
		RequiresWrite:     false,
		NeedClarification: false,
	}
	decision := gate.Evaluate(breakdown, []string{"Host.List"}, nil)
	if decision.Required {
		t.Fatalf("read-only query should not need clarification, got: %+v", decision)
	}
}

func TestClarificationGateAcceptedToolsExist(t *testing.T) {
	gate := NewClarificationGate(ToolDecisionConfig{ClarificationRequiredWrite: true}, nil)
	breakdown := &IntentBreakdown{
		Goal:              "采集资产并分析",
		Actions:           []string{"collect", "analyze"},
		RequiresWrite:     true,
		NeedClarification: false,
	}
	decision := gate.Evaluate(breakdown, []string{"Asset.Collection.Trigger", "Host.List"}, nil)
	if decision.Required {
		t.Fatalf("should not clarify when accepted tools exist, got: %+v", decision)
	}
}

func TestClarificationGateFromDecisionRecord(t *testing.T) {
	gate := NewClarificationGate(ToolDecisionConfig{}, nil)
	breakdown := &IntentBreakdown{
		Goal: "阻断告警",
	}
	records := []ToolDecisionRecord{
		{
			ToolName: "Detection.Alert.Block",
			Decision: toolDecisionClarificationRequired,
			Reason:   "请确认要操作的告警 ID。",
		},
	}
	decision := gate.Evaluate(breakdown, nil, records)
	if !decision.Required {
		t.Fatal("expected clarification from decision record")
	}
	if decision.Source != "missing_entity" {
		t.Fatalf("expected source missing_entity, got %q", decision.Source)
	}
}

func TestClarificationGateNilBreakdown(t *testing.T) {
	gate := NewClarificationGate(ToolDecisionConfig{}, nil)
	decision := gate.Evaluate(nil, nil, nil)
	if decision.Required {
		t.Fatal("nil breakdown should not require clarification")
	}
}
