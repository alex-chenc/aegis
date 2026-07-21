<template>
  <div class="compromise-card" :class="verdict">
    <div class="card-header">
      <span class="card-title">{{ $t('generated.assistantCompromiseScoreCard_research_and_judgment_conclusion_6e1b3d') }}</span>
      <el-tag :type="verdictTagType" size="large" effect="dark">
        {{ verdictLabel }}
      </el-tag>
    </div>

    <div class="score-section">
      <div class="score-bar">
        <div class="score-label">{{ $t('generated.assistantCompromiseScoreCard_risk_score_6580b3') }}</div>
        <el-progress
          :percentage="score"
          :color="scoreColor"
          :stroke-width="20"
          :format="(p: number) => $t('dynamic.score', { value: p })"
        />
      </div>
      <div class="confidence">
        <span class="confidence-label">{{ $t('generated.assistantCompromiseScoreCard_confidence_55141d') }}</span>
        <span class="confidence-value">{{ (confidence * 100).toFixed(0) }}%</span>
      </div>
    </div>

    <div v-if="summary" class="summary-section">
      <div class="summary-text">{{ summary }}</div>
    </div>

    <div v-if="keyReasons?.length" class="reasons-section">
      <div class="reasons-title">{{ $t('generated.assistantCompromiseScoreCard_key_reasons_750ff4') }}</div>
      <ul class="reasons-list">
        <li v-for="(reason, idx) in keyReasons" :key="idx">{{ reason }}</li>
      </ul>
    </div>

    <div v-if="contradictions?.length" class="contradictions-section">
      <div class="contradictions-title">{{ $t('generated.assistantCompromiseScoreCard_contradiction_7e8029') }}</div>
      <ul class="contradictions-list">
        <li v-for="(item, idx) in contradictions" :key="idx">{{ item }}</li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed } from 'vue'

const props = defineProps<{
  verdict: 'confirmed_compromised' | 'suspicious' | 'likely_benign' | 'insufficient_evidence'
  score: number
  confidence: number
  summary?: string
  keyReasons?: string[]
  contradictions?: string[]
}>()

const verdictLabel = computed(() => {
  const map: Record<string, string> = {
    confirmed_compromised: translate('generatedScript.assistantCompromiseScoreCard_confirmed_to_be_under_attack_d6ec3f'),
    suspicious: translate('generatedScript.assistantCompromiseScoreCard_suspicious_e931ad'),
    likely_benign: translate('generatedScript.assistantCompromiseScoreCard_probably_normal_3d7485'),
    insufficient_evidence: translate('generatedScript.assistantCompromiseScoreCard_insufficient_evidence_70568c'),
  }
  return map[props.verdict] || props.verdict
})

const verdictTagType = computed(() => {
  const map: Record<string, string> = {
    confirmed_compromised: 'danger',
    suspicious: 'warning',
    likely_benign: 'success',
    insufficient_evidence: 'info',
  }
  return map[props.verdict] || 'info'
})

const scoreColor = computed(() => {
  if (props.score >= 80) return '#f56c6c'
  if (props.score >= 50) return '#e6a23c'
  if (props.score >= 20) return '#409eff'
  return '#67c23a'
})
</script>

<style scoped>
.compromise-card {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 16px;
}

.compromise-card.confirmed_compromised {
  border-left: 4px solid #f56c6c;
  background: #fef0f0;
}

.compromise-card.suspicious {
  border-left: 4px solid #e6a23c;
  background: #fdf6ec;
}

.compromise-card.likely_benign {
  border-left: 4px solid #67c23a;
  background: #f0f9eb;
}

.compromise-card.insufficient_evidence {
  border-left: 4px solid #909399;
  background: #f4f4f5;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.score-section {
  display: flex;
  align-items: center;
  gap: 24px;
  margin-bottom: 12px;
}

.score-bar {
  flex: 1;
}

.score-label {
  font-size: 13px;
  color: #606266;
  margin-bottom: 4px;
}

.confidence {
  font-size: 13px;
  color: #606266;
  white-space: nowrap;
}

.confidence-label {
  margin-right: 4px;
}

.confidence-value {
  font-weight: 600;
  color: #303133;
  font-size: 16px;
}

.summary-section {
  margin-bottom: 12px;
  padding: 10px;
  background: rgba(0, 0, 0, 0.02);
  border-radius: 6px;
}

.summary-text {
  font-size: 14px;
  color: #303133;
  line-height: 1.6;
}

.reasons-section,
.contradictions-section {
  margin-bottom: 8px;
}

.reasons-title,
.contradictions-title {
  font-size: 13px;
  font-weight: 600;
  color: #606266;
  margin-bottom: 6px;
}

.contradictions-title {
  color: #e6a23c;
}

.reasons-list,
.contradictions-list {
  margin: 0;
  padding-left: 20px;
  font-size: 13px;
  color: #606266;
  line-height: 1.8;
}
</style>
