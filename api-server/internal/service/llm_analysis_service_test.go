package service

import (
	"testing"
)

func TestNewLLMAnalysisService(t *testing.T) {
	s := NewLLMAnalysisService(nil, 60, 3)
	if s == nil {
		t.Fatal("expected non-nil service")
	}
}
