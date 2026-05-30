import { describe, it, expect } from 'vitest'
import { buildItems, fuzzyFilter } from '../useCommandPalette'
import type { CmdItem as _CmdItem } from '../useCommandPalette'

const services = [{ service: 'checkout' }, { service: 'payment' }, { service: 'redis-cache' }]
const navigate = () => {}

describe('useCommandPalette', () => {
  it('builds page items + one item per service', () => {
    const items = buildItems(services as any, navigate)
    expect(items.some(i => i.type === 'page' && i.id === 'traces')).toBe(true)
    expect(items.filter(i => i.type === 'service').map(i => i.label)).toEqual(['checkout', 'payment', 'redis-cache'])
  })

  it('fuzzy matches by token; ranks startsWith higher', () => {
    const items = buildItems(services as any, navigate)
    const hits = fuzzyFilter(items, 'check')
    expect(hits[0].label).toBe('checkout')
  })

  it('empty query returns capped full list', () => {
    const items = buildItems(services as any, navigate)
    expect(fuzzyFilter(items, '').length).toBe(items.length)
  })

  it('no match returns empty', () => {
    const items = buildItems(services as any, navigate)
    expect(fuzzyFilter(items, 'zzzzz')).toEqual([])
  })
})
