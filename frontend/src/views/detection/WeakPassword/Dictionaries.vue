<template>
  <div class="weak-dictionary-page">
    <div class="page-toolbar">
      <div>
        <h1>弱密码字典</h1>
        <p>内置字典和自定义字典统一管理。</p>
      </div>
      <div class="toolbar-actions">
        <el-button :icon="Back" @click="router.push('/risk/weak-password')">返回</el-button>
        <el-button :icon="Refresh" @click="store.fetchDictionaries">刷新</el-button>
        <el-button type="primary" :icon="MagicStick" @click="drawerVisible = true">AI 一键生成字典</el-button>
      </div>
    </div>

    <section class="summary-band">
      <div class="summary-item">
        <span>内置字典</span>
        <strong>{{ store.defaultDictionary?.entry_count || 0 }}</strong>
      </div>
      <div class="summary-item">
        <span>字典总数</span>
        <strong>{{ store.dictionaries.length }}</strong>
      </div>
      <div class="summary-item">
        <span>自定义字典</span>
        <strong>{{ customCount }}</strong>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>字典列表</h2>
        <span class="muted">点击字典可逐条查看弱密码候选。</span>
      </div>
      <el-table v-loading="store.dictionaryLoading" :data="store.dictionaries" class="dense-table">
        <el-table-column label="字典名称" min-width="260" prop="name" />
        <el-table-column label="类型" width="140">
          <template #default="{ row }">
            <el-tag :type="row.dictionary_type === 'default_1000' ? 'success' : 'info'">{{ dictionaryTypeLabel(row.dictionary_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="条数" width="120" prop="entry_count" />
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEntries(row)">查看条目</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="store.dictionaryTotal > 0" class="pagination-bar">
        <el-pagination
          v-model:current-page="store.dictionaryFilters.page"
          v-model:page-size="store.dictionaryFilters.page_size"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          :total="store.dictionaryTotal"
          @size-change="store.fetchDictionaries"
          @current-change="store.fetchDictionaries"
        />
      </div>
    </section>

    <el-drawer v-model="drawerVisible" title="AI 一键生成字典" size="620px">
      <el-form label-position="top" class="drawer-form">
        <el-form-item label="自然语言描述">
          <el-input
            v-model="aiForm.natural_language"
            type="textarea"
            :rows="7"
            maxlength="800"
            show-word-limit
            placeholder="例如：为 Redis 管理员和生产环境生成弱密码字典，包含公司名 aegis、年份、常见符号和 admin/root 等账号习惯"
          />
        </el-form-item>
        <el-form-item label="生成数量">
          <el-input-number v-model="aiForm.count" :min="1" :max="50" />
          <span class="form-tip">单次最多 50 条，生成过程会等待 AI 实际返回。</span>
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="aiForm.deduplicate_with_default">与内置字典去重</el-checkbox>
        </el-form-item>
      </el-form>
      <div class="drawer-actions">
        <el-button @click="drawerVisible = false">取消</el-button>
        <el-button type="primary" :loading="generating" @click="generate">生成并保存</el-button>
      </div>
      <el-alert
        v-if="generated"
        type="success"
        show-icon
        :closable="false"
        :title="`已生成 ${generated.entry_count} 条候选`"
      />
    </el-drawer>

    <el-drawer v-model="entriesVisible" :title="selectedDictionary ? selectedDictionary.name : '字典条目'" size="720px">
      <el-table v-loading="store.dictionaryLoading" :data="store.dictionaryEntries" class="dense-table">
        <el-table-column label="#" type="index" width="70" :index="entryIndex" />
        <el-table-column label="弱密码" min-width="260">
          <template #default="{ row }">
            <code class="candidate-value">{{ row.candidate }}</code>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="store.dictionaryEntryTotal > 0" class="pagination-bar">
        <el-pagination
          v-model:current-page="store.dictionaryEntryFilters.page"
          v-model:page-size="store.dictionaryEntryFilters.page_size"
          :page-sizes="[20, 50, 100, 500, 1000]"
          layout="total, sizes, prev, pager, next"
          :total="store.dictionaryEntryTotal"
          @size-change="fetchEntries"
          @current-change="fetchEntries"
        />
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Back, MagicStick, Refresh } from '@element-plus/icons-vue'
import { useWeakPasswordStore } from '@/store/weakPassword'
import type { WeakPasswordDictionary } from '@/types/weakPassword'

const router = useRouter()
const store = useWeakPasswordStore()
const drawerVisible = ref(false)
const entriesVisible = ref(false)
const generating = ref(false)
const generated = ref<WeakPasswordDictionary | null>(null)
const selectedDictionary = ref<WeakPasswordDictionary | null>(null)

const aiForm = reactive({
  natural_language: '',
  count: 20,
  deduplicate_with_default: true,
})

const customCount = computed(() => store.dictionaries.filter(item => item.dictionary_type !== 'default_1000').length)

async function generate() {
  if (!aiForm.natural_language.trim()) {
    ElMessage.warning('请输入自然语言描述')
    return
  }
  generating.value = true
  try {
    generated.value = await store.generateDictionary({
      natural_language: aiForm.natural_language,
      count: aiForm.count,
      deduplicate_with_default: aiForm.deduplicate_with_default,
    })
    ElMessage.success('字典已保存')
  } finally {
    generating.value = false
  }
}

async function openEntries(row: WeakPasswordDictionary) {
  selectedDictionary.value = row
  store.dictionaryEntryFilters.page = 1
  entriesVisible.value = true
  await fetchEntries()
}

async function fetchEntries() {
  if (!selectedDictionary.value) return
  await store.fetchDictionaryEntries(selectedDictionary.value.id)
}

function entryIndex(index: number) {
  return (store.dictionaryEntryFilters.page - 1) * store.dictionaryEntryFilters.page_size + index + 1
}

function dictionaryTypeLabel(type: string) {
  return type === 'default_1000' ? '内置' : '自定义'
}

onMounted(() => {
  store.fetchDictionaries()
})
</script>

<style scoped>
.weak-dictionary-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.page-toolbar,
.toolbar-actions,
.panel-head,
.drawer-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.toolbar-actions,
.drawer-actions {
  justify-content: flex-end;
}

.page-toolbar h1,
.panel-head h2 {
  margin: 0;
  color: #0f172a;
}

.page-toolbar p,
.muted {
  color: #64748b;
  margin: 6px 0 0;
}

.summary-band {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.summary-item,
.panel {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
}

.summary-item {
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.summary-item span {
  color: #64748b;
}

.summary-item strong {
  color: #0f172a;
  font-size: 26px;
}

.panel {
  padding: 14px;
}

.dense-table {
  width: 100%;
}

.pagination-bar {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

.candidate-value {
  color: #0f172a;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.form-tip {
  margin-left: 12px;
  color: #64748b;
  font-size: 12px;
}

.drawer-form {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
</style>
