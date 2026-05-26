import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('./client', () => ({
  default: {
    get: vi.fn(),
  },
}))

import { listLogs } from './logs'
import client from './client'

const mockGet = client.get as ReturnType<typeof vi.fn>

const RESPONSE = {
  items: [
    {
      ts: '2026-05-26T10:00:00.000Z',
      observed_ts: '2026-05-26T10:00:00.001Z',
      service: 'frontend',
      severity_text: 'INFO',
      severity_number: 9,
      body: 'request received',
      trace_id: 'abc123',
      span_id: 'def456',
      trace_flags: 1,
      resource_attributes: { 'service.version': '1.0' },
      attributes: { 'http.method': 'GET' },
    },
  ],
  has_more: false,
}

beforeEach(() => {
  vi.clearAllMocks()
  mockGet.mockResolvedValue({ data: RESPONSE })
})

describe('listLogs', () => {
  it('calls /v1/logs with no params when empty object passed', async () => {
    await listLogs({})
    expect(mockGet).toHaveBeenCalledWith('/v1/logs', { params: {} })
  })

  it('passes service array param', async () => {
    await listLogs({ service: ['frontend', 'backend'] })
    expect(mockGet).toHaveBeenCalledWith(
      '/v1/logs',
      expect.objectContaining({ params: expect.objectContaining({ service: ['frontend', 'backend'] }) }),
    )
  })

  it('passes severity array param', async () => {
    await listLogs({ severity: ['ERROR', 'WARN'] })
    expect(mockGet).toHaveBeenCalledWith(
      '/v1/logs',
      expect.objectContaining({ params: expect.objectContaining({ severity: ['ERROR', 'WARN'] }) }),
    )
  })

  it('maps tsFrom/tsTo to ts_from/ts_to', async () => {
    await listLogs({ tsFrom: '2026-05-26T00:00:00Z', tsTo: '2026-05-26T23:59:59Z' })
    const callParams = mockGet.mock.calls[0][1].params
    expect(callParams.ts_from).toBe('2026-05-26T00:00:00Z')
    expect(callParams.ts_to).toBe('2026-05-26T23:59:59Z')
  })

  it('maps traceId → trace_id', async () => {
    await listLogs({ traceId: 'abc123' })
    expect(mockGet).toHaveBeenCalledWith(
      '/v1/logs',
      expect.objectContaining({ params: expect.objectContaining({ trace_id: 'abc123' }) }),
    )
  })

  it('maps spanId → span_id', async () => {
    await listLogs({ spanId: 'def456' })
    expect(mockGet).toHaveBeenCalledWith(
      '/v1/logs',
      expect.objectContaining({ params: expect.objectContaining({ span_id: 'def456' }) }),
    )
  })

  it('maps bodyContains → body_contains', async () => {
    await listLogs({ bodyContains: 'error occurred' })
    expect(mockGet).toHaveBeenCalledWith(
      '/v1/logs',
      expect.objectContaining({ params: expect.objectContaining({ body_contains: 'error occurred' }) }),
    )
  })

  it('passes limit and offset', async () => {
    await listLogs({ limit: 50, offset: 100 })
    const callParams = mockGet.mock.calls[0][1].params
    expect(callParams.limit).toBe(50)
    expect(callParams.offset).toBe(100)
  })

  it('omits empty service array', async () => {
    await listLogs({ service: [] })
    const callParams = mockGet.mock.calls[0][1].params
    expect(callParams.service).toBeUndefined()
  })

  it('returns the response data', async () => {
    const result = await listLogs({})
    expect(result).toEqual(RESPONSE)
  })

  it('throws when client rejects', async () => {
    mockGet.mockRejectedValueOnce(new Error('logs list 401'))
    await expect(listLogs({})).rejects.toThrow('logs list 401')
  })
})
