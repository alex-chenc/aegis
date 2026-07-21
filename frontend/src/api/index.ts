import axios from 'axios'
import { ElMessage } from 'element-plus'
import { clearStoredAuth, getAuthToken, getStoredAuth } from '@/utils/auth'
import { getCurrentLocale, translate } from '@/i18n'

const request = axios.create({
  baseURL: '/api/v1',
  timeout: 300000  // 5 分钟，用于 LLM 脚本生成
})

request.interceptors.request.use(config => {
  config.headers['Accept-Language'] = getCurrentLocale()
  const token = getAuthToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

request.interceptors.response.use(
  response => {
    const { code, error_code: errorCode, params, message, data } = response.data
    // 如果响应包含 code 字段，检查是否成功
    if (code !== undefined && code !== 0) {
      const localized = localizeAPIError(errorCode, params, message)
      ElMessage.error(localized)
      return Promise.reject(new Error(localized))
    }
    // 如果有 data 字段，返回 data；否则返回整个响应数据
    return data !== undefined ? data : response.data
  },
  error => {
    if (error.code === 'ECONNABORTED' || String(error.message || '').toLowerCase().includes('timeout')) {
      const timeoutMsg = translate('common.messages.requestTimeout')
      ElMessage.error(timeoutMsg)
      return Promise.reject(new Error(timeoutMsg))
    }
    // 处理 HTTP 错误
    if (error.response) {
      // 服务器返回错误状态码
      const status = error.response.status
      const data = error.response.data
      let errorMsg = localizeAPIError(data?.error_code, data?.params, data?.message || data?.error)

      if (status === 404) {
        errorMsg = translate('common.messages.resourceNotFound')
      } else if (status === 500) {
        errorMsg = translate('common.messages.serverError')
      } else if (status === 401) {
        clearStoredAuth()
        if (window.location.pathname !== '/login') {
          window.location.assign('/login')
        }
        return Promise.reject(new Error(translate('common.messages.unauthorized')))
      } else if (status === 403) {
        errorMsg = localizeAPIError(
          error.response?.data?.error_code,
          error.response?.data?.params,
          error.response?.data?.message,
          'common.messages.forbidden'
        )
        if (getStoredAuth()?.forcePasswordChange && window.location.pathname !== '/force-password-change') {
          window.location.assign('/force-password-change')
        }
      }

      ElMessage.error(errorMsg)
      return Promise.reject(new Error(errorMsg))
    } else if (error.request) {
      // 请求已发送但没有收到响应
      const networkError = translate('common.messages.networkError')
      ElMessage.error(networkError)
      return Promise.reject(new Error(networkError))
    } else {
      // 请求配置出错
      ElMessage.error(error.message || translate('common.messages.requestError'))
      return Promise.reject(error)
    }
  }
)

const ERROR_MESSAGE_KEYS: Record<string, string> = {
  TASK_DELETE_RUNNING: 'errors.taskDeleteRunning',
  RESOURCE_NOT_FOUND: 'common.messages.resourceNotFound',
  UNAUTHORIZED: 'common.messages.unauthorized',
  FORBIDDEN: 'common.messages.forbidden',
}

export function localizeAPIError(
  errorCode?: string,
  params?: Record<string, unknown>,
  fallback?: string,
  defaultKey = 'common.messages.requestFailed'
): string {
  const key = errorCode ? ERROR_MESSAGE_KEYS[errorCode] : undefined
  if (key) return translate(key, params)
  if (getCurrentLocale() === 'zh-CN' && fallback) return fallback
  return translate(defaultKey)
}

export default request
