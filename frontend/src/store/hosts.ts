import { defineStore } from 'pinia';
import { getHosts } from '@/api/hosts';
import type { Host } from '@/types';

export const useHostStore = defineStore('hosts', {
  state: () => ({
    hosts: [] as Host[],
    total: 0,
    isLoading: false,
    error: null as string | null,
  }),
  actions: {
    async fetchHosts(page: number, pageSize: number, search?: string) {
      this.isLoading = true;
      this.error = null;
      try {
        const response = await getHosts({ page, pageSize, search });
        this.hosts = response.items;
        this.total = response.total;
      } catch (err: any) {
        this.error = err.message || 'Failed to fetch hosts';
      } finally {
        this.isLoading = false;
      }
    },
    updateHostStatus(updateData: Partial<Host> & { id: string }) {
      const index = this.hosts.findIndex(h => h.id === updateData.id);
      if (index !== -1 && this.hosts[index]) {
        const currentHost = this.hosts[index];
        const updatedHost: Host = {
            id: currentHost.id,
            ip_address: updateData.ip_address ?? currentHost.ip_address,
            hostname: updateData.hostname ?? currentHost.hostname,
            is_online: updateData.is_online ?? currentHost.is_online,
            os_type: updateData.os_type ?? currentHost.os_type,
            os_version: updateData.os_version ?? currentHost.os_version,
            kernel_version: updateData.kernel_version ?? currentHost.kernel_version,
            architecture: updateData.architecture ?? currentHost.architecture,
            agent_version: updateData.agent_version ?? currentHost.agent_version,
            last_heartbeat_at: updateData.last_heartbeat_at ?? currentHost.last_heartbeat_at,
            cpu_load_1min: updateData.cpu_load_1min ?? currentHost.cpu_load_1min,
            mem_usage_percent: updateData.mem_usage_percent ?? currentHost.mem_usage_percent,
            cpu_info: updateData.cpu_info ?? currentHost.cpu_info,
            total_memory_mb: updateData.total_memory_mb ?? currentHost.total_memory_mb,
            total_disk_gb: updateData.total_disk_gb ?? currentHost.total_disk_gb,
            network_interfaces: updateData.network_interfaces ?? currentHost.network_interfaces,
            created_at: updateData.created_at ?? currentHost.created_at,
            updated_at: updateData.updated_at ?? currentHost.updated_at,
        };
        
        this.hosts[index] = updatedHost;
      }
    },
  },
  getters: {
    onlineCount: (state) => state.hosts.filter((h) => h.is_online).length,
  },
});