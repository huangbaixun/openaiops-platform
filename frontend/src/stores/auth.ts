import { defineStore } from 'pinia'
import client from '../api/client'

interface State {
  tenantId: string | null
  tenantName: string | null
}

export const useAuthStore = defineStore('auth', {
  state: (): State => ({
    tenantId: null,
    tenantName: null,
  }),
  getters: {
    isAuthenticated: (s) => s.tenantId !== null,
  },
  actions: {
    async login(apiKey: string) {
      localStorage.setItem('apiKey', apiKey)
      try {
        const { data } = await client.get('/healthz')
        this.tenantId = data.tenant_id
        this.tenantName = data.tenant_name
      } catch (e) {
        localStorage.removeItem('apiKey')
        this.tenantId = null
        this.tenantName = null
        throw e
      }
    },
    logout() {
      localStorage.removeItem('apiKey')
      this.tenantId = null
      this.tenantName = null
    },
    async restore() {
      const key = localStorage.getItem('apiKey')
      if (!key) return
      try {
        const { data } = await client.get('/healthz')
        this.tenantId = data.tenant_id
        this.tenantName = data.tenant_name
      } catch {
        this.logout()
      }
    },
  },
})
