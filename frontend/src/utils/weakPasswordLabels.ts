import { translate } from '@/i18n'

const statusLabels: Record<string, string> = {
  pending: 'weakPassword.status.pending', task_created: 'weakPassword.status.taskCreated', analyzing_assets: 'weakPassword.status.analyzingAssets', collecting_credentials: 'weakPassword.status.collectingCredentials', collecting: 'weakPassword.status.collecting', dispatch_agent_tool: 'weakPassword.status.dispatchAgentTool', process_based_config_discovery: 'weakPassword.status.processDiscovery', repairing_collection: 'weakPassword.status.repairing', repairing: 'weakPassword.status.repairing', matching: 'weakPassword.status.matching', completed: 'weakPassword.status.completed', partial_failed: 'weakPassword.status.partialFailed', failed: 'weakPassword.status.failed', cancelled: 'weakPassword.status.cancelled', candidate: 'weakPassword.status.candidate', planned: 'weakPassword.status.planned', matched: 'weakPassword.status.matched', no_match: 'weakPassword.status.noMatch', ignored: 'weakPassword.status.ignored', executing: 'weakPassword.status.executing', retry_scheduled: 'weakPassword.status.retryScheduled', enabled: 'weakPassword.status.enabled', disabled: 'weakPassword.status.disabled', online: 'weakPassword.status.online', offline: 'weakPassword.status.offline', unknown: 'weakPassword.status.unknown', unscanned: 'weakPassword.status.unscanned', safe: 'weakPassword.status.safe', alert: 'weakPassword.status.alert', unresolved: 'weakPassword.status.unresolved',
}

const errorCodeLabels: Record<string, string> = {
  no_application_assets: 'weakPassword.error.noApplicationAssets', agent_not_connected: 'weakPassword.error.agentNotConnected', agent_callback_unavailable: 'weakPassword.error.agentCallbackUnavailable', permission_denied: 'weakPassword.error.permissionDenied', file_not_found: 'weakPassword.error.fileNotFound', field_not_found: 'weakPassword.error.fieldNotFound', file_too_large: 'weakPassword.error.fileTooLarge', config_discovery_failed: 'weakPassword.error.configDiscoveryFailed', llm_match_verify_failed: 'weakPassword.error.llmMatchVerifyFailed', unsupported_credential_format: 'weakPassword.error.unsupportedCredentialFormat', agent_execute_failed: 'weakPassword.error.agentExecuteFailed', finding_persist_failed: 'weakPassword.error.findingPersistFailed',
}

const credentialTypeLabels: Record<string, string> = {
  plaintext: 'weakPassword.credential.plaintext', hash: 'weakPassword.credential.hash', salted_hash: 'weakPassword.credential.saltedHash', encrypted_blob: 'weakPassword.credential.encryptedBlob', auth_string: 'weakPassword.credential.authString', unknown: 'weakPassword.credential.unknown',
}

const matchStatusLabels: Record<string, string> = {
  confirmed: 'weakPassword.match.confirmed', ai_inferred_needs_confirm: 'weakPassword.match.needsConfirm', verify_failed: 'weakPassword.match.verifyFailed', false_positive: 'weakPassword.match.falsePositive', fixed: 'weakPassword.match.fixed', risk_accepted: 'weakPassword.match.riskAccepted',
}

const dictionaryTypeLabels: Record<string, string> = {
  default_1000: 'weakPassword.dictionaryType.builtIn', uploaded: 'weakPassword.dictionaryType.uploaded', ai_generated: 'weakPassword.dictionaryType.aiGenerated', task_temp: 'weakPassword.dictionaryType.temporary',
}

const dictionarySourceLabels: Record<string, string> = {
  built_in: 'weakPassword.dictionarySource.builtIn', uploaded: 'weakPassword.dictionarySource.uploaded', ai_generated: 'weakPassword.dictionarySource.aiGenerated', selected_dictionary: 'weakPassword.dictionarySource.selected', default_1000: 'weakPassword.dictionarySource.default',
}

const toolNameLabels: Record<string, string> = {
  'WeakPassword.CollectCredentials': 'weakPassword.tool.collectCredentials', 'WeakPassword.ProcessConfigHints': 'weakPassword.tool.processConfigHints', 'WeakPassword.ServiceUnitInspect': 'weakPassword.tool.serviceUnitInspect', 'WeakPassword.FinalDiagnosis': 'weakPassword.tool.finalDiagnosis',
}

function resolveLabel(labels: Record<string, string>, value: string) {
  return labels[value] ? translate(labels[value]) : value
}

export function weakPasswordStatusLabel(status?: string) {
  return status ? resolveLabel(statusLabels, status) : '-'
}

export function weakPasswordErrorCodeLabel(code?: string) {
  return code ? resolveLabel(errorCodeLabels, code) : '-'
}

export function weakPasswordErrorMessageLabel(message?: string, code?: string) {
  return message && message !== code ? message : weakPasswordErrorCodeLabel(code)
}

export function weakPasswordCredentialTypeLabel(type?: string) {
  return type ? resolveLabel(credentialTypeLabels, type) : '-'
}

export function weakPasswordMatchStatusLabel(status?: string) {
  return status ? (matchStatusLabels[status] ? translate(matchStatusLabels[status]) : weakPasswordStatusLabel(status)) : '-'
}

export function weakPasswordDictionaryTypeLabel(type?: string) {
  return type ? resolveLabel(dictionaryTypeLabels, type) : '-'
}

export function weakPasswordDictionarySourceLabel(source?: string) {
  return source ? resolveLabel(dictionarySourceLabels, source) : '-'
}

export function weakPasswordToolNameLabel(toolName?: string) {
  return toolName ? resolveLabel(toolNameLabels, toolName) : '-'
}
