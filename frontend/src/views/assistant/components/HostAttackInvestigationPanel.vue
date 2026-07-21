<template>
  <div class="investigation-panel">
    <!-- 研判结论 -->
    <CompromiseScoreCard
      :verdict="data.verdict"
      :score="data.score"
      :confidence="data.confidence"
      :summary="data.summary"
      :key-reasons="data.key_reasons"
      :contradictions="data.contradictions"
    />

    <!-- 入口推断 + 数据源覆盖 并排 -->
    <div class="two-column">
      <EntryPointCandidateList :candidates="data.entry_point_candidates || []" />
      <SourceCoveragePanel :coverage="data.source_coverage" />
    </div>

    <!-- 攻击时间线 -->
    <AttackTimelineCard :events="data.attack_timeline || []" />

    <!-- 攻击路径图 -->
    <AttackPathGraph
      :nodes="data.attack_path?.nodes || []"
      :edges="data.attack_path?.edges || []"
    />

    <!-- 证据矩阵 -->
    <EvidenceMatrixTable
      :items="evidenceItems"
      :evidence-count="data.evidence_count || 0"
      :by-source="evidenceBySource"
    />

    <!-- 缺失证据 -->
    <div v-if="data.missing_evidence?.length" class="missing-evidence-section">
      <div class="section-header">
        <span class="section-title">{{ $t('generated.assistantHostAttackInvestigationPanel_missing_evidence_542df7') }}</span>
        <el-tag size="small" type="warning">{{ data.missing_evidence.length }} {{ $t('generated.common_item_64728a') }}</el-tag>
      </div>
      <div class="missing-list">
        <div
          v-for="(item, idx) in data.missing_evidence"
          :key="idx"
          class="missing-item"
        >
          <div class="missing-source">{{ sourceLabel(item.source_type) }}</div>
          <div class="missing-reason">{{ item.reason }}</div>
          <div v-if="item.suggested_tool" class="missing-tool">
            {{ $t('generated.assistantHostAttackInvestigationPanel_suggested_tools_4de096') }} {{ item.suggested_tool }}
          </div>
        </div>
      </div>
    </div>

    <!-- 推荐动作 -->
    <RecommendedActionList :actions="data.recommended_actions" />
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed } from 'vue'
import type { HostAttackInvestigationCardPayload } from '@/api/assistant'
import CompromiseScoreCard from './CompromiseScoreCard.vue'
import EntryPointCandidateList from './EntryPointCandidateList.vue'
import AttackTimelineCard from './AttackTimelineCard.vue'
import AttackPathGraph from './AttackPathGraph.vue'
import EvidenceMatrixTable from './EvidenceMatrixTable.vue'
import SourceCoveragePanel from './SourceCoveragePanel.vue'
import RecommendedActionList from './RecommendedActionList.vue'

const props = defineProps<{
  data: HostAttackInvestigationCardPayload
}>()

// 从 payload 中提取证据项（如果后端直接返回了 evidence_matrix.items）
const evidenceItems = computed(() => {
  const payload = props.data as Record<string, any>
  return payload.evidence_matrix?.items || []
})

const evidenceBySource = computed(() => {
  const payload = props.data as Record<string, any>
  return payload.evidence_matrix?.by_source || {}
})

function sourceLabel(type: string): string {
  const map: Record<string, string> = {
    aegis_alert: translate('generatedScript.common_aegis_alert_cc5691'),
    runtime_event: translate('generatedScript.common_runtime_events_3afbe9'),
    agent_process: translate('generatedScript.common_agent_process_449f5d'),
    agent_network: translate('generatedScript.common_agent_network_26dcfa'),
    agent_file: translate('generatedScript.common_agent_file_fa1097'),
    agent_log: translate('generatedScript.common_agent_log_b9eb43'),
    baseline: translate('generatedScript.common_baseline_check_3c75ff'),
    vulnerability: translate('generatedScript.common_vulnerability_data_792410'),
    external_siem: translate('generatedScript.common_external_siem_b9a80d'),
    external_cmdb: translate('generatedScript.common_external_cmdb_a2b97e'),
    external_edr: translate('generatedScript.common_external_edr_593ae6'),
  }
  return map[type] || type
}
</script>

<style scoped>
.investigation-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.two-column {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

@media (max-width: 900px) {
  .two-column {
    grid-template-columns: 1fr;
  }
}

.missing-evidence-section {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 16px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.missing-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.missing-item {
  padding: 10px;
  background: #fdf6ec;
  border: 1px solid #faecd8;
  border-radius: 6px;
}

.missing-source {
  font-size: 13px;
  font-weight: 500;
  color: #303133;
  margin-bottom: 4px;
}

.missing-reason {
  font-size: 13px;
  color: #606266;
}

.missing-tool {
  font-size: 12px;
  color: #409eff;
  margin-top: 4px;
}
</style>
