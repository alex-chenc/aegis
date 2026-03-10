<template>
  <div class="workbench">
    <el-row :gutter="20">
      <el-col :span="16">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>模板上传</span>
            </div>
          </template>
          <el-upload
            :auto-upload="true"
            :show-file-list="false"
            :before-upload="handleBeforeUpload"
            :http-request="handleUpload"
            drag
          >
            <el-icon class="el-icon--upload"><upload-filled /></el-icon>
            <div class="el-upload__text">将基线文档拖到此处，或<em>点击上传</em></div>
          </el-upload>
        </el-card>

        <el-card style="margin-top: 20px" v-if="currentTemplate">
          <template #header>
            <div class="card-header">
              <span>规则列表 ({{ rules.length }} 条)</span>
              <div>
                <el-button @click="selectAllRules" type="primary" link>全选</el-button>
                <el-button @click="clearRules" link>清空</el-button>
              </div>
            </div>
          </template>
          <el-table :data="rules" v-loading="loading" @selection-change="handleSelectionChange">
            <el-table-column type="selection" width="55" />
            <el-table-column prop="title" label="规则标题" min-width="180" show-overflow-tooltip />
            <el-table-column label="检测内容" min-width="150">
              <template #default="{ row }">
                <el-tooltip :content="row.check_content" placement="top">
                  <span class="text-truncate">{{ truncate(row.check_content, 30) }}</span>
                </el-tooltip>
              </template>
            </el-table-column>
            <el-table-column label="修复内容" min-width="150">
              <template #default="{ row }">
                <el-tooltip :content="row.fix_content" placement="top">
                  <span class="text-truncate">{{ truncate(row.fix_content, 30) }}</span>
                </el-tooltip>
              </template>
            </el-table-column>
            <el-table-column label="脚本状态" width="100">
              <template #default="{ row }">
                <el-tag :type="getScriptStatusType(row.script_status)" size="small">
                  {{ getScriptStatusText(row.script_status) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="版本" width="80">
              <template #default="{ row }">
                <div class="version-info">
                  <div>检:v{{ row.check_script_version || 0 }}</div>
                  <div>修:v{{ row.fix_script_version || 0 }}</div>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="180" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" size="small" @click="showScript(row, 'check')">检测</el-button>
                <el-button link type="primary" size="small" @click="showScript(row, 'fix')">修复</el-button>
                <el-button link type="success" size="small" @click="editScript(row)">编辑</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>

      <el-col :span="8">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>解析状态</span>
              <el-button link @click="refreshStatus" :loading="statusLoading">刷新</el-button>
            </div>
          </template>
          <el-progress
            v-if="parseStatus"
            :percentage="parseStatus.progress"
            :status="parseStatus.status === 'completed' ? 'success' : parseStatus.status === 'failed' ? 'exception' : undefined"
          />
          <div style="margin-top: 10px; color: #666">{{ parseStatus?.message || '等待上传模板' }}</div>
        </el-card>

        <el-card style="margin-top: 20px" v-if="rules.length > 0">
          <template #header>
            <span>选择执行主机</span>
          </template>
          <div v-if="hosts.length === 0" style="color: #999">
            暂无在线主机，请先安装Agent
          </div>
          <el-checkbox-group v-else v-model="selectedHostIds">
            <div v-for="host in hosts" :key="host.id" style="margin-bottom: 8px">
              <el-checkbox :label="host.id">
                <span>{{ host.hostname }}</span>
                <el-tag size="small" style="margin-left: 8px" :type="host.online ? 'success' : 'danger'">
                  {{ host.online ? '在线' : '离线' }}
                </el-tag>
                <span style="color: #999; margin-left: 8px">{{ host.ip_address }}</span>
              </el-checkbox>
            </div>
          </el-checkbox-group>
        </el-card>

        <el-card style="margin-top: 20px" v-if="selectedRules.length > 0">
          <template #header>
            <span>执行操作</span>
          </template>
          <div style="margin-bottom: 10px">
            已选择 <strong>{{ selectedRules.length }}</strong> 条规则，
            <strong>{{ selectedHostIds.length }}</strong> 台主机
          </div>
          <el-button type="primary" @click="executeCheck" :disabled="selectedHostIds.length === 0" :loading="executing">
            下发检测
          </el-button>
          <el-button type="warning" @click="executeFix" :disabled="selectedHostIds.length === 0" :loading="executing">
            下发修复
          </el-button>
        </el-card>
      </el-col>
    </el-row>

    <el-dialog v-model="scriptDialogVisible" :title="scriptDialogTitle" width="70%">
      <div class="script-viewer">
        <div class="script-header">
          <span>脚本内容</span>
          <el-button link @click="copyScript">复制</el-button>
        </div>
        <pre class="script-content">{{ currentScript }}</pre>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { UploadFilled } from '@element-plus/icons-vue'
import { uploadTemplate, getTemplates, getTemplateStatus, getTemplateRules } from '@/api/templates'
import { useHostStore } from '@/store/hosts'
import { useTaskStore } from '@/store/tasks'
import type { UploadRequestOptions } from 'element-plus'
import type { Template, BaselineRule, ParseStatus, Host } from '@/types'

const router = useRouter()
const hostStore = useHostStore()
const taskStore = useTaskStore()

const loading = ref(false)
const statusLoading = ref(false)
const executing = ref(false)
const templates = ref<Template[]>([])
const currentTemplate = ref<Template | null>(null)
const rules = ref<BaselineRule[]>([])
const parseStatus = ref<ParseStatus | null>(null)
const selectedRules = ref<BaselineRule[]>([])
const selectedHostIds = ref<string[]>([])
const scriptDialogVisible = ref(false)
const scriptDialogTitle = ref('')
const currentScript = ref('')

const hosts = computed(() => hostStore.hosts)

let statusPollTimer: number | null = null

const handleBeforeUpload = (file: File) => {
  const allowedTypes = ['application/pdf', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', 'application/msword', 'text/yaml', 'application/yaml']
  const ext = file.name.split('.').pop()?.toLowerCase()
  if (!['pdf', 'docx', 'doc', 'yaml', 'yml'].includes(ext || '') && !allowedTypes.includes(file.type)) {
    ElMessage.error('只支持 PDF、Word、YAML 格式的文件')
    return false
  }
  return true
}

const handleUpload = async (options: UploadRequestOptions) => {
  try {
    const result = await uploadTemplate(options.file as File)
    ElMessage.success('上传成功')
    await fetchTemplates()
    const newTemplate = templates.value.find(t => t.id === result.template_id)
    if (newTemplate) {
      currentTemplate.value = newTemplate
      startStatusPoll(newTemplate.id)
    }
  } catch (e: any) {
    ElMessage.error(e.message || '上传失败')
  }
}

const fetchTemplates = async () => {
  loading.value = true
  try {
    templates.value = await getTemplates()
    if (templates.value.length > 0 && !currentTemplate.value) {
      const latest = templates.value[0]
      currentTemplate.value = latest
      if (latest.status === 'parsing') {
        startStatusPoll(latest.id)
      } else if (latest.status === 'completed') {
        await fetchRules(latest.id)
      }
    }
  } finally {
    loading.value = false
  }
}

const fetchRules = async (templateId: string) => {
  loading.value = true
  try {
    rules.value = await getTemplateRules(templateId)
    parseStatus.value = { status: 'completed', progress: 100, message: `解析完成，共 ${rules.value.length} 条规则` }
  } catch (e: any) {
    ElMessage.error(e.message || '获取规则失败')
  } finally {
    loading.value = false
  }
}

const startStatusPoll = (templateId: string) => {
  if (statusPollTimer) clearInterval(statusPollTimer)
  
  const poll = async () => {
    try {
      statusLoading.value = true
      const status = await getTemplateStatus(templateId)
      parseStatus.value = status
      
      if (status.status === 'completed') {
        if (statusPollTimer) clearInterval(statusPollTimer)
        await fetchRules(templateId)
      } else if (status.status === 'failed') {
        if (statusPollTimer) clearInterval(statusPollTimer)
      }
    } finally {
      statusLoading.value = false
    }
  }
  
  poll()
  statusPollTimer = window.setInterval(poll, 2000)
}

const refreshStatus = async () => {
  if (!currentTemplate.value) return
  try {
    statusLoading.value = true
    const status = await getTemplateStatus(currentTemplate.value.id)
    parseStatus.value = status
    if (status.status === 'completed') {
      await fetchRules(currentTemplate.value.id)
    }
  } finally {
    statusLoading.value = false
  }
}

const handleSelectionChange = (selection: BaselineRule[]) => {
  selectedRules.value = selection
}

const selectAllRules = () => {
  selectedRules.value = [...rules.value]
}

const clearRules = () => {
  selectedRules.value = []
}

const getScriptStatusType = (status: string) => {
  switch (status) {
    case 'generated': return 'success'
    case 'pending': return 'warning'
    case 'failed': return 'danger'
    default: return 'info'
  }
}

const getScriptStatusText = (status: string) => {
  switch (status) {
    case 'generated': return '已生成'
    case 'pending': return '待生成'
    case 'failed': return '生成失败'
    default: return status
  }
}

const showScript = (rule: BaselineRule, type: 'check' | 'fix') => {
  scriptDialogTitle.value = type === 'check' ? `检测脚本 - ${rule.title}` : `修复脚本 - ${rule.title}`
  currentScript.value = type === 'check' 
    ? (rule.generated_check_script || '// 检测脚本尚未生成')
    : (rule.generated_fix_script || '// 修复脚本尚未生成')
  scriptDialogVisible.value = true
}

const copyScript = async () => {
  if (!currentScript.value) {
    ElMessage.warning('没有可复制的内容')
    return
  }
  
  const success = await copyToClipboard(currentScript.value)
  if (success) {
    ElMessage.success('已复制到剪贴板')
  } else {
    ElMessage.error('复制失败')
  }
}

const copyToClipboard = async (text: string): Promise<boolean> => {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch (err) {
      console.warn('Clipboard API failed:', err)
    }
  }
  
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  textarea.style.top = '0'
  document.body.appendChild(textarea)
  textarea.focus()
  textarea.select()
  
  try {
    const success = document.execCommand('copy')
    document.body.removeChild(textarea)
    return success
  } catch {
    document.body.removeChild(textarea)
    return false
  }
}

const truncate = (text: string, length: number): string => {
  if (!text) return ''
  return text.length > length ? text.substring(0, length) + '...' : text
}

const editScript = (rule: BaselineRule) => {
  ElMessage.info('脚本编辑功能开发中...')
}

const executeCheck = async () => {
  if (selectedRules.value.length === 0 || selectedHostIds.value.length === 0) {
    ElMessage.warning('请选择规则和主机')
    return
  }
  executing.value = true
  try {
    taskStore.setSelectedRules(selectedRules.value)
    taskStore.setSelectedHosts(selectedHostIds.value)
    const result = await taskStore.executeCheck()
    if (result) {
      ElMessageBox.confirm(
        `检测任务已下发，任务ID: ${result.task_group_id}`,
        '任务已创建',
        {
          confirmButtonText: '查看任务',
          cancelButtonText: '关闭',
          type: 'success'
        }
      ).then(() => {
        router.push(`/tasks/${result.task_group_id}`)
      }).catch(() => {})
    }
  } catch (e: any) {
    ElMessage.error(e.message || '下发失败')
  } finally {
    executing.value = false
  }
}

const executeFix = async () => {
  if (selectedRules.value.length === 0 || selectedHostIds.value.length === 0) {
    ElMessage.warning('请选择规则和主机')
    return
  }
  executing.value = true
  try {
    taskStore.setSelectedRules(selectedRules.value)
    taskStore.setSelectedHosts(selectedHostIds.value)
    const result = await taskStore.executeFix()
    if (result) {
      ElMessageBox.confirm(
        `修复任务已下发，任务ID: ${result.task_group_id}`,
        '任务已创建',
        {
          confirmButtonText: '查看任务',
          cancelButtonText: '关闭',
          type: 'success'
        }
      ).then(() => {
        router.push(`/tasks/${result.task_group_id}`)
      }).catch(() => {})
    }
  } catch (e: any) {
    ElMessage.error(e.message || '下发失败')
  } finally {
    executing.value = false
  }
}

onMounted(async () => {
  await hostStore.fetchHosts()
  await fetchTemplates()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.script-viewer {
  background: #1e1e1e;
  border-radius: 4px;
  padding: 12px;
}
.script-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: #fff;
  margin-bottom: 8px;
}
.script-content {
  color: #d4d4d4;
  font-family: 'Fira Code', monospace;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}
.text-truncate {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.version-info {
  font-size: 11px;
  color: #666;
}
</style>