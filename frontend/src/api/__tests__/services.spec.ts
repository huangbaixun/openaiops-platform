import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { fetchServices, fetchServiceDetail } from '../services'

describe('services API', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('fetchServices builds correct URL with sort+limit', async () => {
    ;(fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ window: '6h', items: [] }),
    })
    await fetchServices('6h', { limit: 50, sort: 'errors' })
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/services?window=6h&limit=50&sort=errors',
      expect.anything(),
    )
  })

  it('fetchServiceDetail returns null on 404', async () => {
    ;(fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: false,
      status: 404,
    })
    const r = await fetchServiceDetail('checkout', '1h')
    expect(r).toBeNull()
  })

  it('fetchServiceDetail throws on 500', async () => {
    ;(fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: false,
      status: 500,
    })
    await expect(fetchServiceDetail('checkout', '1h')).rejects.toThrow(/500/)
  })
})
