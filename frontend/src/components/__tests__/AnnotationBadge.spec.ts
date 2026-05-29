import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AnnotationBadge from '../AnnotationBadge.vue'

const sample = [
  { id: '1', target_type: 'service', target_id: 'checkout', kind: 'ai_rca',
    payload: { summary: 'db slow' }, ts: '2026-05-29T12:00:00Z', created_at: '2026-05-29T12:00:01Z' },
]

describe('AnnotationBadge', () => {
  it('renders nothing when there are no annotations', () => {
    const w = mount(AnnotationBadge, { props: { annotations: [] } })
    expect(w.find('[data-testid="annotation-badge"]').exists()).toBe(false)
  })

  it('shows the count when annotations exist', () => {
    const w = mount(AnnotationBadge, { props: { annotations: sample } })
    const badge = w.find('[data-testid="annotation-badge"]')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toContain('1')
  })
})
