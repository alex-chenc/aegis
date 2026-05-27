import { ref } from 'vue'
import { detectionPackageApi } from '@/api/detection-packages'
import type { DetectionPackage, DetectionPackageDraft, DetectionPackageBuild, PackageHostStatus, PageQuery } from '@/api/detection-packages'
import { ElMessage } from 'element-plus'

export function useDetectionPackages() {
  const packages = ref<DetectionPackage[]>([])
  const total = ref(0)
  const loading = ref(false)
  const currentPackage = ref<DetectionPackage | null>(null)
  const currentDraft = ref<DetectionPackageDraft | null>(null)
  const currentBuild = ref<DetectionPackageBuild | null>(null)
  const hostStatuses = ref<PackageHostStatus[]>([])
  const hostTotal = ref(0)

  async function fetchPackages(params?: PageQuery) {
    loading.value = true
    try {
      const res = await detectionPackageApi.list(params)
      packages.value = res.data
      total.value = res.total
    } catch (e: any) {
      ElMessage.error(e.message || '获取检测包列表失败')
    } finally {
      loading.value = false
    }
  }

  async function fetchPackage(packageId: string) {
    loading.value = true
    try {
      currentPackage.value = await detectionPackageApi.get(packageId)
    } catch (e: any) {
      ElMessage.error(e.message || '获取检测包详情失败')
    } finally {
      loading.value = false
    }
  }

  async function fetchDraft(packageId: string) {
    try {
      currentDraft.value = await detectionPackageApi.getDraft(packageId)
    } catch (e: any) {
      // Draft not found is expected for non-draft packages
    }
  }

  async function generateDraft(data: any) {
    loading.value = true
    try {
      currentDraft.value = await detectionPackageApi.generateDraft(data)
      ElMessage.success('AI 草稿生成成功')
      return currentDraft.value
    } finally {
      loading.value = false
    }
  }

  async function createDraft(data: any) {
    loading.value = true
    try {
      currentDraft.value = await detectionPackageApi.createDraft(data)
      ElMessage.success('草稿创建成功')
      return currentDraft.value
    } catch (e: any) {
      ElMessage.error(e.message || '草稿创建失败')
      throw e
    } finally {
      loading.value = false
    }
  }

  async function updateDraft(draftId: string, data: any) {
    loading.value = true
    try {
      currentDraft.value = await detectionPackageApi.updateDraft(draftId, data)
      ElMessage.success('草稿更新成功')
      return currentDraft.value
    } catch (e: any) {
      ElMessage.error(e.message || '草稿更新失败')
      throw e
    } finally {
      loading.value = false
    }
  }

  async function startBuild(packageId: string) {
    loading.value = true
    try {
      currentBuild.value = await detectionPackageApi.build(packageId)
      ElMessage.success('构建任务已提交')
      return currentBuild.value
    } catch (e: any) {
      ElMessage.error(e.message || '构建任务提交失败')
      throw e
    } finally {
      loading.value = false
    }
  }

  async function fetchBuild(buildId: string) {
    try {
      currentBuild.value = await detectionPackageApi.getBuild(buildId)
      return currentBuild.value
    } catch (e: any) {
      ElMessage.error(e.message || '获取构建状态失败')
      throw e
    }
  }

  async function fetchLatestBuild(packageId: string) {
    try {
      currentBuild.value = await detectionPackageApi.getLatestBuild(packageId)
    } catch {
      // No build found is expected for new packages
    }
  }

  async function signPackage(packageId: string) {
    loading.value = true
    try {
      currentPackage.value = await detectionPackageApi.sign(packageId)
      ElMessage.success('签名发布成功')
      return currentPackage.value
    } catch (e: any) {
      ElMessage.error(e.message || '签名发布失败')
      throw e
    } finally {
      loading.value = false
    }
  }

  async function enablePackage(packageId: string) {
    loading.value = true
    try {
      await detectionPackageApi.enable(packageId)
      ElMessage.success('检测包已启用')
    } catch (e: any) {
      ElMessage.error(e.message || '启用失败')
      throw e
    } finally {
      loading.value = false
    }
  }

  async function disablePackage(packageId: string) {
    loading.value = true
    try {
      await detectionPackageApi.disable(packageId)
      ElMessage.success('检测包已禁用')
    } catch (e: any) {
      ElMessage.error(e.message || '禁用失败')
      throw e
    } finally {
      loading.value = false
    }
  }

  async function uninstallPackage(packageId: string) {
    loading.value = true
    try {
      await detectionPackageApi.uninstall(packageId)
      ElMessage.success('卸载指令已下发')
    } catch (e: any) {
      ElMessage.error(e.message || '卸载失败')
      throw e
    } finally {
      loading.value = false
    }
  }

  async function deletePackage(packageId: string) {
    loading.value = true
    try {
      await detectionPackageApi.deletePackage(packageId)
      ElMessage.success('删除成功')
    } catch (e: any) {
      ElMessage.error(e.message || '删除失败')
      throw e
    } finally {
      loading.value = false
    }
  }

  async function batchDeletePackages(packageIds: string[]) {
    loading.value = true
    try {
      await detectionPackageApi.batchDeletePackages(packageIds)
      ElMessage.success(`成功删除 ${packageIds.length} 个检测包`)
    } catch (e: any) {
      ElMessage.error(e.message || '批量删除失败')
      throw e
    } finally {
      loading.value = false
    }
  }

  async function fetchHostStatus(packageId: string, params?: PageQuery) {
    try {
      const res = await detectionPackageApi.hostStatus(packageId, params)
      hostStatuses.value = res.data
      hostTotal.value = res.total
    } catch (e: any) {
      ElMessage.error(e.message || '获取主机状态失败')
    }
  }

  return {
    packages,
    total,
    loading,
    currentPackage,
    currentDraft,
    currentBuild,
    hostStatuses,
    hostTotal,
    fetchPackages,
    fetchPackage,
    fetchDraft,
    generateDraft,
    createDraft,
    updateDraft,
    startBuild,
    fetchBuild,
    fetchLatestBuild,
    signPackage,
    enablePackage,
    disablePackage,
    uninstallPackage,
    deletePackage,
    batchDeletePackages,
    fetchHostStatus,
  }
}
