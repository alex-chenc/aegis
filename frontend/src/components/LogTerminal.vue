<template>
  <div class="log-terminal" ref="terminalRef">
    <div 
      v-for="(log, index) in logs" 
      :key="index" 
      class="log-line"
      :class="log.stream === 'stderr' ? 'stderr' : 'stdout'"
    >
      <span class="timestamp">[{{ formatTime(log.timestamp) }}]</span>
      <span class="host-ip">[{{ log.hostIp || 'System' }}]</span>
      <span class="message">{{ log.line }}</span>
    </div>
    <div v-if="logs.length === 0" class="empty-state">
      Waiting for logs...
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, onMounted } from 'vue';

interface LogEntry {
  taskId: string;
  stream: 'stdout' | 'stderr';
  line: string;
  timestamp: string;
  hostIp?: string;
}

const props = defineProps<{
  logs: LogEntry[];
  autoScroll?: boolean;
}>();

const terminalRef = ref<HTMLElement | null>(null);

const formatTime = (iso: string) => {
  if (!iso) return '';
  const date = new Date(iso);
  return date.toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' }) + '.' + date.getMilliseconds().toString().padStart(3, '0');
};

const scrollToBottom = () => {
  if (props.autoScroll !== false && terminalRef.value) {
    terminalRef.value.scrollTop = terminalRef.value.scrollHeight;
  }
};

watch(() => props.logs, () => {
  nextTick(() => {
    scrollToBottom();
  });
}, { deep: true });

onMounted(() => {
  scrollToBottom();
});
</script>

<style scoped>
.log-terminal {
  background-color: #1e1e1e;
  color: #d4d4d4;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  padding: 10px;
  height: 300px;
  overflow-y: auto;
  border-radius: 4px;
  box-shadow: inset 0 0 10px rgba(0,0,0,0.5);
}

.log-line {
  line-height: 1.5;
  word-break: break-all;
  white-space: pre-wrap;
}

.timestamp {
  color: #569cd6;
  margin-right: 8px;
}

.host-ip {
  color: #4ec9b0;
  margin-right: 8px;
}

.stderr .message {
  color: #f44747;
}

.stdout .message {
  color: #d4d4d4;
}

.empty-state {
  color: #808080;
  font-style: italic;
  text-align: center;
  margin-top: 100px;
}

/* Custom scrollbar for terminal */
.log-terminal::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
.log-terminal::-webkit-scrollbar-track {
  background: #1e1e1e;
}
.log-terminal::-webkit-scrollbar-thumb {
  background: #424242;
  border-radius: 4px;
}
.log-terminal::-webkit-scrollbar-thumb:hover {
  background: #4f4f4f;
}
</style>