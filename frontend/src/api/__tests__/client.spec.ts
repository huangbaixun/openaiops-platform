import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import client from '../client'

/**
 * SLICE-3 T15 regression: services.ts + topology.ts originally used raw
 * fetch() and bypassed the shared client's Bearer-injection interceptor.
 * Every SLICE-3 page hit 401 in the running stack.
 *
 * This spec locks in the contract: ANY API module that wants auth MUST
 * import the shared client (api/client.ts). The interceptor reads
 * localStorage.apiKey on every request and stamps the Authorization header.
 *
 * Add the matching shape check to any new API module's test (e.g. assert
 * `client.get` was called, not raw fetch).
 */
describe('api/client', () => {
  beforeEach(() => {
    localStorage.setItem('apiKey', 'test-key-acme')
  })
  afterEach(() => {
    localStorage.removeItem('apiKey')
  })

  it('axios request interceptor injects Bearer header from localStorage', async () => {
    const cfg = await (client.interceptors.request as unknown as {
      handlers: { fulfilled: (cfg: { headers: Record<string, string> }) => unknown }[]
    }).handlers[0].fulfilled({ headers: {} })
    expect((cfg as { headers: { Authorization: string } }).headers.Authorization).toBe(
      'Bearer test-key-acme',
    )
  })

  it('does not set Authorization when no key in localStorage', async () => {
    localStorage.removeItem('apiKey')
    const cfg = await (client.interceptors.request as unknown as {
      handlers: { fulfilled: (cfg: { headers: Record<string, string> }) => unknown }[]
    }).handlers[0].fulfilled({ headers: {} })
    expect((cfg as { headers: { Authorization?: string } }).headers.Authorization).toBeUndefined()
  })

  it('baseURL is /api so callers pass post-strip /v1/* paths', () => {
    expect(client.defaults.baseURL).toBe('/api')
  })
})
