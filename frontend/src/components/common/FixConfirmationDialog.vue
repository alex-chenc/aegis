<template>
  <el-dialog
    v-model="dialogVisible"
    :title="dialogTitle"
    width="700px"
    :close-on-click-modal="false"
    @closed="handleClose"
  >
    <div class="dialog-content">
      <div class="cve-info">
        <el-descriptions :column="2" border size="small">
          <el-descriptions-item label="CVE编号">
            <el-link :href="`https://nvd.nist.gov/vuln/detail/${cve.cve_id}`" target="_blank" type="primary">
              {{ cve.cve_id }}
            </el-link>
          </el-descriptions-item>
          <el-descriptions-item label="严重程度">
            <SeverityTag :severity="cve.severity" />
          </el-descriptions-item>
          <el-descriptions-item label="CVSS评分" :span="2">
            {{ cve.cvss_score || 'N/A' }}
          </el-descriptions-item>
          <el-descriptions-item label="漏洞描述" :span="2">
            {{ cve.description }}
          </el-descriptions-item>
        </el-descriptions>
      </div>

      <div class="host-selection">
        <h4>{{ mode === 'fix' ? '选择修复目标' : '选择验证目标' }}</h4>
        <el-select
          v-model="selectedHosts"
          :multiple="mode === 'fix'"
          filterable
          placeholder="请选择主机"
          style="width: 100%"
        >
          <el-option
            v-for="host in affectedHosts"
            :key="host.id"
            :label="`${host.ip_address} (${host.hostname})`"
            :value="host.id"
          />
        </el-select>
      </div>

      <div v-if="!script" class="script-actions">
        <el-button
          type="primary"
          :loading="generating"
          :disabled="selectedHosts.length === 0"
          @click="generateScript"
        >
          {{ mode === 'fix' ? '生成修复脚本' : '生成POC验证脚本' }}
        </el-button>
        <p class="hint-text">
          {{ mode === 'poc' ? 'POC验证用于确认漏洞是否存在，脚本经过安全设计，不会对系统造成破坏' : '修复脚本将尝试修复此漏洞，建议先进行POC验证确认漏洞存在' }}
        </p>
      </div>

      <div v-else class="script-preview-section">
        <ScriptPreview :script="script" :mode="mode" />
        
        <div class="action-buttons">
          <el-button @click="resetScript">重新生成</el-button>
          <el-button type="primary" :loading="executing" @click="executeScript">
            {{ mode === 'fix' ? '确认执行修复' : '开始验证' }}
          </el-button>
        </div>
      </div>

      <el-alert
        v-if="error"
        :title="error"
        type="error"
        show-icon
        class="error-alert"
      />
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import SeverityTag from './SeverityTag.vue'
import ScriptPreview from './ScriptPreview.vue'
import * as api from '@/api/vulnerability'

interface Vulnerability {
  id: string
  cve_id: string
  severity: 'Critical' | 'High' | 'Medium' | 'Low'
  cvss_score: number | null
  description: string
}

interface AffectedHost {
  id: string
  ip_address: string
  hostname: string
}

const props = defineProps<{
  visible: boolean
  mode: 'fix' | 'poc'
  cve: Vulnerability
  affectedHosts: AffectedHost[]
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'execute', data: { taskId: string; hosts: string[] }): void
}>()

const dialogVisible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})

const dialogTitle = computed(() => 
  props.mode === 'fix' ? '一键修复' : 'POC验证'
)

const selectedHosts = ref<string[]>([])
const script = ref('')
const generating = ref(false)
const executing = ref(false)
const error = ref('')

watch(() => props.visible, (val) => {
  if (val) {
    resetScript()
    selectedHosts.value = props.mode === 'poc' && props.affectedHosts.length > 0 
      ? [props.affectedHosts[0].id] 
      : []
  }
})

async function generateScript() {
  if (selectedHosts.value.length === 0) {
    ElMessage.warning('请选择目标主机')
    return
  }

  generating.value = true
  error.value = ''

  try {
    const hosts = Array.isArray(selectedHosts.value) ? selectedHosts.value : [selectedHosts.value]
    
    if (props.mode === 'fix') {
      const result = await api.generateFixScript(props.cve.cve_id, hosts, true)
      script.value = result.script || ''
    } else {
      const hostId = hosts[0]
      const result = await api.generatePocScript(props.cve.cve_id, hostId, true)
      script.value = result.script || ''
    }
  } catch (err: any) {
    error.value = err.message || '脚本生成失败'
    ElMessage.error(error.value)
  } finally {
    generating.value = false
  }
}

async function executeScript() {
  if (!script.value) {
    ElMessage.warning('请先生成脚本')
    return
  }

  executing.value = true

  try {
    const hosts = Array.isArray(selectedHosts.value) ? selectedHosts.value : [selectedHosts.value]
    
    let result
    if (props.mode === 'fix') {
      result = await api.generateFixScript(props.cve.cve_id, hosts, false)
    } else {
      result = await api.generatePocScript(props.cve.cve_id, hosts[0], false)
    }

    emit('execute', { taskId: result.task_id || '', hosts })
    ElMessage.success(props.mode === 'fix' ? '修复任务已创建' : 'POC 验证任务已创建')
    dialogVisible.value = false
  } catch (err: any) {
    error.value = err.message || '执行失败'
    ElMessage.error(error.value)
  } finally {
    executing.value = false
  }
}

function resetScript() {
  script.value = ''
  error.value = ''
}

function handleClose() {
  resetScript()
  selectedHosts.value = []
}
</script>

<style scoped>
.dialog-content {
  padding: 0;
}

.cve-info {
  margin-bottom: 20px;
}

.host-selection {
  margin-bottom: 20px;
}

.host-selection h4 {
  margin: 0 0 10px 0;
  font-size: 14px;
  color: #606266;
}

.script-actions {
  text-align: center;
  padding: 20px 0;
}

.hint-text {
  margin-top: 12px;
  font-size: 13px;
  color: #909399;
}

.script-preview-section {
  margin-top: 20px;
}

.action-buttons {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 16px;
}

.error-alert {
  margin-top: 16px;
}
</style>