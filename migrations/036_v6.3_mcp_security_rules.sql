-- V6.3 MCP deterministic security rules and historical verdict projection.
INSERT INTO mcp_rule_definitions (id, rule_key, version, name, phase, severity, definition, digest, enabled)
VALUES
    ('00000000-0000-0000-0000-000000006301', 'block_l4_tool_call', 1, 'L4 工具调用阻断', 'pre', 'critical',
     '{"matcher":"tool_risk_at_least","threshold":"l4","action":"block"}'::jsonb,
     '38235673309fef027fe4e81161e83a4131de73c9d69baaf78cfac5e6fccc346d', true),
    ('00000000-0000-0000-0000-000000006302', 'block_sensitive_output_keys', 1, '敏感结果字段阻断', 'post', 'critical',
     '{"matcher":"sensitive_output_keys","keys":["password","secret","token","authorization","private_key","access_key","credential"],"action":"block"}'::jsonb,
     '6a530a2a1d85b9578ee2adc22c9b6bf7516c1809444671e7707e6ebbdc1684f0', true),
    ('00000000-0000-0000-0000-000000006303', 'audit_oversized_response', 1, '超大结果审计', 'post', 'medium',
     '{"matcher":"response_size_bytes","threshold":524288,"action":"audit"}'::jsonb,
     'd71e35104465010399c1ec0f7ce68c9b6a6aa8c9420da876fd3fcca78fffb02a', true),
    ('00000000-0000-0000-0000-000000006304', 'audit_upstream_failure', 1, '上游调用失败审计', 'post', 'medium',
     '{"matcher":"call_failed","action":"audit"}'::jsonb,
     'b07ea45492568cea99c88e0a15f9e54b10a38ef3d4254ea3042ea8c917c3acff', true),
    ('00000000-0000-0000-0000-000000006305', 'block_sensitive_input_keys', 1, '敏感输入字段阻断', 'pre', 'critical',
     '{"matcher":"sensitive_input_keys","keys":["password","secret","token","authorization","private_key","access_key","credential"],"action":"block"}'::jsonb,
     'eb3b8302771a2f7926c7498320813cf865c8b4cdd27ab023e34cd2d870cd9cd9', true),
    ('00000000-0000-0000-0000-000000006306', 'block_injection_input', 1, '注入型输入阻断', 'pre', 'critical',
     '{"matcher":"input_patterns","patterns":["../","..\\","\r\n","$(","`","; rm "," union select "," or 1=1","drop table","ignore previous instructions"],"action":"block"}'::jsonb,
     'a1cc0b727406ffc2380ed249d7a0e126a43c5665fb29adbe14ea51629ceb9283', true),
    ('00000000-0000-0000-0000-000000006307', 'block_prompt_injection_output', 1, '工具结果提示词注入阻断', 'post', 'critical',
     '{"matcher":"output_patterns","patterns":["ignore previous instructions","ignore all previous instructions","reveal the system prompt","system message override","developer message override"],"action":"block"}'::jsonb,
     '25de4e96f0d7a0f816a285e44b1e5a8755ba17c58c53cc0be341cab4c49cdc8b', true)
ON CONFLICT (rule_key, version) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_mcp_rule_definitions_enabled_phase
    ON mcp_rule_definitions(enabled, phase, created_at DESC);

INSERT INTO mcp_security_verdicts (
    id, invocation_id, deterministic_severity, ai_verdict, overall_risk, evidence, updated_at
)
SELECT
    gen_random_uuid(), invocation.id,
    CASE WHEN invocation.status = 'failed' THEN 'medium' ELSE 'low' END,
    'not_run',
    CASE WHEN invocation.status = 'failed' THEN 'medium' ELSE 'low' END,
    jsonb_build_array(jsonb_build_object(
        'type', 'historical_projection',
        'reason', 'historical_payload_unavailable',
        'status', invocation.status
    )),
    COALESCE(invocation.completed_at, invocation.created_at, now())
FROM mcp_invocations AS invocation
LEFT JOIN mcp_security_verdicts AS verdict ON verdict.invocation_id = invocation.id
WHERE verdict.id IS NULL;
