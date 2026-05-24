# openaiops-platform

> 一站式可观测平台 · 对标 SignOz / SkyWalking · 中文一等公民
>
> Architecture decision: see `docs/decisions/0001-initial-architecture.md`

**Status:** Slice 0 (Foundation) — under construction.

## Quickstart

```bash
make up        # docker-compose up -d
make seed      # seed two test tenants (acme, beta) with fixed dev API keys (see deploy/seed.sql)
make smoke     # curl /healthz with the 'acme' test key (test-key-acme)
```

> Note: the dev seed keys are intentionally public and only valid for local docker-compose use. Production deployment must rotate them.

## Layout

- `backend/` — Go 1.22 services (gateway today; query / alert / topo / settings in later slices)
- `frontend/` — Vue 3 + TypeScript + Naive UI
- `deploy/` — docker-compose, Caddy, OTel Collector config
- `docs/` — architecture decisions, runbooks
