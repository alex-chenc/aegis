import { defineStore } from 'pinia'
import { ref, reactive } from 'vue'
import {
  getAssetSummary,
  listSoftwareAssets,
  listApplicationAssets,
  listCollectionTasks,
  triggerAssetCollection,
  getCollectionConfig,
  updateCollectionConfig,
  retryCollectionTask,
  cancelCollectionTask,
  type AssetSummary,
  type SoftwareAsset,
  type ApplicationAsset,
  type CollectionTask,
  type AssetCollectionConfig,
  type SoftwareAssetQuery,
  type ApplicationAssetQuery,
  type CollectionTaskQuery,
  type TriggerCollectionPayload,
} from '@/api/assets'

export const useAssetStore = defineStore('assets', () => {
  // 状态
  const summary = ref<AssetSummary | null>(null)
  const softwareAssets = ref<SoftwareAsset[]>([])
  const applicationAssets = ref<ApplicationAsset[]>([])
  const collectionTasks = ref<CollectionTask[]>([])
  const collectionConfig = ref<AssetCollectionConfig | null>(null)

  const softwareTotal = ref(0)
  const applicationTotal = ref(0)
  const taskTotal = ref(0)

  const loading = ref(false)
  const collecting = ref(false)

  // 筛选条件
  const softwareFilters = reactive<SoftwareAssetQuery>({
    page: 1,
    page_size: 20,
    keyword: '',
    host_id: '',
    package_manager: '',
    os_type: '',
    status: '',
  })

  const applicationFilters = reactive<ApplicationAssetQuery>({
    page: 1,
    page_size: 20,
    keyword: '',
    host_id: '',
    category: '',
    min_confidence: 0,
    review_status: '',
    status: '',
  })

  const taskFilters = reactive<CollectionTaskQuery>({
    page: 1,
    page_size: 20,
    status: '',
  })

  // Actions

  /**
   * 获取资产概览
   */
  async function fetchSummary() {
    try {
      summary.value = await getAssetSummary()
    } catch (error) {
      console.error('Failed to fetch summary:', error)
      throw error
    }
  }

  /**
   * 获取软件资产列表
   */
  async function fetchSoftwareAssets() {
    loading.value = true
    try {
      const result = await listSoftwareAssets(softwareFilters)
      softwareAssets.value = result.items
      softwareTotal.value = result.total
    } catch (error) {
      console.error('Failed to fetch software assets:', error)
      throw error
    } finally {
      loading.value = false
    }
  }

  /**
   * 获取应用资产列表
   */
  async function fetchApplicationAssets() {
    loading.value = true
    try {
      const result = await listApplicationAssets(applicationFilters)
      applicationAssets.value = result.items
      applicationTotal.value = result.total
    } catch (error) {
      console.error('Failed to fetch application assets:', error)
      throw error
    } finally {
      loading.value = false
    }
  }

  /**
   * 获取采集任务列表
   */
  async function fetchCollectionTasks() {
    loading.value = true
    try {
      const result = await listCollectionTasks(taskFilters)
      collectionTasks.value = result.items
      taskTotal.value = result.total
    } catch (error) {
      console.error('Failed to fetch collection tasks:', error)
      throw error
    } finally {
      loading.value = false
    }
  }

  /**
   * 触发资产采集
   */
  async function triggerCollection(payload: TriggerCollectionPayload) {
    collecting.value = true
    try {
      const result = await triggerAssetCollection(payload)
      await fetchSummary()
      return result
    } catch (error) {
      console.error('Failed to trigger collection:', error)
      throw error
    } finally {
      collecting.value = false
    }
  }

  /**
   * 获取采集配置
   */
  async function fetchCollectionConfig() {
    try {
      collectionConfig.value = await getCollectionConfig()
    } catch (error) {
      console.error('Failed to fetch collection config:', error)
      throw error
    }
  }

  /**
   * 更新采集配置
   */
  async function saveCollectionConfig(config: Partial<AssetCollectionConfig>) {
    try {
      collectionConfig.value = await updateCollectionConfig(config)
    } catch (error) {
      console.error('Failed to update collection config:', error)
      throw error
    }
  }

  /**
   * 重试采集任务
   */
  async function retryTask(taskId: string) {
    try {
      await retryCollectionTask(taskId)
      await fetchCollectionTasks()
    } catch (error) {
      console.error('Failed to retry task:', error)
      throw error
    }
  }

  /**
   * 取消采集任务
   */
  async function cancelTask(taskId: string) {
    try {
      await cancelCollectionTask(taskId)
      await fetchCollectionTasks()
    } catch (error) {
      console.error('Failed to cancel task:', error)
      throw error
    }
  }

  /**
   * 重置软件资产筛选
   */
  function resetSoftwareFilters() {
    Object.assign(softwareFilters, {
      page: 1,
      page_size: 20,
      keyword: '',
      host_id: '',
      package_manager: '',
      os_type: '',
      status: '',
    })
  }

  /**
   * 重置应用资产筛选
   */
  function resetApplicationFilters() {
    Object.assign(applicationFilters, {
      page: 1,
      page_size: 20,
      keyword: '',
      host_id: '',
      category: '',
      min_confidence: 0,
      review_status: '',
      status: '',
    })
  }

  /**
   * 重置任务筛选
   */
  function resetTaskFilters() {
    Object.assign(taskFilters, {
      page: 1,
      page_size: 20,
      status: '',
    })
  }

  return {
    // State
    summary,
    softwareAssets,
    applicationAssets,
    collectionTasks,
    collectionConfig,
    softwareTotal,
    applicationTotal,
    taskTotal,
    loading,
    collecting,
    softwareFilters,
    applicationFilters,
    taskFilters,

    // Actions
    fetchSummary,
    fetchSoftwareAssets,
    fetchApplicationAssets,
    fetchCollectionTasks,
    triggerCollection,
    fetchCollectionConfig,
    saveCollectionConfig,
    retryTask,
    cancelTask,
    resetSoftwareFilters,
    resetApplicationFilters,
    resetTaskFilters,
  }
})
