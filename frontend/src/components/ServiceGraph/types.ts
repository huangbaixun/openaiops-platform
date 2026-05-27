import type { SimulationNodeDatum, SimulationLinkDatum } from 'd3-force'

// Extend d3-force base interfaces so forceSimulation<GraphNode> and
// forceLink<GraphNode, GraphEdge> generic constraints are satisfied.
// SimulationNodeDatum provides optional x, y, vx, vy, fx, fy, index.
// SimulationLinkDatum<GraphNode> provides source/target as GraphNode|string|number.
export interface GraphNode extends SimulationNodeDatum {
  service: string
  kind: 'service' | 'external'
  calls: number
  errors?: number
  p95_ms?: number
}

// API payload shape — caller/callee identify endpoints. d3-force needs
// source/target; we map them locally inside useForceSimulation before passing
// to forceLink (see SimEdge there). Keeping these optional here lets callers
// construct GraphEdge from API data without bogus placeholder fields.
export interface GraphEdge {
  caller: string
  callee: string
  callee_kind: 'service' | 'external'
  calls: number
  errors: number
  p95_ms: number
  source?: GraphNode | string
  target?: GraphNode | string
}

// d3-force-compatible edge shape produced inside useForceSimulation by
// mapping caller/callee -> source/target. Satisfies SimulationLinkDatum
// constraint (source/target required, no undefined).
export interface SimEdge extends SimulationLinkDatum<GraphNode> {
  caller: string
  callee: string
  callee_kind: 'service' | 'external'
  calls: number
  errors: number
  p95_ms: number
  source: GraphNode | string | number
  target: GraphNode | string | number
}
