<template>
  <el-card v-if="stats" style="width: 100%">
    <template #header>
      <span>审计统计</span>
    </template>
    <el-row :gutter="20">
      <el-col :span="6">
        <el-statistic title="总审计次数" :value="stats.total" />
      </el-col>
      <el-col :span="6">
        <el-statistic title="通过" :value="stats.passed" />
      </el-col>
      <el-col :span="6">
        <el-statistic title="失败" :value="stats.failed" />
      </el-col>
      <el-col :span="6">
        <div style="text-align: center">
          <div style="font-size: 14px; color: #909399; margin-bottom: 8px">通过率</div>
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
        <el-statistic title="1次通过" :value="stats.retry_distribution['1'] || 0" />
      </el-col>
      <el-col :span="6">
        <el-statistic title="2次通过" :value="stats.retry_distribution['2'] || 0" />
      </el-col>
      <el-col :span="6">
        <el-statistic title="3次通过" :value="stats.retry_distribution['3'] || 0" />
      </el-col>
      <el-col :span="6">
        <el-statistic title="最终失败" :value="stats.retry_distribution['failed'] || 0" />
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
