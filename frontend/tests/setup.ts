// Provide a proper localStorage implementation for Node 25 compatibility
// Node 25 exposes a partial localStorage (missing clear()) so we override it
const store = new Map<string, string>()
const mockStorage: Storage = {
  get length() { return store.size },
  key(index: number) { return [...store.keys()][index] ?? null },
  getItem(key: string) { return store.get(key) ?? null },
  setItem(key: string, value: string) { store.set(key, String(value)) },
  removeItem(key: string) { store.delete(key) },
  clear() { store.clear() },
}
Object.defineProperty(globalThis, 'localStorage', {
  value: mockStorage,
  writable: true,
  configurable: true,
})
