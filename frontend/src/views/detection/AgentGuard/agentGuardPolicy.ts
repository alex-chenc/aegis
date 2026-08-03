import type { AgentGuardPolicyDraftRequest } from '@/types/agentGuard'

export function buildAgentGuardCollectionPolicy(
  toolAdapterEnabled: boolean,
): AgentGuardPolicyDraftRequest['collection'] {
  return {
    categories: [
      'process', 'file', 'network', 'identity', 'isolation', 'kernel',
      ...(toolAdapterEnabled ? ['tool'] : []),
    ],
    tool_adapter_enabled: toolAdapterEnabled,
    command_argv: 'redacted',
    file_content: 'disabled',
    network_content: 'disabled',
    aggregation: { file_read_write_seconds: 2 },
  }
}
