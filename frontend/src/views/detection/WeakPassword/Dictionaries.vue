<template>
  <div class="weak-dictionary-page">
    <div class="page-toolbar">
      <div>
        <h1>弱密码字典</h1>
        <p>默认字典、上传字典和 AI 生成字典统一管理。</p>
      </div>
      <div class="toolbar-actions">
        <el-button :icon="Back" @click="router.push('/risk/weak-password')">返回</el-button>
        <el-button :icon="Refresh" @click="store.fetchDictionaries">刷新</el-button>
        <el-button type="primary" :icon="MagicStick" @click="drawerVisible = true">AI 一键生成字典</el-button>
      </div>
    </div>

    <section class="summary-band">
      <div class="summary-item">
        <span>默认弱密码字典</span>
        <strong>{{ store.defaultDictionary?.entry_count || 0 }}</strong>
      </div>
      <div class="summary-item">
        <span>字典总数</span>
        <strong>{{ store.dictionaries.length }}</strong>
      </div>
      <div class="summary-item">
        <span>启用字典</span>
        <strong>{{ enabledCount }}</strong>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>字典列表</h2>
        <span class="muted">默认字典只展示摘要和分类。</span>
      </div>
      <el-table v-loading="store.dictionaryLoading" :data="store.dictionaries" class="dense-table">
        <el-table-column label="字典名称" min-width="220" prop="name" />
        <el-table-column label="类型" width="150" prop="dictionary_type" />
        <el-table-column label="条数" width="100" prop="entry_count" />
        <el-table-column label="分类" min-width="260">
          <template #default="{ row }">
            <el-tag v-for="item in row.categories" :key="item" effect="plain">{{ item }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="row.status === 'enabled' ? 'success' : 'info'">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="来源" width="140" prop="source" />
      </el-table>
    </section>

    <el-drawer v-model="drawerVisible" title="AI 一键生成字典" size="620px">
      <el-form label-position="top" class="drawer-form">
        <el-form-item label="生成目标">
          <el-segmented v-model="aiForm.target" :options="targetOptions" />
        </el-form-item>
        <el-form-item label="应用类型">
          <el-select v-model="aiForm.application_type" placeholder="应用类型">
            <el-option label="Redis" value="redis" />
            <el-option label="MySQL" value="mysql" />
            <el-option label="PostgreSQL" value="postgresql" />
            <el-option label="Nginx Basic Auth" value="nginx" />
            <el-option label="AI Agent" value="ai_agent" />
            <el-option label="MCP Server" value="mcp_server" />
            <el-option label="LLM Gateway" value="llm_service" />
          </el-select>
        </el-form-item>
        <el-form-item label="组织关键词">
          <el-select v-model="aiForm.organization_keywords" multiple filterable allow-create default-first-option placeholder="输入后回车" />
        </el-form-item>
        <el-form-item label="账号关键词">
          <el-select v-model="aiForm.account_keywords" multiple filterable allow-create default-first-option placeholder="输入后回车" />
        </el-form-item>
        <el-form-item label="生成数量">
          <el-input-number v-model="aiForm.count" :min="1" :max="1000" />
        </el-form-item>
        <el-form-item label="规则">
          <el-checkbox-group v-model="aiForm.rules">
            <el-checkbox label="append_year">年份后缀</el-checkbox>
            <el-checkbox label="append_special_char">特殊字符</el-checkbox>
            <el-checkbox label="capitalize">大小写</el-checkbox>
            <el-checkbox label="leet_replace">leet</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="aiForm.deduplicate_with_default">与默认字典去重</el-checkbox>
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
const generating = ref(false)
const generated = ref<WeakPasswordDictionary | null>(null)

const targetOptions = [
  { label: '通用', value: 'general' },
  { label: '应用', value: 'application' },
  { label: '账号模式', value: 'account' },
]

const aiForm = reactive({
  target: 'application',
  application_type: 'redis',
  organization_keywords: ['aegis'] as string[],
  account_keywords: ['admin', 'root'] as string[],
  count: 200,
  rules: ['append_year', 'append_special_char', 'capitalize'] as string[],
  deduplicate_with_default: true,
})

const enabledCount = computed(() => store.dictionaries.filter(item => item.status === 'enabled').length)

async function generate() {
  generating.value = true
  try {
    generated.value = await store.generateDictionary(aiForm)
    ElMessage.success('字典已保存')
  } finally {
    generating.value = false
  }
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

.drawer-form {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
</style>
