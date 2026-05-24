# Slice 0 执行经验（2026-05-24）

Autonomous subagent-driven build 期间沉淀。规模：14 implementer + 11 reviewer + 2 fixer subagents，~3h 完成。CI green：[run 26359958968](https://github.com/huangbaixun/openaiops-platform/actions/runs/26359958968)。

## 一、Bug：差点漏到生产的 3 个

### B1 · `nginx.conf` 漏 `/api` proxy → 认证假成功

**症状**：浏览器访问 https://localhost → Caddy 正确 proxy 到 `frontend:80` → 前端 axios 调 `/api/healthz` → nginx 没匹配到 `/api` location → fallback 到 `try_files $uri $uri/ /index.html` → 返回 200 + SPA HTML → 前端把 HTML 当 JSON 解析 → `tenant_id` / `tenant_name` 都是 undefined → Login 流程"成功"跳到 Home 页 → 顶栏显示 `租户: undefined`。

**怎么发现**：Playwright E2E `login with valid key shows home + tenant name` 在 `data-testid="tenant-name"` 上 assert `toContainText('acme')` fail。

**Fix**：`frontend/nginx.conf` 必须 `/api` 块在 SPA fallback 之前：

```nginx
location /api/ {
  proxy_pass http://gateway:8080/;
}
location / {
  try_files $uri $uri/ /index.html;
}
```

**教训**：SPA + API 同源走 nginx 时，`/api` proxy 必须**先于** SPA fallback。fall-through 到 `index.html` 让 auth **永远静默成功**（200 + garbage data）—— 这是一个 critical 安全/正确性 footgun。任何 SPA + API 同源部署都要 grep 配置确认这个顺序。

### B2 · NaiveUI `NInput` 把 `<input>` 包在 div 里

**症状**：`page.getByTestId('apiKey-input').fill('test-key-acme')` 直接 fail。NaiveUI 的 `NInput` 把 `data-testid` 放在 wrapper div 上，真正的 `<input>` 在内部。Playwright fill 不进 div。

**Fix**：

```ts
await page.getByTestId('apiKey-input').locator('input').fill('test-key-acme')
```

**教训**：组件库包原生表单元素破坏了 "`data-testid` 1:1 映射可交互元素" 的假设。给 NaiveUI / ElementPlus / Ant 等任何包 wrapper 的库写 E2E，先**本地跑通**再依赖 CI；或者在被测组件上加一层 `:input-props="{ 'data-testid': 'xxx' }"` 把测试 id 透传到内部 input。

### B3 · Node 25 内置 `localStorage` 缺 `.clear()`

**症状**：vitest 测试 `beforeEach(() => localStorage.clear())` 抛 `TypeError: localStorage.clear is not a function`。Node 25 实验性内置了 Web Storage，shadow 了 jsdom 的实现，但缺方法。

**Fix**：`frontend/tests/setup.ts` 装 Map-backed Storage shim：

```ts
class MemStorage implements Storage {
  private m = new Map<string, string>()
  get length() { return this.m.size }
  key(i: number) { return [...this.m.keys()][i] ?? null }
  getItem(k: string) { return this.m.get(k) ?? null }
  setItem(k: string, v: string) { this.m.set(k, String(v)) }
  removeItem(k: string) { this.m.delete(k) }
  clear() { this.m.clear() }
}
Object.defineProperty(globalThis, 'localStorage', { value: new MemStorage(), writable: true })
```

`vitest.config.ts` 加 `setupFiles: ['./tests/setup.ts']`。

**教训**：Node 25 的实验 Web API 与 jsdom 假设有冲突。在 `package.json` `engines` 里 pin Node 版本一旦找到可用组合，并 doc 任何必需 shim。

## 二、环境坑：能预防的 2 个

### E1 · 本地端口与 SignOz 冲突

User 机器跑着 SignOz 占用 canonical 8080 / 4317 / 4318。首次 `compose up` 失败。

**对策**：compose 用 env-var 默认值参数化：

```yaml
ports: ["127.0.0.1:${GATEWAY_HOST_PORT:-8080}:8080"]
```

`deploy/.env.example` doc 覆盖值。CI 用默认（GHA runner 无冲突）。Dev 写 `.env.local`（gitignored）。

**教训**：默认 canonical 端口（保 CI），所有 host-port mapping 必须 env-overridable。

### E2 · Go 版本飘：plan 写 1.22，机器是 1.25

`go mod init` 写当前 Go 版本。macOS 上 brew 装的 Go 1.25 → `go.mod` 写 `go 1.25.0`。原 plan 假设 1.22 → CI workflow 全员手动调 `go-version: '1.25'`。

**教训**：plan 别 hardcode Go 版本。要么 CI 里 `cat backend/go.mod | grep ^go` 读，要么 `go-version: 'stable'`。本仓选了显式 `'1.25'` 因为已知 macOS dev 环境就是 1.25。

## 三、架构债（写进 `claude-progress.json` known_drift）

### D1 · `PGResolver` O(N × bcrypt)

每个请求扫所有 active `api_keys` + bcrypt-verify 每个候选。N=2 时无感；N>50 开始延迟可观。Slice 5 原计划加 `key_prefix` hint 列；Slice 1 ingester 高频调 query API 可能逼前置。

**Fix path**：`api_keys` 加 `key_prefix CHAR(8)`（plaintext 前 8 字符明文），ResolveBearer 先按 prefix 查到 1-2 个候选再 bcrypt-verify。索引 `idx_api_keys_prefix_active(key_prefix) WHERE revoked_at IS NULL`。

### D2 · `migrate` 服务每次 `compose up` 从公网装 goose

`docker-compose.yml` 的 migrate service 跑 `go install github.com/pressly/goose/v3/cmd/goose`。每次 compose up 都触发，网依赖 + 冷启慢，CI go proxy 拥塞时 e2e 会 timeout。

**Fix path**：换 `ghcr.io/pressly/goose:v3` 镜像，或自建 multi-stage Dockerfile 把 goose pre-bake 到 migrate runner。

### D3 · AC-7 goose down 未自动测

`backend/migrations/*.sql` 有 `-- +goose Down` 块，但 CI 只跑 `goose up`。drop-and-recreate 周期是个好的安全网。

**Fix path**：CI backend job 末尾加一步 `goose down` + 再 `goose up` 验证 idempotent。

## 四、过程观察（给下一轮 subagent execution 用）

- **Reviewers 在非平凡代码里找到真 bug**（T1 fix loop、T9 端口飘）。两阶段 review（spec compliance → code quality）在 >50 行代码任务上值这个 round-trip；纯 scaffold 任务上价值低（可考虑合并为单 review）。
- **TDD 纪律守住了**：14 个 dispatch 都看到 RED before GREEN，没人跳。
- **CI iteration 真贵**：T14 推了 4 次 fix 才绿。每次 ~6min CI cycle。Slice 1 教训：CI 改动前先**本地 smoke 跑通**再推。
- **`-tags=integration` 分离 dockertest 与单元测试**这个模式很好用，单元 feedback 快，integration 需要时显式跑。
- **subagent 自带 `git push` 权限**只在用户明确 grant autonomy 时才放（如本次）；默认让 orchestrator commit。

## 五、需要持续做的

- 任何新测试数据走 `backend/cmd/seed-hash <plaintext>` 生成 bcrypt hash，**不要手 hash**。
- 任何 `deploy/*` 或 `backend/*` 改动后 push 前跑 `make smoke`。
- 新 AC 落地时往 `deploy/AC-evidence.txt` 追加证据段（Slice 1 会加几条 trace ingest 的 AC）。
- 多租户：新写任何 SQL builder helper 时同步加 `MustTenantScope` lint —— 杜绝 review 时漏看裸 SQL。
