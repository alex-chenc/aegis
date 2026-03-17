import axios from 'axios'
import { ElMessage } from 'element-plus'

const request = axios.create({
  baseURL: '/api/v1',
  timeout: 300000  // 5 分钟，用于 LLM 脚本生成
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
    ElMessage.error(error.message || '网络错误')
    return Promise.reject(error)
  }
)

export default request