import client from './client'

export interface TraceListItem {
  trace_id: string
  root_service: string
  root_operation: string
  start_ts: string
  duration_ns: number
  span_count: number
  services: string[]
}

export interface TraceListResponse {
  items: TraceListItem[]
  has_more: boolean
}

export interface SpanDetail {
  span_id: string
  parent_span_id: string
  service: string
  operation: string
  ts: string
  duration_ns: number
  status: string
  span_kind: string
  resource_attributes: Record<string, string>
  attributes: Record<string, string>
}

export interface TraceDetailResponse {
  trace_id: string
  spans: SpanDetail[]
}

export interface ListQuery {
  service?: string
  operation?: string
  min_duration_ms?: number
  ts_from?: string
  ts_to?: string
  limit?: number
  offset?: number
  sort?: 'ts' | 'duration'
  order?: 'asc' | 'desc'
}

export async function listTraces(q: ListQuery): Promise<TraceListResponse> {
  const { data } = await client.get<TraceListResponse>('/v1/traces', { params: q })
  return data
}

export async function getTrace(traceId: string): Promise<TraceDetailResponse> {
  const { data } = await client.get<TraceDetailResponse>(`/v1/traces/${traceId}`)
  return data
}
