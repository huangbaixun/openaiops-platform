# openaiops-platform

> 一站式可观测平台 · 对标 SignOz / SkyWalking · 中文一等公民
>
> Architecture decision: see `docs/decisions/0001-initial-architecture.md`

**Status:** Slice 0 (Foundation) — under construction.

## Quickstart

```bash
make up        # docker-compose up -d
make seed      # create test tenant + API key, prints the key
make smoke     # curl /healthz with the seeded key
```

## Layout

- `backend/` — Go 1.22 services (gateway today; query / alert / topo / settings in later slices)
- `frontend/` — Vue 3 + TypeScript + Naive UI
- `deploy/` — docker-compose, Caddy, OTel Collector config
- `docs/` — architecture decisions, runbooks
