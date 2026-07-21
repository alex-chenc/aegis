export default {
  taskStatus: { completed: 'Completed', failed: 'Failed', interrupted: 'Interrupted', limited: 'Limited', running: 'Running', pending: 'Pending', cancelled: 'Cancelled', unknown: 'Unknown Status' },
  stepStatus: { completed: 'Completed', failed: 'Failed', running: 'Running', pending: 'Pending', skipped: 'Skipped', retrying: 'Retrying', unknown: 'Unknown' },
  exitReason: { normalCompleted: 'Completed Normally', maxIterations: 'Maximum Iterations Reached', timeout: 'Timed Out', userCancelled: 'Cancelled by User', cancelled: 'Cancelled', error: 'Execution Error', auditRejected: 'Rejected by Audit', driftDetected: 'Plan Drift Detected', toolFailed: 'Tool Execution Failed', rateLimit: 'Rate Limited', unknown: 'Unknown Reason' },
  verdict: { benign: 'Benign / False Positive', malicious: 'Malicious', suspicious: 'Suspicious', unknown: 'Unknown' },
  conclusion: { completed: 'Execution completed with no clear abnormal conclusion', completedWithErrors: 'Execution completed with collection or check errors; review the errors', failed: 'Execution failed and cannot provide a reliable security conclusion', interrupted: 'The task did not finish; current results are for reference only', unknown: 'No clear security conclusion is available' },
}
