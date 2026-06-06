<template>
  <div class="attack-timeline">
    <div class="section-header">
      <span class="section-title">攻击时间线</span>
      <el-tag v-if="events.length" size="small">{{ events.length }} 个事件</el-tag>
    </div>

    <div v-if="!events.length" class="empty-hint">
      暂无时间线事件
    </div>

    <!-- 按阶段分组 -->
    <div v-for="(group, phase) in groupedEvents" :key="phase" class="phase-group">
      <div class="phase-header">
        <el-tag :type="phaseTag(phase)" size="small" effect="plain">
          {{ phaseLabel(phase) }}
        </el-tag>
        <span class="phase-count">{{ group.length }} 个事件</span>
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
                置信度 {{ (event.confidence * 100).toFixed(0) }}%
              </span>
              <span v-if="event.evidence_ids?.length" class="event-evidence">
                {{ event.evidence_ids.length }} 条证据
              </span>
            </div>
          </div>
        </el-timeline-item>
      </el-timeline>
    </div>
  </div>
</template>

<script setup lang="ts">
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
    reconnaissance: '侦察',
    initial_access: '初始访问',
    execution: '执行',
    persistence: '持久化',
    privilege_escalation: '权限提升',
    defense_evasion: '防御规避',
    lateral_movement: '横向移动',
    impact: '影响',
  }
  return map[phase] || phase
}

function formatTime(time: string): string {
  if (!time) return ''
  return new Date(time).toLocaleString('zh-CN')
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
