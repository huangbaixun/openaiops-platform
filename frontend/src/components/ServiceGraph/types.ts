export interface GraphNode {
  service: string
  kind: 'service' | 'external'
  calls: number
  errors?: number
  p95_ms?: number
  // d3-force mutates these in place during simulation:
  x?: number
  y?: number
  fx?: number | null
  fy?: number | null
  vx?: number
  vy?: number
}

export interface GraphEdge {
  caller: string
  callee: string
  callee_kind: 'service' | 'external'
  calls: number
  errors: number
  p95_ms: number
  // d3-force replaces these strings with node refs after forceLink runs:
  source?: GraphNode | string
  target?: GraphNode | string
}
