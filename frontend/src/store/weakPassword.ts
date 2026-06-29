import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import {
  analyzeAssetApplications,
  createWeakPasswordBatchTasks,
  createWeakPasswordTask,
  deleteWeakPasswordTask,
  deleteWeakPasswordTasks,
  generateWeakPasswordDictionary,
  getDefaultWeakPasswordDictionary,
  getWeakPasswordTask,
  getWeakPasswordTaskProgress,
  listAssetApplications,
  listWeakPasswordDictionaryEntries,
  listWeakPasswordDictionaries,
  listWeakPasswordFindings,
  listWeakPasswordTaskCollectionProgress,
  listWeakPasswordTaskHosts,
  listWeakPasswordTasks,
  retryWeakPasswordFailed,
} from '@/api/weakPassword'
import type {
  AIGenerateDictionaryRequest,
  AnalyzeAssetApplicationsRequest,
  AnalyzeAssetApplicationsResponse,
  CreateWeakPasswordBatchTasksRequest,
  CreateWeakPasswordTaskRequest,
  WeakPasswordCandidateApplication,
  WeakPasswordCollectionProgress,
  WeakPasswordDictionary,
  WeakPasswordDictionaryEntry,
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
  const hostTotal = ref(0)
  const findings = ref<WeakPasswordFinding[]>([])
  const findingTotal = ref(0)
  const errors = ref<WeakPasswordCollectionProgress[]>([])
  const errorTotal = ref(0)
  const dictionaries = ref<WeakPasswordDictionary[]>([])
  const dictionaryTotal = ref(0)
  const dictionaryEntries = ref<WeakPasswordDictionaryEntry[]>([])
  const dictionaryEntryTotal = ref(0)
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

  const hostFilters = reactive({
    page: 1,
    page_size: 10
  })

  const findingFilters = reactive({
    page: 1,
    page_size: 10
  })

  const errorFilters = reactive({
    page: 1,
    page_size: 10
  })

  const dictionaryFilters = reactive({
    page: 1,
    page_size: 20
  })

  const dictionaryEntryFilters = reactive({
    page: 1,
    page_size: 20
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
      await fetchCandidates()
      return result
    } finally {
      creatingTask.value = false
    }
  }

  async function createBatchTasks(payload: CreateWeakPasswordBatchTasksRequest) {
    creatingTask.value = true
    try {
      const result = await createWeakPasswordBatchTasks(payload)
      await fetchTasks()
      await fetchCandidates()
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
      const [taskDetail, taskProgress, hostResult, findingResult, errorResult] = await Promise.all([
        getWeakPasswordTask(taskId),
        getWeakPasswordTaskProgress(taskId),
        listWeakPasswordTaskHosts(taskId, hostFilters),
        listWeakPasswordFindings(taskId, findingFilters),
        listWeakPasswordTaskCollectionProgress(taskId, errorFilters),
      ])
      currentTask.value = taskDetail.task
      progress.value = taskProgress
      hosts.value = hostResult.items
      hostTotal.value = hostResult.total
      findings.value = findingResult.items
      findingTotal.value = findingResult.total
      errors.value = errorResult.items
      errorTotal.value = errorResult.total
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
      hostTotal.value = 0
      findings.value = []
      findingTotal.value = 0
      errors.value = []
      errorTotal.value = 0
    }
    await fetchTasks()
  }

  async function deleteTasks(taskIds: string[]) {
    const result = await deleteWeakPasswordTasks(taskIds)
    if (currentTask.value && result.deleted.includes(currentTask.value.id)) {
      currentTask.value = null
      progress.value = null
      hosts.value = []
      hostTotal.value = 0
      findings.value = []
      findingTotal.value = 0
      errors.value = []
      errorTotal.value = 0
    }
    await fetchTasks()
    return result
  }

  async function fetchDictionaries() {
    dictionaryLoading.value = true
    try {
      const [defaultDict, dictResult] = await Promise.all([
        getDefaultWeakPasswordDictionary(),
        listWeakPasswordDictionaries(dictionaryFilters),
      ])
      defaultDictionary.value = defaultDict
      dictionaries.value = dictResult.items
      dictionaryTotal.value = dictResult.total
    } finally {
      dictionaryLoading.value = false
    }
  }

  async function fetchDictionaryEntries(dictionaryId: string) {
    dictionaryLoading.value = true
    try {
      const result = await listWeakPasswordDictionaryEntries(dictionaryId, dictionaryEntryFilters)
      dictionaryEntries.value = result.items
      dictionaryEntryTotal.value = result.total
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
    hostTotal,
    findings,
    findingTotal,
    errors,
    errorTotal,
    dictionaries,
    dictionaryTotal,
    dictionaryEntries,
    dictionaryEntryTotal,
    defaultDictionary,
    analysisResult,
    loading,
    analyzing,
    creatingTask,
    dictionaryLoading,
    candidateFilters,
    taskFilters,
    hostFilters,
    findingFilters,
    errorFilters,
    dictionaryFilters,
    dictionaryEntryFilters,
    analyze,
    fetchCandidates,
    createTask,
    createBatchTasks,
    fetchTasks,
    fetchTaskDetail,
    retryFailed,
    deleteTask,
    deleteTasks,
    fetchDictionaries,
    fetchDictionaryEntries,
    generateDictionary,
  }
})
