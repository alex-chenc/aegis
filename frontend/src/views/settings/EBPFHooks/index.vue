<template>
  <div class="ebpf-hooks-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ $t('generated.settingsEBPFHooksIndex_ebpf_hook_whitelist_configuration_643d2a') }}</span>
          <el-button type="primary" :loading="saving" @click="handleSave">{{ $t('generated.common_save_configuration_817af1') }}</el-button>
        </div>
      </template>

      <el-alert type="warning" :closable="false" show-icon style="margin-bottom: 16px;">
        <template #title>
          {{ $t('generated.settingsEBPFHooksIndex_high_risk_hook_types_kprobe_lsm_c7c583') }}
        </template>
      </el-alert>

      <el-tabs v-model="activeTab">
        <el-tab-pane label="Tracepoints" name="tracepoints">
          <div class="tab-header">
            <el-text>{{ $t('generated.settingsEBPFHooksIndex_tracepoint_type_allowed_by_default_lower_70bf54') }}</el-text>
            <el-button size="small" @click="addHook('tracepoints')">{{ $t('generated.settingsEBPFHooksIndex_add_to_94191c') }}</el-button>
          </div>
          <el-table :data="allowlist.tracepoints" border size="small">
            <el-table-column prop="" label="Tracepoint" min-width="300">
              <template #default="{ row }">
                <el-input v-model="row.value" size="small" :placeholder="$t('generated.settingsEBPFHooksIndex_such_as_syscalls_sys_enter_execve_5571a7')" />
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="80">
              <template #default="{ $index }">
                <el-button link type="danger" size="small" @click="removeHook('tracepoints', $index)">{{ $t('generated.common_delete_3755f5') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="Kprobes" name="kprobes">
          <div class="tab-header">
            <el-tag type="warning" size="small">{{ $t('generated.settingsEBPFHooksIndex_high_risk_7a83b6') }}</el-tag>
            <el-text style="margin-left: 8px;">{{ $t('generated.settingsEBPFHooksIndex_kprobe_type_fc86c7') }}</el-text>
            <el-button size="small" @click="addHook('kprobes')">{{ $t('generated.settingsEBPFHooksIndex_add_to_94191c') }}</el-button>
          </div>
          <el-alert type="warning" :closable="false" show-icon style="margin: 8px 0;">
            {{ $t('generated.settingsEBPFHooksIndex_kprobe_can_hook_any_kernel_function_042951') }}
          </el-alert>
          <el-table :data="allowlist.kprobes" border size="small">
            <el-table-column prop="" label="Kprobe" min-width="300">
              <template #default="{ row }">
                <el-input v-model="row.value" size="small" :placeholder="$t('generated.settingsEBPFHooksIndex_such_as_do_sys_open_bef84b')" />
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="80">
              <template #default="{ $index }">
                <el-button link type="danger" size="small" @click="removeHook('kprobes', $index)">{{ $t('generated.common_delete_3755f5') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="LSM" name="lsm">
          <div class="tab-header">
            <el-tag type="danger" size="small">{{ $t('generated.settingsEBPFHooksIndex_high_risk_7a83b6') }}</el-tag>
            <el-text style="margin-left: 8px;">{{ $t('generated.settingsEBPFHooksIndex_lsm_hook_type_f4d683') }}</el-text>
            <el-button size="small" @click="addHook('lsm')">{{ $t('generated.settingsEBPFHooksIndex_add_to_94191c') }}</el-button>
          </div>
          <el-alert type="error" :closable="false" show-icon style="margin: 8px 0;">
            {{ $t('generated.settingsEBPFHooksIndex_lsm_hooks_affect_system_security_policies_da8715') }}
          </el-alert>
          <el-table :data="allowlist.lsm" border size="small">
            <el-table-column prop="" label="LSM Hook" min-width="300">
              <template #default="{ row }">
                <el-input v-model="row.value" size="small" :placeholder="$t('generated.settingsEBPFHooksIndex_such_as_bprm_check_security_85a1d2')" />
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="80">
              <template #default="{ $index }">
                <el-button link type="danger" size="small" @click="removeHook('lsm', $index)">{{ $t('generated.common_delete_3755f5') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="XDP" name="xdp">
          <div class="tab-header">
            <el-tag type="danger" size="small">{{ $t('generated.settingsEBPFHooksIndex_high_risk_7a83b6') }}</el-tag>
            <el-text style="margin-left: 8px;">{{ $t('generated.settingsEBPFHooksIndex_xdp_hook_type_b7ca13') }}</el-text>
            <el-button size="small" @click="addHook('xdp')">{{ $t('generated.settingsEBPFHooksIndex_add_to_94191c') }}</el-button>
          </div>
          <el-table :data="allowlist.xdp" border size="small">
            <el-table-column prop="" label="XDP" min-width="300">
              <template #default="{ row }">
                <el-input v-model="row.value" size="small" :placeholder="$t('generated.settingsEBPFHooksIndex_such_as_eth0_d0a8f3')" />
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="80">
              <template #default="{ $index }">
                <el-button link type="danger" size="small" @click="removeHook('xdp', $index)">{{ $t('generated.common_delete_3755f5') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="TC" name="tc">
          <div class="tab-header">
            <el-tag type="danger" size="small">{{ $t('generated.settingsEBPFHooksIndex_high_risk_7a83b6') }}</el-tag>
            <el-text style="margin-left: 8px;">{{ $t('generated.settingsEBPFHooksIndex_tc_hook_type_f6f019') }}</el-text>
            <el-button size="small" @click="addHook('tc')">{{ $t('generated.settingsEBPFHooksIndex_add_to_94191c') }}</el-button>
          </div>
          <el-table :data="allowlist.tc" border size="small">
            <el-table-column prop="" label="TC" min-width="300">
              <template #default="{ row }">
                <el-input v-model="row.value" size="small" :placeholder="$t('generated.settingsEBPFHooksIndex_such_as_eth0_d0a8f3')" />
              </template>
            </el-table-column>
            <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="80">
              <template #default="{ $index }">
                <el-button link type="danger" size="small" @click="removeHook('tc', $index)">{{ $t('generated.common_delete_3755f5') }}</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>

      <el-collapse style="margin-top: 24px;">
        <el-collapse-item :title="$t('generated.settingsEBPFHooksIndex_whitelist_change_history_296031')" name="history">
          <el-table :data="historyRecords" border size="small" v-loading="loadingHistory">
            <el-table-column prop="operator" :label="$t('generated.common_operator_06858d')" width="120" />
            <el-table-column prop="operated_at" :label="$t('generated.settingsEBPFHooksIndex_operating_time_c7fc0c')" width="180">
              <template #default="{ row }">{{ formatDateTime(row.operated_at) }}</template>
            </el-table-column>
            <el-table-column prop="change_reason" :label="$t('generated.settingsEBPFHooksIndex_reason_for_change_53aecf')" min-width="200" />
            <el-table-column :label="$t('generated.settingsEBPFHooksIndex_change_content_a8766c')" min-width="300">
              <template #default="{ row }">
                <pre class="diff-content">{{ formatChangeContent(row) }}</pre>
              </template>
            </el-table-column>
          </el-table>
        </el-collapse-item>
      </el-collapse>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { translate } from '@/i18n'
import { formatDateTime } from '@/i18n/formatters'

import { ref, reactive, onMounted } from 'vue'
import { ebpfHookApi } from '@/api/ebpf-hooks'
import { ElMessage } from 'element-plus'

const activeTab = ref('tracepoints')
const saving = ref(false)
const historyRecords = ref<any[]>([])
const loadingHistory = ref(false)

const allowlist = reactive({
  tracepoints: [] as { value: string }[],
  kprobes: [] as { value: string }[],
  lsm: [] as { value: string }[],
  xdp: [] as { value: string }[],
  tc: [] as { value: string }[],
})

async function loadAllowlist() {
  try {
    const data = await ebpfHookApi.getAllowlist()
    allowlist.tracepoints = (data.tracepoints || []).map(v => ({ value: v }))
    allowlist.kprobes = (data.kprobes || []).map(v => ({ value: v }))
    allowlist.lsm = (data.lsm || []).map(v => ({ value: v }))
    allowlist.xdp = (data.xdp || []).map(v => ({ value: v }))
    allowlist.tc = (data.tc || []).map(v => ({ value: v }))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.settingsEBPFHooksIndex_failed_to_load_whitelist_ea10e6'))
  }
}

function addHook(type: keyof typeof allowlist) {
  allowlist[type].push({ value: '' })
}

function removeHook(type: keyof typeof allowlist, index: number) {
  allowlist[type].splice(index, 1)
}

async function loadHistory() {
  loadingHistory.value = true
  try {
    historyRecords.value = await ebpfHookApi.getAllowlistHistory()
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.settingsEBPFHooksIndex_failed_to_load_change_history_7a3bb9'))
  } finally {
    loadingHistory.value = false
  }
}

function formatChangeContent(row: any) {
  if (row.change_detail) {
    return typeof row.change_detail === 'string' ? row.change_detail : JSON.stringify(row.change_detail, null, 2)
  }
  return '-'
}

async function handleSave() {
  saving.value = true
  try {
    await ebpfHookApi.updateAllowlist({
      tracepoints: allowlist.tracepoints.map(h => h.value).filter(Boolean),
      kprobes: allowlist.kprobes.map(h => h.value).filter(Boolean),
      lsm: allowlist.lsm.map(h => h.value).filter(Boolean),
      xdp: allowlist.xdp.map(h => h.value).filter(Boolean),
      tc: allowlist.tc.map(h => h.value).filter(Boolean),
    })
    ElMessage.success(translate('generatedScript.settingsEBPFHooksIndex_whitelist_configuration_saved_256dcf'))
  } catch (e: any) {
    ElMessage.error(e.message || translate('generatedScript.common_save_failed_40525a'))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadAllowlist()
  loadHistory()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.tab-header {
  display: flex;
  align-items: center;
  margin-bottom: 12px;
  gap: 8px;
}
.diff-content {
  font-family: monospace;
  font-size: 12px;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
