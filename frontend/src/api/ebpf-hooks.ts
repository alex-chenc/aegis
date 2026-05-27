import request from './index'

export interface EBPFHookAllowlist {
  version: number
  tracepoints: string[]
  kprobes: string[]
  lsm: string[]
  xdp: string[]
  tc: string[]
}

export interface UpdateAllowlistRequest {
  tracepoints?: string[]
  kprobes?: string[]
  lsm?: string[]
  xdp?: string[]
  tc?: string[]
}

export const ebpfHookApi = {
  getAllowlist: (): Promise<EBPFHookAllowlist> =>
    request.get('/settings/ebpf-hooks/allowlist'),

  updateAllowlist: (data: UpdateAllowlistRequest): Promise<EBPFHookAllowlist> =>
    request.put('/settings/ebpf-hooks/allowlist', data),

  getAllowlistHistory: (): Promise<any[]> =>
    request.get('/settings/ebpf-hooks/allowlist/history'),
}
