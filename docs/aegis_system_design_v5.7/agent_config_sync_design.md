# Agent Config Sync Design (V5.7)

## 1. Problem Statement

When an aegis-agent goes offline and later reconnects, only Sigma detection rules are re-synced. Other critical configurations — command audit rules (blacklist patterns) and command audit settings (blacklist_enabled, ai_enabled, etc.) — are never pushed to agents. This means:

1. **New agent connects**: Receives Sigma rules but NOT audit configs
2. **Agent goes offline, configs change, agent reconnects**: Sigma rules are re-synced, but changed audit rules/settings are NOT synced
3. **No unified config sync**: There's no single mechanism to push all agent-required configurations

### Current Config Sync on Agent Connect

| Event | What Gets Synced | Mechanism |
|-------|-----------------|-----------|
| `Register` RPC | Sigma rules only | `pushRulesToAgent()` via callback client |
| `ExecuteCommand` stream open | Sigma rules only | `pushActiveRulesToAgent()` via stream |
| Heartbeat | Nothing | Redis timestamp only |

### Gap: What Should Also Be Synced

| Config Type | Synced? | Impact |
|-------------|---------|--------|
| Command audit rules (blacklist patterns) | NO | Agent can't enforce local blacklist checks |
| Command audit settings | NO | Agent doesn't know if blacklist/AI checks are enabled |

## 2. Design Goals

1. When an agent connects (new or reconnecting), the server pushes ALL required configurations
2. Extend the existing `pushRulesToAgent` pattern to include audit configs
3. Minimal proto changes — reuse existing message patterns where possible
4. Agent-side config manager that can accept runtime updates
5. Backward compatible — old agents that don't understand new config types simply ignore them

## 3. Proto Changes

### New Message Types (`agent_comm.proto`)

```protobuf
// ConfigSync 配置同步
message ConfigSync {
  string config_type = 1;    // "sigma_rules", "audit_rules", "audit_settings"
  string action = 2;         // "full_sync", "incremental"
  string payload = 3;        // JSON格式配置内容
}

// ConfigSyncRequest 配置同步请求
message ConfigSyncRequest {
  repeated ConfigSync configs = 1;
}

// ConfigSyncResponse 配置同步响应
message ConfigSyncResponse {
  bool success = 1;
  string message = 2;
  map<string, bool> applied = 3;  // config_type -> success
}
```

### New RPC in `AgentService`

```protobuf
service AgentService {
  // ... existing RPCs ...
  rpc SyncConfig(ConfigSyncRequest) returns (ConfigSyncResponse);
}
```

### Add to `CommandRequest` oneof (bidirectional stream)

```protobuf
message CommandRequest {
  oneof request {
    CommandExecute execute = 1;
    CommandResult result = 2;
    RuleUpdateRequest rule_update = 3;
    BlockCommand block = 4;
    ConfigSync config_sync = 5;    // NEW
  }
}
```

### Payload Formats

**audit_rules** payload (JSON array):
```json
[
  {
    "id": "uuid",
    "name": "rule name",
    "rule_type": "hard_block|soft_warn",
    "match_type": "regex|exact",
    "pattern": "regex_pattern",
    "category": "system|custom",
    "severity": "high|medium|low",
    "applies_to": ["all"],
    "is_enabled": true
  }
]
```

**audit_settings** payload (JSON object):
```json
{
  "blacklist_enabled": true,
  "ai_enabled": true,
  "max_retry": 3,
  "dispatch_check": true,
  "agent_check": true
}
```

## 4. Server-Side Changes

### 4.1 New Models (`server/internal/model/`)

**`command_audit_rule.go`** — Mirror of api-server model:
```go
type CommandAuditRule struct {
    ID        uuid.UUID   `gorm:"type:uuid;primaryKey"`
    Name      string      `gorm:"type:varchar(200);not null"`
    RuleType  string      `gorm:"type:varchar(20);not null;default:hard_block"`
    MatchType string      `gorm:"type:varchar(20);not null;default:regex"`
    Pattern   string      `gorm:"type:text;not null"`
    Category  string      `gorm:"type:varchar(50);not null;default:system"`
    Severity  string      `gorm:"type:varchar(20);not null;default:high"`
    AppliesTo StringArray `gorm:"type:jsonb;not null;default:'[\"all\"]'"`
    IsEnabled bool        `gorm:"not null;default:true"`
}
```

**`system_config.go`** — Mirror of api-server model:
```go
type SystemConfig struct {
    ID          uuid.UUID       `gorm:"type:uuid;primaryKey"`
    ConfigKey   string          `gorm:"type:varchar(200);uniqueIndex;not null"`
    ConfigValue json.RawMessage `gorm:"type:jsonb;not null"`
    Category    string          `gorm:"type:varchar(50);not null"`
}
```

### 4.2 New Repositories (`server/internal/repository/`)

**`command_audit_rule_repo.go`**:
```go
type CommandAuditRuleRepo struct { db *gorm.DB }
func (r *CommandAuditRuleRepo) FindAllEnabled() ([]model.CommandAuditRule, error)
```

**`system_config_repo.go`**:
```go
type SystemConfigRepo struct { db *gorm.DB }
func (r *SystemConfigRepo) GetCommandAuditSettings() (*model.CommandAuditSettings, error)
```

### 4.3 GRPCServer Changes (`server/internal/grpc_server/server.go`)

Add new fields:
```go
type GRPCServer struct {
    // ... existing fields ...
    commandAuditRuleRepo *repository.CommandAuditRuleRepo
    systemConfigRepo     *repository.SystemConfigRepo
}
```

Add setter methods:
```go
func (s *GRPCServer) SetCommandAuditRuleRepo(repo *repository.CommandAuditRuleRepo)
func (s *GRPCServer) SetSystemConfigRepo(repo *repository.SystemConfigRepo)
```

### 4.4 New Function: `pushConfigToAgent`

Replaces the current `pushRulesToAgent` with a unified config push:

```go
func (s *GRPCServer) pushConfigToAgent(hostID uuid.UUID) {
    // Wait for connection
    conn := s.waitForConnection(hostID, 5, 1*time.Second)

    // 1. Build sigma rules config
    sigmaRules := s.buildSigmaRulesConfig()

    // 2. Build audit rules config
    auditRules := s.buildAuditRulesConfig()

    // 3. Build audit settings config
    auditSettings := s.buildAuditSettingsConfig()

    // 4. Send all configs via SyncConfig RPC
    configs := []*pb.ConfigSync{sigmaRules, auditRules, auditSettings}
    conn.CallbackClient.SyncConfig(ctx, &pb.ConfigSyncRequest{Configs: configs})
}
```

### 4.5 Modify `Register` and `ExecuteCommand`

Replace `go s.pushRulesToAgent(hostID)` with `go s.pushConfigToAgent(hostID)`.
Replace `go s.pushActiveRulesToAgent(hostID, connection)` with `go s.pushActiveConfigToAgent(hostID, connection)`.

### 4.6 Wire New Repositories in `server/cmd/main.go`

```go
commandAuditRuleRepo := repository.NewCommandAuditRuleRepo(db)
systemConfigRepo := repository.NewSystemConfigRepo(db)
grpcServer.SetCommandAuditRuleRepo(commandAuditRuleRepo)
grpcServer.SetSystemConfigRepo(systemConfigRepo)
```

## 5. Agent-Side Changes

### 5.1 New: Config Manager (`agent/internal/configmgr/configmgr.go`)

```go
type ConfigManager struct {
    auditRules    []AuditRule
    auditSettings AuditSettings
    mu            sync.RWMutex
}

type AuditRule struct {
    ID        string   `json:"id"`
    Name      string   `json:"name"`
    RuleType  string   `json:"rule_type"`
    MatchType string   `json:"match_type"`
    Pattern   string   `json:"pattern"`
    Category  string   `json:"category"`
    Severity  string   `json:"severity"`
    AppliesTo []string `json:"applies_to"`
    IsEnabled bool     `json:"is_enabled"`
}

type AuditSettings struct {
    BlacklistEnabled bool `json:"blacklist_enabled"`
    AIEnabled        bool `json:"ai_enabled"`
    MaxRetry         int  `json:"max_retry"`
    DispatchCheck    bool `json:"dispatch_check"`
    AgentCheck       bool `json:"agent_check"`
}

func (m *ConfigManager) ApplyConfigSync(sync *pb.ConfigSync) error
func (m *ConfigManager) GetAuditRules() []AuditRule
func (m *ConfigManager) GetAuditSettings() AuditSettings
func (m *ConfigManager) IsBlacklistEnabled() bool
```

### 5.2 Modify Agent Client (`agent/internal/client/client.go`)

Add `configManager` field to `Client` struct.

Add `SyncConfig` RPC handler:
```go
func (c *Client) SyncConfig(ctx context.Context, req *pb.ConfigSyncRequest) (*pb.ConfigSyncResponse, error) {
    applied := make(map[string]bool)
    for _, sync := range req.Configs {
        if err := c.configManager.ApplyConfigSync(sync); err != nil {
            applied[sync.ConfigType] = false
        } else {
            applied[sync.ConfigType] = true
        }
    }
    return &pb.ConfigSyncResponse{Success: true, Applied: applied}, nil
}
```

Handle `config_sync` in stream receive loop:
```go
if configSync := req.GetConfigSync(); configSync != nil {
    c.configManager.ApplyConfigSync(configSync)
}
```

## 6. Implementation Plan

### Phase 1: Proto & Code Generation
1. Update `proto/agent_comm.proto` with new messages and RPC
2. Update `proto/api_server_comm.proto` with new RPC for API Server -> Server
3. Run `protoc` to generate Go code

### Phase 2: Server-Side
1. Add models (command_audit_rule.go, system_config.go)
2. Add repositories (command_audit_rule_repo.go, system_config_repo.go)
3. Add `pushConfigToAgent` and related functions
4. Modify `Register` and `ExecuteCommand` to use new config push
5. Wire repositories in main.go

### Phase 3: Agent-Side
1. Add ConfigManager
2. Implement `SyncConfig` RPC handler
3. Handle `config_sync` in stream loop
4. Wire ConfigManager in agent initialization

### Phase 4: API Server -> Server Config Sync
1. Add `SyncAgentConfig` RPC to api_server_comm.proto
2. Implement in server's APIServerToServer
3. Add API endpoint for manual config re-sync (optional)

## 7. Testing Strategy

### Unit Tests
- Server: `pushConfigToAgent` builds correct payloads
- Server: Config repos query correctly
- Agent: `ConfigManager.ApplyConfigSync` handles each config type
- Agent: `SyncConfig` RPC handler returns correct response

### Integration Tests
- Agent connects -> receives all configs via SyncConfig
- Agent reconnects -> receives updated configs
- Config change on server -> next agent connect gets new config

### Manual Verification
1. Start system, note agent config
2. Add/modify audit rules via API
3. Restart agent
4. Verify agent receives updated audit rules

## 8. Files to Modify

### Proto
- `proto/agent_comm.proto` — Add ConfigSync messages and SyncConfig RPC
- `proto/api_server_comm.proto` — Add SyncAgentConfig RPC (optional, for API Server -> Server)

### Server
- `server/internal/model/command_audit_rule.go` — NEW
- `server/internal/model/system_config.go` — NEW
- `server/internal/repository/command_audit_rule_repo.go` — NEW
- `server/internal/repository/system_config_repo.go` — NEW
- `server/internal/grpc_server/server.go` — Modify Register, ExecuteCommand, add pushConfigToAgent
- `server/internal/grpc_server/api_server_impl.go` — Add SyncAgentConfig handler (optional)
- `server/cmd/main.go` — Wire new repos

### Agent
- `agent/internal/configmgr/configmgr.go` — NEW
- `agent/internal/client/client.go` — Add SyncConfig handler, config_sync stream handling
- `agent/cmd/agent/main.go` — Wire ConfigManager

### Generated Code
- `server/pkg/api/v1/agent_comm.pb.go` — Regenerated
- `server/pkg/api/v1/agent_comm_grpc.pb.go` — Regenerated
- `agent/pkg/api/v1/agent_comm.pb.go` — Regenerated
- `agent/pkg/api/v1/agent_comm_grpc.pb.go` — Regenerated
