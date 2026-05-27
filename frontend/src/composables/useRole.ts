import { ref } from 'vue'

type Role = 'security_analyst' | 'security_developer' | 'admin'

const PERMISSIONS: Record<Role, string[]> = {
  security_analyst: ['view', 'draft', 'ai_generate'],
  security_developer: ['view', 'draft', 'ai_generate', 'build', 'review', 'sign'],
  admin: ['view', 'draft', 'ai_generate', 'build', 'review', 'sign', 'enable', 'disable', 'uninstall', 'rollback', 'allowlist'],
}

const currentRole = ref<Role>('security_analyst')

export function useRole() {
  const canOperate = (operation: string) => PERMISSIONS[currentRole.value].includes(operation)
  const setRole = (role: Role) => { currentRole.value = role }
  return { role: currentRole, canOperate, setRole }
}
