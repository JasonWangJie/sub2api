# AI 无缝接手清单

这份清单用于换电脑、换会话或交给新的 AI。不要直接从某个 Worker 文件开始修改，也不要依据聊天摘要把未验证项写成完成。

## 1. 确认仓库身份

```powershell
Set-Location F:\Code\Git\sub2api
git status --short --branch
git remote -v
git rev-parse HEAD
git describe --tags --always --dirty
Get-Content backend\cmd\server\VERSION
```

交接文档编辑时的基线：

```text
VERSION: 0.1.162
base HEAD: 51b083d374decf811ac88f8b0194165db9a8ba79
base describe: v0.1.162-4-g51b083d37
branch: feat/image-workflow-library-moderation
origin: JasonWangJie/sub2api
upstream: Wei-Shaw/sub2api
```

实际 HEAD 可能已因后续提交变化，以命令输出为准。不得推送到 `upstream`，不得 force push `main`，不得重置共享脏工作树。

## 2. 按真值顺序阅读

1. `git status` 和当前 diff，理解所有未提交/未跟踪文件。
2. `backend/migrations/185_async_image_tasks.sql`。
3. `backend/migrations/186_image_library_and_plaza_moderation.sql`。
4. `backend/migrations/187_async_image_upload_reservations.sql`。
5. 与当前任务直接相关的 Handler/Service/Repository/前端代码。
6. `docs/DURABLE_ASYNC_IMAGE_API.md`（只针对公共持久异步协议）。
7. 本目录 `README.md`、`01-current-status.md` 和对应专题。
8. 根目录 `readmenew.md` 摘要。
9. 原始需求文档只用于理解意图，不作为最终契约。

旧 `docs/ASYNC_IMAGE_TASKS.md` 仍有效，但只描述旧 Redis 24 小时任务。

## 3. 建立代码地图

```powershell
rg -n "allow_async_image_generation|completions_gm|generations_oa|generations_sc" backend frontend
rg -n "image-workbench|image-library|image-plaza" backend\internal\server\routes frontend\src\router
rg -n "image_storage_objects|library_archive|legacy_image_plaza_v1|async_image_upload_reservations" backend
```

核心文件：

```text
backend/internal/handler/durable_async_image_*.go
backend/internal/service/async_image_upload.go
backend/internal/repository/async_image_upload_repo.go
backend/internal/handler/image_workbench_handler.go
backend/internal/handler/image_library_handler.go
backend/internal/handler/image_plaza_handler.go
backend/internal/service/image_workbench.go
backend/internal/service/image_library.go
backend/internal/service/image_library_maintenance.go
backend/internal/service/image_plaza_helpers.go
backend/internal/repository/image_library_repo.go
frontend/src/features/image-workflow/
frontend/src/views/user/ImageWorkbenchView.vue
frontend/src/views/user/ImageLibraryView.vue
frontend/src/views/user/ImagePlazaView.vue
frontend/src/views/admin/ImageModerationView.vue
```

## 4. 牢记工作台不变量

- 模式只能由所选 Key 当前分组决定，用户不能手工切换。
- OpenAI/Gemini 的异步开关关闭时走实时，开启时走持久异步。
- Grok 仅实时；Antigravity/其他平台不进入工作台。
- 提交前重新获取 `capability_version`；变化时停止并要求重新确认。
- 任何错误都不能让实时/异步互相回退。
- 网络结果未知时复用完全相同的 body、multipart boundary 和 `Idempotency-Key`。
- 模型和参数能力以服务端响应为准；不要让前端硬编码成为独立真值。

详见 [10-image-workbench.md](10-image-workbench.md)。

## 5. 牢记计费不变量

- 不复制计价公式，继续复用现有 Gemini/OpenAI/Grok `RecordUsage`。
- 所有输出图片按实际宽高和实际数量计费。
- 混合尺寸按每张图片的档位分别求和。
- 异步上游成功后 Prepare 一次固定账单。
- `storage_failed/billing_failed` 只续跑后处理。
- `execution_unknown` 绝不自动重调。
- 重复 Apply 不重复扣费，UsageLog 失败只补日志。
- 图库归档、投稿、审核、查看和清理不产生图片生成费用。
- 归档失败不能推翻生成成功、重调上游或重复计费。

修改计费前先读 [04-billing-and-idempotency.md](04-billing-and-idempotency.md) 和相关测试。

## 6. 牢记图库和公开不变量

- 新结果默认私有。
- 公开必须显式投稿并由管理员批准。
- 提示词默认私有，只有明确共享才进入广场。
- 对外使用 `img_*`、`imgpub_*`，不泄露顺序 ID。
- 查看时动态签名，数据库不保存预签名 URL。
- 异步结果、图库和投稿共享 `image_storage_objects`；删除前检查全部活动引用。
- 用户越权统一 `404`。
- 旧广场升级后先隐藏，再严格校验迁为私有和待审；危险记录隔离。
- 旧 GET 只返回已批准作品，旧 POST/DELETE 是弃用适配器，不恢复默认公开。
- SC 上传先执行 body 前 PostgreSQL rate admission，再在解码/OSS 前执行 Key 级幂等与字节 reservation；数据库故障必须 fail closed。
- 重放只重签同一输入对象并登记 URL hash alias；已知跨 Key/过期 alias 不得降级为普通远程 URL。alias 注册按输入对象行串行，每对象最多 128 个，过期 alias 继续作为所有权墓碑。
- object intent 必须先于 OSS 写入持久化，Put 默认 300 秒且最大 600 秒。retention 第一次 Delete 后保留 intent，至少十分钟后二次 Delete 成功才移除；期间继续占用 Key 容量。文件名净化后不得进入 object key。

详见 [11-image-library-object-model.md](11-image-library-object-model.md) 和 [12-moderated-plaza-and-migration.md](12-moderated-plaza-and-migration.md)。

## 7. 接手后的第一轮只读检查

```powershell
git diff --check
git diff --stat
git status --short
Test-Path frontend\public\images\sub2api-workbench.webp
rg -n "CompleteAsyncImageResultDeletion|storage_object_id" backend\internal\repository\async_image_retention_repo.go
rg -n "ImageLibraryMaintenance.*Stop|\.Stop\(\)" backend\cmd backend\internal\server backend\internal\service
rg -n "output_format" frontend\src\api\imageWorkbench.ts
```

然后检查 [08-known-risks-and-next-steps.md](08-known-risks-and-next-steps.md) 中的当前产品缺口是否已经由其他协作代理补齐。只有代码和测试都能证明时，才更新文档状态。

## 8. 最低本地验证

后端便携 Go：

```powershell
$go = "$env:LOCALAPPDATA\CodexToolchains\go1.26.5\go\bin\go.exe"
cd backend
& $go generate ./cmd/server
& $go test ./internal/service ./internal/handler ./internal/repository ./internal/server/routes ./cmd/server -run '^$'
& $go test -tags=unit ./...
& $go test ./...
```

前端：

```powershell
cd ..\frontend
pnpm lint:check
pnpm test:run
pnpm typecheck
pnpm build
```

根目录：

```powershell
cd ..
git diff --check
git status --short
```

`2026-07-22` 在安全提交 `f16c2106a` 前、合并最新上游前执行了上述本地门禁：Go 1.26.5 的 generate、SC 定向/三包完整测试、unit tags 全包测试（`197.4s`）、默认全包测试（`121.9s`）和独立 server build 均通过；前端 lint、188 files/1266 tests、typecheck 和 974 modules build 均通过。合并 `upstream/main` 后必须再次执行；浏览器证据仍早于最后一批 SC/后台配置改动。历史格式清单与最终待办见 [07-testing-and-validation.md](07-testing-and-validation.md)。

修改 Ent schema 时执行 `go generate ./ent`；修改 Wire provider/构造函数时执行 `go generate ./cmd/server`。不要只提交手工源文件而漏掉生成文件。

主机没有 Docker/可用 WSL 时，真实 PostgreSQL/testcontainers 测试通过 Fork GitHub Actions 完成；不能因此把集成测试标成通过。

## 9. 浏览器验收

启动后按 [07-testing-and-validation.md](07-testing-and-validation.md) 的固定视口检查工作台、图库、广场、管理员审核、首页和认证页。必须覆盖中英文、深浅主题、键盘、长文本、坏图和 console/network 错误。

特别确认：

- 首页 `/images/sub2api-workbench.webp` 非 404。
- 实时/异步不只靠颜色区分。
- 移动端没有横向溢出或嵌套滚动。
- 私有图片不把一次性签名 URL 当持久数据。
- 待审预览使用管理员签名入口。
- 下架/撤回后旧公开链接立即失效。

`2026-07-22` 本机 Chrome Playwright 10 场景曾覆盖五视口、中英文、深浅主题，0 横向溢出、0 控件裁剪、0 console error；键盘可见焦点、工作台 `aria-live`、广场 dialog 焦点进入和关闭恢复均通过。该证据早于迁移 `187` 最后改动，当前工作树必须最终重跑。

## 10. 逻辑提交与 CI

建议提交顺序：

```text
1. 安全热修：严格图片校验、内容响应、自定义首页净化
2. 工作台与图库：能力矩阵、实时计费、统一对象、归档和 API
3. 审核广场与 UI：投稿/举报/迁移/维护 Worker、页面和可访问性
4. 交接文档：readmenew.md 与 wiki-new/**
```

每次提交前：

```powershell
git diff --check
git status --short
git add <本次明确范围内文件>
git diff --cached --stat
git diff --cached --check
git commit -m "<简体中文提交说明>"
```

先推功能分支：

```powershell
git push -u origin feat/image-workflow-library-moderation
```

等待 Fork GitHub Actions 全绿。CI 未通过时不要合并 `main`。通过后使用非强制合并，并推送：

```powershell
git switch main
git merge --no-ff feat/image-workflow-library-moderation
git push origin main
```

不要向 `upstream` 推送，不要使用 `git push --force`。

## 11. 最终核对

```powershell
git status --short --branch
git rev-parse HEAD
git describe --tags --always
git ls-remote origin refs/heads/main
Get-Content backend\cmd\server\VERSION
```

最终报告必须包含：

- `VERSION`
- 功能分支最终 SHA 和合并后 `main` SHA
- `git describe`
- 推送目标
- Fork CI URL/结果
- 后端/前端/浏览器实际测试结果
- 真实七牛/阿里/腾讯与上游计费是否验证
- 所有仍未完成项及原因
- 本交接文档路径

完成后把 [01-current-status.md](01-current-status.md) 和 [07-testing-and-validation.md](07-testing-and-validation.md) 中相应 `PENDING` 替换为证据；没有验证的项目保留 `PENDING`。

## 12. 一句话恢复上下文

```text
这是 JasonWangJie/sub2api Fork，VERSION 保持 0.1.162。先读 wiki-new/README.md、01-current-status.md、07-testing-and-validation.md 和 09-ai-handoff-checklist.md，再检查脏工作树。185 是持久异步任务，186 是统一对象/个人图库/审核广场，187 是 SC 上传 PostgreSQL admission/幂等/恢复。工作台模式只由 Key 当前分组决定，默认私有、公开需审核，所有计费复用现有链路。2026-07-22 的 Go/前端/Playwright 证据早于 187 最后改动；当前工作树最终重跑、真实 PostgreSQL/testcontainers、三家 OSS、真实上游计费、最终提交 SHA/git describe、Fork CI 和 origin/main 推送仍为 PENDING。
```
