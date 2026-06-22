-- =====================================================
-- V6.1 Weak Password Detection
-- =====================================================

-- 1. Weak Password Scan Tasks (主任务表)
CREATE TABLE IF NOT EXISTS weak_password_scan_tasks (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name               TEXT NOT NULL,
    trigger_source     VARCHAR(32) NOT NULL DEFAULT 'manual',
    status             VARCHAR(32) NOT NULL DEFAULT 'pending',
    progress           INT NOT NULL DEFAULT 0,
    current_stage      VARCHAR(64),
    scope_json         JSONB NOT NULL DEFAULT '{}',
    dictionary_policy_json JSONB NOT NULL DEFAULT '{}',
    ai_policy_json     JSONB NOT NULL DEFAULT '{}',
    total_hosts        INT NOT NULL DEFAULT 0,
    total_applications INT NOT NULL DEFAULT 0,
    matched_findings   INT NOT NULL DEFAULT 0,
    failed_applications INT NOT NULL DEFAULT 0,
    created_by         UUID,
    started_at         TIMESTAMPTZ,
    finished_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_task_status CHECK (
        status IN ('pending','analyzing_assets','collecting_credentials','repairing_collection','matching','completed','partial_failed','failed','cancelled')
    )
);

CREATE INDEX IF NOT EXISTS idx_wp_tasks_status ON weak_password_scan_tasks(status);
CREATE INDEX IF NOT EXISTS idx_wp_tasks_created_at ON weak_password_scan_tasks(created_at DESC);

-- 2. Weak Password Asset App Analyses (应用资产分析批次)
CREATE TABLE IF NOT EXISTS weak_password_asset_app_analyses (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID REFERENCES weak_password_scan_tasks(id) ON DELETE SET NULL,
    scope_json         JSONB NOT NULL DEFAULT '{}',
    status             VARCHAR(32) NOT NULL DEFAULT 'pending',
    application_asset_count INT NOT NULL DEFAULT 0,
    candidate_count    INT NOT NULL DEFAULT 0,
    error_code         VARCHAR(64),
    error_message      TEXT,
    llm_model          VARCHAR(128),
    prompt_summary     TEXT,
    created_by         UUID,
    started_at         TIMESTAMPTZ,
    finished_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_analysis_status CHECK (
        status IN ('pending','analyzing','completed','failed')
    )
);

CREATE INDEX IF NOT EXISTS idx_wp_analyses_status ON weak_password_asset_app_analyses(status);

-- 3. Weak Password Candidate Applications (AI 分析出的候选应用)
CREATE TABLE IF NOT EXISTS weak_password_candidate_applications (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    analysis_id        UUID NOT NULL REFERENCES weak_password_asset_app_analyses(id) ON DELETE CASCADE,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    asset_id           UUID,
    application_name   VARCHAR(255) NOT NULL,
    application_type   VARCHAR(64) NOT NULL,
    application_version VARCHAR(128),
    profile_id         VARCHAR(128),
    confidence         DOUBLE PRECISION NOT NULL DEFAULT 0,
    credential_types   JSONB NOT NULL DEFAULT '[]',
    candidate_paths_json JSONB NOT NULL DEFAULT '[]',
    extractor_plan_json JSONB NOT NULL DEFAULT '[]',
    asset_evidence_json JSONB NOT NULL DEFAULT '{}',
    ai_reason          TEXT,
    status             VARCHAR(32) NOT NULL DEFAULT 'candidate',
    ignored_by         UUID,
    ignored_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_candidate_status CHECK (
        status IN ('candidate','planned','collecting','repairing','matching','matched','no_match','failed','ignored')
    )
);

CREATE INDEX IF NOT EXISTS idx_wp_candidates_analysis ON weak_password_candidate_applications(analysis_id);
CREATE INDEX IF NOT EXISTS idx_wp_candidates_host ON weak_password_candidate_applications(host_id);
CREATE INDEX IF NOT EXISTS idx_wp_candidates_asset ON weak_password_candidate_applications(asset_id);

-- 4. Weak Password Collection Plans (采集计划)
CREATE TABLE IF NOT EXISTS weak_password_collection_plans (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    candidate_application_id UUID REFERENCES weak_password_candidate_applications(id) ON DELETE SET NULL,
    plan_json          JSONB NOT NULL,
    llm_analysis_json  JSONB NOT NULL DEFAULT '{}',
    status             VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_plan_status CHECK (
        status IN ('pending','executing','completed','failed')
    )
);

CREATE INDEX IF NOT EXISTS idx_wp_plans_task ON weak_password_collection_plans(task_id);
CREATE INDEX IF NOT EXISTS idx_wp_plans_host ON weak_password_collection_plans(host_id);

-- 5. Weak Password Scan Hosts (任务维度主机状态)
CREATE TABLE IF NOT EXISTS weak_password_scan_hosts (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    status             VARCHAR(32) NOT NULL DEFAULT 'pending',
    agent_status       VARCHAR(32) NOT NULL DEFAULT 'unknown',
    progress           INT NOT NULL DEFAULT 0,
    current_stage      VARCHAR(64),
    collected_records  INT NOT NULL DEFAULT 0,
    matched_findings   INT NOT NULL DEFAULT 0,
    failed_applications INT NOT NULL DEFAULT 0,
    error_code         VARCHAR(64),
    error_message      TEXT,
    started_at         TIMESTAMPTZ,
    finished_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_host_status CHECK (
        status IN ('pending','collecting','repairing','matching','completed','partial_failed','failed')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_wp_hosts_task_host ON weak_password_scan_hosts(task_id, host_id);
CREATE INDEX IF NOT EXISTS idx_wp_hosts_status ON weak_password_scan_hosts(status);

-- 6. Weak Password Scan Applications (单应用检查状态)
CREATE TABLE IF NOT EXISTS weak_password_scan_applications (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_host_id       UUID NOT NULL REFERENCES weak_password_scan_hosts(id) ON DELETE CASCADE,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    asset_id           UUID,
    candidate_application_id UUID REFERENCES weak_password_candidate_applications(id) ON DELETE SET NULL,
    application_name   VARCHAR(255) NOT NULL,
    application_type   VARCHAR(64) NOT NULL,
    profile_id         VARCHAR(128),
    status             VARCHAR(32) NOT NULL DEFAULT 'pending',
    progress           INT NOT NULL DEFAULT 0,
    current_stage      VARCHAR(64),
    agent_tool_call_count INT NOT NULL DEFAULT 0,
    max_agent_tool_calls INT NOT NULL DEFAULT 10,
    collected_records  INT NOT NULL DEFAULT 0,
    matched_findings   INT NOT NULL DEFAULT 0,
    attempted_paths_json JSONB NOT NULL DEFAULT '[]',
    error_code         VARCHAR(64),
    error_message      TEXT,
    started_at         TIMESTAMPTZ,
    finished_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_app_status CHECK (
        status IN ('pending','planned','collecting','repairing','matching','matched','no_match','failed')
    )
);

CREATE INDEX IF NOT EXISTS idx_wp_apps_task ON weak_password_scan_applications(task_id);
CREATE INDEX IF NOT EXISTS idx_wp_apps_host ON weak_password_scan_applications(host_id);
CREATE INDEX IF NOT EXISTS idx_wp_apps_status ON weak_password_scan_applications(status);

-- 7. Weak Password Agent Tool Calls (Agent 工具调用记录)
CREATE TABLE IF NOT EXISTS weak_password_agent_tool_calls (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_application_id UUID REFERENCES weak_password_scan_applications(id) ON DELETE CASCADE,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    call_id            VARCHAR(255) NOT NULL,
    tool_name          VARCHAR(128) NOT NULL,
    arguments_summary_json JSONB NOT NULL DEFAULT '{}',
    result_summary_json JSONB NOT NULL DEFAULT '{}',
    status             VARCHAR(32) NOT NULL DEFAULT 'pending',
    error_code         VARCHAR(64),
    error_message      TEXT,
    execution_time_ms  BIGINT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_tool_call_status CHECK (
        status IN ('pending','executing','completed','failed')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_wp_tool_calls_call_id ON weak_password_agent_tool_calls(call_id);
CREATE INDEX IF NOT EXISTS idx_wp_tool_calls_task ON weak_password_agent_tool_calls(task_id);
CREATE INDEX IF NOT EXISTS idx_wp_tool_calls_app ON weak_password_agent_tool_calls(scan_application_id);

-- 8. Weak Password Dictionaries (字典元数据)
CREATE TABLE IF NOT EXISTS weak_password_dictionaries (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name               VARCHAR(255) NOT NULL,
    dictionary_type    VARCHAR(32) NOT NULL,
    status             VARCHAR(32) NOT NULL DEFAULT 'enabled',
    entry_count        INT NOT NULL DEFAULT 0,
    source             VARCHAR(64) NOT NULL,
    categories         JSONB NOT NULL DEFAULT '[]',
    generation_policy_json JSONB NOT NULL DEFAULT '{}',
    prompt_summary     TEXT,
    llm_model          VARCHAR(128),
    created_by         UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_dict_type CHECK (
        dictionary_type IN ('default_1000','uploaded','ai_generated','task_temp')
    ),
    CONSTRAINT chk_wp_dict_status CHECK (
        status IN ('enabled','disabled')
    )
);

CREATE INDEX IF NOT EXISTS idx_wp_dicts_type ON weak_password_dictionaries(dictionary_type);
CREATE INDEX IF NOT EXISTS idx_wp_dicts_status ON weak_password_dictionaries(status);

-- 9. Weak Password Dictionary Entries (字典条目)
CREATE TABLE IF NOT EXISTS weak_password_dictionary_entries (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dictionary_id      UUID NOT NULL REFERENCES weak_password_dictionaries(id) ON DELETE CASCADE,
    candidate          TEXT NOT NULL,
    candidate_hash     VARCHAR(64) NOT NULL,
    category           VARCHAR(64),
    rule_source        VARCHAR(128),
    risk_level         VARCHAR(32),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_wp_dict_entries_hash ON weak_password_dictionary_entries(dictionary_id, candidate_hash);
CREATE INDEX IF NOT EXISTS idx_wp_dict_entries_dictionary ON weak_password_dictionary_entries(dictionary_id);

-- 10. Weak Password Match Batches (匹配批次)
CREATE TABLE IF NOT EXISTS weak_password_match_batches (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_application_id UUID REFERENCES weak_password_scan_applications(id) ON DELETE CASCADE,
    batch_type         VARCHAR(32) NOT NULL,
    status             VARCHAR(32) NOT NULL DEFAULT 'pending',
    credential_type    VARCHAR(32),
    candidate_count    INT NOT NULL DEFAULT 0,
    record_count       INT NOT NULL DEFAULT 0,
    llm_model          VARCHAR(128),
    prompt_summary     TEXT,
    result_summary_json JSONB NOT NULL DEFAULT '{}',
    error_code         VARCHAR(64),
    error_message      TEXT,
    started_at         TIMESTAMPTZ,
    finished_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_batch_type CHECK (
        batch_type IN ('plaintext_dictionary','plaintext_hybrid','plaintext_fuzzy','llm_encrypted','llm_hybrid')
    ),
    CONSTRAINT chk_wp_batch_status CHECK (
        status IN ('pending','processing','completed','failed')
    )
);

CREATE INDEX IF NOT EXISTS idx_wp_batches_task ON weak_password_match_batches(task_id);
CREATE INDEX IF NOT EXISTS idx_wp_batches_app ON weak_password_match_batches(scan_application_id);

-- 11. Weak Password Findings (命中结果)
CREATE TABLE IF NOT EXISTS weak_password_findings (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_application_id UUID REFERENCES weak_password_scan_applications(id) ON DELETE CASCADE,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    asset_id           UUID,
    application_name   VARCHAR(255) NOT NULL,
    application_type   VARCHAR(64) NOT NULL,
    account            VARCHAR(255) NOT NULL,
    credential_type    VARCHAR(32) NOT NULL,
    match_status       VARCHAR(32) NOT NULL,
    matched_password_mask VARCHAR(128),
    matched_password_encrypted BYTEA,
    match_source       VARCHAR(64) NOT NULL,
    match_rule         VARCHAR(128) NOT NULL,
    dictionary_id      UUID REFERENCES weak_password_dictionaries(id) ON DELETE SET NULL,
    confidence         DOUBLE PRECISION NOT NULL DEFAULT 0,
    source_path        TEXT,
    field_path         VARCHAR(255),
    evidence_json      JSONB NOT NULL DEFAULT '{}',
    ai_reason          TEXT,
    fixed_at           TIMESTAMPTZ,
    false_positive_at  TIMESTAMPTZ,
    risk_accepted_at   TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_finding_status CHECK (
        match_status IN ('confirmed','ai_inferred_needs_confirm','verify_failed','false_positive','fixed','risk_accepted')
    )
);

CREATE INDEX IF NOT EXISTS idx_wp_findings_task ON weak_password_findings(task_id);
CREATE INDEX IF NOT EXISTS idx_wp_findings_host ON weak_password_findings(host_id);
CREATE INDEX IF NOT EXISTS idx_wp_findings_status ON weak_password_findings(match_status);
CREATE INDEX IF NOT EXISTS idx_wp_findings_app ON weak_password_findings(application_type);

-- 12. Weak Password Collection Errors (采集错误)
CREATE TABLE IF NOT EXISTS weak_password_collection_errors (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_application_id UUID REFERENCES weak_password_scan_applications(id) ON DELETE CASCADE,
    host_id            UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    application_name   VARCHAR(255),
    source_path        TEXT,
    error_code         VARCHAR(64) NOT NULL,
    error_message      TEXT,
    agent_tool_call_count INT NOT NULL DEFAULT 0,
    attempted_paths_json JSONB NOT NULL DEFAULT '[]',
    repair_trace_json  JSONB NOT NULL DEFAULT '[]',
    final_status       VARCHAR(32) NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_error_final_status CHECK (
        final_status IN ('pending','resolved','unresolved','config_discovery_failed')
    )
);

CREATE INDEX IF NOT EXISTS idx_wp_errors_task ON weak_password_collection_errors(task_id);
CREATE INDEX IF NOT EXISTS idx_wp_errors_code ON weak_password_collection_errors(error_code);

-- 13. Weak Password AI Reports (AI 分析报告)
CREATE TABLE IF NOT EXISTS weak_password_ai_reports (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID NOT NULL REFERENCES weak_password_scan_tasks(id) ON DELETE CASCADE,
    scan_application_id UUID REFERENCES weak_password_scan_applications(id) ON DELETE CASCADE,
    report_type        VARCHAR(64) NOT NULL,
    llm_model          VARCHAR(128),
    prompt_summary     TEXT,
    report_json        JSONB NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_report_type CHECK (
        report_type IN ('asset_analysis','collection_repair','encrypted_match','dictionary_generate','result_explain')
    )
);

CREATE INDEX IF NOT EXISTS idx_wp_reports_task ON weak_password_ai_reports(task_id);
CREATE INDEX IF NOT EXISTS idx_wp_reports_app ON weak_password_ai_reports(scan_application_id);

-- 14. Weak Password Reveal Audits (明文查看审计)
CREATE TABLE IF NOT EXISTS weak_password_reveal_audits (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    finding_id         UUID NOT NULL REFERENCES weak_password_findings(id) ON DELETE CASCADE,
    requester_id       UUID NOT NULL,
    approver_id        UUID,
    status             VARCHAR(32) NOT NULL DEFAULT 'pending',
    reason             TEXT,
    watermark          VARCHAR(255),
    revealed_at        TIMESTAMPTZ,
    expires_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wp_reveal_status CHECK (
        status IN ('pending','approved','rejected','expired')
    )
);

CREATE INDEX IF NOT EXISTS idx_wp_reveals_finding ON weak_password_reveal_audits(finding_id);
CREATE INDEX IF NOT EXISTS idx_wp_reveals_requester ON weak_password_reveal_audits(requester_id);
