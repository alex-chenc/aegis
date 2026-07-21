<template>
  <el-table v-if="rows.length > 0" :data="rows" border size="small">
    <el-table-column prop="eventId" label="Event ID" width="110" />
    <el-table-column prop="eventName" label="Event Type" min-width="180" show-overflow-tooltip />
    <el-table-column prop="fieldId" :label="$t('generated.detectionDetectionPackagesEventSchemaTable_field_id_204aa4')" width="100" />
    <el-table-column prop="fieldName" :label="$t('generated.detectionDetectionPackagesEventSchemaTable_field_name_074a5e')" min-width="160" show-overflow-tooltip />
    <el-table-column prop="fieldType" :label="$t('generated.common_type_e4e46c')" width="120" />
  </el-table>
  <el-empty v-else :description="$t('generated.detectionDetectionPackagesEventSchemaTable_no_event_schema_yet_4631ec')" />
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  schema?: Record<string, unknown> | null
  schemaJson?: string
}>()

interface EventSchemaRow {
  eventId: string
  eventName: string
  fieldId: string
  fieldName: string
  fieldType: string
}

const rows = computed(() => flattenEventSchema(readEventSchema()))

function readEventSchema(): Record<string, unknown> | undefined {
  if (props.schema && Object.keys(props.schema).length > 0) {
    return props.schema
  }
  if (!props.schemaJson) return undefined
  try {
    return JSON.parse(props.schemaJson) as Record<string, unknown>
  } catch {
    return undefined
  }
}

function flattenEventSchema(schema?: Record<string, unknown>): EventSchemaRow[] {
  const events = (schema?.events || schema) as Record<string, any> | undefined
  if (!events || typeof events !== 'object') return []

  return Object.entries(events).flatMap(([eventId, eventValue]) => {
    const eventInfo = eventValue as Record<string, any>
    const fields = eventInfo?.fields as Record<string, any> | undefined
    if (!fields || typeof fields !== 'object') {
      return [{
        eventId,
        eventName: String(eventInfo?.name || eventId),
        fieldId: '-',
        fieldName: '-',
        fieldType: '-',
      }]
    }
    return Object.entries(fields).map(([fieldId, fieldValue]) => {
      const fieldInfo = fieldValue as Record<string, any>
      return {
        eventId,
        eventName: String(eventInfo?.name || eventId),
        fieldId,
        fieldName: String(fieldInfo?.name || fieldId),
        fieldType: String(fieldInfo?.type || '-'),
      }
    })
  })
}
</script>
