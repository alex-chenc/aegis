package assistant

// ToolSelectionResult is the exact mapping output passed to Runtime. Tool names
// originate from capability mappings and authorization hard gates, never scores.
type ToolSelectionResult struct {
	SelectedTools  []string     `json:"selected_tools"`
	CandidateTools []string     `json:"candidate_tools"`
	Query          string       `json:"query"`
	Intent         IntentResult `json:"intent"`
	MaxTools       int          `json:"max_tools"`
}

func (r ToolSelectionResult) ToolNames() []string {
	return r.SelectedTools
}
