package agentguard

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
)

type toolAdapterFixture struct {
	adapter       *TrustedToolAdapter
	toolPrivate   ed25519.PrivateKey
	remotePrivate ed25519.PrivateKey
	token         string
}

func newToolAdapterFixture(t *testing.T) toolAdapterFixture {
	return newToolAdapterFixtureWithSocket(t, nil)
}

func newToolAdapterFixtureWithSocket(t *testing.T, socket *TrustedToolSocketPolicy) toolAdapterFixture {
	t.Helper()
	toolPublic, toolPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remotePublic, remotePrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := TrustedToolSourceManifest{
		Schema: toolManifestSchema, Socket: socket,
		Sources: []TrustedToolSource{
			{
				SourceID: "claude-official-hook", SourceType: ToolSourceAgentOfficial,
				Product: "claude-code", Version: "1.0.0", Verifier: "ed25519",
				PublicKey:   base64.StdEncoding.EncodeToString(toolPublic),
				AllowedUIDs: []uint32{uint32(os.Geteuid())},
			},
			{
				SourceID: "remote-aegis-sensor", SourceType: ToolSourceRemoteSensor,
				Product: "aegis-agent", Version: "1.0.0", Verifier: "ed25519",
				PublicKey:   base64.StdEncoding.EncodeToString(remotePublic),
				AllowedUIDs: []uint32{uint32(os.Geteuid())},
			},
		},
	}
	manifest.Digest, err = toolManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tool-sources.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadTrustedToolAdapter(path)
	if err != nil {
		t.Fatalf("load trusted tool adapter: %v", err)
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		t.Fatal(err)
	}
	return toolAdapterFixture{
		adapter: adapter, toolPrivate: toolPrivate, remotePrivate: remotePrivate,
		token: base64.RawURLEncoding.EncodeToString(tokenBytes),
	}
}

func TestToolHookExplicitGroupSocketAndPinnedPeerCredentials(t *testing.T) {
	groupID := uint32(os.Getegid())
	fixture := newToolAdapterFixtureWithSocket(t, &TrustedToolSocketPolicy{
		Mode: "0660", GroupID: &groupID,
	})
	policy := fixture.adapter.SocketPolicy()
	if policy.Mode != 0o660 || policy.GroupID == nil || *policy.GroupID != groupID {
		t.Fatalf("explicit group socket policy lost: %#v", policy)
	}
	if !fixture.adapter.AuthorizePeer("claude-official-hook", uint32(os.Geteuid()), groupID) ||
		fixture.adapter.AuthorizePeer("claude-official-hook", ^uint32(0), ^uint32(0)) ||
		fixture.adapter.AuthorizePeer("unconfigured-source", uint32(os.Geteuid()), groupID) {
		t.Fatal("source-specific peer credential authorization failed closed")
	}

	source := fixture.adapter.sources["claude-official-hook"]
	source.AllowedUIDs = []uint32{^uint32(0)}
	source.AllowedGIDs = []uint32{groupID}
	fixture.adapter.sources[source.SourceID] = source
	if !fixture.adapter.AuthorizePeer(source.SourceID, uint32(os.Geteuid()), groupID) ||
		fixture.adapter.AuthorizePeer(source.SourceID, uint32(os.Geteuid()), groupID+1) {
		t.Fatal("optional pinned group authorization failed closed")
	}

	socketPath := filepath.Join(t.TempDir(), "group-hook.sock")
	receiver, err := StartToolHookReceiver(
		socketPath, policy, fixture.adapter.AuthorizePeer,
		func([]byte) (BehaviorEvent, error) { return BehaviorEvent{}, nil },
	)
	if err != nil {
		t.Fatalf("start explicit group socket: %v", err)
	}
	defer receiver.Stop()
	info, err := os.Lstat(socketPath)
	if err != nil || info.Mode().Perm() != 0o660 {
		t.Fatalf("group socket mode mismatch: info=%v err=%v", info, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Gid != groupID {
		t.Fatalf("group socket ownership mismatch: %#v", stat)
	}
}

func (f toolAdapterFixture) event(process ProcessSnapshot, operation string) TrustedToolEvent {
	now := time.Now().UTC().Truncate(time.Millisecond)
	event := TrustedToolEvent{
		EventID: uuid.NewString(), SourceID: "claude-official-hook", SourceVersion: "1.0.0",
		Operation: operation, ToolName: "shell.exec", ToolCallID: uuid.NewString(),
		CorrelationToken: f.token, PID: process.Identity.PID, StartTicks: process.Identity.StartTicks,
		ProcessEventID: uuid.NewString(), ResourceEventIDs: []string{uuid.NewString()},
		OccurredAt: now, IssuedAt: now,
	}
	signTrustedToolEvent(&event, f.toolPrivate)
	return event
}

func signTrustedToolEvent(event *TrustedToolEvent, privateKey ed25519.PrivateKey) {
	signed := *event
	signed.Proof = ""
	signed.Remote = nil
	data, _ := json.Marshal(signed)
	event.Proof = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, data))
}

func (f toolAdapterFixture) sessionEvent(process ProcessSnapshot, operation, externalSessionID string) TrustedSessionEvent {
	now := time.Now().UTC().Truncate(time.Millisecond)
	event := TrustedSessionEvent{
		EventID: uuid.NewString(), SourceID: "claude-official-hook", SourceVersion: "1.0.0",
		Operation: operation, ExternalSessionID: externalSessionID,
		PID: process.Identity.PID, StartTicks: process.Identity.StartTicks,
		LifecycleReason: "startup", OccurredAt: now, IssuedAt: now,
	}
	SignTrustedSessionEvent(&event, f.toolPrivate)
	return event
}

func signRemoteEvidence(evidence *RemoteSensorEvidence, privateKey ed25519.PrivateKey) {
	signed := *evidence
	signed.Proof = ""
	data, _ := json.Marshal(signed)
	evidence.Proof = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, data))
}

func TestTrustedToolAdapterRequiresSignedConfiguredProofAndVerifiedRemoteEvidence(t *testing.T) {
	fixture := newToolAdapterFixture(t)
	process := confirmedProcess(4100, 100, "/opt/claude/bin/claude", "claude")

	valid := fixture.event(process, ToolEventStarted)
	verified, err := fixture.adapter.Verify(valid)
	if err != nil {
		t.Fatalf("verify signed official hook: %v", err)
	}
	if verified.Source.SourceType != ToolSourceAgentOfficial ||
		verified.CorrelationHash != hashCorrelationToken(fixture.token) ||
		!verified.Proof.Verified || verified.Proof.Verifier != "ed25519" ||
		!validSHA256(verified.Proof.ProofDigest) {
		t.Fatalf("unexpected verified result: %#v", verified)
	}
	encoded, _ := json.Marshal(verified)
	if strings.Contains(string(encoded), fixture.token) {
		t.Fatalf("raw correlation token survived verification: %s", encoded)
	}
	if _, err := fixture.adapter.Verify(valid); err == nil || !strings.Contains(err.Error(), "replayed") {
		t.Fatalf("replay was not rejected: %v", err)
	}

	unsigned := fixture.event(process, ToolEventStarted)
	unsigned.Proof = ""
	if _, err := fixture.adapter.Verify(unsigned); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("unsigned same-UID payload was not rejected: %v", err)
	}
	tampered := fixture.event(process, ToolEventStarted)
	tampered.ToolName = "filesystem.write"
	if _, err := fixture.adapter.Verify(tampered); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("tampered tool payload was not rejected: %v", err)
	}
	spoofed := fixture.event(process, ToolEventStarted)
	spoofed.SourceID = "process-name-inferred"
	if _, err := fixture.adapter.Verify(spoofed); err == nil || !strings.Contains(err.Error(), "source_untrusted") {
		t.Fatalf("unconfigured source was not rejected: %v", err)
	}

	withoutRemote := fixture.event(process, ToolEventCompleted)
	verified, err = fixture.adapter.Verify(withoutRemote)
	if err != nil || verified.Remote != nil {
		t.Fatalf("local event should remain remote-unobservable: verified=%#v err=%v", verified, err)
	}

	withUnsignedRemote := fixture.event(process, ToolEventCompleted)
	withUnsignedRemote.Remote = &RemoteSensorEvidence{
		SourceID: "remote-aegis-sensor", SourceVersion: "1.0.0", EventID: uuid.NewString(),
		CorrelationTokenHash: hashCorrelationToken(fixture.token), RemoteHostID: uuid.NewString(),
		RemoteExecutionUnitID: uuid.NewString(), IssuedAt: withUnsignedRemote.IssuedAt,
	}
	if _, err := fixture.adapter.Verify(withUnsignedRemote); err == nil || !strings.Contains(err.Error(), "remote_evidence_signature") {
		t.Fatalf("unsigned remote evidence was not rejected: %v", err)
	}

	withRemote := fixture.event(process, ToolEventCompleted)
	withRemote.Remote = &RemoteSensorEvidence{
		SourceID: "remote-aegis-sensor", SourceVersion: "1.0.0", EventID: uuid.NewString(),
		CorrelationTokenHash: hashCorrelationToken(fixture.token), RemoteHostID: uuid.NewString(),
		RemoteExecutionUnitID: uuid.NewString(), IssuedAt: withRemote.IssuedAt,
	}
	signRemoteEvidence(withRemote.Remote, fixture.remotePrivate)
	verified, err = fixture.adapter.Verify(withRemote)
	if err != nil || verified.Remote == nil || verified.Remote.EventID != withRemote.Remote.EventID {
		t.Fatalf("signed remote sensor evidence not preserved: verified=%#v err=%v", verified, err)
	}
	invalidRemoteID := fixture.event(process, ToolEventCompleted)
	invalidRemoteID.Remote = &RemoteSensorEvidence{
		SourceID: "remote-aegis-sensor", SourceVersion: "1.0.0", EventID: "not-a-uuid",
		CorrelationTokenHash: hashCorrelationToken(fixture.token), RemoteHostID: uuid.NewString(),
		RemoteExecutionUnitID: uuid.NewString(), IssuedAt: invalidRemoteID.IssuedAt,
	}
	signRemoteEvidence(invalidRemoteID.Remote, fixture.remotePrivate)
	if _, err := fixture.adapter.Verify(invalidRemoteID); err == nil || !strings.Contains(err.Error(), "remote_evidence_invalid") {
		t.Fatalf("invalid remote event ID was not rejected: %v", err)
	}
}

func TestTrustedToolExternalSessionIdentityIsSignedAndValidated(t *testing.T) {
	fixture := newToolAdapterFixture(t)
	process := confirmedProcess(4100, 100, "/opt/claude/bin/claude", "claude")
	event := fixture.event(process, ToolEventStarted)
	event.ExternalSessionID = "thr_01JABCDEFGH1234567890"
	signTrustedToolEvent(&event, fixture.toolPrivate)
	if _, err := fixture.adapter.Verify(event); err != nil {
		t.Fatalf("verified external session rejected: %v", err)
	}

	tampered := fixture.event(process, ToolEventStarted)
	tampered.ExternalSessionID = "thr_original"
	signTrustedToolEvent(&tampered, fixture.toolPrivate)
	tampered.ExternalSessionID = "thr_tampered"
	if _, err := fixture.adapter.Verify(tampered); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("unsigned external session mutation accepted: %v", err)
	}

	invalid := fixture.event(process, ToolEventStarted)
	invalid.ExternalSessionID = strings.Repeat("x", 256)
	signTrustedToolEvent(&invalid, fixture.toolPrivate)
	if _, err := fixture.adapter.Verify(invalid); err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("oversized external session accepted: %v", err)
	}
}

func TestTrustedCodexSessionLifecycleIsSignedAndBoundToRootProcess(t *testing.T) {
	fixture := newToolAdapterFixture(t)
	root := confirmedProcess(4100, 100, "/opt/codex/bin/codex", "codex", "app-server")
	start := fixture.sessionEvent(root, SessionEventStarted, "thr_real_123")
	verified, err := fixture.adapter.VerifySession(start)
	if err != nil || verified.Source.SourceType != ToolSourceAgentOfficial {
		t.Fatalf("verify session start: verified=%#v err=%v", verified, err)
	}

	tampered := fixture.sessionEvent(root, SessionEventEnded, "thr_real_123")
	tampered.ExternalSessionID = "thr_other"
	if _, err := fixture.adapter.VerifySession(tampered); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("tampered lifecycle event accepted: %v", err)
	}

	missingID := fixture.sessionEvent(root, SessionEventStarted, "")
	if _, err := fixture.adapter.VerifySession(missingID); err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("lifecycle without session id accepted: %v", err)
	}
}

func TestManagerTrustedSessionStartMountsForkTreeAndEndClosesOnlySession(t *testing.T) {
	fixture := newToolAdapterFixture(t)
	root := confirmedProcess(4100, 100, "/opt/codex/bin/codex", "codex", "app-server")
	root.ConfigEvidence = []string{".codex/config.toml"}
	child := ProcessSnapshot{
		Identity: ProcessIdentity{PID: 4110, StartTicks: 110}, PPID: root.Identity.PID,
		Exe: "/usr/bin/bash", Argv: []string{"bash", "-lc", "printf ok"},
	}
	scanner := &fakeScanner{processes: map[uint32]ProcessSnapshot{root.Identity.PID: root, child.Identity.PID: child}}
	manager := NewManager(ManagerConfig{
		Enabled: true, BehaviorMonitorEnabled: true, SessionHookEnabled: true,
		ToolAdapter: fixture.adapter, HostID: "host-1", StateDir: t.TempDir(),
		ReconcileInterval: time.Hour, FlushInterval: time.Hour, SpoolCapacity: 64,
	}, scanner, nil)
	manager.sessionEnabled.Store(true)
	manager.tracker.ObserveController(root)

	start := fixture.sessionEvent(root, SessionEventStarted, "thr_real_123")
	payload, _ := json.Marshal(start)
	rootEvent, err := manager.ObserveTrustedSessionPayload(payload)
	if err != nil || rootEvent.Operation != "session_root" || rootEvent.Actor.PID != root.Identity.PID {
		t.Fatalf("observe session start: event=%#v err=%v", rootEvent, err)
	}
	if rootEvent.EventType != "agent_behavior" {
		t.Fatalf("session root must use the existing DC behavior route: %#v", rootEvent)
	}
	if !manager.tracker.OnFork(root.Identity, child) {
		t.Fatal("session root did not mount child")
	}
	childSubject, ok := manager.tracker.LookupProcess(child.Identity)
	if !ok || childSubject.SessionID != rootEvent.SessionID || childSubject.UnitID != rootEvent.ExecutionUnitID {
		t.Fatalf("child scope=%#v root event=%#v", childSubject, rootEvent)
	}

	end := fixture.sessionEvent(root, SessionEventEnded, "thr_real_123")
	end.LifecycleReason = "other"
	SignTrustedSessionEvent(&end, fixture.toolPrivate)
	payload, _ = json.Marshal(end)
	if _, err := manager.ObserveTrustedSessionPayload(payload); err != nil {
		t.Fatalf("observe session end: %v", err)
	}
	for _, session := range manager.tracker.Sessions() {
		if session.ExternalSessionID == "thr_real_123" && session.Status != "ended" {
			t.Fatalf("session not ended: %#v", session)
		}
	}
	for _, instance := range manager.tracker.Instances() {
		if instance.Controller == root.Identity && instance.Status != "running" {
			t.Fatalf("shared runtime stopped by session end: %#v", instance)
		}
	}

	activate := fixture.sessionEvent(root, SessionEventActivated, "thr_real_123")
	payload, _ = json.Marshal(activate)
	resumedRoot, err := manager.ObserveTrustedSessionPayload(payload)
	if err != nil || resumedRoot.Operation != "session_root" || resumedRoot.SessionID != rootEvent.SessionID {
		t.Fatalf("resume ended session from PreToolUse: event=%#v err=%v", resumedRoot, err)
	}
}

func TestToolHookIngressDefaultOffSignedEndToEndAndDisableCleanup(t *testing.T) {
	fixture := newToolAdapterFixture(t)
	controller := confirmedProcess(4100, 100, "/opt/claude/bin/claude", "claude")
	controller.ConfigEvidence = []string{".claude/settings.json"}
	scanner := &fakeScanner{processes: map[uint32]ProcessSnapshot{controller.Identity.PID: controller}}
	reporter := &captureReporter{}
	socketDir := t.TempDir()
	socketPath := filepath.Join(socketDir, "tool-hook.sock")
	manager := NewManager(ManagerConfig{
		Enabled: true, BehaviorMonitorEnabled: true, ToolAdapterEnabled: true,
		ToolHookSocket: socketPath, ToolAdapter: fixture.adapter, HostID: "host-1",
		StateDir: t.TempDir(), ReconcileInterval: time.Hour,
		FlushInterval: 5 * time.Millisecond, SpoolCapacity: 64,
	}, scanner, reporter)
	manager.aggregator = NewAggregator(time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer manager.Stop()
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(socketPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default-off manager created hook socket: %v", err)
	}

	enabled := validBundle(t, 1)
	enabled.Defaults.ToolAdapterEnabled = true
	enabled.Digest, _ = BundleDigest(enabled)
	payload, _ := json.Marshal(enabled)
	if err := manager.ApplyBundle(string(payload)); err != nil {
		t.Fatalf("enable tool adapter bundle: %v", err)
	}
	if info, err := os.Lstat(socketPath); err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("trusted hook socket missing or insecure: info=%v err=%v", info, err)
	}
	baseSubject, ok := manager.tracker.LookupProcess(controller.Identity)
	if !ok {
		t.Fatal("expected controller process attribution")
	}
	session, unit, changed, err := manager.tracker.StartTrustedSession(
		controller, baseSubject, ToolSourceAgentOfficial, "tool-ingress-session", time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("start trusted tool session: %v", err)
	}
	manager.queueTrustedSessionStarted(session, unit, changed)

	unsigned := fixture.event(controller, ToolEventStarted)
	unsigned.ExternalSessionID = "tool-ingress-session"
	unsigned.Proof = ""
	valid := fixture.event(controller, ToolEventStarted)
	valid.ExternalSessionID = "tool-ingress-session"
	signTrustedToolEvent(&valid, fixture.toolPrivate)
	connection, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	unsignedPayload, _ := json.Marshal(unsigned)
	validPayload, _ := json.Marshal(valid)
	if _, err := connection.Write(append(unsignedPayload, '\n')); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Write(append(validPayload, '\n')); err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if link, ok := fixture.adapter.Lookup(controller.Identity); ok && link.ToolCallID == valid.ToolCallID {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("signed ingress was not accepted")
		}
		time.Sleep(5 * time.Millisecond)
	}
	processEvent, accepted := manager.observeRawEvent(RawBehavior{
		EventID: uuid.NewString(), OccurredAt: time.Now().UTC(), Category: CategoryProcess,
		Operation: "exec", Outcome: OutcomeSuccess, Process: controller,
		Resource: Resource{Type: "process", Identity: "/usr/bin/bash"},
		Source:   "ebpf", Sensor: "execve", Visibility: "complete",
	})
	if !accepted || processEvent.Category != CategoryProcess ||
		processEvent.CorrelationID != hashCorrelationToken(fixture.token) {
		t.Fatalf("OS process event was not correlated safely: %#v", processEvent)
	}
	if processEvent.Resource.Identity == valid.ToolName || processEvent.Category == CategoryTool {
		t.Fatalf("process name was incorrectly inferred as tool semantics: %#v", processEvent)
	}
	remote := fixture.event(controller, ToolEventCompleted)
	remote.ExternalSessionID = "tool-ingress-session"
	remote.Remote = &RemoteSensorEvidence{
		SourceID: "remote-aegis-sensor", SourceVersion: "1.0.0", EventID: uuid.NewString(),
		CorrelationTokenHash: hashCorrelationToken(fixture.token), RemoteHostID: uuid.NewString(),
		RemoteExecutionUnitID: uuid.NewString(), IssuedAt: remote.IssuedAt,
	}
	signTrustedToolEvent(&remote, fixture.toolPrivate)
	signRemoteEvidence(remote.Remote, fixture.remotePrivate)
	remotePayload, _ := json.Marshal(remote)
	remoteEvent, err := manager.ObserveTrustedToolPayload(remotePayload)
	if err != nil {
		t.Fatalf("observe signed remote evidence: %v", err)
	}
	if remoteEvent.Evidence["remote_coverage"] != string(CoverageRemoteUnobservable) ||
		stringValue(remoteEvent.Resource.Attributes["remote_sensor_event_id"]) != remote.Remote.EventID {
		t.Fatalf("remote evidence was falsely upgraded or lost: %#v", remoteEvent)
	}

	disabled := validBundle(t, 2)
	disabled.Digest, _ = BundleDigest(disabled)
	payload, _ = json.Marshal(disabled)
	if err := manager.ApplyBundle(string(payload)); err != nil {
		t.Fatalf("disable tool adapter bundle: %v", err)
	}
	if _, err := os.Lstat(socketPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled hook socket was not removed: %v", err)
	}
	postDisable := fixture.event(controller, ToolEventStarted)
	postDisable.ExternalSessionID = "tool-ingress-session"
	postDisablePayload, _ := json.Marshal(postDisable)
	if _, err := manager.ObserveTrustedToolPayload(postDisablePayload); err == nil ||
		!strings.Contains(err.Error(), "tool_semantics_unobservable") {
		t.Fatalf("disabled manager accepted tool event: %v", err)
	}

	time.Sleep(30 * time.Millisecond)
	manager.Stop()
	var toolEvent *BehaviorEvent
	var correlatedProcess *BehaviorEvent
	var sessionUpdate map[string]any
	for _, runtimeEvent := range reporter.snapshot() {
		if strings.Contains(runtimeEvent.EventDataJson, fixture.token) || strings.Contains(runtimeEvent.CommandLine, fixture.token) {
			t.Fatalf("raw correlation token leaked to runtime sink: %#v", runtimeEvent)
		}
		if runtimeEvent.EventType == "agent_behavior_session_updated" {
			var body map[string]any
			if err := json.Unmarshal([]byte(runtimeEvent.EventDataJson), &body); err == nil &&
				body["correlation_token_hash"] == hashCorrelationToken(fixture.token) {
				sessionUpdate = body
			}
		}
		if runtimeEvent.EventType != "agent_tool_call" && runtimeEvent.EventType != "agent_behavior" {
			continue
		}
		var event BehaviorEvent
		if err := json.Unmarshal([]byte(runtimeEvent.EventDataJson), &event); err != nil {
			continue
		}
		if event.EventID == unsigned.EventID {
			t.Fatalf("unsigned same-UID ingress reached runtime sink: %#v", event)
		}
		if event.EventID == valid.EventID {
			copy := event
			toolEvent = &copy
		}
		if event.EventID == processEvent.EventID {
			copy := event
			correlatedProcess = &copy
		}
	}
	if toolEvent == nil || toolEvent.EventType != "agent_behavior" || toolEvent.Category != CategoryTool || toolEvent.Operation != ToolEventStarted ||
		toolEvent.Collection.Source != ToolSourceAgentOfficial || toolEvent.CorrelationID != hashCorrelationToken(fixture.token) {
		t.Fatalf("signed ingress did not reach RuntimeEvent sink: %#v", toolEvent)
	}
	proof, ok := toolEvent.Evidence["trusted_proof"].(map[string]any)
	if !ok || proof["verified"] != true || proof["verifier"] != "ed25519" ||
		!validSHA256(stringValue(proof["proof_digest"])) {
		t.Fatalf("trusted proof contract missing: %#v", toolEvent.Evidence)
	}
	if toolEvent.Evidence["remote_coverage"] != string(CoverageRemoteUnobservable) {
		t.Fatalf("local-only activity claimed remote visibility: %#v", toolEvent.Evidence)
	}
	if correlatedProcess == nil || correlatedProcess.SessionID != toolEvent.SessionID ||
		correlatedProcess.ExecutionUnitID != toolEvent.ExecutionUnitID {
		t.Fatalf("tool and OS events split attribution tree: tool=%#v process=%#v", toolEvent, correlatedProcess)
	}
	if sessionUpdate == nil || stringValue(sessionUpdate["session_id"]) != toolEvent.SessionID ||
		stringValue(sessionUpdate["execution_unit_id"]) != toolEvent.ExecutionUnitID ||
		stringValue(sessionUpdate["source"]) != ToolSourceAgentOfficial ||
		stringValue(sessionUpdate["confidence"]) != string(ConfidenceConfirmed) {
		t.Fatalf("trusted session update cannot satisfy DC linkage: %#v", sessionUpdate)
	}
}
