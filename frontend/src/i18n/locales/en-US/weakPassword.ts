export default {
  status: {
    pending: 'Pending', taskCreated: 'Task Created', analyzingAssets: 'Analyzing Assets', collectingCredentials: 'Collecting Credentials', collecting: 'Collecting', dispatchAgentTool: 'Dispatching Collection Tool', processDiscovery: 'Locating Process Configuration', repairing: 'AI-assisted Location Repair', matching: 'Matching Passwords', completed: 'Completed', partialFailed: 'Partially Failed', failed: 'Failed', cancelled: 'Cancelled', candidate: 'Candidate', planned: 'Planned', matched: 'Matched', noMatch: 'No Match', ignored: 'Ignored', executing: 'Running', retryScheduled: 'Retry Scheduled', enabled: 'Enabled', disabled: 'Disabled', online: 'Online', offline: 'Offline', unknown: 'Unknown', unscanned: 'Not Scanned', safe: 'Safe', alert: 'Alert', unresolved: 'Unresolved',
  },
  error: {
    noApplicationAssets: 'No application assets available for analysis', agentNotConnected: 'Agent is not connected', agentCallbackUnavailable: 'Agent callback is unavailable', permissionDenied: 'Permission denied', fileNotFound: 'File not found', fieldNotFound: 'Credential field not found', fileTooLarge: 'File is too large', configDiscoveryFailed: 'Configuration discovery failed', llmMatchVerifyFailed: 'LLM match verification failed', unsupportedCredentialFormat: 'Unsupported credential format', agentExecuteFailed: 'Agent tool execution failed', findingPersistFailed: 'Failed to save findings',
  },
  credential: { plaintext: 'Plaintext', hash: 'Hash', saltedHash: 'Salted Hash', encryptedBlob: 'Encrypted Data', authString: 'Authentication String', unknown: 'Unknown' },
  match: { confirmed: 'Confirmed', needsConfirm: 'AI-inferred; Confirmation Required', verifyFailed: 'Verification Failed', falsePositive: 'False Positive', fixed: 'Fixed', riskAccepted: 'Risk Accepted' },
  dictionaryType: { builtIn: 'Built-in', uploaded: 'Custom', aiGenerated: 'AI-generated', temporary: 'Temporary' },
  dictionarySource: { builtIn: 'Built-in', uploaded: 'Manually Uploaded', aiGenerated: 'AI-generated', selected: 'Selected Dictionary', default: 'Default Weak Password Dictionary' },
  tool: { collectCredentials: 'Collect Credential Configuration', processConfigHints: 'Analyze Process Configuration Hints', serviceUnitInspect: 'Inspect Service Unit', finalDiagnosis: 'Final Diagnosis' },
}
