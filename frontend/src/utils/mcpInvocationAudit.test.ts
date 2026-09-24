import { describe, expect, it } from 'vitest'
import type { MCPInvocation } from '@/types/mcpAggregation'
import { groupMCPInvocations } from './mcpInvocationAudit'

function invocation(overrides: Partial<MCPInvocation>): MCPInvocation {
  return {
    id: crypto.randomUUID(),
    client_id: 'client-1',
    client_key: 'codex-aegis',
    client_name: 'Codex',
    server_id: 'server-1',
    server_name: 'Aegis Local MCP',
    tool_revision_id: 'tool-revision-1',
    tool_alias: 'list_hosts',
    tool_enabled: true,
    status: 'succeeded',
    policy_decision: 'allow',
    created_at: '2026-08-11T06:00:00Z',
    ...overrides,
  }
}

describe('MCP invocation audit grouping', () => {
  it('groups calls by service, then tool and calling Client', () => {
    const groups = groupMCPInvocations([
      invocation({ id: 'latest', created_at: '2026-08-11T06:02:00Z' }),
      invocation({ id: 'older', created_at: '2026-08-11T06:01:00Z' }),
      invocation({ id: 'health', tool_alias: 'get_aegis_health', client_id: 'client-2', client_key: 'ops-agent' }),
      invocation({ id: 'remote', server_id: 'server-2', server_name: 'Remote MCP' }),
    ])

    expect(groups).toHaveLength(2)
    expect(groups[0].serverName).toBe('Aegis Local MCP')
    expect(groups[0].callCount).toBe(3)
    expect(groups[0].tools).toHaveLength(2)
    expect(groups[0].tools[0].clients[0]).toMatchObject({
      clientKey: 'codex-aegis',
      callCount: 2,
      lastInvocationId: 'latest',
    })
  })

  it('keeps pre-call authorization rule IDs visible for blocked invocations', () => {
    const groups = groupMCPInvocations([invocation({
      id: 'blocked-host-a', status: 'blocked', policy_decision: 'deny',
      authorization_outcome: 'deny', authorization_reason_code: 'POLICY_DENIED',
      authorization_deny_rule_ids: ['host.query.host_a_denied'],
    })])

    expect(groups[0].tools[0].clients[0]).toMatchObject({
      lastStatus: 'blocked',
      lastPolicyDecision: 'deny',
      lastAuthorizationDenyRuleIDs: ['host.query.host_a_denied'],
    })
  })
})
