import { defineStore } from 'pinia'
import { getHosts, type Host } from '@/api/hosts'

interface HostState {
  hosts: Host[]
  total: number
  loading: boolean
}

export const useHostStore = defineStore('hosts', {
  state: (): HostState => ({
    hosts: [],
    total: 0,
    loading: false
  }),

  actions: {
    async fetchHosts(page = 1, pageSize = 10, query = '') {
      this.loading = true
      try {
        const data = await getHosts({ page, pageSize, query })
        this.hosts = data
        this.total = data.length
      } finally {
        this.loading = false
      }
    }
  }
})