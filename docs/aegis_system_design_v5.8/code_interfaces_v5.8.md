# V5.8 关键代码接口草案

**版本**: 5.8  
**日期**: 2026-05-22  
**状态**: 设计中

---

## 1. api-server 模型

```go
type DetectionPackageDraft struct {
    ID                uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
    PackageID         string         `gorm:"size:160;uniqueIndex;not null" json:"package_id"`
    TargetVersion     string         `gorm:"size:32;not null" json:"target_version"`
    Title             string         `gorm:"size:255;not null" json:"title"`
    Description       string         `json:"description"`
    CVEIDs            datatypes.JSON `gorm:"type:jsonb;not null" json:"cve_ids"`
    AIGenerated       bool           `gorm:"not null;default:false" json:"ai_generated"`
    AIGenerationInput datatypes.JSON `gorm:"type:jsonb" json:"ai_generation_input"`
    HookPlanYAML      string         `gorm:"type:text" json:"hook_plan_yaml"`
    EBPFSource        string         `gorm:"type:text" json:"ebpf_source"`
    SigmaRulesYAML    string         `gorm:"type:text" json:"sigma_rules_yaml"`
    CorrelationYAML   string         `gorm:"type:text" json:"correlation_yaml"`
    BuildParams       datatypes.JSON `gorm:"type:jsonb;not null" json:"build_params"`
    Status            string         `gorm:"size:32;not null;default:'draft'" json:"status"`
}

type DetectionPackage struct {
    ID                 uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
    PackageID          string         `gorm:"size:160;uniqueIndex:idx_pkg_version;not null" json:"package_id"`
    Version            string         `gorm:"size:32;uniqueIndex:idx_pkg_version;not null" json:"version"`
    Title              string         `gorm:"size:255;not null" json:"title"`
    Description        string         `json:"description"`
    CVEIDs             datatypes.JSON `gorm:"type:jsonb;not null" json:"cve_ids"`
    Status             string         `gorm:"size:32;not null" json:"status"`
    PackageObjectKey   string         `json:"package_object_key"`
    SignatureObjectKey string         `json:"signature_object_key"`
    PackageSize        int64          `json:"package_size"`
    PackageSHA256      string         `gorm:"size:64" json:"package_sha256"`
    ManifestJSON       datatypes.JSON `gorm:"type:jsonb;not null" json:"manifest_json"`
    HookSummary        datatypes.JSON `gorm:"type:jsonb;not null" json:"hook_summary"`
    EventSchema        datatypes.JSON `gorm:"type:jsonb;not null" json:"event_schema"`
    LimitsJSON         datatypes.JSON `gorm:"type:jsonb;not null" json:"limits_json"`
}
```

---

## 2. api-server 服务接口

```go
type DetectionPackageService interface {
    GenerateDraft(ctx context.Context, req GeneratePackageDraftRequest) (*DetectionPackageDraft, error)
    CreateDraft(ctx context.Context, req CreatePackageDraftRequest) (*DetectionPackageDraft, error)
    UpdateDraft(ctx context.Context, draftID uuid.UUID, req UpdatePackageDraftRequest) (*DetectionPackageDraft, error)
    StartBuild(ctx context.Context, packageID string, req StartBuildRequest) (*DetectionPackageBuild, error)
    GetBuild(ctx context.Context, buildID uuid.UUID) (*DetectionPackageBuild, error)
    SignPackage(ctx context.Context, packageID string, req SignPackageRequest) (*DetectionPackage, error)
    EnablePackage(ctx context.Context, packageID string, req EnablePackageRequest) error
    DisablePackage(ctx context.Context, packageID string, req DisablePackageRequest) error
    UninstallPackage(ctx context.Context, packageID string, req UninstallPackageRequest) error
    ListHostStatus(ctx context.Context, packageID, version string, q PageQuery) ([]PackageHostStatus, int64, error)
}
```

---

## 3. Builder client 接口

API Server 通过内部 gRPC 调用 builder。私钥只在 builder 内，API Server 只保存任务和发布状态。

```go
type BuilderClient interface {
    GetBuilderInfo(ctx context.Context) (*BuilderInfo, error)
    StartPackageBuild(ctx context.Context, req StartPackageBuildRequest) (*StartPackageBuildResult, error)
    GetPackageBuildStatus(ctx context.Context, buildID uuid.UUID) (*PackageBuildStatus, error)
    SignPackage(ctx context.Context, req BuilderSignPackageRequest) (*BuilderSignPackageResult, error)
}

type BuilderInfo struct {
    BuilderVersion               string   `json:"builder_version"`
    BuilderImage                 string   `json:"builder_image"`
    BuilderImageDigest           string   `json:"builder_image_digest"`
    ClangVersion                 string   `json:"clang_version"`
    BPFToolVersion               string   `json:"bpftool_version"`
    SupportedArches              []string `json:"supported_arches"`
    SupportedTransports          []string `json:"supported_transports"`
    SigningPublicKeyFingerprint  string   `json:"signing_public_key_fingerprint"`
}

type StartPackageBuildRequest struct {
    BuildID             uuid.UUID `json:"build_id"`
    PackageID           string    `json:"package_id"`
    Version             string    `json:"version"`
    Title               string    `json:"title"`
    CVEIDs              []string  `json:"cve_ids"`
    Operator            string    `json:"operator"`
    BuilderProfile      string    `json:"builder_profile"`
    TargetArch          string    `json:"target_arch"`
    HookPlanYAML        string    `json:"hook_plan_yaml"`
    EBPFSource          string    `json:"ebpf_source"`
    SigmaRulesYAML      string    `json:"sigma_rules_yaml"`
    CorrelationYAML     string    `json:"correlation_yaml"`
    PackageMetadataJSON string    `json:"package_metadata_json"`
}

type StartPackageBuildResult struct {
    Accepted bool      `json:"accepted"`
    BuildID  uuid.UUID `json:"build_id"`
    Status   string    `json:"status"`
    Message  string    `json:"message"`
}

type PackageBuildStatus struct {
    BuildID                  uuid.UUID       `json:"build_id"`
    PackageID                string          `json:"package_id"`
    Version                  string          `json:"version"`
    Status                   string          `json:"status"`
    ErrorMessage             string          `json:"error_message"`
    BuilderImageDigest       string          `json:"builder_image_digest"`
    ClangVersion             string          `json:"clang_version"`
    BuildLogObjectKey        string          `json:"build_log_object_key"`
    BuildLogTail             string          `json:"build_log_tail"`
    Artifacts                []BuildArtifact `json:"artifacts"`
    HookSummary              []HookSummary   `json:"hook_summary"`
    EventSchemaJSON          string          `json:"event_schema_json"`
    UnsignedPackageObjectKey string          `json:"unsigned_package_object_key"`
    UnsignedPackageSHA256    string          `json:"unsigned_package_sha256"`
    UnsignedPackageSize      int64           `json:"unsigned_package_size"`
}

type BuildArtifact struct {
    Name      string `json:"name"`
    Transport string `json:"transport"`
    ObjectKey string `json:"object_key"`
    SHA256    string `json:"sha256"`
    Size      int64  `json:"size"`
}

type HookSummary struct {
    HookType       string `json:"hook_type"`
    AttachPoint    string `json:"attach_point"`
    ProgramSection string `json:"program_section"`
    RiskLevel      string `json:"risk_level"`
}

type BuilderSignPackageRequest struct {
    BuildID   uuid.UUID `json:"build_id"`
    PackageID string    `json:"package_id"`
    Version   string    `json:"version"`
    Operator  string    `json:"operator"`
    Confirm   bool      `json:"confirm"`
}

type BuilderSignPackageResult struct {
    PackageObjectKey      string `json:"package_object_key"`
    SignatureObjectKey    string `json:"signature_object_key"`
    PackageSHA256         string `json:"package_sha256"`
    PackageSize           int64  `json:"package_size"`
    SignatureAlgorithm    string `json:"signature_algorithm"`
    SigningKeyFingerprint string `json:"signing_key_fingerprint"`
    SignedAt              int64  `json:"signed_at"`
}
```

---

## 4. Hook allowlist 接口

```go
type HookAllowlist struct {
    Version     int64    `json:"version"`
    Tracepoints []string `json:"tracepoints"`
    Kprobes     []string `json:"kprobes"`
    LSM         []string `json:"lsm"`
    XDP         []string `json:"xdp"`
    TC          []string `json:"tc"`
}

type HookAllowlistService interface {
    GetCurrent(ctx context.Context) (*HookAllowlist, error)
    Update(ctx context.Context, req UpdateHookAllowlistRequest) (*HookAllowlist, error)
    Broadcast(ctx context.Context, allowlist *HookAllowlist) error
}
```

---

## 5. Agent package manager

```go
type PackageManager struct {
    publicKey  ed25519.PublicKey
    store      PackageStore
    downloader Downloader
    pluginMgr  *plugin.Manager
    sigmaMgr   *PackageSigmaManager
    corrEngine *correlation.Engine
    allowlist  *HookAllowlist
}

func (m *PackageManager) ApplyAllowlist(ctx context.Context, allowlist HookAllowlist) error
func (m *PackageManager) Install(ctx context.Context, cmd DetectionPackageCommand) error
func (m *PackageManager) Uninstall(ctx context.Context, packageID, version string) error
func (m *PackageManager) DisableByPolicy(ctx context.Context, packageID string, reason string) error
func (m *PackageManager) Status() []DetectionPackageStatus
```

---

## 6. Agent manifest 类型

```go
type PackageManifest struct {
    SchemaVersion    string            `yaml:"schema_version"`
    PackageID        string            `yaml:"package_id"`
    Version          string            `yaml:"version"`
    Title            string            `yaml:"title"`
    Description      string            `yaml:"description"`
    MinAgentVersion  string            `yaml:"min_agent_version"`
    Plugin           PluginRef         `yaml:"plugin"`
    Artifacts        ArtifactRefs      `yaml:"artifacts"`
    SigmaRules       []string          `yaml:"sigma_rules"`
    CorrelationRules []string          `yaml:"correlation_rules"`
    Limits           PackageLimits     `yaml:"limits"`
}

type PluginManifest struct {
    SchemaVersion string                 `yaml:"schema_version"`
    PluginID      string                 `yaml:"plugin_id"`
    PackageID     string                 `yaml:"package_id"`
    EventMap      string                 `yaml:"event_map"`
    Hooks         []PluginHook           `yaml:"hooks"`
    EventSchema   PluginEventSchema      `yaml:"event_schema"`
}

type PluginHook struct {
    Name       string `yaml:"name"`
    AttachType string `yaml:"attach_type"`
    Attach     string `yaml:"attach"`
    Program    string `yaml:"program"`
}
```

---

## 7. Plugin loader

```go
type PluginManager struct {
    caps *kernel.Capabilities
}

type LoadedPlugin struct {
    PackageID      string
    Version        string
    PluginID       string
    ActiveArtifact string
    Collection     *ebpf.Collection
    Links          []link.Link
    Reader         ebpf.EventReader
}

func (m *PluginManager) Load(ctx context.Context, pkg PackageManifest, plugin PluginManifest, dir string) (*LoadedPlugin, error)
func (m *PluginManager) Unload(plugin *LoadedPlugin) error
```

---

## 8. Plugin event and TLV

```go
type PluginEventEnvelope struct {
    TimestampNS  uint64
    PluginIDHash uint32
    EventType    uint32
    PID          uint32
    TID          uint32
    UID          uint32
    GID          uint32
    PayloadLen   uint32
    Payload      [256]byte
}

type DecodedPluginEvent struct {
    PackageID string
    PluginID  string
    EventName string
    Timestamp int64
    PID       int
    TID       int
    UID       int
    GID       int
    Fields    map[string]any
}

func DecodeEnvelope(raw []byte) (*PluginEventEnvelope, error)
func DecodeTLV(payload []byte, schema EventSchema) (map[string]any, error)
```

---

## 9. Correlation engine

```go
type AtomicFinding struct {
    PackageID  string
    Version    string
    RuleID     string
    EventType  string
    Timestamp  int64
    HostID     string
    Hostname   string
    PID        int
    PPID       int
    UID        int
    Process    ProcessContext
    EventMap   map[string]any
}

type CorrelationSpec struct {
    ID        string
    PackageID string
    Requires  []string
    Correlation CorrelationClause
    Alert     AlertSpec
}

type CorrelationClause struct {
    By       string
    Window   time.Duration
    Ordered  bool
    Sequence []SequenceStep
}

type Engine struct {
    specs  map[string]CorrelationSpec
    cache  *FindingCache
    limits CorrelationLimits
}

func (e *Engine) AddSpec(spec CorrelationSpec) error
func (e *Engine) RemovePackage(packageID string)
func (e *Engine) AddFinding(f AtomicFinding) ([]CorrelationAlert, error)
```

---

## 10. Frontend TypeScript

```typescript
export interface DetectionPackage {
  package_id: string
  version: string
  title: string
  description?: string
  cve_ids: string[]
  status: 'draft' | 'build_failed' | 'built' | 'signed' | 'enabled' | 'disabled'
  hook_summary: PackageHook[]
  event_schema: Record<string, unknown>
  host_total?: number
  host_active?: number
  host_failed?: number
}

export interface PackageHook {
  name: string
  attach_type: 'tracepoint' | 'kprobe' | 'lsm' | 'xdp' | 'tc'
  attach: string
  program: string
  allowed?: boolean
}

export interface PackageHostStatus {
  host_id: string
  hostname: string
  status: string
  active_artifact?: 'ringbuf' | 'perf'
  loaded_hooks: string[]
  kernel_release?: string
  arch?: string
  error_message?: string
  last_reported_at?: string
}

export interface EBPFHookAllowlist {
  version: number
  tracepoints: string[]
  kprobes: string[]
  lsm: string[]
  xdp: string[]
  tc: string[]
}
```

---

## 11. eBPF C 公共头

建议新增：

```text
agent/internal/ebpf/bpf/plugin_event.h
```

```c
#ifndef __AEGIS_PLUGIN_EVENT_H
#define __AEGIS_PLUGIN_EVENT_H

#define AEGIS_PLUGIN_PAYLOAD_MAX 256

enum aegis_tlv_type {
    AEGIS_TLV_STRING = 1,
    AEGIS_TLV_INT32  = 2,
    AEGIS_TLV_UINT32 = 3,
    AEGIS_TLV_INT64  = 4,
    AEGIS_TLV_UINT64 = 5,
    AEGIS_TLV_BOOL   = 6,
    AEGIS_TLV_BYTES  = 7,
};

struct aegis_plugin_event {
    __u64 timestamp_ns;
    __u32 plugin_id_hash;
    __u32 event_type;
    __u32 pid;
    __u32 tid;
    __u32 uid;
    __u32 gid;
    __u32 payload_len;
    __u8  payload[AEGIS_PLUGIN_PAYLOAD_MAX];
};

#endif
```
