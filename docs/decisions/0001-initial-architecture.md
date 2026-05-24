# ADR-0001: Initial Architecture (imported from archived OpenAIOps spec 2026-05-24)

## Original spec body

> **本稿同时充当：**
> 1. 老仓库 OpenAIOps 的 **归档触发文档**（配套 ADR-0007）
> 2. 新仓库 `openaiops-platform` 的 **初始架构蓝图**（创建新仓库时落地为该仓库 ADR-0001）
> 3. `features.json` 全量 **superseded** 的正式声明

## 0. 摘要

把 OpenAIOps 从「AIOps 多 Agent 诊断系统」重新定位为「一站式可观测平台（对标 SignOz / SkyWalking）+ AI 诊断 sibling」。当前 OpenAIOps 仓库归档；新仓库 `openaiops-platform` 用 Go 数据面 + TS/Vue 前端从零搭建，Phase 1 MVP 覆盖 Trace / Log / Topology / Metrics / Alerts 的 Lean 子集，按垂直切片推进；Phase 2 启动独立 sibling 仓库 `openaiops-ai`，通过 well-defined API 集成回 platform。

## 1. 产品定位 + 差异化

### 1.1 定位陈述

OpenAIOps Platform 是面向**中文 / 亚太市场**的一站式可观测平台，覆盖 traces / logs / metrics / topology / alerts，面向 SRE 与后端工程师。在 Phase 2 通过 sibling AI 模块叠加 RCA + 故障模拟 + 自动恢复，形成「观测 + 智能诊断」双层产品。

### 1.2 对标 SignOz / SkyWalking 的差异化（4 条）

| 差异点 | 解释 |
|---|---|
| **AI 诊断（Phase 2 杀手锏）** | 一键 RCA、自动恢复建议、故障模拟原生集成 — 不需要拼 Grafana + 自研脚本 |
| **LLM 可观测 + 成本归因** | LLM 调用作为一等公民信号：token 计量、按服务/租户成本归因、日预算闸口 |
| **中文 + 亚太一等公民** | UI/文档中文优先（同时支持英文）；DashScope/qwen 默认接入；时区合规话术贴本地 |
| **UX 完成度** | openapm 风：⌘K、双向贯通（告警↔Trace↔Log↔Exception）、Apple/Linear 视觉感 |

### 1.3 反目标（明确不是）

- APM 全功能堆料
- Datadog 的价格替代
- 单客户自部署到 100w events/s 的超大规模

**v0.1 目标：10w events/s、最多 10 租户、单 ClickHouse 集群。**

## 2. 仓库策略

### 2.1 新仓库

- 名字：`openaiops-platform`
- 位置：https://github.com/huangbaixun/openaiops-platform （已创建于 2026-05-24）
- 形态：单仓 mono-style
  - `backend/` — Go 服务（gateway / query / alert / topo / settings / ingester）
  - `frontend/` — TS + Vue 3 + Vite + Pinia + Naive UI
  - `deploy/` — docker-compose、Caddy、k8s manifests（v1.0）
  - `docs/` — 架构、API、运维 runbook
  - `proto/` — gRPC 契约（service 间通信）

### 2.2 Phase 2 sibling

- 名字：`openaiops-ai`（Phase 2 启动时建，**MVP 发布后立即开**）
- 与 platform 通过 §6 定义的 API 解耦，platform 不内嵌 Python，sibling 不直接读 platform 数据库

### 2.3 老仓库（当前 OpenAIOps）归档动作

伴随 ADR-0007 同步做：

1. 写 **ADR-0007**（`docs/decisions/0007-archive-and-rewrite-as-platform.md`）—— 决策记录
2. `docs/features.json` 全量 `status: superseded`，加 `superseded_by: openaiops-platform`
3. `docs/claude-progress.json` 加归档 note
4. 顶层 `README.md` 顶部加 `> ARCHIVED 2026-05-24. Active work continues at https://github.com/huangbaixun/openaiops-platform`（由维护者手动添加）
5. `main` 分支开 branch protection 禁止接 PR；保留只读
6. 旧 agent 代码作为**参考引用**：新仓库做 Phase 2 时按文件名复制对照，不做 git history 迁移

## 3. 架构总览 + 存储 + 多租户

### 3.1 分层骨架

```
┌─────────────────────────────────────────────────────────────────┐
│  Frontend (TS + Vue 3 + Vite + Naive UI)                :3000   │
│  Overview / Service / Traces / Logs / Topology / Alerts         │
│  + ⌘K Command Palette + i18n (zh-CN, en-US)                     │
└─────────────────┬───────────────────────────────────────────────┘
                  │ HTTPS + Bearer (API key 或 v1.0+ JWT)
┌─────────────────┴───────────────────────────────────────────────┐
│  API Gateway (Go)                                       :8080   │
│  · Auth (API key → tenant_id 解析)                              │
│  · Tenant 注入（ctx.tenant_id，所有下游强制带）                 │
│  · Rate limit (per tenant token bucket via Redis)               │
│  · CORS / audit hook                                            │
└──┬────────┬─────────┬─────────┬─────────────────────────────────┘
   │        │         │         │
   ▼        ▼         ▼         ▼
┌────────┐ ┌──────┐ ┌──────┐ ┌──────────┐
│Query   │ │Alert │ │Topo  │ │ Tenant   │
│API     │ │Engine│ │Engine│ │ Settings │
│(Go)    │ │(Go)  │ │(Go)  │ │ (Go)     │
│:8081   │ │:8082 │ │:8083 │ │ :8084    │
└───┬────┘ └──┬───┘ └──┬───┘ └────┬─────┘
    │         │        │          │
    └─────────┴────────┴──────────┘
              │
              ▼
┌─────────────────────────────────────────────────────────────────┐
│  Storage 层                                                     │
│  · ClickHouse（单实例 v0.1 / 集群 v1.0）                         │
│    └─ traces_v1 / logs_v1 / metrics_v1 / topology_edges_v1      │
│    └─ 所有表 ORDER BY (tenant_id, service, ts) + PARTITION 日   │
│  · PostgreSQL 16（tenants / api_keys / alert_rules /            │
│    notification_channels / audit_log / metering）               │
│  · Redis 7（rate limit token bucket / query cache / session）   │
└─────────────────────────────────────────────────────────────────┘
                  ▲
                  │ OTLP gRPC :4317 / HTTP :4318
┌─────────────────┴───────────────────────────────────────────────┐
│  Ingest 层                                                      │
│  · OTel Collector（官方版 + 自定义 processor 注入 tenant_id）   │
│  · Go ingester（从 Collector 接管 OTLP，写 CH，处理 backpressure）│
└─────────────────────────────────────────────────────────────────┘

Sibling repo（Phase 2）: openaiops-ai (Python)
  · 通过 Query API 读数据
  · 通过 alert webhook 接告警
  · 通过 /api/v1/recovery/actions 提交动作，platform 实际执行
```

### 3.2 存储选型：单 ClickHouse

- v0.1：单 CH 实例，traces / logs / metrics / topology 全打 CH 不同表
- v1.0：CH 集群（sharded MergeTree），由后续 ADR 决策上集群时机
- 不引入 VictoriaMetrics / Prometheus —— 与 SignOz 对齐，简化运维
- PG 只装元数据（租户、API key、告警规则、审计、计量），不装时序

### 3.3 多租户：行级隔离（tenant_id 列）

**实现：**

- PG `tenants` 表：`id (uuid) / name / created_at / plan / rate_limit_per_min / data_retention_days`
- PG `api_keys` 表：`id / tenant_id / hashed_key (bcrypt) / scope / revoked_at`，gateway 启动加载 + 5min 刷新
- CH 所有数据表：首列 `tenant_id LowCardinality(String)`，`ORDER BY (tenant_id, service, ts)`，`PARTITION BY (tenant_id, toYYYYMMDD(ts))` 备选（取决于 cardinality）
- Query 层强制：所有 SQL builder 必须从 `ctx.TenantID` 拼 WHERE；提供 `MustTenantScope(ctx, query)` helper；**禁止裸 SQL**
- Ingest 层注入：OTel Collector 自定义 processor 从 `Authorization` header 或 OTel attribute `tenant.id` 解析，下游 ingester 强校验，缺则 reject
- PG metadata 表（`alert_rule` / `notification_channel` / `audit_log`）全部带 `tenant_id`，按它过滤

**三层兜底（防泄漏）：**

1. ClickHouse Row Policy —— 实例侧二次过滤，相当于硬护栏
2. Query builder 强制 + lint 规则禁裸 SQL
3. E2E 反例测试：两个租户互查必须返回 0 行

## 4. 前端

### 4.1 技术栈

- Vue 3 (Composition API) + Vite + Pinia + Vue Router 4 + Naive UI
- vue-i18n（zh-CN / en-US 同步铺）
- 图：Topology 用 G6 或 D3；时序图用 ECharts

### 4.2 设计 token

- 从 `openapm/styles/core.css` 整体抠出来落进 `frontend/src/styles/tokens.css`
- 单一来源，组件内禁止 hardcode 颜色/间距/圆角/阴影

### 4.3 MVP 页面（按 openapm 顺序，按 slice 解锁）

| 页面 | 来自 slice | 备注 |
|---|---|---|
| Login | Slice 0 | API key 登录，v1.0 加 OIDC |
| Overview（服务卡片） | Slice 3 | 健康度色环 + 数字 |
| Service 详情（6 tabs） | Slice 1-4 渐进 | Signals(1+2) → Dependencies(3) → Runtime/Exceptions(4) |
| Traces | Slice 1 | 列表 + Waterfall + JSON/Service Map subtab |
| Logs | Slice 2 | 全文 + tail SSE + trace_id 反跳 |
| Topology | Slice 3 | 力导向图 + 时间窗重放 |
| Alerts | Slice 4 | 列表 + 规则 CRUD modal |
| Settings | Slice 5 | 成员 / API key / 租户 / 采样 / 计量 |

**MVP 外（Phase 1.5 或后续）：** DB/Redis/Kafka 集群页、LLM 成本页、SLO、Dashboards、AI 诊断 tab（Phase 2）

### 4.4 横向能力

- **⌘K 命令面板**：Slice 0 铺骨架，每个 slice 自己注册条目（移植 openapm/js/cmdk.js）
- **路由参数单次消费**：Vue Router meta + composable，consume + clear（openapm 的 APM.go 模式）
- **双向贯通**：告警↔Trace↔Log，每个 slice 内一起做不留尾巴

## 5. Phase 1 排期（垂直切片 · 路径 B）

- **总长：15 周（理想，2-3 人团队） / 40-41 周（1 人 solo 现实，含 i18n + metering）**
- 公布给外部：**42 周（~10 个月）** 留缓冲
- 单位是「全职周」；不含招聘、设备、生病、客户演示打断

### Slice 0 · Foundation（3 周 solo，原 2 周 + i18n + metering schema）

- Repo init：Go modules + Vite TS + GitHub Actions CI（go test + vitest + Playwright）
- Deploy：docker-compose（CH 23.x + PG 16 + Redis 7 + OTel Collector 官方版 + Caddy）
- PG schema：`tenants` / `api_keys` / `metering_events` 表 + goose migration
- Gateway 雏形：Bearer middleware → `ctx.tenant_id` 注入
- Frontend 壳：router + theme + layout + Login 页 + 设计 token + vue-i18n（zh/en locale 文件）
- **AC**：`docker-compose up` 起栈；`curl -H "Authorization: Bearer xxx" /healthz` 返回 tenant_id 回显

### Slice 1 · Trace 端到端（5 周 solo）

- Collector 自定义 processor（Go）：从 OTLP header / attribute 抽 `tenant.id` 写进 span attribute（缺失 reject）
- Go ingester：从 Collector 接 OTLP → 写 CH `traces_v1`（tenant_id, trace_id, span_id, parent_span_id, service, operation, ts, duration, attributes Map, status, kind）
- Query API：`GET /api/v1/traces`（列表 + 筛选）/ `GET /api/v1/traces/{trace_id}`（waterfall 结构）
- Frontend：Traces 列表 + 详情 waterfall + JSON / Service Map subtab（移植 openapm/js/pages-traces.js）
- 计量埋点：每个 ingest 写一行 PG `metering_events`（tenant_id, signal_type=trace, count, ts）
- **AC**：跑 OTel demo (hot-r.o.d.) → UI 看到 trace；两个租户互不可见；E2E 反例测试通过；metering 表有数据

### Slice 2 · Log 端到端（4 周 solo）

- Ingester 扩展：CH `logs_v1`（tenant_id, ts, service, severity, body, trace_id, span_id, attributes Map）
- Query API：`GET /api/v1/logs`（全文 + 筛选 + tail SSE） / `GET /api/v1/traces/{id}/logs`
- Frontend：Logs 页（移植 openapm/js/pages-logs.js）+ trace_id 链接跳 Traces
- Service 详情 Signals subtab 上线（Trace + Log 合并视图）
- **AC**：同一 trace 的 logs 在 Traces 详情 Logs subtab 可见；Logs 点 trace_id 跳走

### Slice 3 · Topology + Service detail（5 周 solo）

- Topo engine 服务（Go）：每 1min 后台 job 扫 spans 抽 service-to-service 边 → 写 CH `topology_edges_v1`
- Query API：`GET /api/v1/services` / `/services/{name}` / `/topology`
- Frontend：Overview 服务卡片页 + Topology 力导向图（G6）+ Service 详情 Dependencies subtab
- ⌘K 注入 services 候选
- **AC**：hot-r.o.d. 多服务 → topology 节点边正确；时间窗 15m/1h/6h/24h 切换数字重放

### Slice 4 · Metrics + Alerts（6 周 solo）

- Ingester 扩展：CH `metrics_v1`（tenant_id, name, labels Map, ts, value，SummingMergeTree）
- Alert engine 服务（Go）：PG `alert_rules` + 后台 evaluator（每 30s） → PG `alert_events`
- Notification dispatcher：webhook（Slack incoming + SMTP via gomail），死信队列
- Frontend：Alerts 列表 + 规则 CRUD modal + 告警→trace 跳转
- Service 详情 Runtime / Exceptions subtab 上线
- **AC**：注入 CPU spike → alert fire → Slack 收通知；告警点击跳到时间窗 trace

### Slice 5 · MVP 加固 + 发布（5 周 solo）

- Settings 页：成员 / API key / 租户 / 采样 / 计量 UI
- Audit log：所有 mutation 走 audit（PG `audit_log`，append-only）
- Metering UI：当日 / 当月 events 用量 + 按 signal 类型分布
- i18n 翻译收口：所有页面英文 locale 补齐
- 文档：`docs/QUICKSTART.md` / 接入指南 / 运维 runbook / Swagger
- Release：`docker-compose.prod.yml` + Caddy 自动 TLS + smoke E2E 全绿
- **AC**：外部人 30 分钟内拉起栈、注入 OTel demo、看到 traces、收到告警

**Slice 总周数：3 + 5 + 4 + 5 + 6 + 5 = 28 周纯开发**；加 +30% 缓冲（招聘磨合、bug 兜底、文档迭代、客户演示）= **38 周**；再 +i18n 翻译收口的二轮 review = **40-41 周**。

## 6. Phase 2 Sibling 仓库契约

**死契约：platform 不内嵌 Python，sibling 不直接读 platform 数据库；两边只通过下表 API 解耦。**

| 用途 | endpoint | 方向 |
|---|---|---|
| 读 traces / logs / metrics | `GET /api/v1/{traces,logs,metrics}` | ai → platform |
| 读 topology | `GET /api/v1/topology` | ai → platform |
| 接收 alert 触发 | `POST {ai-webhook-url}` | platform → ai（notification channel type=ai-rca） |
| 回写 RCA 结论 / annotation | `POST /api/v1/annotations` | ai → platform（在 trace / topology 节点贴注释） |
| 提交恢复动作 | `POST /api/v1/recovery/actions` | ai → platform（platform 实际执行，保留最终权） |

**纪律：**

- AI 模块拿专用 tenant-scoped API key
- platform 对 AI 提交的 recovery action 仍走 dry_run / approval / Tier 校验（参考老仓库 ADR-0006 的思路在新仓库重写）
- Phase 2 启动时单独走一遍 brainstorming → spec → plan，不复用本稿

## 7. 计费 / 开放问题决策

### 7.1 计费模型：按 events/s

- 计量单位：每个 ingest 的 signal event（trace span / log line / metric data point 各计 1）
- 计量埋点：每个 ingest 写一行 PG `metering_events`（tenant_id, signal_type, count, ts）
- v0.1：仅记录，不收费；v0.2 加配额阀值告警；v1.0 加按月汇总账单出账
- 不按 query 计费（避免用户「不敢查」的反向激励）

### 7.2 客户迁移：无需

- 老仓库无真实付费客户，不需要迁移 ADR
- 老仓库 simulator 数据 → 新仓库**改用 OTel demo (hot-r.o.d.)** 作为标准 fixture

### 7.3 i18n：MVP 内支持英文

- vue-i18n 加入 Slice 0；zh-CN 为默认 locale，en-US 同步铺
- Slice 5 做英文翻译收口 review
- v1.0 加日文 / 韩文（亚太市场扩展时）

## 8. 风险

| # | 风险 | 严重度 | 缓解 |
|---|------|--------|------|
| R1 | 1 人 solo 40 周 = 10 个月，期间任何中断都拉长工期 | 高 | 公布 42 周外部承诺；按 slice 出可演示版本，每 slice 都能截停发版 |
| R2 | Go 团队能力薄 → Slice 0/1 拖期 | 中 | 前 4 周找 Go 资深 mentor（review + pair）；选简单 Go 模式不过度炫技 |
| R3 | 单 CH 实例 10w events/s 触顶 | 中 | v0.1 不解决；定 v1.0 上 CH 集群的 ADR；ingest 加 backpressure metric 提前预警 |
| R4 | 自定义 OTel processor 随 Collector 升级漂移 | 中 | 锁 Collector 版本；E2E 监控 processor 输出；必要时 fork |
| R5 | 多租户行级隔离泄漏 | 高 | §3.3 三层兜底（Row Policy + builder + 反例 E2E） |
| R6 | i18n 翻译质量（中→英）依赖人工 review | 低 | Slice 5 留 1 周专门翻译 review；不接受机器直翻 |
| R7 | Phase 2 立即启动 → 与 Phase 1 后期撞车（同 1 人） | 高 | Phase 2 启动前必须再开一次 brainstorming，重新评估人力配比 |

## 9. 关联文档

- **本稿配套 ADR**：[`docs/decisions/0007-archive-and-rewrite-as-platform.md`](../decisions/0007-archive-and-rewrite-as-platform.md)
- **superseded 的 features**：[`docs/features.json`](../features.json)（全 15 条 status → superseded）
- **设计原型**：`/Users/huangbaixun/code_space/openapm/` 的所有 HTML + js 模块
- **新仓库**：https://github.com/huangbaixun/openaiops-platform （已创建）
- **Phase 2 仓库**：`https://github.com/huangbaixun/openaiops-ai`（待 MVP 发布后创建）

## 10. 下一步

1. 本稿 + ADR-0007 + features.json supersede + progress.json note 一起合入 main（PR）
2. 老仓库 README 顶部加 ARCHIVED 标记
3. 创建 GitHub 新仓库 `openaiops-platform`
4. 本稿作为新仓库 `docs/decisions/0001-initial-architecture.md` 的原型导入
5. 走 `harness:writing-plans` 出 Phase 1 Slice 0 的实施计划（**只 Slice 0**，后续 slice 各自再 plan）
