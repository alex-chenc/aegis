package service

import (
	"testing"
)

func TestNewLLMAnalysisService(t *testing.T) {
	s := NewLLMAnalysisService(nil)
	if s == nil {
		t.Fatal("expected non-nil service")
	}
}
