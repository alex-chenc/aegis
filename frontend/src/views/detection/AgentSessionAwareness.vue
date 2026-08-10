<template>
  <div class="session-awareness page-shell">
    <section class="page-hero">
      <div>
        <h1>智能体会话感知</h1>
        <p>以资产 Agent 为入口，查看该 Agent 的会话，并按需静态采集 Claude Code 与 OpenAI Codex CLI 会话。</p>
      </div>
      <div class="hero-actions">
        <el-button type="primary" plain @click="rulesDialogVisible = true">内置安全规则</el-button>
        <el-button :loading="agentsLoading" @click="loadAgents">刷新 Agent</el-button>
      </div>
    </section>

    <div class="metric-grid">
      <div class="metric-card"><span>Agent 资产</span><strong>{{ agentTotal }}</strong></div>
      <div class="metric-card"><span>运行中 Agent</span><strong>{{ onlineAgents }}</strong></div>
      <div class="metric-card"><span>已采集会话</span><strong>{{ sessionTotal.toLocaleString() }}</strong></div>
      <div class="metric-card"><span>采集方式</span><strong>静态扫描</strong></div>
    </div>

    <el-card class="filter-card">
      <el-form inline @submit.prevent="loadAgents">
        <el-form-item label="Agent 类型">
          <el-select v-model="filters.agentType" clearable placeholder="全部" @change="loadAgents">
            <el-option label="Claude Code" value="claude-code" />
            <el-option label="OpenAI Codex CLI" value="codex" />
          </el-select>
        </el-form-item>
        <el-form-item label="关键词">
          <el-input v-model="filters.keyword" clearable placeholder="主机名或 Agent 名称" @keyup.enter="loadAgents" />
        </el-form-item>
        <el-button type="primary" @click="loadAgents">查询</el-button>
      </el-form>
    </el-card>

    <el-card>
      <template #header>
        <div class="table-header"><strong>Agent 列表</strong><span>数据来源：资产中心 ai_agent</span></div>
      </template>
      <el-table ref="agentTableRef" v-loading="agentsLoading" :data="agents" stripe row-key="agent_scope_key" @row-click="openAgent" @expand-change="handleAgentExpand">
        <el-table-column type="expand" width="48">
          <template #default="{ row }">
            <div class="expanded-sessions">
              <div class="expanded-sessions-head">
                <div class="stacked"><strong>会话列表</strong><span>{{ row.host?.hostname || '-' }} · {{ agentLabel(row.agent_type) }}</span></div>
                <el-button size="small" :loading="sessionLoadingFor(row)" @click.stop="loadAgentSessions(row)">刷新会话</el-button>
              </div>
              <el-alert title="采集为 Agent 本地静态 JSONL 扫描，不安装 Hook、不监听文件系统；入库内容已脱敏，隐藏推理不展示。" type="info" :closable="false" show-icon />
              <el-table v-loading="sessionLoadingFor(row)" :data="sessionRowsFor(row)" stripe class="session-table">
                <el-table-column prop="external_session_id" label="会话 ID" min-width="230" show-overflow-tooltip />
                <el-table-column label="消息" width="90" align="center"><template #default="{ row: session }">{{ session.item_count }}</template></el-table-column>
                <el-table-column label="Token 估算" width="140"><template #default="{ row: session }">{{ (session.estimated_total_tokens || 0).toLocaleString() }}</template></el-table-column>
                <el-table-column label="风险" width="100"><template #default="{ row: session }"><el-tag :type="riskType(session.risk_level)">{{ session.risk_level || 'unknown' }}</el-tag></template></el-table-column>
                <el-table-column label="最后发现" min-width="180"><template #default="{ row: session }">{{ formatTime(session.last_seen_at) }}</template></el-table-column>
                <el-table-column label="操作" width="120" fixed="right" align="center"><template #default="{ row: session }"><el-button link type="primary" @click.stop="openSession(session)">会话详情</el-button></template></el-table-column>
              </el-table>
              <el-empty v-if="!sessionLoadingFor(row) && sessionRowsFor(row).length === 0" :image-size="64" description="暂无已采集会话" />
              <el-pagination v-if="sessionTotalFor(row) > 0" :current-page="sessionPageFor(row)" class="pager" layout="total, prev, pager, next" :page-size="sessionPageSize" :total="sessionTotalFor(row)" @current-change="changeSessionPage(row, $event)" />
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Agent" min-width="190" fixed="left">
          <template #default="{ row }">
            <div class="stacked"><strong>{{ row.display_name || row.agent_type }}</strong><span>{{ agentLabel(row.agent_type) }}</span></div>
          </template>
        </el-table-column>
        <el-table-column label="主机" min-width="190">
          <template #default="{ row }"><div class="stacked"><strong>{{ row.host?.hostname || '-' }}</strong><span>{{ row.host?.ip || '-' }}</span></div></template>
        </el-table-column>
        <el-table-column label="Agent 状态" width="120">
          <template #default="{ row }"><el-tag :type="statusType(row.asset_status)" effect="plain">{{ statusLabel(row.asset_status) }}</el-tag></template>
        </el-table-column>
        <el-table-column label="会话数" width="90" align="center"><template #default="{ row }">{{ row.session_count || 0 }}</template></el-table-column>
        <el-table-column label="风险发现" width="110" align="center"><template #default="{ row }"><strong :class="{ danger: row.high_risk_finding_count > 0 }">{{ row.high_risk_finding_count || 0 }}</strong></template></el-table-column>
        <el-table-column label="资产更新时间" min-width="170"><template #default="{ row }">{{ formatTime(row.asset_collected_at) }}</template></el-table-column>
        <el-table-column label="操作" width="230" fixed="right" align="center">
          <template #default="{ row }">
            <el-button link type="success" :disabled="!canCollect(row)" :loading="collectingHost === row.host.id" @click.stop="collect(row)">采集会话</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination v-model:current-page="agentPage" class="pager" layout="total, sizes, prev, pager, next" :page-size="agentPageSize" :page-sizes="[10, 20, 50]" :total="agentTotal" @current-change="loadAgents" @size-change="changeAgentPageSize" />
    </el-card>

    <BuiltinRuleCatalogDialog
      :visible="rulesDialogVisible"
      title="内置会话安全规则"
      description="规则分析使用系统内置的只读规则，规则定义、版本和摘要由服务端统一提供。"
      :rules="sessionRules"
      :loading="rulesLoading"
      @close="rulesDialogVisible = false"
    />

    <el-drawer v-model="sessionDialog" title="会话详情" direction="rtl" size="min(1180px, 96vw)" class="session-detail-drawer">
      <template v-if="selectedSession">
        <div class="conversation-summary">
          <div class="summary-primary">
            <el-tag round effect="plain">{{ agentLabel(selectedSession.agent_type) }}</el-tag>
            <div class="summary-id"><span>会话 ID</span><strong :title="selectedSession.external_session_id">{{ selectedSession.external_session_id }}</strong></div>
          </div>
          <div class="summary-token"><span>估算 Token</span><strong>{{ (selectedSession.estimated_total_tokens || 0).toLocaleString() }}</strong></div>
        </div>
        <div class="conversation-notice"><span class="notice-dot" />仅展示 Agent 静态解析后的脱敏内容，开发者上下文和工具事件默认折叠。</div>

        <div v-if="detailItems.length" class="conversation-scroll">
          <template v-for="item in detailItems" :key="item.id">
            <div class="conversation-entry" :data-session-sequence="item.sequence" :class="{ 'has-evidence': itemEvidenceCount(item) > 0, 'security-active': isActiveSecuritySequence(item.sequence) }">
              <div class="conversation-main">
                <div v-if="itemKind(item) === 'user' || itemKind(item) === 'assistant'" class="chat-row" :class="itemKind(item)">
                  <div class="chat-avatar">{{ itemKind(item) === 'user' ? 'U' : 'AI' }}</div>
                  <div class="chat-bubble">
                    <div class="chat-meta"><span>{{ itemLabel(item) }}</span><span>#{{ item.sequence }}</span><time>{{ formatTime(item.occurred_at) }}</time></div>
                    <div class="chat-content">{{ itemContent(item) }}</div>
                  </div>
                </div>
                <details v-else class="conversation-event" :class="itemKind(item)">
                  <summary><span class="event-kind">{{ itemLabel(item) }}</span><span>#{{ item.sequence }}</span><time>{{ formatTime(item.occurred_at) }}</time><span class="event-expand">展开</span></summary>
                  <pre>{{ itemContent(item) }}</pre>
                </details>
              </div>
              <aside v-if="itemEvidenceCount(item) > 0" class="evidence-rail">
                <section v-if="ruleHitsFor(item).length" class="evidence-card rule-evidence">
                  <div class="evidence-card-heading"><strong>命中策略</strong><el-tag type="danger" size="small">{{ ruleHitsFor(item).length }} 条</el-tag></div>
                  <article v-for="hit in ruleHitsFor(item)" :key="hit.id" class="evidence-item" :class="{ 'security-card-active': isActiveSecurityTarget(`rule:${hit.id}`) }" @click="focusSecurityTarget(`rule:${hit.id}`)">
                    <div class="evidence-item-title"><el-tag :type="severityType(hit.severity)" size="small">{{ severityLabel(hit.severity) }}</el-tag><strong>{{ ruleName(hit.rule_key) }}</strong></div>
                    <code>{{ hit.rule_key }}</code>
                    <p>{{ hit.evidence_excerpt || '检测到规则匹配模式' }}</p>
                    <small>定位：会话消息 #{{ hit.item_sequence ?? item.sequence }}</small>
                  </article>
                </section>
                <section v-for="chunk in aiChunksFor(item)" :key="`ai-${chunk.index}`" class="evidence-card ai-evidence" :class="{ 'security-card-active': isActiveSecurityTarget(`ai:${chunk.index}`) }" @click="focusSecurityTarget(`ai:${chunk.index}`)">
                  <div class="evidence-card-heading"><strong>AI 分析 · 第 {{ chunk.index + 1 }} 段</strong><el-tag :type="aiRiskType(chunk)" size="small">{{ aiRiskLabel(chunk) }}</el-tag></div>
                  <p class="ai-evidence-summary">{{ aiSummary(chunk) }}</p>
                  <small>定位：会话消息 #{{ item.sequence }}（分段范围 #{{ chunk.start_sequence }}–#{{ chunk.end_sequence }}）</small>
                </section>
              </aside>
            </div>
          </template>
        </div>
        <el-empty v-else description="暂无可展示的脱敏会话内容" />

        <div v-if="securityTargets.length" class="security-navigation">
          <div class="security-navigation-copy">
            <strong>安全分析定位</strong>
            <span>{{ activeSecurityIndex >= 0 ? `${activeSecurityIndex + 1} / ${securityTargets.length} · ${activeSecurityTargetLabel}` : `共 ${securityTargets.length} 个分析位置` }}</span>
          </div>
          <el-button-group>
            <el-button size="small" :disabled="activeSecurityIndex <= 0" @click="navigateSecurity(-1)">上一个</el-button>
            <el-button size="small" :disabled="activeSecurityIndex >= securityTargets.length - 1" @click="navigateSecurity(1)">下一个</el-button>
          </el-button-group>
        </div>

        <div class="conversation-actions">
          <div class="action-summary"><span>{{ detailItems.length }} 条可见消息</span><span>{{ (selectedSession.estimated_total_tokens || 0).toLocaleString() }} tokens</span></div>
          <el-button type="primary" :loading="aiLoading" @click="runAI">运行 AI 分析</el-button>
        </div>
        <el-alert v-if="aiResult && aiResult.status !== 'not_run'" class="ai-overview" type="info" :closable="false" show-icon>
          <template #title>AI 分析已{{ aiResult.status === 'succeeded' ? '完成' : '生成' }}，结果已定位到对应会话消息</template>
          <div>共 {{ aiResult.chunk_count }} 个分析分段<span v-if="aiResult.provider"> · {{ aiResult.provider }}{{ aiResult.model ? ` / ${aiResult.model}` : '' }}</span><span v-if="aiResult.run_id"> · Run {{ aiResult.run_id }}</span></div>
        </el-alert>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { listAgentGuardAgents } from '@/api/agentGuard'
import { collectAgentSessions, getAgentSession, getAgentSessionAIAnalysis, listAgentSessionItems, listAgentSessionRuleHits, listAgentSessionRules, listAgentSessions, runAgentSessionAIAnalysis } from '@/api/agentSession'
import type { AgentGuardAgentSummary } from '@/types/agentGuard'
import type { AgentConversationItem, AgentConversationSession, AgentSessionAIChunk, AgentSessionAIResult, AgentSessionRule, AgentSessionRuleHit } from '@/types/agentSession'
import BuiltinRuleCatalogDialog from './AgentGuard/components/BuiltinRuleCatalogDialog.vue'

const agents = ref<AgentGuardAgentSummary[]>([])
const agentTotal = ref(0); const agentPage = ref(1); const agentPageSize = ref(20); const agentsLoading = ref(false)
const sessionTotal = ref(0); const sessionPageSize = 20
const agentTableRef = ref<{ toggleRowExpansion: (row: AgentGuardAgentSummary, expanded?: boolean) => void }>()
const expandedAgentKeys = ref<string[]>([])
const sessionRowsByAgent = reactive<Record<string, AgentConversationSession[]>>({})
const sessionTotalsByAgent = reactive<Record<string, number>>({})
const sessionPagesByAgent = reactive<Record<string, number>>({})
const sessionLoadingByAgent = reactive<Record<string, boolean>>({})
const collectingHost = ref('')
const selectedSession = ref<AgentConversationSession | null>(null); const sessionDialog = ref(false); const detailItems = ref<AgentConversationItem[]>([]); const ruleHits = ref<AgentSessionRuleHit[]>([]); const aiLoading = ref(false); const aiResult = ref<AgentSessionAIResult | null>(null)
const sessionRules = ref<AgentSessionRule[]>([]); const rulesLoading = ref(false); const rulesDialogVisible = ref(false)
const activeSecurityIndex = ref(-1)
const filters = reactive({ agentType: '', keyword: '' })
const onlineAgents = computed(() => agents.value.filter(item => item.asset_status === 'running').length)

type SecurityTarget = { key: string; sequence: number; label: string }
const securityTargets = computed<SecurityTarget[]>(() => {
  const targets: SecurityTarget[] = []
  for (const item of detailItems.value) {
    for (const hit of ruleHitsFor(item)) {
      targets.push({ key: `rule:${hit.id}`, sequence: item.sequence, label: `规则 · ${ruleName(hit.rule_key)}` })
    }
    for (const chunk of aiChunksFor(item)) {
      targets.push({ key: `ai:${chunk.index}`, sequence: item.sequence, label: `AI 分析 · 第 ${chunk.index + 1} 段` })
    }
  }
  return targets
})
const activeSecurityTargetLabel = computed(() => securityTargets.value[activeSecurityIndex.value]?.label || '')

async function loadAgents() {
  agentsLoading.value = true
  try {
    const result = await listAgentGuardAgents({ page: agentPage.value, page_size: agentPageSize.value, agent_types: filters.agentType ? [filters.agentType] : ['claude-code', 'codex'], keyword: filters.keyword || undefined })
    const items = await Promise.all(result.items.map(async item => {
      try {
        const sessions = await listAgentSessions({ page: 1, page_size: 1, host_id: item.host.id, agent_type: item.agent_type })
        return { ...item, session_count: sessions.total }
      } catch {
        return { ...item, session_count: 0 }
      }
    }))
    agents.value = items; agentTotal.value = result.total
    const summary = await listAgentSessions({ page: 1, page_size: 1 })
    sessionTotal.value = summary.total
  } finally { agentsLoading.value = false }
}

async function loadSessionRules() {
  rulesLoading.value = true
  try {
    sessionRules.value = (await listAgentSessionRules()).items || []
  } finally { rulesLoading.value = false }
}

function changeAgentPageSize(size: number) { agentPageSize.value = size; agentPage.value = 1; loadAgents() }
function agentKey(agent: AgentGuardAgentSummary) { return agent.agent_scope_key || `${agent.host.id}:${agent.agent_type}` }
function sessionRowsFor(agent: AgentGuardAgentSummary) { return sessionRowsByAgent[agentKey(agent)] || [] }
function sessionTotalFor(agent: AgentGuardAgentSummary) { return sessionTotalsByAgent[agentKey(agent)] || 0 }
function sessionPageFor(agent: AgentGuardAgentSummary) { return sessionPagesByAgent[agentKey(agent)] || 1 }
function sessionLoadingFor(agent: AgentGuardAgentSummary) { return Boolean(sessionLoadingByAgent[agentKey(agent)]) }
function openAgent(agent: AgentGuardAgentSummary) {
  const key = agentKey(agent)
  agentTableRef.value?.toggleRowExpansion(agent, !expandedAgentKeys.value.includes(key))
}
function handleAgentExpand(row: AgentGuardAgentSummary, expandedRows: AgentGuardAgentSummary[]) {
  const key = agentKey(row)
  const expanded = expandedRows.some(item => agentKey(item) === key)
  expandedAgentKeys.value = expanded
    ? Array.from(new Set([...expandedAgentKeys.value, key]))
    : expandedAgentKeys.value.filter(item => item !== key)
  if (expanded && !sessionRowsByAgent[key]) void loadAgentSessions(row)
}
async function loadAgentSessions(agent: AgentGuardAgentSummary, page = sessionPageFor(agent)) {
  const key = agentKey(agent)
  sessionLoadingByAgent[key] = true
  sessionPagesByAgent[key] = page
  try {
    const result = await listAgentSessions({ page, page_size: sessionPageSize, host_id: agent.host.id, agent_type: agent.agent_type })
    sessionRowsByAgent[key] = result.items
    sessionTotalsByAgent[key] = result.total
  } finally { sessionLoadingByAgent[key] = false }
}
function changeSessionPage(agent: AgentGuardAgentSummary, page: number) { void loadAgentSessions(agent, page) }
async function collect(agent: AgentGuardAgentSummary) {
  collectingHost.value = agent.host.id
  try {
    const result = await collectAgentSessions(agent.host.id, agent.agent_type)
    if (result.status === 'pending_reconnect') ElMessage.warning(result.message)
    else ElMessage.success('采集请求已下发，稍后刷新会话列表')
    window.setTimeout(() => {
      if (expandedAgentKeys.value.includes(agentKey(agent))) void loadAgentSessions(agent)
      void loadAgents()
    }, 1500)
  } finally {
    collectingHost.value = ''
  }
}
async function openSession(row: AgentConversationSession) {
  detailItems.value = []
  ruleHits.value = []
  aiResult.value = null
  activeSecurityIndex.value = -1
  const detail = await getAgentSession(row.id)
  selectedSession.value = detail
  const [data, hits, analysis] = await Promise.all([
    listAgentSessionItems(row.id),
    listAgentSessionRuleHits(row.id),
    getAgentSessionAIAnalysis(row.id),
  ])
  detailItems.value = data.items
  ruleHits.value = hits || []
  aiResult.value = analysis
  sessionDialog.value = true
}
async function runAI() {
  if (!selectedSession.value) return
  aiLoading.value = true
  try {
    aiResult.value = await runAgentSessionAIAnalysis(selectedSession.value.id)
  } finally {
    aiLoading.value = false
  }
}
function agentLabel(type: string) { return type === 'codex' ? 'OpenAI Codex CLI' : type === 'claude-code' ? 'Claude Code' : type }
function itemKind(item: AgentConversationItem) {
  if (item.item_type === 'tool_call') return 'tool-call'
  if (item.item_type === 'tool_result') return 'tool-result'
  if (item.role === 'user' || item.item_type === 'user_message') return 'user'
  if (item.role === 'assistant' || item.item_type === 'assistant_message') return 'assistant'
  return 'context'
}
function itemLabel(item: AgentConversationItem) {
  if (item.item_type === 'tool_call') return '工具调用'
  if (item.item_type === 'tool_result') return '工具结果'
  if (item.role === 'developer') return '开发者上下文'
  if (item.role === 'system') return '系统事件'
  if (item.role === 'user') return '用户'
  if (item.role === 'assistant') return '助手'
  return item.item_type || item.role || '会话事件'
}
function itemContent(item: AgentConversationItem) { return item.content_redacted || '[工具/结构化事件]' }
function ruleHitsFor(item: AgentConversationItem) {
  return ruleHits.value.filter(hit => hit.item_id === item.id || hit.item_sequence === item.sequence)
}
function aiChunksFor(item: AgentConversationItem) {
  return (aiResult.value?.chunks || []).filter(chunk => {
    if (chunk.status !== 'succeeded') return false
    const sequences = chunk.item_sequences || []
    const firstSequence = sequences.length ? sequences[0] : chunk.start_sequence
    return item.sequence === firstSequence
  })
}
function itemEvidenceCount(item: AgentConversationItem) { return ruleHitsFor(item).length + aiChunksFor(item).length }
function isActiveSecuritySequence(sequence: number) { return securityTargets.value[activeSecurityIndex.value]?.sequence === sequence }
function isActiveSecurityTarget(key: string) { return securityTargets.value[activeSecurityIndex.value]?.key === key }
function focusSecurityTarget(key: string) {
  const index = securityTargets.value.findIndex(target => target.key === key)
  if (index >= 0) {
    activeSecurityIndex.value = index
    scrollToSecurityTarget(index)
  }
}
function navigateSecurity(offset: number) {
  const nextIndex = activeSecurityIndex.value < 0
    ? (offset > 0 ? 0 : securityTargets.value.length - 1)
    : activeSecurityIndex.value + offset
  if (nextIndex < 0 || nextIndex >= securityTargets.value.length) return
  activeSecurityIndex.value = nextIndex
  scrollToSecurityTarget(nextIndex)
}
function scrollToSecurityTarget(index: number) {
  const target = securityTargets.value[index]
  if (!target) return
  void nextTick(() => {
    document.querySelector<HTMLElement>(`[data-session-sequence="${target.sequence}"]`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  })
}
function ruleName(ruleKey: string) { return sessionRules.value.find(rule => rule.rule_key === ruleKey)?.name || ruleKey }
function severityLabel(severity?: string) { return ({ critical: '严重', high: '高', medium: '中', low: '低' } as Record<string, string>)[severity || ''] || severity || '未知' }
function severityType(severity?: string) { return severity === 'critical' ? 'danger' : severity === 'high' ? 'warning' : severity === 'medium' ? '' : 'info' }
function parsedAIContent(chunk: AgentSessionAIChunk): Record<string, unknown> | null {
  if (!chunk.content) return null
  try {
    const parsed = JSON.parse(chunk.content)
    return parsed && typeof parsed === 'object' ? parsed as Record<string, unknown> : null
  } catch {
    return null
  }
}
function aiRiskLabel(chunk: AgentSessionAIChunk) {
  const value = parsedAIContent(chunk)?.risk_level
  return typeof value === 'string' ? value : chunk.status === 'succeeded' ? '已完成' : (chunk.status || '未知')
}
function aiRiskType(chunk: AgentSessionAIChunk) {
  const risk = aiRiskLabel(chunk).toLowerCase()
  return risk === 'critical' || risk === 'high' || risk === 'malicious' ? 'danger' : risk === 'medium' || risk === 'suspicious' ? 'warning' : 'success'
}
function aiSummary(chunk: AgentSessionAIChunk) {
  const parsed = parsedAIContent(chunk)
  const summary = parsed?.summary
  if (typeof summary === 'string' && summary.trim()) return summary
  const reasons = parsed?.reasons
  if (Array.isArray(reasons)) return reasons.filter(item => typeof item === 'string').join('；') || 'AI 未返回文字摘要'
  return chunk.content || 'AI 未返回可展示的分析结果'
}
function statusLabel(status?: string) { return status === 'running' ? '运行中' : '已停止' }
function statusType(status?: string) { return status === 'running' ? 'success' : 'info' }
// Collection is static and does not require the Claude/Codex process to be
// running. An asset row means the configured session files are still managed;
// stopped Agents remain collectible until the asset is removed.
function canCollect(agent: AgentGuardAgentSummary) { return Boolean(agent.asset_id) }
function riskType(level: string) { return level === 'critical' || level === 'high' ? 'danger' : level === 'medium' ? 'warning' : 'success' }
function formatTime(value?: string) { return value ? new Date(value).toLocaleString() : '-' }
onMounted(() => { void loadAgents(); void loadSessionRules() })
</script>

<style scoped>
.session-awareness { width: 100%; min-width: 0; max-width: 100%; overflow-x: hidden; }
.session-awareness :deep(.el-form) { display: flex; flex-wrap: wrap; align-items: center; }
.session-awareness :deep(.el-card), .session-awareness :deep(.el-table) { min-width: 0; max-width: 100%; }
.metric-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; margin: 18px 0; }
.metric-card { padding: 18px; border: 1px solid var(--el-border-color); border-radius: 10px; background: var(--el-bg-color); display: flex; flex-direction: column; gap: 8px; }
.metric-card span, .table-header span, .stacked span { color: var(--el-text-color-secondary); font-size: 13px; }
.metric-card strong { font-size: 24px; }
.filter-card { margin-bottom: 16px; }
.table-header { display: flex; justify-content: space-between; align-items: center; gap: 16px; }
.hero-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.stacked { display: flex; flex-direction: column; gap: 4px; }
.danger { color: var(--el-color-danger); }
.pager { justify-content: flex-end; margin-top: 16px; }
.session-table { margin-top: 16px; }
.expanded-sessions { margin: 0 24px; padding: 16px 18px 14px; border: 1px solid #e5eaf2; border-radius: 12px; background: #f8fafc; }
.expanded-sessions-head { display: flex; justify-content: space-between; align-items: center; gap: 16px; margin-bottom: 12px; }

:deep(.session-detail-drawer .el-drawer__body) { padding: 0 24px 22px; }
.conversation-summary { display: flex; justify-content: space-between; align-items: center; gap: 20px; margin: 2px 0 16px; }
.summary-primary { min-width: 0; display: flex; align-items: center; gap: 14px; }
.summary-id { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.summary-id span, .summary-token span { color: var(--el-text-color-secondary); font-size: 12px; }
.summary-id strong { max-width: 520px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; color: var(--el-text-color-primary); }
.summary-token { flex: none; display: flex; flex-direction: column; align-items: flex-end; gap: 3px; }
.summary-token strong { color: #2563eb; font-size: 20px; }
.conversation-notice { display: flex; align-items: center; gap: 8px; margin-bottom: 14px; padding: 10px 13px; border: 1px solid #dbe5f5; border-radius: 10px; background: #f5f8ff; color: #64748b; font-size: 13px; }
.notice-dot { width: 7px; height: 7px; flex: none; border-radius: 50%; background: #60a5fa; box-shadow: 0 0 0 4px #dbeafe; }
.conversation-scroll { max-height: calc(100vh - 280px); overflow: auto; padding: 10px 14px 12px 8px; border: 1px solid #e5eaf2; border-radius: 16px; background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%); }
.conversation-entry { display: grid; grid-template-columns: minmax(0, 1fr) minmax(260px, 32%); gap: 14px; align-items: start; margin: 14px 0; }
.conversation-main { min-width: 0; }
.conversation-entry:not(.has-evidence) { display: block; }
.conversation-entry.security-active { border-radius: 14px; outline: 2px solid rgba(37, 99, 235, .18); outline-offset: 5px; }
.evidence-rail { display: grid; gap: 10px; min-width: 0; }
.evidence-card { padding: 11px 12px; border: 1px solid #dbe4f0; border-radius: 11px; background: rgba(255, 255, 255, .92); box-shadow: 0 3px 12px rgba(15, 23, 42, .05); }
.rule-evidence { border-left: 3px solid #ef4444; }
.ai-evidence { border-left: 3px solid #2563eb; }
.evidence-card-heading, .evidence-item-title { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.evidence-card-heading { color: #334155; font-size: 13px; }
.evidence-item { margin-top: 10px; padding-top: 10px; border-top: 1px solid #eef2f7; }
.evidence-item, .evidence-card { cursor: pointer; }
.evidence-item.security-card-active, .evidence-card.security-card-active { background: #eff6ff; border-color: #60a5fa; box-shadow: 0 0 0 2px rgba(37, 99, 235, .12); }
.evidence-item-title { justify-content: flex-start; }
.evidence-item code { display: block; margin-top: 5px; color: #64748b; font-size: 11px; }
.evidence-item p, .ai-evidence-summary { margin: 7px 0; color: #475569; line-height: 1.5; font-size: 12px; word-break: break-word; }
.evidence-card small { color: #94a3b8; font-size: 11px; }
.chat-row { display: flex; align-items: flex-end; gap: 10px; margin: 14px 0; }
.chat-row.user { flex-direction: row-reverse; }
.chat-avatar { display: flex; width: 32px; height: 32px; flex: none; align-items: center; justify-content: center; border-radius: 50%; background: #e0e7ff; color: #4338ca; font-size: 11px; font-weight: 700; }
.chat-row.user .chat-avatar { background: #2563eb; color: #fff; }
.chat-bubble { max-width: min(78%, 720px); padding: 10px 14px; border: 1px solid #e2e8f0; border-radius: 16px 16px 16px 5px; background: #fff; box-shadow: 0 3px 12px rgba(15, 23, 42, .06); }
.chat-row.user .chat-bubble { border: 0; border-radius: 16px 16px 5px 16px; background: linear-gradient(135deg, #2563eb, #3b82f6); color: #fff; }
.chat-meta { display: flex; align-items: center; gap: 8px; margin-bottom: 5px; color: #64748b; font-size: 11px; }
.chat-meta time, .conversation-event summary time { margin-left: auto; color: #94a3b8; }
.chat-row.user .chat-meta { color: rgba(255, 255, 255, .78); }
.chat-row.user .chat-meta time { color: rgba(255, 255, 255, .64); }
.chat-content { white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.65; font-size: 14px; }
.conversation-event { margin: 10px 0; border: 1px solid #dbe4f0; border-radius: 12px; background: rgba(255, 255, 255, .76); }
.conversation-event.tool-call { border-left: 3px solid #f59e0b; }
.conversation-event.tool-result { border-left: 3px solid #10b981; }
.conversation-event.context { border-left: 3px solid #94a3b8; background: #f1f5f9; }
.conversation-event summary { display: flex; align-items: center; gap: 10px; cursor: pointer; list-style: none; padding: 10px 12px; color: #64748b; font-size: 13px; }
.conversation-event summary::-webkit-details-marker { display: none; }
.conversation-event summary::before { content: '›'; display: inline-block; color: #94a3b8; font-size: 18px; line-height: 12px; transition: transform .15s ease; }
.conversation-event[open] summary::before { transform: rotate(90deg); }
.event-kind { color: #475569; font-weight: 600; }
.event-expand { margin-left: auto; color: #94a3b8; font-size: 12px; }
.conversation-event[open] .event-expand { color: #2563eb; }
.conversation-event pre { max-height: 360px; overflow: auto; margin: 0; padding: 0 14px 14px; white-space: pre-wrap; overflow-wrap: anywhere; color: #475569; font: 13px/1.65 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.conversation-actions { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-top: 14px; padding-top: 12px; border-top: 1px solid #eef2f7; }
.action-summary { display: flex; gap: 12px; color: var(--el-text-color-secondary); font-size: 12px; }
.security-navigation { display: flex; align-items: center; justify-content: space-between; gap: 14px; margin-top: 14px; padding: 11px 13px; border: 1px solid #dbe5f5; border-radius: 10px; background: #f5f8ff; }
.security-navigation-copy { display: flex; align-items: baseline; gap: 10px; min-width: 0; }
.security-navigation-copy strong { color: #1e3a8a; font-size: 13px; }
.security-navigation-copy span { overflow: hidden; color: #64748b; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.ai-overview { margin-top: 14px; }
@media (max-width: 900px) { .metric-grid { grid-template-columns: repeat(2, 1fr); }.expanded-sessions { margin: 0 8px; padding: 12px; }.expanded-sessions-head { align-items: flex-start; flex-direction: column; } }
@media (max-width: 900px) { .conversation-entry { grid-template-columns: 1fr; }.evidence-rail { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 640px) { .conversation-summary, .conversation-actions, .security-navigation { align-items: flex-start; flex-direction: column; }.summary-primary { width: 100%; }.summary-id strong { max-width: 60vw; }.summary-token { align-items: flex-start; }.chat-bubble { max-width: calc(100% - 42px); }.conversation-scroll { padding-right: 8px; }.action-summary { flex-wrap: wrap; }.evidence-rail { grid-template-columns: 1fr; }.security-navigation-copy { align-items: flex-start; flex-direction: column; gap: 3px; } }
</style>
