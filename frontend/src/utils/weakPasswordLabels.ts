const statusLabels: Record<string, string> = {
  pending: '待执行',
  task_created: '任务已创建',
  analyzing_assets: '资产分析中',
  collecting_credentials: '采集凭据中',
  collecting: '采集中',
  dispatch_agent_tool: '下发采集工具',
  process_based_config_discovery: '进程配置定位中',
  repairing_collection: 'AI 修复定位中',
  repairing: 'AI 修复定位中',
  matching: '密码匹配中',
  completed: '已完成',
  partial_failed: '部分失败',
  failed: '失败',
  cancelled: '已取消',
  candidate: '候选',
  planned: '已计划',
  matched: '已命中',
  no_match: '未命中',
  ignored: '已忽略',
  executing: '执行中',
  retry_scheduled: '已提交重试',
  enabled: '已启用',
  disabled: '已禁用',
  online: '在线',
  offline: '离线',
  unknown: '未知',
  unscanned: '未扫描',
  safe: '安全',
  alert: '告警',
  unresolved: '未解决',
}

const errorCodeLabels: Record<string, string> = {
  no_application_assets: '暂无可分析应用资产',
  agent_not_connected: 'Agent 未连接',
  agent_callback_unavailable: 'Agent 回调不可用',
  permission_denied: '权限不足',
  file_not_found: '文件不存在',
  field_not_found: '未找到凭据字段',
  file_too_large: '文件过大',
  config_discovery_failed: '配置发现失败',
  llm_match_verify_failed: 'LLM 匹配校验失败',
  unsupported_credential_format: '凭据格式不支持',
  agent_execute_failed: 'Agent 工具执行失败',
  finding_persist_failed: '结果入库失败',
}

const credentialTypeLabels: Record<string, string> = {
  plaintext: '明文',
  hash: '哈希',
  salted_hash: '加盐哈希',
  encrypted_blob: '加密密文',
  auth_string: '认证串',
  unknown: '未知',
}

const matchStatusLabels: Record<string, string> = {
  confirmed: '已确认',
  ai_inferred_needs_confirm: 'AI 推断待确认',
  verify_failed: '校验失败',
  false_positive: '误报',
  fixed: '已修复',
  risk_accepted: '已接受风险',
}

const dictionaryTypeLabels: Record<string, string> = {
  default_1000: '内置',
  uploaded: '自定义',
  ai_generated: 'AI 生成',
  task_temp: '任务临时',
}

const dictionarySourceLabels: Record<string, string> = {
  built_in: '内置',
  uploaded: '手动上传',
  ai_generated: 'AI 生成',
  selected_dictionary: '选中字典',
  default_1000: '默认弱密码字典',
}

const toolNameLabels: Record<string, string> = {
  'WeakPassword.CollectCredentials': '采集凭据配置',
  'WeakPassword.ProcessConfigHints': '分析进程配置线索',
  'WeakPassword.ServiceUnitInspect': '检查服务单元',
  'WeakPassword.FinalDiagnosis': '最终诊断',
}

export function weakPasswordStatusLabel(status?: string) {
  if (!status) return '-'
  return statusLabels[status] || status
}

export function weakPasswordErrorCodeLabel(code?: string) {
  if (!code) return '-'
  return errorCodeLabels[code] || code
}

export function weakPasswordErrorMessageLabel(message?: string, code?: string) {
  if (message && message !== code) {
    return message
  }
  return weakPasswordErrorCodeLabel(code)
}

export function weakPasswordCredentialTypeLabel(type?: string) {
  if (!type) return '-'
  return credentialTypeLabels[type] || type
}

export function weakPasswordMatchStatusLabel(status?: string) {
  if (!status) return '-'
  return matchStatusLabels[status] || weakPasswordStatusLabel(status)
}

export function weakPasswordDictionaryTypeLabel(type?: string) {
  if (!type) return '-'
  return dictionaryTypeLabels[type] || type
}

export function weakPasswordDictionarySourceLabel(source?: string) {
  if (!source) return '-'
  return dictionarySourceLabels[source] || source
}

export function weakPasswordToolNameLabel(toolName?: string) {
  if (!toolName) return '-'
  return toolNameLabels[toolName] || toolName
}
