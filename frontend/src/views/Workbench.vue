<template>
  <div class="workbench">
    <el-row :gutter="24">
      <el-col :span="16">
        <el-card class="upload-card">
          <template #header>
            <div class="card-header">
              <span class="card-title">模板上传</span>
            </div>
          </template>
          <el-upload
            :auto-upload="true"
            :show-file-list="false"
            :before-upload="handleBeforeUpload"
            :http-request="handleUpload"
            drag
            class="upload-dragger"
          >
            <el-icon class="el-icon--upload"><upload-filled /></el-icon>
            <div class="el-upload__text">将基线文档拖到此处，或<em>点击上传</em></div>
            <template #tip>
              <div class="el-upload__tip">支持 PDF、Word、YAML 格式，最大 5MB</div>
            </template>
          </el-upload>
        </el-card>

        <el-card class="rules-card" v-if="templates.length > 0">
          <template #header>
            <div class="rules-card-header">
              <div class="rules-title-section">
                <span class="card-title">规则列表</span>
                <span class="rules-count">共 {{ totalRulesCount }} 条规则，{{ templates.length }} 个文件</span>
              </div>
            </div>
          </template>
          
          <div class="rules-container">
            <div 
              v-for="tpl in paginatedTemplates" 
              :key="tpl.id" 
              class="file-section"
            >
              <div class="file-header">
                <div class="file-info">
                  <el-icon class="file-icon"><Document /></el-icon>
                  <span class="file-name">{{ tpl.display_name || tpl.name }}</span>
                  <el-tag 
                    size="small" 
                    :type="tpl.status === 'completed' ? 'success' : tpl.status === 'failed' ? 'danger' : 'warning'"
                    effect="light"
                  >
                    {{ tpl.status === 'completed' ? '已完成' : tpl.status === 'failed' ? '失败' : '解析中' }}
                  </el-tag>
                  <span class="file-meta">{{ tpl.rule_count }} 条规则 · {{ formatTime(tpl.created_at) }}</span>
                </div>
                <div class="file-actions">
                  <el-button 
                    type="primary" 
                    size="small" 
                    plain
                    @click="batchGenerateScripts(tpl.id, 'CHECK')"
                    :loading="batchGeneratingMap[`${tpl.id}-CHECK`]"
                    :disabled="batchGeneratingMap[`${tpl.id}-CHECK`] || tpl.status !== 'completed' || tpl.rule_count === 0"
                  >
                    一键生成检测脚本
                  </el-button>
                  <el-button 
                    type="warning" 
                    size="small" 
                    plain
                    @click="batchGenerateScripts(tpl.id, 'FIX')"
                    :loading="batchGeneratingMap[`${tpl.id}-FIX`]"
                    :disabled="batchGeneratingMap[`${tpl.id}-FIX`] || tpl.status !== 'completed' || tpl.rule_count === 0"
                  >
                    一键生成修复脚本
                  </el-button>
                  <el-button 
                    type="danger" 
                    size="small" 
                    text
                    @click="confirmDeleteTemplate(tpl)" 
                    :disabled="tpl.status === 'parsing'"
                  >
                    <el-icon><Delete /></el-icon>
                  </el-button>
                </div>
              </div>
              
              <el-collapse v-if="tpl.status === 'completed' && tpl.rule_count > 0" class="rules-collapse">
                <el-collapse-item>
                  <template #title>
                    <span class="collapse-title">
                      <el-icon><List /></el-icon>
                      查看规则列表
                      <el-badge :value="getRuleCount(tpl.id)" type="primary" />
                    </span>
                  </template>
                  
                  <el-table 
                    :data="getPaginatedRules(tpl.id)" 
                    size="small"
                    stripe
                    v-loading="loadingTemplateId === tpl.id"
                    @selection-change="(selection: BaselineRule[]) => handleTemplateSelectionChange(tpl.id, selection)"
                    class="rules-table"
                  >
                    <el-table-column type="selection" width="45" />
                    <el-table-column prop="title" label="规则标题" min-width="140" show-overflow-tooltip />
                    <el-table-column label="检测内容" min-width="180">
                      <template #default="{ row }">
                        <el-tooltip :content="row.check_content" placement="top" :disabled="!row.check_content || row.check_content.length <= 30">
                          <span class="truncated-text">{{ truncate(row.check_content, 30) }}</span>
                        </el-tooltip>
                      </template>
                    </el-table-column>
                    <el-table-column label="修复方法" min-width="180">
                      <template #default="{ row }">
                        <el-tooltip :content="row.fix_content" placement="top" :disabled="!row.fix_content || row.fix_content.length <= 30">
                          <span class="truncated-text">{{ truncate(row.fix_content, 30) }}</span>
                        </el-tooltip>
                      </template>
                    </el-table-column>
                    <el-table-column label="脚本" width="100" align="center">
                      <template #default="{ row }">
                        <div class="script-buttons">
                          <el-button 
                            link 
                            :type="getScriptButtonType(row.check_script_status)" 
                            size="small"
                            @click="openScriptEditor(row, 'CHECK')"
                            :loading="row.check_script_status === 'generating'"
                          >
                            检测
                          </el-button>
                          <el-button 
                            link 
                            :type="getScriptButtonType(row.fix_script_status)" 
                            size="small"
                            @click="openScriptEditor(row, 'FIX')"
                            :loading="row.fix_script_status === 'generating'"
                          >
                            修复
                          </el-button>
                        </div>
                      </template>
                    </el-table-column>
                  </el-table>
                  
                  <div class="rule-pagination" v-if="getRuleCount(tpl.id) > rulePageSize">
                    <el-pagination
                      v-model:current-page="rulePageMap[tpl.id]"
                      :page-size="rulePageSize"
                      :total="getRuleCount(tpl.id)"
                      layout="total, prev, pager, next"
                      small
                      background
                    />
                  </div>
                </el-collapse-item>
              </el-collapse>
              
              <div v-else-if="tpl.status === 'parsing'" class="parsing-status">
                <el-progress :percentage="50" :indeterminate="true" :stroke-width="6" />
                <span>正在解析中...</span>
              </div>
              
              <div v-else-if="tpl.status === 'failed'" class="failed-status">
                <el-icon><WarningFilled /></el-icon>
                <div class="failed-content">
                  <span class="failed-title">解析失败</span>
                  <el-tooltip 
                    v-if="tpl.error_message" 
                    :content="tpl.error_message" 
                    placement="top"
                    :disabled="tpl.error_message.length <= 100"
                  >
                    <span class="failed-message">{{ truncate(tpl.error_message, 100) }}</span>
                  </el-tooltip>
                </div>
              </div>
            </div>
          </div>
          
          <div class="file-pagination-wrapper" v-if="templates.length > filePageSize">
            <el-pagination
              v-model:current-page="fileCurrentPage"
              :page-size="filePageSize"
              :total="templates.length"
              layout="total, prev, pager, next"
              background
            />
          </div>
        </el-card>
      </el-col>

      <el-col :span="8">
        <el-card class="sidebar-card">
          <template #header>
            <div class="card-header">
              <span class="card-title">已上传文件</span>
              <el-badge :value="templates.length" type="primary" />
            </div>
          </template>
          <div v-if="templates.length === 0" class="empty-state">
            <el-empty description="暂无已解析文件，请上传基线文档" :image-size="80" />
          </div>
          <div v-else class="file-list">
            <div v-for="tpl in paginatedFileList" :key="tpl.id" class="file-list-item">
              <div class="file-list-info">
                <el-icon class="file-list-icon"><Document /></el-icon>
                <span class="file-list-name">{{ tpl.display_name || tpl.name }}</span>
              </div>
              <el-tag size="small" :type="tpl.status === 'completed' ? 'success' : 'info'" effect="light">
                {{ tpl.rule_count }} 条
              </el-tag>
            </div>
            
            <div class="sidebar-pagination" v-if="templates.length > sidebarPageSize">
              <el-pagination
                v-model:current-page="sidebarCurrentPage"
                :page-size="sidebarPageSize"
                :total="templates.length"
                layout="prev, pager, next"
                small
              />
            </div>
          </div>
        </el-card>

        <el-card class="sidebar-card" v-if="allSelectedRules.length > 0">
          <template #header>
            <span class="card-title">选择执行主机</span>
          </template>
          <div v-if="hosts.length === 0" class="empty-state">
            <span class="empty-text">暂无在线主机，请先安装Agent</span>
          </div>
          <el-checkbox-group v-else v-model="selectedHostIds" class="host-checkbox-group">
            <div v-for="host in hosts" :key="host.id" class="host-item">
              <el-checkbox :label="host.id">
                <span class="host-name">{{ host.hostname }}</span>
                <el-tag size="small" :type="host.online ? 'success' : 'danger'" effect="light">
                  {{ host.online ? '在线' : '离线' }}
                </el-tag>
                <span class="host-ip">{{ host.ip_address }}</span>
              </el-checkbox>
            </div>
          </el-checkbox-group>
        </el-card>

        <el-card class="sidebar-card execute-card" v-if="allSelectedRules.length > 0">
          <template #header>
            <span class="card-title">执行操作</span>
          </template>
          <div class="execute-summary">
            已选择 <strong>{{ allSelectedRules.length }}</strong> 条规则，
            <strong>{{ selectedHostIds.length }}</strong> 台主机
          </div>
          
          <el-alert 
            v-if="checkScriptWarning" 
            :title="checkScriptWarning" 
            type="warning" 
            :closable="false"
            show-icon
            class="script-alert"
          />
          <el-alert 
            v-if="fixScriptWarning" 
            :title="fixScriptWarning" 
            type="warning" 
            :closable="false"
            show-icon
            class="script-alert"
          />
          
          <div class="execute-buttons">
            <el-tooltip 
              :content="checkScriptWarning || '下发检测任务到选中主机'" 
              placement="top"
              :disabled="!checkScriptWarning"
            >
              <el-button 
                type="primary" 
                @click="executeCheck" 
                :disabled="selectedHostIds.length === 0 || !checkScriptStatus.ready" 
                :loading="executing"
              >
                <el-icon><VideoPlay /></el-icon>
                下发检测
              </el-button>
            </el-tooltip>
            <el-tooltip 
              :content="fixScriptWarning || '下发修复任务到选中主机'" 
              placement="top"
              :disabled="!fixScriptWarning"
            >
              <el-button 
                type="warning" 
                @click="executeFix" 
                :disabled="selectedHostIds.length === 0 || !fixScriptStatus.ready" 
                :loading="executing"
              >
                <el-icon><Tools /></el-icon>
                下发修复
              </el-button>
            </el-tooltip>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-dialog v-model="scriptDialogVisible" :title="scriptDialogTitle" width="70%" :close-on-click-modal="false" class="script-dialog">
      <div v-if="scriptLoading" class="loading-container">
        <el-icon class="is-loading" :size="32"><Loading /></el-icon>
        <span class="loading-text">正在请求AI生成脚本...</span>
      </div>
      <div v-else>
        <el-input
          v-model="scriptContent"
          type="textarea"
          :rows="20"
          class="script-editor"
          placeholder="脚本内容"
        />
      </div>
      <template #footer>
        <el-button @click="scriptDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveScript" :loading="scriptSaving" :disabled="scriptLoading || !scriptContent">
          保存
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { UploadFilled, Loading, Document, Delete, List, WarningFilled, VideoPlay, Tools } from '@element-plus/icons-vue'
import SparkMD5 from 'spark-md5'
import { 
  uploadTemplate, 
  getTemplates, 
  getTemplateRules, 
  generateScript, 
  updateScript, 
  deleteTemplate,
  checkFileMD5,
  batchGenerateScripts as batchGenerateScriptsApi
} from '@/api/templates'
import { useHostStore } from '@/store/hosts'
import { useTaskStore } from '@/store/tasks'
import type { UploadRequestOptions } from 'element-plus'
import type { Template, BaselineRule } from '@/types'

const router = useRouter()
const hostStore = useHostStore()
const taskStore = useTaskStore()

const loading = ref(false)
const executing = ref(false)
const templates = ref<Template[]>([])
const templateRulesMap = reactive<Record<string, BaselineRule[]>>({})
const templateSelectionMap = reactive<Record<string, BaselineRule[]>>({})
const loadingTemplateId = ref<string | null>(null)
const selectedHostIds = ref<string[]>([])

const scriptDialogVisible = ref(false)
const scriptDialogTitle = ref('')
const scriptContent = ref('')
const scriptLoading = ref(false)
const scriptSaving = ref(false)
const currentEditRule = ref<BaselineRule | null>(null)
const currentScriptType = ref<'CHECK' | 'FIX'>('CHECK')

const rulePageSize = 10
const rulePageMap = reactive<Record<string, number>>({})

const filePageSize = 5
const fileCurrentPage = ref(1)

const sidebarPageSize = 5
const sidebarCurrentPage = ref(1)

const batchGeneratingMap = reactive<Record<string, boolean>>({})

const hosts = computed(() => hostStore.hosts)

const allSelectedRules = computed(() => {
  const all: BaselineRule[] = []
  Object.values(templateSelectionMap).forEach(rules => {
    all.push(...rules)
  })
  return all
})

const checkScriptStatus = computed(() => {
  const rules = allSelectedRules.value
  if (rules.length === 0) return { ready: true, pending: 0, generating: 0 }
  
  let pending = 0
  let generating = 0
  
  rules.forEach(rule => {
    if (rule.check_script_status === 'generating') {
      generating++
    } else if (rule.check_script_status !== 'generated') {
      pending++
    }
  })
  
  return { ready: pending === 0 && generating === 0, pending, generating }
})

const fixScriptStatus = computed(() => {
  const rules = allSelectedRules.value
  if (rules.length === 0) return { ready: true, pending: 0, generating: 0 }
  
  let pending = 0
  let generating = 0
  
  rules.forEach(rule => {
    if (rule.fix_script_status === 'generating') {
      generating++
    } else if (rule.fix_script_status !== 'generated') {
      pending++
    }
  })
  
  return { ready: pending === 0 && generating === 0, pending, generating }
})

const checkScriptWarning = computed(() => {
  if (checkScriptStatus.value.ready) return ''
  const { pending, generating } = checkScriptStatus.value
  if (generating > 0) {
    return `有 ${generating} 个检测脚本正在生成中，请稍候...`
  }
  if (pending > 0) {
    return `有 ${pending} 个检测脚本未生成，请点击"一键生成检测脚本"或单独生成`
  }
  return ''
})

const fixScriptWarning = computed(() => {
  if (fixScriptStatus.value.ready) return ''
  const { pending, generating } = fixScriptStatus.value
  if (generating > 0) {
    return `有 ${generating} 个修复脚本正在生成中，请稍候...`
  }
  if (pending > 0) {
    return `有 ${pending} 个修复脚本未生成，请点击"一键生成修复脚本"或单独生成`
  }
  return ''
})

const totalRulesCount = computed(() => {
  return Object.values(templateRulesMap).reduce((sum, rules) => sum + rules.length, 0)
})

const paginatedTemplates = computed(() => {
  const start = (fileCurrentPage.value - 1) * filePageSize
  const end = start + filePageSize
  return templates.value.slice(start, end)
})

const paginatedFileList = computed(() => {
  const start = (sidebarCurrentPage.value - 1) * sidebarPageSize
  const end = start + sidebarPageSize
  return templates.value.slice(start, end)
})

const getRuleCount = (templateId: string): number => {
  return templateRulesMap[templateId]?.length || 0
}

const getPaginatedRules = (templateId: string): BaselineRule[] => {
  const rules = templateRulesMap[templateId] || []
  const page = rulePageMap[templateId] || 1
  const start = (page - 1) * rulePageSize
  const end = start + rulePageSize
  return rules.slice(start, end)
}

const handleRulePageChange = (templateId: string, page: number) => {
  rulePageMap[templateId] = page
}

const handleFilePageChange = (page: number) => {
  fileCurrentPage.value = page
}

const handleSidebarPageChange = (page: number) => {
  sidebarCurrentPage.value = page
}

const handleBeforeUpload = (file: File) => {
  if (file.size > 5 * 1024 * 1024) {
    ElMessage.error('文件大小超过 5MB，无法解析')
    return false
  }
  const allowedTypes = ['application/pdf', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', 'application/msword', 'text/yaml', 'application/yaml']
  const ext = file.name.split('.').pop()?.toLowerCase()
  if (!['pdf', 'docx', 'doc', 'yaml', 'yml'].includes(ext || '') && !allowedTypes.includes(file.type)) {
    ElMessage.error('只支持 PDF、Word、YAML 格式的文件')
    return false
  }
  return true
}

const calculateMD5 = (file: File): Promise<string> => {
  return new Promise((resolve, reject) => {
    const blobSlice = File.prototype.slice
    const chunkSize = 2097152
    const chunks = Math.ceil(file.size / chunkSize)
    let currentChunk = 0
    const spark = new SparkMD5.ArrayBuffer()
    const fileReader = new FileReader()

    fileReader.onload = (e) => {
      spark.append(e.target?.result as ArrayBuffer)
      currentChunk++
      if (currentChunk < chunks) {
        loadNext()
      } else {
        resolve(spark.end())
      }
    }

    fileReader.onerror = () => {
      reject(new Error('文件读取失败'))
    }

    const loadNext = () => {
      const start = currentChunk * chunkSize
      const end = Math.min(start + chunkSize, file.size)
      fileReader.readAsArrayBuffer(blobSlice.call(file, start, end))
    }

    loadNext()
  })
}

const handleUpload = async (options: UploadRequestOptions) => {
  const file = options.file as File
  
  try {
    const md5 = await calculateMD5(file)
    const checkResult = await checkFileMD5(md5)
    
    if (checkResult.exists) {
      await ElMessageBox.confirm(
        `该文件已解析过（${checkResult.filename}），是否继续上传？`,
        '文件已存在',
        {
          confirmButtonText: '继续上传',
          cancelButtonText: '取消',
          type: 'info'
        }
      )
    }
    
    const result = await uploadTemplate(file, md5)
    
    if (result.exists) {
      ElMessage.info('文件已存在，跳过解析')
      return
    }
    
    ElMessage.success('上传成功，正在解析...')
    await fetchTemplates()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.message || '上传失败')
    }
  }
}

const fetchTemplates = async () => {
  loading.value = true
  try {
    templates.value = await getTemplates()
    templates.value.forEach(async (tpl) => {
      if (tpl.status === 'completed' && tpl.rule_count > 0) {
        await loadTemplateRules(tpl.id)
      }
    })
  } finally {
    loading.value = false
  }
}

const loadTemplateRules = async (templateId: string) => {
  loadingTemplateId.value = templateId
  try {
    const rules = await getTemplateRules(templateId)
    rules.forEach(rule => {
      if (!rule.check_script_status || rule.check_script_status === 'ready') {
        rule.check_script_status = rule.generated_check_script ? 'generated' : 'pending'
      }
      if (!rule.fix_script_status || rule.fix_script_status === 'ready') {
        rule.fix_script_status = rule.generated_fix_script ? 'generated' : 'pending'
      }
    })
    templateRulesMap[templateId] = rules
    rulePageMap[templateId] = 1
  } catch (e: any) {
    console.error('Failed to load rules:', e)
  } finally {
    loadingTemplateId.value = null
  }
}

const handleTemplateSelectionChange = (templateId: string, selection: BaselineRule[]) => {
  templateSelectionMap[templateId] = selection
}

const confirmDeleteTemplate = async (tpl: Template) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除文件 "${tpl.display_name || tpl.name}" 及其 ${tpl.rule_count} 条规则吗？此操作不可撤销。`,
      '确认删除',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    await deleteTemplate(tpl.id)
    ElMessage.success('删除成功')
    
    delete templateRulesMap[tpl.id]
    delete templateSelectionMap[tpl.id]
    delete rulePageMap[tpl.id]
    
    await fetchTemplates()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e.message || '删除失败')
    }
  }
}

const formatTime = (dateStr: string): string => {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hour = String(date.getHours()).padStart(2, '0')
  const minute = String(date.getMinutes()).padStart(2, '0')
  return `${month}-${day} ${hour}:${minute}`
}

const getScriptButtonType = (status: string) => {
  switch (status) {
    case 'generated': return 'success'
    case 'pending': return 'primary'
    case 'generating': return 'info'
    case 'failed': return 'danger'
    default: return 'primary'
  }
}

const truncate = (text: string, length: number): string => {
  if (!text) return ''
  return text.length > length ? text.substring(0, length) + '...' : text
}

const openScriptEditor = async (rule: BaselineRule, scriptType: 'CHECK' | 'FIX') => {
  const statusField = scriptType === 'CHECK' ? 'check_script_status' : 'fix_script_status'
  if (rule[statusField] === 'generating') {
    ElMessage.info('脚本正在生成中，请稍候')
    return
  }

  currentEditRule.value = rule
  currentScriptType.value = scriptType
  scriptDialogTitle.value = scriptType === 'CHECK' ? `编辑检测脚本 - ${rule.title}` : `编辑修复脚本 - ${rule.title}`
  
  const existingScript = scriptType === 'CHECK' ? rule.generated_check_script : rule.generated_fix_script
  
  if (existingScript) {
    scriptContent.value = existingScript
    scriptLoading.value = false
    scriptDialogVisible.value = true
  } else {
    rule[statusField] = 'generating'
    scriptLoading.value = true
    scriptContent.value = ''
    scriptDialogVisible.value = true
    
    try {
      const result = await generateScript(rule.id, scriptType)
      if (result.script_content) {
        scriptContent.value = result.script_content
        rule[statusField] = 'generated'
        if (scriptType === 'CHECK') {
          rule.generated_check_script = result.script_content
        } else {
          rule.generated_fix_script = result.script_content
        }
      } else if (result.status === 'generating') {
        ElMessage.info('脚本生成任务已提交，请稍后刷新查看')
        scriptDialogVisible.value = false
      }
    } catch (e: any) {
      rule[statusField] = 'failed'
      const errorMsg = e.message || '生成脚本失败'
      if (scriptType === 'CHECK') {
        rule.check_script_error = errorMsg
      } else {
        rule.fix_script_error = errorMsg
      }
      ElMessage.error(errorMsg)
      scriptDialogVisible.value = false
    } finally {
      scriptLoading.value = false
    }
  }
}

const saveScript = async () => {
  if (!currentEditRule.value || !scriptContent.value) return
  
  scriptSaving.value = true
  try {
    await updateScript(currentEditRule.value.id, currentScriptType.value, scriptContent.value)
    ElMessage.success('脚本保存成功')
    
    if (currentScriptType.value === 'CHECK') {
      currentEditRule.value.generated_check_script = scriptContent.value
      currentEditRule.value.check_script_status = 'generated'
    } else {
      currentEditRule.value.generated_fix_script = scriptContent.value
      currentEditRule.value.fix_script_status = 'generated'
    }
    
    scriptDialogVisible.value = false
  } catch (e: any) {
    ElMessage.error(e.message || '保存失败')
  } finally {
    scriptSaving.value = false
  }
}

const batchGenerateScripts = async (templateId: string, scriptType: 'CHECK' | 'FIX') => {
  const key = `${templateId}-${scriptType}`
  batchGeneratingMap[key] = true
  
  try {
    const result = await batchGenerateScriptsApi(templateId, scriptType)
    
    if (result.queued > 0) {
      ElMessage.success(`已提交 ${result.queued} 个脚本生成任务，已存在 ${result.generated} 个`)
      
      const rules = templateRulesMap[templateId] || []
      rules.forEach(rule => {
        const statusField = scriptType === 'CHECK' ? 'check_script_status' : 'fix_script_status'
        const hasScript = scriptType === 'CHECK' 
          ? rule.generated_check_script 
          : rule.generated_fix_script
        if (!hasScript && rule[statusField] !== 'generating') {
          rule[statusField] = 'generating'
        }
      })
    } else if (result.generated > 0) {
      ElMessage.info(`所有 ${result.generated} 个脚本已生成`)
    } else if (result.skipped > 0) {
      ElMessage.info(`${result.skipped} 个脚本正在生成中`)
    }
  } catch (e: any) {
    ElMessage.error(e.message || '批量生成失败')
  } finally {
    batchGeneratingMap[key] = false
  }
}

const executeCheck = async () => {
  if (allSelectedRules.value.length === 0 || selectedHostIds.value.length === 0) {
    ElMessage.warning('请选择规则和主机')
    return
  }
  executing.value = true
  try {
    taskStore.setSelectedRules(allSelectedRules.value)
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
  if (allSelectedRules.value.length === 0 || selectedHostIds.value.length === 0) {
    ElMessage.warning('请选择规则和主机')
    return
  }
  executing.value = true
  try {
    taskStore.setSelectedRules(allSelectedRules.value)
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
.workbench {
  padding: 0;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.upload-card {
  margin-bottom: 20px;
  border-radius: 12px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.04);
}

.upload-dragger :deep(.el-upload-dragger) {
  border-radius: 8px;
  border: 2px dashed #dcdfe6;
  transition: all 0.3s ease;
}

.upload-dragger :deep(.el-upload-dragger:hover) {
  border-color: #409eff;
  background-color: #f5f7fa;
}

.rules-card {
  border-radius: 12px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.04);
}

.rules-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.rules-title-section {
  display: flex;
  align-items: center;
  gap: 16px;
}

.rules-count {
  font-size: 13px;
  color: #909399;
  font-weight: 400;
}

.rules-container {
  max-height: 600px;
  overflow-y: auto;
  padding-right: 8px;
}

.rules-container::-webkit-scrollbar {
  width: 6px;
}

.rules-container::-webkit-scrollbar-thumb {
  background-color: #dcdfe6;
  border-radius: 3px;
}

.rules-container::-webkit-scrollbar-track {
  background-color: #f5f7fa;
  border-radius: 3px;
}

.file-section {
  background: #fafbfc;
  border-radius: 10px;
  padding: 16px;
  margin-bottom: 16px;
  border: 1px solid #ebeef5;
  transition: all 0.3s ease;
}

.file-section:hover {
  border-color: #c0c4cc;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.06);
}

.file-section:last-child {
  margin-bottom: 0;
}

.file-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.file-info {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  min-width: 0;
}

.file-icon {
  font-size: 18px;
  color: #409eff;
}

.file-name {
  font-weight: 600;
  font-size: 14px;
  color: #303133;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 200px;
}

.file-meta {
  font-size: 12px;
  color: #909399;
  white-space: nowrap;
}

.file-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.rules-collapse {
  border: none;
  background: transparent;
}

.rules-collapse :deep(.el-collapse-item__header) {
  background: transparent;
  border: none;
  height: 36px;
  line-height: 36px;
  padding: 0 12px;
  border-radius: 6px;
}

.rules-collapse :deep(.el-collapse-item__wrap) {
  background: transparent;
  border: none;
}

.rules-collapse :deep(.el-collapse-item__content) {
  padding-bottom: 0;
}

.collapse-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #606266;
}

.rules-table {
  border-radius: 8px;
  overflow: hidden;
}

.rules-table :deep(.el-table__header th) {
  background-color: #f5f7fa;
}

.truncated-text {
  color: #606266;
  font-size: 13px;
}

.script-buttons {
  display: flex;
  gap: 8px;
  justify-content: center;
}

.rule-pagination {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}

.parsing-status {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px;
  color: #909399;
  font-size: 13px;
}

.parsing-status .el-progress {
  width: 120px;
}

.failed-status {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 16px;
  color: #f56c6c;
  font-size: 13px;
}

.failed-content {
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
  min-width: 0;
}

.failed-title {
  font-weight: 500;
}

.failed-message {
  color: #909399;
  font-size: 12px;
  word-break: break-all;
  line-height: 1.5;
}

.file-pagination-wrapper {
  display: flex;
  justify-content: center;
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid #ebeef5;
}

.sidebar-card {
  border-radius: 12px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.04);
  margin-bottom: 20px;
}

.empty-state {
  padding: 20px 0;
}

.empty-text {
  color: #909399;
  font-size: 13px;
}

.file-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.file-list-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 14px;
  background: #fafbfc;
  border-radius: 8px;
  transition: all 0.2s ease;
}

.file-list-item:hover {
  background: #f0f2f5;
}

.file-list-info {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

.file-list-icon {
  font-size: 16px;
  color: #909399;
}

.file-list-name {
  font-size: 13px;
  color: #303133;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar-pagination {
  margin-top: 12px;
  display: flex;
  justify-content: center;
}

.host-checkbox-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.host-item {
  padding: 8px 12px;
  background: #fafbfc;
  border-radius: 6px;
}

.host-name {
  font-weight: 500;
  margin-right: 8px;
}

.host-ip {
  color: #909399;
  font-size: 12px;
  margin-left: 8px;
}

.execute-card .execute-summary {
  font-size: 13px;
  color: #606266;
  margin-bottom: 16px;
  padding: 12px;
  background: #f5f7fa;
  border-radius: 6px;
}

.execute-summary strong {
  color: #409eff;
  font-weight: 600;
}

.execute-buttons {
  display: flex;
  gap: 12px;
}

.execute-buttons .el-button {
  flex: 1;
}

.script-alert {
  margin-bottom: 12px;
  border-radius: 8px;
}

.script-alert :deep(.el-alert__title) {
  font-size: 13px;
}

.script-dialog :deep(.el-dialog__body) {
  padding: 20px;
}

.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px;
  color: #409eff;
}

.loading-text {
  margin-top: 16px;
  font-size: 14px;
  color: #909399;
}

.script-editor :deep(textarea) {
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.6;
  border-radius: 8px;
}
</style>