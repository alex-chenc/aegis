<template>
  <el-dialog
    v-model="dialogVisible"
    :title="dialogTitle"
    width="800px"
    :close-on-click-modal="false"
    @closed="handleClose"
  >
    <div class="dialog-content">
      <div v-if="cve" class="cve-info">
        <el-descriptions :column="2" border size="small">
          <el-descriptions-item :label="$t('generated.common_cve_number_1488a8')">
            <el-link :href="`https://nvd.nist.gov/vuln/detail/${cve.cve_id}`" target="_blank" type="primary">
              {{ cve.cve_id }}
            </el-link>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_severity_d918e4')">
            <SeverityTag :severity="cve.severity" />
          </el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_cvss_score_f6c632')" :span="2">
            {{ cve.cvss_score ?? 'N/A' }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('generated.common_vulnerability_description_3eb740')" :span="2">
            {{ cve.description }}
          </el-descriptions-item>
        </el-descriptions>
      </div>

      <div v-if="error" class="error-wrap">
        <el-alert :title="error" type="error" show-icon />
      </div>

      <HostScriptStatusList
        v-if="cve"
        :cve-id="cve.cve_id"
        :script-type="mode"
        :cve-source="cve.source"
        :affected-hosts-count="cve.affected_hosts_count"
        @execute="handleHostScriptExecute"
      />
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import SeverityTag from './SeverityTag.vue'
import HostScriptStatusList from '@/components/vulnerability/HostScriptStatusList.vue'

interface Vulnerability {
  id: string
  cve_id: string
  severity: 'Critical' | 'High' | 'Medium' | 'Low'
  cvss_score: number | null
  description: string
  affected_hosts_count: number
  source?: 'llm_analysis' | 'custom_query' | 'nvd_import' | string
}

const props = defineProps<{
  visible: boolean
  mode: 'fix' | 'poc'
  cve: Vulnerability | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'execute', data: { taskId: string; hosts: string[] }): void
}>()

const router = useRouter()

const dialogVisible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})

const dialogTitle = computed(() => (props.mode === 'fix' ? translate('generatedScript.commonFixConfirmationDialog_one_click_repair_438876') : translate('generatedScript.common_poc_verification_2e1c70')))
const error = ref('')

function handleClose() {
  error.value = ''
}

function handleHostScriptExecute(data: { taskGroupId: string; hosts: string[] }) {
  emit('execute', { taskId: data.taskGroupId || '', hosts: data.hosts || [] })
  ElMessage.success(props.mode === 'fix' ? translate('generatedScript.commonFixConfirmationDialog_repair_task_created_411e08') : translate('generatedScript.commonFixConfirmationDialog_poc_verification_task_created_9d1066'))
  dialogVisible.value = false
  router.push('/vulnerability/tasks')
}
</script>

<style scoped>
.dialog-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.error-wrap {
  margin-bottom: 8px;
}
</style>
