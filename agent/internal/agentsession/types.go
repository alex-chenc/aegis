package agentsession

import "time"

// Source identifies a supported on-disk agent session format.
type Source string

const (
	SourceClaude Source = "claude-code"
	SourceCodex  Source = "codex"
)

type ItemType string

const (
	ItemUserMessage        ItemType = "user_message"
	ItemAssistantMessage   ItemType = "assistant_message"
	ItemToolCall           ItemType = "tool_call"
	ItemToolResult         ItemType = "tool_result"
	ItemPermissionRequest  ItemType = "permission_request"
	ItemPermissionDecision ItemType = "permission_decision"
	ItemCompaction         ItemType = "compaction"
	ItemSubagent           ItemType = "subagent"
	ItemLifecycle          ItemType = "lifecycle"
)

type Usage struct {
	InputTokens              *int64 `json:"input_tokens,omitempty"`
	OutputTokens             *int64 `json:"output_tokens,omitempty"`
	CacheCreationInputTokens *int64 `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     *int64 `json:"cache_read_input_tokens,omitempty"`
}

type Item struct {
	SourceMessageID string         `json:"source_message_id,omitempty"`
	SourcePartID    string         `json:"source_part_id,omitempty"`
	SourceRevision  string         `json:"source_revision"`
	SourceSequence  uint64         `json:"source_sequence"`
	TurnID          string         `json:"turn_id,omitempty"`
	ItemType        ItemType       `json:"item_type"`
	Role            string         `json:"role"`
	OccurredAt      time.Time      `json:"occurred_at"`
	Content         string         `json:"content,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	Usage           *Usage         `json:"usage,omitempty"`
	SourceDigest    string         `json:"source_digest"`
	ContentDigest   string         `json:"content_digest"`
	RedactionState  string         `json:"redaction_state"`
	Visibility      string         `json:"visibility"`
	RedactionCount  int            `json:"redaction_count,omitempty"`
}

type SessionDelta struct {
	Source        Source    `json:"source"`
	SessionID     string    `json:"source_session_id"`
	ParentSession string    `json:"source_parent_session_id,omitempty"`
	SourceVersion string    `json:"source_version,omitempty"`
	ProjectPath   string    `json:"project_path,omitempty"`
	Model         string    `json:"model,omitempty"`
	Items         []Item    `json:"items"`
	LastSourceAt  time.Time `json:"last_source_at"`
	Unsupported   bool      `json:"unsupported"`
}

type SourceRoot struct {
	Source Source
	UID    int
	Root   string
}

type ScanConfig struct {
	Roots           []SourceRoot
	InitialLookback time.Duration
	MaxDepth        int
	MaxFiles        int
	MaxScanDuration time.Duration
	MaxNewBytes     int64
	MaxItemBytes    int
	MaxSessionBytes int64
	ScanInterval    time.Duration
}

func (c ScanConfig) withDefaults() ScanConfig {
	if c.InitialLookback <= 0 {
		c.InitialLookback = 14 * 24 * time.Hour
	}
	if c.MaxDepth <= 0 {
		c.MaxDepth = 8
	}
	if c.MaxFiles <= 0 {
		c.MaxFiles = 2000
	}
	if c.MaxScanDuration <= 0 {
		c.MaxScanDuration = 2 * time.Second
	}
	if c.MaxNewBytes <= 0 {
		c.MaxNewBytes = 64 << 20
	}
	if c.MaxItemBytes <= 0 {
		c.MaxItemBytes = 256 << 10
	}
	if c.MaxSessionBytes <= 0 {
		c.MaxSessionBytes = 50 << 20
	}
	if c.ScanInterval <= 0 {
		c.ScanInterval = 30 * time.Second
	}
	return c
}

type FileCursor struct {
	SourceIdentity         string    `json:"source_identity"`
	ByteOffset             int64     `json:"byte_offset"`
	LastCompleteLineDigest string    `json:"last_complete_line_digest,omitempty"`
	LastFileSize           int64     `json:"last_file_size"`
	LastFileMTime          time.Time `json:"last_file_mtime"`
	LastSourceSequence     uint64    `json:"last_source_sequence"`
	PartialLine            []byte    `json:"-"`
}

type ScanResult struct {
	Sessions         []SessionDelta
	FilesDiscovered  int
	FilesProcessed   int
	BytesRead        int64
	BudgetExhausted  bool
	UnsupportedFiles int
	CursorUpdates    map[string]FileCursor
}
