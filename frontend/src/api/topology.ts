import type { TimeWindow } from '../composables/useTimeWindow'

export interface TopologyNode {
  service: string
  kind: 'service' | 'external'
  calls: number
  errors: number
  p95_ms: number
}

export interface TopologyEdge {
  caller: string
  callee: string
  callee_kind: 'service' | 'external'
  calls: number
  errors: number
  p95_ms: number
}

export interface TopologyResponse {
  window: string
  nodes: TopologyNode[]
  edges: TopologyEdge[]
}

export async function fetchTopology(
  window: TimeWindow,
  nodeLimit = 100,
): Promise<TopologyResponse> {
  const r = await fetch(
    `/api/v1/topology?window=${window}&node_limit=${nodeLimit}`,
    { credentials: 'include' },
  )
  if (!r.ok) throw new Error(`topology ${r.status}`)
  return r.json()
}
