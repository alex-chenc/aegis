package agentguard

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const (
	ToolSourceAgentOfficial = "agent_official"
	ToolSourceAdapterHook   = "adapter_hook"
	ToolSourceAegisWrapper  = "aegis_wrapper"
	ToolSourceRemoteSensor  = "aegis_remote_sensor"

	ToolEventStarted   = "tool_call_started"
	ToolEventCompleted = "tool_call_completed"
	ToolEventFailed    = "tool_call_failed"
	toolManifestSchema = "aegis.agent_guard.tool_sources.v1"
)

var safeToolName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,127}$`)

type TrustedToolSourceManifest struct {
	Schema  string                   `json:"schema"`
	Sources []TrustedToolSource      `json:"sources"`
	Socket  *TrustedToolSocketPolicy `json:"socket,omitempty"`
	Digest  string                   `json:"digest"`
}

type TrustedToolSocketPolicy struct {
	Mode    string  `json:"mode,omitempty"`
	GroupID *uint32 `json:"group_id,omitempty"`
}

type ToolSocketRuntimePolicy struct {
	Mode    os.FileMode
	GroupID *uint32
}

type TrustedToolSource struct {
	SourceID       string   `json:"source_id"`
	SourceType     string   `json:"source_type"`
	Product        string   `json:"product"`
	Version        string   `json:"version"`
	Verifier       string   `json:"verifier"`
	PublicKey      string   `json:"public_key,omitempty"`
	ArtifactPath   string   `json:"artifact_path,omitempty"`
	ArtifactDigest string   `json:"artifact_digest,omitempty"`
	AllowedUIDs    []uint32 `json:"allowed_uids"`
	AllowedGIDs    []uint32 `json:"allowed_gids,omitempty"`
}

type TrustedToolEvent struct {
	EventID          string                `json:"event_id"`
	SourceID         string                `json:"source_id"`
	SourceVersion    string                `json:"source_version"`
	Operation        string                `json:"operation"`
	ToolName         string                `json:"tool_name"`
	ToolCallID       string                `json:"tool_call_id"`
	CorrelationToken string                `json:"correlation_token"`
	PID              uint32                `json:"pid"`
	StartTicks       uint64                `json:"start_ticks"`
	ProcessEventID   string                `json:"process_event_id,omitempty"`
	ResourceEventIDs []string              `json:"resource_event_ids,omitempty"`
	OccurredAt       time.Time             `json:"occurred_at"`
	IssuedAt         time.Time             `json:"issued_at"`
	Proof            string                `json:"proof,omitempty"`
	Remote           *RemoteSensorEvidence `json:"remote,omitempty"`
}

type RemoteSensorEvidence struct {
	SourceID              string    `json:"source_id"`
	SourceVersion         string    `json:"source_version"`
	EventID               string    `json:"event_id"`
	CorrelationTokenHash  string    `json:"correlation_token_hash"`
	RemoteHostID          string    `json:"remote_host_id"`
	RemoteExecutionUnitID string    `json:"remote_execution_unit_id"`
	IssuedAt              time.Time `json:"issued_at"`
	Proof                 string    `json:"proof,omitempty"`
}

type TrustedProof struct {
	Verified    bool      `json:"verified"`
	Verifier    string    `json:"verifier"`
	ProofDigest string    `json:"proof_digest"`
	IssuedAt    time.Time `json:"issued_at"`
}

type verifiedToolEvent struct {
	Source          TrustedToolSource
	CorrelationHash string
	Proof           TrustedProof
	Remote          *RemoteSensorEvidence
}

type toolCorrelationLink struct {
	CorrelationHash string
	ToolEventID     string
	ToolCallID      string
	SessionID       string
	ExpiresAt       time.Time
}

type TrustedToolAdapter struct {
	sources map[string]TrustedToolSource
	mu      sync.Mutex
	seen    map[string]time.Time
	links   map[ProcessIdentity]toolCorrelationLink
	now     func() time.Time
	socket  ToolSocketRuntimePolicy
}

func LoadTrustedToolAdapter(manifestPath string) (*TrustedToolAdapter, error) {
	manifestPath = filepath.Clean(strings.TrimSpace(manifestPath))
	if manifestPath == "." || !filepath.IsAbs(manifestPath) {
		return nil, errors.New("agent_guard_tool_manifest_path_invalid")
	}
	info, err := os.Lstat(manifestPath)
	if err != nil || !trustedOwnedRegularFile(info, 1<<20) {
		return nil, errors.New("agent_guard_tool_manifest_untrusted")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("agent_guard_tool_manifest_read_failed: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest TrustedToolSourceManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, errors.New("agent_guard_tool_manifest_invalid")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("agent_guard_tool_manifest_invalid")
	}
	if manifest.Schema != toolManifestSchema || len(manifest.Sources) == 0 {
		return nil, errors.New("agent_guard_tool_manifest_invalid")
	}
	digest, err := toolManifestDigest(manifest)
	if err != nil || digest != manifest.Digest {
		return nil, errors.New("agent_guard_tool_manifest_digest_mismatch")
	}
	adapter := &TrustedToolAdapter{
		sources: make(map[string]TrustedToolSource), seen: make(map[string]time.Time),
		links: make(map[ProcessIdentity]toolCorrelationLink), now: time.Now,
	}
	adapter.socket, err = normalizeToolSocketPolicy(manifest.Socket)
	if err != nil {
		return nil, err
	}
	for _, source := range manifest.Sources {
		if err := validateTrustedToolSource(source); err != nil {
			return nil, err
		}
		if _, exists := adapter.sources[source.SourceID]; exists {
			return nil, errors.New("agent_guard_tool_source_duplicate")
		}
		if source.ArtifactPath != "" {
			actual, err := digestRegularArtifact(source.ArtifactPath)
			if err != nil || actual != source.ArtifactDigest {
				return nil, errors.New("agent_guard_tool_source_artifact_mismatch")
			}
		}
		adapter.sources[source.SourceID] = source
	}
	return adapter, nil
}

func toolManifestDigest(manifest TrustedToolSourceManifest) (string, error) {
	manifest.Digest = ""
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validateTrustedToolSource(source TrustedToolSource) error {
	if !safeToolName.MatchString(source.SourceID) || !safeToolName.MatchString(source.Product) ||
		strings.TrimSpace(source.Version) == "" {
		return errors.New("agent_guard_tool_source_identity_invalid")
	}
	switch source.SourceType {
	case ToolSourceAgentOfficial, ToolSourceAdapterHook, ToolSourceAegisWrapper, ToolSourceRemoteSensor:
	default:
		return errors.New("agent_guard_tool_source_type_invalid")
	}
	if source.Verifier != "ed25519" {
		return errors.New("agent_guard_tool_source_verifier_invalid")
	}
	if len(source.AllowedUIDs) == 0 || len(source.AllowedUIDs) > 64 || len(source.AllowedGIDs) > 64 {
		return errors.New("agent_guard_tool_source_peer_ids_invalid")
	}
	key, err := base64.StdEncoding.DecodeString(source.PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("agent_guard_tool_source_public_key_invalid")
	}
	if (source.ArtifactPath == "") != (source.ArtifactDigest == "") ||
		(source.ArtifactPath != "" && (!filepath.IsAbs(source.ArtifactPath) || !validSHA256(source.ArtifactDigest))) {
		return errors.New("agent_guard_tool_source_artifact_invalid")
	}
	return nil
}

func normalizeToolSocketPolicy(policy *TrustedToolSocketPolicy) (ToolSocketRuntimePolicy, error) {
	if policy == nil || policy.Mode == "" {
		return ToolSocketRuntimePolicy{Mode: 0o600}, nil
	}
	switch policy.Mode {
	case "0600":
		return ToolSocketRuntimePolicy{Mode: 0o600, GroupID: policy.GroupID}, nil
	case "0660":
		if policy.GroupID == nil {
			return ToolSocketRuntimePolicy{}, errors.New("agent_guard_tool_socket_group_missing")
		}
		return ToolSocketRuntimePolicy{Mode: 0o660, GroupID: policy.GroupID}, nil
	default:
		return ToolSocketRuntimePolicy{}, errors.New("agent_guard_tool_socket_mode_invalid")
	}
}

func (a *TrustedToolAdapter) SocketPolicy() ToolSocketRuntimePolicy {
	if a == nil || a.socket.Mode == 0 {
		return ToolSocketRuntimePolicy{Mode: 0o600}
	}
	return a.socket
}

// AuthorizePeer is only the local credential gate. Verify must still validate
// the event signature for the selected source; a correlation token or UID is
// never sufficient authentication by itself.
func (a *TrustedToolAdapter) AuthorizePeer(sourceID string, uid, gid uint32) bool {
	if a == nil {
		return false
	}
	source, ok := a.sources[sourceID]
	if !ok || source.SourceType == ToolSourceRemoteSensor {
		return false
	}
	for _, allowed := range source.AllowedUIDs {
		if allowed == uid {
			return true
		}
	}
	for _, allowed := range source.AllowedGIDs {
		if allowed == gid {
			return true
		}
	}
	return false
}

func digestRegularArtifact(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !trustedOwnedRegularFile(info, 128<<20) {
		return "", errors.New("untrusted artifact")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func (a *TrustedToolAdapter) Verify(event TrustedToolEvent) (verifiedToolEvent, error) {
	if a == nil {
		return verifiedToolEvent{}, errors.New("tool_semantics_unobservable")
	}
	source, ok := a.sources[event.SourceID]
	if !ok || source.SourceVersionMismatch(event.SourceVersion) || source.SourceType == ToolSourceRemoteSensor {
		return verifiedToolEvent{}, errors.New("agent_guard_tool_source_untrusted")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return verifiedToolEvent{}, errors.New("agent_guard_tool_event_id_invalid")
	}
	if _, err := uuid.Parse(event.ToolCallID); err != nil || !safeToolName.MatchString(event.ToolName) {
		return verifiedToolEvent{}, errors.New("agent_guard_tool_event_invalid")
	}
	if !validCorrelationToken(event.CorrelationToken) || event.PID == 0 || event.StartTicks == 0 {
		return verifiedToolEvent{}, errors.New("agent_guard_tool_correlation_invalid")
	}
	switch event.Operation {
	case ToolEventStarted, ToolEventCompleted, ToolEventFailed:
	default:
		return verifiedToolEvent{}, errors.New("agent_guard_tool_operation_invalid")
	}
	if err := validateEvidenceIDs(event.ProcessEventID, event.ResourceEventIDs); err != nil {
		return verifiedToolEvent{}, err
	}
	if !validIssuedTime(a.now(), event.OccurredAt, event.IssuedAt) {
		return verifiedToolEvent{}, errors.New("agent_guard_tool_event_time_invalid")
	}
	proof, err := verifyToolProof(source, event)
	if err != nil {
		return verifiedToolEvent{}, err
	}
	correlationHash := hashCorrelationToken(event.CorrelationToken)
	var remote *RemoteSensorEvidence
	if event.Remote != nil {
		if event.Remote.CorrelationTokenHash != correlationHash {
			return verifiedToolEvent{}, errors.New("agent_guard_remote_correlation_mismatch")
		}
		verified, err := a.verifyRemote(*event.Remote)
		if err != nil {
			return verifiedToolEvent{}, err
		}
		remote = &verified
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.seen[event.EventID]; exists {
		return verifiedToolEvent{}, errors.New("agent_guard_tool_event_replayed")
	}
	a.seen[event.EventID] = a.now().UTC()
	for id, seenAt := range a.seen {
		if a.now().Sub(seenAt) > 24*time.Hour {
			delete(a.seen, id)
		}
	}
	return verifiedToolEvent{Source: source, CorrelationHash: correlationHash, Proof: proof, Remote: remote}, nil
}

func (s TrustedToolSource) SourceVersionMismatch(version string) bool {
	return strings.TrimSpace(version) == "" || version != s.Version
}

func verifyToolProof(source TrustedToolSource, event TrustedToolEvent) (TrustedProof, error) {
	signed := event
	signed.Proof = ""
	signed.Remote = nil
	data, err := json.Marshal(signed)
	if err != nil {
		return TrustedProof{}, err
	}
	publicKey, _ := base64.StdEncoding.DecodeString(source.PublicKey)
	signature, err := base64.StdEncoding.DecodeString(event.Proof)
	if err != nil || !ed25519.Verify(publicKey, data, signature) {
		return TrustedProof{}, errors.New("agent_guard_tool_event_signature_invalid")
	}
	sum := sha256.Sum256(signature)
	proofDigest := "sha256:" + hex.EncodeToString(sum[:])
	return TrustedProof{Verified: true, Verifier: source.Verifier, ProofDigest: proofDigest, IssuedAt: event.IssuedAt.UTC()}, nil
}

func (a *TrustedToolAdapter) verifyRemote(evidence RemoteSensorEvidence) (RemoteSensorEvidence, error) {
	source, ok := a.sources[evidence.SourceID]
	if !ok || source.SourceType != ToolSourceRemoteSensor || source.SourceVersionMismatch(evidence.SourceVersion) {
		return RemoteSensorEvidence{}, errors.New("agent_guard_remote_sensor_untrusted")
	}
	if _, err := uuid.Parse(evidence.EventID); err != nil ||
		!validUUID(evidence.RemoteHostID) || !validUUID(evidence.RemoteExecutionUnitID) ||
		!validSHA256(evidence.CorrelationTokenHash) || evidence.IssuedAt.IsZero() {
		return RemoteSensorEvidence{}, errors.New("agent_guard_remote_evidence_invalid")
	}
	signed := evidence
	signed.Proof = ""
	data, _ := json.Marshal(signed)
	publicKey, _ := base64.StdEncoding.DecodeString(source.PublicKey)
	signature, err := base64.StdEncoding.DecodeString(evidence.Proof)
	if err != nil || !ed25519.Verify(publicKey, data, signature) {
		return RemoteSensorEvidence{}, errors.New("agent_guard_remote_evidence_signature_invalid")
	}
	return evidence, nil
}

func trustedOwnedRegularFile(info os.FileInfo, maxSize int64) bool {
	if info == nil || !info.Mode().IsRegular() || info.Size() > maxSize ||
		info.Mode().Perm()&0o022 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func (a *TrustedToolAdapter) Bind(identity ProcessIdentity, link toolCorrelationLink) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.links[identity] = link
}

func (a *TrustedToolAdapter) Lookup(identity ProcessIdentity) (toolCorrelationLink, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	link, ok := a.links[identity]
	if !ok || a.now().After(link.ExpiresAt) {
		delete(a.links, identity)
		return toolCorrelationLink{}, false
	}
	return link, true
}

func (a *TrustedToolAdapter) OnFork(parent, child ProcessIdentity) {
	if !parent.Valid() || !child.Valid() {
		return
	}
	if link, ok := a.Lookup(parent); ok {
		a.Bind(child, link)
	}
}

func (a *TrustedToolAdapter) Complete(identity ProcessIdentity) {
	a.mu.Lock()
	delete(a.links, identity)
	a.mu.Unlock()
}

func validCorrelationToken(value string) bool {
	if len(value) < 32 || len(value) > 512 {
		return false
	}
	for _, char := range value {
		if char <= ' ' || char > '~' {
			return false
		}
	}
	return true
}

func hashCorrelationToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validSHA256(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil
}

func validateEvidenceIDs(processEventID string, resourceEventIDs []string) error {
	if processEventID != "" && !validUUID(processEventID) {
		return errors.New("agent_guard_tool_process_event_invalid")
	}
	if len(resourceEventIDs) > 64 {
		return errors.New("agent_guard_tool_resource_events_invalid")
	}
	for _, id := range resourceEventIDs {
		if !validUUID(id) {
			return errors.New("agent_guard_tool_resource_events_invalid")
		}
	}
	return nil
}

func validIssuedTime(now, occurredAt, issuedAt time.Time) bool {
	if occurredAt.IsZero() || issuedAt.IsZero() {
		return false
	}
	delta := occurredAt.Sub(issuedAt)
	if delta < 0 {
		delta = -delta
	}
	return delta <= 5*time.Minute && !issuedAt.After(now.Add(5*time.Minute)) &&
		!issuedAt.Before(now.Add(-24*time.Hour))
}
