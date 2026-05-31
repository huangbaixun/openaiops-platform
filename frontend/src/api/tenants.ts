import client from './client'

export interface TenantOption {
  id: string
  name: string
  environment: string
}

export async function fetchTenants(): Promise<TenantOption[]> {
  const { data } = await client.get('/api/v1/tenants')
  return (data ?? []) as TenantOption[]
}

export async function switchTenant(tenantId: string): Promise<void> {
  await client.post('/api/v1/tenants/switch', { tenant_id: tenantId })
}
