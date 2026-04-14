<template>
  <div class="detection-rules-page">
    <!-- AI规则配置面板 -->
    <AIConfigPanel v-if="showAIConfig" />

    <el-card class="filter-card">
      <div class="filter-row">
        <el-input
          v-model="searchQuery"
          placeholder="搜索规则标题/规则ID/MITRE"
          clearable
          class="search-input"
          @keyup.enter="loadRules"
        />
        <el-select v-model="status" placeholder="规则状态" clearable class="filter-item">
          <el-option label="待审核" value="pending" />
          <el-option label="实验性" value="experimental" />
          <el-option label="已激活" value="active" />
          <el-option label="已禁用" value="disabled" />
        </el-select>
        <el-button type="primary" @click="loadRules">查询</el-button>
        <el-button type="success" @click="showAIGenerateDialog">AI规则</el-button>
        <el-button 
          type="danger" 
          :disabled="selectedRules.length === 0"
          @click="confirmDeleteSelected"
        >
          删除选中 ({{ selectedRules.length }})
        </el-button>
      </div>
    </el-card>

    <el-card>
      <el-table 
        v-loading="ruleLoading" 
        :data="rules" 
        border 
        stripe
        @selection-change="handleSelectionChange"
      >
        <el-table-column type="selection" width="55" />
        <el-table-column prop="title" label="规则标题" min-width="280" />
        <el-table-column label="MITRE" width="120">
          <template #default="{ row }">
            <el-link type="primary" @click="goToPolicies(row.mitre_id)">{{ row.mitre_id }}</el-link>
          </template>
        </el-table-column>
        <el-table-column prop="severity" label="严重程度" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="severityTagType(row.severity)">{{ severityLabel(row.severity) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="version" label="版本" width="80" align="center" />
        <el-table-column prop="created_at" label="创建时间" width="160">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="260" fixed="right" align="center">
          <template #default="{ row }">
            <el-button size="small" @click="showDetail(row)">详情</el-button>
            <el-button size="small" type="success" :disabled="row.status === 'active'" @click="approveRule(row.rule_id)">
              启用
            </el-button>
            <el-button size="small" type="warning" :disabled="row.status === 'disabled'" @click="disableRule(row.rule_id)">
              禁用
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="ruleTotal"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="loadRules"
          @size-change="loadRules"
        />
      </div>
    </el-card>

    <el-dialog v-model="detailVisible" title="规则详情" width="780px">
      <el-descriptions v-if="selectedRule" :column="2" border>
        <el-descriptions-item label="规则ID">{{ selectedRule.rule_id }}</el-descriptions-item>
        <el-descriptions-item label="标题">{{ selectedRule.title || '-' }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ statusLabel(selectedRule.status) }}</el-descriptions-item>
        <el-descriptions-item label="MITRE">{{ selectedRule.mitre_id || '-' }}</el-descriptions-item>
        <el-descriptions-item label="严重程度">{{ severityLabel(selectedRule.severity) || '-' }}</el-descriptions-item>
        <el-descriptions-item label="版本">{{ selectedRule.version }}</el-descriptions-item>
        <el-descriptions-item label="生成方式">{{ selectedRule.generated_by === 'llm' ? 'LLM生成' : '人工导入' }}</el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ formatTime(selectedRule.created_at) }}</el-descriptions-item>
        <el-descriptions-item label="激活时间">{{ formatTime(selectedRule.activated_at || '') }}</el-descriptions-item>
        <el-descriptions-item label="描述" :span="2">{{ selectedRule.description || '-' }}</el-descriptions-item>
        <el-descriptions-item label="规则内容" :span="2">
          <pre class="content-block">{{ selectedRule.content || '-' }}</pre>
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>

    <el-dialog v-model="aiGenerateVisible" title="AI生成Sigma规则" width="700px" :close-on-click-modal="false">
      <el-form :model="aiGenerateForm" label-width="100px">
        <el-form-item label="检测事件" required>
          <el-input
            v-model="aiGenerateForm.event"
            type="textarea"
            :rows="3"
            placeholder="描述要检测的安全事件，例如：检测反向Shell连接行为"
          />
        </el-form-item>
        <el-form-item label="检测方式">
          <el-input
            v-model="aiGenerateForm.method"
            type="textarea"
            :rows="3"
            placeholder="描述检测方式，例如：监控进程命令行参数，检测bash -i、nc -e等反向Shell特征"
          />
        </el-form-item>
        <el-form-item label="MITRE技术">
          <el-input v-model="aiGenerateForm.mitre_id" placeholder="可选，例如：T1059.004" />
        </el-form-item>
        <el-form-item label="严重程度">
          <el-select v-model="aiGenerateForm.severity" placeholder="选择严重程度">
            <el-option label="低" value="low" />
            <el-option label="中" value="medium" />
            <el-option label="高" value="high" />
            <el-option label="严重" value="critical" />
          </el-select>
        </el-form-item>
      </el-form>

      <div v-if="aiGenerateResult" class="ai-result">
        <el-divider>生成结果</el-divider>
        <el-descriptions :column="2" border>
          <el-descriptions-item label="规则ID">{{ aiGenerateResult.rule_id }}</el-descriptions-item>
          <el-descriptions-item label="规则标题">{{ aiGenerateResult.title }}</el-descriptions-item>
          <el-descriptions-item label="MITRE">{{ aiGenerateResult.mitre_id || '-' }}</el-descriptions-item>
          <el-descriptions-item label="严重程度">{{ aiGenerateResult.severity }}</el-descriptions-item>
          <el-descriptions-item label="生成耗时">{{ aiGenerateResult.duration }}秒</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag type="warning">实验性</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="规则内容" :span="2">
            <pre class="content-block">{{ aiGenerateResult.content }}</pre>
          </el-descriptions-item>
        </el-descriptions>
      </div>

      <template #footer>
        <el-button @click="aiGenerateVisible = false">关闭</el-button>
        <el-button type="primary" :loading="aiGenerateLoading" @click="generateRule">
          {{ aiGenerateLoading ? '生成中...' : '开始生成' }}
        </el-button>
        <el-button
          v-if="aiGenerateResult"
          type="success"
          @click="enableGeneratedRule"
        >
          启用规则
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="deleteConfirmVisible" title="确认删除" width="500px">
      <div v-if="deleteCheckResult">
        <el-alert
          v-if="deleteCheckResult.has_alerts"
          title="警告：选中的规则关联了告警数据"
          type="warning"
          :closable="false"
          show-icon
          style="margin-bottom: 16px"
        >
          <template #default>
            <p>以下规则存在关联告警，删除规则将同时删除这些告警和对应的阻断规则：</p>
            <ul style="margin: 8px 0; padding-left: 20px;">
              <li v-for="rule in deleteCheckResult.rules_with_alerts" :key="rule.rule_id">
                {{ rule.title || rule.rule_id }} ({{ rule.alert_count }} 条告警)
              </li>
            </ul>
          </template>
        </el-alert>
        
        <p>确定要删除选中的 {{ selectedRules.length }} 条规则吗？</p>
        <p v-if="deleteCheckResult.has_alerts" style="color: #e6a23c;">
          将同时删除 {{ deleteCheckResult.total_alerts }} 条关联告警和对应的阻断规则。
        </p>
      </div>
      
      <template #footer>
        <el-button @click="deleteConfirmVisible = false">取消</el-button>
        <el-button type="danger" :loading="deleteLoading" @click="deleteSelectedRules">
          确认删除
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useDetectionStore } from '@/store/detection'
import type { SigmaRule } from '@/types'
import { SeverityLabels, RuleStatusLabels } from '@/types'
import * as api from '@/api/detection'
import AIConfigPanel from '@/components/detection/AIConfigPanel.vue'

const route = useRoute()
const router = useRouter()
const store = useDetectionStore()

const searchQuery = ref('')
const status = ref('')
const page = ref(1)
const pageSize = ref(10)
const detailVisible = ref(false)
const selectedRule = ref<SigmaRule | null>(null)

const selectedRules = ref<SigmaRule[]>([])
const deleteConfirmVisible = ref(false)
const showAIConfig = ref(true)
const deleteLoading = ref(false)
const deleteCheckResult = ref<{
  has_alerts: boolean
  rules_with_alerts: Array<{ rule_id: string; title: string; alert_count: number }>
  total_alerts: number
} | null>(null)

const aiGenerateVisible = ref(false)
const aiGenerateLoading = ref(false)
const aiGenerateForm = ref({
  event: '',
  method: '',
  mitre_id: '',
  severity: 'medium'
})
const aiGenerateResult = ref<{
  rule_id: string
  title: string
  mitre_id: string
  severity: string
  content: string
  duration: number
} | null>(null)

const rules = computed(() => store.rules)
const ruleTotal = computed(() => store.ruleTotal)
const ruleLoading = computed(() => store.ruleLoading)

function formatTime(time: string) {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
}

function statusTagType(ruleStatus: string) {
  if (ruleStatus === 'active') return 'success'
  if (ruleStatus === 'disabled') return 'info'
  if (ruleStatus === 'experimental') return 'warning'
  return 'danger'
}

function statusLabel(ruleStatus: string) {
  return RuleStatusLabels[ruleStatus] || ruleStatus
}

function severityTagType(level: string) {
  if (level === 'critical') return 'danger'
  if (level === 'high') return 'warning'
  if (level === 'medium') return 'info'
  return 'success'
}

function severityLabel(level: string) {
  return SeverityLabels[level] || level
}

function goToPolicies(mitreId: string) {
  if (!mitreId) return
  router.push({ path: '/detection/policies', query: { query: mitreId } })
}

async function loadRules() {
  await store.fetchRules({
    page: page.value,
    pageSize: pageSize.value,
    status: status.value || undefined,
    query: searchQuery.value || undefined
  })
}

function handleSelectionChange(selection: SigmaRule[]) {
  selectedRules.value = selection
}

function showDetail(rule: SigmaRule) {
  selectedRule.value = rule
  detailVisible.value = true
}

async function approveRule(ruleId: string) {
  await store.updateRuleStatus(ruleId, 'active')
  ElMessage.success('规则已启用')
}

async function disableRule(ruleId: string) {
  await store.updateRuleStatus(ruleId, 'disabled')
  ElMessage.success('规则已禁用')
}

async function confirmDeleteSelected() {
  if (selectedRules.value.length === 0) {
    ElMessage.warning('请选择要删除的规则')
    return
  }

  deleteLoading.value = true
  try {
    const ruleIds = selectedRules.value.map(r => r.rule_id)
    const result = await api.checkRulesBeforeDelete(ruleIds)
    deleteCheckResult.value = result
    deleteConfirmVisible.value = true
  } catch (error: any) {
    ElMessage.error(error.message || '检查规则失败')
  } finally {
    deleteLoading.value = false
  }
}

async function deleteSelectedRules() {
  deleteLoading.value = true
  try {
    const ruleIds = selectedRules.value.map(r => r.rule_id)
    const result = await api.deleteRules(ruleIds)
    ElMessage.success(`已删除 ${result.deleted_rules} 条规则，${result.deleted_alerts} 条告警，${result.deleted_policies} 条阻断规则`)
    deleteConfirmVisible.value = false
    selectedRules.value = []
    loadRules()
  } catch (error: any) {
    ElMessage.error(error.message || '删除失败')
  } finally {
    deleteLoading.value = false
  }
}

function showAIGenerateDialog() {
  aiGenerateForm.value = {
    event: '',
    method: '',
    mitre_id: '',
    severity: 'medium'
  }
  aiGenerateResult.value = null
  aiGenerateVisible.value = true
}

async function generateRule() {
  if (!aiGenerateForm.value.event) {
    ElMessage.warning('请输入检测事件描述')
    return
  }

  aiGenerateLoading.value = true
  try {
    const result = await api.generateSigmaRule({
      event: aiGenerateForm.value.event,
      method: aiGenerateForm.value.method,
      mitre_id: aiGenerateForm.value.mitre_id,
      severity: aiGenerateForm.value.severity
    })
    aiGenerateResult.value = result
    ElMessage.success('规则生成成功')
    loadRules()
  } catch (error: any) {
    ElMessage.error(error.message || '规则生成失败')
  } finally {
    aiGenerateLoading.value = false
  }
}

async function enableGeneratedRule() {
  if (!aiGenerateResult.value) return

  try {
    await store.updateRuleStatus(aiGenerateResult.value.rule_id, 'active')
    ElMessage.success('规则已启用并下发到Agent')
    aiGenerateVisible.value = false
    loadRules()
  } catch (error: any) {
    ElMessage.error(error.message || '启用规则失败')
  }
}

onMounted(() => {
  const queryParam = route.query.query as string
  if (queryParam) {
    searchQuery.value = queryParam
  }
  loadRules()
})
</script>

<style scoped>
.detection-rules-page {
  padding: 20px;
}

.filter-card {
  margin-bottom: 16px;
}

.filter-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.filter-item {
  width: 160px;
}

.search-input {
  width: 280px;
}

.pagination {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}

.content-block {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 300px;
  overflow-y: auto;
  background: #f5f7fa;
  padding: 10px;
  border-radius: 4px;
  font-size: 12px;
}

.ai-result {
  margin-top: 16px;
}
</style>