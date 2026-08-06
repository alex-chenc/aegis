package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fakeAgentGuardCatalog struct{}

func (fakeAgentGuardCatalog) ListProfiles(context.Context, model.AgentGuardProfileQuery) ([]model.AgentGuardAdapterProfile, int64, error) {
	return []model.AgentGuardAdapterProfile{}, 0, nil
}
func (fakeAgentGuardCatalog) GetProfile(context.Context, uuid.UUID) (*model.AgentGuardAdapterProfile, error) {
	return nil, repository.ErrAgentGuardProfileNotFound
}
func (fakeAgentGuardCatalog) ListRules(context.Context, model.AgentBehaviorRuleQuery) ([]model.AgentBehaviorRuleDefinition, int64, error) {
	return []model.AgentBehaviorRuleDefinition{}, 0, nil
}
func (fakeAgentGuardCatalog) GetRule(context.Context, string, int64) (*model.AgentBehaviorRuleDefinition, error) {
	return nil, repository.ErrAgentGuardRuleNotFound
}
func (fakeAgentGuardCatalog) ListRuleVersions(context.Context, string, int, int) ([]model.AgentBehaviorRuleDefinition, int64, error) {
	return []model.AgentBehaviorRuleDefinition{}, 0, nil
}

type fakeAgentGuardPolicies struct{}

func (fakeAgentGuardPolicies) List(context.Context, model.AgentGuardPolicyQuery) ([]model.AgentGuardPolicy, int64, error) {
	return []model.AgentGuardPolicy{}, 0, nil
}
func (fakeAgentGuardPolicies) GetByID(context.Context, uuid.UUID) (*model.AgentGuardPolicy, error) {
	return nil, repository.ErrAgentGuardPolicyNotFound
}
func (fakeAgentGuardPolicies) ListDeliveries(context.Context, string, int64, model.AgentGuardDeliveryQuery) ([]model.AgentGuardPolicyDelivery, int64, error) {
	return []model.AgentGuardPolicyDelivery{}, 0, nil
}

type fakeAgentGuardQuery struct {
	agents          []model.AgentGuardAgentSummary
	instances       []model.AgentRuntimeInstance
	sessions        []model.AgentBehaviorSession
	units           []model.AgentExecutionUnit
	behaviors       []model.AgentBehaviorEvent
	runtimeEvents   []model.RuntimeEvent
	findings        []model.AgentSecurityFinding
	analyses        []model.AgentSecurityAnalysisRun
	actions         []model.AgentGuardAction
	lastAgentQuery  model.AgentGuardAgentQuery
	lastInstance    model.AgentRuntimeInstanceQuery
	lastSession     model.AgentBehaviorSessionQuery
	lastFinding     model.AgentSecurityFindingQuery
	lastAction      model.AgentGuardActionQuery
	listAgentsCalls int
	instanceCalls   int
	instanceTotal   int64
}

func (f *fakeAgentGuardQuery) GetOverview(context.Context) (*model.AgentGuardOverview, error) {
	return &model.AgentGuardOverview{}, nil
}
func (f *fakeAgentGuardQuery) GetCoverage(context.Context, model.AgentRuntimeInstanceQuery) ([]model.AgentGuardCoverageSummary, int64, error) {
	return []model.AgentGuardCoverageSummary{}, 0, nil
}
func (f *fakeAgentGuardQuery) GetHostStatus(context.Context, uuid.UUID) (*model.AgentGuardHostStatus, error) {
	return nil, repository.ErrAgentGuardHostNotFound
}
func (f *fakeAgentGuardQuery) ListAgents(_ context.Context, query model.AgentGuardAgentQuery) ([]model.AgentGuardAgentSummary, int64, error) {
	f.listAgentsCalls++
	f.lastAgentQuery = query
	return f.agents, int64(len(f.agents)), nil
}
func (f *fakeAgentGuardQuery) ListInstances(_ context.Context, query model.AgentRuntimeInstanceQuery) ([]model.AgentRuntimeInstance, int64, error) {
	f.instanceCalls++
	f.lastInstance = query
	total := int64(len(f.instances))
	if f.instanceTotal > 0 && len(query.InstanceIDs) == 0 {
		total = f.instanceTotal
	}
	return f.instances, total, nil
}
func (f *fakeAgentGuardQuery) GetInstance(context.Context, uuid.UUID) (*model.AgentRuntimeInstance, error) {
	if len(f.instances) == 0 {
		return nil, repository.ErrAgentGuardInstanceNotFound
	}
	item := f.instances[0]
	return &item, nil
}
func (f *fakeAgentGuardQuery) ListSessions(_ context.Context, query model.AgentBehaviorSessionQuery) ([]model.AgentBehaviorSession, int64, error) {
	f.lastSession = query
	return f.sessions, int64(len(f.sessions)), nil
}
func (f *fakeAgentGuardQuery) GetSession(_ context.Context, id uuid.UUID) (*model.AgentBehaviorSession, error) {
	for index := range f.sessions {
		if f.sessions[index].ID == id {
			item := f.sessions[index]
			return &item, nil
		}
	}
	return nil, repository.ErrAgentGuardSessionNotFound
}
func (f *fakeAgentGuardQuery) ListExecutionUnits(context.Context, model.AgentExecutionUnitQuery) ([]model.AgentExecutionUnit, int64, error) {
	return f.units, int64(len(f.units)), nil
}

func (f *fakeAgentGuardQuery) GetExecutionUnit(_ context.Context, id uuid.UUID) (*model.AgentExecutionUnit, error) {
	for index := range f.units {
		if f.units[index].ID == id {
			item := f.units[index]
			return &item, nil
		}
	}
	return nil, repository.ErrAgentGuardExecutionUnitNotFound
}
func (f *fakeAgentGuardQuery) ListBehaviors(_ context.Context, query model.AgentBehaviorEventQuery) ([]model.AgentBehaviorEvent, int64, error) {
	items := make([]model.AgentBehaviorEvent, 0, len(f.behaviors))
	for _, behavior := range f.behaviors {
		if query.Category != "" && behavior.Category != query.Category {
			continue
		}
		items = append(items, behavior)
	}
	total := int64(len(items))
	page, pageSize := query.Page, query.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = len(items)
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []model.AgentBehaviorEvent{}, total, nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], total, nil
}
func (f *fakeAgentGuardQuery) ListProcessFacts(_ context.Context, query model.AgentBehaviorEventQuery, limit int) ([]model.AgentBehaviorEvent, int64, error) {
	items := make([]model.AgentBehaviorEvent, 0, len(f.behaviors))
	for _, behavior := range f.behaviors {
		if behavior.Category != "process" || query.InstanceID != "" && (behavior.InstanceID == nil || behavior.InstanceID.String() != query.InstanceID) ||
			query.SessionID != "" && (behavior.SessionID == nil || behavior.SessionID.String() != query.SessionID) ||
			query.ExecutionUnitID != "" && (behavior.ExecutionUnitID == nil || behavior.ExecutionUnitID.String() != query.ExecutionUnitID) {
			continue
		}
		items = append(items, behavior)
	}
	total := int64(len(items))
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}
func (f *fakeAgentGuardQuery) GetBehavior(_ context.Context, rawEventID string) (*model.AgentBehaviorEvent, error) {
	for index := range f.behaviors {
		if f.behaviors[index].RawEventID == rawEventID {
			item := f.behaviors[index]
			return &item, nil
		}
	}
	return nil, repository.ErrAgentGuardBehaviorNotFound
}
func (f *fakeAgentGuardQuery) GetRuntimeEvent(_ context.Context, eventID string) (*model.RuntimeEvent, error) {
	for index := range f.runtimeEvents {
		if f.runtimeEvents[index].EventID == eventID || f.runtimeEvents[index].ID.String() == eventID {
			item := f.runtimeEvents[index]
			return &item, nil
		}
	}
	return nil, repository.ErrAgentGuardBehaviorNotFound
}
func (f *fakeAgentGuardQuery) GetRawBehavior(context.Context, string) (*model.RuntimeEvent, error) {
	return nil, repository.ErrAgentGuardBehaviorNotFound
}
func (f *fakeAgentGuardQuery) ListFindings(_ context.Context, query model.AgentSecurityFindingQuery) ([]model.AgentSecurityFinding, int64, error) {
	f.lastFinding = query
	return f.findings, int64(len(f.findings)), nil
}
func (f *fakeAgentGuardQuery) GetFinding(_ context.Context, id uuid.UUID) (*model.AgentSecurityFinding, error) {
	for index := range f.findings {
		if f.findings[index].ID == id {
			item := f.findings[index]
			return &item, nil
		}
	}
	return nil, repository.ErrAgentGuardFindingNotFound
}
func (f *fakeAgentGuardQuery) ListAnalyses(context.Context, model.AgentSecurityAnalysisQuery) ([]model.AgentSecurityAnalysisRun, int64, error) {
	return f.analyses, int64(len(f.analyses)), nil
}
func (f *fakeAgentGuardQuery) GetAnalysis(_ context.Context, id uuid.UUID) (*model.AgentSecurityAnalysisRun, error) {
	for index := range f.analyses {
		if f.analyses[index].ID == id {
			item := f.analyses[index]
			return &item, nil
		}
	}
	return nil, repository.ErrAgentGuardAnalysisNotFound
}
func (f *fakeAgentGuardQuery) ListActions(_ context.Context, query model.AgentGuardActionQuery) ([]model.AgentGuardAction, int64, error) {
	f.lastAction = query
	return f.actions, int64(len(f.actions)), nil
}

func (f *fakeAgentGuardQuery) GetAction(_ context.Context, id uuid.UUID) (*model.AgentGuardAction, error) {
	for index := range f.actions {
		if f.actions[index].ID == id {
			item := f.actions[index]
			return &item, nil
		}
	}
	return nil, repository.ErrAgentGuardActionNotFound
}

type fakeAgentGuardAnalysisRequester struct {
	findingID   uuid.UUID
	requestedBy string
}

type fakeAgentGuardActionRequester struct {
	unitID      uuid.UUID
	instanceID  uuid.UUID
	action      string
	request     service.AgentGuardManualActionRequest
	requestedBy string
}

func (f *fakeAgentGuardActionRequester) RequestExecutionUnit(
	_ context.Context,
	unitID uuid.UUID,
	action string,
	request service.AgentGuardManualActionRequest,
	requestedBy string,
) (*model.AgentGuardAction, error) {
	f.unitID, f.action, f.request, f.requestedBy = unitID, action, request, requestedBy
	actionID := uuid.New()
	return &model.AgentGuardAction{
		ID: actionID, CommandID: "AG-GUARD-" + actionID.String(), ExecutionUnitID: &unitID, Action: action,
		Status: model.AgentGuardActionStatusDispatching,
	}, nil
}

func (f *fakeAgentGuardActionRequester) RequestInstanceKill(
	_ context.Context,
	instanceID uuid.UUID,
	request service.AgentGuardManualActionRequest,
	requestedBy string,
) (*model.AgentGuardAction, error) {
	f.instanceID, f.action, f.request, f.requestedBy = instanceID, model.AgentGuardActionKillAgentInstance, request, requestedBy
	actionID := uuid.New()
	return &model.AgentGuardAction{
		ID: actionID, CommandID: "AG-GUARD-" + actionID.String(), InstanceID: &instanceID, Action: model.AgentGuardActionKillAgentInstance,
		Status: model.AgentGuardActionStatusDispatching,
	}, nil
}

func (f *fakeAgentGuardAnalysisRequester) Request(
	_ context.Context,
	findingID uuid.UUID,
	requestedBy string,
) (*model.AgentSecurityAnalysisRun, error) {
	f.findingID = findingID
	f.requestedBy = requestedBy
	return &model.AgentSecurityAnalysisRun{
		ID:        uuid.New(),
		FindingID: findingID,
		Status:    model.AgentGuardAnalysisStatusPending,
	}, nil
}

func TestAgentGuardAgentsHaveStableLogicalOpaqueKeys(t *testing.T) {
	hostID := uuid.New()
	assetOne := uuid.New()
	assetTwo := uuid.New()
	query := &fakeAgentGuardQuery{agents: []model.AgentGuardAgentSummary{
		{
			AssetID: &assetOne, Host: model.AgentGuardHostSummary{ID: hostID},
			AgentType: "codex", ProfileKey: "codex-linux",
		},
		{
			AssetID: &assetTwo, Host: model.AgentGuardHostSummary{ID: hostID},
			AgentType: "codex", ProfileKey: "codex-linux",
		},
		{
			Host:      model.AgentGuardHostSummary{ID: hostID},
			AgentType: "hermes", ProfileKey: "hermes-linux",
		},
	}}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))

	response := serveAgentGuardRequest(engine, http.MethodGet, "/api/v1/agent-guard/agents?page=1&page_size=20", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			Items []model.AgentGuardAgentSummary `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Items) != 3 {
		t.Fatalf("items=%d, want 3", len(body.Data.Items))
	}
	for _, item := range body.Data.Items {
		if !strings.HasPrefix(item.AgentScopeKey, "ags1.") {
			t.Fatalf("agent_scope_key=%q, want signed key", item.AgentScopeKey)
		}
	}
	if body.Data.Items[0].AgentScopeKey != body.Data.Items[1].AgentScopeKey {
		t.Fatal("same host/type/profile must share one logical scope key")
	}
	if body.Data.Items[0].AgentScopeKey == body.Data.Items[2].AgentScopeKey {
		t.Fatal("different agent types must have distinct logical scope keys")
	}
}

func TestAgentGuardSignedAssetScopeCannotExpandToSiblingAsset(t *testing.T) {
	hostID := uuid.New()
	assetOne := uuid.New()
	assetTwo := uuid.New()
	instanceOne := uuid.New()
	signer := testAgentGuardSigner(t)
	query := &fakeAgentGuardQuery{instances: []model.AgentRuntimeInstance{{
		ID: instanceOne, HostID: hostID, AssetID: &assetOne, AgentType: "codex",
		ProfileKey: "codex-linux", ControllerCmdline: "secret --token", ControllerExe: "/secret/codex",
		Metadata: []byte(`{"sensitive":true}`),
	}}}
	engine := newAgentGuardHandlerTestEngine(t, query, signer)

	scopeOne, err := signer.Sign(service.AgentGuardScope{
		HostID: hostID.String(), AgentType: "codex", ProfileKey: "codex-linux", AssetID: assetOne.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	scopeTwo, err := signer.Sign(service.AgentGuardScope{
		HostID: hostID.String(), AgentType: "codex", ProfileKey: "codex-linux", AssetID: assetTwo.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if scopeOne == scopeTwo {
		t.Fatal("two linked assets received the same signed scope")
	}

	response := serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/instances?agent_scope_key="+scopeOne,
		nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := query.lastInstance.AssetIDs; len(got) != 1 || got[0] != assetOne.String() {
		t.Fatalf("scope expanded to query %#v, want only asset one", query.lastInstance)
	}
	if len(query.lastInstance.AgentTypes) != 0 || query.lastInstance.ProfileKey != "" {
		t.Fatalf("linked scope was downgraded to broad type/profile query: %#v", query.lastInstance)
	}
	for _, secret := range []string{"secret --token", "/secret/codex", "sensitive"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("read response leaked %q: %s", secret, response.Body.String())
		}
	}

	response = serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/instances?agent_scope_key="+scopeTwo,
		nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("sibling status=%d body=%s", response.Code, response.Body.String())
	}
	if got := query.lastInstance.AssetIDs; len(got) != 1 || got[0] != assetTwo.String() {
		t.Fatalf("sibling scope expanded to query %#v, want only asset two", query.lastInstance)
	}
}

func TestAgentGuardInvalidScopeRejectedBeforeRepository(t *testing.T) {
	query := &fakeAgentGuardQuery{}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))
	response := serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/instances?agent_scope_key=ags1.tampered.value",
		nil,
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if query.instanceCalls != 0 {
		t.Fatalf("repository called %d times for invalid scope", query.instanceCalls)
	}
	assertAgentGuardErrorCode(t, response, "agent_guard_scope_invalid")
}

func TestAgentGuardPanoramaAcceptsFrontendSingularAssetID(t *testing.T) {
	hostID := uuid.New()
	assetID := uuid.New()
	query := &fakeAgentGuardQuery{instances: []model.AgentRuntimeInstance{{
		ID: uuid.New(), HostID: hostID, AssetID: &assetID, AgentType: "codex",
		DisplayName: "Codex", LastSeenAt: time.Now().UTC(),
	}}}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))
	response := serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/panorama?asset_id="+assetID.String(),
		nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := query.lastInstance.AssetIDs; len(got) != 1 || got[0] != assetID.String() {
		t.Fatalf("singular asset_id mapped to %#v", query.lastInstance)
	}
	if query.lastInstance.Status != "running" {
		t.Fatalf("panorama included historical instances: %#v", query.lastInstance)
	}
	if !strings.Contains(response.Body.String(), `"node_type":"agent_asset"`) {
		t.Fatalf("missing panorama root: %s", response.Body.String())
	}
}

func TestAgentGuardFindingScopeUsesResolvedInstanceIDsAndRedactsEvidence(t *testing.T) {
	hostID := uuid.New()
	assetID := uuid.New()
	instanceID := uuid.New()
	sessionID := uuid.New()
	query := &fakeAgentGuardQuery{
		instances: []model.AgentRuntimeInstance{{ID: instanceID, HostID: hostID, AssetID: &assetID}},
		findings: []model.AgentSecurityFinding{{
			ID: uuid.New(), HostID: hostID, InstanceID: &instanceID, Title: "finding",
			EvidenceEventIDs: []byte(`["secret-event"]`),
			EvidenceGraph:    []byte(`{"path":"/secret"}`),
			Summary:          "secret summary",
		}},
	}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))
	response := serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/findings?asset_id="+assetID.String()+"&session_id="+sessionID.String()+"&finding_domain=tool&page=1&page_size=20",
		nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if query.lastFinding.AssetID != assetID.String() ||
		query.lastFinding.SessionID != sessionID.String() ||
		query.lastFinding.FindingDomain != model.AgentSecurityFindingDomainTool ||
		len(query.lastFinding.InstanceIDs) != 0 {
		t.Fatalf("finding query did not preserve direct asset scope: %#v", query.lastFinding)
	}
	for _, secret := range []string{"secret-event", "/secret", "secret summary"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("finding list leaked %q: %s", secret, response.Body.String())
		}
	}
}

func TestAgentGuardFindingLogicalScopeDoesNotEnumerateHistoricalInstances(t *testing.T) {
	hostID := uuid.New()
	signer := testAgentGuardSigner(t)
	query := &fakeAgentGuardQuery{instanceTotal: 636}
	engine := newAgentGuardHandlerTestEngine(t, query, signer)
	scopeKey, err := signer.Sign(service.AgentGuardScope{
		HostID: hostID.String(), AgentType: "codex", ProfileKey: "codex-linux",
	})
	if err != nil {
		t.Fatal(err)
	}

	response := serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/findings?agent_scope_key="+scopeKey+"&page=1&page_size=20",
		nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if query.instanceCalls != 0 {
		t.Fatalf("logical scope enumerated %d runtime-instance pages", query.instanceCalls)
	}
	if query.lastFinding.HostID != hostID.String() || query.lastFinding.AgentType != "codex" ||
		query.lastFinding.ProfileKey != "codex-linux" {
		t.Fatalf("finding scope not preserved: %#v", query.lastFinding)
	}
}

func TestAgentGuardFindingLogicalScopeValidatesOnlySelectedInstance(t *testing.T) {
	hostID := uuid.New()
	instanceID := uuid.New()
	signer := testAgentGuardSigner(t)
	query := &fakeAgentGuardQuery{
		instanceTotal: 636,
		instances: []model.AgentRuntimeInstance{{
			ID: instanceID, HostID: hostID, AgentType: "codex", ProfileKey: "codex-linux",
		}},
	}
	engine := newAgentGuardHandlerTestEngine(t, query, signer)
	scopeKey, err := signer.Sign(service.AgentGuardScope{
		HostID: hostID.String(), AgentType: "codex", ProfileKey: "codex-linux",
	})
	if err != nil {
		t.Fatal(err)
	}

	response := serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/findings?agent_scope_key="+scopeKey+"&instance_id="+instanceID.String(),
		nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if query.instanceCalls != 1 || len(query.lastInstance.InstanceIDs) != 1 ||
		query.lastInstance.InstanceIDs[0] != instanceID.String() {
		t.Fatalf("scope enumerated history instead of selected instance: %#v", query.lastInstance)
	}
}

func TestAgentGuardFindingLogicalScopeRejectsExplicitForeignInstance(t *testing.T) {
	hostID := uuid.New()
	signer := testAgentGuardSigner(t)
	query := &fakeAgentGuardQuery{}
	engine := newAgentGuardHandlerTestEngine(t, query, signer)
	scopeKey, err := signer.Sign(service.AgentGuardScope{
		HostID: hostID.String(), AgentType: "codex", ProfileKey: "codex-linux",
	})
	if err != nil {
		t.Fatal(err)
	}

	response := serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/findings?agent_scope_key="+scopeKey+"&instance_id="+uuid.NewString(),
		nil,
	)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if query.instanceCalls != 1 || len(query.lastInstance.InstanceIDs) != 1 {
		t.Fatalf("explicit instance membership was not checked: %#v", query.lastInstance)
	}
}

func TestAgentGuardP3RouteAndPermissionContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAgentGuardHandler(
		fakeAgentGuardCatalog{},
		fakeAgentGuardPolicies{},
		&fakeAgentGuardQuery{},
		nil,
		testAgentGuardSigner(t),
		nil,
	)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	marker := func(permission string) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Header("X-Agent-Guard-Permission", permission)
			c.Next()
		}
	}
	handler.RegisterRoutes(
		v1,
		marker("read"),
		marker("evidence"),
		marker("analysis"),
		marker("analysis-run"),
		marker("policy"),
		marker("publish"),
		marker("session-delete"),
		marker("freeze"),
		marker("resume"),
		marker("kill"),
	)

	cases := []struct {
		method     string
		path       string
		permission string
		body       []byte
	}{
		{http.MethodGet, "/api/v1/agent-guard/agents", "read", nil},
		{http.MethodGet, "/api/v1/agent-guard/execution-units/" + uuid.NewString() + "/timeline", "read", nil},
		{http.MethodGet, "/api/v1/agent-guard/behaviors/event-1/raw", "evidence", nil},
		{http.MethodGet, "/api/v1/agent-guard/findings/" + uuid.NewString(), "analysis", nil},
		{http.MethodGet, "/api/v1/agent-guard/findings/" + uuid.NewString() + "/analyses", "analysis", nil},
		{http.MethodPost, "/api/v1/agent-guard/findings/" + uuid.NewString() + "/analyze", "analysis-run", []byte(`{}`)},
		{http.MethodPost, "/api/v1/agent-guard/policies", "policy", []byte(`{} {}`)},
		{http.MethodPost, "/api/v1/agent-guard/policies/" + uuid.NewString() + "/publish", "publish", []byte(`{}`)},
		{http.MethodDelete, "/api/v1/agent-guard/sessions", "session-delete", []byte(`{"session_ids":["` + uuid.NewString() + `"]}`)},
		{http.MethodPost, "/api/v1/agent-guard/execution-units/" + uuid.NewString() + "/freeze", "freeze", []byte(`{"reason":"manual containment"}`)},
		{http.MethodPost, "/api/v1/agent-guard/execution-units/" + uuid.NewString() + "/resume", "resume", []byte(`{"reason":"manual recovery"}`)},
		{http.MethodPost, "/api/v1/agent-guard/execution-units/" + uuid.NewString() + "/kill", "kill", []byte(`{"reason":"manual termination"}`)},
		{http.MethodPost, "/api/v1/agent-guard/instances/" + uuid.NewString() + "/kill", "kill", []byte(`{"reason":"manual termination"}`)},
	}
	for _, testCase := range cases {
		response := serveAgentGuardRequest(engine, testCase.method, testCase.path, testCase.body)
		if got := response.Header().Get("X-Agent-Guard-Permission"); got != testCase.permission {
			t.Errorf("%s %s permission=%q, want %q", testCase.method, testCase.path, got, testCase.permission)
		}
	}

	for _, forbidden := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/agent-guard/profiles"},
		{http.MethodPut, "/api/v1/agent-guard/rules/AGB-BUILTIN-001"},
	} {
		response := serveAgentGuardRequest(engine, forbidden.method, forbidden.path, []byte(`{}`))
		if response.Code != http.StatusNotFound {
			t.Errorf("forbidden P1 route %s %s status=%d", forbidden.method, forbidden.path, response.Code)
		}
	}
}

func TestAgentGuardAnalyzeFindingQueuesReadOnlyAnalysis(t *testing.T) {
	findingID := uuid.New()
	hostID := uuid.New()
	query := &fakeAgentGuardQuery{findings: []model.AgentSecurityFinding{{
		ID:     findingID,
		HostID: hostID,
	}}}
	requester := &fakeAgentGuardAnalysisRequester{}
	handler := NewAgentGuardHandler(
		fakeAgentGuardCatalog{},
		fakeAgentGuardPolicies{},
		query,
		nil,
		testAgentGuardSigner(t),
		nil,
	)
	handler.SetAnalysisService(requester)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("username", "security-developer")
		c.Next()
	})
	pass := func(c *gin.Context) { c.Next() }
	handler.RegisterRoutes(engine.Group("/api/v1"), pass, pass, pass, pass, pass, pass, pass, pass, pass, pass)

	response := serveAgentGuardRequest(
		engine,
		http.MethodPost,
		"/api/v1/agent-guard/findings/"+findingID.String()+"/analyze",
		[]byte(`{}`),
	)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if requester.findingID != findingID || requester.requestedBy != "security-developer" {
		t.Fatalf("requester binding=%s/%q", requester.findingID, requester.requestedBy)
	}
}

func TestAgentGuardManualActionStrictBodyAndAuthenticatedOperator(t *testing.T) {
	unitID := uuid.New()
	requester := &fakeAgentGuardActionRequester{}
	handler := NewAgentGuardHandler(
		fakeAgentGuardCatalog{}, fakeAgentGuardPolicies{}, &fakeAgentGuardQuery{},
		nil, testAgentGuardSigner(t), nil,
	)
	handler.SetActionService(requester)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("username", "security-admin")
		c.Next()
	})
	pass := func(c *gin.Context) { c.Next() }
	handler.RegisterRoutes(engine.Group("/api/v1"), pass, pass, pass, pass, pass, pass, pass, pass, pass, pass)

	response := serveAgentGuardRequest(
		engine,
		http.MethodPost,
		"/api/v1/agent-guard/execution-units/"+unitID.String()+"/freeze",
		[]byte(`{"reason":"confirmed namespace escape","hold":false}`),
	)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if requester.unitID != unitID || requester.action != model.AgentGuardActionFreezeExecutionUnit ||
		requester.requestedBy != "security-admin" || requester.request.Hold {
		t.Fatalf("unexpected action request: %#v", requester)
	}
	var accepted struct {
		Data struct {
			ActionID  string `json:"action_id"`
			CommandID string `json:"command_id"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil || accepted.Data.ActionID == "" || accepted.Data.CommandID == "" ||
		accepted.Data.Status != model.AgentGuardActionStatusDispatching {
		t.Fatalf("frontend action acceptance contract: body=%s err=%v", response.Body.String(), err)
	}

	response = serveAgentGuardRequest(
		engine,
		http.MethodPost,
		"/api/v1/agent-guard/execution-units/"+unitID.String()+"/freeze",
		[]byte(`{"reason":"manual","host_id":"*"}`),
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown target field status=%d body=%s", response.Code, response.Body.String())
	}
	assertAgentGuardErrorCode(t, response, "agent_guard_request_invalid")

	response = serveAgentGuardRequest(
		engine,
		http.MethodPost,
		"/api/v1/agent-guard/execution-units/"+unitID.String()+"/freeze",
		[]byte(`{"reason":"manual containment","freeze_timeout_seconds":120}`),
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("external timeout field status=%d body=%s", response.Code, response.Body.String())
	}
	assertAgentGuardErrorCode(t, response, "agent_guard_request_invalid")
}

func TestAgentGuardExecutionUnitTimelinePinsResolvedUUIDFilter(t *testing.T) {
	unitID := uuid.New()
	query := &fakeAgentGuardQuery{
		units: []model.AgentExecutionUnit{{ID: unitID}},
		actions: []model.AgentGuardAction{{
			ID: uuid.New(), ExecutionUnitID: &unitID, Action: model.AgentGuardActionFreezeExecutionUnit,
			Status: model.AgentGuardActionStatusSuccess,
		}},
	}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))
	response := serveAgentGuardRequest(
		engine, http.MethodGet,
		"/api/v1/agent-guard/execution-units/"+unitID.String()+"/timeline?page=1&page_size=100", nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if query.lastAction.ExecutionUnitID != unitID.String() || query.lastAction.Page != 1 || query.lastAction.PageSize != 100 {
		t.Fatalf("timeline query escaped target: %#v", query.lastAction)
	}
}

func TestAgentGuardFindingDetailExposesEvidenceAssessmentFields(t *testing.T) {
	findingID := uuid.New()
	query := &fakeAgentGuardQuery{findings: []model.AgentSecurityFinding{{
		ID:               findingID,
		HostID:           uuid.New(),
		EvidenceEventIDs: []byte(`["event-1","event-2"]`),
		EvidenceGraph: []byte(`{
			"counter_evidence_ids":["event-2"],
			"tool_semantics":{"remote_coverage":"remote_unobservable","limitations":["remote_sensor_evidence_not_yet_correlated"]}
		}`),
	}}}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))
	response := serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/findings/"+findingID.String(),
		nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			EvidenceCompleteness map[string]any `json:"evidence_completeness"`
			EvidenceGraph        map[string]any `json:"evidence_graph"`
			CounterEvidence      []string       `json:"counter_evidence"`
			Uncertainties        []string       `json:"uncertainties"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.EvidenceCompleteness["referenced_event_count"] != float64(2) ||
		len(body.Data.CounterEvidence) != 1 ||
		body.Data.CounterEvidence[0] != "event-2" ||
		body.Data.Uncertainties == nil {
		t.Fatalf("unexpected evidence assessment: %#v", body.Data)
	}
	toolSemantics, _ := body.Data.EvidenceGraph["tool_semantics"].(map[string]any)
	if toolSemantics["remote_coverage"] != "remote_unobservable" {
		t.Fatalf("finding evidence lost remote coverage: %#v", body.Data.EvidenceGraph)
	}
}

func TestAgentGuardFindingDetailDoesNotExposeMatchedProcessTree(t *testing.T) {
	hostID := uuid.New()
	instanceID := uuid.New()
	sessionID := uuid.New()
	unitID := uuid.New()
	rootPID, rootPPID := 100, 1
	childPID, childPPID := 200, 100
	root := processBehavior("process-root", hostID, instanceID, sessionID, unitID,
		rootPID, rootPPID, "100", "exec", "agent", time.Unix(100, 0).UTC())
	root.CommandArgv = []byte(`["/usr/bin/agent","--run"]`)
	child := processBehavior("process-child", hostID, instanceID, sessionID, unitID,
		childPID, childPPID, "200", "exec", "shell", time.Unix(101, 0).UTC())
	child.CommandArgv = []byte(`["/bin/sh","-c","touch /etc/passwd"]`)
	unrelatedEvidence := processBehavior("process-evidence-only", hostID, instanceID, sessionID, unitID,
		300, 1, "300", "exec", "unrelated", time.Unix(103, 0).UTC())
	hit := model.AgentBehaviorEvent{
		RawEventID: "event-hit", HostID: hostID, InstanceID: &instanceID, SessionID: &sessionID,
		ExecutionUnitID: &unitID, Category: "file", Operation: "write", Outcome: "success",
		PID: &childPID, PPID: &childPPID, ProcessStartTicks: "200", ProcessName: "shell",
		CommandArgv: []byte(`["/bin/sh","-c","touch /etc/passwd"]`), Severity: "high",
		OccurredAt: time.Unix(102, 0).UTC(),
	}
	tool := model.AgentBehaviorEvent{
		RawEventID: "tool-call", HostID: hostID, InstanceID: &instanceID, SessionID: &sessionID,
		ExecutionUnitID: &unitID, Category: "tool", Operation: "tool_call_completed", Outcome: "success",
		ResourceIdentity: "Bash", CorrelationID: "sha256:tool-correlation",
		Resource:    []byte(`{"attributes":{"tool_call_id":"call-1","command":"touch /etc/passwd","process_event_id":"event-hit","correlation_status":"matched","correlation_method":"ebpf_command_match"}}`),
		CommandArgv: []byte(`["/bin/bash","-lc","touch /etc/passwd"]`),
		PID:         &childPID, PPID: &childPPID, OccurredAt: time.Unix(102, 0).UTC(),
	}
	findingID := uuid.New()
	query := &fakeAgentGuardQuery{
		behaviors: []model.AgentBehaviorEvent{root, child, unrelatedEvidence, hit, tool},
		findings: []model.AgentSecurityFinding{{
			ID: findingID, HostID: hostID, InstanceID: &instanceID, SessionID: &sessionID,
			ExecutionUnitID: &unitID, RuleHits: []byte(`[{"rule_key":"AGB-BUILTIN-004","rule_version":1,"rule_name":"敏感命令","event_id":"event-hit","evidence_event_ids":["process-evidence-only"],"severity":"high"}]`),
			EvidenceEventIDs: []byte(`["event-hit"]`),
		}},
	}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))
	response := serveAgentGuardRequest(engine, http.MethodGet,
		"/api/v1/agent-guard/findings/"+findingID.String(), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			MatchedRules []struct {
				Name      string `json:"name"`
				ToolCalls []struct {
					ToolName string `json:"tool_name"`
					Command  string `json:"command"`
				} `json:"tool_calls"`
				ProcessTree []struct {
					PID      int    `json:"pid"`
					PPID     int    `json:"ppid"`
					Cmdline  string `json:"cmdline"`
					Matched  bool   `json:"matched"`
					Children []struct {
						PID     int    `json:"pid"`
						Cmdline string `json:"cmdline"`
						Matched bool   `json:"matched"`
					} `json:"children"`
				} `json:"process_tree"`
			} `json:"matched_rules"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.MatchedRules) != 1 || body.Data.MatchedRules[0].Name != "敏感命令" ||
		len(body.Data.MatchedRules[0].ProcessTree) != 0 || len(body.Data.MatchedRules[0].ToolCalls) != 1 ||
		body.Data.MatchedRules[0].ToolCalls[0].ToolName != "Bash" ||
		body.Data.MatchedRules[0].ToolCalls[0].Command != "touch /etc/passwd" {
		t.Fatalf("unexpected matched rule detail: %s", response.Body.String())
	}
}

func TestAgentGuardUnmatchedToolDoesNotExposeHookPIDAsExecutor(t *testing.T) {
	pid, ppid := 6006, 1
	call := projectAgentGuardFindingToolCall(model.AgentBehaviorEvent{
		RawEventID: "tool-unmatched", Category: "tool", ResourceIdentity: "Bash",
		Resource:    []byte(`{"attributes":{"command":"true","correlation_status":"unmatched"}}`),
		CommandArgv: []byte(`["codex","--worker"]`), PID: &pid, PPID: &ppid,
	})
	if call.PID != 0 || call.PPID != 0 || call.CommandLine != "" || call.ProcessStartTicks != "" {
		t.Fatalf("hook PID leaked as executor: %#v", call)
	}
}

func TestAgentGuardFindingDetailDoesNotExposeRuntimeProcessTree(t *testing.T) {
	hostID, instanceID, sessionID, unitID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	base := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	root := processBehavior("process-root", hostID, instanceID, sessionID, unitID,
		100, 1, "100", "exec", "agent", base)
	root.CommandArgv = []byte(`["/usr/bin/agent","--run"]`)
	child := processBehavior("process-child", hostID, instanceID, sessionID, unitID,
		200, 100, "200", "exec", "sh", base.Add(time.Second))
	child.CommandArgv = []byte(`["/bin/sh","-c","touch /var/run/docker.sock"]`)
	findingID := uuid.New()
	runtimeEvent := model.RuntimeEvent{
		ID:        uuid.New(),
		EventID:   "runtime-hit",
		HostID:    hostID,
		EventType: "agent_sandbox_violation",
		EventData: fmt.Sprintf(`{
			"category":"isolation","operation":"connect_unix_violation","outcome":"success",
			"decision":"would_deny","severity":"critical","instance_id":"%s","session_id":"%s",
			"execution_unit_id":"%s","occurred_at":"2026-08-05T10:00:02Z",
			"actor":{"pid":200,"ppid":100,"start_ticks":"200","exe":"/bin/sh","cwd":"/tmp",
			"argv":["/bin/sh","-c","touch /var/run/docker.sock"]},
			"collection":{"visibility":"complete"}}`, instanceID, sessionID, unitID),
		PID:         200,
		CommandLine: "/bin/sh -c touch /var/run/docker.sock",
		Severity:    "critical",
		CreatedAt:   base.Add(2 * time.Second),
	}
	query := &fakeAgentGuardQuery{
		behaviors:     []model.AgentBehaviorEvent{root, child},
		runtimeEvents: []model.RuntimeEvent{runtimeEvent},
		findings: []model.AgentSecurityFinding{{
			ID: findingID, HostID: hostID, InstanceID: &instanceID, SessionID: &sessionID,
			ExecutionUnitID: &unitID,
			RuleHits: []byte(`[{
				"rule_key":"access_container_runtime_socket","rule_name":"访问容器运行时套接字",
				"event_id":"runtime-hit","severity":"critical"}]`),
			EvidenceEventIDs: []byte(`["runtime-hit"]`),
		}},
	}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))
	response := serveAgentGuardRequest(engine, http.MethodGet,
		"/api/v1/agent-guard/findings/"+findingID.String(), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			MatchedRules []struct {
				Name        string `json:"name"`
				ProcessTree []struct {
					PID      int    `json:"pid"`
					Cmdline  string `json:"cmdline"`
					Matched  bool   `json:"matched"`
					Children []struct {
						PID     int    `json:"pid"`
						Cmdline string `json:"cmdline"`
						Matched bool   `json:"matched"`
					} `json:"children"`
				} `json:"process_tree"`
			} `json:"matched_rules"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.MatchedRules) != 1 || body.Data.MatchedRules[0].Name != "访问容器运行时套接字" ||
		len(body.Data.MatchedRules[0].ProcessTree) != 0 {
		t.Fatalf("runtime event exposed a process tree: %s", response.Body.String())
	}
}

func TestAgentGuardEscapeFindingDetailBuildsHookProcessAndExecutionEvidenceChain(t *testing.T) {
	hostID, instanceID := uuid.New(), uuid.New()
	findingID := uuid.New()
	runtimeEvent := model.RuntimeEvent{
		ID: uuid.New(), EventID: "escape-runtime-hit", HostID: hostID,
		EventType: "agent_sandbox_violation",
		EventData: fmt.Sprintf(`{
			"category":"escape","operation":"connect_unix","outcome":"success",
			"decision":"would_deny","instance_id":"%s",
			"actor":{"pid":200,"ppid":100,"start_ticks":"200","exe":"/bin/sh","argv":["/bin/sh","-c","touch /var/run/docker.sock"]},
			"evidence":{"permission":{"class":"restricted","complete":true},"rule":"access_container_runtime_socket","operation":"connect_unix","hook_pid_matched":true}}`, instanceID),
		PID: 200, CommandLine: "/bin/sh -c touch /var/run/docker.sock", CreatedAt: time.Now().UTC(),
	}
	query := &fakeAgentGuardQuery{
		runtimeEvents: []model.RuntimeEvent{runtimeEvent},
		findings: []model.AgentSecurityFinding{{
			ID: findingID, HostID: hostID, InstanceID: &instanceID,
			Title:            "Agent sandbox violation: access_container_runtime_socket",
			RuleHits:         []byte(`[{"rule_key":"access_container_runtime_socket","event_id":"escape-runtime-hit"}]`),
			EvidenceEventIDs: []byte(`["escape-runtime-hit"]`),
		}},
	}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))
	response := serveAgentGuardRequest(engine, http.MethodGet, "/api/v1/agent-guard/findings/"+findingID.String(), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			EscapeChain struct {
				HookEvents []struct {
					ToolName string `json:"tool_name"`
					Command  string `json:"command"`
					PID      int    `json:"pid"`
				} `json:"hook_events"`
				ProcessEvidence   []map[string]any `json:"process_evidence"`
				ExecutionEvidence []map[string]any `json:"execution_evidence"`
				Permission        map[string]any   `json:"permission"`
			} `json:"escape_chain"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	chain := body.Data.EscapeChain
	if len(chain.HookEvents) != 1 || chain.HookEvents[0].ToolName != "sh" ||
		chain.HookEvents[0].Command != "/bin/sh -c touch /var/run/docker.sock" || chain.HookEvents[0].PID != 200 ||
		len(chain.ProcessEvidence) == 0 || len(chain.ExecutionEvidence) != 1 ||
		chain.Permission["class"] != "restricted" {
		t.Fatalf("unexpected escape evidence chain: %s", response.Body.String())
	}
}

func TestAgentGuardSessionReadReturnsExternalSessionIDWithoutCorrelationHash(t *testing.T) {
	instanceID := uuid.New()
	sessionID := uuid.New()
	query := &fakeAgentGuardQuery{sessions: []model.AgentBehaviorSession{{
		ID: sessionID, InstanceID: instanceID, HostID: uuid.New(),
		ExternalSessionID:    "upstream-session-secret",
		CorrelationTokenHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Source:               model.AgentGuardSessionSourceAdapterHook, Confidence: "confirmed",
	}}}
	engine := newAgentGuardHandlerTestEngine(t, query, testAgentGuardSigner(t))

	for _, path := range []string{
		"/api/v1/agent-guard/instances/" + instanceID.String() + "/sessions",
		"/api/v1/agent-guard/sessions/" + sessionID.String(),
	} {
		response := serveAgentGuardRequest(engine, http.MethodGet, path, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), "upstream-session-secret") {
			t.Fatalf("GET %s did not return the real external session ID: %s", path, response.Body.String())
		}
		for _, secret := range []string{"correlation_token_hash", "sha256:aaaaaaaa"} {
			if strings.Contains(response.Body.String(), secret) {
				t.Fatalf("GET %s leaked %q: %s", path, secret, response.Body.String())
			}
		}
	}
}

func TestAgentGuardPanoramaProjectsOnlyTrustedToolSemantics(t *testing.T) {
	const tokenHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hostID := uuid.New()
	instanceID := uuid.New()
	unitID := uuid.New()
	sessionID := uuid.New()
	trustedInstanceID := instanceID
	trustedUnitID := unitID
	trustedSessionID := sessionID
	query := &fakeAgentGuardQuery{
		sessions: []model.AgentBehaviorSession{{
			ID: sessionID, HostID: hostID, InstanceID: instanceID, ExecutionUnitID: &unitID,
			ExternalSessionID: "external-secret", Source: model.AgentGuardSessionSourceAdapterHook,
			Confidence: "confirmed", CorrelationTokenHash: tokenHash,
		}},
		units: []model.AgentExecutionUnit{{
			ID: unitID, HostID: hostID, InstanceID: instanceID, UnitType: "local_process_tree",
		}},
		behaviors: []model.AgentBehaviorEvent{
			{
				RawEventID: "trusted-tool", HostID: hostID, InstanceID: &trustedInstanceID,
				ExecutionUnitID: &trustedUnitID, SessionID: &trustedSessionID,
				Category: "tool", Operation: "tool_call_started", ProcessName: "must-not-be-tool-label",
				ResourceType: "tool", ResourceIdentity: "filesystem.read", CorrelationID: tokenHash,
				Decision: "audit", Severity: "info", OccurredAt: time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC),
				Collection: []byte(`{"source":"adapter_hook"}`),
				Evidence:   []byte(`{"correlation_token_hash":"` + tokenHash + `","trusted_proof":{"verified":true,"verifier":"ed25519","proof_digest":"` + tokenHash + `","issued_at":"2026-08-03T10:00:00Z"}}`),
			},
			{
				RawEventID: "untrusted-tool", HostID: hostID, InstanceID: &trustedInstanceID,
				ExecutionUnitID: &trustedUnitID, SessionID: &trustedSessionID,
				Category: "tool", Operation: "tool_call_completed", ProcessName: "fake-tool-from-process",
				ResourceType: "tool", ResourceIdentity: "forged.tool",
				Collection: []byte(`{"source":"ebpf"}`),
				Evidence:   []byte(`{"trusted_proof":{"verified":true}}`),
			},
		},
	}
	signer := testAgentGuardSigner(t)
	engine := newAgentGuardHandlerTestEngine(t, query, signer)
	nodeID, err := signer.SignPanoramaNode(service.AgentGuardPanoramaNodeRef{
		NodeType: "execution_unit", ObjectID: unitID.String(), HostID: hostID.String(),
		InstanceID: instanceID.String(), SessionID: sessionID.String(),
	}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	response := serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/panorama/nodes/"+nodeID+"/children?page=1&page_size=20",
		nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			Items []struct {
				ID         string                                `json:"id"`
				NodeType   string                                `json:"node_type"`
				Label      string                                `json:"label"`
				Trust      *service.AgentGuardPanoramaTrust      `json:"trust"`
				Collection *service.AgentGuardPanoramaCollection `json:"collection"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Items) != 2 {
		t.Fatalf("items=%d body=%s", len(body.Data.Items), response.Body.String())
	}
	trusted := body.Data.Items[0]
	if trusted.NodeType != "tool_call" || trusted.Label != "filesystem.read" || trusted.Trust == nil ||
		trusted.Trust.ToolSemantics != service.AgentGuardToolSemanticsTrusted ||
		trusted.Trust.Source != model.AgentGuardSessionSourceAdapterHook ||
		!trusted.Trust.ProofVerified || trusted.Trust.Correlation != "matched" || trusted.Collection != nil {
		t.Fatalf("unexpected trusted tool projection: %#v", trusted)
	}
	untrusted := body.Data.Items[1]
	if untrusted.NodeType != "tool_call" || untrusted.Label != "tool call completed" || untrusted.Trust == nil ||
		untrusted.Trust.ToolSemantics != service.AgentGuardToolSemanticsUnobservable ||
		untrusted.Trust.Source != "" || untrusted.Trust.ProofVerified || untrusted.Collection == nil {
		t.Fatalf("unexpected untrusted tool projection: %#v", untrusted)
	}
	for _, secret := range []string{tokenHash, "proof_digest", "ed25519", "external-secret", "fake-tool-from-process", "must-not-be-tool-label"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("Panorama leaked %q: %s", secret, response.Body.String())
		}
	}

	trustedChildren := serveAgentGuardRequest(
		engine, http.MethodGet,
		"/api/v1/agent-guard/panorama/nodes/"+trusted.ID+"/children?page=1&page_size=20", nil,
	)
	if trustedChildren.Code != http.StatusOK {
		t.Fatalf("trusted children status=%d body=%s", trustedChildren.Code, trustedChildren.Body.String())
	}
	if !strings.Contains(trustedChildren.Body.String(), `"label":"filesystem.read"`) ||
		!strings.Contains(trustedChildren.Body.String(), `"tool_semantics":"trusted"`) {
		t.Fatalf("trusted tool child lost safe semantics: %s", trustedChildren.Body.String())
	}

	untrustedChildren := serveAgentGuardRequest(
		engine, http.MethodGet,
		"/api/v1/agent-guard/panorama/nodes/"+untrusted.ID+"/children?page=1&page_size=20", nil,
	)
	if untrustedChildren.Code != http.StatusOK {
		t.Fatalf("untrusted children status=%d body=%s", untrustedChildren.Code, untrustedChildren.Body.String())
	}
	if !strings.Contains(untrustedChildren.Body.String(), `"label":"tool_semantics_unobservable"`) ||
		!strings.Contains(untrustedChildren.Body.String(), `"tool_semantics":"tool_semantics_unobservable"`) ||
		strings.Contains(untrustedChildren.Body.String(), "forged.tool") {
		t.Fatalf("untrusted tool child exposed inferred semantics: %s", untrustedChildren.Body.String())
	}
}

func TestAgentGuardPanoramaMarksRemoteWithoutSensorUnobservable(t *testing.T) {
	hostID := uuid.New()
	instanceID := uuid.New()
	unitID := uuid.New()
	sessionID := uuid.New()
	query := &fakeAgentGuardQuery{
		sessions: []model.AgentBehaviorSession{{
			ID: sessionID, HostID: hostID, InstanceID: instanceID, ExecutionUnitID: &unitID,
		}},
		units: []model.AgentExecutionUnit{{
			ID: unitID, HostID: hostID, InstanceID: instanceID, UnitType: "remote_sandbox",
			RemoteBackend: "ssh", RemoteExecutionID: "remote-secret", RemoteHostRef: "host-secret",
			CoverageLevel: model.AgentGuardCoverageRemoteUnobservable,
		}},
	}
	signer := testAgentGuardSigner(t)
	engine := newAgentGuardHandlerTestEngine(t, query, signer)
	nodeID, err := signer.SignPanoramaNode(service.AgentGuardPanoramaNodeRef{
		NodeType: "session", ObjectID: sessionID.String(), HostID: hostID.String(),
		InstanceID: instanceID.String(), SessionID: sessionID.String(),
	}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	response := serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/panorama/nodes/"+nodeID+"/children?page=1&page_size=20",
		nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"remote_visibility":"remote_unobservable"`) ||
		!strings.Contains(response.Body.String(), `"limitations":["remote_unobservable"]`) {
		t.Fatalf("remote limitation is missing: %s", response.Body.String())
	}
	for _, secret := range []string{"remote-secret", "host-secret"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("Panorama leaked %q: %s", secret, response.Body.String())
		}
	}
}

func TestAgentGuardStrictInputsReturnStableErrorCode(t *testing.T) {
	engine := newAgentGuardHandlerTestEngine(t, &fakeAgentGuardQuery{}, testAgentGuardSigner(t))

	response := serveAgentGuardRequest(
		engine,
		http.MethodGet,
		"/api/v1/agent-guard/rules/AGB-BUILTIN-001/versions?page=abc",
		nil,
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid page status=%d body=%s", response.Code, response.Body.String())
	}
	assertAgentGuardErrorCode(t, response, "agent_guard_request_invalid")

	response = serveAgentGuardRequest(
		engine,
		http.MethodPost,
		"/api/v1/agent-guard/policies",
		[]byte(`{} {}`),
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON status=%d body=%s", response.Code, response.Body.String())
	}
	assertAgentGuardErrorCode(t, response, "agent_guard_request_invalid")
}

func newAgentGuardHandlerTestEngine(
	t *testing.T,
	query *fakeAgentGuardQuery,
	signer *service.AgentGuardScopeSigner,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := NewAgentGuardHandler(
		fakeAgentGuardCatalog{},
		fakeAgentGuardPolicies{},
		query,
		nil,
		signer,
		nil,
	)
	engine := gin.New()
	pass := func(c *gin.Context) { c.Next() }
	handler.RegisterRoutes(engine.Group("/api/v1"), pass, pass, pass, pass, pass, pass, pass, pass, pass, pass)
	return engine
}

func testAgentGuardSigner(t *testing.T) *service.AgentGuardScopeSigner {
	t.Helper()
	signer, err := service.NewAgentGuardScopeSigner("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func serveAgentGuardRequest(engine http.Handler, method string, path string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func assertAgentGuardErrorCode(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body struct {
		Code      int    `json:"code"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code == 0 || body.ErrorCode != want {
		t.Fatalf("code=%d error_code=%q, want nonzero/%q body=%s", body.Code, body.ErrorCode, want, response.Body.String())
	}
}
