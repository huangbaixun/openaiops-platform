import { defineStore } from 'pinia'
import client from '../api/client'
import { fetchTenants, switchTenant, type TenantOption } from '../api/tenants'

interface State {
  tenantId: string | null
  tenantName: string | null
  activeTenantId: string | null
  domainTenants: TenantOption[]
}

export const useAuthStore = defineStore('auth', {
  state: (): State => ({
    tenantId: null,
    tenantName: null,
    activeTenantId: null,
    domainTenants: [],
  }),
  getters: {
    isAuthenticated: (s) => s.tenantId !== null,
    activeTenant: (s) => s.domainTenants.find((t) => t.id === s.activeTenantId) ?? null,
  },
  actions: {
    async loadTenants() {
      try {
        this.domainTenants = await fetchTenants()
        const persisted = localStorage.getItem('activeTenantId')
        const valid = persisted && this.domainTenants.some((t) => t.id === persisted)
        this.activeTenantId = valid ? persisted : this.tenantId
        this.persistActive()
      } catch {
        this.domainTenants = this.tenantId
          ? [{ id: this.tenantId, name: this.tenantName ?? '', environment: '' }]
          : []
        this.activeTenantId = this.tenantId
      }
    },
    persistActive() {
      if (this.activeTenantId) localStorage.setItem('activeTenantId', this.activeTenantId)
      else localStorage.removeItem('activeTenantId')
    },
    async switchActiveTenant(tenantId: string) {
      await switchTenant(tenantId)
      this.activeTenantId = tenantId
      this.persistActive()
    },
    async login(apiKey: string) {
      localStorage.setItem('apiKey', apiKey)
      try {
        const { data } = await client.get('/healthz')
        this.tenantId = data.tenant_id
        this.tenantName = data.tenant_name
        await this.loadTenants()
      } catch (e) {
        localStorage.removeItem('apiKey')
        this.tenantId = null
        this.tenantName = null
        throw e
      }
    },
    logout() {
      localStorage.removeItem('apiKey')
      localStorage.removeItem('activeTenantId')
      this.tenantId = null
      this.tenantName = null
      this.activeTenantId = null
      this.domainTenants = []
    },
    async restore() {
      const key = localStorage.getItem('apiKey')
      if (!key) return
      try {
        const { data } = await client.get('/healthz')
        this.tenantId = data.tenant_id
        this.tenantName = data.tenant_name
        await this.loadTenants()
      } catch {
        this.logout()
      }
    },
  },
})
