import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useWeakPasswordStore } from './weakPassword'

const analyzeMock = vi.fn()
const listCandidatesMock = vi.fn()
const createTaskMock = vi.fn()
const createBatchTasksMock = vi.fn()
const listTasksMock = vi.fn()
const getTaskMock = vi.fn()
const getProgressMock = vi.fn()
const listHostsMock = vi.fn()
const listFindingsMock = vi.fn()
const listErrorsMock = vi.fn()
const retryMock = vi.fn()
const deleteTaskMock = vi.fn()
const getDefaultDictMock = vi.fn()
const listDictsMock = vi.fn()
const listDictEntriesMock = vi.fn()
const generateDictMock = vi.fn()

vi.mock('@/api/weakPassword', () => ({
  analyzeAssetApplications: (...args: any[]) => analyzeMock(...args),
  listAssetApplications: (...args: any[]) => listCandidatesMock(...args),
  createWeakPasswordTask: (...args: any[]) => createTaskMock(...args),
  createWeakPasswordBatchTasks: (...args: any[]) => createBatchTasksMock(...args),
  listWeakPasswordTasks: (...args: any[]) => listTasksMock(...args),
  getWeakPasswordTask: (...args: any[]) => getTaskMock(...args),
  getWeakPasswordTaskProgress: (...args: any[]) => getProgressMock(...args),
  listWeakPasswordTaskHosts: (...args: any[]) => listHostsMock(...args),
  listWeakPasswordFindings: (...args: any[]) => listFindingsMock(...args),
  listWeakPasswordTaskErrors: (...args: any[]) => listErrorsMock(...args),
  retryWeakPasswordFailed: (...args: any[]) => retryMock(...args),
  deleteWeakPasswordTask: (...args: any[]) => deleteTaskMock(...args),
  getDefaultWeakPasswordDictionary: (...args: any[]) => getDefaultDictMock(...args),
  listWeakPasswordDictionaries: (...args: any[]) => listDictsMock(...args),
  listWeakPasswordDictionaryEntries: (...args: any[]) => listDictEntriesMock(...args),
  generateWeakPasswordDictionary: (...args: any[]) => generateDictMock(...args),
}))

describe('weak password store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('stores no_application_assets analysis response', async () => {
    analyzeMock.mockResolvedValue({
      analysis_id: '',
      status: 'failed',
      application_asset_count: 0,
      candidate_count: 0,
      error_code: 'no_application_assets',
      message: '当前范围没有应用资产，请先采集资产',
      candidates: [],
    })

    const store = useWeakPasswordStore()
    const result = await store.analyze({
      scope: {
        host_ids: [],
        host_group_ids: [],
        application_types: [],
        online_agents_only: true,
      },
    })

    expect(result.error_code).toBe('no_application_assets')
    expect(store.candidates).toEqual([])
    expect(store.analysisResult?.message).toContain('先采集资产')
  })

  it('loads task progress with config discovery failure', async () => {
    getTaskMock.mockResolvedValue({
      task: { id: 'task-1', name: '弱密码检查', status: 'failed', progress: 100, current_stage: 'config_discovery_failed' },
      errors: [{ id: 'err-1', error_code: 'config_discovery_failed', agent_tool_call_count: 10 }],
    })
    getProgressMock.mockResolvedValue({
      task_id: 'task-1',
      status: 'failed',
      progress: 100,
      current_stage: 'config_discovery_failed',
      current_host_id: 'host-1',
      current_application: 'redis',
      agent_tool_call_count: 10,
      max_agent_tool_calls: 10,
      last_agent_tool: 'WeakPassword.ServiceUnitInspect',
      last_error_code: 'config_discovery_failed',
      message: 'AI 已尝试 10 次受控 Agent 工具调用',
    })
    listHostsMock.mockResolvedValue({ items: [], total: 0 })
    listFindingsMock.mockResolvedValue({ items: [], total: 0 })
    listErrorsMock.mockResolvedValue({ items: [{ id: 'err-1', error_code: 'config_discovery_failed', agent_tool_call_count: 10 }], total: 1 })

    const store = useWeakPasswordStore()
    await store.fetchTaskDetail('task-1')

    expect(store.progress?.agent_tool_call_count).toBe(10)
    expect(store.errors[0].error_code).toBe('config_discovery_failed')
    expect(store.errorTotal).toBe(1)
  })

  it('loads default dictionary summary', async () => {
    getDefaultDictMock.mockResolvedValue({ id: 'dict-default', name: '默认弱密码字典', entry_count: 1000 })
    listDictsMock.mockResolvedValue({ items: [{ id: 'dict-default', name: '默认弱密码字典', status: 'enabled', entry_count: 1000 }], total: 1 })

    const store = useWeakPasswordStore()
    await store.fetchDictionaries()

    expect(store.defaultDictionary?.entry_count).toBe(1000)
    expect(store.dictionaries).toHaveLength(1)
  })

  it('creates batch weak password tasks and refreshes candidates', async () => {
    createBatchTasksMock.mockResolvedValue({
      created: [{ candidate_application_id: 'cand-1', task_id: 'task-1', scan_application_id: 'scan-1', status: 'pending' }],
      skipped: [{ candidate_application_id: 'cand-2', reason: 'host_offline' }],
    })
    listTasksMock.mockResolvedValue({ items: [], total: 0 })
    listCandidatesMock.mockResolvedValue({ items: [], total: 0 })

    const store = useWeakPasswordStore()
    const result = await store.createBatchTasks({
      candidate_application_ids: ['cand-1', 'cand-2'],
      dictionary_policy: { use_default_1000: true, dictionary_ids: [], use_ai_generated: false },
      ai_policy: { repair_collection_errors: true, max_agent_tool_calls_per_app: 10 },
    })

    expect(createBatchTasksMock).toHaveBeenCalled()
    expect(result.created).toHaveLength(1)
    expect(result.skipped[0].reason).toBe('host_offline')
    expect(listCandidatesMock).toHaveBeenCalled()
  })

  it('generates dictionary from natural language', async () => {
    generateDictMock.mockResolvedValue({ id: 'dict-ai', name: 'AI 生成弱密码字典', dictionary_type: 'ai_generated', entry_count: 20 })
    getDefaultDictMock.mockResolvedValue({ id: 'dict-default', name: '默认弱密码字典', entry_count: 1000 })
    listDictsMock.mockResolvedValue({ items: [], total: 0 })

    const store = useWeakPasswordStore()
    await store.generateDictionary({
      natural_language: '为 Redis 生产环境生成弱密码字典，包含 aegis 和 admin',
      count: 20,
      deduplicate_with_default: true,
    })

    expect(generateDictMock).toHaveBeenCalledWith(expect.objectContaining({
      natural_language: expect.stringContaining('Redis'),
    }))
    expect(listDictsMock).toHaveBeenCalled()
  })

  it('loads dictionary entries with pagination state', async () => {
    listDictEntriesMock.mockResolvedValue({
      items: [{ id: 'entry-1', dictionary_id: 'dict-default', candidate: 'Admin@123', category: '通用弱口令' }],
      total: 1000,
    })

    const store = useWeakPasswordStore()
    store.dictionaryEntryFilters.page_size = 1000
    await store.fetchDictionaryEntries('dict-default')

    expect(listDictEntriesMock).toHaveBeenCalledWith('dict-default', store.dictionaryEntryFilters)
    expect(store.dictionaryEntries[0].candidate).toBe('Admin@123')
    expect(store.dictionaryEntryTotal).toBe(1000)
  })

  it('deletes weak password task and refreshes task list', async () => {
    deleteTaskMock.mockResolvedValue({ deleted: 1 })
    listTasksMock.mockResolvedValue({ items: [], total: 0 })

    const store = useWeakPasswordStore()
    await store.deleteTask('task-1')

    expect(deleteTaskMock).toHaveBeenCalledWith('task-1')
    expect(listTasksMock).toHaveBeenCalled()
  })
})
