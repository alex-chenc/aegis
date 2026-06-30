<template>
  <div class="dashboard page-shell">
    <section class="page-hero dashboard-hero">
      <h1>主机资产态势</h1>
      <p>集中查看 Agent 在线状态、主机身份和最后心跳，快速定位失联节点与需要处置的资产。</p>
    </section>

    <div class="metric-grid">
      <div class="metric-card">
        <div class="metric-label">总主机</div>
        <div class="metric-value">{{ hosts.length }}</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">在线 Agent</div>
        <div class="metric-value">{{ onlineCount }}</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">离线节点</div>
        <div class="metric-value">{{ offlineCount }}</div>
      </div>
    </div>

    <el-card>
      <template #header>
        <div class="card-header">
          <span>主机列表</span>
          <el-button @click="refresh" :loading="loading">刷新</el-button>
        </div>
      </template>
      
      <el-table :data="hosts" v-loading="loading" style="width: 100%">
        <el-table-column prop="ip_address" label="IP 地址">
          <template #default="{ row }">
            <el-link type="primary" @click="openAssetDrawer(row)">{{ row.ip_address }}</el-link>
          </template>
        </el-table-column>
        <el-table-column prop="hostname" label="主机名" />
        <el-table-column prop="os_type" label="系统类型" />
        <el-table-column prop="agent_version" label="Agent 版本" />
        <el-table-column prop="last_heartbeat_at" label="最后心跳" />
        <el-table-column label="状态">
          <template #default="{ row }">
            <span class="status-pill" :class="row.online ? 'status-online' : 'status-offline'">
              {{ row.online ? '在线' : '离线' }}
            </span>
          </template>
        </el-table-column>
      </el-table>
      
      <el-empty v-if="!loading && hosts.length === 0" description="暂无数据" />
    </el-card>

    <el-drawer
      v-model="assetDrawerVisible"
      size="720px"
      direction="rtl"
      class="asset-drawer"
    >
      <template #header>
        <div class="asset-drawer-title">
          <span>{{ selectedHost?.hostname || '资产详情' }}</span>
          <small>{{ selectedHost?.ip_address }}</small>
        </div>
      </template>

      <div v-if="selectedHost" class="asset-detail-layout">
        <aside class="asset-nav" aria-label="资产分类">
          <button
            v-for="item in assetNavItems"
            :key="item.key"
            type="button"
            :class="{ active: activeAssetSection === item.key }"
            @click="activeAssetSection = item.key"
          >
            <span>{{ item.label }}</span>
            <em v-if="item.loading">加载中</em>
            <em v-else>{{ item.total }}</em>
          </button>
        </aside>

        <main class="asset-detail-main">
          <section v-if="activeAssetSection === 'software'" class="asset-section">
          <div class="asset-section-header">
            <div>
              <h3>软件清单</h3>
              <span>{{ softwareSection.total }} 项</span>
            </div>
          </div>
          <el-skeleton v-if="softwareSection.loading" :rows="3" animated />
          <div v-else-if="softwareSection.items.length" class="asset-list">
            <div v-for="item in softwareSection.items" :key="item.id" class="asset-item">
              <div class="asset-item-main">
                <strong>{{ item.name }}</strong>
                <span>{{ item.version || 'unknown' }}</span>
              </div>
              <div class="asset-item-meta">
                <el-tag size="small" effect="plain">{{ item.package_manager || '-' }}</el-tag>
                <span>{{ formatTime(item.collected_at) }}</span>
              </div>
            </div>
          </div>
          <el-empty v-else description="暂无软件资产" :image-size="72" />
          <el-pagination
            v-if="softwareSection.total > assetPageSize"
            v-model:current-page="softwareSection.page"
            :page-size="assetPageSize"
            :total="softwareSection.total"
            layout="prev, pager, next"
            small
            @current-change="loadSoftwareSection"
          />
        </section>

        <section v-else-if="activeApplicationSection" class="asset-section">
          <div class="asset-section-header">
            <div>
              <h3>{{ activeApplicationSection.label }}</h3>
              <span>{{ activeApplicationSection.total }} 项</span>
            </div>
          </div>
          <el-skeleton v-if="activeApplicationSection.loading" :rows="3" animated />
          <div v-else-if="activeApplicationSection.items.length" class="asset-list">
            <div v-for="item in activeApplicationSection.items" :key="item.id" class="asset-item">
              <div class="asset-item-main">
                <strong>{{ item.display_name || item.name }}</strong>
                <span>{{ item.version || 'unknown' }}</span>
              </div>
              <div class="asset-item-meta">
                <el-tag size="small" :type="item.is_container ? 'success' : 'info'" effect="plain">
                  {{ item.is_container ? item.container_runtime || 'container' : item.category }}
                </el-tag>
                <span v-if="item.listen_ports?.length">端口 {{ item.listen_ports.slice(0, 4).join(', ') }}</span>
                <span v-else>{{ item.run_user || '-' }}</span>
              </div>
            </div>
          </div>
          <el-empty v-else :description="`暂无${activeApplicationSection.label}`" :image-size="72" />
          <el-pagination
            v-if="activeApplicationSection.total > assetPageSize"
            v-model:current-page="activeApplicationSection.page"
            :page-size="assetPageSize"
            :total="activeApplicationSection.total"
            layout="prev, pager, next"
            small
            @current-change="loadActiveApplicationSection"
          />
        </section>
        </main>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useHostStore } from '@/store/hosts'
import {
  listApplicationAssets,
  listSoftwareAssets,
  type ApplicationAsset,
  type SoftwareAsset
} from '@/api/assets'
import type { Host } from '@/types'

interface ApplicationSection {
  key: string
  label: string
  category: string
  items: ApplicationAsset[]
  total: number
  page: number
  loading: boolean
}

const hostStore = useHostStore()
const { hosts, loading } = storeToRefs(hostStore)
const onlineCount = computed(() => hosts.value.filter((host: any) => host.online).length)
const offlineCount = computed(() => Math.max(hosts.value.length - onlineCount.value, 0))
const assetPageSize = 10
const assetDrawerVisible = ref(false)
const selectedHost = ref<Host | null>(null)
const activeAssetSection = ref('software')

const softwareSection = reactive({
  items: [] as SoftwareAsset[],
  total: 0,
  page: 1,
  loading: false
})

const applicationSections = reactive<ApplicationSection[]>([
  { key: 'database', label: '数据库', category: 'database', items: [] as ApplicationAsset[], total: 0, page: 1, loading: false },
  { key: 'web_service', label: 'Web 服务', category: 'web_service', items: [] as ApplicationAsset[], total: 0, page: 1, loading: false },
  { key: 'web_site', label: 'Web 站点', category: 'web_site', items: [] as ApplicationAsset[], total: 0, page: 1, loading: false },
  { key: 'web_framework', label: 'Web 框架', category: 'web_framework', items: [] as ApplicationAsset[], total: 0, page: 1, loading: false },
  { key: 'llm_service', label: 'AI LLM', category: 'llm_service', items: [] as ApplicationAsset[], total: 0, page: 1, loading: false },
  { key: 'ai_agent', label: 'AI Agent', category: 'ai_agent', items: [] as ApplicationAsset[], total: 0, page: 1, loading: false },
  { key: 'mcp_server', label: 'MCP', category: 'mcp_server', items: [] as ApplicationAsset[], total: 0, page: 1, loading: false }
])

const activeApplicationSection = computed(() =>
  applicationSections.find(section => section.key === activeAssetSection.value) || null
)

const assetNavItems = computed(() => [
  {
    key: 'software',
    label: '软件清单',
    total: softwareSection.total,
    loading: softwareSection.loading
  },
  ...applicationSections.map(section => ({
    key: section.key,
    label: section.label,
    total: section.total,
    loading: section.loading
  }))
])

const refresh = () => {
  hostStore.fetchHosts()
}

onMounted(() => {
  refresh()
})

async function openAssetDrawer(host: Host) {
  selectedHost.value = host
  assetDrawerVisible.value = true
  activeAssetSection.value = 'software'
  softwareSection.page = 1
  applicationSections.forEach(section => {
    section.page = 1
    section.items = []
    section.total = 0
  })
  await Promise.all([
    loadSoftwareSection(),
    ...applicationSections.map(section => loadApplicationSection(section))
  ])
}

async function loadSoftwareSection() {
  if (!selectedHost.value) return
  softwareSection.loading = true
  try {
    const result = await listSoftwareAssets({
      host_id: selectedHost.value.id,
      page: softwareSection.page,
      page_size: assetPageSize
    })
    softwareSection.items = result.items
    softwareSection.total = result.total
  } finally {
    softwareSection.loading = false
  }
}

async function loadApplicationSection(section: ApplicationSection) {
  if (!selectedHost.value) return
  section.loading = true
  try {
    const result = await listApplicationAssets({
      host_id: selectedHost.value.id,
      category: section.category,
      page: section.page,
      page_size: assetPageSize
    })
    section.items = result.items
    section.total = result.total
  } finally {
    section.loading = false
  }
}

async function loadActiveApplicationSection() {
  if (!activeApplicationSection.value) return
  await loadApplicationSection(activeApplicationSection.value)
}

function formatTime(time: string) {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
}
</script>

<style scoped>
.dashboard-hero {
  margin-bottom: 0;
}

.status-online {
  color: #047857;
  background: rgba(16, 185, 129, 0.1);
}

.status-offline {
  color: #be123c;
  background: rgba(225, 29, 72, 0.1);
}

.asset-drawer-title {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-weight: 700;
}

.asset-drawer-title small {
  color: var(--aegis-text-muted);
  font-weight: 500;
}

.asset-detail-layout {
  display: grid;
  grid-template-columns: 170px minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.asset-nav {
  position: sticky;
  top: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.asset-nav button {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  min-height: 42px;
  padding: 9px 10px;
  border: 1px solid transparent;
  border-radius: 8px;
  color: var(--aegis-text);
  background: #f8fafc;
  cursor: pointer;
  text-align: left;
}

.asset-nav button.active {
  border-color: rgba(37, 99, 235, 0.28);
  color: var(--aegis-action-blue);
  background: #eff6ff;
}

.asset-nav span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.asset-nav em {
  color: var(--aegis-text-muted);
  font-size: 12px;
  font-style: normal;
}

.asset-detail-main {
  min-width: 0;
}

.asset-section {
  padding: 16px;
  border: 1px solid var(--aegis-border);
  border-radius: 8px;
  background: #fff;
}

.asset-section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.asset-section h3 {
  margin: 0 0 4px;
  font-size: 15px;
}

.asset-section-header span {
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.asset-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}

.asset-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  background: #f8fafc;
}

.asset-item-main,
.asset-item-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.asset-item-main strong {
  overflow: hidden;
  color: var(--aegis-text);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.asset-item-main span,
.asset-item-meta span {
  color: var(--aegis-text-muted);
  font-size: 12px;
}

@media (max-width: 760px) {
  .asset-detail-layout {
    grid-template-columns: 1fr;
  }

  .asset-nav {
    position: static;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
