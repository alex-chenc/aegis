<template>
  <div class="entry-candidates">
    <div class="section-header">
      <span class="section-title">{{ $t('generated.assistantEntryPointCandidateList_entrance_inference_9bbfec') }}</span>
      <el-tag v-if="candidates.length" size="small">{{ candidates.length }} {{ $t('generated.assistantEntryPointCandidateList_candidates_f4555b') }}</el-tag>
    </div>

    <div v-if="!candidates.length" class="empty-hint">
      {{ $t('generated.assistantEntryPointCandidateList_no_entrance_candidates_yet_70fa2f') }}
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
            <span class="score-unit">{{ $t('generated.assistantEntryPointCandidateList_point_0d7416') }}</span>
          </div>
          <div class="candidate-confidence">
            {{ $t('generated.common_confidence_b78c2d') }} {{ (candidate.confidence * 100).toFixed(0) }}%
          </div>
        </div>

        <div class="candidate-title">{{ candidate.title }}</div>
        <div class="candidate-explanation">{{ candidate.explanation }}</div>

        <div class="evidence-row">
          <div class="evidence-tag supporting">
            <el-icon><CircleCheck /></el-icon>
            <span>{{ candidate.evidence_ids?.length || 0 }} {{ $t('generated.assistantEntryPointCandidateList_piece_of_supporting_evidence_1157df') }}</span>
          </div>
          <div v-if="candidate.counter_evidence_ids?.length" class="evidence-tag counter">
            <el-icon><CircleClose /></el-icon>
            <span>{{ candidate.counter_evidence_ids.length }} {{ $t('generated.assistantEntryPointCandidateList_evidence_to_the_contrary_f73075') }}</span>
          </div>
        </div>

        <div v-if="candidate.first_seen_at" class="first-seen">
          {{ $t('generated.assistantEntryPointCandidateList_first_discovered_f9508b') }} {{ formatTime(candidate.first_seen_at) }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

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
    ssh_bruteforce: translate('generatedScript.assistantEntryPointCandidateList_ssh_brute_force_cracking_f76e47'),
    exposed_service_cve: translate('generatedScript.assistantEntryPointCandidateList_service_vulnerability_fd3544'),
    webshell: 'WebShell',
    stolen_credential: translate('generatedScript.assistantEntryPointCandidateList_credential_theft_31a10d'),
    scheduled_task: translate('generatedScript.assistantEntryPointCandidateList_scheduled_tasks_d065d4'),
    package_supply_chain: translate('generatedScript.assistantEntryPointCandidateList_supply_chain_0978a2'),
    weak_config: translate('generatedScript.assistantEntryPointCandidateList_weak_configuration_19c953'),
    unknown: translate('generatedScript.common_unknown_d9c32a'),
  }
  return map[type] || type
}

function formatTime(time: string): string {
  if (!time) return ''
  return formatDateTime(time)
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
