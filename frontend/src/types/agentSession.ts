export interface AgentConversationSession {
  id: string
  host_id: string
  agent_type: 'claude-code' | 'codex' | string
  source_mode: string
  source_subject_uid: number
  external_session_id: string
  project_digest?: string
  title?: string
  model?: string
  state: string
  last_seen_at?: string
  item_count: number
  prompt_count: number
  assistant_count: number
  tool_call_count: number
  estimated_input_tokens: number
  estimated_output_tokens: number
  estimated_total_tokens: number
  token_estimate_method: string
  risk_level: string
  rule_hit_count: number
  ai_risk_score?: number
}

export interface AgentConversationItem {
  id: string
  item_id: string
  sequence: number
  item_type: string
  role?: string
  occurred_at?: string
  content_redacted?: string
  visibility: string
  redaction_applied: boolean
  total_tokens?: number
}

export interface AgentSessionRuleHit {
  id: string
  session_id: string
  item_id?: string
  item_sequence?: number
  rule_id?: string
  rule_key: string
  severity: string
  category: string
  evidence_digest?: string
  evidence_excerpt?: string
  status: string
  created_at?: string
}

export interface AgentSessionAIChunk {
  index: number
  start_sequence: number
  end_sequence: number
  item_sequences?: number[]
  input_token_estimate: number
  content?: string
  provider?: string
  model?: string
  status?: string
  usage?: { input: number; output: number; total: number }
}

export interface AgentSessionAIResult {
  run_id?: string
  status: string
  prompt_version?: string
  provider?: string
  model?: string
  summary?: string
  chunk_count: number
  chunks: AgentSessionAIChunk[]
}

export interface AgentSessionRule {
  rule_key: string
  rule_version?: number
  name: string
  source?: string
  engine?: string
  categories?: string[]
  default_enabled?: boolean
  default_severity?: string
  default_action?: string
  recommended_action?: string
  immutable?: boolean
  digest?: string
  description?: string
}

export interface AgentSessionRulePage {
  items: AgentSessionRule[]
  total: number
}

export interface AgentSessionPage {
  items: AgentConversationSession[]
  total: number
  page: number
  page_size: number
}
