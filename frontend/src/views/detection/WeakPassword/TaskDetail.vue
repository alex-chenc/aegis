<template>
  <div class="weak-task-page">
    <div class="page-toolbar">
      <div>
        <h1>{{ store.currentTask?.name || '弱密码任务详情' }}</h1>
        <p>{{ store.currentTask ? weakPasswordStatusLabel(store.currentTask.status) : '加载中' }}</p>
      </div>
      <div class="toolbar-actions">
        <el-button :icon="Back" @click="router.push('/risk/weak-password')">返回</el-button>
        <el-button :icon="Refresh" @click="loadDetail">刷新</el-button>
        <el-button v-if="store.currentTask" type="danger" plain :disabled="!canDeleteTask(store.currentTask.status)" @click="deleteTask">删除</el-button>
        <el-button v-if="store.currentTask?.status === 'failed'" type="primary" @click="retryFailed">重试失败项</el-button>
      </div>
    </div>

    <section class="panel">
      <div class="progress-grid">
        <div class="progress-main">
          <el-progress :percentage="store.progress?.progress || 0" :stroke-width="14" />
          <div class="stage-list">
            <span v-for="stage in stages" :key="stage">{{ stage }}</span>
          </div>
        </div>
        <div class="metric">
          <span>当前应用</span>
          <strong>{{ store.progress?.current_application || '-' }}</strong>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>主机执行</h2>
      </div>
      <el-table v-loading="store.loading" :data="store.hosts" class="dense-table">
        <el-table-column label="IP" min-width="150">
          <template #default="{ row }">
            <div class="primary-cell">{{ row.ip_address || '-' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="主机名称" min-width="180">
          <template #default="{ row }">
            <div class="primary-cell">{{ row.hostname || row.host_id }}</div>
          </template>
        </el-table-column>
        <el-table-column label="Agent 状态" width="120">
          <template #default="{ row }">{{ weakPasswordStatusLabel(row.agent_status) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="150">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)">{{ weakPasswordStatusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="采集记录" prop="collected_records" width="120" />
        <el-table-column label="命中" prop="matched_findings" width="100" />
        <el-table-column label="失败原因" min-width="240">
          <template #default="{ row }">
            <div>{{ weakPasswordErrorCodeLabel(row.error_code) }}</div>
            <div v-if="row.error_message" class="secondary-cell">{{ row.error_message }}</div>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="store.hostTotal > 0" class="pagination-bar">
        <el-pagination
          v-model:current-page="store.hostFilters.page"
          v-model:page-size="store.hostFilters.page_size"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          :total="store.hostTotal"
          @size-change="loadDetail"
          @current-change="loadDetail"
        />
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>命中结果</h2>
      </div>
      <el-table :data="store.findings" class="dense-table">
        <el-table-column label="应用" prop="application_name" min-width="140" />
        <el-table-column label="账号" prop="account" min-width="120" />
        <el-table-column label="凭据类型" width="150">
          <template #default="{ row }">{{ weakPasswordCredentialTypeLabel(row.credential_type) }}</template>
        </el-table-column>
        <el-table-column label="命中密码" width="150">
          <template #default="{ row }">
            <span class="password-mask">{{ row.matched_password_mask }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="180">
          <template #default="{ row }">
            <el-tag :type="row.match_status === 'confirmed' ? 'success' : 'warning'">{{ weakPasswordMatchStatusLabel(row.match_status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="配置来源" min-width="220">
          <template #default="{ row }">
            <div class="secondary-cell">{{ row.source_path }}</div>
            <div class="secondary-cell">{{ row.field_path }}</div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="showFindingDetail(row.id)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="store.findingTotal > 0" class="pagination-bar">
        <el-pagination
          v-model:current-page="store.findingFilters.page"
          v-model:page-size="store.findingFilters.page_size"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          :total="store.findingTotal"
          @size-change="loadDetail"
          @current-change="loadDetail"
        />
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>采集进度</h2>
      </div>
      <el-table :data="store.errors" class="dense-table">
        <el-table-column label="应用" prop="application_name" min-width="140" />
        <el-table-column label="轮次" prop="round" width="90" />
        <el-table-column label="工具" min-width="230">
          <template #default="{ row }">{{ weakPasswordToolNameLabel(row.tool_name) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="130">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)">{{ weakPasswordStatusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="错误码" min-width="170">
          <template #default="{ row }">{{ weakPasswordErrorCodeLabel(row.error_code) }}</template>
        </el-table-column>
        <el-table-column label="耗时" width="110">
          <template #default="{ row }">{{ row.execution_time_ms || 0 }}ms</template>
        </el-table-column>
        <el-table-column label="说明" min-width="240">
          <template #default="{ row }">{{ weakPasswordErrorMessageLabel(row.error_message, row.error_code) }}</template>
        </el-table-column>
      </el-table>
      <div v-if="store.errorTotal > 10" class="pagination-bar">
        <el-pagination
          v-model:current-page="store.errorFilters.page"
          v-model:page-size="store.errorFilters.page_size"
          :page-sizes="[10]"
          :pager-count="10"
          layout="total, prev, pager, next"
          :total="store.errorTotal"
          @size-change="loadDetail"
          @current-change="loadDetail"
        />
      </div>
    </section>

    <el-dialog v-model="passwordDialogVisible" title="命中密码详情" width="460px" destroy-on-close>
      <div v-if="revealedFinding" class="password-detail">
        <div class="fact-row"><span>应用</span><strong>{{ revealedFinding.application_name }}</strong></div>
        <div class="fact-row"><span>账号</span><strong>{{ revealedFinding.account || '-' }}</strong></div>
        <div class="fact-row"><span>凭据类型</span><strong>{{ weakPasswordCredentialTypeLabel(revealedFinding.credential_type) }}</strong></div>
        <div class="fact-row password-row"><span>完整密码</span><code>{{ revealedFinding.matched_password }}</code></div>
        <div class="fact-row"><span>配置来源</span><strong>{{ revealedFinding.source_path || '-' }}</strong></div>
      </div>
      <template #footer>
        <el-button type="primary" @click="passwordDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Back, Refresh } from '@element-plus/icons-vue'
import { revealWeakPasswordFinding } from '@/api/weakPassword'
import { useWeakPasswordStore } from '@/store/weakPassword'
import type { RevealedWeakPasswordFinding } from '@/types/weakPassword'
import {
  weakPasswordCredentialTypeLabel,
  weakPasswordErrorCodeLabel,
  weakPasswordErrorMessageLabel,
  weakPasswordMatchStatusLabel,
  weakPasswordStatusLabel,
  weakPasswordToolNameLabel,
} from '@/utils/weakPasswordLabels'

const route = useRoute()
const router = useRouter()
const store = useWeakPasswordStore()
let timer: number | undefined
const passwordDialogVisible = ref(false)
const revealedFinding = ref<RevealedWeakPasswordFinding | null>(null)

const stages = ['资产分析', '连接主机', '读取配置', '密码匹配', '结果入库']
const taskId = computed(() => String(route.params.id || ''))

async function loadDetail() {
  if (taskId.value) {
    await store.fetchTaskDetail(taskId.value)
  }
}

async function retryFailed() {
  await store.retryFailed(taskId.value)
  ElMessage.success('已提交重试')
}

async function deleteTask() {
  if (!store.currentTask || !canDeleteTask(store.currentTask.status)) return
  try {
    await ElMessageBox.confirm('确定删除此弱密码检测任务吗？', '删除任务', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
    })
    await store.deleteTask(store.currentTask.id)
    ElMessage.success('已删除任务')
    router.push('/risk/weak-password')
  } catch {
    // user cancelled
  }
}

async function showFindingDetail(findingId: string) {
  try {
    const password = await ElMessageBox.prompt('请输入当前系统密码', '查看命中密码', {
      confirmButtonText: '查看',
      cancelButtonText: '取消',
      inputType: 'password',
      inputPattern: /.+/,
      inputErrorMessage: '系统密码不能为空',
    })
    revealedFinding.value = await revealWeakPasswordFinding(findingId, password.value)
    passwordDialogVisible.value = true
  } catch {
    // user cancelled
  }
}

function canDeleteTask(status: string) {
  return !['pending', 'analyzing_assets', 'collecting_credentials', 'repairing_collection', 'matching'].includes(status)
}

function statusType(status: string) {
  if (status === 'completed') return 'success'
  if (['failed', 'partial_failed'].includes(status)) return 'danger'
  if (['matching', 'collecting', 'collecting_credentials', 'repairing', 'repairing_collection', 'analyzing_assets', 'pending', 'executing'].includes(status)) return 'warning'
  return 'info'
}

onMounted(() => {
  loadDetail()
  timer = window.setInterval(loadDetail, 5000)
})

onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<style scoped>
.weak-task-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.page-toolbar,
.panel-head,
.toolbar-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.toolbar-actions {
  justify-content: flex-end;
}

.page-toolbar h1,
.panel-head h2 {
  margin: 0;
  color: #0f172a;
}

.page-toolbar p,
.secondary-cell {
  color: #64748b;
}

.panel {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 14px;
}

.progress-grid {
  display: grid;
  grid-template-columns: minmax(360px, 1fr) 220px;
  gap: 14px;
  align-items: stretch;
}

.progress-main,
.metric {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 12px;
}

.stage-list {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 6px;
  margin-top: 10px;
  color: #64748b;
  font-size: 12px;
}

.metric {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 6px;
}

.metric span {
  color: #64748b;
  font-size: 12px;
}

.metric strong {
  color: #0f172a;
  font-size: 18px;
}

.dense-table {
  width: 100%;
}

.pagination-bar {
  display: flex;
  justify-content: flex-end;
}

.primary-cell {
  font-weight: 700;
  color: #0f172a;
}

.password-mask {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  letter-spacing: 0;
}

.password-detail {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.fact-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 10px 0;
  border-bottom: 1px solid #e2e8f0;
}

.fact-row span {
  color: #64748b;
}

.fact-row strong,
.fact-row code {
  color: #0f172a;
  text-align: right;
  word-break: break-all;
}

.password-row code {
  padding: 6px 8px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>
