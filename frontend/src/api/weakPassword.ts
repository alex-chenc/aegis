import request from './index'
import type {
  AIGenerateDictionaryRequest,
  AnalyzeAssetApplicationsRequest,
  AnalyzeAssetApplicationsResponse,
  CreateWeakPasswordBatchTasksRequest,
  CreateWeakPasswordBatchTasksResponse,
  DeleteWeakPasswordTasksResponse,
  CreateWeakPasswordDictionaryRequest,
  CreateWeakPasswordTaskRequest,
  CreateWeakPasswordTaskResponse,
  PageResult,
  RevealedWeakPasswordFinding,
  WeakPasswordCandidateApplication,
  WeakPasswordCollectionError,
  WeakPasswordCollectionProgress,
  WeakPasswordDictionary,
  WeakPasswordDictionaryEntry,
  WeakPasswordFinding,
  WeakPasswordScanHost,
  WeakPasswordTask,
  WeakPasswordTaskProgress,
} from '@/types/weakPassword'

export function analyzeAssetApplications(payload: AnalyzeAssetApplicationsRequest): Promise<AnalyzeAssetApplicationsResponse> {
  return request.post('/weak-password/asset-applications/analyze', payload)
}

export function listAssetApplications(params: Record<string, any>): Promise<PageResult<WeakPasswordCandidateApplication>> {
  return request.get('/weak-password/asset-applications', { params })
}

export function createWeakPasswordTask(payload: CreateWeakPasswordTaskRequest): Promise<CreateWeakPasswordTaskResponse> {
  return request.post('/weak-password/tasks/by-application', payload)
}

export function createWeakPasswordBatchTasks(payload: CreateWeakPasswordBatchTasksRequest): Promise<CreateWeakPasswordBatchTasksResponse> {
  return request.post('/weak-password/tasks/by-applications', payload)
}

export function listWeakPasswordTasks(params: Record<string, any>): Promise<PageResult<WeakPasswordTask>> {
  return request.get('/weak-password/tasks', { params })
}

export function getWeakPasswordTask(id: string): Promise<{ task: WeakPasswordTask; errors: WeakPasswordCollectionError[] }> {
  return request.get(`/weak-password/tasks/${id}`)
}

export function getWeakPasswordTaskProgress(id: string): Promise<WeakPasswordTaskProgress> {
  return request.get(`/weak-password/tasks/${id}/progress`)
}

export function listWeakPasswordTaskHosts(id: string, params?: Record<string, any>): Promise<PageResult<WeakPasswordScanHost>> {
  return request.get(`/weak-password/tasks/${id}/hosts`, { params })
}

export function listWeakPasswordFindings(id: string, params?: Record<string, any>): Promise<PageResult<WeakPasswordFinding>> {
  return request.get(`/weak-password/tasks/${id}/findings`, { params })
}

export function listWeakPasswordTaskErrors(id: string, params?: Record<string, any>): Promise<PageResult<WeakPasswordCollectionError>> {
  return request.get(`/weak-password/tasks/${id}/errors`, { params })
}

export function listWeakPasswordTaskCollectionProgress(id: string, params?: Record<string, any>): Promise<PageResult<WeakPasswordCollectionProgress>> {
  return request.get(`/weak-password/tasks/${id}/collection-progress`, { params })
}

export function retryWeakPasswordFailed(id: string): Promise<{ status: string }> {
  return request.post(`/weak-password/tasks/${id}/retry-failed`)
}

export function deleteWeakPasswordTask(id: string): Promise<{ deleted: number }> {
  return request.delete(`/weak-password/tasks/${id}`)
}

export function deleteWeakPasswordTasks(taskIds: string[]): Promise<DeleteWeakPasswordTasksResponse> {
  return request.post('/weak-password/tasks/batch-delete', { task_ids: taskIds })
}

export function getDefaultWeakPasswordDictionary(): Promise<WeakPasswordDictionary> {
  return request.get('/weak-password/dictionaries/default')
}

export function listWeakPasswordDictionaries(params?: Record<string, any>): Promise<PageResult<WeakPasswordDictionary>> {
  return request.get('/weak-password/dictionaries', { params })
}

export function listWeakPasswordDictionaryEntries(id: string, params?: Record<string, any>): Promise<PageResult<WeakPasswordDictionaryEntry>> {
  return request.get(`/weak-password/dictionaries/${id}/entries`, { params })
}

export function createWeakPasswordDictionary(payload: CreateWeakPasswordDictionaryRequest): Promise<WeakPasswordDictionary> {
  return request.post('/weak-password/dictionaries', payload)
}

export function generateWeakPasswordDictionary(payload: AIGenerateDictionaryRequest): Promise<WeakPasswordDictionary> {
  return request.post('/weak-password/dictionaries/ai-generate', payload)
}

export function revealWeakPasswordFinding(id: string, password: string): Promise<RevealedWeakPasswordFinding> {
  return request.post(`/weak-password/findings/${id}/reveal`, { password })
}
