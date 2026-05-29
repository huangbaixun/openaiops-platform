import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('../client', () => ({
  default: { get: vi.fn(), post: vi.fn() },
}))

import client from '../client'
import { fetchAnnotations, createAnnotation } from '../annotations'

const mockGet = client.get as ReturnType<typeof vi.fn>
const mockPost = client.post as ReturnType<typeof vi.fn>

beforeEach(() => vi.clearAllMocks())

describe('fetchAnnotations', () => {
  it('routes through shared axios client at /v1/annotations (NOT raw fetch)', async () => {
    mockGet.mockResolvedValue({ data: [] })
    await fetchAnnotations('service', { targetId: 'checkout' })
    expect(mockGet).toHaveBeenCalledWith('/v1/annotations', {
      params: { target_type: 'service', target_id: 'checkout' },
    })
  })

  it('omits target_id when not given', async () => {
    mockGet.mockResolvedValue({ data: [] })
    await fetchAnnotations('service')
    expect(mockGet).toHaveBeenCalledWith('/v1/annotations', {
      params: { target_type: 'service' },
    })
  })
})

describe('createAnnotation', () => {
  it('POSTs through the shared client and returns annotation_id', async () => {
    mockPost.mockResolvedValue({ data: { annotation_id: 'abc' } })
    const id = await createAnnotation({
      target_type: 'trace', target_id: 't1', kind: 'ai_rca',
      payload: { x: 1 }, ts: '2026-05-29T12:00:00Z',
    })
    expect(mockPost).toHaveBeenCalledWith('/v1/annotations', expect.objectContaining({
      target_type: 'trace', target_id: 't1',
    }))
    expect(id).toBe('abc')
  })
})
