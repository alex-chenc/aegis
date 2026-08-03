package service

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"api-server/internal/model"
)

const (
	AgentGuardToolSemanticsTrusted      = "trusted"
	AgentGuardToolSemanticsUnobservable = "tool_semantics_unobservable"
	AgentGuardRemoteVisibilityTrusted   = "trusted_sensor"
	AgentGuardRemoteUnobservable        = "remote_unobservable"
)

var agentGuardSHA256Pattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// AgentGuardPanoramaTrust is the deliberately small trust projection exposed
// by Panorama. Raw correlation tokens, token hashes, proof digests, verifier
// metadata and external session IDs never cross this boundary.
type AgentGuardPanoramaTrust struct {
	ToolSemantics    string `json:"tool_semantics,omitempty"`
	Source           string `json:"source,omitempty"`
	ProofVerified    bool   `json:"proof_verified,omitempty"`
	RemoteVisibility string `json:"remote_visibility,omitempty"`
	Correlation      string `json:"correlation,omitempty"`
}

type AgentGuardPanoramaCollection struct {
	Limitations []string `json:"limitations,omitempty"`
}

type AgentGuardEvidenceProjection struct {
	Trust      *AgentGuardPanoramaTrust      `json:"trust,omitempty"`
	Collection *AgentGuardPanoramaCollection `json:"collection,omitempty"`
}

// ProjectAgentGuardToolEvidence accepts tool semantics only when a trusted
// producer, a complete verified proof and the stored trusted session all agree.
// Process names and command lines are intentionally not inputs to this check.
func ProjectAgentGuardToolEvidence(
	event model.AgentBehaviorEvent,
	session *model.AgentBehaviorSession,
) AgentGuardEvidenceProjection {
	if event.Category != "tool" {
		return AgentGuardEvidenceProjection{}
	}

	unobservable := AgentGuardEvidenceProjection{
		Trust: &AgentGuardPanoramaTrust{
			ToolSemantics: AgentGuardToolSemanticsUnobservable,
			Correlation:   "unmatched",
		},
		Collection: &AgentGuardPanoramaCollection{
			Limitations: []string{AgentGuardToolSemanticsUnobservable},
		},
	}
	if !isAgentGuardToolOperation(event.Operation) {
		return unobservable
	}

	var collection struct {
		Source string `json:"source"`
	}
	if json.Unmarshal(event.Collection, &collection) != nil || !isTrustedAgentGuardSessionSource(collection.Source) {
		return unobservable
	}
	if session == nil || event.SessionID == nil || *event.SessionID != session.ID ||
		session.Source != collection.Source || !isTrustedAgentGuardSessionConfidence(session.Confidence) {
		return unobservable
	}
	if event.ResourceType != "tool" || strings.TrimSpace(event.ResourceIdentity) == "" ||
		event.Decision != "audit" || event.RuleID != "" ||
		(event.Severity != "info" && event.Severity != "low") {
		return unobservable
	}

	var evidence struct {
		CorrelationTokenHash string `json:"correlation_token_hash"`
		TrustedProof         struct {
			Verified    bool   `json:"verified"`
			Verifier    string `json:"verifier"`
			ProofDigest string `json:"proof_digest"`
			IssuedAt    string `json:"issued_at"`
		} `json:"trusted_proof"`
	}
	if json.Unmarshal(event.Evidence, &evidence) != nil ||
		!evidence.TrustedProof.Verified || evidence.TrustedProof.Verifier != "ed25519" ||
		!agentGuardSHA256Pattern.MatchString(evidence.TrustedProof.ProofDigest) ||
		!agentGuardSHA256Pattern.MatchString(evidence.CorrelationTokenHash) ||
		!agentGuardSHA256Pattern.MatchString(session.CorrelationTokenHash) ||
		evidence.CorrelationTokenHash != session.CorrelationTokenHash ||
		event.CorrelationID != evidence.CorrelationTokenHash {
		return unobservable
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, evidence.TrustedProof.IssuedAt)
	if err != nil || issuedAt.IsZero() || event.OccurredAt.IsZero() || absoluteDuration(event.OccurredAt.Sub(issuedAt)) > 5*time.Minute {
		return unobservable
	}

	return AgentGuardEvidenceProjection{
		Trust: &AgentGuardPanoramaTrust{
			ToolSemantics: AgentGuardToolSemanticsTrusted,
			Source:        collection.Source,
			ProofVerified: true,
			Correlation:   "matched",
		},
	}
}

func absoluteDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

// ProjectAgentGuardRemoteVisibility fails remote execution closed. A remote
// unit is observable only when the state projection carries an explicit sensor
// verification marker; the mere presence of a remote process or ID is not proof.
func ProjectAgentGuardRemoteVisibility(unit model.AgentExecutionUnit) AgentGuardEvidenceProjection {
	if unit.UnitType != "remote_sandbox" && strings.TrimSpace(unit.RemoteBackend) == "" {
		return AgentGuardEvidenceProjection{}
	}
	verified := false
	var actual struct {
		RemoteSensorVerified bool `json:"remote_sensor_verified"`
	}
	if json.Unmarshal(unit.IsolationActual, &actual) == nil {
		verified = actual.RemoteSensorVerified
	}
	if verified && unit.CoverageLevel != model.AgentGuardCoverageRemoteUnobservable {
		return AgentGuardEvidenceProjection{
			Trust: &AgentGuardPanoramaTrust{RemoteVisibility: AgentGuardRemoteVisibilityTrusted},
		}
	}
	return AgentGuardEvidenceProjection{
		Trust: &AgentGuardPanoramaTrust{RemoteVisibility: AgentGuardRemoteUnobservable},
		Collection: &AgentGuardPanoramaCollection{
			Limitations: []string{AgentGuardRemoteUnobservable},
		},
	}
}

func isTrustedAgentGuardSessionSource(source string) bool {
	switch source {
	case model.AgentGuardSessionSourceAgentOfficial,
		model.AgentGuardSessionSourceAdapterHook,
		model.AgentGuardSessionSourceAegisWrapper:
		return true
	default:
		return false
	}
}

func isTrustedAgentGuardSessionConfidence(confidence string) bool {
	return confidence == "confirmed"
}

func isAgentGuardToolOperation(operation string) bool {
	switch operation {
	case "tool_call_started", "tool_call_completed", "tool_call_failed":
		return true
	default:
		return false
	}
}
