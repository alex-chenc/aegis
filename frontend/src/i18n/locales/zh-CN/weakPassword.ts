export default {
  status: {
    pending: '待执行', taskCreated: '任务已创建', analyzingAssets: '资产分析中', collectingCredentials: '采集凭据中', collecting: '采集中', dispatchAgentTool: '下发采集工具', processDiscovery: '进程配置定位中', repairing: 'AI 修复定位中', matching: '密码匹配中', completed: '已完成', partialFailed: '部分失败', failed: '失败', cancelled: '已取消', candidate: '候选', planned: '已计划', matched: '已命中', noMatch: '未命中', ignored: '已忽略', executing: '执行中', retryScheduled: '已提交重试', enabled: '已启用', disabled: '已禁用', online: '在线', offline: '离线', unknown: '未知', unscanned: '未扫描', safe: '安全', alert: '告警', unresolved: '未解决',
  },
  error: {
    noApplicationAssets: '暂无可分析应用资产', agentNotConnected: 'Agent 未连接', agentCallbackUnavailable: 'Agent 回调不可用', permissionDenied: '权限不足', fileNotFound: '文件不存在', fieldNotFound: '未找到凭据字段', fileTooLarge: '文件过大', configDiscoveryFailed: '配置发现失败', llmMatchVerifyFailed: 'LLM 匹配校验失败', unsupportedCredentialFormat: '凭据格式不支持', agentExecuteFailed: 'Agent 工具执行失败', findingPersistFailed: '结果入库失败',
  },
  credential: { plaintext: '明文', hash: '哈希', saltedHash: '加盐哈希', encryptedBlob: '加密密文', authString: '认证串', unknown: '未知' },
  match: { confirmed: '已确认', needsConfirm: 'AI 推断待确认', verifyFailed: '校验失败', falsePositive: '误报', fixed: '已修复', riskAccepted: '已接受风险' },
  dictionaryType: { builtIn: '内置', uploaded: '自定义', aiGenerated: 'AI 生成', temporary: '任务临时' },
  dictionarySource: { builtIn: '内置', uploaded: '手动上传', aiGenerated: 'AI 生成', selected: '选中字典', default: '默认弱密码字典' },
  tool: { collectCredentials: '采集凭据配置', processConfigHints: '分析进程配置线索', serviceUnitInspect: '检查服务单元', finalDiagnosis: '最终诊断' },
}
