package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	agentGuardScopeTokenVersion = "ags1"
	agentGuardNodeTokenVersion  = "agn1"
)

var (
	ErrAgentGuardScopeSigningKey = errors.New("agent guard scope signing key must contain at least 32 bytes")
	ErrAgentGuardScopeInvalid    = errors.New("invalid agent guard scope key")
	ErrAgentGuardNodeInvalid     = errors.New("invalid or expired agent guard panorama node")
)

// AgentGuardScope is the minimum identity needed to address a confirmed Agent
// runtime. AssetID binds linked assets; host/type/profile form the stable
// fallback for confirmed runtimes without a static asset. It deliberately
// excludes PID, cmdline, paths, and other evidence.
type AgentGuardScope struct {
	HostID     string `json:"host_id"`
	AgentType  string `json:"agent_type"`
	ProfileKey string `json:"profile_key,omitempty"`
	AssetID    string `json:"asset_id,omitempty"`
}

// AgentGuardScopeSigner produces a stable, opaque, tamper-evident key. The key
// is a locator, not an authentication credential; every API call still passes
// normal authentication and host/evidence authorization.
type AgentGuardScopeSigner struct {
	key []byte
	now func() time.Time
}

func NewAgentGuardScopeSigner(key string) (*AgentGuardScopeSigner, error) {
	if len([]byte(key)) < 32 {
		return nil, ErrAgentGuardScopeSigningKey
	}
	return &AgentGuardScopeSigner{key: []byte(key), now: time.Now}, nil
}

func (s *AgentGuardScopeSigner) Sign(scope AgentGuardScope) (string, error) {
	scope.HostID = strings.TrimSpace(scope.HostID)
	scope.AgentType = strings.ToLower(strings.TrimSpace(scope.AgentType))
	scope.ProfileKey = strings.TrimSpace(scope.ProfileKey)
	scope.AssetID = strings.TrimSpace(scope.AssetID)
	if scope.HostID == "" || scope.AgentType == "" {
		return "", ErrAgentGuardScopeInvalid
	}

	payload, err := json.Marshal(scope)
	if err != nil {
		return "", fmt.Errorf("encode agent guard scope: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := s.signature(encoded)
	return agentGuardScopeTokenVersion + "." + encoded + "." + signature, nil
}

func (s *AgentGuardScopeSigner) Verify(token string) (AgentGuardScope, error) {
	var scope AgentGuardScope
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != agentGuardScopeTokenVersion {
		return scope, ErrAgentGuardScopeInvalid
	}
	expected := s.signature(parts[1])
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return scope, ErrAgentGuardScopeInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return scope, ErrAgentGuardScopeInvalid
	}
	if err := json.Unmarshal(payload, &scope); err != nil {
		return scope, ErrAgentGuardScopeInvalid
	}
	scope.HostID = strings.TrimSpace(scope.HostID)
	scope.AgentType = strings.ToLower(strings.TrimSpace(scope.AgentType))
	scope.ProfileKey = strings.TrimSpace(scope.ProfileKey)
	scope.AssetID = strings.TrimSpace(scope.AssetID)
	if scope.HostID == "" || scope.AgentType == "" {
		return AgentGuardScope{}, ErrAgentGuardScopeInvalid
	}
	return scope, nil
}

func (s *AgentGuardScopeSigner) signature(payload string) string {
	return s.signatureFor(agentGuardScopeTokenVersion, payload)
}

// AgentGuardPanoramaNodeRef is the authorization-bound lookup state encoded in
// a short-lived panorama node ID. Clients cannot turn arbitrary UUIDs into
// repository predicates by concatenating strings.
type AgentGuardPanoramaNodeRef struct {
	NodeType   string `json:"node_type"`
	ObjectID   string `json:"object_id"`
	HostID     string `json:"host_id"`
	AssetID    string `json:"asset_id,omitempty"`
	InstanceID string `json:"instance_id,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	ExpiresAt  int64  `json:"expires_at"`
}

func (s *AgentGuardScopeSigner) SignPanoramaNode(ref AgentGuardPanoramaNodeRef, ttl time.Duration) (string, error) {
	ref.NodeType = strings.TrimSpace(ref.NodeType)
	ref.ObjectID = strings.TrimSpace(ref.ObjectID)
	ref.HostID = strings.TrimSpace(ref.HostID)
	if ref.NodeType == "" || ref.ObjectID == "" || ref.HostID == "" || ttl <= 0 {
		return "", ErrAgentGuardNodeInvalid
	}
	ref.ExpiresAt = s.now().Add(ttl).Unix()
	payload, err := json.Marshal(ref)
	if err != nil {
		return "", fmt.Errorf("encode agent guard panorama node: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return agentGuardNodeTokenVersion + "." + encoded + "." + s.signatureFor(agentGuardNodeTokenVersion, encoded), nil
}

func (s *AgentGuardScopeSigner) VerifyPanoramaNode(token string) (AgentGuardPanoramaNodeRef, error) {
	var ref AgentGuardPanoramaNodeRef
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != agentGuardNodeTokenVersion {
		return ref, ErrAgentGuardNodeInvalid
	}
	expected := s.signatureFor(agentGuardNodeTokenVersion, parts[1])
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return ref, ErrAgentGuardNodeInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(payload, &ref) != nil {
		return AgentGuardPanoramaNodeRef{}, ErrAgentGuardNodeInvalid
	}
	if strings.TrimSpace(ref.NodeType) == "" ||
		strings.TrimSpace(ref.ObjectID) == "" ||
		strings.TrimSpace(ref.HostID) == "" ||
		ref.ExpiresAt <= s.now().Unix() {
		return AgentGuardPanoramaNodeRef{}, ErrAgentGuardNodeInvalid
	}
	return ref, nil
}

func (s *AgentGuardScopeSigner) signatureFor(version, payload string) string {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(version))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
