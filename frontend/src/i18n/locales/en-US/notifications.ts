export default {
  title: 'Notifications',
  markAllRead: 'Mark All as Read',
  markRead: 'Mark as Read',
  unread: 'Unread',
  read: 'Read',
  noUnread: 'No unread notifications',
  noRead: 'No read notifications',
  view: 'View',
  ruleAdjusted: {
    title: 'AI Rule Update',
    content: 'AI automatically updated rule {ruleTitle} (MITRE: {mitreId}) to version {version} to reduce false positives.',
  },
  ruleAdjustmentSuggested: {
    title: 'AI Rule Update Suggestion',
    content: 'AI generated an adjustment suggestion for rule {ruleTitle} (MITRE: {mitreId}). Review it before deployment.',
  },
  ruleApproved: {
    title: 'Rule Approved',
    content: 'Rule {ruleTitle} was approved and activated.',
  },
  ruleGenerated: {
    title: 'AI Rule Generated',
    content: 'AI generated rule {ruleTitle} (MITRE: {mitreId}); its current status is {status}.',
  },
}
