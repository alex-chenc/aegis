<template>
  <div class="weak-dictionary-page">
    <div class="page-toolbar">
      <div>
        <h1>{{ $t('generated.detectionWeakPasswordDictionaries_weak_password_dictionary_716a20') }}</h1>
        <p>{{ $t('generated.detectionWeakPasswordDictionaries_built_in_dictionaries_and_custom_dictionaries_50eccb') }}</p>
      </div>
      <div class="toolbar-actions">
        <el-button :icon="Back" @click="router.push('/risk/weak-password')">{{ $t('generated.common_return_11d024') }}</el-button>
        <el-button :icon="Refresh" @click="store.fetchDictionaries">{{ $t('generated.common_refresh_38108e') }}</el-button>
        <el-button type="primary" :icon="MagicStick" @click="drawerVisible = true">{{ $t('generated.detectionWeakPasswordDictionaries_ai_generates_dictionary_with_one_click_6704be') }}</el-button>
      </div>
    </div>

    <section class="summary-band">
      <div class="summary-item">
        <span>{{ $t('generated.detectionWeakPasswordDictionaries_built_in_dictionary_0db734') }}</span>
        <strong>{{ store.defaultDictionary?.entry_count || 0 }}</strong>
      </div>
      <div class="summary-item">
        <span>{{ $t('generated.detectionWeakPasswordDictionaries_total_number_of_dictionaries_9f65e7') }}</span>
        <strong>{{ store.dictionaries.length }}</strong>
      </div>
      <div class="summary-item">
        <span>{{ $t('generated.detectionWeakPasswordDictionaries_custom_dictionary_344b2f') }}</span>
        <strong>{{ customCount }}</strong>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>{{ $t('generated.detectionWeakPasswordDictionaries_dictionary_list_83f2e3') }}</h2>
        <span class="muted">{{ $t('generated.detectionWeakPasswordDictionaries_click_the_dictionary_to_view_weak_fd19bf') }}</span>
      </div>
      <el-table v-loading="store.dictionaryLoading" :data="store.dictionaries" class="dense-table">
        <el-table-column :label="$t('generated.detectionWeakPasswordDictionaries_dictionary_name_32bc83')" min-width="260" prop="name" />
        <el-table-column :label="$t('generated.common_type_e4e46c')" width="140">
          <template #default="{ row }">
            <el-tag :type="row.dictionary_type === 'default_1000' ? 'success' : 'info'">{{ dictionaryTypeLabel(row.dictionary_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="$t('generated.detectionWeakPasswordDictionaries_number_of_items_d67cef')" width="120" prop="entry_count" />
        <el-table-column :label="$t('generated.common_operate_f3ea6d')" width="130" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEntries(row)">{{ $t('generated.detectionWeakPasswordDictionaries_view_entry_fdcf34') }}</el-button>
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

    <el-drawer v-model="drawerVisible" :title="$t('generated.detectionWeakPasswordDictionaries_ai_generates_dictionary_with_one_click_6704be')" size="620px">
      <el-form label-position="top" class="drawer-form">
        <el-form-item :label="$t('generated.detectionWeakPasswordDictionaries_natural_language_description_687fc2')">
          <el-input
            v-model="aiForm.natural_language"
            type="textarea"
            :rows="7"
            maxlength="800"
            show-word-limit
            :placeholder="$t('generated.detectionWeakPasswordDictionaries_for_example_generate_a_weak_password_ae6257')"
          />
        </el-form-item>
        <el-form-item :label="$t('generated.detectionWeakPasswordDictionaries_generate_quantity_cb7916')">
          <el-input-number v-model="aiForm.count" :min="1" :max="50" />
          <span class="form-tip">{{ $t('generated.detectionWeakPasswordDictionaries_the_maximum_number_of_entries_is_1300dc') }}</span>
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="aiForm.deduplicate_with_default">{{ $t('generated.detectionWeakPasswordDictionaries_deduplication_with_built_in_dictionary_88ccb1') }}</el-checkbox>
        </el-form-item>
      </el-form>
      <div class="drawer-actions">
        <el-button @click="drawerVisible = false">{{ $t('generated.common_cancel_4d0b46') }}</el-button>
        <el-button type="primary" :loading="generating" @click="generate">{{ $t('generated.detectionWeakPasswordDictionaries_generate_and_save_1ca174') }}</el-button>
      </div>
      <el-alert
        v-if="generated"
        type="success"
        show-icon
        :closable="false"
        :title="$t('dynamic.generatedCandidates', { count: generated.entry_count })"
      />
    </el-drawer>

    <el-drawer v-model="entriesVisible" :title="selectedDictionary ? selectedDictionary.name : $t('dynamic.dictionaryEntries')" size="720px">
      <el-table v-loading="store.dictionaryLoading" :data="store.dictionaryEntries" class="dense-table">
        <el-table-column label="#" type="index" width="70" :index="entryIndex" />
        <el-table-column :label="$t('generated.detectionWeakPasswordDictionaries_weak_password_65116f')" min-width="260">
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
import { translate } from '@/i18n'

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
    ElMessage.warning(translate('generatedScript.detectionWeakPasswordDictionaries_please_enter_a_natural_language_description_e66b20'))
    return
  }
  generating.value = true
  try {
    generated.value = await store.generateDictionary({
      natural_language: aiForm.natural_language,
      count: aiForm.count,
      deduplicate_with_default: aiForm.deduplicate_with_default,
    })
    ElMessage.success(translate('generatedScript.detectionWeakPasswordDictionaries_dictionary_saved_54ada8'))
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
  return type === 'default_1000' ? translate('generatedScript.detectionWeakPasswordDictionaries_built_in_09ceea') : translate('generatedScript.common_customize_c49333')
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
