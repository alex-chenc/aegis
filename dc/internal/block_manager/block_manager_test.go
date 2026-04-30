package block_manager

import (
	"testing"

	"dc/internal/model"
)

func TestShouldAutoBlock_EnabledWithAutoBlock(t *testing.T) {
	m := NewBlockManager()
	m.StorePolicy(&model.BlockPolicy{
		MitreID:   "T1059.004",
		Enabled:   true,
		AutoBlock: true,
	})

	if !m.ShouldAutoBlock("T1059.004") {
		t.Fatal("expected ShouldAutoBlock=true when Enabled=true and AutoBlock=true")
	}
}

func TestShouldAutoBlock_EnabledWithoutAutoBlock(t *testing.T) {
	m := NewBlockManager()
	m.StorePolicy(&model.BlockPolicy{
		MitreID:   "T1059.004",
		Enabled:   true,
		AutoBlock: false,
	})

	if m.ShouldAutoBlock("T1059.004") {
		t.Fatal("expected ShouldAutoBlock=false when Enabled=true and AutoBlock=false")
	}
}

func TestShouldAutoBlock_DisabledWithAutoBlock(t *testing.T) {
	m := NewBlockManager()
	m.StorePolicy(&model.BlockPolicy{
		MitreID:   "T1059.004",
		Enabled:   false,
		AutoBlock: true,
	})

	if m.ShouldAutoBlock("T1059.004") {
		t.Fatal("expected ShouldAutoBlock=false when Enabled=false")
	}
}

func TestShouldAutoBlock_AutoDisposeShouldNotTriggerBlock(t *testing.T) {
	m := NewBlockManager()
	m.StorePolicy(&model.BlockPolicy{
		MitreID:     "T1059.004",
		Enabled:     true,
		AutoBlock:   false,
		AutoDispose: true,
	})

	if m.ShouldAutoBlock("T1059.004") {
		t.Fatal("expected ShouldAutoBlock=false when AutoBlock=false even if AutoDispose=true")
	}
}

func TestShouldAutoBlock_NoPolicy(t *testing.T) {
	m := NewBlockManager()

	if m.ShouldAutoBlock("T9999") {
		t.Fatal("expected ShouldAutoBlock=false when no policy exists")
	}
}

func TestIsBlocked_Enabled(t *testing.T) {
	m := NewBlockManager()
	m.StorePolicy(&model.BlockPolicy{
		MitreID: "T1059.004",
		Enabled: true,
	})

	if !m.IsBlocked("T1059.004") {
		t.Fatal("expected IsBlocked=true when policy enabled")
	}
}

func TestIsBlocked_Disabled(t *testing.T) {
	m := NewBlockManager()
	m.StorePolicy(&model.BlockPolicy{
		MitreID: "T1059.004",
		Enabled: false,
	})

	if m.IsBlocked("T1059.004") {
		t.Fatal("expected IsBlocked=false when policy disabled")
	}
}
