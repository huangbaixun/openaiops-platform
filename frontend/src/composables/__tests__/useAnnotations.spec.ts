import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'

vi.mock('../../api/annotations', () => ({
  fetchAnnotations: vi.fn(),
}))

import { fetchAnnotations } from '../../api/annotations'
import { useAnnotations } from '../useAnnotations'

const mockFetch = fetchAnnotations as ReturnType<typeof vi.fn>

beforeEach(() => vi.clearAllMocks())

describe('useAnnotations', () => {
  it('loads annotations for a target on creation', async () => {
    mockFetch.mockResolvedValue([{ id: '1', kind: 'ai_rca', target_id: 'checkout' }])
    const { annotations, loading } = useAnnotations('service', () => 'checkout')
    await flushPromises()
    expect(mockFetch).toHaveBeenCalledWith('service', { targetId: 'checkout' })
    expect(annotations.value).toHaveLength(1)
    expect(loading.value).toBe(false)
  })

  it('captures errors', async () => {
    mockFetch.mockRejectedValue(new Error('boom'))
    const { error } = useAnnotations('trace', () => 't1')
    await flushPromises()
    expect(error.value).toBe('boom')
  })
})
