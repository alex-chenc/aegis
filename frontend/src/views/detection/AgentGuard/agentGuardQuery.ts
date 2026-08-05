import type { LocationQuery, LocationQueryRaw } from 'vue-router'
import type {
	AgentBehaviorSession,
	AgentGuardDetailTab,
	AgentGuardInstanceQuery,
	AgentGuardListFilters,
	AgentRuntimeInstance,
} from '@/types/agentGuard'

export const DEFAULT_AGENT_GUARD_FILTERS: AgentGuardListFilters = {
  host_id: '',
  agent_types: [],
  runtime_status: '',
  coverage: '',
  isolation_type: '',
  keyword: '',
}

export interface AgentGuardDetailQuery {
  assetId: string
  scopeKey: string
  instanceId: string
  sessionId: string
  findingId: string
  eventId: string
  tab: AgentGuardDetailTab
}

export interface ParsedAgentGuardQuery {
  filters: AgentGuardListFilters
  page: number
  pageSize: number
  detail: AgentGuardDetailQuery
}

export function buildAgentGuardDetailInstanceQuery(
  detail: Pick<AgentGuardDetailQuery, 'assetId' | 'scopeKey' | 'instanceId'>,
): AgentGuardInstanceQuery {
  return {
    asset_ids: detail.assetId ? [detail.assetId] : undefined,
    agent_scope_key: detail.scopeKey || undefined,
    instance_ids: detail.instanceId ? [detail.instanceId] : undefined,
    page: 1,
    page_size: 100,
  }
}

// A newly discovered running instance may not have its first session and
// behavior facts persisted yet. Prefer an instance that already has a session
// when opening the aggregate agent detail, otherwise the drawer looks empty
// even though older instances in the same scope contain the data.
export function selectPreferredAgentGuardInstance(
	instances: AgentRuntimeInstance[],
	sessions: AgentBehaviorSession[],
): AgentRuntimeInstance | undefined {
	const sessionInstanceIDs = new Set(sessions.map(session => session.instance_id))
	return [...instances].sort((left, right) => {
		const rightHasSession = sessionInstanceIDs.has(right.id) ? 1 : 0
		const leftHasSession = sessionInstanceIDs.has(left.id) ? 1 : 0
		if (rightHasSession !== leftHasSession) return rightHasSession - leftHasSession
		const riskDelta = (right.high_risk_finding_count || 0) - (left.high_risk_finding_count || 0)
		if (riskDelta !== 0) return riskDelta
		return String(right.last_seen_at || '').localeCompare(String(left.last_seen_at || ''))
	})[0]
}

const DETAIL_QUERY_KEYS = [
  'asset_id',
  'agent_scope_key',
  'instance_id',
  'session_id',
  'finding_id',
  'event_id',
  'detail_tab',
] as const

function scalar(value: unknown): string {
  if (Array.isArray(value)) return value[0] == null ? '' : String(value[0])
  return value == null ? '' : String(value)
}

function positiveInt(value: unknown, fallback: number): number {
  const parsed = Number.parseInt(scalar(value), 10)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback
}

function pageSize(value: unknown): number {
  const parsed = positiveInt(value, 20)
  return [10, 20, 50, 100].includes(parsed) ? parsed : 20
}

function stringList(value: unknown): string[] {
  const values = Array.isArray(value) ? value : [value]
  return values
    .flatMap(item => scalar(item).split(','))
    .map(item => item.trim())
    .filter(Boolean)
}

export function parseAgentGuardQuery(
  query: LocationQuery | Record<string, unknown>,
): ParsedAgentGuardQuery {
  const tab = scalar(query.detail_tab)
  const findingId = scalar(query.finding_id)
  const eventId = scalar(query.event_id)
  return {
    filters: {
      host_id: scalar(query.host_id),
      agent_types: stringList(query.agent_types),
      runtime_status: scalar(query.status),
      coverage: scalar(query.coverage),
      isolation_type: scalar(query.isolation_type),
      keyword: scalar(query.keyword),
    },
    page: positiveInt(query.page, 1),
    pageSize: pageSize(query.page_size),
    detail: {
      assetId: scalar(query.asset_id),
      scopeKey: scalar(query.agent_scope_key),
      instanceId: scalar(query.instance_id),
      sessionId: scalar(query.session_id),
      findingId,
      eventId,
      tab: tab === 'analysis' || (!tab && (findingId || eventId)) ? 'analysis' : 'panorama',
    },
  }
}

export function serializeAgentGuardListQuery(
  filters: AgentGuardListFilters,
  page: number,
  pageSize: number,
): LocationQueryRaw {
  const query: LocationQueryRaw = {
    page: String(page),
    page_size: String(pageSize),
  }
  if (filters.host_id) query.host_id = filters.host_id
  if (filters.agent_types.length) query.agent_types = filters.agent_types.join(',')
  if (filters.runtime_status) query.status = filters.runtime_status
  if (filters.coverage) query.coverage = filters.coverage
  if (filters.isolation_type) query.isolation_type = filters.isolation_type
  if (filters.keyword) query.keyword = filters.keyword
  return query
}

export function withAgentGuardDetailQuery(
  current: LocationQuery | LocationQueryRaw | Record<string, unknown>,
  detail: Pick<AgentGuardDetailQuery, 'tab'> & Partial<AgentGuardDetailQuery>,
): LocationQueryRaw {
  return {
    ...current,
    asset_id: detail.assetId || undefined,
    agent_scope_key: detail.scopeKey || undefined,
    instance_id: detail.instanceId || undefined,
    session_id: detail.sessionId || undefined,
    finding_id: detail.findingId || undefined,
    event_id: detail.eventId || undefined,
    detail_tab: detail.tab,
  } as LocationQueryRaw
}

export function clearAgentGuardDetailQuery(
  current: LocationQuery | LocationQueryRaw | Record<string, unknown>,
): LocationQueryRaw {
  const next = { ...current } as Record<string, unknown>
  DETAIL_QUERY_KEYS.forEach(key => {
    delete next[key]
  })
  return next as LocationQueryRaw
}
