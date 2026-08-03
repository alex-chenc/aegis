import { defineStore } from 'pinia'
import {
  createAgentGuardPolicy,
  freezeAgentExecutionUnit,
  getAgentGuardAction,
  analyzeAgentSecurityFinding,
  getAgentBehavior,
  getAgentExecutionUnit,
  getAgentGuardOverview,
  getAgentPanorama,
  getAgentPanoramaChildren,
  getAgentSecurityFinding,
  killAgentExecutionUnit,
  listAgentSecurityFindingAnalyses,
  listAgentGuardAgents,
  listAgentExecutionUnits,
  listAgentExecutionUnitTimeline,
  listAgentGuardInstances,
  listAgentGuardPolicies,
  listAgentGuardPolicyDeliveries,
  listAgentSecurityFindings,
  listBuiltinAgentBehaviorRules,
  publishAgentGuardPolicy,
  resumeAgentExecutionUnit,
  validateAgentGuardPolicy,
} from '@/api/agentGuard'
import type {
  AgentBehaviorIndex,
  AgentGuardAction,
  AgentGuardActionName,
  AgentGuardActionRequest,
  AgentExecutionUnit,
  AgentGuardAgentQuery,
  AgentGuardAgentSummary,
  AgentGuardFindingQuery,
  AgentGuardInstanceQuery,
  AgentGuardOverview,
  AgentGuardPanoramaQuery,
  AgentGuardPolicy,
  AgentGuardPolicyDelivery,
  AgentGuardPolicyDraftRequest,
  AgentGuardPolicyValidation,
  AgentSecurityAnalysisRun,
  AgentRuntimeInstance,
  AgentSecurityFindingSummary,
  BuiltinAgentBehaviorRuleSummary,
  PanoramaTreeNode,
} from '@/types/agentGuard'

interface AgentGuardLoadingState {
  overview: boolean
  agents: boolean
  instances: boolean
  panorama: boolean
  analysis: boolean
  policies: boolean
  actions: boolean
}

interface AgentGuardErrorState {
  overview: string
  agents: string
  instances: string
  panorama: string
  analysis: string
  policies: string
  actions: string
}

function initialLoading(): AgentGuardLoadingState {
  return {
    overview: false,
    agents: false,
    instances: false,
    panorama: false,
    analysis: false,
    policies: false,
    actions: false,
  }
}

function initialErrors(): AgentGuardErrorState {
  return {
    overview: '',
    agents: '',
    instances: '',
    panorama: '',
    analysis: '',
    policies: '',
    actions: '',
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : 'request_failed'
}

export const useAgentGuardStore = defineStore('agentGuard', {
  state: () => ({
    overview: null as AgentGuardOverview | null,
    agents: [] as AgentGuardAgentSummary[],
    agentTotal: 0,
    instances: [] as AgentRuntimeInstance[],
    instanceTotal: 0,
    panoramaNodes: [] as PanoramaTreeNode[],
    selectedPanoramaNode: null as PanoramaTreeNode | null,
    executionUnits: [] as AgentExecutionUnit[],
    selectedExecutionUnit: null as AgentExecutionUnit | null,
    builtinRules: [] as BuiltinAgentBehaviorRuleSummary[],
    findings: [] as AgentSecurityFindingSummary[],
    findingTotal: 0,
    selectedFinding: null as AgentSecurityFindingSummary | null,
    analyses: [] as AgentSecurityAnalysisRun[],
    selectedBehavior: null as AgentBehaviorIndex | null,
    policies: [] as AgentGuardPolicy[],
    policyTotal: 0,
    deliveries: [] as AgentGuardPolicyDelivery[],
    actions: [] as AgentGuardAction[],
    policyValidation: null as AgentGuardPolicyValidation | null,
    loading: initialLoading(),
    errors: initialErrors(),
    lastUpdatedAt: '',
  }),

  actions: {
    async fetchPage(params: AgentGuardAgentQuery) {
      await Promise.all([
        this.fetchOverview(params),
        this.fetchAgents(params),
      ])
    },

    async fetchOverview(params: AgentGuardAgentQuery) {
      this.loading.overview = true
      this.errors.overview = ''
      try {
        this.overview = await getAgentGuardOverview({
          host_ids: params.host_ids,
          agent_types: params.agent_types,
        })
      } catch (error) {
        this.errors.overview = errorMessage(error)
      } finally {
        this.loading.overview = false
      }
    },

    async fetchAgents(params: AgentGuardAgentQuery) {
      this.loading.agents = true
      this.errors.agents = ''
      try {
        const result = await listAgentGuardAgents(params)
        this.agents = result.items || []
        this.agentTotal = result.total || 0
        this.lastUpdatedAt = new Date().toISOString()
      } catch (error) {
        this.errors.agents = errorMessage(error)
      } finally {
        this.loading.agents = false
      }
    },

    async fetchInstances(params: AgentGuardInstanceQuery) {
      this.loading.instances = true
      this.errors.instances = ''
      try {
        const result = await listAgentGuardInstances(params)
        this.instances = result.items || []
        this.instanceTotal = result.total || 0
      } catch (error) {
        this.errors.instances = errorMessage(error)
      } finally {
        this.loading.instances = false
      }
    },

    async fetchPanorama(params: AgentGuardPanoramaQuery) {
      this.loading.panorama = true
      this.errors.panorama = ''
      try {
        const result = await getAgentPanorama(params)
        this.panoramaNodes = result.items || result.nodes || (result.root ? [result.root] : [])
      } catch (error) {
        this.errors.panorama = errorMessage(error)
      } finally {
        this.loading.panorama = false
      }
    },

    async fetchPanoramaChildren(nodeId: string) {
      const result = await getAgentPanoramaChildren(nodeId)
      return result.items || []
    },

    async fetchExecutionUnit(id: string) {
      this.loading.panorama = true
      this.errors.panorama = ''
      try {
        this.selectedExecutionUnit = await getAgentExecutionUnit(id)
        return this.selectedExecutionUnit
      } catch (error) {
        this.errors.panorama = errorMessage(error)
        return null
      } finally {
        this.loading.panorama = false
      }
    },

    async fetchExecutionUnits(instanceId: string) {
      this.loading.panorama = true
      this.errors.panorama = ''
      try {
        const result = await listAgentExecutionUnits({
          instance_id: instanceId,
          page: 1,
          page_size: 100,
        })
        this.executionUnits = result.items || []
        this.selectedExecutionUnit = this.executionUnits[0] || null
        return this.executionUnits
      } catch (error) {
        this.errors.panorama = errorMessage(error)
        return []
      } finally {
        this.loading.panorama = false
      }
    },

    async fetchExecutionUnitTimeline(unitId: string) {
      this.loading.actions = true
      this.errors.actions = ''
      try {
        const result = await listAgentExecutionUnitTimeline(unitId)
        this.actions = result.items || []
        return this.actions
      } catch (error) {
        this.errors.actions = errorMessage(error)
        return []
      } finally {
        this.loading.actions = false
      }
    },

    async executeUnitAction(
      unitId: string,
      action: Extract<AgentGuardActionName,
        'freeze_execution_unit' | 'resume_execution_unit' | 'kill_execution_unit'>,
      payload: AgentGuardActionRequest,
    ) {
      this.loading.actions = true
      this.errors.actions = ''
      try {
        const accepted = action === 'freeze_execution_unit'
          ? await freezeAgentExecutionUnit(unitId, payload)
          : action === 'resume_execution_unit'
            ? await resumeAgentExecutionUnit(unitId, payload)
            : await killAgentExecutionUnit(unitId, payload)
        const pending: AgentGuardAction = {
          id: accepted.action_id,
          command_id: accepted.command_id,
          execution_unit_id: unitId,
          action,
          status: accepted.status,
          reason: payload.reason,
          hold_requested: payload.hold,
          requested_at: new Date().toISOString(),
        }
        this.actions = [pending, ...this.actions.filter(item => item.id !== pending.id)]
        return accepted
      } catch (error) {
        this.errors.actions = errorMessage(error)
        throw error
      } finally {
        this.loading.actions = false
      }
    },

    async refreshAction(actionId: string) {
      try {
        const action = await getAgentGuardAction(actionId)
        const index = this.actions.findIndex(item => item.id === action.id)
        if (index >= 0) this.actions[index] = action
        else this.actions = [action, ...this.actions]
        return action
      } catch (error) {
        this.errors.actions = errorMessage(error)
        return null
      }
    },

    selectPanoramaNode(node: PanoramaTreeNode) {
      this.selectedPanoramaNode = node
    },

    async fetchBuiltinRules() {
      try {
        const result = await listBuiltinAgentBehaviorRules()
        this.builtinRules = result.items || []
      } catch (error) {
        this.errors.analysis = errorMessage(error)
      }
    },

    async fetchFindings(params: AgentGuardFindingQuery) {
      this.loading.analysis = true
      this.errors.analysis = ''
      try {
        const result = await listAgentSecurityFindings(params)
        this.findings = result.items || []
        this.findingTotal = result.total || 0
      } catch (error) {
        this.errors.analysis = errorMessage(error)
      } finally {
        this.loading.analysis = false
      }
    },

    async fetchFinding(id: string) {
      this.loading.analysis = true
      this.errors.analysis = ''
      try {
        const result = await getAgentSecurityFinding(id)
        this.selectedFinding = result || null
        if (result) {
          const index = this.findings.findIndex(item => item.id === result.id)
          if (index >= 0) this.findings[index] = result
          else this.findings = [result]
        }
        this.findingTotal = Math.max(this.findingTotal, result ? 1 : 0)
        if (result) await this.fetchFindingAnalyses(result.id)
      } catch (error) {
        this.errors.analysis = errorMessage(error)
      } finally {
        this.loading.analysis = false
      }
    },

    async fetchFindingAnalyses(id: string) {
      try {
        const result = await listAgentSecurityFindingAnalyses(id)
        this.analyses = result.items || []
      } catch (error) {
        this.errors.analysis = errorMessage(error)
      }
    },

    async analyzeFinding(id: string) {
      this.loading.analysis = true
      this.errors.analysis = ''
      try {
        const run = await analyzeAgentSecurityFinding(id)
        this.analyses = [run, ...this.analyses.filter(item => item.id !== run.id)]
        return run
      } catch (error) {
        this.errors.analysis = errorMessage(error)
        throw error
      } finally {
        this.loading.analysis = false
      }
    },

    async fetchBehavior(id: string) {
      this.loading.analysis = true
      this.errors.analysis = ''
      try {
        this.selectedBehavior = await getAgentBehavior(id)
      } catch (error) {
        this.errors.analysis = errorMessage(error)
      } finally {
        this.loading.analysis = false
      }
    },

    async fetchPolicies() {
      this.loading.policies = true
      this.errors.policies = ''
      try {
        const result = await listAgentGuardPolicies({ page: 1, page_size: 100 })
        this.policies = result.items || []
        this.policyTotal = result.total || 0
      } catch (error) {
        this.errors.policies = errorMessage(error)
      } finally {
        this.loading.policies = false
      }
    },

    async createPolicy(payload: AgentGuardPolicyDraftRequest) {
      this.loading.policies = true
      this.errors.policies = ''
      try {
        const result = await createAgentGuardPolicy(payload)
        this.policyValidation = result.validation
        await this.fetchPolicies()
        return result.policy
      } catch (error) {
        this.errors.policies = errorMessage(error)
        throw error
      } finally {
        this.loading.policies = false
      }
    },

    async validatePolicy(id: string, payload: AgentGuardPolicyDraftRequest) {
      this.loading.policies = true
      this.errors.policies = ''
      try {
        this.policyValidation = await validateAgentGuardPolicy(id, payload)
        return this.policyValidation
      } catch (error) {
        this.errors.policies = errorMessage(error)
        throw error
      } finally {
        this.loading.policies = false
      }
    },

    async publishPolicy(id: string, reason: string) {
      this.loading.policies = true
      this.errors.policies = ''
      try {
        const result = await publishAgentGuardPolicy(id, reason)
        this.deliveries = result.deliveries || []
        await this.fetchPolicies()
        return result
      } catch (error) {
        this.errors.policies = errorMessage(error)
        throw error
      } finally {
        this.loading.policies = false
      }
    },

    async fetchPolicyDeliveries(id: string) {
      this.loading.policies = true
      this.errors.policies = ''
      try {
        const result = await listAgentGuardPolicyDeliveries(id, { page: 1, page_size: 100 })
        this.deliveries = result.items || []
      } catch (error) {
        this.errors.policies = errorMessage(error)
      } finally {
        this.loading.policies = false
      }
    },

    resetDetail() {
      this.instances = []
      this.instanceTotal = 0
      this.panoramaNodes = []
      this.selectedPanoramaNode = null
      this.executionUnits = []
      this.selectedExecutionUnit = null
      this.builtinRules = []
      this.findings = []
      this.findingTotal = 0
      this.selectedFinding = null
      this.analyses = []
      this.selectedBehavior = null
      this.actions = []
      this.errors.instances = ''
      this.errors.panorama = ''
      this.errors.analysis = ''
      this.errors.actions = ''
      this.policyValidation = null
    },
  },
})
