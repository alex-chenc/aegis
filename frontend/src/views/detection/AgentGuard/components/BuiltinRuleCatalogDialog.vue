<template>
  <el-dialog
    :model-value="visible"
    :title="title"
    width="1080px"
    destroy-on-close
    @close="emit('close')"
  >
    <el-alert type="info" :title="description" :closable="false" show-icon />
    <el-empty v-if="loading || !rules.length" :description="loading ? '正在加载内置规则…' : '当前没有可展示的内置规则'" />
    <div v-else class="builtin-rule-list">
      <article v-for="rule in rules" :key="`${rule.rule_key}:${rule.rule_version || 1}`" class="builtin-rule-card">
        <div class="rule-card-heading">
          <div>
            <h3>{{ rule.name }}</h3>
            <span class="rule-key">{{ rule.rule_key }} · v{{ rule.rule_version || 1 }}</span>
          </div>
          <div class="rule-tags">
            <el-tag :type="severityType(rule.default_severity)">{{ severityLabel(rule.default_severity) }}</el-tag>
            <el-tag type="success" effect="plain">内置 · 只读</el-tag>
          </div>
        </div>
        <p class="rule-description">{{ rule.description || '暂无规则说明' }}</p>
        <div class="rule-meta-grid">
          <div><span class="meta-label">分类</span><div class="meta-values"><el-tag v-for="category in rule.categories || []" :key="category" size="small">{{ categoryLabel(category) }}</el-tag><span v-if="!rule.categories?.length" class="muted">-</span></div></div>
          <div><span class="meta-label">执行引擎</span><code>{{ rule.engine || '-' }}</code></div>
          <div><span class="meta-label">默认动作</span><span>{{ actionLabel(rule.default_action) }}</span></div>
          <div><span class="meta-label">推荐动作</span><span>{{ actionLabel(rule.recommended_action || rule.default_action) }}</span></div>
          <div><span class="meta-label">默认状态</span><span>{{ rule.default_enabled === false ? '停用' : '启用' }}</span></div>
          <div><span class="meta-label">摘要校验</span><code class="digest">{{ rule.digest || '-' }}</code></div>
        </div>
      </article>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import type { PropType } from 'vue'
import type { AgentSessionRule } from '@/types/agentSession'

defineProps({
  visible: { type: Boolean, required: true },
  title: { type: String, required: true },
  description: { type: String, required: true },
  rules: { type: Array as PropType<AgentSessionRule[]>, required: true },
  loading: { type: Boolean, default: false },
})
const emit = defineEmits<{ close: [] }>()

function categoryLabel(category: string) {
  return ({ prompt_injection: '提示词注入', jailbreak: '越狱绕过', secret_exposure: '敏感信息暴露', tool_abuse: '工具滥用', permission: '权限', sandbox: '沙箱', network: '网络', secret: '敏感信息', integrity: '完整性', hook: 'Hook' } as Record<string, string>)[category] || category
}
function severityLabel(severity?: string) { return ({ critical: '严重', high: '高', medium: '中', low: '低' } as Record<string, string>)[severity || ''] || severity || '未知' }
function severityType(severity?: string) { return severity === 'critical' ? 'danger' : severity === 'high' ? 'warning' : severity === 'medium' ? '' : 'info' }
function actionLabel(action?: string) { return ({ alert: '告警', audit: '审计', monitor: '监控', block: '阻断' } as Record<string, string>)[action || ''] || action || '-' }
</script>

<style scoped>
.builtin-rule-list { display: grid; gap: 14px; max-height: 68vh; overflow: auto; margin-top: 16px; padding-right: 4px; }
.builtin-rule-card { padding: 16px; border: 1px solid var(--el-border-color-lighter); border-radius: 12px; background: var(--el-fill-color-blank); }
.rule-card-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.rule-card-heading h3 { margin: 0 0 5px; font-size: 16px; }
.rule-key { color: var(--el-text-color-secondary); font: 12px/1.4 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.rule-tags { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 6px; }
.rule-description { margin: 13px 0; color: var(--el-text-color-regular); line-height: 1.6; }
.rule-meta-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px 18px; padding-top: 12px; border-top: 1px solid var(--el-border-color-lighter); }
.rule-meta-grid > div { display: flex; min-width: 0; flex-direction: column; gap: 5px; }
.meta-label { color: var(--el-text-color-secondary); font-size: 12px; }
.meta-values { display: flex; flex-wrap: wrap; gap: 5px; }
.rule-meta-grid code { overflow-wrap: anywhere; word-break: break-word; font-size: 12px; }
.digest { color: var(--el-text-color-secondary); }
.muted { color: var(--el-text-color-secondary); }
@media (max-width: 720px) { .rule-card-heading { flex-direction: column; }.rule-tags { justify-content: flex-start; }.rule-meta-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
