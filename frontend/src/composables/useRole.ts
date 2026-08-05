import { ref } from 'vue'
import { getStoredAuth } from '@/utils/auth'
import { getCurrentUser } from '@/api/auth'

type Role = 'security_analyst' | 'security_developer' | 'admin'

const PERMISSIONS: Record<Role, string[]> = {
  security_analyst: ['view', 'draft', 'ai_generate'],
  security_developer: ['view', 'draft', 'ai_generate', 'build', 'review', 'sign'],
  admin: ['view', 'draft', 'ai_generate', 'build', 'review', 'sign', 'enable', 'disable', 'uninstall', 'rollback', 'allowlist', 'agent_guard_action', 'agent_guard_session_delete', 'agent_guard_settings'],
}

function getDefaultRole(): Role {
  const stored = getStoredAuth()
  if (stored?.role && stored.role in PERMISSIONS) {
    return stored.role as Role
  }
  return 'security_analyst'
}

const currentRole = ref<Role>(getDefaultRole())
let roleFetched = false

export function useRole() {
  const canOperate = (operation: string) => PERMISSIONS[currentRole.value].includes(operation)
  const setRole = (role: Role) => { currentRole.value = role }

  // Fetch role from server on first use if not in storage
  if (!roleFetched && !getStoredAuth()?.role) {
    roleFetched = true
    getCurrentUser().then(user => {
      if (user.role && user.role in PERMISSIONS) {
        currentRole.value = user.role as Role
        // Update stored auth with role
        const stored = getStoredAuth()
        if (stored) {
          stored.role = user.role
          localStorage.setItem('aegis-auth', JSON.stringify(stored))
        }
      }
    }).catch(() => {})
  }

  return { role: currentRole, canOperate, setRole }
}
