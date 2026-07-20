# AI 无缝接手清单

这份清单用于换电脑、换会话或交给新的 AI。按顺序执行，不要直接从某个 Worker 文件开始修改。

## 1. 确认仓库身份

```bash
git status --short --branch
git remote -v
git rev-parse HEAD
git describe --tags --always --dirty
Get-Content backend/cmd/server/VERSION
```

期望：

- 工作仓库为 `F:\Code\Git\sub2api` 或新电脑上的对应 clone。
- 当前主分支为 `main`。
- `origin` 指向 `JasonWangJie/sub2api`。
- `upstream` 指向 `Wei-Shaw/sub2api`。
- 发布版本仍为 `0.1.162`，除非后续已有明确版本提交。

不要推送到 `upstream`，不要对共享 `main` 使用 force push。

## 2. 按真值顺序阅读

1. `backend/migrations/185_async_image_tasks.sql` 和要修改的实际代码。
2. `docs/DURABLE_ASYNC_IMAGE_API.md`。
3. 本目录 `README.md` 及相关专题。
4. 根目录 `readmenew.md` 的 Fork 总览。
5. `docs/图片生成新功能请求说明.md` 只用来理解原始需求，不当作最终契约。

旧 `docs/ASYNC_IMAGE_TASKS.md` 仍有效，但只描述旧 Redis 24 小时任务。

## 3. 先检查交付状态

```bash
git log -10 --oneline --decorate
git status --short
rg -n "allow_async_image_generation|completions_gm|generations_oa|generations_sc" backend frontend
```

若工作树不干净，先理解每个现有修改；不要重置或覆盖不属于当前任务的用户改动。

## 4. 牢记计费和可靠性不变量

- 不复制计价公式，继续复用现有 RecordUsage。
- 上游成功后按实际宽高和数量 Prepare 一次固定账单。
- 后台改价不能改变已固定任务费用。
- `storage_failed`/`billing_failed` 只续跑后处理。
- `execution_unknown` 绝不自动重调上游。
- 重复 Apply 不得重复扣费；UsageLog 失败只补日志。
- 普通用户在存储和账务都成功前看不到结果。
- 软删除 Key 的 tombstone 只能完成结算，不能恢复鉴权。
- simple 模式使用 `not_billable`，仍写 UsageLog。

修改计费前先读 [04-billing-and-idempotency.md](04-billing-and-idempotency.md) 和对应单测。

## 5. 牢记协议不变量

- BB 与 SC 分别渲染，不做兼容超集。
- 固定 7 条路径，不配置接口路径。
- `public_base_url` 只配置外部根地址。
- 公共查询只能使用同一 API Key，其他 Key 统一 `404`。
- 新开关不改变任何旧同步或旧 `/async` 接口。
- SC 首版仅 Gemini；OpenAI 使用 `_oa`。
- 视频不在当前范围。

## 6. 本地修改后的最低验证

后端：

```bash
cd backend
go test ./... -run '^$'
go test ./... -skip '^TestContentModerationRuntimeSnapshotRefreshFailureKeepsStaleConfig$' -count=1
go vet ./internal/service ./internal/repository ./internal/handler
```

前端：

```bash
cd ../frontend
pnpm typecheck
pnpm test:run -- src/features/async-image-tasks/__tests__/api.spec.ts src/views/admin/__tests__/groupsAsyncImage.spec.ts
pnpm build
```

根目录：

```bash
cd ..
git diff --check
git status --short
```

如果修改 Ent schema：

```bash
cd backend
go generate ./ent
```

如果修改 Wire provider/构造函数：

```bash
go run github.com/google/wire/cmd/wire@v0.7.0 ./cmd/server
```

生成文件必须与 schema/DI 源文件一同提交。

## 7. 上线或继续验收

优先完成 [08-known-risks-and-next-steps.md](08-known-risks-and-next-steps.md) 的 P0 项。没有真实云凭证和浏览器截图证据时，不要把状态改成“生产验收完成”。

OSS 切换前先处理历史对象。`execution_unknown` 只能人工决定是否用新任务再生成。多实例部署要计算总 Worker 并发。

## 8. 提交与推送

```bash
git fetch origin main
git rev-list --left-right --count origin/main...HEAD
git diff --check
git status --short
git add <本次明确范围内的文件>
git diff --cached --stat
git diff --cached --check
git commit -m "<简体中文提交说明>"
git push --dry-run origin HEAD:main
git push origin HEAD:main
```

推送后核对：

```bash
git status --short --branch
git log -1 --oneline --decorate
git ls-remote origin refs/heads/main
git describe --tags --always
```

最终交接报告至少写明：提交 SHA、推送目标、`VERSION`、`git describe`、测试结果、未完成验收和文档路径。

## 9. 一句话恢复上下文

可把下面这段交给新的 AI：

```text
这是 JasonWangJie/sub2api Fork。请先读 wiki-new/README.md 和 docs/DURABLE_ASYNC_IMAGE_API.md，再检查 HEAD 和工作树。持久异步生图代码已完成，VERSION 保持 0.1.162；剩余重点是真实 OSS/上游端到端计费验收、桌面移动视觉验收和历史存储配置 resolver。严禁让旧接口自动异步、重复上游生成或重复扣费。
```
