package model

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestAgentGuardProcessColumnsMatchMigration(t *testing.T) {
	behaviorSchema, err := schema.Parse(&AgentBehaviorEvent{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse behavior schema: %v", err)
	}
	runtimeSchema, err := schema.Parse(&AgentRuntimeInstance{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse runtime schema: %v", err)
	}
	unitSchema, err := schema.Parse(&AgentExecutionUnit{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse execution unit schema: %v", err)
	}
	for _, test := range []struct {
		modelSchema *schema.Schema
		field       string
		want        string
	}{
		{behaviorSchema, "PID", "pid"},
		{behaviorSchema, "PPID", "ppid"},
		{runtimeSchema, "ControllerPID", "controller_pid"},
		{runtimeSchema, "RunUID", "run_uid"},
		{unitSchema, "RootPID", "root_pid"},
	} {
		field := test.modelSchema.LookUpField(test.field)
		if field == nil || field.DBName != test.want {
			t.Fatalf("%s DB column = %v, want %s", test.field, field, test.want)
		}
	}
}
