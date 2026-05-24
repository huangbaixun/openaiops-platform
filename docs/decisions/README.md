# Architecture Decision Records

Append-only ledger of architectural decisions. Each ADR captures one decision: the context that forced it, the options considered, the choice made, and what it costs us. ADRs are immutable once Accepted — if a decision changes, write a new ADR that supersedes it.

## How to write one

1. Number sequentially. Slug after the number.
2. Front-matter fields (in body, not YAML): `Status / Date / Deciders / Tracks / Supersedes` (and `Superseded by` once that happens).
3. Sections: `Context → Decision → Consequences → Verification`. Add `Open questions / Forward references` if useful.
4. Cross-link to the design spec under `docs/specs/` if brainstorming produced one.
5. Add a one-line entry to the index below.

ADRs that originate from `/harness:brainstorming` with `type: adr-proposal` get an auto-generated spec at `docs/specs/YYYY-MM-DD-<slug>-design.md` that the ADR back-references. Decision-only ADRs (no design exploration needed) can skip the spec.

## Statuses

- **Proposed** — under review, not yet binding.
- **Accepted** — current law. Implementation either landed or scheduled.
- **Superseded** — replaced by a later ADR. Body kept verbatim; superseded-by link added.
- **Rejected** — explored and dropped. Kept so we don't re-litigate the same trade-off.

## Index

| # | Title | Status | Date | Tracks |
|---|---|---|---|---|
| [0001](0001-initial-architecture.md) | Initial architecture (Go + Vue + multi-tenant CH, Phase 1 MVP scope) | Accepted | 2026-05-24 | repo bootstrap |
| [0002](0002-clickhouse-schema-migrations.md) | ClickHouse schema migrations — dedicated `ch-migrate` service, forward-only, plain SQL | Accepted | 2026-05-24 | PRE-1 |
| [0003](0003-query-api-deployment-shape.md) | Query API deployment shape — split `cmd/query/` from day 1 under Heavy-soon workload | Accepted | 2026-05-24 | PRE-2 |

## Pending decisions (no ADR yet)

| Tracker | Title | Owner | Blocking |
|---|---|---|---|
| PRE-3 | MustTenantScope builder + CH Row Policy + cross-tenant reverse E2E | @huangbaixun | SLICE-1 first CH write |

Live status lives in `docs/claude-progress.json`. When PRE-3 lands, the resulting ADR (likely 0004) gets indexed here and the PRE-3 row moves out of "Pending".
