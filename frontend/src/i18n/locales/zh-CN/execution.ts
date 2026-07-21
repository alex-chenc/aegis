export default {
  taskStatus: { completed: '已完成', failed: '执行失败', interrupted: '已中断', limited: '已受限', running: '执行中', pending: '等待中', cancelled: '已取消', unknown: '未知状态' },
  stepStatus: { completed: '已完成', failed: '失败', running: '执行中', pending: '等待中', skipped: '已跳过', retrying: '正在重试', unknown: '未知' },
  exitReason: { normalCompleted: '正常完成', maxIterations: '达到最大轮次', timeout: '执行超时', userCancelled: '用户取消', cancelled: '已取消', error: '执行错误', auditRejected: '审计拒绝', driftDetected: '检测到计划漂移', toolFailed: '工具执行失败', rateLimit: '速率限制', unknown: '未知原因' },
  verdict: { benign: '良性/误报', malicious: '恶意', suspicious: '可疑', unknown: '未知' },
  conclusion: { completed: '执行完成，未发现明确异常结论', completedWithErrors: '执行完成，但存在采集或检查错误，需要结合错误信息复核', failed: '执行失败，无法形成可靠安全结论', interrupted: '任务未完整执行，当前结果仅供参考', unknown: '暂无明确安全结论' },
}
