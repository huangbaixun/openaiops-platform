# openaiops-platform — Claude 开发守则

一站式可观测平台 · 对标 SignOz/SkyWalking · Go 数据面 + Vue 前端 + 多租户行级隔离 + Docker Compose 部署。AI 诊断 Phase 2 走 sibling 仓库 `openaiops-ai`（待 MVP 后建）。

详细架构见 `docs/decisions/0001-initial-architecture.md`；执行经验见 `docs/lessons-learned-2026-05-24.md`。

## 活规则

### 多租户（载重 · 任何 schema/路由 day-1 就要带）
- 任何 PG/CH 查询必须 tenant-scoped；**禁裸 SQL**，必走 query builder。
- 新增受保护路由必须挂 `auth.Middleware(resolver)`，handler 必须调 `auth.TenantID(r.Context())` —— 否则在路由层就静默漏。
- 新 CH 表第一列必须 `tenant_id LowCardinality(String)`，`ORDER BY (tenant_id, ...)` 打头。
- Slice 1 起：所有 CH 查询必须经 `MustTenantScope(ctx, q)` helper（待实现，ADR-0001 §3.3 三层兜底 = builder + CH Row Policy + 反例 E2E）。

### 端口
- canonical 8080 / 4317 / 4318 / 3000。本地若与 SignOz 等冲突走 `deploy/.env.local` 覆盖 `GATEWAY_HOST_PORT` / `OTEL_GRPC_HOST_PORT` / `OTEL_HTTP_HOST_PORT` / `FRONTEND_HOST_PORT`。CI 必须用默认。

### Goose 迁移（PG）
- PG schema 改写新 `backend/migrations/YYYYMMDDHHMMSS_<verb>.sql`，必带 `-- +goose Up` 和 `-- +goose Down`。

### CH 迁移（自研 runner，forward-only · ADR-0002）
- CH schema 改写新 `backend/ch-migrations/YYYYMMDDHHMMSS_<verb>.sql`，**纯 SQL**（不带 goose pragma），可多语句分号分隔。
- 业务表第一列必须 `tenant_id LowCardinality(String)`，`ORDER BY (tenant_id, ...)` 打头（ADR-0001 §3.3）。
- 应用方式：`make up` 自动跑 `ch-migrate` 服务；或 `make migrate-ch-up` 单独跑。re-run 幂等（`_schema_migrations` 跟踪表）。
- 无 DOWN。dev 想撤就 `docker compose down -v` 清 `chdata` 重来。第一次出现 `ALTER TABLE` 之前必须回到 ADR-0002 重谈。

### Bcrypt 测试
- 任何接 `apikey.Hash` 的测试需 `-timeout` 至少 60s（每次 hash ~150ms）。包级 timeout 120s+；CI 240s+ for integration。

### Test 双轨
- 默认 `go test ./...` 跑单元；`-tags=integration` 跑 dockertest（自动起 ephemeral PG 容器）。
- frontend：`npx vitest run` 单元；`npx playwright test` 跑 E2E（需 stack up 且 seed 过）。

## 已知陷阱（详见 `docs/lessons-learned-2026-05-24.md`）
- **nginx `/api` proxy 必须**：缺了认证假成功 + 返回空租户。
- **NaiveUI `NInput` data-testid 在 wrapper div 上**：测试要 `.locator('input')` 钻进去。
- **Node 25 内置 Storage 缺 `.clear()`**：vitest 用 `tests/setup.ts` shim 兜。
- **Go 版本：本机 + go.mod + Dockerfile + CI matrix 必须一致**（当前 1.25.0）。
- **`migrate` 服务每次 `compose up` 装 goose**：网络抖会拖死 e2e；下次重构走 pre-baked。

## 工作流纪律
- 改 `backend/`：`cd backend && go test -count=1 ./...`；改 SQL/repo 加 `-tags=integration` 跑 dockertest。
- 改 `frontend/`：`cd frontend && npm run build && npx vitest run`；改路由/views 跑 `npx playwright test`（需 stack up + seed）。
- 改 `deploy/`：`make up && make seed && make smoke` 一遍，保 AC-2 不破。
- 新架构决策写 ADR `docs/decisions/NNNN-<slug>.md`。
- 跨 session 进度写 `docs/claude-progress.json` 的 `current_focus / open_tasks / known_drift`。
- **修 bug 必须配回归测试**（fails-without-fix + passes-with-fix），否则 status 降 `done_with_concerns` + progress.json `known_drift` 记一笔。

## 调试速查（投到 6 类，定位 → 改对地方）
| 模式 | 信号 | 第一时间看哪 |
|---|---|---|
| 多租户泄漏 | 跨租户看到别人数据 | query builder 是否带 `tenant_id`；Row Policy 是否启用 |
| 401 流转 | login 假成功 / tenant 空 | gateway → 中间件 → resolver 链 + nginx/Caddy proxy 路径 |
| Race | 间歇性、和时间相关 | 共享 state（Redis、ctx 传递、goroutine） |
| Nil/None | TypeError / nil pointer | Optional 值缺空检查（pgx Scan、Vue ref） |
| Integration | timeout / 422 / shape mismatch | 跨服务边界（gateway↔CH、ingester↔Collector、AI sibling↔platform） |
| Config drift | 本地 ok / CI 炸 | env 变量、Go 版本、compose port、Caddy 路由 |

## 指针表

| 需要查 | 去哪 |
|---|---|
| 架构总图 | `docs/decisions/0001-initial-architecture.md` |
| Slice 0 执行经验 | `docs/lessons-learned-2026-05-24.md` |
| Slice 0 完成证据 | `features.json` SLICE-0 evidence 段 + `deploy/AC-evidence.txt` |
| Slice 1 前置决策（3 件） | `features.json` SLICE-0.slice_1_prerequisites + `claude-progress.json` open_tasks |
| Slice 0 怎么搭的（历史 plan） | `docs/plans/2026-05-24-slice-0-plan.md` |
| 端口约定 + dev override | `deploy/docker-compose.yml` + `deploy/.env.example` |
| API key 测试数据 | `deploy/seed.sql`（plaintexts `test-key-acme` / `test-key-beta` 公开 dev-only） |
| 老仓（已归档，只读） | https://github.com/huangbaixun/OpenAIOps |
