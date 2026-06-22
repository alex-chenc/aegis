package weakpass

type CredentialCollectionRequest struct {
	TaskID           string                   `json:"task_id"`
	PlanID           string                   `json:"plan_id"`
	HostID           string                   `json:"host_id"`
	Applications     []ApplicationCollectPlan `json:"applications"`
	CollectionPolicy CollectionPolicy         `json:"collection_policy"`
}

type ApplicationCollectPlan struct {
	Application string                `json:"application"`
	AssetID     string                `json:"asset_id"`
	ProfileID   string                `json:"profile_id"`
	Paths       []string              `json:"paths"`
	Extractors  []CredentialExtractor `json:"extractors"`
}

type CredentialExtractor struct {
	Type             string `json:"type"`
	Section          string `json:"section,omitempty"`
	AccountSelector  string `json:"account_selector,omitempty"`
	PasswordSelector string `json:"password_selector,omitempty"`
	FormatHint       string `json:"format_hint,omitempty"`
	SourceKind       string `json:"source_kind,omitempty"`
}

type CollectionPolicy struct {
	MaxFileBytes          int64 `json:"max_file_bytes"`
	MaxRecords            int   `json:"max_records"`
	RedactContextValues   bool  `json:"redact_context_values"`
	ForbidFindCommand     bool  `json:"forbid_find_command"`
	ForbidRecursiveSearch bool  `json:"forbid_recursive_search"`
}

type CredentialRecord struct {
	RecordID        string  `json:"record_id"`
	Application     string  `json:"application"`
	AssetID         string  `json:"asset_id"`
	SourcePath      string  `json:"source_path"`
	SourceKind      string  `json:"source_kind"`
	Account         string  `json:"account"`
	CredentialType  string  `json:"credential_type"`
	CredentialValue string  `json:"credential_value"`
	Salt            string  `json:"salt"`
	AlgorithmHint   string  `json:"algorithm_hint"`
	FieldPath       string  `json:"field_path"`
	Parser          string  `json:"parser"`
	Confidence      float64 `json:"confidence"`
}

type CredentialCollectionError struct {
	Application             string   `json:"application"`
	SourcePath              string   `json:"source_path"`
	ErrorCode               string   `json:"error_code"`
	Message                 string   `json:"message"`
	Retryable               bool     `json:"retryable"`
	SuggestedAuxiliaryTools []string `json:"suggested_auxiliary_tools"`
}

type CredentialCollectionResult struct {
	TaskID  string                      `json:"task_id"`
	PlanID  string                      `json:"plan_id"`
	HostID  string                      `json:"host_id"`
	Records []CredentialRecord          `json:"records"`
	Errors  []CredentialCollectionError `json:"errors"`
}

type PathProbeRequest struct {
	Path string `json:"path"`
}

type PathProbeResult struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Type   string `json:"type"`
	Size   int64  `json:"size"`
	Mode   string `json:"mode"`
	Owner  string `json:"owner"`
}

type ConfigDirListRequest struct {
	Dir             string   `json:"dir"`
	SuffixAllowlist []string `json:"suffix_allowlist"`
	MaxEntries      int      `json:"max_entries"`
	Recursive       bool     `json:"recursive"`
}

type ConfigDirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

type ConfigDirListResult struct {
	Dir     string           `json:"dir"`
	Entries []ConfigDirEntry `json:"entries"`
}

type ConfigSliceRequest struct {
	Path         string `json:"path"`
	StartLine    int    `json:"start_line"`
	EndLine      int    `json:"end_line"`
	MaxBytes     int64  `json:"max_bytes"`
	RedactValues bool   `json:"redact_values"`
}

type ConfigSliceResult struct {
	Path      string   `json:"path"`
	StartLine int      `json:"start_line"`
	EndLine   int      `json:"end_line"`
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"`
	Redacted  bool     `json:"redacted"`
}

type ServiceUnitInspectRequest struct {
	Service string   `json:"service"`
	Paths   []string `json:"paths"`
}

type ServiceUnitInspectResult struct {
	Service          string   `json:"service"`
	UnitPath         string   `json:"unit_path"`
	ExecStart        []string `json:"exec_start"`
	EnvironmentFiles []string `json:"environment_files"`
	WorkingDirectory string   `json:"working_directory"`
	User             string   `json:"user"`
}

type ProcessConfigHintsRequest struct {
	PID                 int      `json:"pid"`
	IncludeOpenFiles    bool     `json:"include_open_files"`
	FileSuffixAllowlist []string `json:"file_suffix_allowlist"`
	MaxFiles            int      `json:"max_files"`
}

type ProcessConfigHintsResult struct {
	PID             int      `json:"pid"`
	Cmdline         []string `json:"cmdline"`
	CWD             string   `json:"cwd"`
	OpenConfigFiles []string `json:"open_config_files"`
}

type PurgeCredentialCacheRequest struct {
	TaskID string `json:"task_id"`
}

type PurgeResult struct {
	TaskID string `json:"task_id"`
	Purged bool   `json:"purged"`
}

const (
	CredentialTypePlaintext     = "plaintext"
	CredentialTypeHash          = "hash"
	CredentialTypeSaltedHash    = "salted_hash"
	CredentialTypeEncryptedBlob = "encrypted_blob"
	CredentialTypeAuthString    = "auth_string"
	CredentialTypeUnknown       = "unknown"
)

const (
	ErrPermissionDenied    = "permission_denied"
	ErrFileNotFound        = "file_not_found"
	ErrFieldNotFound       = "field_not_found"
	ErrFileTooLarge        = "file_too_large"
	ErrInvalidPath         = "invalid_path"
	ErrInvalidPolicy       = "invalid_policy"
	ErrUnsupportedFormat   = "unsupported_credential_format"
	ErrRecordLimitReached  = "record_limit_reached"
	ErrRecursiveNotAllowed = "recursive_not_allowed"
	ErrInvalidRequest      = "invalid_request"
)
