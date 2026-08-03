package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"api-server/internal/model"

	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type fakeAgentGuardBundleStore struct {
	draft          model.AgentGuardPolicy
	published      []model.AgentGuardPolicy
	hostIDs        []uuid.UUID
	maxVersion     int64
	deliveries     []model.AgentGuardPolicyDelivery
	statusByID     map[uuid.UUID]string
	publishInvoked bool
}

func (f *fakeAgentGuardBundleStore) GetByID(context.Context, uuid.UUID) (*model.AgentGuardPolicy, error) {
	copy := f.draft
	return &copy, nil
}

func (f *fakeAgentGuardBundleStore) List(context.Context, model.AgentGuardPolicyQuery) ([]model.AgentGuardPolicy, int64, error) {
	return f.published, int64(len(f.published)), nil
}

func (f *fakeAgentGuardBundleStore) ListDeliveries(context.Context, string, int64, model.AgentGuardDeliveryQuery) ([]model.AgentGuardPolicyDelivery, int64, error) {
	return f.deliveries, int64(len(f.deliveries)), nil
}

func (f *fakeAgentGuardBundleStore) ResolveTargetHostIDs(context.Context, model.AgentGuardPolicyTargets) ([]uuid.UUID, error) {
	return f.hostIDs, nil
}

func (f *fakeAgentGuardBundleStore) MaxBundleVersion(context.Context, []uuid.UUID) (int64, error) {
	return f.maxVersion, nil
}

func (f *fakeAgentGuardBundleStore) PublishDraftWithDeliveries(
	_ context.Context,
	_ uuid.UUID,
	_ string,
	deliveries []model.AgentGuardPolicyDelivery,
) (*model.AgentGuardPolicy, []model.AgentGuardPolicyDelivery, error) {
	f.publishInvoked = true
	f.deliveries = append([]model.AgentGuardPolicyDelivery(nil), deliveries...)
	copy := f.draft
	copy.Status = "published"
	return &copy, f.deliveries, nil
}

func (f *fakeAgentGuardBundleStore) UpdateDeliveryDispatchStatus(
	_ context.Context,
	id uuid.UUID,
	status string,
	_ string,
	_ string,
) error {
	if f.statusByID == nil {
		f.statusByID = map[uuid.UUID]string{}
	}
	f.statusByID[id] = status
	return nil
}

type fakeAgentGuardBundleCatalog struct{}

func (fakeAgentGuardBundleCatalog) ListProfiles(context.Context, model.AgentGuardProfileQuery) ([]model.AgentGuardAdapterProfile, int64, error) {
	return []model.AgentGuardAdapterProfile{{
		ID: uuid.New(), ProfileKey: model.AgentGuardProfileKeyCodexLinux,
		ProfileVersion: 1, AgentType: "codex", Enabled: true, Digest: "sha256:profile",
	}}, 1, nil
}

func (fakeAgentGuardBundleCatalog) ListRules(context.Context, model.AgentBehaviorRuleQuery) ([]model.AgentBehaviorRuleDefinition, int64, error) {
	return []model.AgentBehaviorRuleDefinition{{
		ID: uuid.New(), RuleKey: model.AgentGuardRuleKeySensitiveDirectory,
		RuleVersion: 1, DefaultEnabled: true, DefaultAction: "alert", Digest: "sha256:rule",
		DefaultParameters: datatypes.JSON([]byte(`{"resource_groups":["credential"]}`)),
	}}, 1, nil
}

type fakeAgentGuardBundleDispatcher struct {
	affected int32
	err      error
	hostID   string
	config   *pb.AgentConfig
}

func (f *fakeAgentGuardBundleDispatcher) SyncAgentConfig(_ context.Context, hostID string, configs []*pb.AgentConfig) (int32, error) {
	f.hostID = hostID
	if len(configs) == 1 {
		f.config = configs[0]
	}
	return f.affected, f.err
}

func TestAgentGuardBundlePublishBuildsMonitorOnlyBundleAndTracksDispatch(t *testing.T) {
	hostID := uuid.New()
	store := &fakeAgentGuardBundleStore{
		draft: model.AgentGuardPolicy{
			ID: uuid.New(), PolicyKey: "prod-agent-guard", Version: 1, Status: "draft",
			Targets:          datatypes.JSON([]byte(`{"host_ids":["` + hostID.String() + `"],"agent_types":["codex"]}`)),
			CollectionPolicy: datatypes.JSON([]byte(`{"categories":["process","tool"],"tool_adapter_enabled":true}`)),
			Digest:           "sha256:policy",
		},
		hostIDs:    []uuid.UUID{hostID},
		maxVersion: 41,
	}
	dispatcher := &fakeAgentGuardBundleDispatcher{affected: 1}
	service := NewAgentGuardBundleService(store, fakeAgentGuardBundleCatalog{}, dispatcher, true, true, nil)

	result, err := service.Publish(context.Background(), store.draft.ID, "admin")
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !store.publishInvoked || result.Policy.Status != "published" {
		t.Fatalf("policy was not published: %#v", result)
	}
	if dispatcher.hostID != hostID.String() || dispatcher.config == nil {
		t.Fatalf("unexpected dispatch: host=%q config=%#v", dispatcher.hostID, dispatcher.config)
	}
	if dispatcher.config.ConfigType != AgentGuardBundleConfigType {
		t.Fatalf("config_type=%q", dispatcher.config.ConfigType)
	}
	var bundle AgentGuardBundle
	if err := json.Unmarshal([]byte(dispatcher.config.ConfigJson), &bundle); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if bundle.Schema != AgentGuardBundleSchema || bundle.HostID != hostID.String() || bundle.BundleVersion <= 41 {
		t.Fatalf("unexpected bundle identity: %#v", bundle)
	}
	if bundle.Defaults.Mode != "monitor_only" || bundle.Defaults.EnforcementEnabled || bundle.Defaults.FreezeEnabled {
		t.Fatalf("P1 bundle enabled enforcement: %#v", bundle.Defaults)
	}
	if !bundle.Defaults.ToolAdapterEnabled {
		t.Fatalf("explicit control-plane and policy rollout did not enable tool adapter: %#v", bundle.Defaults)
	}
	if bundle.Digest == "" || len(bundle.Profiles) != 1 || len(bundle.BuiltinRules) != 1 {
		t.Fatalf("incomplete bundle: %#v", bundle)
	}
	if got := store.statusByID[store.deliveries[0].ID]; got != "dispatching" {
		t.Fatalf("delivery status=%q, want dispatching", got)
	}
}

func TestAgentGuardBundleToolAdapterRequiresGlobalAndPolicyGates(t *testing.T) {
	hostID := uuid.New()
	requested := model.AgentGuardPolicy{
		PolicyKey: "tool-rollout", Version: 1,
		CollectionPolicy: datatypes.JSON([]byte(`{"categories":["tool"],"tool_adapter_enabled":true}`)),
	}
	notRequested := model.AgentGuardPolicy{
		PolicyKey: "monitor-only", Version: 1,
		CollectionPolicy: datatypes.JSON([]byte(`{"categories":["process"],"tool_adapter_enabled":false}`)),
	}
	tests := []struct {
		name       string
		globalGate bool
		policies   []model.AgentGuardPolicy
		want       bool
	}{
		{name: "both gates", globalGate: true, policies: []model.AgentGuardPolicy{requested}, want: true},
		{name: "global disabled", globalGate: false, policies: []model.AgentGuardPolicy{requested}},
		{name: "policy disabled", globalGate: true, policies: []model.AgentGuardPolicy{notRequested}},
		{name: "no policies", globalGate: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, _, err := buildAgentGuardBundle(
				hostID, 1, time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC),
				nil, nil, test.policies, test.globalGate,
			)
			if err != nil {
				t.Fatalf("buildAgentGuardBundle: %v", err)
			}
			if bundle.Defaults.ToolAdapterEnabled != test.want {
				t.Fatalf("tool_adapter_enabled=%v, want %v", bundle.Defaults.ToolAdapterEnabled, test.want)
			}
		})
	}
}

func TestAgentGuardBundleRejectsInvalidToolAdapterPolicy(t *testing.T) {
	policy := model.AgentGuardPolicy{
		PolicyKey: "invalid-tool-rollout", Version: 1,
		CollectionPolicy: datatypes.JSON([]byte(`{"categories":["process"],"tool_adapter_enabled":true}`)),
	}
	if _, _, err := buildAgentGuardBundle(uuid.New(), 1, time.Now(), nil, nil, []model.AgentGuardPolicy{policy}, true); err == nil {
		t.Fatal("tool adapter rollout without tool collection category was accepted")
	}
}

func TestAgentGuardBundlePublishDisabledBeforeMutation(t *testing.T) {
	store := &fakeAgentGuardBundleStore{draft: model.AgentGuardPolicy{ID: uuid.New(), Status: "draft"}}
	service := NewAgentGuardBundleService(store, fakeAgentGuardBundleCatalog{}, &fakeAgentGuardBundleDispatcher{}, false, false, nil)

	if _, err := service.Publish(context.Background(), store.draft.ID, "admin"); !errors.Is(err, ErrAgentGuardPolicyPublishDisabled) {
		t.Fatalf("Publish error=%v, want disabled", err)
	}
	if store.publishInvoked {
		t.Fatal("disabled publish mutated repository")
	}
}

func TestAgentGuardBundleOfflineDispatchRemainsFailedNotApplied(t *testing.T) {
	hostID := uuid.New()
	store := &fakeAgentGuardBundleStore{
		draft: model.AgentGuardPolicy{
			ID: uuid.New(), PolicyKey: "offline-agent-guard", Version: 1, Status: "draft",
			Targets: datatypes.JSON([]byte(`{"host_ids":["` + hostID.String() + `"],"agent_types":["codex"]}`)),
		},
		hostIDs: []uuid.UUID{hostID},
	}
	service := NewAgentGuardBundleService(
		store,
		fakeAgentGuardBundleCatalog{},
		&fakeAgentGuardBundleDispatcher{affected: 0},
		true,
		false,
		nil,
	)

	if _, err := service.Publish(context.Background(), store.draft.ID, "admin"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got := store.statusByID[store.deliveries[0].ID]; got != "failed" {
		t.Fatalf("offline delivery status=%q, want failed", got)
	}
}
