import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('../client', () => ({
  default: {
    get: vi.fn(),
  },
}))

import client from '../client'
import { fetchTopology } from '../topology'

const mockGet = client.get as ReturnType<typeof vi.fn>

beforeEach(() => {
  vi.clearAllMocks()
})

describe('fetchTopology', () => {
  it('routes through shared axios client at /v1/topology (NOT raw fetch — SLICE-3 T15 regression)', async () => {
    mockGet.mockResolvedValue({ data: { window: '1h', nodes: [], edges: [] } })
    await fetchTopology('1h')
    expect(mockGet).toHaveBeenCalledWith('/v1/topology', expect.anything())
  })

  it('passes window + node_limit params', async () => {
    mockGet.mockResolvedValue({ data: { window: '1h', nodes: [], edges: [] } })
    await fetchTopology('1h', 100)
    expect(mockGet).toHaveBeenCalledWith('/v1/topology', {
      params: { window: '1h', node_limit: 100 },
    })
  })

  it('defaults node_limit to 100 when omitted', async () => {
    mockGet.mockResolvedValue({ data: { window: '1h', nodes: [], edges: [] } })
    await fetchTopology('1h')
    expect(mockGet).toHaveBeenCalledWith('/v1/topology', {
      params: { window: '1h', node_limit: 100 },
    })
  })

  it('returns unwrapped data payload', async () => {
    mockGet.mockResolvedValue({
      data: {
        window: '1h',
        nodes: [{ service: 'a', kind: 'service', calls: 1, errors: 0, p95_ms: 1 }],
        edges: [],
      },
    })
    const r = await fetchTopology('1h')
    expect(r.nodes).toHaveLength(1)
  })

  it('rethrows when axios rejects', async () => {
    mockGet.mockRejectedValue(new Error('Network Error'))
    await expect(fetchTopology('1h')).rejects.toThrow('Network Error')
  })
})
