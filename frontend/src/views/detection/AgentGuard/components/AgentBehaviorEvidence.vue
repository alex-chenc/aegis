<template>
  <section class="behavior-evidence">
    <header>
      <div>
        <h3>{{ behavior.category || '-' }} · {{ behavior.operation || '-' }}</h3>
        <p>{{ behavior.event_id || behavior.id }} · {{ behavior.occurred_at || '-' }}</p>
      </div>
      <el-tag effect="plain">{{ behavior.outcome || 'unknown' }}</el-tag>
    </header>

    <el-alert
      v-if="hasGap"
      type="warning"
      :closable="false"
      :title="t('agentGuard.panorama.incompleteEvidence')"
      show-icon
    />

    <div class="evidence-sections">
      <article>
        <h4>Actor</h4>
        <dl>
          <div v-for="field in actorFields" :key="field.label">
            <dt>{{ field.label }}</dt>
            <dd>{{ field.value }}</dd>
          </div>
        </dl>
      </article>
      <article>
        <h4>Resource</h4>
        <dl>
          <div v-for="field in resourceFields" :key="field.label">
            <dt>{{ field.label }}</dt>
            <dd>{{ field.value }}</dd>
          </div>
        </dl>
      </article>
      <article>
        <h4>Collection</h4>
        <dl>
          <div v-for="field in collectionFields" :key="field.label">
            <dt>{{ field.label }}</dt>
            <dd>{{ field.value }}</dd>
          </div>
        </dl>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AgentBehaviorIndex } from '@/types/agentGuard'

const props = defineProps<{ behavior: AgentBehaviorIndex }>()
const { t } = useI18n()

const actor = computed<Record<string, unknown>>(() => ({
  pid: props.behavior.pid,
  ppid: props.behavior.ppid,
  start_ticks: props.behavior.process_start_ticks,
  exe: props.behavior.process_exe,
  argv: props.behavior.command_argv,
  cwd: props.behavior.command_cwd,
  visibility: props.behavior.command_visibility,
  ...(props.behavior.actor || {}),
}))
const resource = computed(() => props.behavior.resource || {})
const collection = computed(() => props.behavior.collection || {})

const actorFields = computed(() => compactFields([
  ['PID', actor.value.pid],
  ['PPID', actor.value.ppid],
  ['start_ticks', actor.value.start_ticks],
  ['exe', actor.value.exe],
  ['argv', formatArgv(actor.value.argv)],
  ['cwd', actor.value.cwd],
  ['UID', actor.value.uid],
  ['GID', actor.value.gid],
]))

const resourceFields = computed(() => compactFields([
  ['type', resource.value.type],
  ['classification', resource.value.classification],
  ['identity', resource.value.identity],
  ['resolved_path', resource.value.resolved_path],
  ['outcome', resource.value.outcome],
  ['errno', resource.value.errno],
]))

const collectionFields = computed(() => compactFields([
  ['sensor', collection.value.sensor],
  ['visibility', collection.value.visibility],
  ['truncated_fields', formatList(collection.value.truncated_fields)],
  ['lost_events_since_last', collection.value.lost_events_since_last],
  ['aggregated_count', collection.value.aggregated_count],
]))

const hasGap = computed(() =>
  collection.value.visibility === 'partial' ||
  collection.value.visibility === 'unobservable' ||
  Number(collection.value.lost_events_since_last || 0) > 0 ||
  (Array.isArray(collection.value.truncated_fields) && collection.value.truncated_fields.length > 0),
)

function compactFields(values: Array<[string, unknown]>) {
  return values
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .map(([label, value]) => ({ label, value: String(value) }))
}

function formatArgv(value: unknown) {
  return Array.isArray(value) ? value.map(String).join(' ') : value
}

function formatList(value: unknown) {
  return Array.isArray(value) ? value.map(String).join(' · ') : value
}
</script>

<style scoped>
.behavior-evidence {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 14px;
  padding: 14px;
  border: 1px solid var(--aegis-border);
  border-radius: 14px;
  background: #fff;
}

.behavior-evidence > header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.behavior-evidence h3,
.behavior-evidence p,
.behavior-evidence h4 {
  margin: 0;
}

.behavior-evidence p {
  color: var(--aegis-text-muted);
  font-size: 12px;
}

.evidence-sections {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.evidence-sections article {
  padding: 12px;
  border: 1px solid var(--aegis-border);
  border-radius: 10px;
}

.evidence-sections dl > div {
  display: grid;
  grid-template-columns: 100px minmax(0, 1fr);
  gap: 8px;
  padding-top: 8px;
}

.evidence-sections dt {
  color: var(--aegis-text-muted);
}

.evidence-sections dd {
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
}

@media (max-width: 1100px) {
  .evidence-sections {
    grid-template-columns: 1fr;
  }
}
</style>
