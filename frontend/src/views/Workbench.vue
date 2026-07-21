<template>
  <div class="workbench page-shell">
    <section class="page-hero workbench-hero">
      <h1>{{ $t('generated.workbench_rule_management_9cf357') }}</h1>
      <p>{{ $t('generated.workbench_centrally_parse_baseline_documents_maintain_detection_1543d8') }}</p>
    </section>

    <el-card class="toolbar-card">
      <div class="toolbar-row">
        <el-input
          v-model="ruleSearch"
          :placeholder="$t('generated.workbench_search_rules_inspection_items_repair_items_047c85')"
          clearable
          class="rule-search"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>

        <el-select v-model="templateFilter" :placeholder="$t('generated.workbench_template_06d0f3')" clearable class="template-filter">
          <el-option
            v-for="tpl in templates"
            :key="tpl.id"
            :label="tpl.display_name || tpl.name"
            :value="tpl.id"
          />
        </el-select>
        <el-button
          :disabled="!templateFilter"
          type="danger"
          plain
          @click="deleteSelectedTemplate"
        >
          <el-icon><Delete /></el-icon>
          {{ $t('generated.workbench_delete_template_b31745') }}
        </el-button>

        <div class="toolbar-actions">
          <el-button type="primary" @click="parseDialogVisible = true">
            <el-icon><Upload /></el-icon>
            {{ $t('generated.workbench_file_parsing_53823f') }}
          </el-button>
          <el-button
            type="success"
            @click="openDispatchDialog"
          >
            <el-icon><Select /></el-icon>
            {{ $t('generated.workbench_task_distribution_b43df0') }}
          </el-button>
          <el-button :loading="loading" @click="fetchTemplates">
            <el-icon><Refresh /></el-icon>
            {{ $t('generated.common_refresh_38108e') }}
          </el-button>
        </div>
      </div>

      <div v-if="parsingStatusList.length" class="parse-progress-list">
        <div v-for="item in parsingStatusList" :key="item.templateId" class="parse-progress-item">
          <div class="parse-progress-copy">
            <strong>{{ item.filename }}</strong>
            <span>{{ item.message || parseStageText(item.progress) }} · {{ item.progress }}%</span>
          </div>
          <el-progress :percentage="item.progress" :stroke-width="8" />
        </div>
      </div>
    </el-card>

    <div class="metric-grid">
      <div class="metric-card">
        <div class="metric-label">{{ $t('generated.workbench_template_file_df4718') }}</div>
        <div class="metric-value">{{ templates.length }}</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">{{ $t('generated.workbench_total_number_of_rules_7e7664') }}</div>
        <div class="metric-value">{{ allRules.length }}</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">{{ $t('generated.workbench_selected_rules_2648e2') }}</div>
        <div class="metric-value">{{ selectedRules.length }}</div>
      </div>
      <div class="metric-card">
        <div class="metric-label">{{ $t('generated.workbench_online_hosting_0b16f5') }}</div>
        <div class="metric-value">{{ onlineHostCount }}</div>
      </div>
    </div>

    <el-card class="rules-card">
      <template #header>
        <div class="card-header">
          <div>
            <span class="card-title">{{ $t('generated.workbench_rule_list_764d4c') }}</span>
            <span class="card-subtitle">{{ ruleViewMode === 'file' ? $t('dynamic.parsedRulesFile') : $t('dynamic.parsedRulesPaged') }}</span>
          </div>
          <div class="header-actions">
            <el-segmented
              v-model="ruleViewMode"
              :options="ruleViewOptions"
              size="small"
            />
            <el-button
              :disabled="selectedRules.length === 0"
              :loading="batchGeneratingCheck"
              @click="batchGenerateForSelection('CHECK')"
            >
              <el-icon><MagicStick /></el-icon>
              {{ $t('generated.workbench_generate_detection_script_4d4ca7') }}
            </el-button>
            <el-button
              :disabled="selectedRules.length === 0"
              :loading="batchGeneratingFix"
              @click="batchGenerateForSelection('FIX')"
            >
              <el-icon><Tools /></el-icon>
              {{ $t('generated.workbench_generate_repair_script_16d182') }}
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        v-if="ruleViewMode === 'all'"
        ref="rulesTableRef"
        :data="paginatedRules"
        row-key="id"
        stripe
        class="rules-table"
        v-loading="loading"
        @selection-change="handleVisibleSelectionChange"
      >
        <el-table-column type="selection" width="48" />
        <el-table-column prop="title" :label="$t('generated.common_rule_title_298a16')" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="rule-title-cell">
              <strong>{{ row.title }}</strong>
              <span>{{ row.template_name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.workbench_detection_content_51c307')" min-width="240">
          <template #default="{ row }">
            <el-tooltip :content="row.check_content" placement="top" :disabled="!row.check_content || row.check_content.length <= 42">
              <span class="line-clamp">{{ truncate(row.check_content, 42) }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.workbench_fix_91bff2')" min-width="240">
          <template #default="{ row }">
            <el-tooltip :content="row.fix_content" placement="top" :disabled="!row.fix_content || row.fix_content.length <= 42">
              <span class="line-clamp">{{ truncate(row.fix_content, 42) }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.common_script_status_7dbd2a')" width="190">
          <template #default="{ row }">
            <div class="script-status">
              <el-button
                link
                :type="getScriptButtonType(row.check_script_status)"
                :loading="row.check_script_status === 'generating'"
                @click="openScriptEditor(row, 'CHECK')"
              >
                {{ $t('generated.common_detection_b3ff0c') }}
              </el-button>
              <el-button
                link
                :type="getScriptButtonType(row.fix_script_status)"
                :loading="row.fix_script_status === 'generating'"
                @click="openScriptEditor(row, 'FIX')"
              >
                {{ $t('generated.common_repair_590253') }}
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-container">
        <el-pagination
          v-if="ruleViewMode === 'all'"
          v-model:current-page="ruleCurrentPage"
          :page-size="rulePageSize"
          :total="filteredRules.length"
          layout="total, prev, pager, next"
          background
        />
      </div>

      <div v-if="ruleViewMode === 'file'" class="file-rule-groups" v-loading="loading">
        <section v-for="tpl in filteredTemplateGroups" :key="tpl.id" class="file-rule-group">
          <div class="file-rule-header">
            <div>
              <h3>{{ tpl.display_name || tpl.name }}</h3>
              <span>{{ templateRuleRows(tpl).length }} / {{ tpl.rule_count || 0 }} {{ $t('generated.workbench_rules_84b6d1') }}</span>
            </div>
            <div class="file-rule-actions">
              <el-tag :type="tpl.status === 'completed' ? 'success' : tpl.status === 'failed' ? 'danger' : 'warning'" effect="plain">
                {{ tpl.status === 'completed' ? $t('dynamic.parsed') : tpl.status === 'failed' ? $t('common.status.failed') : $t('dynamic.parsing') }}
              </el-tag>
              <el-button link type="danger" @click="confirmDeleteTemplate(tpl)">{{ $t('generated.common_delete_3755f5') }}</el-button>
            </div>
          </div>

          <div v-if="templateRuleRows(tpl).length" class="file-rule-list">
            <article v-for="rule in templateRulePageRows(tpl)" :key="rule.id" class="file-rule-item">
              <el-checkbox
                :model-value="isRuleSelected(rule.id)"
                @change="checked => toggleRuleSelection(rule.id, Boolean(checked))"
              />
              <div class="file-rule-main">
                <strong>{{ rule.title }}</strong>
                <p>{{ truncate(rule.check_content || rule.fix_content || $t('dynamic.noRuleDescription'), 96) }}</p>
                <div class="file-rule-meta">
                  <el-tag size="small" effect="plain">{{ $t('generated.common_detection_b3ff0c') }} {{ scriptStatusText(rule.check_script_status) }}</el-tag>
                  <el-tag size="small" effect="plain" type="warning">{{ $t('generated.common_repair_590253') }} {{ scriptStatusText(rule.fix_script_status) }}</el-tag>
                </div>
              </div>
              <div class="file-rule-tools">
                <el-button link :type="getScriptButtonType(rule.check_script_status)" @click="openScriptEditor(rule, 'CHECK')">{{ $t('generated.workbench_detection_script_351873') }}</el-button>
                <el-button link :type="getScriptButtonType(rule.fix_script_status)" @click="openScriptEditor(rule, 'FIX')">{{ $t('generated.workbench_fix_script_f411bd') }}</el-button>
              </div>
            </article>
          </div>
          <el-pagination
            v-if="templateRuleRows(tpl).length > fileRulePageSize"
            class="file-rule-pagination"
            :current-page="templateRuleCurrentPage(tpl.id)"
            :page-size="fileRulePageSize"
            :total="templateRuleRows(tpl).length"
            layout="total, prev, pager, next"
            background
            @current-change="page => setTemplateRulePage(tpl.id, page)"
          />
          <el-empty v-if="templateRuleRows(tpl).length === 0" :description="$t('generated.workbench_there_are_currently_no_matching_rules_601e63')" :image-size="72" />
        </section>

        <el-empty v-if="filteredTemplateGroups.length === 0" :description="$t('generated.workbench_no_matching_files_yet_a8df5b')" />
      </div>

      <el-collapse v-if="failedTemplates.length" class="failed-templates">
        <el-collapse-item name="failed">
          <template #title>
            <span class="failed-title">{{ $t('generated.workbench_files_that_failed_to_parse_06e850') }}{{ failedTemplates.length }}）</span>
          </template>
          <div v-for="tpl in failedTemplates" :key="tpl.id" class="failed-item">
            <span class="failed-name">{{ tpl.display_name || tpl.name }}</span>
            <el-button link type="danger" @click="confirmDeleteTemplate(tpl)">{{ $t('generated.common_delete_3755f5') }}</el-button>
          </div>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-dialog v-model="parseDialogVisible" :title="$t('generated.workbench_file_parsing_53823f')" width="560px" :close-on-click-modal="false">
      <el-upload
        :auto-upload="true"
        :show-file-list="false"
        :before-upload="handleBeforeUpload"
        :http-request="handleUpload"
        drag
        class="upload-dragger"
      >
        <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
        <div class="el-upload__text">{{ $t('generated.workbench_drag_your_baseline_document_here_or_0f57b9') }}<em>{{ $t('generated.common_click_to_upload_69aaf1') }}</em></div>
        <template #tip>
          <div class="el-upload__tip">{{ $t('generated.workbench_supports_pdf_word_yaml_formats_maximum_7badca') }}</div>
        </template>
      </el-upload>

      <div v-if="uploading" class="upload-progress">
        <div>
          <strong>{{ activeUploadName }}</strong>
          <span>{{ $t('generated.workbench_uploading_6818eb') }} {{ uploadPercent }}%</span>
        </div>
        <el-progress :percentage="uploadPercent" :stroke-width="8" />
      </div>

      <div v-if="latestParseStatus" class="upload-progress">
        <div>
          <strong>{{ $t('generated.workbench_parsing_progress_50e028') }}</strong>
          <span>{{ latestParseStatus.message || parseStageText(latestParseStatus.progress) }} · {{ latestParseStatus.progress }}%</span>
        </div>
        <el-progress
          :percentage="latestParseStatus.progress"
          :status="latestParseStatus.status === 'failed' ? 'exception' : latestParseStatus.status === 'completed' ? 'success' : undefined"
          :stroke-width="8"
        />
      </div>
    </el-dialog>

    <el-dialog v-model="dispatchDialogVisible" :title="$t('generated.workbench_task_distribution_b43df0')" width="920px" :close-on-click-modal="false">
      <div class="dispatch-layout">
        <section class="dispatch-panel">
          <div class="dispatch-panel-header">
            <h3>{{ $t('generated.common_rule_ed904c') }}</h3>
            <span>{{ selectedRuleIds.length }} / {{ allRules.length }}</span>
          </div>
          <el-input v-model="dispatchRuleSearch" :placeholder="$t('generated.workbench_search_rules_64a8cd')" clearable>
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-checkbox-group v-model="selectedRuleIds" class="dispatch-list">
            <el-checkbox
              v-for="rule in paginatedDispatchRules"
              :key="rule.id"
              :label="rule.id"
              class="dispatch-check"
            >
              <strong :title="rule.title">{{ rule.title }}</strong>
            </el-checkbox>
          </el-checkbox-group>
          <el-pagination
            v-if="dispatchFilteredRules.length > dispatchRulePageSize"
            v-model:current-page="dispatchRulePage"
            :page-size="dispatchRulePageSize"
            :total="dispatchFilteredRules.length"
            layout="total, prev, pager, next"
            background
            small
            style="margin-top: 8px; justify-content: center;"
          />
        </section>

        <section class="dispatch-panel">
          <div class="dispatch-panel-header">
            <h3>{{ $t('generated.common_host_2e8a0c') }}</h3>
            <span>{{ selectedHostIds.length }} / {{ hosts.length }}</span>
          </div>
          <el-input v-model="hostSearch" :placeholder="$t('generated.common_search_for_hostname_or_ip_565f3f')" clearable>
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
          <el-checkbox-group v-model="selectedHostIds" class="dispatch-list">
            <el-checkbox
              v-for="host in filteredHosts"
              :key="host.id"
              :label="host.id"
              class="dispatch-check host-check"
            >
              <strong>{{ host.hostname }}</strong>
              <span>{{ host.ip_address }}</span>
              <el-tag size="small" :type="host.online ? 'success' : 'danger'" effect="plain">
                {{ host.online ? $t('common.status.online') : $t('common.status.offline') }}
              </el-tag>
            </el-checkbox>
          </el-checkbox-group>
        </section>
      </div>

      <div class="dispatch-options">
        <el-form label-width="110px">
          <el-form-item :label="$t('generated.common_maximum_number_of_rounds_7b621d')">
            <el-input-number v-model="dispatchMaxRounds" :min="1" :max="10" />
          </el-form-item>
          <el-form-item :label="$t('generated.common_automatic_verification_30b2d5')">
            <el-switch v-model="dispatchAutoVerify" />
            <span style="font-size: 12px; color: #666; margin-left: 8px;">
              {{ $t('generated.workbench_automatically_repair_and_re_test_when_bb0e32') }}
            </span>
          </el-form-item>
        </el-form>
        <div class="dispatch-warning">
          <el-alert v-if="checkScriptWarning" :title="checkScriptWarning" type="warning" :closable="false" show-icon />
          <el-alert v-if="fixScriptWarning" :title="fixScriptWarning" type="warning" :closable="false" show-icon />
        </div>
      </div>

      <template #footer>
        <el-button @click="dispatchDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button
          type="primary"
          :disabled="!canDispatchCheck"
          :loading="executing"
          @click="executeTask('CHECK')"
        >
          <el-icon><VideoPlay /></el-icon>
          {{ $t('generated.workbench_issue_detection_6bb263') }}
        </el-button>
        <el-button
          type="warning"
          :disabled="!canDispatchFix"
          :loading="executing"
          @click="executeTask('FIX')"
        >
          <el-icon><Tools /></el-icon>
          {{ $t('generated.workbench_issue_repair_40f204') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="scriptDialogVisible"
      :title="scriptDialogTitle"
      width="76%"
      :close-on-click-modal="false"
      class="script-dialog"
      @closed="destroyScriptEditor"
    >
      <div v-if="scriptLoading" class="loading-container">
        <el-icon class="is-loading" :size="32"><Loading /></el-icon>
        <span>{{ $t('generated.workbench_requesting_ai_generation_script_eefff6') }}</span>
      </div>
      <template v-else>
        <div class="script-toolbar">
          <el-tabs v-model="scriptActiveTab" class="script-tabs">
            <el-tab-pane :label="$t('generated.workbench_editor_86362a')" name="editor" />
            <el-tab-pane label="Diff" name="diff" />
          </el-tabs>
          <div class="script-toolbar-actions">
            <el-tag v-if="scriptReadOnly" type="info" effect="plain">{{ $t('generated.workbench_read_only_ffc1d0') }}</el-tag>
            <el-button v-if="scriptReadOnly" type="primary" plain @click="enableScriptEditing">
              <el-icon><Edit /></el-icon>
              {{ $t('generated.common_edit_a7f814') }}
            </el-button>
          </div>
        </div>

        <div v-show="scriptActiveTab === 'editor'" ref="scriptEditorEl" class="codemirror-shell" />
        <div v-show="scriptActiveTab === 'diff'" class="diff-view">
          <div v-for="(row, index) in scriptDiffRows" :key="index" class="diff-row" :class="row.type">
            <span class="diff-line">{{ index + 1 }}</span>
            <pre>{{ row.text }}</pre>
          </div>
        </div>
      </template>

      <template #footer>
        <el-button @click="scriptDialogVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button
          type="primary"
          @click="saveScript"
          :loading="scriptSaving"
          :disabled="scriptLoading || scriptReadOnly || !scriptContent"
        >
          {{ $t('generated.workbench_save_fadf24') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'

import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { UploadRequestOptions } from 'element-plus'
import {
  Delete,
  Edit,
  Loading,
  MagicStick,
  Refresh,
  Search,
  Select,
  Tools,
  Upload,
  UploadFilled,
  VideoPlay
} from '@element-plus/icons-vue'
import SparkMD5 from 'spark-md5'
import { EditorState, Compartment } from '@codemirror/state'
import { EditorView, drawSelection, highlightActiveLine, keymap, lineNumbers } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { defaultHighlightStyle, StreamLanguage, syntaxHighlighting } from '@codemirror/language'
import { shell } from '@codemirror/legacy-modes/mode/shell'
import {
  batchGenerateScripts as batchGenerateScriptsApi,
  checkFileMD5,
  deleteTemplate,
  generateScript,
  getTemplateRules,
  getTemplates,
  getTemplateStatus,
  updateScript,
  uploadTemplate
} from '@/api/templates'
import { useHostStore } from '@/store/hosts'
import { useTaskStore } from '@/store/tasks'
import type { BaselineRule, ParseStatus, Template } from '@/types'
import { paginate } from '@/utils/paginate'
import { compareRulesByScriptStatus } from '@/utils/ruleSort'

type RuleRow = BaselineRule & {
  template_name: string
  template_status: Template['status']
}

type DiffRow = {
  type: 'same' | 'added' | 'removed' | 'changed'
  text: string
}

const router = useRouter()
const hostStore = useHostStore()
const taskStore = useTaskStore()

const loading = ref(false)
const executing = ref(false)
const templates = ref<Template[]>([])
const templateRulesMap = reactive<Record<string, BaselineRule[]>>({})
const templateStatusMap = reactive<Record<string, ParseStatus>>({})
const pollingTimers = new Map<string, number>()

const ruleSearch = ref('')
const templateFilter = ref('')
const ruleViewMode = ref<'file' | 'all'>('file')
const ruleViewOptions = computed(() => [
  { label: translate('generatedScript.workbench_file_perspective_d6ec8d'), value: 'file' },
  { label: translate('generatedScript.workbench_all_perspectives_89088b'), value: 'all' }
])
const ruleCurrentPage = ref(1)
const rulePageSize = 10
const rulesTableRef = ref<any>(null)

// 文件视角下，每个规则集合（模板）独立维护当前页码，互不干扰
const fileRulePageSize = 10
const templateRulePageMap = reactive<Record<string, number>>({})

function templateRuleCurrentPage(tplId: string): number {
  return templateRulePageMap[tplId] || 1
}

function setTemplateRulePage(tplId: string, page: number) {
  templateRulePageMap[tplId] = page
}
const syncingTableSelection = ref(false)
const selectedRuleIds = ref<string[]>([])
const selectedHostIds = ref<string[]>([])
const hostSearch = ref('')
const dispatchRuleSearch = ref('')
const dispatchRulePage = ref(1)
const dispatchRulePageSize = 10
const dispatchDialogVisible = ref(false)
const dispatchMaxRounds = ref(1)
const dispatchAutoVerify = ref(false)

const parseDialogVisible = ref(false)
const uploading = ref(false)
const uploadPercent = ref(0)
const activeUploadName = ref('')
const latestParseTemplateId = ref('')

const scriptDialogVisible = ref(false)
const scriptDialogTitle = ref('')
const scriptActiveTab = ref<'editor' | 'diff'>('editor')
const scriptEditorEl = ref<HTMLElement | null>(null)
const scriptContent = ref('')
const originalScriptContent = ref('')
const scriptReadOnly = ref(true)
const scriptLoading = ref(false)
const scriptSaving = ref(false)
const currentEditRule = ref<RuleRow | null>(null)
const currentScriptType = ref<'CHECK' | 'FIX'>('CHECK')
let scriptEditorView: EditorView | null = null
const editorReadOnly = new Compartment()

const batchGeneratingMap = reactive<Record<string, boolean>>({})

const hosts = computed(() => hostStore.hosts)
const onlineHostCount = computed(() => hosts.value.filter(host => host.online).length)

const allRules = computed<RuleRow[]>(() => {
  return templates.value.flatMap(tpl => {
    // 解析失败的模板不进入可用规则集合
    if (tpl.status === 'failed') return []
    const templateName = tpl.display_name || tpl.name
    return (templateRulesMap[tpl.id] || []).map(rule => ({
      ...rule,
      template_name: templateName,
      template_status: tpl.status
    }))
  })
  // 按脚本就绪度排序：已生成 > 生成中 > 未生成/失败
  .sort(compareRulesByScriptStatus)
})

const filteredRules = computed(() => {
  const keyword = ruleSearch.value.trim().toLowerCase()
  return allRules.value.filter(rule => {
    if (templateFilter.value && rule.template_id !== templateFilter.value) return false
    if (!keyword) return true
    return [
      rule.title,
      rule.check_content,
      rule.fix_content,
      rule.template_name
    ].some(value => String(value || '').toLowerCase().includes(keyword))
  })
})

const filteredTemplateGroups = computed(() => {
  return templates.value.filter(tpl => {
    // 解析失败的文件不应出现在基线工作区
    if (tpl.status === 'failed') return false
    if (templateFilter.value && tpl.id !== templateFilter.value) return false
    const rows = templateRuleRows(tpl)
    if (!ruleSearch.value.trim()) return true
    return rows.length > 0 || [tpl.display_name, tpl.name].some(value =>
      String(value || '').toLowerCase().includes(ruleSearch.value.trim().toLowerCase())
    )
  })
})

// 解析失败的文件单独归并，供用户清理（不在工作区展示）
const failedTemplates = computed(() => {
  return templates.value.filter(tpl => tpl.status === 'failed')
})

const paginatedRules = computed(() => {
  const start = (ruleCurrentPage.value - 1) * rulePageSize
  return filteredRules.value.slice(start, start + rulePageSize)
})

const selectedRules = computed(() => {
  const selected = new Set(selectedRuleIds.value)
  return allRules.value.filter(rule => selected.has(rule.id))
})

const dispatchFilteredRules = computed(() => {
  const keyword = dispatchRuleSearch.value.trim().toLowerCase()
  if (!keyword) return allRules.value
  return allRules.value.filter(rule =>
    [rule.title, rule.check_content, rule.fix_content, rule.template_name]
      .some(value => String(value || '').toLowerCase().includes(keyword))
  )
})

const paginatedDispatchRules = computed(() => {
  const start = (dispatchRulePage.value - 1) * dispatchRulePageSize
  return dispatchFilteredRules.value.slice(start, start + dispatchRulePageSize)
})

const filteredHosts = computed(() => {
  const keyword = hostSearch.value.trim().toLowerCase()
  if (!keyword) return hosts.value
  return hosts.value.filter(host =>
    [host.hostname, host.ip_address, host.os_type]
      .some(value => String(value || '').toLowerCase().includes(keyword))
  )
})

const checkScriptStatus = computed(() => getScriptStatusSummary(selectedRules.value, 'CHECK'))
const fixScriptStatus = computed(() => getScriptStatusSummary(selectedRules.value, 'FIX'))
const checkScriptWarning = computed(() => scriptWarningText(checkScriptStatus.value, translate('generatedScript.common_detection_b3ff0c')))
const fixScriptWarning = computed(() => scriptWarningText(fixScriptStatus.value, translate('generatedScript.common_repair_590253')))
const canDispatchCheck = computed(() => selectedRuleIds.value.length > 0 && selectedHostIds.value.length > 0 && checkScriptStatus.value.ready)
const canDispatchFix = computed(() => selectedRuleIds.value.length > 0 && selectedHostIds.value.length > 0 && fixScriptStatus.value.ready)
const batchGeneratingCheck = computed(() => Object.entries(batchGeneratingMap).some(([key, value]) => key.endsWith('-CHECK') && value))
const batchGeneratingFix = computed(() => Object.entries(batchGeneratingMap).some(([key, value]) => key.endsWith('-FIX') && value))

const parsingStatusList = computed(() => {
  return templates.value
    .filter(tpl => tpl.status === 'parsing' || templateStatusMap[tpl.id]?.status === 'parsing')
    .map(tpl => {
      const status = templateStatusMap[tpl.id]
      return {
        templateId: tpl.id,
        filename: tpl.display_name || tpl.name,
        progress: Math.min(100, Math.max(0, status?.progress ?? 0)),
        message: status?.message || ''
      }
    })
})

const latestParseStatus = computed(() => {
  if (!latestParseTemplateId.value) return null
  return templateStatusMap[latestParseTemplateId.value] || null
})

const scriptDiffRows = computed<DiffRow[]>(() => {
  const before = originalScriptContent.value.split('\n')
  const after = scriptContent.value.split('\n')
  const max = Math.max(before.length, after.length)
  const rows: DiffRow[] = []
  for (let i = 0; i < max; i++) {
    const oldLine = before[i]
    const newLine = after[i]
    if (oldLine === newLine) {
      rows.push({ type: 'same', text: newLine ?? '' })
    } else if (oldLine === undefined) {
      rows.push({ type: 'added', text: `+ ${newLine}` })
    } else if (newLine === undefined) {
      rows.push({ type: 'removed', text: `- ${oldLine}` })
    } else {
      rows.push({ type: 'changed', text: `- ${oldLine}\n+ ${newLine}` })
    }
  }
  return rows
})

watch([paginatedRules, selectedRuleIds], () => {
  nextTick(syncVisibleTableSelection)
})

watch([ruleSearch, templateFilter], () => {
  ruleCurrentPage.value = 1
  // 搜索 / 模板筛选变化时，文件视角各规则集合回到第 1 页
  for (const key of Object.keys(templateRulePageMap)) {
    delete templateRulePageMap[key]
  }
})

watch(dispatchRuleSearch, () => {
  dispatchRulePage.value = 1
})

watch([scriptDialogVisible, scriptActiveTab], () => {
  if (scriptDialogVisible.value && scriptActiveTab.value === 'editor') {
    nextTick(mountScriptEditor)
  }
})

function getScriptStatusSummary(rules: RuleRow[], scriptType: 'CHECK' | 'FIX') {
  let pending = 0
  let generating = 0
  rules.forEach(rule => {
    const status = scriptType === 'CHECK' ? rule.check_script_status : rule.fix_script_status
    if (status === 'generating') generating += 1
    else if (status !== 'generated') pending += 1
  })
  return { ready: pending === 0 && generating === 0, pending, generating }
}

function templateRuleRows(tpl: Template): RuleRow[] {
  const keyword = ruleSearch.value.trim().toLowerCase()
  return (templateRulesMap[tpl.id] || []).map(rule => ({
    ...rule,
    template_name: tpl.display_name || tpl.name,
    template_status: tpl.status
  })).filter(rule => {
    if (!keyword) return true
    return [rule.title, rule.check_content, rule.fix_content, rule.template_name]
      .some(value => String(value || '').toLowerCase().includes(keyword))
  })
    // 按脚本就绪度排序：已生成 > 生成中 > 未生成/失败
    .sort(compareRulesByScriptStatus)
}

// 文件视角下，按模板独立分页（每页 fileRulePageSize 条）
function templateRulePageRows(tpl: Template): RuleRow[] {
  return paginate(templateRuleRows(tpl), templateRuleCurrentPage(tpl.id), fileRulePageSize)
}

function isRuleSelected(ruleId: string) {
  return selectedRuleIds.value.includes(ruleId)
}

function toggleRuleSelection(ruleId: string, checked: boolean) {
  if (checked) {
    if (!selectedRuleIds.value.includes(ruleId)) {
      selectedRuleIds.value = [...selectedRuleIds.value, ruleId]
    }
    return
  }
  selectedRuleIds.value = selectedRuleIds.value.filter(id => id !== ruleId)
}

function scriptStatusText(status: string) {
  switch (status) {
    case 'generated': return translate('generatedScript.common_generated_79c74f')
    case 'generating': return translate('generatedScript.common_generating_57c08c')
    case 'failed': return translate('generatedScript.common_fail_3e3c80')
    default: return translate('generatedScript.common_not_generated_3c04f9')
  }
}

function scriptWarningText(status: { ready: boolean; pending: number; generating: number }, label: string) {
  if (status.ready) return ''
  if (status.generating > 0) return translate('generatedScript.workbench_scripts_are_being_generated_0af6d6', { p0: status.generating, p1: label })
  if (status.pending > 0) return translate('generatedScript.workbench_scripts_not_generated_e9d1be', { p0: status.pending, p1: label })
  return ''
}

function normalizeRuleScriptStatus(rule: BaselineRule) {
  if (!rule.check_script_status || String(rule.check_script_status) === 'ready') {
    rule.check_script_status = rule.generated_check_script ? 'generated' : 'pending'
  }
  if (!rule.fix_script_status || String(rule.fix_script_status) === 'ready') {
    rule.fix_script_status = rule.generated_fix_script ? 'generated' : 'pending'
  }
}

async function fetchTemplates() {
  loading.value = true
  try {
    templates.value = await getTemplates()
    await Promise.all(templates.value.map(async tpl => {
      if (tpl.status === 'completed' && tpl.rule_count > 0) {
        await loadTemplateRules(tpl.id)
      } else if (tpl.status === 'parsing') {
        startTemplatePolling(tpl.id)
      }
    }))
  } finally {
    loading.value = false
  }
}

async function loadTemplateRules(templateId: string) {
  const rules = await getTemplateRules(templateId)
  rules.forEach(normalizeRuleScriptStatus)
  templateRulesMap[templateId] = rules
}

function startTemplatePolling(templateId: string) {
  if (pollingTimers.has(templateId)) return
  pollTemplateStatus(templateId)
  const timer = window.setInterval(() => pollTemplateStatus(templateId), 2000)
  pollingTimers.set(templateId, timer)
}

function stopTemplatePolling(templateId: string) {
  const timer = pollingTimers.get(templateId)
  if (timer) {
    window.clearInterval(timer)
    pollingTimers.delete(templateId)
  }
}

async function pollTemplateStatus(templateId: string) {
  try {
    const status = await getTemplateStatus(templateId)
    templateStatusMap[templateId] = {
      ...status,
      progress: Math.min(100, Math.max(0, Number(status.progress || 0)))
    }
    const target = templates.value.find(tpl => tpl.id === templateId)
    if (target) {
      target.status = status.status
      if (status.status === 'failed') target.error_message = status.message
    }
    if (status.status === 'completed') {
      stopTemplatePolling(templateId)
      await fetchTemplates()
      await loadTemplateRules(templateId)
      ElMessage.success(translate('generatedScript.workbench_template_parsing_completed_6f493d'))
    } else if (status.status === 'failed') {
      stopTemplatePolling(templateId)
      ElMessage.error(status.message || translate('generatedScript.workbench_template_parsing_failed_62a125'))
    }
  } catch (e) {
    console.error('failed to poll template status', e)
  }
}

function parseStageText(progress: number) {
  if (progress < 40) return translate('generatedScript.workbench_reading_template_content_206638')
  if (progress < 60) return translate('generatedScript.workbench_calling_llm_extraction_rules_50ab5a')
  if (progress < 80) return translate('generatedScript.workbench_structuring_check_items_04ad91')
  if (progress < 100) return translate('generatedScript.workbench_writing_to_the_rule_base_eff437')
  return translate('generatedScript.workbench_parsing_completed_65eddd')
}

function handleBeforeUpload(file: File) {
  if (file.size > 5 * 1024 * 1024) {
    ElMessage.error(translate('generatedScript.workbench_file_size_exceeds_5mb_and_cannot_e45f64'))
    return false
  }
  const ext = file.name.split('.').pop()?.toLowerCase()
  if (!['pdf', 'docx', 'doc', 'yaml', 'yml'].includes(ext || '')) {
    ElMessage.error(translate('generatedScript.workbench_only_supports_files_in_pdf_word_e50bfc'))
    return false
  }
  return true
}

function calculateMD5(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunkSize = 2097152
    const chunks = Math.ceil(file.size / chunkSize)
    let currentChunk = 0
    const spark = new SparkMD5.ArrayBuffer()
    const fileReader = new FileReader()

    fileReader.onload = e => {
      spark.append(e.target?.result as ArrayBuffer)
      currentChunk += 1
      if (currentChunk < chunks) loadNext()
      else resolve(spark.end())
    }
    fileReader.onerror = () => reject(new Error(translate('generatedScript.workbench_file_read_failed_ea2e2f')))

    const loadNext = () => {
      const start = currentChunk * chunkSize
      const end = Math.min(start + chunkSize, file.size)
      fileReader.readAsArrayBuffer(file.slice(start, end))
    }
    loadNext()
  })
}

async function handleUpload(options: UploadRequestOptions) {
  const file = options.file as File
  uploading.value = true
  uploadPercent.value = 0
  activeUploadName.value = file.name
  latestParseTemplateId.value = ''

  try {
    const md5 = await calculateMD5(file)
    const checkResult = await checkFileMD5(md5)
    if (checkResult.exists) {
      await ElMessageBox.confirm(
        translate('generatedScript.workbench_the_file_has_been_parsed_do_bfd01b', { p0: checkResult.filename }),
        translate('generatedScript.workbench_file_already_exists_ec9089'),
        { confirmButtonText: translate('generatedScript.workbench_continue_to_upload_ade9a3'), cancelButtonText: translate('generatedScript.common_cancel_4d0b46'), type: 'info' }
      )
    }

    const result = await uploadTemplate(file, md5, progressEvent => {
      if (!progressEvent.total) return
      uploadPercent.value = Math.min(95, Math.round((progressEvent.loaded / progressEvent.total) * 100))
    })
    uploadPercent.value = 100

    if (result.exists) {
      ElMessage.info(translate('generatedScript.workbench_file_already_exists_skip_parsing_ae34e6'))
      return
    }

    latestParseTemplateId.value = result.template_id
    templateStatusMap[result.template_id] = {
      status: 'parsing',
      progress: 20,
      message: translate('generatedScript.workbench_file_upload_completed_waiting_for_parsing_26de7c')
    }
    ElMessage.success(translate('generatedScript.workbench_uploaded_successfully_parsing_is_in_progress_ae0b16'))
    await fetchTemplates()
    startTemplatePolling(result.template_id)
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message || translate('generatedScript.common_upload_failed_a6f805'))
  } finally {
    uploading.value = false
  }
}

function handleVisibleSelectionChange(selection: RuleRow[]) {
  if (syncingTableSelection.value) return
  const visibleIds = new Set(paginatedRules.value.map(rule => rule.id))
  const nextIds = selectedRuleIds.value.filter(id => !visibleIds.has(id))
  selection.forEach(rule => {
    if (!nextIds.includes(rule.id)) nextIds.push(rule.id)
  })
  selectedRuleIds.value = nextIds
}

function syncVisibleTableSelection() {
  if (!rulesTableRef.value) return
  syncingTableSelection.value = true
  rulesTableRef.value.clearSelection()
  const selected = new Set(selectedRuleIds.value)
  paginatedRules.value.forEach(rule => {
    if (selected.has(rule.id)) {
      rulesTableRef.value.toggleRowSelection(rule, true)
    }
  })
  syncingTableSelection.value = false
}

function truncate(text: string, length: number) {
  if (!text) return ''
  return text.length > length ? `${text.slice(0, length)}...` : text
}

function getScriptButtonType(status: string) {
  switch (status) {
    case 'generated': return 'success'
    case 'generating': return 'info'
    case 'failed': return 'danger'
    default: return 'primary'
  }
}

async function openScriptEditor(rule: RuleRow, scriptType: 'CHECK' | 'FIX') {
  const statusField = scriptType === 'CHECK' ? 'check_script_status' : 'fix_script_status'
  if (rule[statusField] === 'generating') {
    ElMessage.info(translate('generatedScript.workbench_the_script_is_being_generated_please_6ccfbe'))
    return
  }

  currentEditRule.value = rule
  currentScriptType.value = scriptType
  scriptDialogTitle.value = `${scriptType === 'CHECK' ? translate('generatedScript.workbench_detection_script_351873') : translate('generatedScript.common_fix_script_f411bd')} - ${rule.title}`
  scriptActiveTab.value = 'editor'
  scriptReadOnly.value = true

  const existingScript = scriptType === 'CHECK' ? rule.generated_check_script : rule.generated_fix_script
  if (existingScript) {
    originalScriptContent.value = existingScript
    scriptContent.value = existingScript
    scriptLoading.value = false
    scriptDialogVisible.value = true
    await nextTick()
    mountScriptEditor()
    return
  }

  rule[statusField] = 'generating'
  scriptLoading.value = true
  scriptContent.value = ''
  originalScriptContent.value = ''
  scriptDialogVisible.value = true
  try {
    const result = await generateScript(rule.id, scriptType)
    if (result.script_content) {
      scriptContent.value = result.script_content
      originalScriptContent.value = result.script_content
      rule[statusField] = 'generated'
      if (scriptType === 'CHECK') rule.generated_check_script = result.script_content
      else rule.generated_fix_script = result.script_content
      await nextTick()
      mountScriptEditor()
    } else {
      ElMessage.info(translate('generatedScript.workbench_the_script_generation_task_has_been_6b8e75'))
      scriptDialogVisible.value = false
    }
  } catch (e: any) {
    rule[statusField] = 'failed'
    ElMessage.error(e.message || translate('generatedScript.workbench_build_script_failed_e681f1'))
    scriptDialogVisible.value = false
  } finally {
    scriptLoading.value = false
  }
}

function mountScriptEditor() {
  if (!scriptEditorEl.value || scriptEditorView) return
  const state = EditorState.create({
    doc: scriptContent.value,
    extensions: [
      lineNumbers(),
      history(),
      drawSelection(),
      highlightActiveLine(),
      StreamLanguage.define(shell),
      syntaxHighlighting(defaultHighlightStyle),
      keymap.of([...defaultKeymap, ...historyKeymap]),
      editorReadOnly.of([
        EditorState.readOnly.of(scriptReadOnly.value),
        EditorView.editable.of(!scriptReadOnly.value)
      ]),
      EditorView.lineWrapping,
      EditorView.updateListener.of(update => {
        if (update.docChanged) {
          scriptContent.value = update.state.doc.toString()
        }
      }),
      EditorView.theme({
        '&': {
          minHeight: '420px',
          border: '1px solid #dbe3ef',
          borderRadius: '8px',
          fontSize: '13px'
        },
        '.cm-scroller': {
          fontFamily: 'var(--aegis-font-mono)',
          minHeight: '420px'
        },
        '.cm-gutters': {
          backgroundColor: '#f8fafc',
          borderRight: '1px solid #e2e8f0'
        }
      })
    ]
  })
  scriptEditorView = new EditorView({ state, parent: scriptEditorEl.value })
}

function destroyScriptEditor() {
  scriptEditorView?.destroy()
  scriptEditorView = null
}

function enableScriptEditing() {
  scriptReadOnly.value = false
  scriptEditorView?.dispatch({
    effects: editorReadOnly.reconfigure([
      EditorState.readOnly.of(false),
      EditorView.editable.of(true)
    ])
  })
}

async function saveScript() {
  if (!currentEditRule.value || !scriptContent.value) return
  scriptSaving.value = true
  try {
    await updateScript(currentEditRule.value.id, currentScriptType.value, scriptContent.value)
    if (currentScriptType.value === 'CHECK') {
      currentEditRule.value.generated_check_script = scriptContent.value
      currentEditRule.value.check_script_status = 'generated'
    } else {
      currentEditRule.value.generated_fix_script = scriptContent.value
      currentEditRule.value.fix_script_status = 'generated'
    }
    originalScriptContent.value = scriptContent.value
    scriptReadOnly.value = true
    ElMessage.success(translate('generatedScript.workbench_script_saved_successfully_efd963'))
    scriptDialogVisible.value = false
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_save_failed_40525a'))
  } finally {
    scriptSaving.value = false
  }
}

async function batchGenerateForSelection(scriptType: 'CHECK' | 'FIX') {
  const templateIds = [...new Set(selectedRules.value.map(rule => rule.template_id))]
  await Promise.all(templateIds.map(templateId => batchGenerateScripts(templateId, scriptType)))
}

async function batchGenerateScripts(templateId: string, scriptType: 'CHECK' | 'FIX') {
  const key = `${templateId}-${scriptType}`
  batchGeneratingMap[key] = true
  try {
    const result = await batchGenerateScriptsApi(templateId, scriptType)
    if (result.queued > 0) {
      ElMessage.success(translate('generatedScript.workbench_script_generation_tasks_submitted_eaf734', { p0: result.queued }))
      const rules = templateRulesMap[templateId] || []
      rules.forEach(rule => {
        const statusField = scriptType === 'CHECK' ? 'check_script_status' : 'fix_script_status'
        const hasScript = scriptType === 'CHECK' ? rule.generated_check_script : rule.generated_fix_script
        if (!hasScript) rule[statusField] = 'generating'
      })
    } else {
      ElMessage.info(translate('generatedScript.workbench_the_script_has_been_generated_or_ba7f3e'))
    }
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.workbench_batch_generation_failed_6fbd54'))
  } finally {
    batchGeneratingMap[key] = false
  }
}

function openDispatchDialog() {
  dispatchDialogVisible.value = true
}

async function executeTask(type: 'CHECK' | 'FIX') {
  if (!selectedRules.value.length || !selectedHostIds.value.length) {
    ElMessage.warning(translate('generatedScript.workbench_please_select_a_rule_and_host_198b1d'))
    return
  }
  executing.value = true
  try {
    taskStore.setSelectedRules(selectedRules.value)
    taskStore.setSelectedHosts(selectedHostIds.value)
    const result = type === 'CHECK'
      ? await taskStore.executeCheck(dispatchMaxRounds.value, dispatchAutoVerify.value)
      : await taskStore.executeFix(dispatchMaxRounds.value, dispatchAutoVerify.value)
    if (result) {
      dispatchDialogVisible.value = false
      await ElMessageBox.confirm(
        translate('generatedScript.workbench_task_has_been_issued_task_group_630cf3', { p0: type === 'CHECK' ? translate('generatedScript.common_detection_b3ff0c') : translate('generatedScript.common_repair_590253'), p1: result.task_group_id }),
        translate('generatedScript.common_task_has_been_created_27a0d5'),
        { confirmButtonText: translate('generatedScript.workbench_view_tasks_ec20be'), cancelButtonText: translate('generatedScript.workbench_closure_6c14bd'), type: 'success' }
      ).then(() => {
        router.push(`/baseline/tasks/${result.task_group_id}`)
      }).catch(() => {})
    }
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.workbench_delivery_failed_2d0918'))
  } finally {
    executing.value = false
  }
}

async function confirmDeleteTemplate(tpl: Template) {
  try {
    await ElMessageBox.confirm(
      translate('generatedScript.workbench_are_you_sure_you_want_to_290344', { p0: tpl.display_name || tpl.name, p1: tpl.rule_count }),
      translate('generatedScript.common_confirm_deletion_3c06ab'),
      { confirmButtonText: translate('generatedScript.common_delete_3755f5'), cancelButtonText: translate('generatedScript.common_cancel_4d0b46'), type: 'warning' }
    )
    await deleteTemplate(tpl.id)
    delete templateRulesMap[tpl.id]
    delete templateStatusMap[tpl.id]
    stopTemplatePolling(tpl.id)
    ElMessage.success(translate('generatedScript.common_delete_successfully_86e8d1'))
    await fetchTemplates()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message || translate('generatedScript.common_delete_failed_72250c'))
  }
}

function deleteSelectedTemplate() {
  const tpl = templates.value.find(item => item.id === templateFilter.value)
  if (tpl) confirmDeleteTemplate(tpl)
}

onMounted(async () => {
  await hostStore.fetchHosts()
  await fetchTemplates()
})

onBeforeUnmount(() => {
  pollingTimers.forEach(timer => window.clearInterval(timer))
  pollingTimers.clear()
  destroyScriptEditor()
})
</script>

<style scoped>
.workbench-hero {
  margin-bottom: 0;
}

.failed-templates {
  margin-top: 16px;
}

.failed-title {
  color: var(--el-color-danger);
  font-weight: 600;
}

.failed-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 4px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.failed-item:last-child {
  border-bottom: none;
}

.failed-name {
  color: var(--el-text-color-regular);
}

.toolbar-card :deep(.el-card__body) {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.toolbar-row,
.card-header,
.header-actions,
.script-toolbar,
.script-toolbar-actions {
  display: flex;
  align-items: center;
}

.toolbar-row,
.card-header {
  justify-content: space-between;
  gap: 14px;
}

.rule-search {
  max-width: 420px;
}

.template-filter {
  width: 240px;
}

.toolbar-actions,
.header-actions,
.script-toolbar-actions {
  gap: 10px;
}

.card-title {
  display: block;
  font-size: 16px;
  font-weight: 700;
}

.card-subtitle {
  display: block;
  margin-top: 4px;
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.parse-progress-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 12px;
}

.parse-progress-item,
.upload-progress {
  padding: 12px;
  border: 1px solid rgba(37, 99, 235, 0.14);
  border-radius: 8px;
  background: #f8fbff;
}

.parse-progress-copy,
.upload-progress > div {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.parse-progress-copy strong,
.upload-progress strong {
  max-width: 240px;
  overflow: hidden;
  color: var(--aegis-text);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rules-table {
  width: 100%;
}

.rule-title-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.rule-title-cell span {
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.line-clamp {
  color: #475569;
  font-size: 13px;
}

.script-status {
  display: flex;
  gap: 8px;
}

.pagination-container {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.file-rule-groups {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.file-rule-group {
  border: 1px solid var(--aegis-border);
  border-radius: 8px;
  background: #fff;
}

.file-rule-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--aegis-border);
  background: #f8fafc;
}

.file-rule-header h3 {
  margin: 0 0 4px;
  font-size: 15px;
}

.file-rule-header span {
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.file-rule-actions,
.file-rule-tools,
.file-rule-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.file-rule-list {
  display: flex;
  flex-direction: column;
}

.file-rule-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 12px 16px 4px;
}

.file-rule-item {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) auto;
  gap: 12px;
  align-items: start;
  padding: 12px 16px;
  border-bottom: 1px solid #eef2f7;
}

.file-rule-item:last-child {
  border-bottom: 0;
}

.file-rule-main {
  min-width: 0;
}

.file-rule-main strong {
  display: block;
  overflow: hidden;
  color: var(--aegis-text);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-rule-main p {
  margin: 4px 0 8px;
  color: #64748b;
  font-size: 13px;
  line-height: 1.5;
}

.file-rule-tools {
  justify-content: flex-end;
  flex-wrap: wrap;
}

.upload-dragger {
  margin-bottom: 16px;
}

.dispatch-layout {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  padding: 16px;
  background: #f8f9fb;
  border: 1px solid var(--aegis-border);
  border-radius: 8px;
  overflow: hidden;
  box-sizing: border-box;
}

.dispatch-panel {
  padding: 14px;
  border: 1px solid var(--aegis-border);
  border-radius: 8px;
  background: #fff;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.dispatch-panel-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: 12px;
}

.dispatch-panel h3 {
  margin: 0;
  font-size: 15px;
}

.dispatch-panel-header span {
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.dispatch-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
  min-height: 0;
  margin-top: 12px;
  overflow-y: auto;
}

.dispatch-check {
  width: 100%;
  min-height: 44px;
  margin-right: 0;
  padding: 8px 10px;
  border-radius: 8px;
  background: #f8fafc;
}

.dispatch-check :deep(.el-checkbox__label) {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.dispatch-check strong {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dispatch-check span {
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.host-check strong {
  max-width: 150px;
}

.dispatch-options {
  display: grid;
  grid-template-columns: 220px 1fr;
  gap: 16px;
  margin-top: 16px;
}

.dispatch-warning {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 300px;
  gap: 12px;
  color: var(--aegis-action-blue);
}

.script-toolbar {
  justify-content: space-between;
  margin-bottom: 12px;
}

.script-tabs {
  flex: 1;
}

.codemirror-shell {
  min-height: 420px;
}

.diff-view {
  max-height: 460px;
  overflow: auto;
  border: 1px solid #dbe3ef;
  border-radius: 8px;
  background: #0f172a;
}

.diff-row {
  display: grid;
  grid-template-columns: 54px 1fr;
  border-bottom: 1px solid rgba(148, 163, 184, 0.14);
}

.diff-row pre {
  margin: 0;
  padding: 6px 10px;
  color: #dbeafe;
  font-family: var(--aegis-font-mono);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
}

.diff-line {
  padding: 6px 8px;
  color: #94a3b8;
  text-align: right;
  background: rgba(15, 23, 42, 0.75);
  font-family: var(--aegis-font-mono);
  font-size: 12px;
}

.diff-row.added pre {
  color: #bbf7d0;
  background: rgba(22, 101, 52, 0.24);
}

.diff-row.removed pre,
.diff-row.changed pre {
  color: #fecaca;
  background: rgba(153, 27, 27, 0.22);
}

@media (max-width: 900px) {
  .toolbar-row,
  .dispatch-layout,
  .dispatch-options {
    grid-template-columns: 1fr;
  }

  .toolbar-row {
    align-items: stretch;
    flex-direction: column;
  }

  .rule-search,
  .template-filter {
    width: 100%;
    max-width: none;
  }

  .file-rule-item {
    grid-template-columns: 28px minmax(0, 1fr);
  }

  .file-rule-tools {
    grid-column: 2;
    justify-content: flex-start;
  }
}
</style>
