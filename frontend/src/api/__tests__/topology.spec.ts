import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { fetchTopology } from '../topology'

describe('fetchTopology', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('GETs /api/v1/topology with window+node_limit', async () => {
    ;(fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ window: '1h', nodes: [], edges: [] }),
    })
    const r = await fetchTopology('1h', 100)
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/topology?window=1h&node_limit=100',
      expect.anything(),
    )
    expect(r.nodes).toEqual([])
  })

  it('throws on non-OK', async () => {
    ;(fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: false,
      status: 500,
    })
    await expect(fetchTopology('1h')).rejects.toThrow(/500/)
  })
})
