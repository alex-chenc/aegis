import request from './index'
import type { AgentConversationItem, AgentConversationSession, AgentSessionAIResult, AgentSessionPage, AgentSessionRuleHit, AgentSessionRulePage } from '@/types/agentSession'

export function listAgentSessionRules(): Promise<AgentSessionRulePage> {
  return request.get('/agent-guard/session-awareness/rules')
}

export function listAgentSessions(params: { page?: number; page_size?: number; host_id?: string; agent_type?: string; risk_level?: string } = {}): Promise<AgentSessionPage> {
  return request.get('/agent-guard/session-awareness/sessions', { params })
}

export function collectAgentSessions(hostId: string, agentType: string): Promise<{
  host_id: string
  agent_type: string
  accepted: boolean
  status: string
  message: string
}> {
  return request.post(`/agent-guard/session-awareness/agents/${encodeURIComponent(hostId)}/collect`, {}, {
    params: { agent_type: agentType },
  })
}

export function getAgentSession(id: string): Promise<AgentConversationSession> {
  return request.get(`/agent-guard/session-awareness/sessions/${encodeURIComponent(id)}`)
}

export function listAgentSessionItems(id: string): Promise<{ session: AgentConversationSession; items: AgentConversationItem[] }> {
  return request.get(`/agent-guard/session-awareness/sessions/${encodeURIComponent(id)}/items`)
}

export function listAgentSessionRuleHits(id: string): Promise<AgentSessionRuleHit[]> {
  return request.get(`/agent-guard/session-awareness/sessions/${encodeURIComponent(id)}/rule-hits`)
}

export function getAgentSessionAIAnalysis(id: string): Promise<AgentSessionAIResult> {
  return request.get(`/agent-guard/session-awareness/sessions/${encodeURIComponent(id)}/ai-analysis`)
}

export function runAgentSessionAIAnalysis(id: string): Promise<AgentSessionAIResult> {
  return request.post(`/agent-guard/session-awareness/sessions/${encodeURIComponent(id)}/ai-analysis`)
}
