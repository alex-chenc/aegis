package service

import (
	"testing"
)

func TestNewBlockService(t *testing.T) {
	s := NewBlockService(nil)
	if s == nil {
		t.Fatal("expected non-nil service")
	}
}
