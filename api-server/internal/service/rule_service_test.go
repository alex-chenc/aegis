package service

import (
	"testing"
)

func TestNewRuleService(t *testing.T) {
	s := NewRuleService(nil, nil)
	if s == nil {
		t.Fatal("expected non-nil service")
	}
}
