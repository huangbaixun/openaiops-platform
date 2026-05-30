import { ref } from 'vue'
import type { ServiceListItem } from '../api/services'

export type CmdItem = {
  type: 'page' | 'service'
  id?: string
  label: string
  hint: string
  keywords: string
  action: () => void
}

const PAGES: { id: string; label: string; path: string }[] = [
  { id: 'overview', label: '服务概览 Overview', path: '/overview' },
  { id: 'traces', label: '调用链 Traces', path: '/traces' },
  { id: 'logs', label: '日志 Logs', path: '/logs' },
  { id: 'topology', label: '拓扑 Topology', path: '/topology' },
]

export function buildItems(services: ServiceListItem[], navigate: (path: string) => void): CmdItem[] {
  const items: CmdItem[] = []
  services.forEach(s => items.push({
    type: 'service', label: s.service, hint: 'service',
    keywords: s.service,
    action: () => navigate(`/services/${s.service}`),
  }))
  PAGES.forEach(p => items.push({
    type: 'page', id: p.id, label: p.label, hint: '跳转',
    keywords: `${p.id} ${p.label}`,
    action: () => navigate(p.path),
  }))
  return items
}

// Ported from OpenAPM js/cmdk.js _cmdkFilter.
export function fuzzyFilter(items: CmdItem[], query: string): CmdItem[] {
  if (!query) return items.slice(0, 50)
  const q = query.toLowerCase().trim()
  const tokens = q.split(/\s+/).filter(Boolean)
  const out: { it: CmdItem; score: number }[] = []
  items.forEach(it => {
    const hay = `${it.label} ${it.keywords}`.toLowerCase()
    if (!tokens.every(tk => hay.includes(tk))) return
    let score = 0
    if (hay.includes(q)) score += 50
    if (it.label.toLowerCase().startsWith(q)) score += 40
    if (it.label.toLowerCase().includes(q)) score += 20
    out.push({ it, score })
  })
  out.sort((a, b) => b.score - a.score)
  return out.slice(0, 50).map(x => x.it)
}

const open = ref(false)
export function useCommandPalette() {
  return {
    open,
    openPalette: () => { open.value = true },
    closePalette: () => { open.value = false },
    togglePalette: () => { open.value = !open.value },
  }
}
