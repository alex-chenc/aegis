import { defineStore } from 'pinia'
import * as api from '@/api/detection'
import type { Alert, BlockPolicy, SigmaRule, ThreatStatistics, AlertTrendPoint, AttackMatrix } from '@/types'

export const useDetectionStore = defineStore('detection', {
  state: () => ({
    alerts: [] as Alert[],
    alertTotal: 0,
    alertLoading: false,
    blockPolicies: [] as BlockPolicy[],
    policyLoading: false,
    rules: [] as SigmaRule[],
    ruleTotal: 0,
    ruleLoading: false,
    threatStats: null as ThreatStatistics | null,
    alertTrend: [] as AlertTrendPoint[],
    attackMatrix: null as AttackMatrix | null
  }),
  actions: {
    async fetchAlerts(params: any = {}) {
      this.alertLoading = true
      try {
        const r = await api.getAlerts(params)
        this.alerts = r.data || []
        this.alertTotal = r.total || 0
      } finally {
        this.alertLoading = false
      }
    },
    async fetchBlockPolicies() {
      this.policyLoading = true
      try {
        this.blockPolicies = await api.getBlockPolicies()
      } finally {
        this.policyLoading = false
      }
    },
    async updateBlockPolicy(mitreId: string, data: any) {
      await api.updateBlockPolicy(mitreId, data)
      const p = this.blockPolicies.find(x => x.mitre_id === mitreId)
      if (p) {
        if (data.enabled !== undefined) p.enabled = data.enabled
        if (data.auto_block !== undefined) p.auto_block = data.auto_block
        if (data.auto_dispose !== undefined) p.auto_dispose = data.auto_dispose
        if (data.action !== undefined) p.action = data.action
      }
    },
    async fetchRules(params: any = {}) {
      this.ruleLoading = true
      try {
        const r = await api.getRules(params)
        this.rules = r.data || []
        this.ruleTotal = r.total || 0
      } finally {
        this.ruleLoading = false
      }
    },
    async updateRuleStatus(ruleId: string, status: 'active' | 'disabled') {
      await api.updateRuleStatus(ruleId, status)
      const r = this.rules.find(x => x.rule_id === ruleId)
      if (r) r.status = status
    },
    async fetchThreatStatistics() {
      this.threatStats = await api.getThreatStatistics()
    },
    async fetchAlertTrend(hours: number = 24) {
      this.alertTrend = await api.getAlertTrend(hours)
    },
    async fetchAttackMatrix() {
      this.attackMatrix = await api.getAttackMatrix()
    }
  }
})
