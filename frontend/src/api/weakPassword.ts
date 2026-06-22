import request from './index'
import type {
  AIGenerateDictionaryRequest,
  AnalyzeAssetApplicationsRequest,
  AnalyzeAssetApplicationsResponse,
  CreateWeakPasswordDictionaryRequest,
  CreateWeakPasswordTaskRequest,
  CreateWeakPasswordTaskResponse,
  PageResult,
  RevealedWeakPasswordFinding,
  WeakPasswordCandidateApplication,
  WeakPasswordCollectionError,
  WeakPasswordDictionary,
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

export function listWeakPasswordTasks(params: Record<string, any>): Promise<PageResult<WeakPasswordTask>> {
  return request.get('/weak-password/tasks', { params })
}

export function getWeakPasswordTask(id: string): Promise<{ task: WeakPasswordTask; errors: WeakPasswordCollectionError[] }> {
  return request.get(`/weak-password/tasks/${id}`)
}

export function getWeakPasswordTaskProgress(id: string): Promise<WeakPasswordTaskProgress> {
  return request.get(`/weak-password/tasks/${id}/progress`)
}

export function listWeakPasswordTaskHosts(id: string): Promise<PageResult<WeakPasswordScanHost>> {
  return request.get(`/weak-password/tasks/${id}/hosts`)
}

export function listWeakPasswordFindings(id: string): Promise<PageResult<WeakPasswordFinding>> {
  return request.get(`/weak-password/tasks/${id}/findings`)
}

export function retryWeakPasswordFailed(id: string): Promise<{ status: string }> {
  return request.post(`/weak-password/tasks/${id}/retry-failed`)
}

export function deleteWeakPasswordTask(id: string): Promise<{ deleted: number }> {
  return request.delete(`/weak-password/tasks/${id}`)
}

export function getDefaultWeakPasswordDictionary(): Promise<WeakPasswordDictionary> {
  return request.get('/weak-password/dictionaries/default')
}

export function listWeakPasswordDictionaries(): Promise<PageResult<WeakPasswordDictionary>> {
  return request.get('/weak-password/dictionaries')
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
