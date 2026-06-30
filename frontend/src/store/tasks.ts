import { defineStore } from 'pinia'
import { runCheck, runFix, getTaskLogs, type TaskLog, type RunTaskResponse } from '@/api/tasks'
import type { BaselineRule } from '@/types'

interface TaskState {
  selectedRules: BaselineRule[]
  selectedHostIds: string[]
  currentTaskGroupId: string | null
  taskLogs: TaskLog[]
  loading: boolean
}

export const useTaskStore = defineStore('tasks', {
  state: (): TaskState => ({
    selectedRules: [],
    selectedHostIds: [],
    currentTaskGroupId: null,
    taskLogs: [],
    loading: false
  }),

  getters: {
    selectedRuleIds: (state) => state.selectedRules.map(r => r.id),
    hasSelection: (state) => state.selectedRules.length > 0 && state.selectedHostIds.length > 0
  },

  actions: {
    setSelectedRules(rules: BaselineRule[]) {
      this.selectedRules = rules
    },

    toggleRule(rule: BaselineRule) {
      const index = this.selectedRules.findIndex(r => r.id === rule.id)
      if (index > -1) {
        this.selectedRules.splice(index, 1)
      } else {
        this.selectedRules.push(rule)
      }
    },

    setSelectedHosts(hostIds: string[]) {
      this.selectedHostIds = hostIds
    },

    clearSelection() {
      this.selectedRules = []
      this.selectedHostIds = []
    },

    async executeCheck(maxRounds = 1): Promise<RunTaskResponse | null> {
      if (!this.hasSelection) return null
      
      this.loading = true
      try {
        const result = await runCheck({
          rule_ids: this.selectedRuleIds,
          host_ids: this.selectedHostIds,
          max_rounds: maxRounds
        })
        this.currentTaskGroupId = result.task_group_id
        return result
      } finally {
        this.loading = false
      }
    },

    async executeFix(maxRounds = 1): Promise<RunTaskResponse | null> {
      if (!this.hasSelection) return null
      
      this.loading = true
      try {
        const result = await runFix({
          rule_ids: this.selectedRuleIds,
          host_ids: this.selectedHostIds,
          max_rounds: maxRounds
        })
        this.currentTaskGroupId = result.task_group_id
        return result
      } finally {
        this.loading = false
      }
    },

    async fetchTaskLogs(taskGroupId: string) {
      this.loading = true
      try {
        this.taskLogs = await getTaskLogs(taskGroupId)
      } finally {
        this.loading = false
      }
    }
  }
})
