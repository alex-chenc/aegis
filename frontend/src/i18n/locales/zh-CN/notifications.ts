export default {
  title: '消息通知',
  markAllRead: '全部标为已读',
  markRead: '标为已读',
  unread: '未读',
  read: '已读',
  noUnread: '暂无未读通知',
  noRead: '暂无已读通知',
  view: '前往查看',
  ruleAdjusted: {
    title: 'AI规则更新通知',
    content: 'AI已自动更新规则：{ruleTitle}（MITRE：{mitreId}），更新版本至 {version}，减少误报',
  },
  ruleAdjustmentSuggested: {
    title: 'AI规则更新建议',
    content: 'AI已生成规则调整建议：{ruleTitle}（MITRE：{mitreId}），请审核后下发',
  },
  ruleApproved: {
    title: '规则审核通过',
    content: '规则 {ruleTitle} 已审核通过并激活',
  },
  ruleGenerated: {
    title: 'AI规则生成通知',
    content: 'AI已自动生成新规则：{ruleTitle}（MITRE：{mitreId}），当前状态为 {status}',
  },
}
