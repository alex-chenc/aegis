package service

import (
	"context"
	"testing"
)

func TestCreateAndDispatchTasksRejectsInvalidRuleIDBeforeCreatingGroup(t *testing.T) {
	svc := NewTaskService(nil, nil, nil, nil, nil, nil)
	result, err := svc.CreateAndDispatchTasks(context.Background(), []string{"not-a-uuid"}, []string{"also-not-a-uuid"}, "CHECK", nil)
	if err == nil {
		t.Fatalf("expected invalid task scope to fail, got result %#v", result)
	}
}

func TestCreateAndDispatchTasksRejectsEmptyScope(t *testing.T) {
	svc := NewTaskService(nil, nil, nil, nil, nil, nil)
	result, err := svc.CreateAndDispatchTasks(context.Background(), nil, nil, "CHECK", nil)
	if err == nil {
		t.Fatalf("expected empty task scope to fail, got result %#v", result)
	}
}
