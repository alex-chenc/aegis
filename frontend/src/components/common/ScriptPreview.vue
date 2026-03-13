<template>
  <div class="script-preview">
    <div class="preview-header">
      <span class="language-badge">{{ language }}</span>
      <el-button
        :icon="CopyDocument"
        size="small"
        @click="copyToClipboard"
      >
        复制
      </el-button>
    </div>
    
    <div v-if="showSecurityWarning" class="security-warning">
      <el-alert
        :title="warningTitle"
        :type="warningType"
        show-icon
        :closable="false"
      >
        <template #default>
          <p>{{ warningMessage }}</p>
        </template>
      </el-alert>
    </div>
    
    <div class="code-container">
      <pre><code :class="`language-${language}`" v-html="highlightedCode"></code></pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'

const props = withDefaults(defineProps<{
  script: string
  language?: 'shell' | 'python' | 'javascript'
  mode?: 'fix' | 'poc'
}>(), {
  language: 'shell',
  mode: 'poc'
})

const showSecurityWarning = computed(() => props.mode === 'poc')

const warningTitle = computed(() => 
  props.mode === 'poc' ? '安全验证脚本' : '修复脚本'
)

const warningType = computed(() => 
  props.mode === 'poc' ? 'success' : 'warning'
)

const warningMessage = computed(() => 
  props.mode === 'poc' 
    ? '此脚本仅用于验证漏洞是否存在，不会修改系统配置' 
    : '此脚本将修改系统配置以修复漏洞，请确认后执行'
)

const highlightedCode = computed(() => {
  return highlightShell(props.script)
})

function highlightShell(code: string): string {
  const keywords = ['if', 'then', 'else', 'fi', 'for', 'do', 'done', 'while', 'case', 'esac', 'function', 'return', 'exit', 'echo', 'printf', 'read', 'test', 'true', 'false']
  const builtins = ['yum', 'apt', 'dnf', 'rpm', 'dpkg', 'systemctl', 'service', 'chmod', 'chown', 'mkdir', 'rm', 'cp', 'mv', 'cat', 'grep', 'sed', 'awk', 'find', 'curl', 'wget']
  
  let highlighted = escapeHtml(code)
  
  keywords.forEach(kw => {
    const regex = new RegExp(`\\b(${kw})\\b`, 'g')
    highlighted = highlighted.replace(regex, '<span class="keyword">$1</span>')
  })
  
  builtins.forEach(cmd => {
    const regex = new RegExp(`\\b(${cmd})\\b`, 'g')
    highlighted = highlighted.replace(regex, '<span class="builtin">$1</span>')
  })
  
  highlighted = highlighted
    .replace(/(&lt;!--.*?--&gt;|&lt;#.*?#&gt;)/gs, '<span class="comment">$1</span>')
    .replace(/(#.*$)/gm, '<span class="comment">$1</span>')
    .replace(/(".*?"|'.*?')/g, '<span class="string">$&</span>')
    .replace(/\$(\w+|\{[^}]+\})/g, '<span class="variable">$$$1</span>')
  
  return highlighted
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

async function copyToClipboard() {
  try {
    await navigator.clipboard.writeText(props.script)
    ElMessage.success('脚本已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}
</script>

<style scoped>
.script-preview {
  background-color: #1e1e1e;
  border-radius: 8px;
  overflow: hidden;
}

.preview-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 16px;
  background-color: #2d2d2d;
  border-bottom: 1px solid #3d3d3d;
}

.language-badge {
  background-color: #409EFF;
  color: white;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
}

.security-warning {
  padding: 12px 16px;
  background-color: #1a1a1a;
}

.code-container {
  padding: 16px;
  overflow-x: auto;
  max-height: 400px;
}

pre {
  margin: 0;
  font-family: 'Fira Code', 'Monaco', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.6;
  color: #d4d4d4;
}

code {
  font-family: inherit;
}

:deep(.keyword) {
  color: #569cd6;
  font-weight: 500;
}

:deep(.builtin) {
  color: #dcdcaa;
}

:deep(.comment) {
  color: #6a9955;
  font-style: italic;
}

:deep(.string) {
  color: #ce9178;
}

:deep(.variable) {
  color: #9cdcfe;
}
</style>