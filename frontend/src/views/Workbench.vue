<template>
  <div class="workbench-page">
    <el-row :gutter="20">
      <el-col :span="16">
        <el-card class="box-card mb-20">
          <template #header>
            <div class="card-header">
              <span>基线模板上传</span>
            </div>
          </template>
          <el-upload
            class="upload-demo"
            drag
            action="#"
            :auto-upload="false"
            :on-change="handleFileChange"
            :show-file-list="false"
            accept=".txt,.md,.pdf,.doc,.docx"
          >
            <el-icon class="el-icon--upload"><upload-filled /></el-icon>
            <div class="el-upload__text">
              将文件拖到此处，或 <em>点击上传</em>
            </div>
            <template #tip>
              <div class="el-upload__tip">
                支持 .txt, .md, .pdf 等格式文档，上传后将自动解析生成基线规则
              </div>
            </template>
          </el-upload>
        </el-card>

        <el-card class="box-card mb-20">
          <template #header>
            <div class="card-header">
              <span>基线规则任务 ({{ rules.length }})</span>
              <el-button type="primary" size="small" @click="refreshTemplates">刷新</el-button>
            </div>
          </template>
          
          <div v-if="rules.length === 0" class="empty-rules text-center">
            <el-empty description="暂无基线规则，请先上传模板文件" />
          </div>

          <div v-for="(rule, index) in rules" :key="rule.id" class="rule-card mb-20">
            <div class="rule-header">
              <div class="rule-title">
                <span class="font-bold">{{ rule.title || rule.name }}</span>
                <el-tag :type="getStatusType(rule.status)" class="ml-10" size="small">
                  {{ getStatusText(rule.status) }}
                </el-tag>
              </div>
              <div class="rule-actions">
                <el-select
                  v-model="rule.selectedHosts"
                  multiple
                  collapse-tags
                  collapse-tags-tooltip
                  placeholder="选择目标主机"
                  style="width: 260px"
                  :disabled="!canSelectHosts(rule.status)"
                  class="mr-10"
                >
                  <el-option
                    v-for="host in onlineHosts"
                    :key="host.id"
                    :label="`${host.hostname} (${host.ip_address})`"
                    :value="host.id"
                    :disabled="!host.is_online"
                  />
                </el-select>
                
                <el-button 
                  type="primary" 
                  :disabled="!canRunCheck(rule)"
                  :loading="rule.status === 'Checking'"
                  @click="confirmTask(rule, 'CHECK')"
                >
                  下发检查
                </el-button>
                <el-button 
                  type="warning" 
                  :disabled="!canRunFix(rule)"
                  :loading="rule.status === 'Fixing'"
                  @click="confirmTask(rule, 'FIX')"
                >
                  下发修复
                </el-button>
              </div>
            </div>
            
            <div class="rule-content mt-10" v-if="rule.status !== 'Uploading' && rule.status !== 'Parsing' && rule.status !== 'UploadFailed' && rule.status !== 'ParseFailed'">
              <el-descriptions :column="1" border size="small">
                <el-descriptions-item label="检查内容">{{ rule.check_content || rule.description }}</el-descriptions-item>
                <el-descriptions-item label="修复内容">{{ rule.fix_content || '-' }}</el-descriptions-item>
                <el-descriptions-item label="检查脚本" v-if="rule.generated_check_script">
                  <pre class="script-preview">{{ rule.generated_check_script }}</pre>
                </el-descriptions-item>
                <el-descriptions-item label="修复脚本" v-if="rule.generated_fix_script">
                  <pre class="script-preview">{{ rule.generated_fix_script }}</pre>
                </el-descriptions-item>
              </el-descriptions>
            </div>
            
            <div v-if="rule.status === 'Uploading' || rule.status === 'Parsing'" class="progress-container mt-10">
              <el-progress :percentage="rule.progress" :status="rule.status === 'Parsing' ? 'success' : ''" />
              <span class="progress-text">{{ rule.status === 'Uploading' ? '上传中...' : '大模型解析中...' }}</span>
            </div>
          </div>
        </el-card>
      </el-col>
      
      <el-col :span="8">
        <el-card class="box-card log-card">
          <template #header>
            <div class="card-header">
              <span>实时执行日志</span>
              <el-button size="small" @click="clearLogs">清空</el-button>
            </div>
          </template>
          <LogTerminal :logs="currentLogs" />
        </el-card>
      </el-col>
    </el-row>

    <el-dialog
      v-model="confirmDialogVisible"
      :title="confirmDialogType === 'CHECK' ? '确认下发检查' : '确认下发修复'"
      width="550px"
    >
      <div v-if="activeRuleToConfirm">
        <el-alert type="info" :closable="false" class="mb-10">
          <template #title>
            将向 {{ activeRuleToConfirm.selectedHosts.length }} 台主机下发任务
          </template>
        </el-alert>
        <ul class="host-list-preview">
          <li v-for="hostId in activeRuleToConfirm.selectedHosts" :key="hostId">
            {{ getHostDisplay(hostId) }}
          </li>
        </ul>
        <p class="mt-10">即将执行的脚本：</p>
        <div class="code-preview">
          <pre>{{ confirmDialogType === 'CHECK' ? activeRuleToConfirm.generated_check_script : activeRuleToConfirm.generated_fix_script }}</pre>
        </div>
      </div>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="confirmDialogVisible = false">取消</el-button>
          <el-button type="primary" @click="executeTask">
            确认执行
          </el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { UploadFilled } from '@element-plus/icons-vue';
import { useHostStore } from '@/store/hosts';
import { ElMessage } from 'element-plus';
import LogTerminal from '@/components/LogTerminal.vue';
import type { UploadFile } from 'element-plus';
import type { Host, BaselineRule } from '@/types';

const hostStore = useHostStore();

type RuleStatus = 'Ready' | 'Uploading' | 'UploadFailed' | 'Parsing' | 'ParseFailed' | 'RuleReady' | 'Checking' | 'CheckPassed_True' | 'CheckPassed_False' | 'CheckFailed' | 'Fixing' | 'Fixed' | 'FixFailed';

interface Rule extends BaselineRule {
  status: RuleStatus;
  progress: number;
  selectedHosts: string[];
  name?: string;
  description?: string;
}

const rules = ref<Rule[]>([]);
const currentLogs = ref<any[]>([]);

const confirmDialogVisible = ref(false);
const confirmDialogType = ref<'CHECK' | 'FIX'>('CHECK');
const activeRuleToConfirm = ref<Rule | null>(null);

const onlineHosts = computed(() => {
  return hostStore.hosts.filter((h: Host) => h.is_online);
});

const getHostDisplay = (hostId: string) => {
  const host = hostStore.hosts.find((h: Host) => h.id === hostId);
  return host ? `${host.hostname} (${host.ip_address})` : hostId;
};

const refreshTemplates = async () => {
  ElMessage.info('刷新模板列表');
};

onMounted(async () => {
  if (hostStore.hosts.length === 0) {
    await hostStore.fetchHosts(1, 100);
  }
});

const getStatusType = (status: RuleStatus) => {
  switch (status) {
    case 'CheckPassed_True':
    case 'Fixed':
    case 'RuleReady': return 'success';
    case 'UploadFailed':
    case 'ParseFailed':
    case 'CheckPassed_False':
    case 'CheckFailed':
    case 'FixFailed': return 'danger';
    case 'Uploading':
    case 'Parsing':
    case 'Checking':
    case 'Fixing': return 'warning';
    default: return 'info';
  }
};

const getStatusText = (status: RuleStatus) => {
  switch (status) {
    case 'Ready': return '准备就绪';
    case 'Uploading': return '上传中';
    case 'UploadFailed': return '上传失败';
    case 'Parsing': return '解析中';
    case 'ParseFailed': return '解析失败';
    case 'RuleReady': return '规则就绪';
    case 'Checking': return '检查中';
    case 'CheckPassed_True': return '合规';
    case 'CheckPassed_False': return '不合规';
    case 'CheckFailed': return '检查失败';
    case 'Fixing': return '修复中';
    case 'Fixed': return '已修复';
    case 'FixFailed': return '修复失败';
    default: return status;
  }
};

const canSelectHosts = (status: RuleStatus) => {
  return ['RuleReady', 'CheckPassed_True', 'CheckPassed_False', 'CheckFailed', 'Fixed', 'FixFailed'].includes(status);
};

const canRunCheck = (rule: Rule) => {
  return rule.selectedHosts.length > 0 && 
         rule.generated_check_script && 
         ['RuleReady', 'CheckPassed_True', 'CheckPassed_False', 'CheckFailed', 'Fixed', 'FixFailed'].includes(rule.status);
};

const canRunFix = (rule: Rule) => {
  return rule.selectedHosts.length > 0 && 
         rule.generated_fix_script && 
         ['CheckPassed_False', 'FixFailed'].includes(rule.status);
};

const handleFileChange = (file: UploadFile) => {
  const newRule: Rule = {
    id: Date.now().toString(),
    template_id: '',
    title: file.name,
    check_content: '',
    fix_content: '',
    generated_check_script: '',
    generated_fix_script: '',
    name: file.name,
    description: '解析中...',
    status: 'Uploading',
    progress: 0,
    selectedHosts: [],
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString()
  };
  rules.value.unshift(newRule);
  
  let upInterval = setInterval(() => {
    if (newRule.progress >= 100) {
      clearInterval(upInterval);
      newRule.status = 'Parsing';
      newRule.progress = 0;
      
      setTimeout(() => {
        newRule.status = 'RuleReady';
        newRule.title = '检查 /etc/passwd 文件权限';
        newRule.check_content = '确保 /etc/passwd 权限为 644';
        newRule.fix_content = '修复 /etc/passwd 权限为 644';
        newRule.generated_check_script = 'stat -c "%a" /etc/passwd | grep -q "644"';
        newRule.generated_fix_script = 'chmod 644 /etc/passwd';
        ElMessage.success('基线模板解析成功');
      }, 2000);
    } else {
      newRule.progress += 20;
    }
  }, 300);
};

const confirmTask = (rule: Rule, type: 'CHECK' | 'FIX') => {
  activeRuleToConfirm.value = rule;
  confirmDialogType.value = type;
  confirmDialogVisible.value = true;
};

const addLog = (stream: 'stdout' | 'stderr', line: string, hostIp: string) => {
  currentLogs.value.push({
    taskId: 'task-' + Date.now(),
    stream,
    line,
    timestamp: new Date().toISOString(),
    hostIp
  });
};

const clearLogs = () => {
  currentLogs.value = [];
};

const executeTask = () => {
  if (!activeRuleToConfirm.value) return;
  const rule = activeRuleToConfirm.value;
  const type = confirmDialogType.value;
  confirmDialogVisible.value = false;
  
  rule.status = type === 'CHECK' ? 'Checking' : 'Fixing';
  
  const hostInfo = getHostDisplay(rule.selectedHosts[0] || '');
  const hostIp: string = hostInfo.split(' (')[1]?.replace(')', '') || 'System';
  
  addLog('stdout', `[系统] 开始执行${type === 'CHECK' ? '基线检查' : '自动修复'}...`, hostIp);
  
  setTimeout(() => {
    const script = type === 'CHECK' ? rule.generated_check_script : rule.generated_fix_script;
    addLog('stdout', `[${hostIp}] 执行脚本: ${script}`, hostIp);
  }, 500);

  setTimeout(() => {
    if (type === 'CHECK') {
      const isCompliant = Math.random() > 0.5;
      if (isCompliant) {
        addLog('stdout', `[${hostIp}] 脚本执行完成，退出码: 0`, hostIp);
        rule.status = 'CheckPassed_True';
        addLog('stdout', '[系统] 检查结果: 合规', hostIp);
      } else {
        addLog('stderr', `[${hostIp}] 脚本执行完成，退出码: 1`, hostIp);
        rule.status = 'CheckPassed_False';
        addLog('stderr', '[系统] 检查结果: 不合规，可下发修复', hostIp);
      }
    } else {
      addLog('stdout', `[${hostIp}] 脚本执行完成，退出码: 0`, hostIp);
      rule.status = 'Fixed';
      addLog('stdout', '[系统] 修复成功', hostIp);
    }
  }, 2500);
};
</script>

<style scoped>
.workbench-page {
  height: 100%;
}
.mb-10 {
  margin-bottom: 10px;
}
.mb-20 {
  margin-bottom: 20px;
}
.mt-10 {
  margin-top: 10px;
}
.ml-10 {
  margin-left: 10px;
}
.mr-10 {
  margin-right: 10px;
}
.font-bold {
  font-weight: bold;
}
.text-center {
  text-align: center;
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.rule-card {
  border: 1px solid #ebeef5;
  border-radius: 4px;
  padding: 15px;
  background-color: #fafafa;
}
.rule-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.rule-title {
  display: flex;
  align-items: center;
}
.rule-actions {
  display: flex;
  align-items: center;
}
.script-preview {
  margin: 0;
  padding: 8px;
  background-color: #f5f7fa;
  border-radius: 4px;
  font-family: monospace;
  white-space: pre-wrap;
  word-wrap: break-word;
  max-height: 150px;
  overflow-y: auto;
}
.progress-container {
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.progress-text {
  font-size: 12px;
  color: #909399;
}
.log-card {
  height: calc(100vh - 120px);
  display: flex;
  flex-direction: column;
}
:deep(.log-card .el-card__body) {
  flex: 1;
  padding: 0;
  display: flex;
  flex-direction: column;
}
:deep(.log-terminal) {
  flex: 1;
  height: 100%;
  border-radius: 0 0 4px 4px;
}
.host-list-preview {
  margin: 10px 0;
  padding-left: 20px;
  max-height: 120px;
  overflow-y: auto;
}
.code-preview {
  background-color: #282c34;
  color: #abb2bf;
  padding: 12px;
  border-radius: 4px;
  max-height: 200px;
  overflow-y: auto;
}
.code-preview pre {
  margin: 0;
  font-family: Consolas, Monaco, monospace;
  font-size: 13px;
}
</style>