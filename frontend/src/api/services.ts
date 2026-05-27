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

export async function fetchServices(
  window: TimeWindow,
  opts: { limit?: number; sort?: 'calls' | 'errors' | 'p95' } = {},
): Promise<ServicesListResponse> {
  const p = new URLSearchParams({ window })
  if (opts.limit) p.set('limit', String(opts.limit))
  if (opts.sort) p.set('sort', opts.sort)
  const r = await fetch(`/api/v1/services?${p.toString()}`, {
    credentials: 'include',
  })
  if (!r.ok) throw new Error(`services ${r.status}`)
  return r.json()
}

export async function fetchServiceDetail(
  name: string,
  window: TimeWindow,
): Promise<ServiceDetail | null> {
  const r = await fetch(
    `/api/v1/services/${encodeURIComponent(name)}?window=${window}`,
    { credentials: 'include' },
  )
  if (r.status === 404) return null
  if (!r.ok) throw new Error(`services/${name} ${r.status}`)
  return r.json()
}
