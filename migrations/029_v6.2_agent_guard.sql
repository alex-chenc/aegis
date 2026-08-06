-- Aegis V6.2 Agent Guard schema and immutable built-in catalog.
-- This migration is intentionally repeatable. Published migrations must not be
-- edited to install or roll back Agent Guard.

CREATE TABLE IF NOT EXISTS agent_guard_adapter_profiles (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_key              VARCHAR(128) NOT NULL,
    profile_version          BIGINT NOT NULL,
    agent_type               VARCHAR(64) NOT NULL,
    display_name             VARCHAR(255) NOT NULL,
    source                   VARCHAR(32) NOT NULL DEFAULT 'builtin',
    sandbox_family           VARCHAR(32) NOT NULL,
    controller_match         JSONB NOT NULL DEFAULT '[]'::jsonb,
    worker_match             JSONB NOT NULL DEFAULT '[]'::jsonb,
    backend_detectors        JSONB NOT NULL DEFAULT '[]'::jsonb,
    isolation_expectation    JSONB NOT NULL DEFAULT '{}'::jsonb,
    default_escape_rules     JSONB NOT NULL DEFAULT '[]'::jsonb,
    digest                   VARCHAR(80) NOT NULL,
    enabled                  BOOLEAN NOT NULL DEFAULT TRUE,
    created_by               VARCHAR(100),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agent_guard_profile_version
        UNIQUE (profile_key, profile_version),
    CONSTRAINT chk_agent_guard_profile_source
        CHECK (source IN ('builtin', 'server', 'imported')),
    CONSTRAINT chk_agent_guard_profile_digest
        CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT chk_agent_guard_profile_family
        CHECK (sandbox_family IN (
            'local_process_tree',
            'linux_namespace',
            'oci_container',
            'remote_sandbox',
            'whole_process_container'
        ))
);

CREATE INDEX IF NOT EXISTS idx_agent_guard_profiles_agent
    ON agent_guard_adapter_profiles(agent_type, enabled);
CREATE INDEX IF NOT EXISTS idx_agent_guard_profiles_key
    ON agent_guard_adapter_profiles(profile_key, profile_version DESC);
CREATE INDEX IF NOT EXISTS idx_agent_guard_profiles_match_gin
    ON agent_guard_adapter_profiles USING GIN(controller_match);

CREATE TABLE IF NOT EXISTS agent_behavior_rule_definitions (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_key                 VARCHAR(128) NOT NULL,
    rule_version             BIGINT NOT NULL,
    name                     VARCHAR(255) NOT NULL,
    description              TEXT,
    source                   VARCHAR(24) NOT NULL DEFAULT 'builtin',
    engine                   VARCHAR(32) NOT NULL,
    categories               JSONB NOT NULL DEFAULT '[]'::jsonb,
    default_enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    default_severity         VARCHAR(20) NOT NULL,
    default_action           VARCHAR(40) NOT NULL,
    recommended_action       VARCHAR(40) NOT NULL,
    parameters_schema        JSONB NOT NULL DEFAULT '{}'::jsonb,
    default_parameters       JSONB NOT NULL DEFAULT '{}'::jsonb,
    required_evidence        JSONB NOT NULL DEFAULT '[]'::jsonb,
    allow_conditions         JSONB NOT NULL DEFAULT '[]'::jsonb,
    mitre                    JSONB NOT NULL DEFAULT '[]'::jsonb,
    immutable                BOOLEAN NOT NULL DEFAULT TRUE,
    digest                   VARCHAR(80) NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agent_behavior_rule_version
        UNIQUE (rule_key, rule_version),
    CONSTRAINT chk_agent_behavior_rule_source
        CHECK (source IN ('builtin', 'custom', 'imported')),
    CONSTRAINT chk_agent_behavior_rule_digest
        CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT chk_agent_behavior_rule_engine
        CHECK (engine IN (
            'agent_atomic',
            'dc_single_event',
            'dc_correlation',
            'agent_and_dc'
        )),
    CONSTRAINT chk_agent_behavior_rule_severity
        CHECK (default_severity IN ('info', 'low', 'medium', 'high', 'critical')),
    CONSTRAINT chk_agent_behavior_rule_action
        CHECK (default_action IN ('audit', 'alert', 'deny', 'deny_and_freeze')),
    CONSTRAINT chk_agent_behavior_rule_recommended_action
        CHECK (recommended_action IN ('audit', 'alert', 'deny', 'deny_and_freeze'))
);

CREATE INDEX IF NOT EXISTS idx_agent_behavior_rules_source
    ON agent_behavior_rule_definitions(source, rule_key, rule_version DESC);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_rules_categories
    ON agent_behavior_rule_definitions USING GIN(categories);

CREATE TABLE IF NOT EXISTS agent_guard_policies (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_key               VARCHAR(128) NOT NULL,
    version                  BIGINT NOT NULL,
    name                     VARCHAR(255) NOT NULL,
    description              TEXT,
    status                   VARCHAR(32) NOT NULL DEFAULT 'draft',
    priority                 INT NOT NULL DEFAULT 100,
    targets                  JSONB NOT NULL DEFAULT '{}'::jsonb,
    collection_policy        JSONB NOT NULL DEFAULT '{}'::jsonb,
    builtin_rule_overrides   JSONB NOT NULL DEFAULT '[]'::jsonb,
    atomic_rules             JSONB NOT NULL DEFAULT '[]'::jsonb,
    correlation_rules        JSONB NOT NULL DEFAULT '[]'::jsonb,
    analysis_policy          JSONB NOT NULL DEFAULT '{}'::jsonb,
    escape_rules             JSONB NOT NULL DEFAULT '[]'::jsonb,
    freeze_timeout_seconds   INT NOT NULL DEFAULT 300,
    compiled_preview         JSONB NOT NULL DEFAULT '{}'::jsonb,
    digest                   VARCHAR(80),
    created_by               VARCHAR(100) NOT NULL,
    published_by             VARCHAR(100),
    published_at             TIMESTAMPTZ,
    disabled_by              VARCHAR(100),
    disabled_at              TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agent_guard_policy_version
        UNIQUE (policy_key, version),
    CONSTRAINT chk_agent_guard_policy_status
        CHECK (status IN ('draft', 'published', 'superseded', 'disabled')),
    CONSTRAINT chk_agent_guard_policy_priority
        CHECK (priority >= 0 AND priority <= 10000),
    CONSTRAINT chk_agent_guard_freeze_timeout
        CHECK (freeze_timeout_seconds BETWEEN 30 AND 900)
);

CREATE INDEX IF NOT EXISTS idx_agent_guard_policies_status
    ON agent_guard_policies(status, priority DESC);
CREATE INDEX IF NOT EXISTS idx_agent_guard_policies_key
    ON agent_guard_policies(policy_key, version DESC);
CREATE INDEX IF NOT EXISTS idx_agent_guard_policies_targets_gin
    ON agent_guard_policies USING GIN(targets);

CREATE TABLE IF NOT EXISTS agent_guard_policy_deliveries (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id                  UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    bundle_version           BIGINT NOT NULL,
    bundle_digest            VARCHAR(80) NOT NULL,
    policy_versions          JSONB NOT NULL DEFAULT '[]'::jsonb,
    profile_versions         JSONB NOT NULL DEFAULT '[]'::jsonb,
    builtin_rule_versions    JSONB NOT NULL DEFAULT '{}'::jsonb,
    status                   VARCHAR(32) NOT NULL DEFAULT 'pending',
    capability_snapshot      JSONB NOT NULL DEFAULT '{}'::jsonb,
    coverage_level           VARCHAR(40),
    error_code               VARCHAR(100),
    error_message            TEXT,
    generated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at            TIMESTAMPTZ,
    received_at              TIMESTAMPTZ,
    applied_at               TIMESTAMPTZ,
    last_reported_at         TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agent_guard_delivery
        UNIQUE (host_id, bundle_version),
    CONSTRAINT chk_agent_guard_delivery_status
        CHECK (status IN (
            'pending',
            'dispatching',
            'received',
            'applied',
            'degraded',
            'failed',
            'stale',
            'unsupported_agent_version'
        )),
    CONSTRAINT chk_agent_guard_delivery_coverage
        CHECK (
            coverage_level IS NULL OR
            coverage_level IN (
                'full_enforcement',
                'behavior_monitor_escape_enforce',
                'monitor_only',
                'no_isolation',
                'remote_unobservable',
                'unsupported_profile',
                'degraded'
            )
        )
);

CREATE INDEX IF NOT EXISTS idx_agent_guard_deliveries_host_status
    ON agent_guard_policy_deliveries(host_id, status, bundle_version DESC);
CREATE INDEX IF NOT EXISTS idx_agent_guard_deliveries_status
    ON agent_guard_policy_deliveries(status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_guard_deliveries_policy_versions_gin
    ON agent_guard_policy_deliveries USING GIN(policy_versions);

CREATE TABLE IF NOT EXISTS agent_runtime_instances (
    id                       UUID PRIMARY KEY,
    host_id                  UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    asset_id                 UUID REFERENCES host_application_assets(id) ON DELETE SET NULL,
    adapter_profile_id       UUID REFERENCES agent_guard_adapter_profiles(id) ON DELETE SET NULL,
    profile_key              VARCHAR(128) NOT NULL,
    profile_version          BIGINT NOT NULL,
    agent_type               VARCHAR(64) NOT NULL,
    display_name             VARCHAR(255),
    controller_pid           INT NOT NULL,
    controller_start_ticks   NUMERIC(20,0) NOT NULL,
    controller_exe           TEXT,
    controller_cmdline       TEXT,
    run_uid                  INT,
    run_user                 VARCHAR(255),
    detection_confidence     VARCHAR(32) NOT NULL DEFAULT 'candidate',
    status                   VARCHAR(32) NOT NULL DEFAULT 'running',
    coverage_level           VARCHAR(40) NOT NULL DEFAULT 'monitor_only',
    coverage_reasons         JSONB NOT NULL DEFAULT '[]'::jsonb,
    metadata                 JSONB NOT NULL DEFAULT '{}'::jsonb,
    first_seen_at            TIMESTAMPTZ NOT NULL,
    last_seen_at             TIMESTAMPTZ NOT NULL,
    stopped_at               TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agent_runtime_process
        UNIQUE (host_id, controller_pid, controller_start_ticks),
    CONSTRAINT chk_agent_runtime_confidence
        CHECK (detection_confidence IN ('candidate', 'probable', 'confirmed')),
    CONSTRAINT chk_agent_runtime_status
        CHECK (status IN ('running', 'stale', 'stopped', 'unknown')),
    CONSTRAINT chk_agent_runtime_coverage
        CHECK (coverage_level IN (
            'full_enforcement',
            'behavior_monitor_escape_enforce',
            'monitor_only',
            'no_isolation',
            'remote_unobservable',
            'unsupported_profile',
            'degraded'
        ))
);

CREATE INDEX IF NOT EXISTS idx_agent_runtime_instances_host
    ON agent_runtime_instances(host_id, status, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_runtime_instances_type
    ON agent_runtime_instances(agent_type, status, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_runtime_instances_asset
    ON agent_runtime_instances(asset_id);
CREATE INDEX IF NOT EXISTS idx_agent_runtime_instances_coverage
    ON agent_runtime_instances(coverage_level, status);

CREATE TABLE IF NOT EXISTS agent_execution_units (
    id                       UUID PRIMARY KEY,
    host_id                  UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    instance_id              UUID NOT NULL REFERENCES agent_runtime_instances(id) ON DELETE CASCADE,
    unit_type                VARCHAR(40) NOT NULL,
    fingerprint              VARCHAR(160) NOT NULL,
    root_pid                 INT,
    root_start_ticks         NUMERIC(20,0),
    cgroup_id                VARCHAR(32),
    cgroup_path              TEXT,
    container_id             VARCHAR(128),
    container_runtime        VARCHAR(64),
    remote_backend           VARCHAR(64),
    remote_execution_id      VARCHAR(255),
    remote_host_ref          VARCHAR(255),
    isolation_baseline       JSONB NOT NULL DEFAULT '{}'::jsonb,
    isolation_actual         JSONB NOT NULL DEFAULT '{}'::jsonb,
    isolation_diff           JSONB NOT NULL DEFAULT '{}'::jsonb,
    coverage_level           VARCHAR(40) NOT NULL,
    coverage_reasons         JSONB NOT NULL DEFAULT '[]'::jsonb,
    status                   VARCHAR(32) NOT NULL DEFAULT 'observed',
    first_seen_at            TIMESTAMPTZ NOT NULL,
    last_seen_at             TIMESTAMPTZ NOT NULL,
    frozen_at                TIMESTAMPTZ,
    stopped_at               TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agent_execution_unit_fingerprint
        UNIQUE (host_id, fingerprint),
    CONSTRAINT chk_agent_execution_unit_type
        CHECK (unit_type IN (
            'local_process_tree',
            'linux_namespace',
            'oci_container',
            'remote_sandbox',
            'whole_process_container'
        )),
    CONSTRAINT chk_agent_execution_unit_status
        CHECK (status IN (
            'observed',
            'healthy',
            'violating',
            'freezing',
            'frozen',
            'resuming',
            'stopped',
            'stale',
            'unobservable',
            'degraded'
        )),
    CONSTRAINT chk_agent_execution_unit_coverage
        CHECK (coverage_level IN (
            'full_enforcement',
            'behavior_monitor_escape_enforce',
            'monitor_only',
            'no_isolation',
            'remote_unobservable',
            'unsupported_profile',
            'degraded'
        ))
);

CREATE INDEX IF NOT EXISTS idx_agent_execution_units_instance
    ON agent_execution_units(instance_id, status, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_execution_units_host
    ON agent_execution_units(host_id, status, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_execution_units_container
    ON agent_execution_units(container_id)
    WHERE container_id IS NOT NULL AND container_id <> '';
CREATE INDEX IF NOT EXISTS idx_agent_execution_units_cgroup
    ON agent_execution_units(cgroup_id)
    WHERE cgroup_id IS NOT NULL AND cgroup_id <> '';
CREATE INDEX IF NOT EXISTS idx_agent_execution_units_baseline_gin
    ON agent_execution_units USING GIN(isolation_baseline);

CREATE TABLE IF NOT EXISTS agent_behavior_sessions (
    id                       UUID PRIMARY KEY,
    host_id                  UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    instance_id              UUID NOT NULL REFERENCES agent_runtime_instances(id) ON DELETE CASCADE,
    execution_unit_id        UUID REFERENCES agent_execution_units(id) ON DELETE SET NULL,
    external_session_id      VARCHAR(255),
    source                   VARCHAR(32) NOT NULL,
    confidence               VARCHAR(20) NOT NULL,
    correlation_token_hash   VARCHAR(80),
    status                   VARCHAR(24) NOT NULL DEFAULT 'active',
    behavior_count           BIGINT NOT NULL DEFAULT 0,
    finding_count            BIGINT NOT NULL DEFAULT 0,
    completeness             JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at               TIMESTAMPTZ NOT NULL,
    last_seen_at             TIMESTAMPTZ NOT NULL,
    ended_at                 TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_agent_behavior_session_source
        CHECK (source IN (
            'agent_official',
            'adapter_hook',
            'aegis_wrapper',
            'execution_unit',
            'activity_window'
        )),
    CONSTRAINT chk_agent_behavior_session_confidence
        CHECK (confidence IN ('confirmed', 'probable', 'inferred')),
    CONSTRAINT chk_agent_behavior_session_token_hash
        CHECK (
            correlation_token_hash IS NULL OR
            correlation_token_hash ~ '^sha256:[0-9a-f]{64}$'
        ),
    CONSTRAINT chk_agent_behavior_session_status
        CHECK (status IN ('active', 'ended', 'stale'))
);

CREATE INDEX IF NOT EXISTS idx_agent_behavior_sessions_instance_time
    ON agent_behavior_sessions(instance_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_sessions_unit_time
    ON agent_behavior_sessions(execution_unit_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_sessions_external
    ON agent_behavior_sessions(host_id, external_session_id)
    WHERE external_session_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS agent_behavior_events (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    raw_event_id             VARCHAR(100) NOT NULL UNIQUE,
    host_id                  UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    host_boot_id             VARCHAR(100) NOT NULL,
    agent_sequence           BIGINT NOT NULL,
    instance_id              UUID REFERENCES agent_runtime_instances(id) ON DELETE SET NULL,
    session_id               UUID REFERENCES agent_behavior_sessions(id) ON DELETE SET NULL,
    execution_unit_id        UUID REFERENCES agent_execution_units(id) ON DELETE SET NULL,
    policy_id                UUID REFERENCES agent_guard_policies(id) ON DELETE SET NULL,
    policy_version           BIGINT,
    rule_id                  VARCHAR(100),
    schema_version           VARCHAR(64) NOT NULL DEFAULT 'aegis.agent_behavior.v1',
    correlation_id           VARCHAR(100),
    parent_event_id          VARCHAR(100),
    agent_type               VARCHAR(64),
    profile_key              VARCHAR(128),
    profile_version          BIGINT,
    category                 VARCHAR(32) NOT NULL,
    operation                VARCHAR(64) NOT NULL,
    outcome                  VARCHAR(24) NOT NULL,
    errno                    INT,
    decision                 VARCHAR(40) NOT NULL DEFAULT 'audit',
    severity                 VARCHAR(20) NOT NULL DEFAULT 'info',
    pid                      INT,
    ppid                     INT,
    process_start_ticks      NUMERIC(20,0),
    process_name             VARCHAR(255),
    process_exe              TEXT,
    command_argv             JSONB NOT NULL DEFAULT '[]'::jsonb,
    command_cwd              TEXT,
    command_visibility       VARCHAR(24) NOT NULL DEFAULT 'complete',
    process_chain            JSONB NOT NULL DEFAULT '[]'::jsonb,
    resource_type            VARCHAR(32),
    resource_identity        TEXT,
    resource_identity_hash   VARCHAR(80),
    resource_classification  VARCHAR(64),
    resource                 JSONB NOT NULL DEFAULT '{}'::jsonb,
    isolation                JSONB NOT NULL DEFAULT '{}'::jsonb,
    collection               JSONB NOT NULL DEFAULT '{}'::jsonb,
    evidence                 JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at              TIMESTAMPTZ NOT NULL,
    occurred_monotonic_ns    NUMERIC(20,0),
    received_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_agent_behavior_category
        CHECK (category IN (
            'process',
            'file',
            'network',
            'identity',
            'persistence',
            'isolation',
            'kernel',
            'ipc',
            'tool',
            'control'
        )),
    CONSTRAINT chk_agent_behavior_outcome
        CHECK (outcome IN ('success', 'failure', 'denied', 'unknown')),
    CONSTRAINT chk_agent_behavior_decision
        CHECK (decision IN (
            'allow',
            'audit',
            'alert',
            'deny',
            'deny_and_freeze',
            'would_deny',
            'enforcement_unavailable'
        )),
    CONSTRAINT chk_agent_behavior_severity
        CHECK (severity IN ('info', 'low', 'medium', 'high', 'critical')),
    CONSTRAINT chk_agent_behavior_command_visibility
        CHECK (command_visibility IN ('complete', 'partial', 'unobservable'))
);

CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_host_time
    ON agent_behavior_events(host_id, occurred_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_behavior_events_host_sequence
    ON agent_behavior_events(host_id, host_boot_id, agent_sequence);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_instance_time
    ON agent_behavior_events(instance_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_session_time
    ON agent_behavior_events(session_id, occurred_at ASC);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_unit_time
    ON agent_behavior_events(execution_unit_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_category_time
    ON agent_behavior_events(category, operation, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_decision_time
    ON agent_behavior_events(decision, severity, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_policy
    ON agent_behavior_events(policy_id, policy_version, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_rule_time
    ON agent_behavior_events(rule_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_resource_hash
    ON agent_behavior_events(resource_identity_hash, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_resource_gin
    ON agent_behavior_events USING GIN(resource);
CREATE INDEX IF NOT EXISTS idx_agent_behavior_events_collection_gin
    ON agent_behavior_events USING GIN(collection);

CREATE TABLE IF NOT EXISTS agent_security_findings (
    id                       UUID PRIMARY KEY,
    finding_key              VARCHAR(255) NOT NULL UNIQUE,
    host_id                  UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    instance_id              UUID REFERENCES agent_runtime_instances(id) ON DELETE SET NULL,
    session_id               UUID REFERENCES agent_behavior_sessions(id) ON DELETE SET NULL,
    execution_unit_id        UUID REFERENCES agent_execution_units(id) ON DELETE SET NULL,
    policy_id                UUID REFERENCES agent_guard_policies(id) ON DELETE SET NULL,
    policy_version           BIGINT,
    title                    VARCHAR(500) NOT NULL,
    severity                 VARCHAR(20) NOT NULL,
    verdict                  VARCHAR(24) NOT NULL DEFAULT 'suspicious',
    confidence               NUMERIC(5,4) NOT NULL DEFAULT 0,
    status                   VARCHAR(24) NOT NULL DEFAULT 'open',
    decision_sources         JSONB NOT NULL DEFAULT '[]'::jsonb,
    rule_hits                JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence_event_ids       JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence_graph           JSONB NOT NULL DEFAULT '{}'::jsonb,
    attack_stages            JSONB NOT NULL DEFAULT '[]'::jsonb,
    summary                  TEXT,
    recommended_action       VARCHAR(64),
    latest_analysis_id       UUID,
    handled_by               VARCHAR(100),
    handled_note             TEXT,
    handled_at               TIMESTAMPTZ,
    first_observed_at        TIMESTAMPTZ NOT NULL,
    last_observed_at         TIMESTAMPTZ NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_agent_security_finding_severity
        CHECK (severity IN ('info', 'low', 'medium', 'high', 'critical')),
    CONSTRAINT chk_agent_security_finding_verdict
        CHECK (verdict IN ('benign', 'suspicious', 'malicious', 'inconclusive')),
    CONSTRAINT chk_agent_security_finding_confidence
        CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT chk_agent_security_finding_status
        CHECK (status IN ('open', 'investigating', 'contained', 'resolved', 'dismissed'))
);

CREATE INDEX IF NOT EXISTS idx_agent_security_findings_host_time
    ON agent_security_findings(host_id, last_observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_security_findings_instance_time
    ON agent_security_findings(instance_id, last_observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_security_findings_session_time
    ON agent_security_findings(session_id, last_observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_security_findings_unit_time
    ON agent_security_findings(execution_unit_id, last_observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_security_findings_status
    ON agent_security_findings(status, severity, last_observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_security_findings_rule_hits
    ON agent_security_findings USING GIN(rule_hits);

CREATE TABLE IF NOT EXISTS agent_security_analysis_runs (
    id                       UUID PRIMARY KEY,
    finding_id               UUID NOT NULL REFERENCES agent_security_findings(id) ON DELETE CASCADE,
    attempt                  INT NOT NULL,
    status                   VARCHAR(24) NOT NULL DEFAULT 'pending',
    provider                 VARCHAR(64),
    model                    VARCHAR(128),
    prompt_version           VARCHAR(64) NOT NULL,
    input_digest             VARCHAR(80) NOT NULL,
    evidence_event_ids       JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence_summary         JSONB NOT NULL DEFAULT '{}'::jsonb,
    output                   JSONB NOT NULL DEFAULT '{}'::jsonb,
    verdict                  VARCHAR(24),
    attack_probability       NUMERIC(5,4),
    confidence               NUMERIC(5,4),
    error_code               VARCHAR(100),
    error_message            TEXT,
    requested_by             VARCHAR(100),
    queued_at                TIMESTAMPTZ NOT NULL,
    started_at               TIMESTAMPTZ,
    completed_at             TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_agent_security_analysis_attempt
        UNIQUE (finding_id, attempt),
    CONSTRAINT chk_agent_security_analysis_status
        CHECK (status IN (
            'pending',
            'running',
            'succeeded',
            'failed',
            'invalid_output',
            'inconclusive',
            'cancelled'
        )),
    CONSTRAINT chk_agent_security_analysis_verdict
        CHECK (
            verdict IS NULL OR
            verdict IN ('benign', 'suspicious', 'malicious', 'inconclusive')
        ),
    CONSTRAINT chk_agent_security_analysis_probability
        CHECK (
            attack_probability IS NULL OR
            attack_probability BETWEEN 0 AND 1
        ),
    CONSTRAINT chk_agent_security_analysis_confidence
        CHECK (confidence IS NULL OR confidence BETWEEN 0 AND 1)
);

CREATE INDEX IF NOT EXISTS idx_agent_security_analysis_finding
    ON agent_security_analysis_runs(finding_id, attempt DESC);
CREATE INDEX IF NOT EXISTS idx_agent_security_analysis_status
    ON agent_security_analysis_runs(status, queued_at);
CREATE INDEX IF NOT EXISTS idx_agent_security_analysis_digest
    ON agent_security_analysis_runs(input_digest, model, prompt_version);

DO $agent_guard_latest_analysis_fk$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'fk_agent_security_findings_latest_analysis'
          AND conrelid = 'agent_security_findings'::regclass
    ) THEN
        ALTER TABLE agent_security_findings
            ADD CONSTRAINT fk_agent_security_findings_latest_analysis
            FOREIGN KEY (latest_analysis_id)
            REFERENCES agent_security_analysis_runs(id)
            ON DELETE SET NULL;
    END IF;
EXCEPTION
    WHEN duplicate_object THEN NULL;
END
$agent_guard_latest_analysis_fk$;

CREATE TABLE IF NOT EXISTS agent_guard_actions (
    id                        UUID PRIMARY KEY,
    command_id                VARCHAR(100) UNIQUE,
    trigger_behavior_event_id UUID REFERENCES agent_behavior_events(id) ON DELETE SET NULL,
    trigger_finding_id        UUID REFERENCES agent_security_findings(id) ON DELETE SET NULL,
    host_id                   UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    instance_id               UUID REFERENCES agent_runtime_instances(id) ON DELETE SET NULL,
    execution_unit_id         UUID REFERENCES agent_execution_units(id) ON DELETE SET NULL,
    action                    VARCHAR(40) NOT NULL,
    source                    VARCHAR(32) NOT NULL,
    status                    VARCHAR(32) NOT NULL DEFAULT 'pending',
    reason                    TEXT NOT NULL,
    requested_by              VARCHAR(100),
    hold_requested            BOOLEAN NOT NULL DEFAULT FALSE,
    freeze_timeout_seconds    INT,
    result                    JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_code                VARCHAR(100),
    error_message             TEXT,
    requested_at              TIMESTAMPTZ NOT NULL,
    dispatched_at             TIMESTAMPTZ,
    completed_at              TIMESTAMPTZ,
    expires_at                TIMESTAMPTZ,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_agent_guard_action
        CHECK (action IN (
            'deny',
            'freeze_execution_unit',
            'resume_execution_unit',
            'hold_execution_unit',
            'kill_execution_unit',
            'kill_agent_instance',
            'auto_resume'
        )),
    CONSTRAINT chk_agent_guard_action_source
        CHECK (source IN (
            'local_policy',
            'correlation_policy',
            'manual',
            'timeout',
            'system'
        )),
    CONSTRAINT chk_agent_guard_action_status
        CHECK (status IN (
            'pending',
            'dispatching',
            'running',
            'success',
            'failed',
            'expired',
            'cancelled'
        )),
    CONSTRAINT chk_agent_guard_action_timeout
        CHECK (
            freeze_timeout_seconds IS NULL OR
            freeze_timeout_seconds BETWEEN 30 AND 900
        )
);

CREATE INDEX IF NOT EXISTS idx_agent_guard_actions_host_time
    ON agent_guard_actions(host_id, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_guard_actions_instance_time
    ON agent_guard_actions(instance_id, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_guard_actions_unit_time
    ON agent_guard_actions(execution_unit_id, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_guard_actions_status
    ON agent_guard_actions(status, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_guard_actions_behavior
    ON agent_guard_actions(trigger_behavior_event_id);
CREATE INDEX IF NOT EXISTS idx_agent_guard_actions_finding
    ON agent_guard_actions(trigger_finding_id);

-- The JSON payload below is the migration-side copy of the Go built-in
-- manifest. api-server repository tests parse it, canonicalize every
-- definition and verify each declared SHA-256 digest.
WITH builtin_rule_seed AS (
    SELECT *
    FROM jsonb_to_recordset(
$agent_guard_rules$
[
  {
    "id": "62000000-0000-4000-8000-000000000001",
    "rule_key": "AGB-BUILTIN-001",
    "rule_version": 1,
    "name": "操作敏感目录",
    "description": "检测已归属智能体进程对凭据、权限策略、持久化、安全控制和容器控制资源的访问或修改。",
    "source": "builtin",
    "engine": "agent_and_dc",
    "categories": ["file"],
    "default_enabled": true,
    "default_severity": "medium",
    "default_action": "alert",
    "recommended_action": "alert",
    "parameters_schema": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "resource_groups": {
          "type": "array",
          "uniqueItems": true,
          "items": {
            "enum": ["credential", "privilege_policy", "cloud_or_cluster_credential", "persistence", "security_control", "container_control"]
          }
        },
        "operations": {
          "type": "array",
          "uniqueItems": true,
          "items": {
            "enum": ["open_intent", "read_observed", "write", "create", "truncate", "delete", "rename", "chmod", "chown", "execute"]
          }
        }
      }
    },
    "default_parameters": {
      "resource_groups": ["credential", "privilege_policy", "cloud_or_cluster_credential", "persistence", "security_control", "container_control"],
      "operations": ["open_intent", "read_observed", "write", "create", "truncate", "delete", "rename", "chmod", "chown", "execute"]
    },
    "required_evidence": ["actor.pid", "actor.ppid", "actor.start_ticks", "operation", "resource.resolved_path", "resource.classification", "outcome"],
    "allow_conditions": ["trusted_process_digest", "policy_exception", "approved_change_window"],
    "mitre": ["T1005", "T1543", "T1552"],
    "immutable": true,
    "digest": "sha256:e9a7f8b0dda7c742557bbc1a0551ea4caeb0329973ec1c24f7751b4cd2902a82"
  },
  {
    "id": "62000000-0000-4000-8000-000000000002",
    "rule_key": "AGB-BUILTIN-002",
    "rule_version": 1,
    "name": "外部网络连接",
    "description": "检测已归属智能体进程主动连接非本机、非内网且不在管理员信任范围内的目标。",
    "source": "builtin",
    "engine": "agent_and_dc",
    "categories": ["network"],
    "default_enabled": true,
    "default_severity": "medium",
    "default_action": "alert",
    "recommended_action": "alert",
    "parameters_schema": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "trusted_cidrs": {"type": "array", "uniqueItems": true, "items": {"type": "string", "maxLength": 64}},
        "trusted_domains": {"type": "array", "uniqueItems": true, "items": {"type": "string", "maxLength": 253}},
        "trusted_ports": {"type": "array", "uniqueItems": true, "items": {"type": "integer", "minimum": 1, "maximum": 65535}}
      }
    },
    "default_parameters": {"trusted_cidrs": [], "trusted_domains": [], "trusted_ports": []},
    "required_evidence": ["actor.pid", "actor.ppid", "actor.start_ticks", "network.direction", "network.destination_ip", "network.destination_port", "network.protocol", "outcome"],
    "allow_conditions": ["loopback_or_link_local", "private_or_cluster_network", "trusted_destination", "policy_exception"],
    "mitre": ["T1041", "T1071"],
    "immutable": true,
    "digest": "sha256:5852cf43c0be2ddc21e83c8c12fb898ac2aae47bc0d7bff2a5246d4d2436e613"
  },
  {
    "id": "62000000-0000-4000-8000-000000000003",
    "rule_key": "AGB-BUILTIN-003",
    "rule_version": 1,
    "name": "文件生成",
    "description": "记录已归属智能体进程成功创建此前不存在的文件，并区分失败的创建意图。",
    "source": "builtin",
    "engine": "agent_and_dc",
    "categories": ["file"],
    "default_enabled": true,
    "default_severity": "low",
    "default_action": "audit",
    "recommended_action": "alert",
    "parameters_schema": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "alert_on_executable": {"type": "boolean"},
        "alert_on_hidden": {"type": "boolean"},
        "hash_max_bytes": {"type": "integer", "minimum": 0, "maximum": 104857600}
      }
    },
    "default_parameters": {"alert_on_executable": true, "alert_on_hidden": true, "hash_max_bytes": 10485760},
    "required_evidence": ["actor.pid", "actor.ppid", "actor.start_ticks", "operation", "resource.inode_created", "resource.resolved_path", "outcome"],
    "allow_conditions": ["workspace_low_risk_file", "policy_exception"],
    "mitre": ["T1105", "T1204"],
    "immutable": true,
    "digest": "sha256:b066e0b452fb7749f9afb49e8a6b918de1285ccef912f50680795a4bc110e03e"
  },
  {
    "id": "62000000-0000-4000-8000-000000000004",
    "rule_key": "AGB-BUILTIN-004",
    "rule_version": 1,
    "name": "敏感命令执行",
    "description": "检测具有网络传输、提权、权限变更、隔离控制、持久化、破坏或防御规避能力的命令执行。",
    "source": "builtin",
    "engine": "agent_and_dc",
    "categories": ["process"],
    "default_enabled": true,
    "default_severity": "medium",
    "default_action": "alert",
    "recommended_action": "alert",
    "parameters_schema": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "command_categories": {
          "type": "array",
          "uniqueItems": true,
          "items": {
            "enum": ["network_transfer", "privilege", "permission_change", "namespace_mount", "account_persistence", "destructive", "security_control"]
          }
        },
        "require_resolved_executable": {"type": "boolean"}
      }
    },
    "default_parameters": {
      "command_categories": ["network_transfer", "privilege", "permission_change", "namespace_mount", "account_persistence", "destructive", "security_control"],
      "require_resolved_executable": false
    },
    "required_evidence": ["actor.pid", "actor.ppid", "actor.start_ticks", "process.executable", "process.argv", "process.cwd", "outcome"],
    "allow_conditions": ["trusted_process_digest", "policy_exception", "approved_change_window"],
    "mitre": ["T1059", "T1105", "T1548", "T1562"],
    "immutable": true,
    "digest": "sha256:43e4e365124e4d895a27f8267e8ab424f0482f121794a812aea946231520e130"
  },
  {
    "id": "62000000-0000-4000-8000-000000000005",
    "rule_key": "AGB-BUILTIN-005",
    "rule_version": 1,
    "name": "提权行为",
    "description": "检测智能体进程尝试或成功获得高于基线的 UID、GID 或 capability，并区分 attempted、succeeded 与 inconclusive。",
    "source": "builtin",
    "engine": "agent_and_dc",
    "categories": ["identity", "process"],
    "default_enabled": true,
    "default_severity": "high",
    "default_action": "alert",
    "recommended_action": "alert",
    "parameters_schema": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "alert_on_failed_attempt": {"type": "boolean"},
        "host_root_severity": {"enum": ["high", "critical"]},
        "unexpected_capability_severity": {"enum": ["high", "critical"]}
      }
    },
    "default_parameters": {"alert_on_failed_attempt": true, "host_root_severity": "critical", "unexpected_capability_severity": "high"},
    "required_evidence": ["actor.pid", "actor.ppid", "actor.start_ticks", "identity.before", "identity.after", "identity.user_namespace", "outcome"],
    "allow_conditions": ["profile_expected_identity_transition", "container_user_namespace_root", "policy_exception"],
    "mitre": ["T1068", "T1548"],
    "immutable": true,
    "digest": "sha256:63ce19628fc8285ded19f9609ec93770341c14e24e47680e95aff3cec4d775f1"
  }
]
$agent_guard_rules$::jsonb
    ) AS seed(
        id UUID,
        rule_key TEXT,
        rule_version BIGINT,
        name TEXT,
        description TEXT,
        source TEXT,
        engine TEXT,
        categories JSONB,
        default_enabled BOOLEAN,
        default_severity TEXT,
        default_action TEXT,
        recommended_action TEXT,
        parameters_schema JSONB,
        default_parameters JSONB,
        required_evidence JSONB,
        allow_conditions JSONB,
        mitre JSONB,
        immutable BOOLEAN,
        digest TEXT
    )
)
INSERT INTO agent_behavior_rule_definitions (
    id,
    rule_key,
    rule_version,
    name,
    description,
    source,
    engine,
    categories,
    default_enabled,
    default_severity,
    default_action,
    recommended_action,
    parameters_schema,
    default_parameters,
    required_evidence,
    allow_conditions,
    mitre,
    immutable,
    digest
)
SELECT
    id,
    rule_key,
    rule_version,
    name,
    description,
    source,
    engine,
    categories,
    default_enabled,
    default_severity,
    default_action,
    recommended_action,
    parameters_schema,
    default_parameters,
    required_evidence,
    allow_conditions,
    mitre,
    immutable,
    digest
FROM builtin_rule_seed
ON CONFLICT (rule_key, rule_version) DO NOTHING;

WITH builtin_profile_seed AS (
    SELECT *
    FROM jsonb_to_recordset(
$agent_guard_profiles$
[
  {
    "id": "62000000-0000-4000-8000-000000000101",
    "profile_key": "codex-linux",
    "profile_version": 1,
    "agent_type": "codex",
    "display_name": "Codex",
    "source": "builtin",
    "sandbox_family": "linux_namespace",
    "controller_match": [
      {"exe_basenames": ["codex"], "cmdline_tokens": ["codex"], "evidence_weight": 60},
      {"config_paths": [".codex/config.toml"], "evidence_weight": 40}
    ],
    "worker_match": [
      {"exe_basenames": ["codex-linux-sandbox", "bwrap"], "ancestor_basenames": ["codex"]},
      {"namespace_helper": true, "required_namespace_changes": ["mnt", "pid"]}
    ],
    "backend_detectors": [
      {"backend": "linux_namespace", "signals": ["codex-linux-sandbox", "bubblewrap", "namespace_tuple"]}
    ],
    "isolation_expectation": {
      "namespaces": ["mnt", "pid", "user", "net"],
      "require_no_new_privs": true,
      "seccomp": "profile_or_filter",
      "controller_outside_worker_namespace": true
    },
    "default_escape_rules": ["access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"],
    "digest": "sha256:5e2058f4656ea4d7540ac1a662b4806edcc46aa9a860ac38ca65c1bb27deb629",
    "enabled": true
  },
  {
    "id": "62000000-0000-4000-8000-000000000102",
    "profile_key": "openclaw-linux",
    "profile_version": 1,
    "agent_type": "openclaw",
    "display_name": "OpenClaw",
    "source": "builtin",
    "sandbox_family": "local_process_tree",
    "controller_match": [
      {"exe_basenames": ["openclaw"], "cmdline_tokens": ["openclaw"], "evidence_weight": 60},
      {"config_paths": [".openclaw/config.json"], "evidence_weight": 40}
    ],
    "worker_match": [
      {"ancestor_basenames": ["openclaw"], "fork_descendant": true},
      {"container_labels": ["openclaw"], "backend_required": "docker"}
    ],
    "backend_detectors": [
      {"backend": "local", "signals": ["sandbox_off", "local_backend"]},
      {"backend": "docker", "signals": ["docker_request", "container_id", "container_label", "cgroup"]},
      {"backend": "ssh", "signals": ["ssh_backend", "remote_execution_id"]},
      {"backend": "openshell", "signals": ["openshell_backend", "remote_execution_id"]}
    ],
    "isolation_expectation": {
      "local": {"coverage": "no_isolation"},
      "docker": {"family": "oci_container", "require_container_cgroup": true},
      "ssh": {"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"},
      "openshell": {"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"}
    },
    "default_escape_rules": ["access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"],
    "digest": "sha256:e6f916a2eb9b4fab6efd72f539efa7d7c9d51ab2f2d14255a55689249b9cfb79",
    "enabled": true
  },
  {
    "id": "62000000-0000-4000-8000-000000000103",
    "profile_key": "hermes-linux",
    "profile_version": 1,
    "agent_type": "hermes",
    "display_name": "Hermes",
    "source": "builtin",
    "sandbox_family": "local_process_tree",
    "controller_match": [
      {"exe_basenames": ["hermes"], "cmdline_tokens": ["hermes"], "evidence_weight": 60},
      {"exe_basenames": ["python", "python3"], "cmdline_tokens": ["hermes"], "config_paths": [".hermes/config.yaml"], "evidence_weight": 40}
    ],
    "worker_match": [
      {"ancestor_cmdline_tokens": ["hermes"], "fork_descendant": true},
      {"container_labels": ["hermes"], "backend_required": "docker"}
    ],
    "backend_detectors": [
      {"backend": "local", "signals": ["terminal_local"]},
      {"backend": "docker", "signals": ["docker_request", "container_id", "cgroup"]},
      {"backend": "singularity", "signals": ["singularity_process", "namespace_tuple"]},
      {"backend": "ssh", "signals": ["ssh_backend", "remote_execution_id"]},
      {"backend": "modal", "signals": ["modal_backend", "remote_execution_id"]},
      {"backend": "daytona", "signals": ["daytona_backend", "remote_execution_id"]},
      {"backend": "openshell", "signals": ["whole_process_wrapper", "remote_execution_id"]}
    ],
    "isolation_expectation": {
      "local": {"coverage": "no_isolation"},
      "docker": {"family": "oci_container", "require_container_cgroup": true},
      "singularity": {"family": "oci_container", "require_namespace_baseline": true},
      "remote": {"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"},
      "whole_process_wrapper": {"family": "whole_process_container"}
    },
    "default_escape_rules": ["access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"],
    "digest": "sha256:eccaf4fdc6287ff8cfb74e03c3c15aa86304d32d2f2401794d7f31b6fbfb9166",
    "enabled": true
  },
  {
    "id": "62000000-0000-4000-8000-000000000104",
    "profile_key": "claude-code-linux",
    "profile_version": 1,
    "agent_type": "claude-code",
    "display_name": "Claude Code",
    "source": "builtin",
    "sandbox_family": "local_process_tree",
    "controller_match": [
      {"exe_basenames": ["claude"], "cmdline_tokens": ["claude"], "evidence_weight": 60},
      {"config_paths": [".claude/settings.json"], "evidence_weight": 40}
    ],
    "worker_match": [
      {"ancestor_basenames": ["claude"], "fork_descendant": true}
    ],
    "backend_detectors": [
      {"backend": "local", "signals": ["terminal_local"]},
      {"backend": "ssh", "signals": ["ssh_backend", "remote_execution_id"]}
    ],
    "isolation_expectation": {
      "local": {"coverage": "no_isolation"},
      "ssh": {"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"}
    },
    "default_escape_rules": ["access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"],
    "digest": "sha256:94eb603baadec817c6e03857064fbe809aa5da42d612d9dd7e8b486f66cb63a7",
    "enabled": true
  },
  {
    "id": "62000000-0000-4000-8000-000000000105",
    "profile_key": "opencode-linux",
    "profile_version": 1,
    "agent_type": "opencode",
    "display_name": "OpenCode",
    "source": "builtin",
    "sandbox_family": "local_process_tree",
    "controller_match": [
      {"exe_basenames": ["opencode"], "cmdline_tokens": ["opencode"], "evidence_weight": 60},
      {"config_paths": [".config/opencode/opencode.json"], "evidence_weight": 40}
    ],
    "worker_match": [
      {"ancestor_basenames": ["opencode"], "fork_descendant": true}
    ],
    "backend_detectors": [
      {"backend": "local", "signals": ["terminal_local"]},
      {"backend": "ssh", "signals": ["ssh_backend", "remote_execution_id"]}
    ],
    "isolation_expectation": {
      "local": {"coverage": "no_isolation"},
      "ssh": {"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"}
    },
    "default_escape_rules": ["access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"],
    "digest": "sha256:b0fff61d935a97de75d5e90c658248201117608d256cd9be3f9a30d1ee3a34c2",
    "enabled": true
  },
  {
    "id": "62000000-0000-4000-8000-000000000106",
    "profile_key": "gemini-cli-linux",
    "profile_version": 1,
    "agent_type": "gemini-cli",
    "display_name": "Gemini CLI",
    "source": "builtin",
    "sandbox_family": "local_process_tree",
    "controller_match": [
      {"exe_basenames": ["gemini"], "cmdline_tokens": ["gemini"], "evidence_weight": 60},
      {"config_paths": [".gemini/settings.json"], "evidence_weight": 40}
    ],
    "worker_match": [
      {"ancestor_basenames": ["gemini"], "fork_descendant": true}
    ],
    "backend_detectors": [
      {"backend": "local", "signals": ["terminal_local"]},
      {"backend": "ssh", "signals": ["ssh_backend", "remote_execution_id"]}
    ],
    "isolation_expectation": {
      "local": {"coverage": "no_isolation"},
      "ssh": {"family": "remote_sandbox", "coverage_without_sensor": "remote_unobservable"}
    },
    "default_escape_rules": ["access_outside_workspace", "network_boundary_violation", "access_container_runtime_socket", "process_boundary_operation"],
    "digest": "sha256:300f72f233925ac36203a8a6d6ad4d8aa3247b93cf03474e8cede761315f5f66",
    "enabled": true
  }
]
$agent_guard_profiles$::jsonb
    ) AS seed(
        id UUID,
        profile_key TEXT,
        profile_version BIGINT,
        agent_type TEXT,
        display_name TEXT,
        source TEXT,
        sandbox_family TEXT,
        controller_match JSONB,
        worker_match JSONB,
        backend_detectors JSONB,
        isolation_expectation JSONB,
        default_escape_rules JSONB,
        digest TEXT,
        enabled BOOLEAN
    )
)
INSERT INTO agent_guard_adapter_profiles (
    id,
    profile_key,
    profile_version,
    agent_type,
    display_name,
    source,
    sandbox_family,
    controller_match,
    worker_match,
    backend_detectors,
    isolation_expectation,
    default_escape_rules,
    digest,
    enabled
)
SELECT
    id,
    profile_key,
    profile_version,
    agent_type,
    display_name,
    source,
    sandbox_family,
    controller_match,
    worker_match,
    backend_detectors,
    isolation_expectation,
    default_escape_rules,
    digest,
    enabled
FROM builtin_profile_seed
ON CONFLICT (profile_key, profile_version) DO NOTHING;
