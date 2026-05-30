---
date: 2026-05-30
topic: platform-ui-1-refactor-design
type: feature
status: proposed
features: [PLATFORM-UI-1]
---

# PLATFORM-UI-1 design — Frontend UI refactor (align to OpenAPM hi-fi design)

## Context

The platform frontend (`frontend/`, Vue 3 + TS + NaiveUI + vue-i18n + Pinia) currently
ships a minimal shell (flat sidebar, brand-text topbar) over six backend-wired pages:
Overview, Traces, Logs, Topology, Service-detail, Login. The designated visual source of
truth is the OpenAPM hi-fi prototype (`https://huangbaixun.github.io/OpenAPM/`, local clone
at `/Users/huangbaixun/code_space/OpenAPM/`) — a framework-free HTML/JS/CSS mock with a rich
shell (grouped sidebar w/ icons + collapse, scope-pill topbar, time-range, auto-refresh,
theme toggle, ⌘K command palette) across 14 pages.

The platform's design tokens (`frontend/src/styles/tokens.css`) are already ported verbatim
from OpenAPM `styles/core.css` (Apple-HIG glass system, light + dark). This feature ports the
rest of OpenAPM's **shell chrome and component vocabulary** and restyles the six real pages to
match — it does **not** reproduce OpenAPM's nine backend-less mock pages with fake data.

This is **Feature A** of a two-feature decomposition agreed with the user. **Feature B**
(`PLATFORM-MT-1`, separate spec + ADR) is the multi-tenant access model — Domain grouping,
env-tagged tenants, user↔multi-tenant ACL, tenant switching + re-auth — which is security-
critical (it changes the auth model from "one key → one tenant" to "one user → many tenants")
and therefore must not be bolted onto a visual refactor. This spec builds the topbar
Domain/Project/Env selectors as a **forward-compatible presentation layer** that Feature B
later wires to real switching.

## Goals / non-goals

**Goals**
- Port OpenAPM `core.css` shell rules + `components.css` component vocabulary into `src/styles/`
  as the visual single source of truth.
- Rebuild the shell (AppLayout, SideBar, TopBar) as Vue SFCs matching OpenAPM.
- Add three signature interactions: theme toggle (light/dark), ⌘K command palette, time-range +
  auto-refresh in the topbar.
- Restyle the six real pages to OpenAPM's card / table / badge / waterfall / subtab vocabulary.
- Keep the refactor purely presentational — no changes to `api/`, `composables/`, `stores/`,
  the router table, or any backend.

**Non-goals (YAGNI)**
- Real Domain/Env switching → Feature B (`PLATFORM-MT-1`).
- The nine backend-less OpenAPM pages (exceptions, database, redis, kafka, llm, dashboards,
  alerts, onboarding, settings) — rendered only as grouped, disabled "即将上线" nav placeholders.
- The design-annotations (`?`) overlay — a design-review tool, not a product feature.
- Mobile/responsive — OpenAPM is fixed-width desktop; we stay desktop-first.

## Approach: OpenAPM-CSS-first hybrid

OpenAPM's `DESIGN-DISCUSSION.md` explicitly warns against antd/shadcn ("留白和阴影规则不一样").
We therefore lead with OpenAPM's own plain-CSS vocabulary rather than re-theming a component
library:

- Port `OpenAPM/styles/core.css` (shell grid: `.app`, `.topbar`, `.sidebar`, `.main`,
  `.nav-group`, `.nav-item`, `.pill`, `.icon-btn`) and `OpenAPM/styles/components.css`
  (`.dd-menu`, `.dd-item`, `.scope-pill`, `.refresh-group`, `.avatar`, `.badge.*`, `.tbl`,
  `.kpi`, `.bar`, `pre.code`, `.waterfall`/`.wf-row`, `.subtabs`/`.subtab`) into
  `frontend/src/styles/` as global stylesheets imported once in `main.ts`. `tokens.css` stays
  the variable source; ported sheets reference `var(--*)` only.
- Build the shell and page chrome as Vue SFCs that apply those classes.
- Retain NaiveUI **only** for the heavy interactive widgets already in use (`NSpin`, `NModal`
  behind `AnnotationBadge`, `NSelect`/`NAlert` where convenient), theming them to the tokens so
  they don't visually drift.

### Why purely presentational matters

Because data flow (`api/`, `composables/`, `stores/`, router) is untouched, the existing test
suite is the refactor's built-in regression guard: **96 vitest + 28 Playwright must stay green**,
which requires every existing `data-testid` to survive the restyle unchanged. Any test that goes
red is a real regression, not an expected churn.

## Components

### Shell

**AppLayout** (`layouts/AppLayout.vue`) — CSS-grid shell: collapsible sidebar (width animates
between expanded/collapsed), topbar spanning full width, scrolling `.main`. Sidebar collapsed
state persisted to `localStorage` (`apm:sidebar`).

**SideBar** (`components/SideBar.vue`) — three nav groups matching OpenAPM:
- `OBSERVE`: 概览(overview) · Traces · Logs · Exceptions
- `ANALYZE`: Topology · DB Calls · Redis · Kafka(NEW) · LLM(NEW)
- `PLATFORM`: Dashboards · Alerts · Onboarding · Settings

Each item: SVG icon + label + optional `NEW`/count badge + active state. **Only the six
backend-wired routes are live `RouterLink`s** (overview, traces, logs, topology, plus
service-detail reached from overview, login outside the shell); all other items render as
disabled placeholders with an "即将上线" affordance and no navigation. Collapse toggle at top.

**TopBar** (`components/TopBar.vue`) — left: woodpecker logo + "啄木鸟 / OpenAIOps" brand +
scope-pill (§ Scope-pill). Right: time-range dropdown, refresh-group (manual refresh + auto
30s/1m/5m + countdown), bell (→ disabled alerts, shows nothing actionable yet), theme-toggle,
language `NSelect` (kept), avatar user-menu (个人设置 placeholder, 命令面板 ⌘K, 注销→logout).

**CommandPalette** (`components/CommandPalette.vue` + `composables/useCommandPalette.ts`) —
global ⌘K / Ctrl+K overlay; fuzzy-matches across two candidate sources: (1) the live nav pages,
(2) the service list from `fetchServices(window)`. Enter navigates via `router.push`. Esc / click-
outside closes. Self-contained; no backend beyond the existing services call.

### Scope-pill (forward-compatible presentation layer)

`components/ScopePill.vue` renders the OpenAPM `.scope-pill` breadcrumb (Domain / Project) plus
the Env pill:
- **Project** = the current logged-in tenant (`auth.tenantName`) — real, displayed, not switchable.
- **Domain** = a single read-only label, derived statically (e.g. constant "默认域" or grouped from
  tenant name) — single-item dropdown, no switching.
- **Environment** = a single read-only label (e.g. "Production") — single-item dropdown, no switching.

Each selector is rendered with its full OpenAPM dropdown chrome but in a disabled/single-item
state, with an explicit code seam comment `// FEATURE-B (PLATFORM-MT-1): wire tenant switching here`.
Feature B replaces the static sources with real domain/env/tenant lists + switch handlers.

### Theme toggle

`composables/useTheme.ts` — toggles `data-theme="dark"|"light"` on `<html>`, persisted to
`localStorage` (`apm:theme`), restored on app boot. Tokens already define the dark palette, so
this is purely an attribute flip.

### Page restyle (six real pages)

| Page | Restyle |
|---|---|
| Overview | service cards → OpenAPM health cards (color ring + health score + KPI + sparkline via the `core.js` `sparkline` helper port); `.pfilter` time filter row |
| Traces | list as `.tbl`; query/filter row as `.pfilter`; detail waterfall → `.waterfall`/`.wf-row`; Logs/ServiceMap/JSON views via `.subtabs` |
| Logs | rows as `.tbl`; severity → `.badge.{ok,warn,err}`; `trace=` chip link preserved (navigates to trace) |
| Topology | keep `ServiceGraph` SVG + force sim; wrap in OpenAPM card chrome + `.pfilter` time filter; annotation node markers preserved |
| Service detail | keep existing real tabs; restyle to `.subtabs`; `AnnotationBadge` themed to tokens |
| Login | OpenAPM auth-screen styling; `apiKey-input` + `submit-btn` testids preserved |

i18n stays key-based; new keys added for nav-group labels, command-palette copy, theme/refresh
controls (`en-US` + `zh-CN`).

## Data flow

Unchanged. All pages keep their current composables (`useTimeWindow`, `useTraces`, `useLogs`,
`useAnnotations`, etc.) and api modules. The topbar time-range control drives the existing
`useTimeWindow` store so the six pages re-query through their current code paths; auto-refresh
re-invokes the same loaders on an interval. The ⌘K palette and theme toggle add no new backend
calls beyond the existing `fetchServices`.

## Error handling

No new failure surfaces (presentational). The ⌘K service-candidate fetch degrades gracefully:
if `fetchServices` fails, the palette still offers page navigation and shows an inline "服务列表
加载失败" note rather than blocking. Theme/sidebar `localStorage` writes are wrapped in try/catch
(Node-25 Storage shim already in `tests/setup.ts`).

## Testing strategy

- **Regression guard (primary):** existing 96 vitest + 28 Playwright stay green; every existing
  `data-testid` preserved. A red test = real regression.
- **New unit (vitest):** ⌘K fuzzy match + navigation; theme toggle persistence + restore; sidebar
  collapse persistence; nav-group rendering incl. disabled placeholders; ScopePill shows current
  tenant.
- **New e2e (Playwright):** ⌘K open → type service → Enter navigates; theme toggle persists across
  reload; sidebar collapse persists.
- **Build:** `npm run build` (vue-tsc) must pass — per the project lesson, frontend verification is
  not complete on vitest alone (esbuild does not typecheck).
- **Visual:** manual parity check against the OpenAPM site + screenshots of each restyled page.

## Acceptance criteria

1. Sidebar renders OpenAPM's three nav groups with icons; the six backend-wired pages navigate;
   the nine backend-less items render as disabled "即将上线" placeholders; collapse state persists.
2. TopBar renders logo + brand + scope-pill + time-range + refresh-group + theme toggle + language
   + avatar user-menu; logout works.
3. Theme toggle switches light/dark via `data-theme` and persists across reload.
4. ⌘K opens a command palette that fuzzy-jumps to any page and to any service from the live list.
5. Time-range control drives the six pages' existing query window; auto-refresh re-queries on the
   selected interval.
6. All six real pages are restyled to OpenAPM's component vocabulary with every existing
   `data-testid` intact.
7. Scope-pill shows Project = current tenant (real); Domain/Env present as read-only forward-
   compatible selectors with a documented Feature-B seam.
8. Existing 96 vitest + 28 Playwright remain green; `npm run build` passes; new shell unit/e2e tests pass.

## Out of scope

- Real Domain/Env/tenant switching and any backend change (→ `PLATFORM-MT-1`, Feature B, w/ ADR).
- Backend-less mock pages with fake data.
- Design-annotations overlay; mobile responsive.

## Dependencies

None blocking. Forward-references `PLATFORM-MT-1` (Feature B) for scope-pill wiring.

## Related files

- `frontend/src/styles/tokens.css` (existing, ported from OpenAPM core.css)
- `frontend/src/styles/` (new: ported core/components stylesheets)
- `frontend/src/layouts/AppLayout.vue`, `frontend/src/components/{SideBar,TopBar}.vue` (rewrite)
- `frontend/src/components/{ScopePill,CommandPalette}.vue` (new)
- `frontend/src/composables/{useTheme,useCommandPalette}.ts` (new)
- `frontend/src/views/**` (restyle, testids preserved)
- Reference: `/Users/huangbaixun/code_space/OpenAPM/{styles/core.css,styles/components.css,js/shell.js,js/cmdk.js}`
