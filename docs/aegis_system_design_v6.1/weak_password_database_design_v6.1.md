# Aegis V6.1 弱密码检测数据库设计

## 1. 文档目标

本文定义 V6.1 弱密码检测的数据模型、表结构、索引、敏感字段存储策略和迁移建议。数据库以 PostgreSQL 为主，遵循当前 Aegis GORM repository 模式。

## 2. 设计原则

1. 原始密码、hash、salt、token 默认不进入普通业务表。
2. 命中结果展示使用脱敏值；完整命中密码如需保存，必须加密存储并通过 reveal 审批查看。
3. 应用资产分析结果、采集计划、任务进度、Agent 工具调用、匹配结果和错误原因分表保存。
4. 字典元数据和字典条目分表，便于默认 1000 字典、上传字典和 AI 生成字典统一管理。
5. LLM prompt 原文不写普通日志；数据库仅保存摘要、模型、批次、结果和审计必要字段。

## 3. 表关系概览

```text
weak_password_scan_tasks
  ├─ weak_password_asset_app_analyses
  │    └─ weak_password_candidate_applications
  ├─ weak_password_collection_plans
  ├─ weak_password_scan_hosts
  │    └─ weak_password_scan_applications
  │         ├─ weak_password_agent_tool_calls
  │         ├─ weak_password_collection_errors
  │         └─ weak_password_findings
  ├─ weak_password_match_batches
  └─ weak_password_ai_reports

weak_password_dictionaries
  └─ weak_password_dictionary_entries

weak_password_findings
  └─ weak_password_reveal_audits
```

## 4. 枚举约定

### 4.1 任务状态

| 值 | 说明 |
|:---|:---|
| `pending` | 已创建，等待执行 |
| `analyzing_assets` | 正在分析应用资产 |
| `collecting_credentials` | 正在下发 Agent 工具并采集配置 |
| `repairing_collection` | 正在 AI 修复定位配置 |
| `matching` | 正在匹配弱密码 |
| `completed` | 执行完成 |
| `partial_failed` | 部分主机或应用失败 |
| `failed` | 全部失败 |
| `cancelled` | 已取消 |

### 4.2 应用检查状态

| 值 | 说明 |
|:---|:---|
| `candidate` | AI 认为可能存在密码 |
| `planned` | 已生成采集计划 |
| `collecting` | Agent 正在采集 |
| `repairing` | AI 正在修复配置定位 |
| `matching` | 正在匹配 |
| `matched` | 有命中 |
| `no_match` | 未命中 |
| `failed` | 失败 |
| `ignored` | 用户忽略 |

### 4.3 错误码

| 错误码 | 说明 |
|:---|:---|
| `no_application_assets` | 当前范围没有应用资产 |
| `agent_not_connected` | Agent 不在线 |
| `agent_callback_unavailable` | Agent callback 不可用 |
| `permission_denied` | 文件权限不足 |
| `file_not_found` | 文件不存在 |
| `field_not_found` | 密码字段不存在 |
| `file_too_large` | 文件超过限制 |
| `config_discovery_failed` | AI 调用 10 次 Agent 工具仍未定位有效配置 |
| `llm_match_verify_failed` | AI 命中未通过服务端校验 |
| `unsupported_credential_format` | 凭据格式不支持 |

## 5. 表结构

### 5.1 `weak_password_scan_tasks`

保存弱密码检测任务主表。

```sql
CREATE TABLE weak_password_scan_tasks (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    trigger_source TEXT NOT NULL,
    status TEXT NOT NULL,
    progress INT NOT NULL DEFAULT 0,
    current_stage TEXT,
    scope_json JSONB NOT NULL DEFAULT '{}',
    dictionary_policy_json JSONB NOT NULL DEFAULT '{}',
    ai_policy_json JSONB NOT NULL DEFAULT '{}',
    total_hosts INT NOT NULL DEFAULT 0,
    total_applications INT NOT NULL DEFAULT 0,
    matched_findings INT NOT NULL DEFAULT 0,
    failed_applications INT NOT NULL DEFAULT 0,
    created_by UUID,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weak_password_scan_tasks_status ON weak_password_scan_tasks(status);
CREATE INDEX idx_weak_password_scan_tasks_created_at ON weak_password_scan_tasks(created_at DESC);
```

### 5.2 `weak_password_asset_app_analyses`

保存“一键分析资产应用”的批次。

```sql
CREATE TABLE weak_password_asset_app_analyses (
    id UUID PRIMARY KEY,
    task_id UUID REFERENCES weak_password_scan_tasks(id),
    scope_json JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL,
    application_asset_count INT NOT NULL DEFAULT 0,
    candidate_count INT NOT NULL DEFAULT 0,
    error_code TEXT,
    error_message TEXT,
    llm_model TEXT,
    prompt_summary TEXT,
    created_by UUID,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weak_password_asset_app_analyses_status ON weak_password_asset_app_analyses(status);
```

### 5.3 `weak_password_candidate_applications`

保存 AI 分析出的“可能存在密码的应用”。

```sql
CREATE TABLE weak_password_candidate_applications (
    id UUID PRIMARY KEY,
    analysis_id UUID NOT NULL REFERENCES weak_password_asset_app_analyses(id) ON DELETE CASCADE,
    host_id UUID NOT NULL,
    asset_id UUID,
    application_name TEXT NOT NULL,
    application_type TEXT NOT NULL,
    application_version TEXT,
    profile_id TEXT,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    credential_types TEXT[] NOT NULL DEFAULT '{}',
    candidate_paths_json JSONB NOT NULL DEFAULT '[]',
    extractor_plan_json JSONB NOT NULL DEFAULT '[]',
    asset_evidence_json JSONB NOT NULL DEFAULT '{}',
    ai_reason TEXT,
    status TEXT NOT NULL DEFAULT 'candidate',
    ignored_by UUID,
    ignored_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weak_password_candidate_apps_analysis ON weak_password_candidate_applications(analysis_id);
CREATE INDEX idx_weak_password_candidate_apps_host ON weak_password_candidate_applications(host_id);
CREATE INDEX idx_weak_password_candidate_apps_asset ON weak_password_candidate_applications(asset_id);
```

### 5.4 `weak_password_collection_plans`

保存服务端生成并下发给 Agent 的采集计划。

```sql
CREATE TABLE weak_password_collection_plans (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    host_id UUID NOT NULL,
    candidate_application_id UUID REFERENCES weak_password_candidate_applications(id),
    plan_json JSONB NOT NULL,
    llm_analysis_json JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weak_password_collection_plans_task ON weak_password_collection_plans(task_id);
CREATE INDEX idx_weak_password_collection_plans_host ON weak_password_collection_plans(host_id);
```

### 5.5 `weak_password_scan_hosts`

保存任务维度下的主机执行状态。

```sql
CREATE TABLE weak_password_scan_hosts (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    host_id UUID NOT NULL,
    status TEXT NOT NULL,
    agent_status TEXT NOT NULL,
    progress INT NOT NULL DEFAULT 0,
    current_stage TEXT,
    collected_records INT NOT NULL DEFAULT 0,
    matched_findings INT NOT NULL DEFAULT 0,
    failed_applications INT NOT NULL DEFAULT 0,
    error_code TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uniq_weak_password_scan_hosts_task_host ON weak_password_scan_hosts(task_id, host_id);
CREATE INDEX idx_weak_password_scan_hosts_status ON weak_password_scan_hosts(status);
```

### 5.6 `weak_password_scan_applications`

保存单应用检查状态，是前端单应用检查的主要明细表。

```sql
CREATE TABLE weak_password_scan_applications (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_host_id UUID NOT NULL REFERENCES weak_password_scan_hosts(id) ON DELETE CASCADE,
    host_id UUID NOT NULL,
    asset_id UUID,
    candidate_application_id UUID REFERENCES weak_password_candidate_applications(id),
    application_name TEXT NOT NULL,
    application_type TEXT NOT NULL,
    profile_id TEXT,
    status TEXT NOT NULL,
    progress INT NOT NULL DEFAULT 0,
    current_stage TEXT,
    agent_tool_call_count INT NOT NULL DEFAULT 0,
    max_agent_tool_calls INT NOT NULL DEFAULT 10,
    collected_records INT NOT NULL DEFAULT 0,
    matched_findings INT NOT NULL DEFAULT 0,
    attempted_paths_json JSONB NOT NULL DEFAULT '[]',
    error_code TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weak_password_scan_apps_task ON weak_password_scan_applications(task_id);
CREATE INDEX idx_weak_password_scan_apps_host ON weak_password_scan_applications(host_id);
CREATE INDEX idx_weak_password_scan_apps_status ON weak_password_scan_applications(status);
```

### 5.7 `weak_password_agent_tool_calls`

保存 Agent 工具调用记录，用于进度条和失败排查。

```sql
CREATE TABLE weak_password_agent_tool_calls (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_application_id UUID REFERENCES weak_password_scan_applications(id) ON DELETE CASCADE,
    host_id UUID NOT NULL,
    call_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    arguments_summary_json JSONB NOT NULL DEFAULT '{}',
    result_summary_json JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL,
    error_code TEXT,
    error_message TEXT,
    execution_time_ms BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uniq_weak_password_agent_tool_calls_call_id ON weak_password_agent_tool_calls(call_id);
CREATE INDEX idx_weak_password_agent_tool_calls_task ON weak_password_agent_tool_calls(task_id);
CREATE INDEX idx_weak_password_agent_tool_calls_app ON weak_password_agent_tool_calls(scan_application_id);
```

### 5.8 `weak_password_dictionaries`

保存字典元数据。

```sql
CREATE TABLE weak_password_dictionaries (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    dictionary_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'enabled',
    entry_count INT NOT NULL DEFAULT 0,
    source TEXT NOT NULL,
    categories TEXT[] NOT NULL DEFAULT '{}',
    generation_policy_json JSONB NOT NULL DEFAULT '{}',
    prompt_summary TEXT,
    llm_model TEXT,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weak_password_dictionaries_type ON weak_password_dictionaries(dictionary_type);
CREATE INDEX idx_weak_password_dictionaries_status ON weak_password_dictionaries(status);
```

字典类型：

| 值 | 说明 |
|:---|:---|
| `default_1000` | 系统内置 1000 条默认弱密码字典 |
| `uploaded` | 用户上传字典 |
| `ai_generated` | AI 一键生成字典 |
| `task_temp` | 任务临时字典 |

### 5.9 `weak_password_dictionary_entries`

保存字典条目。字典值可以明文存储在受控库中，也可以根据部署策略加密存储。普通日志不得打印条目明文。

```sql
CREATE TABLE weak_password_dictionary_entries (
    id UUID PRIMARY KEY,
    dictionary_id UUID NOT NULL REFERENCES weak_password_dictionaries(id) ON DELETE CASCADE,
    candidate TEXT NOT NULL,
    candidate_hash TEXT NOT NULL,
    category TEXT,
    rule_source TEXT,
    risk_level TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uniq_weak_password_dictionary_entry_hash ON weak_password_dictionary_entries(dictionary_id, candidate_hash);
CREATE INDEX idx_weak_password_dictionary_entries_dictionary ON weak_password_dictionary_entries(dictionary_id);
```

### 5.10 `weak_password_match_batches`

保存服务端匹配和 LLM 匹配批次。

```sql
CREATE TABLE weak_password_match_batches (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_application_id UUID REFERENCES weak_password_scan_applications(id) ON DELETE CASCADE,
    batch_type TEXT NOT NULL,
    status TEXT NOT NULL,
    credential_type TEXT,
    candidate_count INT NOT NULL DEFAULT 0,
    record_count INT NOT NULL DEFAULT 0,
    llm_model TEXT,
    prompt_summary TEXT,
    result_summary_json JSONB NOT NULL DEFAULT '{}',
    error_code TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weak_password_match_batches_task ON weak_password_match_batches(task_id);
CREATE INDEX idx_weak_password_match_batches_app ON weak_password_match_batches(scan_application_id);
```

### 5.11 `weak_password_findings`

保存命中结果。

```sql
CREATE TABLE weak_password_findings (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_application_id UUID REFERENCES weak_password_scan_applications(id) ON DELETE CASCADE,
    host_id UUID NOT NULL,
    asset_id UUID,
    application_name TEXT NOT NULL,
    application_type TEXT NOT NULL,
    account TEXT NOT NULL,
    credential_type TEXT NOT NULL,
    match_status TEXT NOT NULL,
    matched_password_mask TEXT,
    matched_password_encrypted BYTEA,
    match_source TEXT NOT NULL,
    match_rule TEXT NOT NULL,
    dictionary_id UUID REFERENCES weak_password_dictionaries(id),
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    source_path TEXT,
    field_path TEXT,
    evidence_json JSONB NOT NULL DEFAULT '{}',
    ai_reason TEXT,
    fixed_at TIMESTAMPTZ,
    false_positive_at TIMESTAMPTZ,
    risk_accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weak_password_findings_task ON weak_password_findings(task_id);
CREATE INDEX idx_weak_password_findings_host ON weak_password_findings(host_id);
CREATE INDEX idx_weak_password_findings_status ON weak_password_findings(match_status);
CREATE INDEX idx_weak_password_findings_app ON weak_password_findings(application_type);
```

### 5.12 `weak_password_collection_errors`

保存采集失败和 AI 修复失败。

```sql
CREATE TABLE weak_password_collection_errors (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_application_id UUID REFERENCES weak_password_scan_applications(id) ON DELETE CASCADE,
    host_id UUID NOT NULL,
    application_name TEXT,
    source_path TEXT,
    error_code TEXT NOT NULL,
    error_message TEXT,
    agent_tool_call_count INT NOT NULL DEFAULT 0,
    attempted_paths_json JSONB NOT NULL DEFAULT '[]',
    repair_trace_json JSONB NOT NULL DEFAULT '[]',
    final_status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weak_password_collection_errors_task ON weak_password_collection_errors(task_id);
CREATE INDEX idx_weak_password_collection_errors_code ON weak_password_collection_errors(error_code);
```

### 5.13 `weak_password_ai_reports`

保存 AI 分析摘要。

```sql
CREATE TABLE weak_password_ai_reports (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_application_id UUID REFERENCES weak_password_scan_applications(id) ON DELETE CASCADE,
    report_type TEXT NOT NULL,
    llm_model TEXT,
    prompt_summary TEXT,
    report_json JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weak_password_ai_reports_task ON weak_password_ai_reports(task_id);
CREATE INDEX idx_weak_password_ai_reports_app ON weak_password_ai_reports(scan_application_id);
```

### 5.14 `weak_password_reveal_audits`

保存完整命中密码查看审计。

```sql
CREATE TABLE weak_password_reveal_audits (
    id UUID PRIMARY KEY,
    finding_id UUID NOT NULL REFERENCES weak_password_findings(id) ON DELETE CASCADE,
    requester_id UUID NOT NULL,
    approver_id UUID,
    status TEXT NOT NULL,
    reason TEXT,
    watermark TEXT,
    revealed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weak_password_reveal_audits_finding ON weak_password_reveal_audits(finding_id);
CREATE INDEX idx_weak_password_reveal_audits_requester ON weak_password_reveal_audits(requester_id);
```

## 6. 敏感字段策略

| 字段 | 策略 |
|:---|:---|
| `CredentialRecord.credential_value` | 不落普通业务表，仅在匹配任务内存短时存在 |
| `CredentialRecord.salt` | 默认不入库；必要 evidence 中只保存 `salt_present=true` 或加密值 |
| `matched_password_mask` | 可入库，用于列表展示 |
| `matched_password_encrypted` | 可选加密入库，用于 reveal 审批 |
| `prompt_summary` | 只保存摘要，不保存完整敏感 prompt |
| `arguments_summary_json` | 保存脱敏参数，不保存密码、hash、salt |

## 7. 默认 1000 字典初始化

迁移或 seed 阶段创建：

1. 插入 `weak_password_dictionaries`，`dictionary_type=default_1000`。
2. 插入 1000 条 `weak_password_dictionary_entries`。
3. 对条目按分类打标签。
4. 使用 `candidate_hash=sha256(candidate)` 做去重。

默认字典可启用/停用，但不能删除；复制后生成用户自定义字典。

## 8. Repository 设计

建议新增：

| Repository | 责任 |
|:---|:---|
| `WeakPasswordTaskRepository` | 任务、主机、应用状态 |
| `WeakPasswordAssetAnalysisRepository` | 应用资产分析和候选应用 |
| `WeakPasswordPlanRepository` | 采集计划 |
| `WeakPasswordDictionaryRepository` | 字典和条目 |
| `WeakPasswordFindingRepository` | 命中结果和 reveal 审计 |
| `WeakPasswordToolCallRepository` | Agent 工具调用记录 |
| `WeakPasswordReportRepository` | AI 报告和匹配批次 |

## 9. 迁移建议

新增迁移文件建议命名：

```text
migrations/006_v6.1_weak_password_detection.sql
```

迁移顺序：

1. 创建字典表。
2. 创建任务、分析、候选应用表。
3. 创建采集计划、主机、应用、工具调用表。
4. 创建匹配批次、finding、错误、AI 报告、reveal 审计表。
5. 创建索引。
6. seed 默认 1000 字典。

## 10. 测试用例

- 迁移 SQL 可重复在空库执行。
- 默认 1000 字典 seed 后条数正确。
- 字典条目去重正确。
- `weak_password_scan_applications.agent_tool_call_count` 能正确累计到 10。
- `config_discovery_failed` 能写入错误表。
- finding 默认只展示脱敏密码。
- reveal 审计能关联 finding。
- 删除任务时级联删除计划、主机、应用、工具调用、错误和报告。

