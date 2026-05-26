import client from './client'

export interface LogItem {
  ts: string
  observed_ts: string
  service: string
  severity_text: string
  severity_number: number
  body: string
  trace_id: string
  span_id: string
  trace_flags: number
  resource_attributes: Record<string, string>
  attributes: Record<string, string>
}

export interface LogsListParams {
  service?: string[]
  severity?: string[]
  tsFrom?: string
  tsTo?: string
  traceId?: string
  spanId?: string
  bodyContains?: string
  limit?: number
  offset?: number
}

export interface LogsListResponse {
  items: LogItem[]
  has_more: boolean
}

export async function listLogs(params: LogsListParams): Promise<LogsListResponse> {
  const p: Record<string, string | string[] | number> = {}
  if (params.service?.length) p['service'] = params.service
  if (params.severity?.length) p['severity'] = params.severity
  if (params.tsFrom) p['ts_from'] = params.tsFrom
  if (params.tsTo) p['ts_to'] = params.tsTo
  if (params.traceId) p['trace_id'] = params.traceId
  if (params.spanId) p['span_id'] = params.spanId
  if (params.bodyContains) p['body_contains'] = params.bodyContains
  if (params.limit != null) p['limit'] = params.limit
  if (params.offset != null) p['offset'] = params.offset

  const { data } = await client.get<LogsListResponse>('/v1/logs', { params: p })
  return data
}
