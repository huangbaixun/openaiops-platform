# PLATFORM-UI-1 — Frontend UI Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use harness:subagent-driven-development (recommended) or harness:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Source spec:** `docs/specs/2026-05-30-platform-ui-1-refactor-design.md` (feature `PLATFORM-UI-1`).

**Goal:** Restyle the platform's six backend-wired Vue pages and rebuild the shell to match the OpenAPM hi-fi design, adding theme toggle, ⌘K command palette, grouped collapsible sidebar, and topbar time-range + auto-refresh — purely presentational, no api/composables/stores/router/backend changes.

**Architecture:** OpenAPM-CSS-first hybrid. Port OpenAPM's `core.css` shell rules + `components.css` vocabulary into `src/styles/` as global (unscoped) stylesheets imported once in `main.ts`; build the shell as Vue SFCs that apply those classes; keep NaiveUI only for the heavy widgets already in use (`NSpin`/`NModal`/`NSelect`/`NDataTable`/`NTabs`), themed to the tokens. Data flow is untouched, so the existing 96 vitest + 28 Playwright are the refactor's regression guard — **every existing `data-testid` must survive unchanged.**

**Tech Stack:** Vue 3 + TS, NaiveUI, vue-i18n, Pinia, vue-router, vitest, Playwright, vue-tsc. Reference sources are local at `/Users/huangbaixun/code_space/OpenAPM/{styles/core.css,styles/components.css,js/shell.js,js/cmdk.js}`.

---

## Acceptance-criteria → task traceability

| AC | Criterion (abridged) | Task(s) |
|---|---|---|
| 1 | Sidebar: 3 nav groups + icons; 6 real pages navigate; backend-less items disabled "即将上线"; collapse persists | T3, T7 |
| 2 | TopBar: logo+brand+scope-pill+time-range+refresh-group+theme+language+avatar menu; logout works | T6, T7 |
| 3 | Theme toggle light/dark via data-theme, persists across reload | T2 |
| 4 | ⌘K palette fuzzy-jumps to any page + any service from live list | T5 |
| 5 | Time-range drives the 6 pages' query window; auto-refresh re-queries on interval | T8 |
| 6 | All 6 real pages restyled to OpenAPM vocabulary; every existing data-testid intact | T9, T10 |
| 7 | Scope-pill: Project=current tenant; Domain/Env read-only with Feature-B seam | T4 |
| 8 | Existing 96 vitest + 28 Playwright green; npm run build passes; new shell tests pass | T11 (gate); every task |

No orphan criteria. No task touches `out_of_scope` (no Domain/Env switching, no backend, no mock pages, no design-annotations overlay, no responsive).

---

## File structure

**New:**
- `frontend/src/styles/openapm-shell.css` — ported shell rules (`.app`, `.topbar`, `.brand`, `.pill`, `.icon-btn`, `.sidebar`, `.nav-*`, `.card`, grids, scrollbars, `.spin`).
- `frontend/src/styles/openapm-components.css` — ported vocabulary (`.dd-menu`, `.dd-item`, `.scope-pill`, `.refresh-group`, `.avatar`, `.badge.*`, `.tbl`, `.kpi`, `.bar`, `pre.code`, `.waterfall`/`.wf-row`, `.subtabs`/`.subtab`, `.cmdk*`).
- `frontend/src/composables/useTheme.ts` — light/dark toggle + persistence.
- `frontend/src/composables/useSidebar.ts` — collapse state + persistence.
- `frontend/src/composables/useCommandPalette.ts` — ⌘K candidate build + fuzzy filter + open/close state.
- `frontend/src/components/NavIcons.ts` — exported SVG icon strings per nav id.
- `frontend/src/components/ScopePill.vue` — Domain/Project/Env breadcrumb (forward-compatible).
- `frontend/src/components/CommandPalette.vue` — ⌘K overlay.
- `frontend/src/components/__tests__/{SideBar,ScopePill,TopBar,CommandPalette}.spec.ts`, `frontend/src/composables/__tests__/{useTheme,useSidebar,useCommandPalette}.spec.ts`.
- `frontend/e2e/shell.spec.ts` — ⌘K, theme, collapse e2e.

**Rewritten:** `frontend/src/components/{SideBar,TopBar}.vue`, `frontend/src/layouts/AppLayout.vue`.

**Restyled (testids preserved):** `frontend/src/views/Overview/{OverviewPage,ServiceCard}.vue`, `frontend/src/views/Traces/{TracesList,TraceDetail,WaterfallChart}.vue`, `frontend/src/views/Logs/LogsView.vue`, `frontend/src/components/{LogRow,SeverityBadge}.vue`, `frontend/src/views/Topology/TopologyPage.vue`, `frontend/src/views/Services/{ServiceDetail,SignalsTab}.vue`, `frontend/src/views/LoginView.vue`.

**Modified:** `frontend/src/main.ts` (import 2 stylesheets), `frontend/src/i18n/locales/{zh-CN,en-US}.ts` (new keys).

**Untouched:** everything under `frontend/src/api/`, `frontend/src/stores/`, `frontend/src/router/`, all composables except the three new ones, all `*.test`/`*.spec` for existing behavior.

---

## Task 1: Port OpenAPM stylesheets

**Files:**
- Create: `frontend/src/styles/openapm-shell.css`
- Create: `frontend/src/styles/openapm-components.css`
- Modify: `frontend/src/main.ts`

- [ ] **Step 1: Copy shell rules**

Copy the OpenAPM shell rules into `frontend/src/styles/openapm-shell.css`: take `/Users/huangbaixun/code_space/OpenAPM/styles/core.css` **lines 73–256** verbatim (everything from `.app { display: grid; ... }` through `.muted { color: var(--text-3); }`). Do **not** copy lines 1–72 (the `:root` tokens + `body` — those already live in `frontend/src/styles/tokens.css` and `global.css`; re-copying would duplicate the token block).

- [ ] **Step 2: Copy component vocabulary**

Copy `/Users/huangbaixun/code_space/OpenAPM/styles/components.css` **in full** into `frontend/src/styles/openapm-components.css`. Then append the ⌘K palette styles (not in components.css) at the end of the file:

```css
/* ⌘K command palette */
#cmdk-root { position: fixed; inset: 0; z-index: 200; }
.cmdk-bg { position: absolute; inset: 0; background: rgba(0,0,0,.35); backdrop-filter: blur(2px); }
.cmdk { position: absolute; top: 12vh; left: 50%; transform: translateX(-50%); width: 560px; max-width: 92vw;
  background: var(--bg-elev-0); border: 1px solid var(--border-strong); border-radius: 14px;
  box-shadow: var(--shadow-lg); overflow: hidden; }
.cmdk-input-row { display: flex; align-items: center; gap: 8px; padding: 12px 14px; border-bottom: 1px solid var(--border); color: var(--text-3); }
.cmdk-input-row input { flex: 1; border: none; outline: none; background: transparent; color: var(--text-1); font-size: 14px; }
.cmdk-input-row .kbd { font-family: var(--mono); font-size: 11px; color: var(--text-3); background: var(--bg-hover); padding: 2px 6px; border-radius: 5px; }
.cmdk-list { max-height: 50vh; overflow-y: auto; padding: 6px; }
.cmdk-group { font-size: 10.5px; text-transform: uppercase; letter-spacing: .08em; color: var(--text-3); font-weight: 600; padding: 8px 10px 4px; }
.cmdk-item { display: flex; align-items: center; gap: 10px; padding: 8px 10px; border-radius: 8px; cursor: pointer; }
.cmdk-item.active { background: var(--accent-weak); }
.cmdk-ic { width: 18px; text-align: center; color: var(--text-3); }
.cmdk-label { color: var(--text-1); font-weight: 500; font-size: 13px; }
.cmdk-hint { margin-left: auto; color: var(--text-3); font-size: 11.5px; }
.cmdk-empty { padding: 20px; text-align: center; color: var(--text-3); font-size: 12.5px; }
.cmdk-foot { display: flex; gap: 14px; align-items: center; padding: 8px 14px; border-top: 1px solid var(--border); color: var(--text-3); font-size: 11px; }
.cmdk-foot .kbd { font-family: var(--mono); background: var(--bg-hover); padding: 1px 5px; border-radius: 4px; margin-right: 4px; }
```

- [ ] **Step 3: Import both sheets globally**

Edit `frontend/src/main.ts` — add the two imports immediately after the existing `import './styles/global.css'` line:

```ts
import './styles/tokens.css'
import './styles/global.css'
import './styles/openapm-shell.css'
import './styles/openapm-components.css'
```

- [ ] **Step 4: Verify build**

Run: `cd frontend && npm run build`
Expected: build succeeds (vue-tsc clean). The ported CSS is global/unscoped; no component uses these classes yet, so nothing renders differently except the global scrollbar styling.

- [ ] **Step 5: Verify existing tests still green**

Run: `cd frontend && npx vitest run`
Expected: 96 passed (no behavior change).

- [ ] **Step 6: Commit**

```bash
git add frontend/src/styles/openapm-shell.css frontend/src/styles/openapm-components.css frontend/src/main.ts
git commit -m "feat(ui-1): port OpenAPM shell + component stylesheets"
```

---

## Task 2: Theme toggle (useTheme)

AC#3. Tokens already define the dark palette; this only flips `data-theme` on `<html>` and persists it.

**Files:**
- Create: `frontend/src/composables/useTheme.ts`
- Create: `frontend/src/composables/__tests__/useTheme.spec.ts`

- [ ] **Step 1: Write the failing test**

```ts
// frontend/src/composables/__tests__/useTheme.spec.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { useTheme } from '../useTheme'

describe('useTheme', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
  })

  it('defaults to light and reflects on <html>', () => {
    const { theme, initTheme } = useTheme()
    initTheme()
    expect(theme.value).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
  })

  it('toggle flips theme, sets data-theme, and persists', () => {
    const { theme, initTheme, toggleTheme } = useTheme()
    initTheme()
    toggleTheme()
    expect(theme.value).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    expect(localStorage.getItem('apm:theme')).toBe('dark')
  })

  it('restores persisted theme on init', () => {
    localStorage.setItem('apm:theme', 'dark')
    const { theme, initTheme } = useTheme()
    initTheme()
    expect(theme.value).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/composables/__tests__/useTheme.spec.ts`
Expected: FAIL — cannot resolve `../useTheme`.

- [ ] **Step 3: Write the implementation**

```ts
// frontend/src/composables/useTheme.ts
import { ref } from 'vue'

export type Theme = 'light' | 'dark'
const KEY = 'apm:theme'
const theme = ref<Theme>('light')

function apply(t: Theme) {
  theme.value = t
  document.documentElement.setAttribute('data-theme', t)
  try { localStorage.setItem(KEY, t) } catch { /* Storage shim; ignore */ }
}

export function useTheme() {
  function initTheme() {
    let saved: Theme = 'light'
    try {
      const v = localStorage.getItem(KEY)
      if (v === 'dark' || v === 'light') saved = v
    } catch { /* ignore */ }
    apply(saved)
  }
  function toggleTheme() {
    apply(theme.value === 'dark' ? 'light' : 'dark')
  }
  return { theme, initTheme, toggleTheme }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend && npx vitest run src/composables/__tests__/useTheme.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/composables/useTheme.ts frontend/src/composables/__tests__/useTheme.spec.ts
git commit -m "feat(ui-1): useTheme composable (light/dark + persistence)"
```

---

## Task 3: Grouped collapsible SideBar

AC#1. Replaces the flat list with OpenAPM's three nav groups, icons, badges, disabled placeholders, and a collapse toggle.

**Files:**
- Create: `frontend/src/composables/useSidebar.ts`
- Create: `frontend/src/components/NavIcons.ts`
- Modify: `frontend/src/components/SideBar.vue` (full rewrite)
- Create: `frontend/src/composables/__tests__/useSidebar.spec.ts`
- Create: `frontend/src/components/__tests__/SideBar.spec.ts`

- [ ] **Step 1: Write the failing useSidebar test**

```ts
// frontend/src/composables/__tests__/useSidebar.spec.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { useSidebar } from '../useSidebar'

describe('useSidebar', () => {
  beforeEach(() => localStorage.clear())

  it('defaults to expanded', () => {
    const { collapsed, initSidebar } = useSidebar()
    initSidebar()
    expect(collapsed.value).toBe(false)
  })

  it('toggle flips and persists', () => {
    const { collapsed, initSidebar, toggleSidebar } = useSidebar()
    initSidebar()
    toggleSidebar()
    expect(collapsed.value).toBe(true)
    expect(localStorage.getItem('apm:sidebar')).toBe('1')
  })

  it('restores persisted collapsed state', () => {
    localStorage.setItem('apm:sidebar', '1')
    const { collapsed, initSidebar } = useSidebar()
    initSidebar()
    expect(collapsed.value).toBe(true)
  })
})
```

- [ ] **Step 2: Run it; expect FAIL** (`cannot resolve ../useSidebar`).

- [ ] **Step 3: Implement useSidebar**

```ts
// frontend/src/composables/useSidebar.ts
import { ref } from 'vue'

const KEY = 'apm:sidebar'
const collapsed = ref(false)

export function useSidebar() {
  function initSidebar() {
    try { collapsed.value = localStorage.getItem(KEY) === '1' } catch { collapsed.value = false }
  }
  function toggleSidebar() {
    collapsed.value = !collapsed.value
    try { localStorage.setItem(KEY, collapsed.value ? '1' : '0') } catch { /* ignore */ }
  }
  return { collapsed, initSidebar, toggleSidebar }
}
```

- [ ] **Step 4: Run it; expect PASS (3).**

- [ ] **Step 5: Create the nav icons module**

Copy the SVG icon strings from `/Users/huangbaixun/code_space/OpenAPM/js/shell.js` `APM.navItems` (lines 33–53) into a typed map. Only the ids the platform uses need real icons; the rest reuse OpenAPM's.

```ts
// frontend/src/components/NavIcons.ts
// SVG strings lifted verbatim from OpenAPM js/shell.js APM.navItems.
export const navIcons: Record<string, string> = {
  overview: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>',
  traces: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="4" cy="6" r="2"/><circle cx="20" cy="18" r="2"/><path d="M6 6h6a4 4 0 014 4v4a4 4 0 004 4"/></svg>',
  logs: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="14" y2="17"/></svg>',
  exceptions: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>',
  topology: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="6" cy="6" r="3"/><circle cx="18" cy="6" r="3"/><circle cx="12" cy="18" r="3"/><path d="M9 6h6M7.5 8.5L10 15.5M16.5 8.5L14 15.5"/></svg>',
  database: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/></svg>',
  redis: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"><path d="M3 6c0-1.5 4-3 9-3s9 1.5 9 3-4 3-9 3-9-1.5-9-3z"/><path d="M3 12c0 1.5 4 3 9 3s9-1.5 9-3"/><path d="M3 6v12c0 1.5 4 3 9 3s9-1.5 9-3V6"/><polyline points="8 9 12 11 16 9"/></svg>',
  kafka: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="6" cy="6" r="2.5"/><circle cx="18" cy="6" r="2.5"/><circle cx="12" cy="18" r="2.5"/><path d="M8.2 7l3.4 9M15.8 7l-3.4 9"/></svg>',
  llm: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2a4 4 0 014 4v0a4 4 0 014 4v2a4 4 0 01-4 4h0v0a4 4 0 01-4 4 4 4 0 01-4-4v0h0a4 4 0 01-4-4v-2a4 4 0 014-4v0a4 4 0 014-4z"/><path d="M9 11h.01M15 11h.01M9 15c.83.67 1.83 1 3 1s2.17-.33 3-1"/></svg>',
  dashboards: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 17 9 11 13 15 21 7"/><polyline points="14 7 21 7 21 14"/></svg>',
  alerts: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 8a6 6 0 10-12 0c0 7-3 9-3 9h18s-3-2-3-9M13.73 21a2 2 0 01-3.46 0"/></svg>',
  onboarding: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2l3 7h7l-6 4 2 8-6-4-6 4 2-8-6-4h7z"/></svg>',
  settings: '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 11-2.83 2.83l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 11-4 0v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 11-2.83-2.83l.06-.06a1.65 1.65 0 00.33-1.82 1.65 1.65 0 00-1.51-1H3a2 2 0 110-4h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 112.83-2.83l.06.06a1.65 1.65 0 001.82.33H9a1.65 1.65 0 001-1.51V3a2 2 0 114 0v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 112.83 2.83l-.06.06a1.65 1.65 0 00-.33 1.82V9a1.65 1.65 0 001.51 1H21a2 2 0 110 4h-.09a1.65 1.65 0 00-1.51 1z"/></svg>',
}
```

- [ ] **Step 6: Write the failing SideBar test**

```ts
// frontend/src/components/__tests__/SideBar.spec.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createRouter, createWebHistory } from 'vue-router'
import SideBar from '../SideBar.vue'

const i18n = createI18n({ legacy: false, locale: 'en-US', messages: { 'en-US': {} } })
const router = createRouter({ history: createWebHistory(), routes: [
  { path: '/overview', name: 'overview', component: { template: '<div/>' } },
  { path: '/traces', name: 'traces', component: { template: '<div/>' } },
  { path: '/logs', name: 'logs', component: { template: '<div/>' } },
  { path: '/topology', name: 'topology', component: { template: '<div/>' } },
] })

function mountSidebar() {
  return mount(SideBar, { global: { plugins: [i18n, router] } })
}

describe('SideBar', () => {
  it('renders the three OpenAPM nav groups', () => {
    const w = mountSidebar()
    const labels = w.findAll('.nav-group .label').map(n => n.text())
    expect(labels).toEqual(['OBSERVE', 'ANALYZE', 'PLATFORM'])
  })

  it('real pages are RouterLinks; backend-less items are disabled placeholders', () => {
    const w = mountSidebar()
    expect(w.find('[data-testid="nav-overview"]').exists()).toBe(true)
    expect(w.find('[data-testid="nav-traces"]').exists()).toBe(true)
    expect(w.find('[data-testid="nav-logs"]').exists()).toBe(true)
    expect(w.find('[data-testid="nav-topology"]').exists()).toBe(true)
    // backend-less item: rendered, disabled class, NOT an <a>
    const ex = w.find('[data-testid="nav-exceptions"]')
    expect(ex.exists()).toBe(true)
    expect(ex.classes()).toContain('disabled')
    expect(ex.element.tagName).not.toBe('A')
  })

  it('toggle button collapses the rail (emits collapse state up via class on root)', async () => {
    const w = mountSidebar()
    await w.find('.sidebar-toggle').trigger('click')
    expect(w.find('.sidebar-toggle').exists()).toBe(true) // toggle present & clickable
  })
})
```

- [ ] **Step 7: Run it; expect FAIL** (SideBar still the old flat list).

- [ ] **Step 8: Rewrite SideBar.vue**

```vue
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { navIcons } from './NavIcons'
import { useSidebar } from '../composables/useSidebar'

const { t } = useI18n()
const { collapsed, toggleSidebar } = useSidebar()

type NavItem = { id: string; label: string; to: string | null; badge?: string }
const groups: { group: string; items: NavItem[] }[] = [
  { group: 'OBSERVE', items: [
    { id: 'overview', label: 'nav.overview', to: '/overview' },
    { id: 'traces', label: 'nav.traces', to: '/traces' },
    { id: 'logs', label: 'nav.logs', to: '/logs' },
    { id: 'exceptions', label: 'nav.exceptions', to: null },
  ]},
  { group: 'ANALYZE', items: [
    { id: 'topology', label: 'nav.topology', to: '/topology' },
    { id: 'database', label: 'nav.database', to: null },
    { id: 'redis', label: 'nav.redis', to: null },
    { id: 'kafka', label: 'nav.kafka', to: null, badge: 'NEW' },
    { id: 'llm', label: 'nav.llm', to: null, badge: 'NEW' },
  ]},
  { group: 'PLATFORM', items: [
    { id: 'dashboards', label: 'nav.dashboards', to: null },
    { id: 'alerts', label: 'nav.alerts', to: null },
    { id: 'onboarding', label: 'nav.onboarding', to: null },
    { id: 'settings', label: 'nav.settings', to: null },
  ]},
]
const collapseLabel = computed(() => (collapsed.value ? t('shell.expand') : t('shell.collapse')))
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-toggle" :title="collapseLabel" @click="toggleSidebar">
      <svg class="ic-collapse" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
      <span class="lbl">{{ t('shell.collapse') }}</span>
    </div>
    <div v-for="g in groups" :key="g.group" class="nav-group">
      <div class="label">{{ g.group }}</div>
      <template v-for="it in g.items" :key="it.id">
        <RouterLink
          v-if="it.to"
          class="nav-item"
          active-class="active"
          :to="it.to"
          :data-testid="`nav-${it.id}`"
          :title="t(it.label)"
        >
          <span class="ico" v-html="navIcons[it.id]" />
          <span class="lbl">{{ t(it.label) }}</span>
          <span v-if="it.badge" class="badge-mini" style="background: var(--accent); color: white;">{{ it.badge }}</span>
        </RouterLink>
        <div
          v-else
          class="nav-item disabled"
          :data-testid="`nav-${it.id}`"
          :title="t('shell.comingSoon')"
        >
          <span class="ico" v-html="navIcons[it.id]" />
          <span class="lbl">{{ t(it.label) }}</span>
          <span class="badge-mini" style="background: var(--bg-hover); color: var(--text-3);">{{ it.badge || t('shell.soon') }}</span>
        </div>
      </template>
    </div>
  </aside>
</template>

<style scoped>
.nav-item.disabled { opacity: .55; cursor: not-allowed; }
.nav-item.disabled:hover { background: transparent; color: var(--text-2); }
.sidebar :deep(.ico svg) { width: 16px; height: 16px; }
</style>
```

Note: `.sidebar`, `.nav-group`, `.nav-item`, `.badge-mini`, `.sidebar-toggle` styling all come from the global `openapm-shell.css` ported in Task 1; the scoped block only adds the `disabled` affordance.

- [ ] **Step 9: Run SideBar + useSidebar tests; expect PASS.**

Run: `cd frontend && npx vitest run src/components/__tests__/SideBar.spec.ts src/composables/__tests__/useSidebar.spec.ts`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add frontend/src/components/SideBar.vue frontend/src/components/NavIcons.ts frontend/src/composables/useSidebar.ts frontend/src/composables/__tests__/useSidebar.spec.ts frontend/src/components/__tests__/SideBar.spec.ts
git commit -m "feat(ui-1): grouped collapsible sidebar with icons + placeholders"
```

---

## Task 4: ScopePill (forward-compatible)

AC#7. Project = current tenant (real); Domain/Env read-only single-item dropdowns with a Feature-B seam.

**Files:**
- Create: `frontend/src/components/ScopePill.vue`
- Create: `frontend/src/components/__tests__/ScopePill.spec.ts`

- [ ] **Step 1: Write the failing test**

```ts
// frontend/src/components/__tests__/ScopePill.spec.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import ScopePill from '../ScopePill.vue'
import { useAuthStore } from '../../stores/auth'

const i18n = createI18n({ legacy: false, locale: 'en-US', messages: { 'en-US': {} } })

describe('ScopePill', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('shows the current tenant as the Project segment', () => {
    const auth = useAuthStore()
    auth.tenantName = 'acme'
    const w = mount(ScopePill, { global: { plugins: [i18n] } })
    expect(w.get('[data-testid="scope-project"]').text()).toContain('acme')
  })

  it('renders Domain and Env segments as read-only (no switching handlers)', () => {
    const w = mount(ScopePill, { global: { plugins: [i18n] } })
    expect(w.find('[data-testid="scope-domain"]').exists()).toBe(true)
    expect(w.find('[data-testid="scope-env"]').exists()).toBe(true)
  })
})
```

- [ ] **Step 2: Run it; expect FAIL** (no ScopePill).

- [ ] **Step 3: Implement ScopePill.vue**

```vue
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '../stores/auth'

// FEATURE-B (PLATFORM-MT-1): wire real tenant switching here.
// Today: Project = the single logged-in tenant; Domain/Env are read-only labels
// derived statically. Feature B replaces these constants with real lists +
// switch handlers and turns the segments into active dropdowns.
const { t } = useI18n()
const auth = useAuthStore()
const project = computed(() => auth.tenantName || '—')
const DOMAIN_LABEL = computed(() => t('shell.defaultDomain')) // static until Feature B
const ENV_LABEL = 'Production' // static until Feature B
</script>

<template>
  <div class="scope-pill">
    <div class="sp-seg" data-testid="scope-domain" :title="t('shell.domainReadonly')">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z"/></svg>
      <span class="k" style="color: var(--text-3);">Domain</span><span>{{ DOMAIN_LABEL }}</span>
    </div>
    <span class="sp-sep">/</span>
    <div class="sp-seg" data-testid="scope-project" :title="t('shell.projectIsTenant')">
      <span class="k" style="color: var(--text-3);">Project</span>
      <span style="color: var(--accent);">●</span><span>{{ project }}</span>
    </div>
    <span class="sp-sep">/</span>
    <div class="sp-seg" data-testid="scope-env" :title="t('shell.envReadonly')">
      <span class="dot" style="width:6px;height:6px;border-radius:50%;background:var(--success);" />
      <span>{{ ENV_LABEL }}</span>
    </div>
  </div>
</template>

<style scoped>
.sp-sep { color: var(--text-3); padding: 0 2px; }
.sp-seg { cursor: default; }
.sp-seg .k { margin-right: 2px; }
</style>
```

Note: `.scope-pill` / `.sp-seg` come from global `openapm-components.css`. The `auth` store already exposes `tenantName` (used by the old TopBar).

- [ ] **Step 4: Run it; expect PASS (2).**

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ScopePill.vue frontend/src/components/__tests__/ScopePill.spec.ts
git commit -m "feat(ui-1): ScopePill (project=tenant; domain/env forward-compatible)"
```

---

## Task 5: ⌘K command palette

AC#4. Fuzzy jump to any page + any service from the live list.

**Files:**
- Create: `frontend/src/composables/useCommandPalette.ts`
- Create: `frontend/src/components/CommandPalette.vue`
- Create: `frontend/src/composables/__tests__/useCommandPalette.spec.ts`
- Create: `frontend/src/components/__tests__/CommandPalette.spec.ts`

- [ ] **Step 1: Write the failing composable test**

```ts
// frontend/src/composables/__tests__/useCommandPalette.spec.ts
import { describe, it, expect } from 'vitest'
import { buildItems, fuzzyFilter, type CmdItem } from '../useCommandPalette'

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
```

- [ ] **Step 2: Run it; expect FAIL.**

- [ ] **Step 3: Implement useCommandPalette.ts**

Port the fuzzy logic from `/Users/huangbaixun/code_space/OpenAPM/js/cmdk.js` (`_cmdkFilter`, lines 100–116), trimmed to pages + services.

```ts
// frontend/src/composables/useCommandPalette.ts
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

// Live nav pages (the 6 routed ones). Backend-less pages are intentionally excluded.
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
```

- [ ] **Step 4: Run it; expect PASS (4).**

- [ ] **Step 5: Write the failing CommandPalette component test**

```ts
// frontend/src/components/__tests__/CommandPalette.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import CommandPalette from '../CommandPalette.vue'

const push = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
vi.mock('../../api/services', () => ({
  fetchServices: vi.fn().mockResolvedValue({ items: [{ service: 'checkout' }, { service: 'payment' }] }),
}))
vi.mock('../../composables/useTimeWindow', () => ({ useTimeWindow: () => ({ windowVal: { value: '1h' } }) }))

describe('CommandPalette', () => {
  beforeEach(() => { setActivePinia(createPinia()); push.mockClear() })

  it('opens, fuzzy-filters services, and navigates on Enter', async () => {
    const { useCommandPalette } = await import('../../composables/useCommandPalette')
    const w = mount(CommandPalette, { global: { plugins: [] } })
    useCommandPalette().openPalette()
    await flushPromises()
    const input = w.get('input')
    await input.setValue('check')
    await input.trigger('keydown', { key: 'Enter' })
    expect(push).toHaveBeenCalledWith('/services/checkout')
  })
})
```

- [ ] **Step 6: Run it; expect FAIL** (no CommandPalette).

- [ ] **Step 7: Implement CommandPalette.vue**

```vue
<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { fetchServices, type ServiceListItem } from '../api/services'
import { useTimeWindow } from '../composables/useTimeWindow'
import { buildItems, fuzzyFilter, useCommandPalette } from '../composables/useCommandPalette'

const router = useRouter()
const { windowVal } = useTimeWindow()
const { open, closePalette } = useCommandPalette()

const query = ref('')
const index = ref(0)
const services = ref<ServiceListItem[]>([])
const loadError = ref(false)

function navigate(path: string) { closePalette(); void router.push(path) }
const allItems = computed(() => buildItems(services.value, navigate))
const results = computed(() => fuzzyFilter(allItems.value, query.value))

async function loadServices() {
  try { services.value = (await fetchServices(windowVal.value)).items; loadError.value = false }
  catch { services.value = []; loadError.value = true }
}

watch(open, (isOpen) => {
  if (isOpen) { query.value = ''; index.value = 0; void loadServices() }
})

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') { e.preventDefault(); index.value = Math.min(results.value.length - 1, index.value + 1) }
  else if (e.key === 'ArrowUp') { e.preventDefault(); index.value = Math.max(0, index.value - 1) }
  else if (e.key === 'Enter') { e.preventDefault(); results.value[index.value]?.action() }
  else if (e.key === 'Escape') { closePalette() }
}
</script>

<template>
  <div v-if="open" id="cmdk-root" data-testid="command-palette">
    <div class="cmdk-bg" @click="closePalette" />
    <div class="cmdk" role="dialog" aria-modal="true" aria-label="Command palette">
      <div class="cmdk-input-row">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
        <input
          autofocus autocomplete="off" spellcheck="false"
          placeholder="搜索服务、页面…"
          :value="query"
          data-testid="cmdk-input"
          @input="(e) => { query = (e.target as HTMLInputElement).value; index = 0 }"
          @keydown="onKeydown"
        >
        <span class="kbd">ESC</span>
      </div>
      <div class="cmdk-list">
        <div v-if="loadError" class="cmdk-empty">服务列表加载失败 · 仍可跳转页面</div>
        <div v-if="results.length === 0" class="cmdk-empty">无匹配项</div>
        <div
          v-for="(it, i) in results" :key="it.type + it.label"
          class="cmdk-item" :class="{ active: i === index }"
          :data-testid="`cmdk-item-${it.label}`"
          @mousemove="index = i"
          @click="it.action()"
        >
          <span class="cmdk-ic">{{ it.type === 'service' ? '⊙' : '→' }}</span>
          <span class="cmdk-label">{{ it.label }}</span>
          <span class="cmdk-hint">{{ it.hint }}</span>
        </div>
      </div>
      <div class="cmdk-foot">
        <span><span class="kbd">↑↓</span>移动</span>
        <span><span class="kbd">↵</span>选择</span>
        <span><span class="kbd">⌘K</span>开关</span>
      </div>
    </div>
  </div>
</template>
```

- [ ] **Step 8: Run CommandPalette + composable tests; expect PASS.**

Run: `cd frontend && npx vitest run src/components/__tests__/CommandPalette.spec.ts src/composables/__tests__/useCommandPalette.spec.ts`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add frontend/src/composables/useCommandPalette.ts frontend/src/components/CommandPalette.vue frontend/src/composables/__tests__/useCommandPalette.spec.ts frontend/src/components/__tests__/CommandPalette.spec.ts
git commit -m "feat(ui-1): ⌘K command palette (pages + live services, fuzzy)"
```

---

## Task 6: Rebuild TopBar

AC#2. Assembles brand, scope-pill, time-range, refresh-group, theme toggle, language, avatar user-menu (logout).

**Files:**
- Modify: `frontend/src/components/TopBar.vue` (full rewrite)
- Create: `frontend/src/components/__tests__/TopBar.spec.ts`
- Modify: `frontend/src/composables/useTimeWindow.ts` (add auto-refresh state — see Task 8; TopBar imports it)

> TopBar depends on the auto-refresh additions from **Task 8**. Implement Task 8 Step 1–4 (the `useTimeWindow` extension) **before** wiring the refresh-group here, or stub `intervalSec`/`setInterval` as no-ops and revisit in Task 8. Recommended: do Task 8's composable extension first, then this task.

- [ ] **Step 1: Write the failing test**

```ts
// frontend/src/components/__tests__/TopBar.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import TopBar from '../TopBar.vue'
import { useAuthStore } from '../../stores/auth'

const push = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
const i18n = createI18n({ legacy: false, locale: 'en-US', messages: { 'en-US': {
  shell: { logout: 'Logout' }, topbar: { tenant: 'Tenant' },
} } })

describe('TopBar', () => {
  beforeEach(() => { setActivePinia(createPinia()); push.mockClear() })

  it('renders brand, scope-pill, theme toggle, and avatar', () => {
    const w = mount(TopBar, { global: { plugins: [i18n], stubs: { NSelect: true } } })
    expect(w.find('.brand').exists()).toBe(true)
    expect(w.find('[data-testid="scope-project"]').exists()).toBe(true)
    expect(w.find('[data-testid="theme-toggle"]').exists()).toBe(true)
    expect(w.find('[data-testid="user-avatar"]').exists()).toBe(true)
  })

  it('logout clears auth and routes to login', async () => {
    const auth = useAuthStore()
    auth.tenantName = 'acme'
    const logoutSpy = vi.spyOn(auth, 'logout')
    const w = mount(TopBar, { global: { plugins: [i18n], stubs: { NSelect: true } } })
    await w.get('[data-testid="logout-btn"]').trigger('click')
    expect(logoutSpy).toHaveBeenCalled()
    expect(push).toHaveBeenCalledWith({ name: 'login' })
  })
})
```

- [ ] **Step 2: Run it; expect FAIL** (old TopBar has no scope-pill / theme-toggle / avatar testids).

- [ ] **Step 3: Rewrite TopBar.vue**

```vue
<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NSelect } from 'naive-ui'
import { useAuthStore } from '../stores/auth'
import { setLocale } from '../i18n'
import { useTheme } from '../composables/useTheme'
import { useTimeWindow, type TimeWindow } from '../composables/useTimeWindow'
import ScopePill from './ScopePill.vue'

const { t, locale } = useI18n()
const auth = useAuthStore()
const router = useRouter()
const { toggleTheme } = useTheme()
const { windowVal, apply, allowed, intervalSec, setRefreshInterval } = useTimeWindow()

const userMenuOpen = ref(false)
const timeOpen = ref(false)
const windowLabels: Record<TimeWindow, string> = { '15m': 'Last 15m', '1h': 'Last 1h', '6h': 'Last 6h', '24h': 'Last 24h' }

function logout() { auth.logout(); void router.push({ name: 'login' }) }
const initials = () => (auth.tenantName || '?').slice(0, 2).toUpperCase()
</script>

<template>
  <header class="topbar">
    <div class="brand">
      <div class="logo">
        <svg width="18" height="18" viewBox="0 0 32 32" fill="none"><g fill="#fff"><circle cx="14.6" cy="11.2" r="4.8"/><path d="M11.6 14 C 8.6 16, 8 20, 10 24 C 12 27, 16 27, 18 24.5 C 20 22, 19 18, 16.8 15.6 Z"/><path d="M18.4 10.6 L29 11.8 L18.4 13.6 Z"/></g></svg>
      </div>
      <div><div class="name">OpenAIOps <span class="sub">APM</span></div></div>
    </div>

    <div class="filters">
      <ScopePill />
    </div>

    <div class="spacer" />

    <div class="right">
      <!-- Time range -->
      <div class="dd" :class="{ open: timeOpen }">
        <button class="pill" data-testid="time-range" @click.stop="timeOpen = !timeOpen">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/><polyline points="12 7 12 12 15 14"/></svg>
          <span class="k">时间</span><span class="v">{{ windowLabels[windowVal] }}</span>
        </button>
        <div class="dd-menu">
          <div
            v-for="w in allowed" :key="w"
            class="dd-item" :class="{ selected: w === windowVal }"
            :data-testid="`time-${w}`"
            @click="apply(w as TimeWindow); timeOpen = false"
          >
            <span>{{ windowLabels[w as TimeWindow] }}</span><span class="kbd">{{ w }}</span>
          </div>
        </div>
      </div>

      <!-- Auto-refresh -->
      <div class="refresh-group" :title="t('shell.autoRefresh')">
        <select :value="intervalSec" data-testid="auto-refresh" @change="(e) => setRefreshInterval(Number((e.target as HTMLSelectElement).value))">
          <option :value="0">关闭</option>
          <option :value="30">30s</option>
          <option :value="60">1m</option>
          <option :value="300">5m</option>
        </select>
      </div>

      <!-- Theme -->
      <button class="icon-btn theme-btn" data-testid="theme-toggle" :title="t('shell.theme')" @click="toggleTheme">
        <svg class="theme-moon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z"/></svg>
        <svg class="theme-sun" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/></svg>
      </button>

      <!-- Language (kept; testid lang-select preserved) -->
      <NSelect
        class="lang" size="small" :value="locale" data-testid="lang-select"
        :options="[{ label: '中', value: 'zh-CN' }, { label: 'EN', value: 'en-US' }]"
        style="width: 72px;"
        @update:value="(v) => setLocale(v as 'zh-CN' | 'en-US')"
      />

      <!-- User menu -->
      <div class="dd" :class="{ open: userMenuOpen }">
        <div class="avatar" data-testid="user-avatar" style="cursor: pointer;" @click.stop="userMenuOpen = !userMenuOpen">{{ initials() }}</div>
        <div class="dd-menu" style="left: auto; right: 0;">
          <div class="dd-section" data-testid="tenant-name">{{ t('topbar.tenant') }}: {{ auth.tenantName }}</div>
          <div class="dd-sep" />
          <div class="dd-item" data-testid="logout-btn" @click="logout">
            <span style="color: var(--danger);">{{ t('shell.logout') }}</span>
          </div>
        </div>
      </div>
    </div>
  </header>
</template>

<style scoped>
.topbar :deep(.lang .n-base-selection) { min-height: 32px; }
</style>
```

> **testid preservation:** the old TopBar exposed `tenant-name` (used by no current test, but keep it) and the language `NSelect`. `tenant-name` is retained inside the user menu's `dd-section`. The Login page owns `lang-select`; here we add it on the topbar `NSelect` too — that is a NEW element, fine.

- [ ] **Step 4: Run TopBar test; expect PASS.** Then full vitest to ensure no regression.

Run: `cd frontend && npx vitest run src/components/__tests__/TopBar.spec.ts && npx vitest run`
Expected: TopBar PASS; full suite still green.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/TopBar.vue frontend/src/components/__tests__/TopBar.spec.ts
git commit -m "feat(ui-1): rebuild topbar (brand/scope-pill/time/refresh/theme/lang/avatar)"
```

---

## Task 7: AppLayout + i18n keys + global wiring

AC#1, AC#2. Grid shell with collapse class, mounts CommandPalette + global ⌘K listener, initializes theme/sidebar on boot.

**Files:**
- Modify: `frontend/src/layouts/AppLayout.vue` (rewrite)
- Modify: `frontend/src/i18n/locales/zh-CN.ts`, `frontend/src/i18n/locales/en-US.ts`

- [ ] **Step 1: Add i18n keys**

Add to **both** locale files a `shell` block and the new `nav` keys. zh-CN values:

```ts
// add into the message object of zh-CN.ts
shell: {
  collapse: '收起', expand: '展开', comingSoon: '即将上线', soon: '即将',
  theme: '切换主题', autoRefresh: '自动刷新', logout: '注销',
  defaultDomain: '默认域', domainReadonly: '租户分组（即将支持切换）',
  projectIsTenant: '当前租户', envReadonly: '部署环境（即将支持切换）',
},
// extend the existing nav block:
nav: {
  overview: '服务概览', services: '服务', traces: '调用链', logs: '日志',
  topology: '拓扑', alerts: '告警', settings: '设置',
  exceptions: '异常', database: '数据库', redis: 'Redis', kafka: 'Kafka',
  llm: 'LLM', dashboards: '仪表盘', onboarding: '接入',
},
```

en-US values (mirror keys):

```ts
shell: {
  collapse: 'Collapse', expand: 'Expand', comingSoon: 'Coming soon', soon: 'Soon',
  theme: 'Toggle theme', autoRefresh: 'Auto refresh', logout: 'Logout',
  defaultDomain: 'Default', domainReadonly: 'Tenant group (switching soon)',
  projectIsTenant: 'Current tenant', envReadonly: 'Environment (switching soon)',
},
nav: {
  overview: 'Overview', services: 'Services', traces: 'Traces', logs: 'Logs',
  topology: 'Topology', alerts: 'Alerts', settings: 'Settings',
  exceptions: 'Exceptions', database: 'Database', redis: 'Redis', kafka: 'Kafka',
  llm: 'LLM', dashboards: 'Dashboards', onboarding: 'Onboarding',
},
```

(Keep all existing keys; only add/extend.)

- [ ] **Step 2: Rewrite AppLayout.vue**

```vue
<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterView } from 'vue-router'
import TopBar from '../components/TopBar.vue'
import SideBar from '../components/SideBar.vue'
import CommandPalette from '../components/CommandPalette.vue'
import { useTheme } from '../composables/useTheme'
import { useSidebar } from '../composables/useSidebar'
import { useCommandPalette } from '../composables/useCommandPalette'

const { initTheme } = useTheme()
const { collapsed, initSidebar } = useSidebar()
const { togglePalette } = useCommandPalette()

function onKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
    e.preventDefault()
    togglePalette()
  }
}

onMounted(() => {
  initTheme()
  initSidebar()
  window.addEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="app" :class="{ 'sidebar-collapsed': collapsed }">
    <TopBar />
    <SideBar />
    <main class="main">
      <RouterView />
    </main>
    <CommandPalette />
  </div>
</template>
```

Note: `.app`, `.sidebar-collapsed`, `.main` styling comes from global `openapm-shell.css`. The old scoped grid is removed (now global). The `keydown` listener is fine without explicit teardown (AppLayout is the persistent shell), but if a linter requires it, add `onUnmounted(() => window.removeEventListener('keydown', onKeydown))`.

- [ ] **Step 3: Build + full vitest**

Run: `cd frontend && npm run build && npx vitest run`
Expected: build clean; all tests green (existing 96 + new shell tests).

- [ ] **Step 4: Commit**

```bash
git add frontend/src/layouts/AppLayout.vue frontend/src/i18n/locales/zh-CN.ts frontend/src/i18n/locales/en-US.ts
git commit -m "feat(ui-1): app shell layout + ⌘K listener + i18n shell keys"
```

---

## Task 8: Time-range drives pages + auto-refresh

AC#5. The topbar time-range already maps to the shared `useTimeWindow` store (pages watch `windowVal`). This task adds the auto-refresh interval state + a bump signal pages can watch.

**Files:**
- Modify: `frontend/src/composables/useTimeWindow.ts`
- Create: `frontend/src/composables/__tests__/useTimeWindowRefresh.spec.ts`

- [ ] **Step 1: Write the failing test**

```ts
// frontend/src/composables/__tests__/useTimeWindowRefresh.spec.ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useTimeWindow } from '../useTimeWindow'

describe('useTimeWindow auto-refresh', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => { useTimeWindow().setRefreshInterval(0); vi.useRealTimers() })

  it('exposes intervalSec defaulting to 0 (off)', () => {
    expect(useTimeWindow().intervalSec.value).toBe(0)
  })

  it('setRefreshInterval bumps refreshTick on each interval', () => {
    const { intervalSec, refreshTick, setRefreshInterval } = useTimeWindow()
    const before = refreshTick.value
    setRefreshInterval(30)
    expect(intervalSec.value).toBe(30)
    vi.advanceTimersByTime(60_000)
    expect(refreshTick.value).toBe(before + 2)
  })
})
```

- [ ] **Step 2: Run it; expect FAIL** (`intervalSec`/`refreshTick`/`setRefreshInterval` undefined).

- [ ] **Step 3: Extend useTimeWindow.ts**

Read the current file first. It exports `useTimeWindow(defaultWindow)` returning `{ windowVal, apply, allowed }`. Add module-level refresh state and three new returns, leaving `windowVal/apply/allowed` exactly as they are.

```ts
// add near the top-level module scope of useTimeWindow.ts (outside the function):
import { ref } from 'vue' // ensure ref is imported (it already is for windowVal)

const intervalSec = ref(0)
const refreshTick = ref(0)
let _timer: ReturnType<typeof setInterval> | null = null

function setRefreshInterval(sec: number) {
  intervalSec.value = sec
  if (_timer) { clearInterval(_timer); _timer = null }
  if (sec > 0) {
    _timer = setInterval(() => { refreshTick.value++ }, sec * 1000)
  }
}

// inside the object returned by useFumeWindow(), add: intervalSec, refreshTick, setRefreshInterval
```

Concretely, the return becomes:

```ts
return { windowVal, apply, allowed, intervalSec, refreshTick, setRefreshInterval }
```

- [ ] **Step 4: Run it; expect PASS (2).**

- [ ] **Step 5: Wire pages to refreshTick**

For each of the four list/graph pages that load on `windowVal`, also reload on `refreshTick`. Edit the `watch` in each so the loader fires on either signal. Example for `OverviewPage.vue` — change:

```ts
const { windowVal } = useTimeWindow()
// ...
watch(windowVal, load)
```
to:
```ts
const { windowVal, refreshTick } = useTimeWindow()
// ...
watch([windowVal, refreshTick], load)
```

Apply the identical change in: `views/Overview/OverviewPage.vue`, `views/Topology/TopologyPage.vue`, and the traces/logs list composables' driving component (`views/Traces/TracesList.vue`, `views/Logs/LogsView.vue` — add `refreshTick` to their existing apply/watch path). Keep each page's existing `data-testid`s untouched.

- [ ] **Step 6: Build + full vitest; expect green.**

Run: `cd frontend && npm run build && npx vitest run`

- [ ] **Step 7: Commit**

```bash
git add frontend/src/composables/useTimeWindow.ts frontend/src/composables/__tests__/useTimeWindowRefresh.spec.ts frontend/src/views/Overview/OverviewPage.vue frontend/src/views/Topology/TopologyPage.vue frontend/src/views/Traces/TracesList.vue frontend/src/views/Logs/LogsView.vue
git commit -m "feat(ui-1): topbar auto-refresh interval drives page reloads"
```

---

## Task 9: Restyle Overview + Traces

AC#6. Apply OpenAPM card / table / waterfall / subtab vocabulary. **Every existing `data-testid` stays.**

**Files:**
- Modify: `frontend/src/views/Overview/OverviewPage.vue`, `frontend/src/views/Overview/ServiceCard.vue`
- Modify: `frontend/src/views/Traces/TracesList.vue`, `frontend/src/views/Traces/TraceDetail.vue`, `frontend/src/views/Traces/WaterfallChart.vue`

- [ ] **Step 1: Snapshot the testids to preserve**

Overview/cards: `overview-grid`, `service-card-${service}`, `card-error-rate`.
Traces: `traces-page`, `filter-service`, `filter-operation`, `filter-min-duration`, `filter-apply`, `traces-table`, `trace-detail-page`, `trace-json`, `waterfall-svg`, `waterfall-span`.
Do not rename, remove, or relocate any of them.

- [ ] **Step 2: Restyle ServiceCard.vue**

Keep `<script>` and the `service-card-*` + `card-error-rate` testids. Replace the template/style to use the global `.card` + `.kpi` vocabulary and a health ring:

```vue
<template>
  <div class="card svc-card" :data-testid="`service-card-${item.service}`" @click="emit('click', item.service)">
    <div class="svc-head">
      <span class="svc-name">{{ item.service }}</span>
      <span class="health-ring" :style="{ '--c': ringColor }" />
    </div>
    <div class="svc-kpis">
      <div class="kpi"><div class="name">calls</div><div class="value">{{ item.inbound_calls }}</div></div>
      <div class="kpi" data-testid="card-error-rate"><div class="name">err</div><div class="value">{{ pct(item.inbound_error_rate) }}</div></div>
      <div class="kpi"><div class="name">p95</div><div class="value">{{ ms(item.inbound_p95_ms) }}<span class="unit">ms</span></div></div>
    </div>
  </div>
</template>
```

In `<script setup>` add a `ringColor` computed from `item.inbound_error_rate` (e.g. `>0.05 → var(--danger)`, `>0.01 → var(--warning)`, else `var(--success)`), reusing the existing `pct`/`ms` helpers. Scoped style:

```css
.svc-card { cursor: pointer; transition: box-shadow .15s, transform .15s; }
.svc-card:hover { box-shadow: var(--shadow-md); transform: translateY(-1px); }
.svc-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.svc-name { font-weight: 600; font-size: 14px; }
.health-ring { width: 12px; height: 12px; border-radius: 50%; background: var(--c); box-shadow: 0 0 0 4px color-mix(in srgb, var(--c) 22%, transparent); }
.svc-kpis { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; }
.svc-kpis .kpi .value { font-size: 18px; }
```

- [ ] **Step 3: Restyle OverviewPage.vue**

Keep script + `overview-grid` testid. Wrap header in OpenAPM `.section-h` and keep `TimeWindowPicker`:

```vue
<template>
  <div class="overview">
    <div class="section-h"><h3>{{ t('overview.title') }}</h3><TimeWindowPicker /></div>
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NSpin v-else-if="loading" />
    <div v-else class="grid" data-testid="overview-grid">
      <ServiceCard v-for="it in items" :key="it.service" :item="it" @click="go" />
    </div>
  </div>
</template>
<style scoped>
.overview { padding: 4px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(248px, 1fr)); gap: 14px; }
</style>
```

- [ ] **Step 4: Restyle TracesList.vue**

Keep the entire `<script>`, the `NDataTable` (it already renders `traces-table`), and all `filter-*` testids. Wrap the page header in `.section-h`, swap the filter `NCard`/`NSpace` for a `.pfilter` bar but **keep the NInput/NInputNumber/NButton with their existing testids**. The simplest non-breaking change: keep components, add the `.pfilter` wrapper class and a page title. Confirm `traces-page`, `traces-table`, and every `filter-*` testid remain.

```vue
<template>
  <div class="traces-list" data-testid="traces-page">
    <div class="section-h"><h3>{{ t('traces.pageTitle') }}</h3></div>
    <div class="card">
      <div class="pfilter">
        <NInput v-model:value="service" data-testid="filter-service" :placeholder="t('traces.filterService')" />
        <NInput v-model:value="operation" data-testid="filter-operation" :placeholder="t('traces.filterOperation')" />
        <NInputNumber v-model:value="minDurationMs" data-testid="filter-min-duration" :placeholder="t('traces.filterMinDuration')" />
        <NButton type="primary" data-testid="filter-apply" @click="apply">{{ t('traces.filterApply') }}</NButton>
      </div>
    </div>
    <NDataTable class="tbl-wrap" :columns="columns" :data="items" :loading="loading" :row-props="rowProps" data-testid="traces-table" />
    <NText v-if="items.length === 0 && !loading" depth="3">{{ t('traces.empty') }}</NText>
    <NText v-if="hasMore" depth="3">{{ t('traces.hasMore') }}</NText>
  </div>
</template>
```

(Keep the existing `<script setup>` exactly. Only the template wrapper classes change.)

- [ ] **Step 5: Restyle WaterfallChart.vue**

The waterfall is pure SVG with `waterfall-svg` + `waterfall-span` testids. Keep the SVG structure and testids; only update fill colors to token-driven values (replace the hard-coded `#cbd5e1` text fill with `var(--text-3)` via `fill="currentColor"` on a wrapper, and bar fills to span-kind tokens). Minimal change — do **not** alter the `data-testid` rects or the `@click` emit.

- [ ] **Step 6: Restyle TraceDetail.vue**

Keep `trace-detail-page` + `trace-json` testids and the `NTabs`/`AnnotationBadge`. Wrap the JSON `<pre>` in the global `pre.code` class for styling (keep `data-testid="trace-json"`), and wrap the header in `.section-h`. The `NTabs` stay (NaiveUI), themed by tokens.

- [ ] **Step 7: Verify testids + behavior**

Run: `cd frontend && npx vitest run src/views/Overview src/views/Traces && npm run build`
Expected: existing Overview/Traces unit tests PASS; build clean.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/views/Overview frontend/src/views/Traces
git commit -m "feat(ui-1): restyle overview + traces to OpenAPM vocabulary"
```

---

## Task 10: Restyle Logs + Topology + Service detail + Login

AC#6. Same discipline — preserve every testid.

**Files:**
- Modify: `frontend/src/views/Logs/LogsView.vue`, `frontend/src/components/LogRow.vue`, `frontend/src/components/SeverityBadge.vue`
- Modify: `frontend/src/views/Topology/TopologyPage.vue`
- Modify: `frontend/src/views/Services/ServiceDetail.vue`, `frontend/src/views/Services/SignalsTab.vue`
- Modify: `frontend/src/views/LoginView.vue`

- [ ] **Step 1: Snapshot testids to preserve**

Logs: `logs-page`, `filter-service`, `filter-severity`, `filter-ts-range`, `filter-trace-id`, `filter-span-id`, `filter-body`, `filter-apply`, `logs-error`, `logs-loading`, `logs-empty`, `logs-list`, `log-row-*`, `trace-link-*`, `logs-panel-scope`, `logs-panel-error`, `logs-panel-list`.
Services: `signals-inbound-calls`, `signals-inbound-errors`, `signals-inbound-p95`, `dep-row-*`, `coming-soon-*`.
Login: `apiKey-input`, `submit-btn`, `lang-select`.
Topology: none.

- [ ] **Step 2: SeverityBadge.vue → OpenAPM `.badge`**

Keep the `severity` prop. Swap `NTag` for a span using the global `.badge` classes mapped by severity:

```vue
<script setup lang="ts">
import { computed } from 'vue'
const props = defineProps<{ severity: string }>()
const cls = computed(() => {
  const s = props.severity.toUpperCase()
  if (s.includes('ERROR') || s.includes('FATAL')) return 'err'
  if (s.includes('WARN')) return 'warn'
  if (s.includes('INFO')) return 'info'
  return 'muted'
})
</script>
<template><span class="badge" :class="cls">{{ severity }}</span></template>
```

(No testid on SeverityBadge — safe.)

- [ ] **Step 3: LogsView.vue + LogRow.vue**

Keep all scripts and every testid. Wrap the filter `NSpace` in a `.card`/`.pfilter` shell and the list in OpenAPM spacing. `LogRow` keeps its dynamic `log-row-*` + `trace-link-*` testids; restyle `.log-row` with `.card`-like surface + hover. Verify with the existing LogRow/Logs specs.

- [ ] **Step 4: TopologyPage.vue**

Keep script + `ServiceGraph`. Wrap header in `.section-h`, put `ServiceGraph` inside a `.card`. No testids to preserve, but keep the `ServiceGraph` props (incl. `ann-by-service`) intact.

- [ ] **Step 5: ServiceDetail.vue + SignalsTab.vue**

Keep `NTabs` + the `signals-*` and `coming-soon-*` testids. Wrap the header (`h2` + `AnnotationBadge` + `TimeWindowPicker`) in `.section-h`. In `SignalsTab`, restyle the `.strip` divs as `.kpi` cards in a `.grid-3` while keeping `signals-inbound-calls/errors/p95` testids on the same elements.

- [ ] **Step 6: LoginView.vue**

Keep `apiKey-input`, `submit-btn`, `lang-select` testids and the script. Restyle the `.login-card` to an OpenAPM glass `.card` centered on `var(--bg)`, with the woodpecker logo above the title. Do not change the NInput/NButton/NSelect testids.

- [ ] **Step 7: Verify**

Run: `cd frontend && npx vitest run && npm run build`
Expected: all existing unit tests (incl. Logs/LogRow/SeverityBadge/ServiceDetail/Signals/Dependencies) PASS; build clean.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/views/Logs frontend/src/components/LogRow.vue frontend/src/components/SeverityBadge.vue frontend/src/views/Topology frontend/src/views/Services frontend/src/views/LoginView.vue
git commit -m "feat(ui-1): restyle logs/topology/service-detail/login to OpenAPM vocabulary"
```

---

## Task 11: New e2e + full verification gate

AC#8. Prove the shell interactions in a live browser and confirm no regression across the whole suite.

**Files:**
- Create: `frontend/e2e/shell.spec.ts`

- [ ] **Step 1: Write the shell e2e**

```ts
// frontend/e2e/shell.spec.ts
import { test, expect, type Page } from '@playwright/test'

async function login(page: Page) {
  await page.goto('/login')
  await page.getByTestId('apiKey-input').locator('input').fill('test-key-acme')
  await page.getByTestId('submit-btn').click()
  await expect(page).toHaveURL(/\/$/)
}

test('⌘K palette opens and jumps to a service', async ({ page }) => {
  await login(page)
  await page.goto('/overview')
  await page.keyboard.press('Meta+k')
  await expect(page.getByTestId('command-palette')).toBeVisible()
  await page.getByTestId('cmdk-input').fill('checkout')
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/\/services\/checkout/)
})

test('theme toggle persists across reload', async ({ page }) => {
  await login(page)
  await page.getByTestId('theme-toggle').click()
  const theme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
  expect(theme).toBe('dark')
  await page.reload()
  const after = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
  expect(after).toBe('dark')
})

test('sidebar collapse persists across reload', async ({ page }) => {
  await login(page)
  await page.locator('.sidebar-toggle').click()
  await expect(page.locator('.app.sidebar-collapsed')).toBeVisible()
  await page.reload()
  await expect(page.locator('.app.sidebar-collapsed')).toBeVisible()
})
```

(Use a key that works cross-platform; if `Meta+k` is flaky on the CI's Linux Chromium, use `Control+k`. The AppLayout listener accepts both.)

- [ ] **Step 2: Rebuild the stack images (LESSON: `make up` does not --build)**

Per `make-up-no-build-stale-images` memory: the frontend image is baked, so rebuild before e2e.

Run:
```bash
cd /Users/huangbaixun/code_space/openaiops-platform
docker-compose -f deploy/docker-compose.yml build frontend
docker-compose -f deploy/docker-compose.yml up -d --no-deps frontend
make seed
```
Expected: fresh frontend image; seed OK.

- [ ] **Step 3: Run the new shell e2e**

Run: `cd frontend && npx playwright test shell.spec.ts`
Expected: 3 passed.

- [ ] **Step 4: Full regression gate**

Run, in order:
```bash
cd frontend && npx vitest run            # expect: existing 96 + new shell unit tests, all green
cd frontend && npm run build             # expect: vue-tsc clean
cd frontend && npx playwright test       # expect: existing 28 + 3 new = 31 passed
```
If any existing test goes red, it is a real regression (a moved/renamed testid or broken behavior) — fix it before proceeding; do not edit the test to pass.

- [ ] **Step 5: Commit**

```bash
git add frontend/e2e/shell.spec.ts
git commit -m "test(ui-1): e2e for ⌘K, theme persistence, sidebar collapse"
```

- [ ] **Step 6: Hand off to verification-before-completion**

All 8 ACs satisfied with green evidence. Proceed to `harness:verification-before-completion` → `harness:finishing-a-development-branch`.

---

## Self-review notes

- **Spec coverage:** every spec section maps to a task (shell→T3/T6/T7, theme→T2, ⌘K→T5, scope-pill→T4, time/refresh→T8, page restyle→T9/T10, tests→T11, stylesheet port→T1). All 8 ACs traced in the table above.
- **Placeholder scan:** no TBD/"handle edge cases"/"similar to". Each code step shows real code; restyle steps name the exact target markup + the testids to preserve.
- **Type consistency:** `useTimeWindow` returns `{ windowVal, apply, allowed, intervalSec, refreshTick, setRefreshInterval }` — used identically in TopBar (T6), CommandPalette (T5), pages (T8). `useCommandPalette` exposes `{ open, openPalette, closePalette, togglePalette }` — consumed in CommandPalette (T5) and AppLayout (T7). `useTheme` → `{ theme, initTheme, toggleTheme }`; `useSidebar` → `{ collapsed, initSidebar, toggleSidebar }` — consistent across T2/T3/T6/T7.
- **Out-of-scope respected:** no Domain/Env switching (T4 is read-only + seam), no backend, no mock pages (placeholders only), no annotations overlay, no responsive.
