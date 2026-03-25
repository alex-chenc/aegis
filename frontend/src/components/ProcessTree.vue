<template>
  <div class="process-tree">
    <div v-if="!treeData" class="empty-state">
      <el-empty description="暂无进程树数据" :image-size="80" />
    </div>
    <div v-else class="tree-container">
      <!-- Parent Process Node -->
      <div v-if="treeData.ppid > 0" class="tree-level">
        <div class="level-label">父进程</div>
        <div class="process-card parent">
          <div class="process-header">
            <el-icon class="process-icon"><Folder /></el-icon>
            <span class="process-name">{{ getParentProcessName() }}</span>
          </div>
          <div class="process-details">
            <div class="detail-row">
              <span class="label">PID:</span>
              <el-tag size="small" type="info">{{ treeData.ppid }}</el-tag>
            </div>
            <div class="detail-row" v-if="treeData.ppid_user">
              <span class="label">用户:</span>
              <span class="value">{{ treeData.ppid_user }}</span>
            </div>
            <div class="detail-row cmdline-row" v-if="treeData.ppid_command_line">
              <span class="label">命令行:</span>
              <code class="cmdline">{{ treeData.ppid_command_line }}</code>
            </div>
          </div>
        </div>
        <div class="tree-branch down"></div>
      </div>

      <!-- Main Process Node -->
      <div class="tree-level main-level">
        <div class="level-label threat">威胁进程</div>
        <div class="process-card threat">
          <div class="process-header">
            <el-icon class="process-icon"><WarningFilled /></el-icon>
            <span class="process-name">{{ treeData.name }}</span>
            <el-tag type="danger" size="small" effect="dark">威胁</el-tag>
          </div>
          <div class="process-details">
            <div class="detail-grid">
              <div class="detail-item">
                <span class="label">PID</span>
                <el-tag type="primary">{{ treeData.pid }}</el-tag>
              </div>
              <div class="detail-item">
                <span class="label">PPID</span>
                <el-tag type="info">{{ treeData.ppid }}</el-tag>
              </div>
              <div class="detail-item">
                <span class="label">用户</span>
                <span class="value">{{ treeData.user || '-' }}</span>
              </div>
              <div class="detail-item">
                <span class="label">用户组</span>
                <span class="value">{{ treeData.user_group || '-' }}</span>
              </div>
            </div>
            <div class="detail-row" v-if="treeData.exe_path">
              <span class="label">启动路径:</span>
              <code class="cmdline">{{ treeData.exe_path }}</code>
            </div>
            <div class="detail-row" v-if="treeData.command_line">
              <span class="label">命令行:</span>
              <code class="cmdline">{{ treeData.command_line }}</code>
            </div>
          </div>
        </div>
      </div>

      <!-- Child Process Nodes -->
      <div v-if="treeData.children && treeData.children.length > 0" class="tree-level children-level">
        <div class="tree-branch up"></div>
        <div class="level-label">子进程 ({{ treeData.children.length }})</div>
        <div class="children-grid">
          <div v-for="(child, index) in treeData.children" :key="index" class="process-card child">
            <div class="process-header">
              <el-icon class="process-icon"><Document /></el-icon>
              <span class="process-name">{{ child.name }}</span>
            </div>
            <div class="process-details compact">
              <div class="detail-row">
                <el-tag size="small">{{ child.pid }}</el-tag>
                <span class="user-badge" v-if="child.user">{{ child.user }}</span>
              </div>
              <div class="detail-row cmdline-row" v-if="child.command_line">
                <code class="cmdline small">{{ truncate(child.command_line, 50) }}</code>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { Folder, WarningFilled, Document } from '@element-plus/icons-vue'

const props = defineProps<{
  processTree?: string
}>()

const treeData = ref<any>(null)

const parseProcessTree = () => {
  if (!props.processTree) {
    treeData.value = null
    return
  }
  
  try {
    treeData.value = typeof props.processTree === 'string' 
      ? JSON.parse(props.processTree) 
      : props.processTree
  } catch (e) {
    console.error('Failed to parse process tree:', e)
    treeData.value = null
  }
}

function getParentProcessName(): string {
  if (!treeData.value?.ppid_command_line) return `PID ${treeData.value.ppid}`
  const parts = treeData.value.ppid_command_line.split(' ')
  return parts[0]?.split('/').pop() || `PID ${treeData.value.ppid}`
}

function truncate(str: string, len: number): string {
  if (!str) return ''
  return str.length > len ? str.slice(0, len) + '...' : str
}

onMounted(() => {
  parseProcessTree()
})

watch(() => props.processTree, () => {
  parseProcessTree()
})
</script>

<style scoped>
.process-tree {
  margin-top: 16px;
}

.empty-state {
  padding: 20px;
  text-align: center;
}

.tree-container {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.tree-level {
  position: relative;
}

.level-label {
  font-size: 12px;
  color: #909399;
  margin-bottom: 8px;
  font-weight: 500;
}

.level-label.threat {
  color: #f56c6c;
  font-weight: 600;
}

.process-card {
  border-radius: 8px;
  background: #fff;
  border: 1px solid #e4e7ed;
  padding: 16px;
  margin-bottom: 8px;
  transition: all 0.3s ease;
}

.process-card.parent {
  background: linear-gradient(135deg, #fdf6ec 0%, #fff 100%);
  border-color: #e6a23c;
}

.process-card.threat {
  background: linear-gradient(135deg, #fef0f0 0%, #fff 100%);
  border-color: #f56c6c;
  border-width: 2px;
  box-shadow: 0 2px 12px rgba(245, 108, 108, 0.2);
}

.process-card.child {
  background: #fafafa;
  border-color: #dcdfe6;
  padding: 12px;
}

.process-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.process-icon {
  font-size: 18px;
  color: #909399;
}

.process-card.threat .process-icon {
  color: #f56c6c;
}

.process-name {
  font-weight: 600;
  font-size: 14px;
  color: #303133;
  flex: 1;
}

.process-details {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.process-details.compact {
  gap: 4px;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
  margin-bottom: 8px;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.detail-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.detail-row .label {
  color: #909399;
  font-size: 12px;
  min-width: 60px;
  flex-shrink: 0;
}

.detail-item .label {
  color: #909399;
  font-size: 11px;
  text-transform: uppercase;
}

.detail-item .value {
  font-size: 13px;
  color: #303133;
  font-weight: 500;
}

.cmdline {
  font-family: 'SF Mono', Monaco, Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
  background: #f5f7fa;
  padding: 4px 8px;
  border-radius: 4px;
  color: #606266;
  word-break: break-all;
  flex: 1;
}

.cmdline.small {
  font-size: 11px;
  padding: 2px 6px;
}

.cmdline-row {
  align-items: flex-start;
}

.user-badge {
  font-size: 11px;
  color: #909399;
  background: #f5f7fa;
  padding: 2px 6px;
  border-radius: 4px;
}

.tree-branch {
  display: flex;
  justify-content: center;
  height: 20px;
  position: relative;
}

.tree-branch::before {
  content: '';
  width: 2px;
  height: 100%;
  background: #dcdfe6;
}

.tree-branch.down::after {
  content: '';
  position: absolute;
  bottom: 0;
  left: 50%;
  transform: translateX(-50%);
  width: 0;
  height: 0;
  border-left: 6px solid transparent;
  border-right: 6px solid transparent;
  border-top: 8px solid #dcdfe6;
}

.tree-branch.up::after {
  content: '';
  position: absolute;
  top: 0;
  left: 50%;
  transform: translateX(-50%);
  width: 0;
  height: 0;
  border-left: 6px solid transparent;
  border-right: 6px solid transparent;
  border-bottom: 8px solid #dcdfe6;
}

.children-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
  gap: 12px;
  margin-top: 8px;
}

.children-level {
  margin-top: 0;
}

.main-level {
  margin: 0;
}
</style>