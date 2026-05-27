import client from './client'
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

// Same auth-via-shared-client fix as services.ts — SLICE-3 T15 regression.
export async function fetchTopology(
  window: TimeWindow,
  nodeLimit = 100,
): Promise<TopologyResponse> {
  const { data } = await client.get<TopologyResponse>('/v1/topology', {
    params: { window, node_limit: nodeLimit },
  })
  return data
}
