import axios from 'axios'
import { ElMessage } from 'element-plus'
import { clearStoredAuth, getAuthToken, getStoredAuth } from '@/utils/auth'

const request = axios.create({
  baseURL: '/api/v1',
  timeout: 300000  // 5 分钟，用于 LLM 脚本生成
})

request.interceptors.request.use(config => {
  const token = getAuthToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

request.interceptors.response.use(
  response => {
    const { code, message, data } = response.data
    // 如果响应包含 code 字段，检查是否成功
    if (code !== undefined && code !== 0) {
      ElMessage.error(message || '请求失败')
      return Promise.reject(new Error(message || 'Error'))
    }
    // 如果有 data 字段，返回 data；否则返回整个响应数据
    return data !== undefined ? data : response.data
  },
  error => {
    // 处理 HTTP 错误
    if (error.response) {
      // 服务器返回错误状态码
      const status = error.response.status
      const data = error.response.data
      let errorMsg = data?.message || data?.error || `请求失败 (${status})`

      if (status === 404) {
        errorMsg = '请求的资源不存在'
      } else if (status === 500) {
        errorMsg = '服务器内部错误'
      } else if (status === 401) {
        errorMsg = '未授权'
        clearStoredAuth()
        if (window.location.pathname !== '/login') {
          window.location.assign('/login')
        }
      } else if (status === 403) {
        errorMsg = '禁止访问'
        if (getStoredAuth()?.forcePasswordChange && window.location.pathname !== '/force-password-change') {
          window.location.assign('/force-password-change')
        }
      }

      ElMessage.error(errorMsg)
      return Promise.reject(new Error(errorMsg))
    } else if (error.request) {
      // 请求已发送但没有收到响应
      ElMessage.error('网络错误，请检查网络连接')
      return Promise.reject(new Error('网络错误'))
    } else {
      // 请求配置出错
      ElMessage.error(error.message || '请求错误')
      return Promise.reject(error)
    }
  }
)

export default request
