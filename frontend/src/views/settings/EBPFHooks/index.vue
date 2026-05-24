<template>
  <div class="ebpf-hooks-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>eBPF Hook 白名单配置</span>
          <el-button type="primary" :loading="saving" @click="handleSave">保存配置</el-button>
        </div>
      </template>

      <el-alert type="warning" :closable="false" show-icon style="margin-bottom: 16px;">
        <template #title>
          高风险 Hook 类型（kprobe/lsm/xdp/tc）默认关闭。启用后需谨慎评估安全影响。
        </template>
      </el-alert>

      <el-tabs v-model="activeTab">
        <el-tab-pane label="Tracepoints" name="tracepoints">
          <div class="tab-header">
            <el-text>Tracepoint 类型（默认允许，风险较低）</el-text>
            <el-button size="small" @click="addHook('tracepoints')">添加</el-button>
          </div>
          <el-table :data="allowlist.tracepoints" border size="small">
            <el-table-column prop="" label="Tracepoint" min-width="300">
              <template #default="{ row }">
                <el-input v-model="row.value" size="small" placeholder="如 syscalls/sys_enter_execve" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="80">
              <template #default="{ $index }">
                <el-button link type="danger" size="small" @click="removeHook('tracepoints', $index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="Kprobes" name="kprobes">
          <div class="tab-header">
            <el-tag type="warning" size="small">高风险</el-tag>
            <el-text style="margin-left: 8px;">Kprobe 类型</el-text>
            <el-button size="small" @click="addHook('kprobes')">添加</el-button>
          </div>
          <el-alert type="warning" :closable="false" show-icon style="margin: 8px 0;">
            Kprobe 可以 hook 任意内核函数，存在稳定性风险。仅在必要时启用。
          </el-alert>
          <el-table :data="allowlist.kprobes" border size="small">
            <el-table-column prop="" label="Kprobe" min-width="300">
              <template #default="{ row }">
                <el-input v-model="row.value" size="small" placeholder="如 do_sys_open" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="80">
              <template #default="{ $index }">
                <el-button link type="danger" size="small" @click="removeHook('kprobes', $index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="LSM" name="lsm">
          <div class="tab-header">
            <el-tag type="danger" size="small">高风险</el-tag>
            <el-text style="margin-left: 8px;">LSM Hook 类型</el-text>
            <el-button size="small" @click="addHook('lsm')">添加</el-button>
          </div>
          <el-alert type="error" :closable="false" show-icon style="margin: 8px 0;">
            LSM hook 影响系统安全策略。仅限高级管理员配置。
          </el-alert>
          <el-table :data="allowlist.lsm" border size="small">
            <el-table-column prop="" label="LSM Hook" min-width="300">
              <template #default="{ row }">
                <el-input v-model="row.value" size="small" placeholder="如 bprm_check_security" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="80">
              <template #default="{ $index }">
                <el-button link type="danger" size="small" @click="removeHook('lsm', $index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="XDP" name="xdp">
          <div class="tab-header">
            <el-tag type="danger" size="small">高风险</el-tag>
            <el-text style="margin-left: 8px;">XDP Hook 类型</el-text>
            <el-button size="small" @click="addHook('xdp')">添加</el-button>
          </div>
          <el-table :data="allowlist.xdp" border size="small">
            <el-table-column prop="" label="XDP" min-width="300">
              <template #default="{ row }">
                <el-input v-model="row.value" size="small" placeholder="如 eth0" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="80">
              <template #default="{ $index }">
                <el-button link type="danger" size="small" @click="removeHook('xdp', $index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="TC" name="tc">
          <div class="tab-header">
            <el-tag type="danger" size="small">高风险</el-tag>
            <el-text style="margin-left: 8px;">TC Hook 类型</el-text>
            <el-button size="small" @click="addHook('tc')">添加</el-button>
          </div>
          <el-table :data="allowlist.tc" border size="small">
            <el-table-column prop="" label="TC" min-width="300">
              <template #default="{ row }">
                <el-input v-model="row.value" size="small" placeholder="如 eth0" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="80">
              <template #default="{ $index }">
                <el-button link type="danger" size="small" @click="removeHook('tc', $index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ebpfHookApi } from '@/api/ebpf-hooks'
import { ElMessage } from 'element-plus'

const activeTab = ref('tracepoints')
const saving = ref(false)

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
    ElMessage.error(e.message || '加载白名单失败')
  }
}

function addHook(type: keyof typeof allowlist) {
  allowlist[type].push({ value: '' })
}

function removeHook(type: keyof typeof allowlist, index: number) {
  allowlist[type].splice(index, 1)
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
    ElMessage.success('白名单配置已保存')
  } catch (e: any) {
    ElMessage.error(e.message || '保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadAllowlist()
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
</style>
