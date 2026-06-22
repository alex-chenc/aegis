import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import {
  analyzeAssetApplications,
  createWeakPasswordTask,
  deleteWeakPasswordTask,
  generateWeakPasswordDictionary,
  getDefaultWeakPasswordDictionary,
  getWeakPasswordTask,
  getWeakPasswordTaskProgress,
  listAssetApplications,
  listWeakPasswordDictionaries,
  listWeakPasswordFindings,
  listWeakPasswordTaskHosts,
  listWeakPasswordTasks,
  retryWeakPasswordFailed,
} from '@/api/weakPassword'
import type {
  AIGenerateDictionaryRequest,
  AnalyzeAssetApplicationsRequest,
  AnalyzeAssetApplicationsResponse,
  CreateWeakPasswordTaskRequest,
  WeakPasswordCandidateApplication,
  WeakPasswordCollectionError,
  WeakPasswordDictionary,
  WeakPasswordFinding,
  WeakPasswordScanHost,
  WeakPasswordTask,
  WeakPasswordTaskProgress,
} from '@/types/weakPassword'

export const useWeakPasswordStore = defineStore('weakPassword', () => {
  const candidates = ref<WeakPasswordCandidateApplication[]>([])
  const candidateTotal = ref(0)
  const tasks = ref<WeakPasswordTask[]>([])
  const taskTotal = ref(0)
  const currentTask = ref<WeakPasswordTask | null>(null)
  const progress = ref<WeakPasswordTaskProgress | null>(null)
  const hosts = ref<WeakPasswordScanHost[]>([])
  const findings = ref<WeakPasswordFinding[]>([])
  const errors = ref<WeakPasswordCollectionError[]>([])
  const dictionaries = ref<WeakPasswordDictionary[]>([])
  const defaultDictionary = ref<WeakPasswordDictionary | null>(null)
  const analysisResult = ref<AnalyzeAssetApplicationsResponse | null>(null)
  const loading = ref(false)
  const analyzing = ref(false)
  const creatingTask = ref(false)
  const dictionaryLoading = ref(false)

  const candidateFilters = reactive({
    page: 1,
    page_size: 20,
    analysis_id: '',
    host_id: '',
    application_type: '',
    confidence: ''
  })

  const taskFilters = reactive({
    page: 1,
    page_size: 20,
    status: ''
  })

  async function analyze(payload: AnalyzeAssetApplicationsRequest) {
    analyzing.value = true
    try {
      const result = await analyzeAssetApplications(payload)
      analysisResult.value = result
      candidates.value = result.candidates || []
      candidateTotal.value = result.candidate_count || 0
      candidateFilters.analysis_id = result.analysis_id || ''
      return result
    } finally {
      analyzing.value = false
    }
  }

  async function fetchCandidates() {
    loading.value = true
    try {
      const result = await listAssetApplications(candidateFilters)
      candidates.value = result.items
      candidateTotal.value = result.total
    } finally {
      loading.value = false
    }
  }

  async function createTask(payload: CreateWeakPasswordTaskRequest) {
    creatingTask.value = true
    try {
      const result = await createWeakPasswordTask(payload)
      await fetchTasks()
      return result
    } finally {
      creatingTask.value = false
    }
  }

  async function fetchTasks() {
    loading.value = true
    try {
      const result = await listWeakPasswordTasks(taskFilters)
      tasks.value = result.items
      taskTotal.value = result.total
    } finally {
      loading.value = false
    }
  }

  async function fetchTaskDetail(taskId: string) {
    loading.value = true
    try {
      const [taskDetail, taskProgress, hostResult, findingResult] = await Promise.all([
        getWeakPasswordTask(taskId),
        getWeakPasswordTaskProgress(taskId),
        listWeakPasswordTaskHosts(taskId),
        listWeakPasswordFindings(taskId),
      ])
      currentTask.value = taskDetail.task
      errors.value = taskDetail.errors || []
      progress.value = taskProgress
      hosts.value = hostResult.items
      findings.value = findingResult.items
    } finally {
      loading.value = false
    }
  }

  async function retryFailed(taskId: string) {
    await retryWeakPasswordFailed(taskId)
    await fetchTaskDetail(taskId)
  }

  async function deleteTask(taskId: string) {
    await deleteWeakPasswordTask(taskId)
    if (currentTask.value?.id === taskId) {
      currentTask.value = null
      progress.value = null
      hosts.value = []
      findings.value = []
      errors.value = []
    }
    await fetchTasks()
  }

  async function fetchDictionaries() {
    dictionaryLoading.value = true
    try {
      const [defaultDict, dictResult] = await Promise.all([
        getDefaultWeakPasswordDictionary(),
        listWeakPasswordDictionaries(),
      ])
      defaultDictionary.value = defaultDict
      dictionaries.value = dictResult.items
    } finally {
      dictionaryLoading.value = false
    }
  }

  async function generateDictionary(payload: AIGenerateDictionaryRequest) {
    const result = await generateWeakPasswordDictionary(payload)
    await fetchDictionaries()
    return result
  }

  return {
    candidates,
    candidateTotal,
    tasks,
    taskTotal,
    currentTask,
    progress,
    hosts,
    findings,
    errors,
    dictionaries,
    defaultDictionary,
    analysisResult,
    loading,
    analyzing,
    creatingTask,
    dictionaryLoading,
    candidateFilters,
    taskFilters,
    analyze,
    fetchCandidates,
    createTask,
    fetchTasks,
    fetchTaskDetail,
    retryFailed,
    deleteTask,
    fetchDictionaries,
    generateDictionary,
  }
})
