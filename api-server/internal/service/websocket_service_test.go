package service

import (
	"testing"
)

func TestNewWebSocketService(t *testing.T) {
	s := NewWebSocketService()
	if s == nil {
		t.Fatal("expected non-nil service")
	}

	if s.GetClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", s.GetClientCount())
	}
}
