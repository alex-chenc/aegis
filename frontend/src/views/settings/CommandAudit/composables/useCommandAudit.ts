import { ref } from 'vue'
import { commandAuditApi, type CommandAuditRule, type CommandAuditSettings, type RuleListParams, type CreateRulePayload, type TestPatternPayload } from '@/api/command-audit'

export function useCommandAudit() {
  const rules = ref<CommandAuditRule[]>([])
  const total = ref(0)
  const loading = ref(false)
  const settings = ref<CommandAuditSettings | null>(null)
  const queryParams = ref<RuleListParams>({ page: 1, page_size: 20 })

  const fetchRules = async (params?: RuleListParams) => {
    if (params) queryParams.value = { ...queryParams.value, ...params }
    loading.value = true
    try {
      const res = await commandAuditApi.getRules(queryParams.value)
      rules.value = res.rules || []
      total.value = res.total || 0
    } finally {
      loading.value = false
    }
  }

  const fetchSettings = async () => {
    settings.value = await commandAuditApi.getSettings()
  }

  const createRule = async (data: CreateRulePayload) => {
    await commandAuditApi.createRule(data)
    await fetchRules()
  }

  const updateRule = async (id: string, data: Partial<CreateRulePayload & { is_enabled: boolean }>) => {
    await commandAuditApi.updateRule(id, data)
    await fetchRules()
  }

  const deleteRule = async (id: string) => {
    await commandAuditApi.deleteRule(id)
    await fetchRules()
  }

  const toggleRule = async (id: string) => {
    await commandAuditApi.toggleRule(id)
    await fetchRules()
  }

  const testPattern = async (data: TestPatternPayload) => {
    return await commandAuditApi.testPattern(data)
  }

  const updateSettings = async (data: Partial<CommandAuditSettings>) => {
    await commandAuditApi.updateSettings(data)
    await fetchSettings()
  }

  return {
    rules,
    total,
    loading,
    settings,
    queryParams,
    fetchRules,
    fetchSettings,
    createRule,
    updateRule,
    deleteRule,
    toggleRule,
    testPattern,
    updateSettings
  }
}
