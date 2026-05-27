import { mount } from '@vue/test-utils'
import { describe, it, expect } from 'vitest'
import ServiceCard from '../ServiceCard.vue'

describe('ServiceCard', () => {
  it('renders error rate as percent', () => {
    const w = mount(ServiceCard, {
      props: { item: { service: 'checkout', inbound_calls: 100, inbound_errors: 2, inbound_error_rate: 0.02, inbound_p95_ms: 50, outbound_calls: 0 } },
    })
    expect(w.find('[data-testid="card-error-rate"]').text()).toContain('2.00%')
    expect(w.find('[data-testid="service-card-checkout"]').exists()).toBe(true)
  })

  it('emits click with service name', async () => {
    const w = mount(ServiceCard, {
      props: { item: { service: 'svc', inbound_calls: 1, inbound_errors: 0, inbound_error_rate: 0, inbound_p95_ms: 1, outbound_calls: 0 } },
    })
    await w.find('[data-testid="service-card-svc"]').trigger('click')
    expect(w.emitted('click')?.[0]).toEqual(['svc'])
  })
})
