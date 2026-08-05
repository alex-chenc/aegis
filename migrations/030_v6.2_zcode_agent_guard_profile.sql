-- Aegis V6.2 follow-up: Zcode native Hook/Profile support.
-- Keep the original immutable catalog migration unchanged; this is repeatable
-- for hosts that already applied 029_v6.2_agent_guard.sql.

INSERT INTO agent_guard_adapter_profiles (
    id, profile_key, profile_version, agent_type, display_name, source,
    sandbox_family, controller_match, worker_match, backend_detectors,
    isolation_expectation, default_escape_rules, digest, enabled
)
VALUES (
    '62000000-0000-4000-8000-000000000107',
    'zcode-linux',
    1,
    'zcode',
    'Zcode',
    'builtin',
    'local_process_tree',
    '[
      {"exe_basenames":["zcode","zcode-cli"],"cmdline_tokens":["zcode"],"evidence_weight":60},
      {"config_paths":[".zcode/cli/config.json"],"evidence_weight":40}
    ]'::jsonb,
    '[{"ancestor_basenames":["zcode","zcode-cli"],"fork_descendant":true}]'::jsonb,
    '[
      {"backend":"local","signals":["terminal_local"]},
      {"backend":"ssh","signals":["ssh_backend","remote_execution_id"]}
    ]'::jsonb,
    '{
      "local":{"coverage":"no_isolation"},
      "ssh":{"family":"remote_sandbox","coverage_without_sensor":"remote_unobservable"}
    }'::jsonb,
    '["access_container_runtime_socket","join_external_namespace","write_cgroupfs","credential_or_capability_gain","isolation_baseline_drift"]'::jsonb,
    'sha256:bcb65be77f138f3f0f5d6de4ac2d017b43876f9cd98a0d0a7c55bd0f8dd5389c',
    TRUE
)
ON CONFLICT (profile_key, profile_version) DO NOTHING;
