<template>
  <div class="agent-config-page page-shell">
    <section class="page-hero">
      <div>
        <h1>{{ t('agentGuard.config.title') }}</h1>
        <p>{{ t('agentGuard.config.description') }}</p>
      </div>
      <el-tag type="info" effect="plain">{{ t('agentGuard.config.readOnly') }}</el-tag>
    </section>

    <el-alert
      class="privacy-alert"
      type="warning"
      :title="t('agentGuard.config.privacy')"
      :closable="false"
      show-icon
    />

    <el-card class="scan-card">
      <el-form inline @submit.prevent="scan">
        <el-form-item :label="t('agentGuard.config.host')">
          <el-select
            v-model="selectedHostId"
            filterable
            clearable
            :loading="hostsLoading"
            :placeholder="t('agentGuard.config.hostPlaceholder')"
            class="host-select"
          >
            <el-option
              v-for="host in hosts"
              :key="host.id"
              :label="`${host.hostname || host.id} (${host.ip_address || '-'})${host.online ? '' : ` · ${t('agentGuard.config.offline')}`}`"
              :value="host.id"
            />
          </el-select>
        </el-form-item>
        <el-button type="primary" :loading="scanning" :disabled="!selectedHostId || !selectedHostOnline" @click="scan">
          {{ t('agentGuard.config.scan') }}
        </el-button>
        <el-button :loading="hostsLoading" @click="loadHosts">{{ t('common.actions.refresh') }}</el-button>
      </el-form>
    </el-card>

    <el-skeleton v-if="hostsLoading && !hosts.length" :rows="3" animated />
    <el-result v-else-if="hostsError" icon="error" :title="t('agentGuard.config.loadFailed')" />
    <el-empty v-else-if="!result" :description="t('agentGuard.config.empty')" />
    <template v-else>
      <div class="metric-grid">
        <div class="metric-card"><span>{{ t('agentGuard.config.metrics.agents') }}</span><strong>{{ result.agents.length }}</strong></div>
        <div class="metric-card"><span>{{ t('agentGuard.config.metrics.files') }}</span><strong>{{ fileCount }}</strong></div>
        <div class="metric-card"><span>{{ t('agentGuard.config.metrics.hooks') }}</span><strong>{{ hookCount }}</strong></div>
      </div>

      <el-alert
        v-for="error in result.errors"
        :key="`${error.stage}-${error.message}`"
        class="scan-error"
        type="warning"
        :title="`${error.stage}: ${error.message}`"
        :closable="false"
      />

      <el-card class="agent-list-card">
        <template #header>
          <div class="list-header">
            <div>
              <strong>{{ t('agentGuard.config.listTitle') }}</strong>
              <p>{{ t('agentGuard.config.listHint') }}</p>
            </div>
            <span class="scan-time">{{ t('agentGuard.config.scannedAt') }}：{{ formatTime(result.scanned_at) }}</span>
          </div>
        </template>

        <el-empty v-if="!result.agents.length" :description="t('agentGuard.config.noAgents')" />
        <el-table
          v-else
          :data="result.agents"
          row-key="agent_type"
          class="agent-summary-table"
          @row-click="openAgent"
        >
          <el-table-column :label="t('agentGuard.config.agent')" min-width="190" fixed="left">
            <template #default="{ row }">
              <div class="agent-identity">
                <strong>{{ row.display_name }}</strong>
                <span>{{ row.agent_type }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="t('agentGuard.config.files')" width="130" align="center">
            <template #default="{ row }">{{ row.files.length }}</template>
          </el-table-column>
          <el-table-column :label="t('agentGuard.config.hooks')" width="130" align="center">
            <template #default="{ row }">{{ row.hooks.length }}</template>
          </el-table-column>
          <el-table-column :label="t('agentGuard.config.risk')" width="130" align="center">
            <template #default="{ row }">
              <strong :class="{ 'risk-number': row.finding_count > 0 }">{{ row.finding_count }}</strong>
            </template>
          </el-table-column>
          <el-table-column :label="t('agentGuard.config.status')" min-width="150">
            <template #default="{ row }">
              <el-tag :type="row.finding_count ? 'danger' : 'success'" effect="plain">
                {{ row.finding_count ? t('agentGuard.config.riskCount', { count: row.finding_count }) : t('agentGuard.config.safe') }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('agentGuard.config.actions')" width="120" fixed="right" align="center">
            <template #default="{ row }">
              <el-button link type="primary" @click.stop="openAgent(row)">
                {{ t('common.actions.details') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>

    <el-drawer
      class="agent-config-detail-drawer"
      :model-value="detailVisible"
      direction="rtl"
      size="78%"
      :append-to-body="true"
      :destroy-on-close="false"
      @close="detailVisible = false"
    >
      <template #header>
        <div class="drawer-header" v-if="selectedAgent">
          <div>
            <h2>{{ selectedAgent.display_name }}</h2>
            <div class="drawer-meta">
              <span>{{ t('agentGuard.config.agentType') }}：{{ selectedAgent.agent_type }}</span>
              <span>{{ t('agentGuard.config.host') }}：{{ result?.hostname || result?.host_id || '-' }}</span>
              <el-tag :type="selectedAgent.finding_count ? 'danger' : 'success'" effect="plain" size="small">
                {{ selectedAgent.finding_count ? t('agentGuard.config.riskCount', { count: selectedAgent.finding_count }) : t('agentGuard.config.safe') }}
              </el-tag>
            </div>
          </div>
        </div>
      </template>

      <div v-if="selectedAgent" class="drawer-body">
        <el-alert
          :type="selectedAgent.finding_count ? 'warning' : 'success'"
          :title="selectedAgent.finding_count ? t('agentGuard.config.riskAlert') : t('agentGuard.config.safeAlert')"
          :closable="false"
          show-icon
        />

        <div class="detail-metric-grid">
          <div class="detail-metric"><span>{{ t('agentGuard.config.files') }}</span><strong>{{ selectedAgent.files.length }}</strong></div>
          <div class="detail-metric"><span>{{ t('agentGuard.config.hooks') }}</span><strong>{{ selectedAgent.hooks.length }}</strong></div>
          <div class="detail-metric"><span>{{ t('agentGuard.config.risk') }}</span><strong :class="{ 'risk-number': selectedAgent.finding_count > 0 }">{{ selectedAgent.finding_count }}</strong></div>
        </div>

        <el-tabs v-model="detailTab" class="detail-tabs">
          <el-tab-pane :label="t('agentGuard.config.detail.filesTab')" name="files">
            <div class="detail-workspace">
              <section class="detail-list-panel">
                <header class="detail-panel-header">
                  <div>
                    <h3>{{ t('agentGuard.config.detail.fileList') }}</h3>
                    <p>{{ t('agentGuard.config.detail.fileListHint') }}</p>
                  </div>
                  <el-tag size="small" effect="plain">{{ selectedAgent.files.length }}</el-tag>
                </header>
                <el-empty v-if="!selectedAgent.files.length" :description="t('agentGuard.config.noContent')" />
                <button
                  v-for="file in selectedAgent.files"
                  :key="file.path"
                  type="button"
                  class="detail-list-row"
                  :class="{ selected: activeFile?.path === file.path }"
                  @click="selectedFilePath = file.path"
                >
                  <span class="detail-list-main">
                    <strong>{{ file.path.split('/').pop() || file.path }}</strong>
                    <small>{{ file.path }}</small>
                  </span>
                  <span class="detail-list-side">
                    <el-tag size="small" :type="file.findings.length ? 'danger' : 'success'" effect="plain">{{ file.findings.length }}</el-tag>
                    <small>{{ file.status }}</small>
                  </span>
                </button>
              </section>

              <section class="detail-content-panel">
                <el-empty v-if="!activeFile" :description="t('agentGuard.config.detail.selectFile')" />
                <template v-else>
                  <header class="content-header">
                    <div>
                      <h3>{{ activeFile.path }}</h3>
                      <p>{{ activeFile.format }} · {{ formatBytes(activeFile.size) }} · {{ formatTime(activeFile.modified_at) }}</p>
                    </div>
                    <el-tag :type="activeFile.findings.length ? 'danger' : 'success'" effect="plain">
                      {{ activeFile.findings.length ? t('agentGuard.config.risky') : t('agentGuard.config.normal') }}
                    </el-tag>
                  </header>
                  <el-alert v-if="activeFile.error" type="warning" :title="activeFile.error" :closable="false" />
                  <pre v-if="activeFile.content" class="config-content">{{ activeFile.content }}</pre>
                  <el-empty v-else :description="t('agentGuard.config.noContent')" />
                  <FindingTable
                    :findings="activeFile.findings"
                    :reason-label="t('agentGuard.config.detail.verdictReason')"
                    :remediation-label="t('agentGuard.config.detail.remediation')"
                  />
                </template>
              </section>
            </div>
          </el-tab-pane>

          <el-tab-pane :label="t('agentGuard.config.detail.hooksTab')" name="hooks">
            <section class="detail-section">
              <header class="detail-section-header">
                <div>
                  <h3>{{ t('agentGuard.config.hooks') }}</h3>
                  <p>{{ t('agentGuard.config.detail.hooksHint') }}</p>
                </div>
                <el-tag size="small" effect="plain">{{ selectedAgent.hooks.length }}</el-tag>
              </header>
              <el-empty v-if="!selectedAgent.hooks.length" :description="t('agentGuard.config.detail.noHooks')" />
              <div v-else class="hook-list">
                <button
                  v-for="(hook, index) in selectedAgent.hooks"
                  :key="`${hook.file_path}-${hook.field_path}-${index}`"
                  type="button"
                  class="hook-row"
                  :class="{ selected: activeHook === hook }"
                  @click="selectedHookKey = hookKey(hook, index)"
                >
                  <span class="hook-row-main">
                    <strong>{{ hook.event || '-' }}</strong>
                    <small>{{ hook.field_path || '-' }}</small>
                    <code>{{ hook.command || '-' }}</code>
                  </span>
                  <el-tag :type="hook.findings.length ? 'danger' : 'success'" size="small" effect="plain">
                    {{ hook.findings.length ? t('agentGuard.config.risky') : t('agentGuard.config.normal') }}
                  </el-tag>
                </button>
              </div>
            </section>
            <section v-if="activeHook" class="detail-section hook-detail-section">
              <header class="detail-section-header">
                <div>
                  <h3>{{ t('agentGuard.config.detail.hookDetail') }}</h3>
                  <p>{{ activeHook.file_path }}</p>
                </div>
                <el-tag :type="activeHook.findings.length ? 'danger' : 'success'" effect="plain">
                  {{ activeHook.findings.length ? t('agentGuard.config.risky') : t('agentGuard.config.normal') }}
                </el-tag>
              </header>
              <div class="field-grid">
                <div class="field"><span>{{ t('agentGuard.config.event') }}</span><strong>{{ activeHook.event || '-' }}</strong></div>
                <div class="field"><span>{{ t('agentGuard.config.detail.executor') }}</span><code>{{ activeHook.executor || '-' }}</code></div>
                <div class="field field-wide"><span>{{ t('agentGuard.config.field') }}</span><code>{{ activeHook.field_path || '-' }}</code></div>
                <div class="field field-wide"><span>{{ t('agentGuard.config.command') }}</span><code>{{ activeHook.command || '-' }}</code></div>
              </div>
              <FindingTable
                :findings="activeHook.findings"
                :reason-label="t('agentGuard.config.detail.verdictReason')"
                :remediation-label="t('agentGuard.config.detail.remediation')"
              />
            </section>
          </el-tab-pane>

        </el-tabs>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { formatDateTime } from '@/i18n/formatters'
import { scanAgentConfigurations } from '@/api/agentGuard'
import { getHosts } from '@/api/hosts'
import type { Host } from '@/types'
import type { AgentConfigAgent, AgentConfigFile, AgentConfigHook, AgentConfigScanResult } from '@/types/agentGuard'

const { t } = useI18n()
const hosts = ref<Host[]>([])
const selectedHostId = ref('')
const result = ref<AgentConfigScanResult | null>(null)
const selectedAgent = ref<AgentConfigAgent | null>(null)
const selectedFilePath = ref('')
const selectedHookKey = ref('')
const detailVisible = ref(false)
const detailTab = ref('files')
const hostsLoading = ref(false)
const hostsError = ref(false)
const scanning = ref(false)

const fileCount = computed(() => result.value?.agents.reduce((count, agent) => count + agent.files.length, 0) || 0)
const hookCount = computed(() => result.value?.agents.reduce((count, agent) => count + agent.hooks.length, 0) || 0)
const selectedHostOnline = computed(() => hosts.value.find(host => host.id === selectedHostId.value)?.online === true)
const activeFile = computed<AgentConfigFile | null>(() => {
  const agent = selectedAgent.value
  return agent?.files.find(file => file.path === selectedFilePath.value) || agent?.files[0] || null
})
const activeHook = computed<AgentConfigHook | null>(() => {
  const agent = selectedAgent.value
  if (!agent) return null
  const index = agent.hooks.findIndex((hook, hookIndex) => hookKey(hook, hookIndex) === selectedHookKey.value)
  return agent.hooks[index >= 0 ? index : 0] || null
})
async function loadHosts() {
  hostsLoading.value = true
  hostsError.value = false
  try {
    const response = await getHosts({ page: 1, pageSize: 1000 })
    const unique = new Map<string, Host>()
    for (const host of response || []) {
      if (host.id && !unique.has(host.id)) unique.set(host.id, host)
    }
    hosts.value = [...unique.values()]
    if (!selectedHostId.value && hosts.value.length) selectedHostId.value = hosts.value[0].id
  } catch {
    hostsError.value = true
  } finally {
    hostsLoading.value = false
  }
}

async function scan() {
  if (!selectedHostId.value || !selectedHostOnline.value) return
  scanning.value = true
  detailVisible.value = false
  try {
    result.value = normalizeScanResult(await scanAgentConfigurations(selectedHostId.value))
  } catch {
    ElMessage.error(t('agentGuard.config.scanFailed'))
  } finally {
    scanning.value = false
  }
}

function openAgent(agent: AgentConfigAgent) {
  selectedAgent.value = agent
  selectedFilePath.value = agent.files[0]?.path || ''
  selectedHookKey.value = agent.hooks.length ? hookKey(agent.hooks[0], 0) : ''
  detailTab.value = 'files'
  detailVisible.value = true
}

function normalizeScanResult(input: AgentConfigScanResult | null | undefined): AgentConfigScanResult {
  const raw = input || {} as AgentConfigScanResult
  return {
    host_id: raw.host_id || selectedHostId.value,
    hostname: raw.hostname || '',
    scanned_at: raw.scanned_at || new Date().toISOString(),
    finding_count: Number(raw.finding_count) || 0,
    errors: Array.isArray(raw.errors) ? raw.errors.filter(Boolean) : [],
    agents: Array.isArray(raw.agents) ? raw.agents.filter(Boolean).map(agent => ({
      ...agent,
      files: Array.isArray(agent.files) ? agent.files.filter(Boolean).map(file => ({ ...file, findings: Array.isArray(file.findings) ? file.findings : [] })) : [],
      hooks: Array.isArray(agent.hooks) ? agent.hooks.filter(Boolean).map(hook => ({ ...hook, findings: Array.isArray(hook.findings) ? hook.findings : [] })) : [],
    })) : [],
  }
}

function hookKey(hook: AgentConfigHook, index: number) {
  return `${hook.file_path}|${hook.field_path}|${index}`
}

function formatTime(value?: string) {
  return value ? formatDateTime(value) : '-'
}

function formatBytes(value?: number) {
  if (!value) return '-'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / (1024 * 1024)).toFixed(1)} MB`
}

onMounted(loadHosts)
</script>

<script lang="ts">
import { defineComponent, h } from 'vue'
import type { PropType } from 'vue'
import type { AgentConfigFinding } from '@/types/agentGuard'

const FindingTable = defineComponent({
  name: 'FindingTable',
  props: {
    findings: { type: Array as PropType<AgentConfigFinding[]>, required: true },
    reasonLabel: { type: String, default: '判定原因' },
    remediationLabel: { type: String, default: '整改建议' },
  },
  setup(props) {
    return () => props.findings.length ? h('div', { class: 'finding-list' }, props.findings.map(finding => h('div', { class: 'finding-item', key: `${finding.rule_id}-${finding.field_path}` }, [
      h('el-tag', { type: finding.severity === 'critical' || finding.severity === 'high' ? 'danger' : 'warning', size: 'small' }, () => finding.severity),
      h('strong', finding.title),
      h('code', finding.field_path || '-'),
      h('div', { class: 'finding-reason' }, [
        h('strong', { class: 'finding-detail-label' }, `${props.reasonLabel}：`),
        h('span', finding.reason || '-'),
      ]),
      finding.remediation ? h('div', { class: 'finding-remediation' }, [
        h('strong', { class: 'finding-detail-label' }, `${props.remediationLabel}：`),
        h('span', finding.remediation),
      ]) : null,
    ]))) : null
  },
})

export default { components: { FindingTable } }
</script>

<style scoped>
.agent-config-page { display: flex; flex-direction: column; gap: 16px; }
.page-hero, .list-header, .drawer-header, .content-header { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.scan-card { padding-bottom: 0; }
.host-select { width: 360px; }
.metric-grid, .detail-metric-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.metric-card, .detail-metric { padding: 16px; border: 1px solid var(--el-border-color); border-radius: 8px; background: var(--el-bg-color-overlay); }
.metric-card span, .detail-metric span { display: block; color: var(--el-text-color-secondary); font-size: 13px; }
.metric-card strong, .detail-metric strong { display: block; margin-top: 6px; font-size: 26px; }
.metric-card.risk strong, .risk-number { color: var(--el-color-danger); }
.scan-error { margin-top: -4px; }
.list-header { align-items: flex-start; }
.list-header p { margin: 5px 0 0; color: var(--el-text-color-secondary); font-size: 12px; }
.scan-time { color: var(--el-text-color-secondary); font-size: 12px; }
.agent-summary-table :deep(.el-table__row) { cursor: pointer; }
.agent-identity { display: flex; flex-direction: column; gap: 3px; }
.agent-identity span, .drawer-meta, .content-header p, .detail-panel-header p { color: var(--el-text-color-secondary); font-size: 12px; }
.drawer-header { align-items: flex-start; }
.drawer-header h2 { margin: 0 0 8px; font-size: 20px; }
.drawer-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
.drawer-body { display: grid; gap: 16px; min-width: 0; }
.detail-metric-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.detail-tabs { min-width: 0; }
.detail-workspace { display: grid; grid-template-columns: minmax(240px, 1fr) minmax(0, 2.2fr); gap: 14px; align-items: start; }
.detail-list-panel, .detail-content-panel, .detail-section { min-width: 0; padding: 14px; border: 1px solid var(--el-border-color-lighter); border-radius: 10px; background: var(--el-fill-color-blank); }
.detail-list-panel { display: flex; flex-direction: column; gap: 8px; }
.detail-panel-header, .detail-section-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding-bottom: 10px; border-bottom: 1px solid var(--el-border-color-lighter); }
.detail-panel-header h3, .detail-section-header h3, .content-header h3 { margin: 0; font-size: 16px; }
.detail-panel-header p, .detail-section-header p { margin: 4px 0 0; line-height: 1.5; }
.detail-list-row, .hook-row { display: flex; width: 100%; min-width: 0; align-items: flex-start; justify-content: space-between; gap: 10px; padding: 11px; border: 1px solid var(--el-border-color-lighter); border-radius: 8px; background: var(--el-bg-color); color: inherit; text-align: left; cursor: pointer; }
.detail-list-row:hover, .detail-list-row.selected, .hook-row:hover, .hook-row.selected { border-color: var(--el-color-primary-light-5); background: var(--el-color-primary-light-9); }
.detail-list-main, .hook-row-main { display: flex; min-width: 0; flex: 1 1 auto; flex-direction: column; gap: 4px; }
.detail-list-main strong, .detail-list-main small, .hook-row-main strong, .hook-row-main small, .hook-row-main code { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.detail-list-main small, .hook-row-main small { color: var(--el-text-color-secondary); font-size: 11px; }
.detail-list-side { display: flex; flex: 0 0 auto; flex-direction: column; align-items: flex-end; gap: 4px; color: var(--el-text-color-secondary); font-size: 11px; }
.detail-content-panel { display: grid; gap: 12px; }
.content-header { align-items: flex-start; padding-bottom: 10px; border-bottom: 1px solid var(--el-border-color-lighter); }
.content-header h3 { overflow-wrap: anywhere; word-break: break-word; }
.content-header p { margin: 5px 0 0; }
.config-content { max-height: 460px; overflow: auto; margin: 0; padding: 14px; border-radius: 6px; background: #111827; color: #d1fae5; white-space: pre-wrap; word-break: break-word; font-size: 12px; line-height: 1.55; }
.detail-section { display: grid; gap: 14px; }
.hook-list { display: grid; gap: 8px; }
.hook-row-main code { color: var(--el-text-color-regular); font-size: 12px; }
.hook-detail-section { margin-top: 14px; }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px 18px; }
.field { display: grid; min-width: 0; gap: 4px; }
.field-wide { grid-column: 1 / -1; }
.field span { color: var(--el-text-color-secondary); font-size: 12px; }
.field strong, .field code { min-width: 0; overflow-wrap: anywhere; word-break: break-word; white-space: pre-wrap; font-size: 13px; }
.finding-list { display: flex; flex-direction: column; gap: 8px; }
.finding-item { display: grid; grid-template-columns: 80px 180px minmax(160px, 260px) 1fr; gap: 8px; align-items: center; padding: 10px; border-left: 3px solid var(--el-color-danger); background: var(--el-fill-color-light); font-size: 13px; }
.finding-reason, .finding-remediation { grid-column: 1 / -1; display: flex; gap: 4px; min-width: 0; line-height: 1.5; }
.finding-reason { margin-top: 2px; }
.finding-remediation { color: var(--el-text-color-secondary); font-size: 12px; }
.finding-detail-label { flex: 0 0 auto; color: var(--el-text-color-regular); }
.finding-reason span, .finding-remediation span { min-width: 0; overflow-wrap: anywhere; word-break: break-word; }
.finding-item code { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
:deep(.el-drawer__body) { min-width: 0; overflow-x: hidden; }
:deep(.el-alert__content), :deep(.el-alert__title) { min-width: 0; max-width: 100%; overflow-wrap: anywhere; white-space: normal; }
@media (max-width: 1000px) { .metric-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .detail-workspace { grid-template-columns: 1fr; } }
@media (max-width: 640px) { .host-select { width: 240px; } .page-hero, .list-header { align-items: flex-start; flex-direction: column; } .detail-metric-grid { grid-template-columns: 1fr; } .field-grid, .finding-item { grid-template-columns: 1fr; } .finding-item code, .finding-item span, .finding-item small { grid-column: 1; } }
</style>
