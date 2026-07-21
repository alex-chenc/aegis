<template>
  <div class="attack-timeline">
    <div class="section-header">
      <span class="section-title">{{ $t('generated.common_attack_timeline_3e8bf0') }}</span>
      <el-tag v-if="events.length" size="small">{{ events.length }} {{ $t('generated.assistantAttackTimelineCard_events_29cfe3') }}</el-tag>
    </div>

    <div v-if="!events.length" class="empty-hint">
      {{ $t('generated.assistantAttackTimelineCard_no_timeline_events_yet_1fac91') }}
    </div>

    <!-- 按阶段分组 -->
    <div v-for="(group, phase) in groupedEvents" :key="phase" class="phase-group">
      <div class="phase-header">
        <el-tag :type="phaseTag(phase)" size="small" effect="plain">
          {{ phaseLabel(phase) }}
        </el-tag>
        <span class="phase-count">{{ group.length }} {{ $t('generated.assistantAttackTimelineCard_events_29cfe3') }}</span>
      </div>
      <el-timeline>
        <el-timeline-item
          v-for="event in group"
          :key="event.event_id"
          :timestamp="formatTime(event.time)"
          :type="phaseTag(phase)"
          placement="top"
        >
          <div class="timeline-event">
            <div class="event-title">{{ event.title }}</div>
            <div class="event-summary">{{ event.summary }}</div>
            <div class="event-meta">
              <span class="event-confidence">
                {{ $t('generated.common_confidence_b78c2d') }} {{ (event.confidence * 100).toFixed(0) }}%
              </span>
              <span v-if="event.evidence_ids?.length" class="event-evidence">
                {{ event.evidence_ids.length }} {{ $t('generated.common_pieces_of_evidence_bf9c74') }}
              </span>
            </div>
          </div>
        </el-timeline-item>
      </el-timeline>
    </div>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

import { computed } from 'vue'
import type { AttackTimelineEvent } from '@/api/assistant'

const props = defineProps<{
  events: AttackTimelineEvent[]
}>()

const phaseOrder = [
  'reconnaissance',
  'initial_access',
  'execution',
  'persistence',
  'privilege_escalation',
  'defense_evasion',
  'lateral_movement',
  'impact',
]

const groupedEvents = computed(() => {
  const groups: Record<string, AttackTimelineEvent[]> = {}
  const sorted = [...props.events].sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime())
  for (const event of sorted) {
    if (!groups[event.phase]) {
      groups[event.phase] = []
    }
    groups[event.phase].push(event)
  }
  // 按 MITRE 阶段顺序排列
  const ordered: Record<string, AttackTimelineEvent[]> = {}
  for (const phase of phaseOrder) {
    if (groups[phase]) {
      ordered[phase] = groups[phase]
    }
  }
  // 添加未在预定义列表中的阶段
  for (const phase of Object.keys(groups)) {
    if (!ordered[phase]) {
      ordered[phase] = groups[phase]
    }
  }
  return ordered
})

function phaseTag(phase: string): string {
  const map: Record<string, string> = {
    reconnaissance: 'info',
    initial_access: 'danger',
    execution: 'danger',
    persistence: 'warning',
    privilege_escalation: 'danger',
    defense_evasion: 'warning',
    lateral_movement: 'danger',
    impact: 'danger',
  }
  return map[phase] || 'info'
}

function phaseLabel(phase: string): string {
  const map: Record<string, string> = {
    reconnaissance: translate('generatedScript.assistantAttackTimelineCard_reconnaissance_5a0497'),
    initial_access: translate('generatedScript.assistantAttackTimelineCard_initial_access_798005'),
    execution: translate('generatedScript.common_implement_28febb'),
    persistence: translate('generatedScript.common_persistence_63cca6'),
    privilege_escalation: translate('generatedScript.common_privilege_escalation_b6f22d'),
    defense_evasion: translate('generatedScript.assistantAttackTimelineCard_defense_evasion_f93ea1'),
    lateral_movement: translate('generatedScript.assistantAttackTimelineCard_lateral_movement_9e6f0a'),
    impact: translate('generatedScript.assistantAttackTimelineCard_influence_5be321'),
  }
  return map[phase] || phase
}

function formatTime(time: string): string {
  if (!time) return ''
  return formatDateTime(time)
}
</script>

<style scoped>
.attack-timeline {
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

.phase-group {
  margin-bottom: 16px;
}

.phase-group:last-child {
  margin-bottom: 0;
}

.phase-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid #ebeef5;
}

.phase-count {
  font-size: 12px;
  color: #909399;
}

.timeline-event {
  padding: 2px 0;
}

.event-title {
  font-size: 14px;
  font-weight: 500;
  color: #303133;
  margin-bottom: 4px;
}

.event-summary {
  font-size: 13px;
  color: #606266;
  line-height: 1.5;
  margin-bottom: 4px;
}

.event-meta {
  display: flex;
  gap: 12px;
  font-size: 12px;
  color: #909399;
}
</style>
