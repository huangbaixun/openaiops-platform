import client from './client'
import type { TimeWindow } from '../composables/useTimeWindow'

export interface ServiceListItem {
  service: string
  inbound_calls: number
  inbound_errors: number
  inbound_error_rate: number
  inbound_p95_ms: number
  outbound_calls: number
}

export interface ServicesListResponse {
  window: string
  items: ServiceListItem[]
}

export interface DirectionStats {
  calls: number
  errors?: number
  error_rate?: number
  p95_ms?: number
}

export interface Dependency {
  peer: string
  peer_kind: 'service' | 'external'
  calls: number
  errors: number
  p95_ms: number
}

export interface ServiceDetail {
  service: string
  window: string
  stats: { inbound: DirectionStats; outbound: DirectionStats }
  dependencies: { inbound: Dependency[]; outbound: Dependency[] }
}

// Use shared axios client (api/client.ts) so the localStorage apiKey is
// auto-injected as a Bearer header via the request interceptor — same path as
// traces.ts / logs.ts. SLICE-3 T15 caught that raw fetch() here skipped auth
// entirely (page rendered with "services 401" error). Regression test lives in
// __tests__/services.spec.ts.
export async function fetchServices(
  window: TimeWindow,
  opts: { limit?: number; sort?: 'calls' | 'errors' | 'p95' } = {},
): Promise<ServicesListResponse> {
  const params: Record<string, string | number> = { window }
  if (opts.limit) params.limit = opts.limit
  if (opts.sort) params.sort = opts.sort
  const { data } = await client.get<ServicesListResponse>('/v1/services', { params })
  return data
}

export async function fetchServiceDetail(
  name: string,
  window: TimeWindow,
): Promise<ServiceDetail | null> {
  try {
    const { data } = await client.get<ServiceDetail>(
      `/v1/services/${encodeURIComponent(name)}`,
      { params: { window } },
    )
    return data
  } catch (e: unknown) {
    const err = e as { response?: { status?: number } }
    if (err?.response?.status === 404) return null
    throw e
  }
}
