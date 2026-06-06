<template>
  <div class="entry-candidates">
    <div class="section-header">
      <span class="section-title">入口推断</span>
      <el-tag v-if="candidates.length" size="small">{{ candidates.length }} 个候选</el-tag>
    </div>

    <div v-if="!candidates.length" class="empty-hint">
      暂无入口候选
    </div>

    <div class="candidate-list">
      <div
        v-for="(candidate, idx) in sortedCandidates"
        :key="candidate.candidate_id"
        class="candidate-item"
        :class="{ 'top-candidate': idx === 0 }"
      >
        <div class="candidate-header">
          <div class="candidate-rank">#{{ idx + 1 }}</div>
          <el-tag :type="entryTypeTag(candidate.entry_type)" size="small">
            {{ entryTypeLabel(candidate.entry_type) }}
          </el-tag>
          <div class="candidate-score">
            <span class="score-value">{{ candidate.score }}</span>
            <span class="score-unit">分</span>
          </div>
          <div class="candidate-confidence">
            置信度 {{ (candidate.confidence * 100).toFixed(0) }}%
          </div>
        </div>

        <div class="candidate-title">{{ candidate.title }}</div>
        <div class="candidate-explanation">{{ candidate.explanation }}</div>

        <div class="evidence-row">
          <div class="evidence-tag supporting">
            <el-icon><CircleCheck /></el-icon>
            <span>{{ candidate.evidence_ids?.length || 0 }} 条支持证据</span>
          </div>
          <div v-if="candidate.counter_evidence_ids?.length" class="evidence-tag counter">
            <el-icon><CircleClose /></el-icon>
            <span>{{ candidate.counter_evidence_ids.length }} 条反证</span>
          </div>
        </div>

        <div v-if="candidate.first_seen_at" class="first-seen">
          首次发现: {{ formatTime(candidate.first_seen_at) }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CircleCheck, CircleClose } from '@element-plus/icons-vue'
import type { EntryPointCandidate } from '@/api/assistant'

const props = defineProps<{
  candidates: EntryPointCandidate[]
}>()

const sortedCandidates = computed(() =>
  [...props.candidates].sort((a, b) => b.score - a.score)
)

function entryTypeTag(type: string): string {
  const map: Record<string, string> = {
    ssh_bruteforce: 'danger',
    exposed_service_cve: 'danger',
    webshell: 'danger',
    stolen_credential: 'warning',
    scheduled_task: 'warning',
    package_supply_chain: 'warning',
    weak_config: 'warning',
    unknown: 'info',
  }
  return map[type] || 'info'
}

function entryTypeLabel(type: string): string {
  const map: Record<string, string> = {
    ssh_bruteforce: 'SSH 暴力破解',
    exposed_service_cve: '服务漏洞',
    webshell: 'WebShell',
    stolen_credential: '凭据窃取',
    scheduled_task: '计划任务',
    package_supply_chain: '供应链',
    weak_config: '弱配置',
    unknown: '未知',
  }
  return map[type] || type
}

function formatTime(time: string): string {
  if (!time) return ''
  return new Date(time).toLocaleString('zh-CN')
}
</script>

<style scoped>
.entry-candidates {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 16px;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.empty-hint {
  text-align: center;
  color: #909399;
  font-size: 13px;
  padding: 16px;
}

.candidate-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.candidate-item {
  padding: 12px;
  background: #f5f7fa;
  border-radius: 8px;
  border: 1px solid #e4e7ed;
}

.candidate-item.top-candidate {
  border-color: #e6a23c;
  background: #fdf6ec;
}

.candidate-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.candidate-rank {
  font-weight: 700;
  color: #909399;
  font-size: 14px;
  min-width: 24px;
}

.candidate-score {
  margin-left: auto;
  display: flex;
  align-items: baseline;
  gap: 2px;
}

.score-value {
  font-size: 20px;
  font-weight: 700;
  color: #303133;
}

.score-unit {
  font-size: 12px;
  color: #909399;
}

.candidate-confidence {
  font-size: 12px;
  color: #909399;
}

.candidate-title {
  font-size: 14px;
  font-weight: 500;
  color: #303133;
  margin-bottom: 4px;
}

.candidate-explanation {
  font-size: 13px;
  color: #606266;
  line-height: 1.5;
  margin-bottom: 8px;
}

.evidence-row {
  display: flex;
  gap: 12px;
  margin-bottom: 4px;
}

.evidence-tag {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
}

.evidence-tag.supporting {
  color: #67c23a;
}

.evidence-tag.counter {
  color: #e6a23c;
}

.first-seen {
  font-size: 12px;
  color: #909399;
}
</style>
