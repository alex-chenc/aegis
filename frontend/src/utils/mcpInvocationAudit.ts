import type { MCPInvocation } from '@/types/mcpAggregation'

export interface MCPInvocationClientGroup {
  key: string
  clientId?: string
  clientKey: string
  clientName: string
  callCount: number
  lastInvocationId: string
  lastStatus: string
  lastPolicyDecision?: string
  lastCalledAt: string
  toolEnabled: boolean
}

export interface MCPInvocationToolGroup {
  alias: string
  callCount: number
  clients: MCPInvocationClientGroup[]
}

export interface MCPInvocationServiceGroup {
  serverId: string
  serverName: string
  callCount: number
  tools: MCPInvocationToolGroup[]
}

export function groupMCPInvocations(items: MCPInvocation[]): MCPInvocationServiceGroup[] {
  const services = new Map<string, {
    group: MCPInvocationServiceGroup
    tools: Map<string, { group: MCPInvocationToolGroup; clients: Map<string, MCPInvocationClientGroup> }>
  }>()

  for (const invocation of items) {
    const serverKey = invocation.server_id || `unknown:${invocation.tool_revision_id || invocation.tool_alias}`
    let service = services.get(serverKey)
    if (!service) {
      service = {
        group: {
          serverId: invocation.server_id,
          serverName: invocation.server_name || '—',
          callCount: 0,
          tools: [],
        },
        tools: new Map(),
      }
      services.set(serverKey, service)
    }
    service.group.callCount += 1

    let tool = service.tools.get(invocation.tool_alias)
    if (!tool) {
      tool = {
        group: { alias: invocation.tool_alias, callCount: 0, clients: [] },
        clients: new Map(),
      }
      service.tools.set(invocation.tool_alias, tool)
      service.group.tools.push(tool.group)
    }
    tool.group.callCount += 1

    const clientKey = invocation.client_id || invocation.client_key || `unknown:${invocation.id}`
    let client = tool.clients.get(clientKey)
    if (!client) {
      client = {
        key: `${serverKey}:${invocation.tool_alias}:${clientKey}`,
        clientId: invocation.client_id,
        clientKey: invocation.client_key || '—',
        clientName: invocation.client_name || invocation.client_key || '—',
        callCount: 0,
        lastInvocationId: invocation.id,
        lastStatus: invocation.status,
        lastPolicyDecision: invocation.policy_decision,
        lastCalledAt: invocation.created_at,
        toolEnabled: invocation.tool_enabled && Boolean(invocation.tool_revision_id),
      }
      tool.clients.set(clientKey, client)
      tool.group.clients.push(client)
    }
    client.callCount += 1
  }

  return Array.from(services.values(), item => item.group)
}
