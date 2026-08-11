<template>
  <div class="mcp-platform page-shell">
    <section class="page-hero mcp-hero">
      <div>
        <span class="hero-kicker">Remote MCP Governance</span>
        <h1>{{ t('app.mcpAggregation.title') }}</h1>
        <p>{{ t('app.mcpAggregation.description') }}</p>
      </div>
      <div class="hero-actions">
        <el-button type="primary" :icon="Connection" @click="onboardingVisible = true">
          {{ t('app.mcpAggregation.connectRemote') }}
        </el-button>
        <el-button type="success" :icon="Plus" @click="openClientEndpoint">
          {{ t('app.mcpAggregation.createClientEndpoint') }}
        </el-button>
      </div>
    </section>

    <div class="metric-grid">
        <button v-for="metric in metrics" :key="metric.key" class="metric-card" type="button" @click="activeTab = metric.tab">
          <span class="metric-label">{{ metric.label }}</span>
          <strong>{{ metric.value }}</strong>
          <small>{{ store.lastUpdatedAt ? formatTime(store.lastUpdatedAt) : '—' }}</small>
        </button>
    </div>

    <el-card class="aegis-card platform-card" shadow="never">
        <el-tabs v-model="activeTab" class="platform-tabs" @tab-change="onTabChange">
          <el-tab-pane name="servers" :label="t('app.mcpAggregation.servers')">
            <div class="filter-bar">
              <el-input v-model="filters.keyword" :placeholder="t('app.mcpAggregation.search')" clearable @keyup.enter="load" />
              <el-select v-model="filters.environment" :placeholder="t('app.mcpAggregation.environment')" clearable>
                <el-option value="dev" :label="t('app.mcpAggregation.dev')" />
                <el-option value="test" :label="t('app.mcpAggregation.test')" />
                <el-option value="prod" :label="t('app.mcpAggregation.prod')" />
              </el-select>
              <el-select v-model="filters.status" :placeholder="t('app.mcpAggregation.status')" clearable>
                <el-option value="draft" :label="statusLabel('draft')" />
                <el-option value="approved" :label="statusLabel('approved')" />
                <el-option value="published" :label="statusLabel('published')" />
                <el-option value="quarantined" :label="statusLabel('quarantined')" />
              </el-select>
              <el-select v-model="filters.risk_tier" :placeholder="t('app.mcpAggregation.risk')" clearable>
                <el-option value="l1" label="L1" />
                <el-option value="l2" label="L2" />
                <el-option value="l3" label="L3" />
                <el-option value="l4" label="L4" />
              </el-select>
              <el-button type="primary" :loading="store.loading" @click="load">{{ t('app.mcpAggregation.query') }}</el-button>
              <el-button @click="resetFilters">{{ t('app.mcpAggregation.reset') }}</el-button>
            </div>

            <el-alert v-if="store.error" :title="store.error" type="error" show-icon :closable="false" class="state-alert" />
            <el-table v-loading="store.loading" :data="store.servers" row-key="id" class="mcp-table">
              <el-table-column prop="display_name" :label="t('app.mcpAggregation.serviceName')" min-width="180" />
              <el-table-column prop="endpoint_display" :label="t('app.mcpAggregation.endpoint')" min-width="220" />
              <el-table-column prop="environment" :label="t('app.mcpAggregation.environment')" width="110" />
              <el-table-column :label="t('app.mcpAggregation.authType')" width="130">
                <template #default="{ row }">{{ row.transport }} / {{ row.auth_type }}</template>
              </el-table-column>
              <el-table-column :label="t('app.mcpAggregation.tools')" width="90">
                <template #default="{ row }">{{ row.tool_count }}</template>
              </el-table-column>
              <el-table-column :label="t('app.mcpAggregation.risk')" width="80">
                <template #default="{ row }"><el-tag :type="riskTag(row.risk_tier)" size="small">{{ row.risk_tier.toUpperCase() }}</el-tag></template>
              </el-table-column>
              <el-table-column :label="t('app.mcpAggregation.status')" width="135">
                <template #default="{ row }"><el-tag :type="statusTag(row.lifecycle_status)" size="small">{{ statusLabel(row.lifecycle_status) }}</el-tag></template>
              </el-table-column>
              <el-table-column :label="t('app.mcpAggregation.actions')" width="105" fixed="right">
                <template #default="{ row }"><el-button link type="primary" @click="selectedServer = row">{{ t('app.mcpAggregation.detail') }}</el-button></template>
              </el-table-column>
              <template #empty><el-empty :description="t('app.mcpAggregation.empty')" /></template>
            </el-table>

          </el-tab-pane>
          <el-tab-pane name="tools" :label="t('app.mcpAggregation.toolList')">
            <el-table v-loading="store.loading" :data="toolRows" row-key="id" class="mcp-table tool-table">
              <el-table-column :label="t('app.mcpAggregation.server')" min-width="180">
                <template #default="{ row }">
                  <div class="tool-service">{{ row.serverName }}</div>
                  <small class="tool-revision">{{ row.serverRevisionId }}</small>
                </template>
              </el-table-column>
              <el-table-column prop="alias" :label="t('app.mcpAggregation.alias')" min-width="180" />
              <el-table-column :label="t('app.mcpAggregation.toolName')" min-width="180">
                <template #default="{ row }">{{ row.title || row.upstream_name }}</template>
              </el-table-column>
              <el-table-column :label="t('app.mcpAggregation.toolDescription')" min-width="320" show-overflow-tooltip>
                <template #default="{ row }">{{ row.description || '—' }}</template>
              </el-table-column>
              <el-table-column :label="t('app.mcpAggregation.inputSchema')" min-width="220" show-overflow-tooltip>
                <template #default="{ row }">{{ formatSchema(row.input_schema) }}</template>
              </el-table-column>
              <el-table-column :label="t('app.mcpAggregation.risk')" width="80">
                <template #default="{ row }"><el-tag :type="riskTag(row.risk_tier)" size="small">{{ row.risk_tier.toUpperCase() }}</el-tag></template>
              </el-table-column>
              <el-table-column :label="t('app.mcpAggregation.status')" width="110">
                <template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag></template>
              </el-table-column>
              <template #empty><el-empty :description="t('app.mcpAggregation.empty')" /></template>
            </el-table>
          </el-tab-pane>
          <el-tab-pane name="clients" :label="t('app.mcpAggregation.clients')">
            <div class="client-endpoint-toolbar">
              <div><strong>{{ t('app.mcpAggregation.clientEndpoints') }}</strong><span>{{ t('app.mcpAggregation.clientEndpointsHint') }}</span></div>
              <el-button type="primary" :icon="Plus" @click="openClientEndpoint">{{ t('app.mcpAggregation.createClientEndpoint') }}</el-button>
            </div>
            <el-table v-loading="store.loading" :data="store.clientEndpoints" row-key="client_id" class="mcp-table">
              <el-table-column prop="display_name" :label="t('app.mcpAggregation.clientName')" min-width="160" />
              <el-table-column prop="client_key" :label="t('app.mcpAggregation.clientKey')" min-width="150" />
              <el-table-column prop="server_name" :label="t('app.mcpAggregation.boundService')" min-width="180" />
              <el-table-column :label="t('app.mcpAggregation.endpoint')" min-width="320" show-overflow-tooltip>
                <template #default="{ row }"><code>{{ row.endpoint }}</code></template>
              </el-table-column>
              <el-table-column :label="t('app.mcpAggregation.tools')" width="110"><template #default="{ row }">{{ enabledToolCount(row) }} / {{ row.tools.length }}</template></el-table-column>
              <el-table-column :label="t('app.mcpAggregation.status')" width="110"><template #default="{ row }"><el-tag type="success" size="small">{{ statusLabel(row.status) }}</el-tag></template></el-table-column>
              <el-table-column :label="t('app.mcpAggregation.actions')" width="130" fixed="right"><template #default="{ row }"><el-button link type="primary" :icon="Setting" @click="openEndpointTools(row)">{{ t('app.mcpAggregation.toolControl') }}</el-button></template></el-table-column>
              <template #empty><el-empty :description="t('app.mcpAggregation.empty')" /></template>
            </el-table>
            <el-divider />
            <el-table v-loading="store.loading" :data="store.clients" row-key="id" class="mcp-table">
              <el-table-column prop="client_key" :label="t('app.mcpAggregation.client')" />
              <el-table-column prop="display_name" :label="t('app.mcpAggregation.serviceName')" />
              <el-table-column prop="client_type" :label="t('app.mcpAggregation.authType')" />
              <el-table-column :label="t('app.mcpAggregation.status')"><template #default="{ row }">{{ statusLabel(row.status) }}</template></el-table-column>
              <template #empty><el-empty :description="t('app.mcpAggregation.empty')" /></template>
            </el-table>
          </el-tab-pane>
          <el-tab-pane name="approvals" :label="t('app.mcpAggregation.approvals')">
            <el-table v-loading="store.loading" :data="store.approvals" row-key="id" class="mcp-table">
              <el-table-column :label="t('app.mcpAggregation.approval')" min-width="130">
                <template #default="{ row }">{{ approvalTypeLabel(row.approval_type) }}</template>
              </el-table-column>
              <el-table-column :label="t('app.mcpAggregation.serviceName')" min-width="180">
                <template #default="{ row }">{{ approvalSubjectLabel(row.subject_type) }}</template>
              </el-table-column>
              <el-table-column :label="t('app.mcpAggregation.status')" width="110">
                <template #default="{ row }"><el-tag :type="statusTag(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag></template>
              </el-table-column>
              <el-table-column prop="requested_by" label="申请人" width="120" />
              <el-table-column prop="request_digest" :label="t('app.mcpAggregation.digest')" show-overflow-tooltip />
              <el-table-column :label="t('app.mcpAggregation.actions')" width="180" fixed="right">
                <template #default="{ row }">
                  <template v-if="row.status === 'pending'">
                    <template v-if="canDecideApproval(row)">
                      <el-button link type="success" :loading="approvalSubmitting === row.id" @click="decideApproval(row, 'approved')">{{ t('app.mcpAggregation.approve') }}</el-button>
                      <el-button link type="danger" :loading="approvalSubmitting === row.id" @click="decideApproval(row, 'rejected')">{{ t('app.mcpAggregation.reject') }}</el-button>
                    </template>
                    <el-tooltip v-else :content="t('app.mcpAggregation.selfApprovalForbidden')" placement="top">
                      <span class="approval-blocked">{{ t('app.mcpAggregation.selfApprovalBlocked') }}</span>
                    </el-tooltip>
                  </template>
                  <span v-else>—</span>
                </template>
              </el-table-column>
              <template #empty><el-empty :description="t('app.mcpAggregation.empty')" /></template>
            </el-table>
          </el-tab-pane>
          <el-tab-pane name="invocations" :label="t('app.mcpAggregation.invocations')">
            <el-table v-loading="store.loading" :data="store.invocations" row-key="id" class="mcp-table">
              <el-table-column prop="tool_alias" :label="t('app.mcpAggregation.alias')" />
              <el-table-column :label="t('app.mcpAggregation.status')"><template #default="{ row }">{{ statusLabel(row.status) }}</template></el-table-column>
              <el-table-column prop="policy_decision" :label="t('app.mcpAggregation.decision')" />
              <el-table-column prop="ai_status" :label="t('app.mcpAggregation.aiStatus')" />
              <template #empty><el-empty :description="t('app.mcpAggregation.empty')" /></template>
            </el-table>
          </el-tab-pane>
          <el-tab-pane name="security" :label="t('app.mcpAggregation.security')">
            <el-table v-loading="store.loading" :data="store.securityVerdicts" row-key="id" class="mcp-table">
              <el-table-column prop="invocation_id" :label="t('app.mcpAggregation.invocation')" show-overflow-tooltip />
              <el-table-column prop="deterministic_severity" :label="t('app.mcpAggregation.deterministic')" />
              <el-table-column prop="ai_verdict" :label="t('app.mcpAggregation.aiStatus')" />
              <el-table-column prop="overall_risk" :label="t('app.mcpAggregation.overallRisk')" />
              <template #empty><el-empty :description="t('app.mcpAggregation.empty')" /></template>
            </el-table>
          </el-tab-pane>
        </el-tabs>
    </el-card>

    <el-drawer v-model="onboardingVisible" :title="t('app.mcpAggregation.connectRemote')" size="520px" destroy-on-close>
      <el-form ref="formRef" :model="form" label-position="top" @submit.prevent="submitOnboarding">
        <el-form-item :label="t('app.mcpAggregation.serviceName')" required><el-input v-model="form.display_name" maxlength="160" /></el-form-item>
        <el-form-item :label="t('app.mcpAggregation.endpoint')" required>
          <el-input v-model="form.endpoint_url" placeholder="https://mcp.example.com/mcp" autocomplete="off" />
          <div class="field-hint">{{ t('app.mcpAggregation.endpointHint') }}</div>
        </el-form-item>
        <el-form-item :label="t('app.mcpAggregation.authType')" required><el-select v-model="form.auth_type" class="full-width"><el-option value="none" :label="t('app.mcpAggregation.none')" /><el-option value="oauth2" :label="t('app.mcpAggregation.oauth2')" /><el-option value="bearer" :label="t('app.mcpAggregation.bearer')" /><el-option value="api_key" :label="t('app.mcpAggregation.apiKey')" /></el-select></el-form-item>
        <el-form-item v-if="form.auth_type !== 'none'" :label="t('app.mcpAggregation.credentialRef')"><el-input v-model="form.credential_ref" type="password" show-password autocomplete="new-password" /></el-form-item>
        <el-form-item :label="t('app.mcpAggregation.environment')" required><el-select v-model="form.environment" class="full-width"><el-option value="dev" :label="t('app.mcpAggregation.dev')" /><el-option value="test" :label="t('app.mcpAggregation.test')" /><el-option value="prod" :label="t('app.mcpAggregation.prod')" /></el-select></el-form-item>
        <el-form-item :label="t('app.mcpAggregation.targetCatalog')"><el-select v-model="form.target_catalog_id" class="full-width" clearable :placeholder="t('app.mcpAggregation.empty')"><el-option v-for="catalog in store.catalogs" :key="catalog.id" :value="catalog.id" :label="`${catalog.display_name} (${catalog.catalog_key})`" /></el-select></el-form-item>
        <el-form-item :label="t('app.mcpAggregation.publishPolicy')" required><el-select v-model="form.publish_policy" class="full-width"><el-option value="auto_if_l1" :label="t('app.mcpAggregation.autoIfL1')" /><el-option value="approval_required" :label="t('app.mcpAggregation.approvalRequired')" /></el-select></el-form-item>
        <el-form-item><el-button type="primary" native-type="submit" :loading="submitting">{{ t('app.mcpAggregation.submit') }}</el-button><el-button @click="onboardingVisible = false">{{ t('app.mcpAggregation.cancel') }}</el-button></el-form-item>
      </el-form>
    </el-drawer>

    <el-drawer v-model="clientEndpointVisible" :title="t('app.mcpAggregation.createClientEndpoint')" size="560px" destroy-on-close>
      <el-form :model="clientEndpointForm" label-position="top" @submit.prevent="submitClientEndpoint">
        <el-form-item :label="t('app.mcpAggregation.clientKey')" required><el-input v-model="clientEndpointForm.client_key" maxlength="80" placeholder="codex-aegis" /></el-form-item>
        <el-form-item :label="t('app.mcpAggregation.clientName')" required><el-input v-model="clientEndpointForm.display_name" maxlength="160" /></el-form-item>
        <el-form-item :label="t('app.mcpAggregation.boundService')" required>
          <el-select v-model="clientEndpointForm.server_id" class="full-width" @change="onEndpointServerChange">
            <el-option v-for="server in publishedServers" :key="server.id" :value="server.id" :label="server.display_name" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('app.mcpAggregation.toolControl')">
          <div v-if="endpointToolOptions.length" class="endpoint-tool-options">
            <div v-for="tool in endpointToolOptions" :key="tool.alias" class="endpoint-tool-option">
              <div><strong>{{ tool.alias }}</strong><small>{{ tool.description || tool.title || '—' }}</small></div>
              <el-switch v-model="endpointEnabledTools[tool.alias]" />
            </div>
          </div>
          <el-empty v-else :description="t('app.mcpAggregation.noPublishedTools')" :image-size="72" />
        </el-form-item>
        <el-alert :title="t('app.mcpAggregation.endpointTokenOnce')" type="info" show-icon :closable="false" />
        <div class="drawer-actions"><el-button type="primary" native-type="submit" :loading="clientEndpointSubmitting">{{ t('app.mcpAggregation.generateEndpoint') }}</el-button><el-button @click="clientEndpointVisible = false">{{ t('app.mcpAggregation.cancel') }}</el-button></div>
      </el-form>
    </el-drawer>

    <el-drawer v-model="endpointToolsVisible" :title="selectedEndpoint ? `${selectedEndpoint.display_name} · ${t('app.mcpAggregation.toolControl')}` : t('app.mcpAggregation.toolControl')" size="520px">
      <template v-if="selectedEndpoint">
        <div class="endpoint-detail"><span>{{ t('app.mcpAggregation.endpoint') }}</span><code>{{ selectedEndpoint.endpoint }}</code></div>
        <div v-for="tool in selectedEndpoint.tools" :key="tool.alias" class="endpoint-tool-option">
          <div><strong>{{ tool.alias }}</strong><small>{{ tool.description || tool.title || '—' }}</small></div>
          <el-switch v-model="tool.enabled" :loading="toolUpdating === tool.alias" @change="toggleEndpointTool(tool)" />
        </div>
      </template>
    </el-drawer>

    <el-dialog v-model="createdEndpointVisible" :title="t('app.mcpAggregation.endpointCreated')" width="620px" destroy-on-close>
      <template v-if="createdEndpoint">
        <el-alert :title="t('app.mcpAggregation.endpointTokenOnce')" type="warning" show-icon :closable="false" />
        <el-descriptions :column="1" border class="created-endpoint-details">
          <el-descriptions-item :label="t('app.mcpAggregation.endpoint')"><div class="copy-line"><code>{{ createdEndpoint.endpoint }}</code><el-button link type="primary" @click="copyText(createdEndpoint.endpoint)">{{ t('app.mcpAggregation.copy') }}</el-button></div></el-descriptions-item>
          <el-descriptions-item :label="t('app.mcpAggregation.token')"><div class="copy-line"><code>{{ createdEndpoint.token }}</code><el-button link type="primary" @click="copyText(createdEndpoint.token)">{{ t('app.mcpAggregation.copy') }}</el-button></div></el-descriptions-item>
        </el-descriptions>
      </template>
      <template #footer><el-button type="primary" @click="createdEndpointVisible = false">{{ t('app.mcpAggregation.done') }}</el-button></template>
    </el-dialog>

    <el-drawer :model-value="Boolean(selectedServer)" title="MCP Server" size="720px" @close="selectedServer = null">
      <template v-if="selectedServer"><el-descriptions :column="1" border><el-descriptions-item label="ID">{{ selectedServer.id }}</el-descriptions-item><el-descriptions-item :label="t('app.mcpAggregation.serviceName')">{{ selectedServer.display_name }}</el-descriptions-item><el-descriptions-item :label="t('app.mcpAggregation.endpoint')">{{ selectedServer.endpoint_display }}</el-descriptions-item><el-descriptions-item :label="t('app.mcpAggregation.status')">{{ statusLabel(selectedServer.lifecycle_status) }}</el-descriptions-item><el-descriptions-item :label="t('app.mcpAggregation.risk')">{{ selectedServer.risk_tier.toUpperCase() }}</el-descriptions-item></el-descriptions></template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Connection, Plus, Setting } from '@element-plus/icons-vue'
import type { MCPApprovalDecisionStatus } from '@/api/mcpAggregation'
import { useMCPAggregationStore } from '@/store/mcpAggregation'
import type { MCPClientEndpoint, MCPClientEndpointCreated, MCPOnboardingPayload, MCPServer, MCPToolRevision } from '@/types/mcpAggregation'
import { getStoredAuth } from '@/utils/auth'

const { t } = useI18n()
const store = useMCPAggregationStore()
const activeTab = ref('servers')
const onboardingVisible = ref(false)
const clientEndpointVisible = ref(false)
const endpointToolsVisible = ref(false)
const createdEndpointVisible = ref(false)
const submitting = ref(false)
const clientEndpointSubmitting = ref(false)
const selectedServer = ref<MCPServer | null>(null)
const selectedEndpoint = ref<MCPClientEndpoint | null>(null)
const createdEndpoint = ref<MCPClientEndpointCreated | null>(null)
const approvalSubmitting = ref('')
const toolUpdating = ref('')
const clientEndpointForm = reactive({ client_key: '', display_name: '', server_id: '' })
const endpointEnabledTools = reactive<Record<string, boolean>>({})
const currentUsername = computed(() => getStoredAuth()?.username || '')
const currentRole = computed(() => getStoredAuth()?.role || '')
const filters = reactive({ keyword: '', environment: '', status: '', risk_tier: '' })
const form = reactive<MCPOnboardingPayload>({ display_name: '', endpoint_url: '', auth_type: 'oauth2', credential_ref: '', environment: 'test', publish_policy: 'approval_required' })
let refreshTimer: ReturnType<typeof setInterval> | undefined

const toolRows = computed(() => store.tools.map(tool => {
  const server = store.servers.find(item => item.active_revision_id === tool.server_revision_id)
  return {
    ...tool,
    serverName: server?.display_name || '—',
    serverRevisionId: tool.server_revision_id,
  }
}))
const publishedServers = computed(() => store.servers.filter(server => server.lifecycle_status === 'published' && server.active_revision_id))
const endpointToolOptions = computed(() => {
  if (!clientEndpointForm.server_id) return [] as MCPToolRevision[]
  const server = store.servers.find(item => item.id === clientEndpointForm.server_id)
  return store.tools.filter(tool => tool.server_revision_id === server?.active_revision_id && tool.status === 'approved')
})

const metrics = computed(() => [
  { key: 'servers', tab: 'servers', label: t('app.mcpAggregation.remoteServers'), value: store.overview?.remote_servers ?? '—' },
  { key: 'tools', tab: 'tools', label: t('app.mcpAggregation.publishedTools'), value: store.overview?.published_tools ?? '—' },
  { key: 'clients', tab: 'clients', label: t('app.mcpAggregation.activeClients'), value: store.overview?.active_clients ?? '—' },
  { key: 'approvals', tab: 'approvals', label: t('app.mcpAggregation.pendingApprovals'), value: store.overview?.pending_approvals ?? '—' },
  { key: 'risk', tab: 'security', label: t('app.mcpAggregation.highRiskCalls'), value: store.overview?.high_risk_calls_24h ?? '—' },
])

async function load() {
  try { await store.loadPrimary({ ...filters }) } catch { /* store contains safe error state */ }
}

async function onTabChange(name: string | number) {
  try { await store.loadTab(String(name)) } catch { /* store exposes the primary error state */ }
}

function resetFilters() { filters.keyword = ''; filters.environment = ''; filters.status = ''; filters.risk_tier = ''; load() }

function openClientEndpoint() {
  clientEndpointForm.client_key = ''
  clientEndpointForm.display_name = ''
  clientEndpointForm.server_id = publishedServers.value[0]?.id || ''
  Object.keys(endpointEnabledTools).forEach(key => delete endpointEnabledTools[key])
  onEndpointServerChange()
  clientEndpointVisible.value = true
}

function onEndpointServerChange() {
  Object.keys(endpointEnabledTools).forEach(key => delete endpointEnabledTools[key])
  endpointToolOptions.value.forEach(tool => { endpointEnabledTools[tool.alias] = true })
}

function enabledToolCount(row: MCPClientEndpoint) { return row.tools.filter(tool => tool.enabled).length }

function openEndpointTools(row: MCPClientEndpoint) {
  selectedEndpoint.value = row
  endpointToolsVisible.value = true
}

async function toggleEndpointTool(tool: { alias: string; enabled: boolean }) {
  if (!selectedEndpoint.value) return
  const previous = !tool.enabled
  toolUpdating.value = tool.alias
  try {
    const aliases = selectedEndpoint.value.tools.filter(item => item.enabled).map(item => item.alias)
    const updated = await store.updateClientEndpointTools(selectedEndpoint.value.grant_id, aliases)
    selectedEndpoint.value = updated
    ElMessage.success(t('app.mcpAggregation.toolUpdated'))
  } catch (error) {
    tool.enabled = previous
    ElMessage.error(error instanceof Error ? error.message : t('app.mcpAggregation.toolUpdateFailed'))
  } finally { toolUpdating.value = '' }
}

async function submitClientEndpoint() {
  if (!clientEndpointForm.client_key.trim() || !clientEndpointForm.display_name.trim() || !clientEndpointForm.server_id) {
    ElMessage.warning(t('app.mcpAggregation.clientEndpointRequired'))
    return
  }
  clientEndpointSubmitting.value = true
  try {
    const result = await store.createClientEndpoint({
      client_key: clientEndpointForm.client_key.trim(),
      display_name: clientEndpointForm.display_name.trim(),
      client_type: 'service',
      server_id: clientEndpointForm.server_id,
      tool_allowlist: Object.keys(endpointEnabledTools).filter(alias => endpointEnabledTools[alias]),
    })
    createdEndpoint.value = result
    clientEndpointVisible.value = false
    createdEndpointVisible.value = true
    activeTab.value = 'clients'
    await store.loadTab('clients')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : t('app.mcpAggregation.clientEndpointCreateFailed'))
  } finally { clientEndpointSubmitting.value = false }
}

async function copyText(value: string) {
  try { await navigator.clipboard.writeText(value); ElMessage.success(t('app.mcpAggregation.copied')) } catch { ElMessage.warning(t('app.mcpAggregation.copyFailed')) }
}

async function submitOnboarding() {
  if (!form.display_name.trim() || !form.endpoint_url.trim()) { ElMessage.warning(t('app.mcpAggregation.requiredFields')); return }
  submitting.value = true
  try {
    await store.createOnboarding({ ...form, credential_ref: form.credential_ref || undefined })
    ElMessage.success(t('app.mcpAggregation.onboardingCreated'))
    form.credential_ref = ''
    onboardingVisible.value = false
    await load()
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : t('app.mcpAggregation.onboardingFailed')) } finally { submitting.value = false }
}

function formatTime(value: string) { return new Date(value).toLocaleString() }
function formatSchema(value: Record<string, unknown>) {
  const serialized = JSON.stringify(value || {})
  return serialized.length > 180 ? `${serialized.slice(0, 177)}...` : serialized
}
function riskTag(value: string) { return value === 'l4' ? 'danger' : value === 'l3' ? 'warning' : value === 'l2' ? 'primary' : 'success' }
function statusTag(value: string) { return ['failed', 'quarantined', 'suspended', 'rejected'].includes(value) ? 'danger' : ['pending', 'awaiting_approval', 'review_required', 'drift_detected'].includes(value) ? 'warning' : value === 'active' || value === 'published' ? 'success' : 'info' }
function statusLabel(value: string) {
  const labels: Record<string, string> = {
    created: '已创建', validating_endpoint: '校验端点', awaiting_auth: '等待认证', authenticating: '认证中',
    discovering: '发现工具', validating_tools: '校验工具', security_scanning: '安全扫描', classifying: '风险分类',
    building_release: '构建发布', awaiting_approval: '待审批', publishing: '发布中', active: '已生效',
    approved: '已审核', published: '已发布', pending: '待审批', rejected: '已拒绝', cancelled: '已取消',
    draft: '草稿', review_required: '待审核', quarantined: '已隔离', suspended: '已暂停', drift_detected: '检测到漂移',
    failed: '失败', retired: '已退役', discovered: '已发现',
  }
  return labels[value] || value || '—'
}
function approvalTypeLabel(value: string) { return value === 'admission' ? t('app.mcpAggregation.approvalTypeAdmission') : statusLabel(value) }
function approvalSubjectLabel(value: string) { return value === 'server_revision' ? t('app.mcpAggregation.subjectTypeServerRevision') : value }
function canDecideApproval(row: { requested_by: string }) {
  return Boolean(currentUsername.value) && (currentRole.value === 'admin' || currentUsername.value === 'admin' || currentUsername.value !== row.requested_by)
}

async function decideApproval(row: { id: string; status: string; request_digest?: string }, status: MCPApprovalDecisionStatus) {
  if (!row.request_digest) {
    ElMessage.error('审批摘要缺失，无法提交审批')
    return
  }
  try {
    await ElMessageBox.confirm(
      status === 'approved' ? t('app.mcpAggregation.approveConfirm') : t('app.mcpAggregation.rejectConfirm'),
      t('app.mcpAggregation.approvalConfirm'),
      { type: status === 'approved' ? 'success' : 'warning' },
    )
    const result = await ElMessageBox.prompt(
      t('app.mcpAggregation.approvalReason'),
      t('app.mcpAggregation.approvalConfirm'),
      {
        inputPlaceholder: t('app.mcpAggregation.approvalReasonPlaceholder'),
        inputValidator: (value: string) => value.trim() ? true : t('app.mcpAggregation.approvalReasonRequired'),
      },
    )
    approvalSubmitting.value = row.id
    await store.decideApproval(row.id, status, row.request_digest, result.value.trim())
    await Promise.all([load(), store.loadTab('approvals')])
    ElMessage.success(t('app.mcpAggregation.approvalSucceeded'))
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    // Axios displays the localized API error once in its response interceptor.
    // Avoid showing the same server rejection a second time here.
    if (!(error instanceof Error)) ElMessage.error('审批操作失败')
  } finally {
    approvalSubmitting.value = ''
  }
}

onMounted(() => {
  load()
  refreshTimer = setInterval(load, 10_000)
})
onBeforeUnmount(() => { if (refreshTimer) clearInterval(refreshTimer) })
</script>

<style scoped>
.mcp-hero { margin-bottom: 18px; }
.hero-actions { display: flex; flex-wrap: wrap; gap: 10px; }
.hero-kicker { display: inline-flex; margin-bottom: 8px; color: #0891b2; font-size: 12px; font-weight: 800; letter-spacing: .12em; text-transform: uppercase; }
.metric-grid { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 12px; margin-bottom: 18px; }
.metric-card { display: grid; gap: 8px; border: 1px solid rgba(37, 99, 235, .12); border-radius: 16px; background: #fff; padding: 16px; text-align: left; cursor: pointer; transition: transform .2s ease, box-shadow .2s ease; }
.metric-card:hover { transform: translateY(-2px); box-shadow: 0 12px 28px rgba(15, 23, 42, .08); }
.metric-label, .metric-card small { color: #64748b; font-size: 12px; }
.metric-card strong { color: #0f172a; font-size: 28px; }
.filter-bar { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 16px; }
.filter-bar .el-input { width: 220px; }
.filter-bar .el-select { width: 140px; }
.mcp-table { min-height: 260px; }
.tool-table { min-width: 1180px; }
.tool-service { color: #0f172a; font-weight: 700; }
.tool-revision { display: block; margin-top: 4px; color: #94a3b8; font-size: 11px; }
.approval-blocked { color: #94a3b8; font-size: 12px; }
.client-endpoint-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 14px; }
.client-endpoint-toolbar strong, .client-endpoint-toolbar span { display: block; }
.client-endpoint-toolbar span { margin-top: 4px; color: #64748b; font-size: 12px; }
.endpoint-tool-options { width: 100%; }
.endpoint-tool-option { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 0; border-bottom: 1px solid #eef2f7; }
.endpoint-tool-option strong, .endpoint-tool-option small { display: block; }
.endpoint-tool-option small { max-width: 390px; margin-top: 4px; color: #64748b; line-height: 1.4; }
.endpoint-detail { display: grid; gap: 6px; margin-bottom: 12px; padding: 12px; border-radius: 10px; background: #f8fafc; }
.endpoint-detail span { color: #64748b; font-size: 12px; }
.endpoint-detail code, .copy-line code { word-break: break-all; color: #0f172a; }
.drawer-actions { display: flex; gap: 10px; margin-top: 18px; }
.created-endpoint-details { margin-top: 16px; }
.copy-line { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.state-alert { margin-bottom: 14px; }
.field-hint { margin-top: 6px; color: #64748b; font-size: 12px; line-height: 1.5; }
.full-width { width: 100%; }
@media (max-width: 1100px) { .metric-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 720px) { .metric-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .filter-bar .el-input, .filter-bar .el-select { width: 100%; } .client-endpoint-toolbar { align-items: flex-start; flex-direction: column; } .hero-actions { width: 100%; } }
</style>
