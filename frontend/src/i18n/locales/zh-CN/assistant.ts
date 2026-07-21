export default {
  session: {
    new: '新会话',
  },
  progress: {
    callingTool: '正在调用工具: {name}',
    stepCompleted: '已完成步骤：{title}',
    stepTitle: '步骤 {number}',
  },
  outcome: {
    accepted: '请求已受理，业务操作尚未完成。',
    running: '业务操作仍在执行。',
    failed: '业务操作执行失败，详情见返回结果。',
    skipped: '业务操作已跳过。',
  },
  errors: {
    fetchSessions: '获取会话列表失败',
    deleteSession: '删除会话失败',
    createSession: '创建会话失败',
    fetchSession: '获取会话详情失败',
    fetchMessages: '获取消息列表失败',
    sendMessage: '发送消息失败',
    cancelRun: '取消运行失败',
    fetchContextRefs: '获取上下文引用失败',
    sessionRequired: '请先创建会话',
    fetchToolCalls: '获取工具调用列表失败',
    fetchApprovals: '获取审批列表失败',
    approval: '审批操作失败',
    compression: '上下文压缩失败',
    run: '助手运行出错',
    openSession: '打开会话失败',
  },
}
