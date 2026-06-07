<template>
  <div class="investigation-panel">
    <!-- 置信度评分 -->
    <div class="verdict-section">
      <div class="verdict-header">
        <span class="verdict-label">研判结论</span>
        <el-tag :type="getVerdictTag(data.verdict)" size="large">
          {{ getVerdictLabel(data.verdict) }}
        </el-tag>
      </div>
      <div class="score-bar">
        <div class="score-label">风险评分</div>
        <el-progress
          :percentage="data.score"
          :color="getScoreColor(data.score)"
          :stroke-width="20"
          :format="(p: number) => `${p}分`"
        />
      </div>
      <div class="confidence">
        <span class="confidence-label">置信度:</span>
        <span class="confidence-value">{{ (data.confidence * 100).toFixed(0) }}%</span>
      </div>
    </div>

    <!-- 入口推断 -->
    <div v-if="data.entry_point_candidates?.length" class="section">
      <div class="section-title">入口推断</div>
      <div class="entry-list">
        <div
          v-for="ep in data.entry_point_candidates"
          :key="ep.candidate_id"
          class="entry-item"
        >
          <div class="entry-header">
            <el-tag :type="getEntryTypeTag(ep.entry_type)" size="small">
              {{ getEntryTypeLabel(ep.entry_type) }}
            </el-tag>
            <span class="entry-score">评分: {{ ep.score }}</span>
          </div>
          <div class="entry-title">{{ ep.title }}</div>
          <div class="entry-explanation">{{ ep.explanation }}</div>
        </div>
      </div>
    </div>

    <!-- 攻击时间线 -->
    <div v-if="data.attack_timeline?.length" class="section">
      <div class="section-title">攻击时间线</div>
      <el-timeline>
        <el-timeline-item
          v-for="event in data.attack_timeline"
          :key="event.event_id"
          :timestamp="formatTime(event.time)"
          :type="getPhaseType(event.phase)"
          placement="top"
        >
          <div class="timeline-content">
            <div class="timeline-title">{{ event.title }}</div>
            <div class="timeline-summary">{{ event.summary }}</div>
            <el-tag size="small">{{ getPhaseLabel(event.phase) }}</el-tag>
          </div>
        </el-timeline-item>
      </el-timeline>
    </div>

    <!-- 证据矩阵 -->
    <div v-if="data.evidence_count > 0" class="section">
      <div class="section-title">
        证据矩阵
        <el-tag size="small">{{ data.evidence_count }} 条证据</el-tag>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { HostAttackInvestigationCardPayload } from '@/api/assistant'

defineProps<{
  data: HostAttackInvestigationCardPayload
}>()

function getVerdictTag(verdict: string): string {
  const map: Record<string, string> = {
    confirmed_compromised: 'danger',
    suspicious: 'warning',
    likely_benign: 'success',
    insufficient_evidence: 'info',
  }
  return map[verdict] || 'info'
}

function getVerdictLabel(verdict: string): string {
  const map: Record<string, string> = {
    confirmed_compromised: '确认被攻击',
    suspicious: '可疑',
    likely_benign: '可能正常',
    insufficient_evidence: '证据不足',
  }
  return map[verdict] || verdict
}

function getScoreColor(score: number): string {
  if (score >= 80) return '#f56c6c'
  if (score >= 50) return '#e6a23c'
  if (score >= 20) return '#409eff'
  return '#67c23a'
}

function getEntryTypeTag(type: string): string {
  const map: Record<string, string> = {
    ssh_bruteforce: 'danger',
    exposed_service_cve: 'danger',
    webshell: 'danger',
    stolen_credential: 'warning',
    weak_config: 'warning',
    unknown: 'info',
  }
  return map[type] || 'info'
}

function getEntryTypeLabel(type: string): string {
  const map: Record<string, string> = {
    ssh_bruteforce: 'SSH暴力破解',
    exposed_service_cve: '服务漏洞',
    webshell: 'WebShell',
    stolen_credential: '凭据窃取',
    weak_config: '弱配置',
    unknown: '未知',
  }
  return map[type] || type
}

function getPhaseType(phase: string): string {
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

function getPhaseLabel(phase: string): string {
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
.investigation-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.verdict-section {
  background: #f5f7fa;
  padding: 16px;
  border-radius: 8px;
}

.verdict-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.verdict-label {
  font-size: 16px;
  font-weight: 600;
}

.score-bar {
  margin-bottom: 8px;
}

.score-label {
  font-size: 13px;
  color: #606266;
  margin-bottom: 4px;
}

.confidence {
  font-size: 13px;
  color: #606266;
}

.confidence-label {
  margin-right: 4px;
}

.confidence-value {
  font-weight: 600;
  color: #303133;
}

.section {
  padding: 12px;
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.entry-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.entry-item {
  padding: 10px;
  background: #f5f7fa;
  border-radius: 6px;
}

.entry-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 4px;
}

.entry-score {
  font-size: 12px;
  color: #909399;
}

.entry-title {
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 4px;
}

.entry-explanation {
  font-size: 13px;
  color: #606266;
}

.timeline-content {
  padding: 4px 0;
}

.timeline-title {
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 4px;
}

.timeline-summary {
  font-size: 13px;
  color: #606266;
  margin-bottom: 4px;
}
</style>
