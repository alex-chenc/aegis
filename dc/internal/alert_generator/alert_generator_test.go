package alert_generator

import (
	"testing"

	"dc/internal/block_manager"
	"dc/internal/model"
	"dc/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func init() {
	if logger.Logger == nil {
		logger.Logger, _ = zap.NewDevelopment()
	}
}

func newTestEvent(mitreID string) *model.RuntimeEvent {
	return &model.RuntimeEvent{
		EventID:       "evt-test-001",
		HostID:        uuid.New(),
		EventType:     "process_exec",
		MatchedRuleID: "rule-001",
		MitreID:       mitreID,
		Severity:      "high",
		PID:           1234,
		ProcessName:   "bash",
		CommandLine:   "/bin/bash -c 'test'",
	}
}

func TestGenerateAlert_AutoBlockedWhenPolicyEnabled(t *testing.T) {
	blockMgr := block_manager.NewBlockManager()
	blockMgr.StorePolicy(&model.BlockPolicy{
		MitreID:   "T1059.004",
		Enabled:   true,
		AutoBlock: true,
	})

	gen := NewAlertGenerator(blockMgr)
	event := newTestEvent("T1059.004")
	alert := gen.GenerateAlert(event)

	if alert == nil {
		t.Fatal("expected alert, got nil")
	}
	if !alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=true when policy Enabled=true and AutoBlock=true")
	}
	if alert.BlockStatus == nil || *alert.BlockStatus != "blocking" {
		t.Fatal("expected BlockStatus=blocking")
	}
	if alert.Status != "pending" {
		t.Fatalf("expected Status=pending, got %s", alert.Status)
	}
}

func TestGenerateAlert_NotAutoBlockedWhenAutoBlockDisabled(t *testing.T) {
	blockMgr := block_manager.NewBlockManager()
	blockMgr.StorePolicy(&model.BlockPolicy{
		MitreID:   "T1059.004",
		Enabled:   true,
		AutoBlock: false,
	})

	gen := NewAlertGenerator(blockMgr)
	event := newTestEvent("T1059.004")
	alert := gen.GenerateAlert(event)

	if alert == nil {
		t.Fatal("expected alert, got nil")
	}
	if alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=false when AutoBlock=false")
	}
	if alert.BlockStatus != nil {
		t.Fatal("expected BlockStatus=nil when AutoBlock=false")
	}
}

func TestGenerateAlert_NotAutoBlockedWhenPolicyDisabled(t *testing.T) {
	blockMgr := block_manager.NewBlockManager()
	blockMgr.StorePolicy(&model.BlockPolicy{
		MitreID:   "T1059.004",
		Enabled:   false,
		AutoBlock: true,
	})

	gen := NewAlertGenerator(blockMgr)
	event := newTestEvent("T1059.004")
	alert := gen.GenerateAlert(event)

	if alert == nil {
		t.Fatal("expected alert, got nil")
	}
	if alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=false when policy Enabled=false")
	}
}

func TestGenerateAlert_AutoDisposeShouldNotTriggerAutoBlock(t *testing.T) {
	blockMgr := block_manager.NewBlockManager()
	blockMgr.StorePolicy(&model.BlockPolicy{
		MitreID:     "T1059.004",
		Enabled:     true,
		AutoBlock:   false,
		AutoDispose: true,
	})

	gen := NewAlertGenerator(blockMgr)
	event := newTestEvent("T1059.004")
	alert := gen.GenerateAlert(event)

	if alert == nil {
		t.Fatal("expected alert, got nil")
	}
	if alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=false when AutoBlock=false even if AutoDispose=true")
	}
}

func TestGenerateAlert_NoPolicy(t *testing.T) {
	blockMgr := block_manager.NewBlockManager()

	gen := NewAlertGenerator(blockMgr)
	event := newTestEvent("T9999")
	alert := gen.GenerateAlert(event)

	if alert == nil {
		t.Fatal("expected alert, got nil")
	}
	if alert.AutoBlocked {
		t.Fatal("expected AutoBlocked=false when no policy exists")
	}
}

func TestGenerateAlert_NilWhenNoMatchedRule(t *testing.T) {
	blockMgr := block_manager.NewBlockManager()
	gen := NewAlertGenerator(blockMgr)

	event := &model.RuntimeEvent{
		EventID:   "evt-test-002",
		HostID:    uuid.New(),
		EventType: "process_exec",
		MitreID:   "T1059.004",
		Severity:  "high",
	}

	alert := gen.GenerateAlert(event)
	if alert != nil {
		t.Fatal("expected nil when MatchedRuleID is empty")
	}
}
