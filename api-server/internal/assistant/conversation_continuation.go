package assistant

import (
	"encoding/json"
	"fmt"
	"strings"
)

const pendingClarificationMetadataKey = "pending_clarification"

// PendingClarification is the durable boundary between a run that requested
// one missing field and the next user turn. It stores semantic state, never a
// previous authorization decision, so resumed work must pass Mapping and hard
// gates again.
type PendingClarification struct {
	OriginalQuery   string            `json:"original_query"`
	Goal            string            `json:"goal,omitempty"`
	Question        string            `json:"question"`
	WorkflowIDs     []string          `json:"workflow_ids,omitempty"`
	MissingInfo     []MissingInfo     `json:"missing_info,omitempty"`
	IntentBreakdown *IntentBreakdown  `json:"intent_breakdown,omitempty"`
	Artifacts       map[string]string `json:"artifacts,omitempty"`
}

func pendingClarificationFromMetadata(metadata map[string]interface{}) *PendingClarification {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata[pendingClarificationMetadataKey]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var pending PendingClarification
	if err := json.Unmarshal(encoded, &pending); err != nil {
		return nil
	}
	if strings.TrimSpace(pending.OriginalQuery) == "" || strings.TrimSpace(pending.Question) == "" {
		return nil
	}
	pending.WorkflowIDs = dedupeStrings(pending.WorkflowIDs)
	return &pending
}

func resolveContinuationQuery(currentQuery string, intent IntentResult, pending *PendingClarification) (string, []string, bool) {
	currentQuery = strings.TrimSpace(currentQuery)
	// An explicit Client authorization is a new goal even when the previous
	// turn left a Server-onboarding clarification in session metadata. Do not
	// merge the old endpoint question or onboarding workflow into this request.
	if isMCPClientAuthorizationRequest(currentQuery) {
		return currentQuery, dedupeStrings(intent.WorkflowIDs), false
	}
	if pending == nil || !strings.EqualFold(intent.ContinuationMode, "resume_pending") {
		return currentQuery, dedupeStrings(intent.WorkflowIDs), false
	}
	resolved := strings.TrimSpace(intent.ResolvedQuery)
	if resolved == "" {
		resolved = fmt.Sprintf(
			"Original request:\n%s\n\nClarification question:\n%s\n\nUser answer:\n%s",
			strings.TrimSpace(pending.OriginalQuery),
			strings.TrimSpace(pending.Question),
			currentQuery,
		)
	}
	// A short clarification answer is allowed to look like another domain
	// (for example an IP looking like host_management), but it must not replace
	// the already-authorized workflow family. The combined request is fully
	// decomposed and authorized again below.
	return resolved, dedupeStrings(pending.WorkflowIDs), true
}
