<template>
  <el-card v-if="stats" style="width: 100%">
    <template #header>
      <span>{{ $t('generated.settingsAuditLogsAuditStatsCard_audit_statistics_858c38') }}</span>
    </template>
    <el-row :gutter="20">
      <el-col :span="6">
        <el-statistic :title="$t('generated.settingsAuditLogsAuditStatsCard_total_number_of_audits_b61f8c')" :value="stats.total" />
      </el-col>
      <el-col :span="6">
        <el-statistic :title="$t('generated.common_pass_dcc423')" :value="stats.passed" />
      </el-col>
      <el-col :span="6">
        <el-statistic :title="$t('generated.common_fail_3e3c80')" :value="stats.failed" />
      </el-col>
      <el-col :span="6">
        <div style="text-align: center">
          <div style="font-size: 14px; color: #909399; margin-bottom: 8px">{{ $t('generated.common_pass_rate_b4582b') }}</div>
          <el-progress
            type="circle"
            :percentage="stats.pass_rate"
            :color="stats.pass_rate >= 90 ? '#67c23a' : '#e6a23c'"
            :width="70"
          />
        </div>
      </el-col>
    </el-row>
    <el-row :gutter="20" style="margin-top: 16px" v-if="stats.retry_distribution">
      <el-col :span="6">
        <el-statistic :title="$t('generated.settingsAuditLogsAuditStatsCard_1_pass_8392e2')" :value="stats.retry_distribution['1'] || 0" />
      </el-col>
      <el-col :span="6">
        <el-statistic :title="$t('generated.settingsAuditLogsAuditStatsCard_2_passes_2de445')" :value="stats.retry_distribution['2'] || 0" />
      </el-col>
      <el-col :span="6">
        <el-statistic :title="$t('generated.settingsAuditLogsAuditStatsCard_3_passes_efeb8b')" :value="stats.retry_distribution['3'] || 0" />
      </el-col>
      <el-col :span="6">
        <el-statistic :title="$t('generated.settingsAuditLogsAuditStatsCard_ultimately_failed_a3635d')" :value="stats.retry_distribution['failed'] || 0" />
      </el-col>
    </el-row>
  </el-card>
</template>

<script setup lang="ts">
import type { AuditStats } from '@/api/audit-logs'

defineProps<{
  stats: AuditStats | null
}>()
</script>
