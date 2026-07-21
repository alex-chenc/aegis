import { ref } from 'vue'
import { detectionPackageApi } from '@/api/detection-packages'
import type { DetectionPackage, DetectionPackageDraft, DetectionPackageBuild, PackageHostStatus, PageQuery } from '@/api/detection-packages'
import { ElMessage } from 'element-plus'
import { translate } from '@/i18n'

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
      ElMessage.error(e.message || translate('dynamic.packageListFailed'))
    } finally {
      loading.value = false
    }
  }

  async function fetchPackage(packageId: string) {
    loading.value = true
    try {
      currentPackage.value = await detectionPackageApi.get(packageId)
    } catch (e: any) {
      ElMessage.error(e.message || translate('dynamic.packageDetailsFailed'))
    } finally {
      loading.value = false
    }
  }

  async function fetchDraft(packageId: string) {
    currentDraft.value = null
    try {
      currentDraft.value = await detectionPackageApi.getDraft(packageId)
    } catch (e: any) {
      // Draft not found is expected for non-draft packages
      currentDraft.value = null
    }
  }

  async function generateDraft(data: any) {
    loading.value = true
    try {
      currentDraft.value = await detectionPackageApi.generateDraft(data)
      ElMessage.success(translate('dynamic.aiDraftGenerated'))
      return currentDraft.value
    } finally {
      loading.value = false
    }
  }

  async function createDraft(data: any) {
    loading.value = true
    try {
      currentDraft.value = await detectionPackageApi.createDraft(data)
      ElMessage.success(translate('dynamic.draftCreated'))
      return currentDraft.value
    } catch (e: any) {
      ElMessage.error(e.message || translate('dynamic.draftCreateFailed'))
      throw e
    } finally {
      loading.value = false
    }
  }

  async function updateDraft(draftId: string, data: any) {
    loading.value = true
    try {
      currentDraft.value = await detectionPackageApi.updateDraft(draftId, data)
      ElMessage.success(translate('dynamic.draftUpdated'))
      return currentDraft.value
    } catch (e: any) {
      ElMessage.error(e.message || translate('dynamic.draftUpdateFailed'))
      throw e
    } finally {
      loading.value = false
    }
  }

  async function startBuild(packageId: string) {
    loading.value = true
    try {
      currentBuild.value = await detectionPackageApi.build(packageId)
      ElMessage.success(translate('dynamic.buildSubmitted'))
      return currentBuild.value
    } catch (e: any) {
      ElMessage.error(e.message || translate('dynamic.buildSubmitFailed'))
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
      ElMessage.error(e.message || translate('dynamic.buildStatusFailed'))
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
      ElMessage.success(translate('dynamic.packageSigned'))
      return currentPackage.value
    } catch (e: any) {
      ElMessage.error(e.message || translate('dynamic.packageSignFailed'))
      throw e
    } finally {
      loading.value = false
    }
  }

  async function enablePackage(packageId: string) {
    loading.value = true
    try {
      await detectionPackageApi.enable(packageId)
      ElMessage.success(translate('dynamic.packageEnabled'))
    } catch (e: any) {
      ElMessage.error(e.message || translate('dynamic.enableFailed'))
      throw e
    } finally {
      loading.value = false
    }
  }

  async function disablePackage(packageId: string) {
    loading.value = true
    try {
      await detectionPackageApi.disable(packageId)
      ElMessage.success(translate('dynamic.packageDisabled'))
    } catch (e: any) {
      ElMessage.error(e.message || translate('dynamic.disableFailed'))
      throw e
    } finally {
      loading.value = false
    }
  }

  async function uninstallPackage(packageId: string) {
    loading.value = true
    try {
      await detectionPackageApi.uninstall(packageId)
      ElMessage.success(translate('dynamic.uninstallDispatched'))
    } catch (e: any) {
      ElMessage.error(e.message || translate('dynamic.uninstallFailed'))
      throw e
    } finally {
      loading.value = false
    }
  }

  async function deletePackage(packageId: string) {
    loading.value = true
    try {
      await detectionPackageApi.deletePackage(packageId)
      ElMessage.success(translate('common.messages.deleteSuccess'))
    } catch (e: any) {
      ElMessage.error(e.message || translate('dynamic.deleteFailed'))
      throw e
    } finally {
      loading.value = false
    }
  }

  async function batchDeletePackages(packageIds: string[]) {
    loading.value = true
    try {
      await detectionPackageApi.batchDeletePackages(packageIds)
      ElMessage.success(translate('dynamic.packagesDeleted', { count: packageIds.length }))
    } catch (e: any) {
      ElMessage.error(e.message || translate('dynamic.batchDeleteFailed'))
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
      ElMessage.error(e.message || translate('dynamic.hostStatusFailed'))
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
