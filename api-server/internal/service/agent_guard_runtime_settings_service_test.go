package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"api-server/internal/model"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type runtimeSettingsStoreFake struct {
	settings *model.AgentGuardRuntimeSettings
}

func (f *runtimeSettingsStoreFake) Get(hostID string) (*model.AgentGuardRuntimeSettings, error) {
	if f.settings == nil {
		defaults := model.DefaultAgentGuardRuntimeSettings(hostID)
		return &defaults, nil
	}
	copy := *f.settings
	return &copy, nil
}

func (f *runtimeSettingsStoreFake) Upsert(settings *model.AgentGuardRuntimeSettings) error {
	copy := *settings
	f.settings = &copy
	return nil
}

type runtimeSettingsDispatcherFake struct {
	hostID   string
	config   *pb.AgentConfig
	affected int32
}

func (f *runtimeSettingsDispatcherFake) SyncAgentConfig(_ context.Context, hostID string, configs []*pb.AgentConfig) (int32, error) {
	f.hostID = hostID
	if len(configs) > 0 {
		f.config = configs[0]
	}
	return f.affected, nil
}

func TestAgentGuardRuntimeSettingsUpdatePersistsAndDispatchesSanitizedPayload(t *testing.T) {
	hostID := uuid.New()
	store := &runtimeSettingsStoreFake{}
	dispatcher := &runtimeSettingsDispatcherFake{affected: 1}
	service := NewAgentGuardRuntimeSettingsService(store, dispatcher, zap.NewNop())
	service.now = func() time.Time { return time.UnixMilli(1000).UTC() }

	result, err := service.Update(context.Background(), model.AgentGuardRuntimeSettings{
		HostID: hostID.String(), ToolAdapterEnabled: true, SessionHookEnabled: false,
		Injections: []model.AgentGuardHookInjection{
			{AgentType: "claude-code", Enabled: true, Status: "forged"},
			{AgentType: "not-supported", Enabled: true},
		},
	}, "admin")
	if err != nil {
		t.Fatalf("update runtime settings: %v", err)
	}
	if result.DispatchStatus != "dispatched" || store.settings == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
	for _, injection := range result.Injections {
		if injection.Enabled && injection.Status != "dispatched" {
			t.Fatalf("enabled injection status = %q, want dispatched: %#v", injection.Status, result.Injections)
		}
	}
	if dispatcher.hostID != hostID.String() || dispatcher.config.ConfigType != AgentGuardRuntimeSettingsConfigType {
		t.Fatalf("settings were not dispatched to the requested host: %#v", dispatcher)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(dispatcher.config.ConfigJson), &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["status"]; ok {
		t.Fatal("control-plane status must not be sent to Agent")
	}
	if got := payload["host_id"]; got != hostID.String() {
		t.Fatalf("payload host_id = %v, want %s", got, hostID)
	}
	if got := len(payload["injections"].([]any)); got != len(model.AgentGuardHookAgentTypes) {
		t.Fatalf("injection payload count = %d, want %d", got, len(model.AgentGuardHookAgentTypes))
	}
}

func TestAgentGuardRuntimeSettingsGetRetriesPendingReconnect(t *testing.T) {
	hostID := uuid.New()
	settings := model.DefaultAgentGuardRuntimeSettings(hostID.String())
	settings.Version = 7
	settings.DispatchStatus = "pending_reconnect"
	store := &runtimeSettingsStoreFake{settings: &settings}
	dispatcher := &runtimeSettingsDispatcherFake{affected: 1}
	service := NewAgentGuardRuntimeSettingsService(store, dispatcher, zap.NewNop())

	result, err := service.Get(context.Background(), hostID)
	if err != nil {
		t.Fatalf("get runtime settings: %v", err)
	}
	if result.DispatchStatus != "dispatched" {
		t.Fatalf("dispatch status = %q, want dispatched", result.DispatchStatus)
	}
	if store.settings == nil || store.settings.DispatchStatus != "dispatched" {
		t.Fatalf("reconciled status was not persisted: %#v", store.settings)
	}
	for _, injection := range result.Injections {
		if injection.Enabled && injection.Status != "dispatched" {
			t.Fatalf("enabled injection status = %q, want dispatched", injection.Status)
		}
	}
}
