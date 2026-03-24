<template>
  <div class="detection-rules-page">
    <el-card class="filter-card">
      <div class="filter-row">
        <el-select v-model="status" placeholder="规则状态" clearable class="filter-item">
          <el-option label="待审核" value="pending" />
          <el-option label="实验性" value="experimental" />
          <el-option label="已激活" value="active" />
          <el-option label="已禁用" value="disabled" />
        </el-select>
        <el-button type="primary" @click="loadRules">查询</el-button>
      </div>
    </el-card>

    <el-card>
      <el-table v-loading="ruleLoading" :data="rules" border stripe>
        <el-table-column prop="rule_id" label="规则ID" min-width="180" />
        <el-table-column prop="title" label="规则标题" min-width="220" />
        <el-table-column prop="mitre_id" label="MITRE" width="120" />
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useDetectionStore } from '@/store/detection'
import type { SigmaRule } from '@/types'
import { SeverityLabels, RuleStatusLabels } from '@/types'

const store = useDetectionStore()

const status = ref('')
const page = ref(1)
const pageSize = ref(10)
const detailVisible = ref(false)
const selectedRule = ref<SigmaRule | null>(null)

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

async function loadRules() {
  await store.fetchRules({
    page: page.value,
    pageSize: pageSize.value,
    status: status.value || undefined
  })
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

onMounted(() => {
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

.pagination {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}

.content-block {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
