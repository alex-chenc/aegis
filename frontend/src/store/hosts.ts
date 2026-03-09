import { defineStore } from 'pinia'
import { getHosts } from '@/api/hosts'
import type { Host } from '@/types'

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
    async fetchHosts(page = 1, pageSize = 100, query = '') {
      this.loading = true
      try {
        this.hosts = await getHosts({ page, pageSize, query })
        this.total = this.hosts.length
      } finally {
        this.loading = false
      }
    }
  }
})