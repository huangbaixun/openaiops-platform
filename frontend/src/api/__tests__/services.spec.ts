import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('../client', () => ({
  default: {
    get: vi.fn(),
  },
}))

import client from '../client'
import { fetchServices, fetchServiceDetail } from '../services'

const mockGet = client.get as ReturnType<typeof vi.fn>

beforeEach(() => {
  vi.clearAllMocks()
})

describe('fetchServices', () => {
  it('routes through shared axios client at /v1/services (NOT raw fetch — SLICE-3 T15 regression)', async () => {
    mockGet.mockResolvedValue({ data: { window: '1h', items: [] } })
    await fetchServices('1h')
    expect(mockGet).toHaveBeenCalledTimes(1)
    // First arg must be the relative path; baseURL=/api prepends + interceptor
    // attaches Bearer token. SLICE-3 T11 originally used raw fetch('/api/v1/...')
    // which skipped the interceptor entirely → "services 401" on every page load.
    expect(mockGet).toHaveBeenCalledWith('/v1/services', expect.anything())
  })

  it('passes window + sort + limit params', async () => {
    mockGet.mockResolvedValue({ data: { window: '6h', items: [] } })
    await fetchServices('6h', { limit: 50, sort: 'errors' })
    expect(mockGet).toHaveBeenCalledWith('/v1/services', {
      params: { window: '6h', limit: 50, sort: 'errors' },
    })
  })

  it('returns the unwrapped data payload', async () => {
    mockGet.mockResolvedValue({ data: { window: '1h', items: [{ service: 'a' }] } })
    const r = await fetchServices('1h')
    expect(r.items).toHaveLength(1)
  })
})

describe('fetchServiceDetail', () => {
  it('GETs /v1/services/<name> with window param', async () => {
    mockGet.mockResolvedValue({
      data: {
        service: 'checkout',
        window: '1h',
        stats: { inbound: { calls: 0 }, outbound: { calls: 0 } },
        dependencies: { inbound: [], outbound: [] },
      },
    })
    await fetchServiceDetail('checkout', '1h')
    expect(mockGet).toHaveBeenCalledWith('/v1/services/checkout', { params: { window: '1h' } })
  })

  it('returns null on 404', async () => {
    mockGet.mockRejectedValue({ response: { status: 404 } })
    const r = await fetchServiceDetail('missing', '1h')
    expect(r).toBeNull()
  })

  it('rethrows on non-404 error', async () => {
    mockGet.mockRejectedValue({ response: { status: 500 } })
    await expect(fetchServiceDetail('checkout', '1h')).rejects.toEqual(
      expect.objectContaining({ response: { status: 500 } }),
    )
  })

  it('URL-encodes service names containing /', async () => {
    mockGet.mockResolvedValue({
      data: {
        service: 'a/b',
        window: '1h',
        stats: { inbound: { calls: 0 }, outbound: { calls: 0 } },
        dependencies: { inbound: [], outbound: [] },
      },
    })
    await fetchServiceDetail('a/b', '1h')
    expect(mockGet).toHaveBeenCalledWith('/v1/services/a%2Fb', expect.anything())
  })
})
