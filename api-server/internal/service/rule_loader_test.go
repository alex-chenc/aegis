package service

import (
	"testing"
)

func TestExtractMitreID(t *testing.T) {
	tests := []struct {
		tags     []string
		expected string
	}{
		{[]string{"attack.t1059.004"}, "t1059.004"},
		{[]string{"attack.T1059.004"}, "T1059.004"},
		{[]string{"other", "attack.t1068"}, "t1068"},
		{[]string{"no-mitre"}, ""},
		{[]string{}, ""},
	}

	for _, tt := range tests {
		result := extractMitreID(tt.tags)
		if result != tt.expected {
			t.Errorf("extractMitreID(%v) = %s, want %s", tt.tags, result, tt.expected)
		}
	}
}

func TestNewRuleLoader(t *testing.T) {
	loader := NewRuleLoader(nil)
	if loader == nil {
		t.Fatal("expected non-nil loader")
	}
}
