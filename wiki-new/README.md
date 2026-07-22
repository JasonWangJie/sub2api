# Sub2API 图片工作流二次开发交接 Wiki

本目录只记录 `JasonWangJie/sub2api` Fork 的图片相关二次开发，不替代原项目 `README*.md`、`docs/` 或 `wiki/`。内容覆盖：持久化异步生图任务中心，以及图片工作台、服务端个人图库、审核广场、安全迁移，与本机延期投稿（审核通过后再同步 OSS）。

这是随 Git 仓库分发的交接资料。GitHub 网站的 Wiki 标签页属于独立的 `sub2api.wiki.git` 仓库；本目录没有声称更新那个独立仓库。根目录 `异步生图接口文档.md` 是面向下游对接方的异步 API 说明，不替代本交接 Wiki。

## 当前结论

- 发布版本文件仍是 `backend/cmd/server/VERSION = 0.1.162`，本轮不主动升级版本号。
- 本轮开发基线是 `51b083d374decf811ac88f8b0194165db9a8ba79`，基线描述为 `v0.1.162-4-g51b083d37`。
- 文档记录时 HEAD 为 `63db6642`；`188` 延期投稿、OSS 年月日目录与工作台 UX 等多在 dirty 工作树，以 `git status` 为准。
- 当前及后续默认工作分支是 `main`；历史功能分支 `feat/image-workflow-library-moderation` 保留用于审计。
- 图片工作台、服务端图库、统一对象引用、投稿审核、举报、安全迁移、批量审核、维护 Worker、SC 上传安全层，以及迁移 `188` 本机延期投稿的代码已存在（后者可能尚未单独提交）。
- `2026-07-22` 已合并 `upstream/main=5a8d6c4e4`；合并后的强制 Go 全仓、server build、前端 frozen/lint/typecheck、189 files/1277 tests 和 974 modules build 全部通过。
- 合并后的浏览器控制器被当前环境缺失 `sandboxPolicy` 元数据阻断；历史 Chrome 十场景证据不能冒充当前复验。
- 功能分支已推送，功能代码以 `a9d23973d` 非强制合并进 `main` 并交付 `origin/main`。这是用户明确要求先于 CI 完成的主线交付；Fork Actions 页面仍显示 `Enable Actions`，CI 历史运行数仍为 0。真实 PostgreSQL/testcontainers、真实云厂商与上游计费也仍为 `PENDING`。
- 没有真实执行记录的项目一律不得改写为“通过”或“生产可用”。

精确状态以 [01-current-status.md](01-current-status.md) 为准。后续每项外部验收完成后，应把其中对应的 `PENDING` 更新为真实命令、时间、结果和提交 SHA。

## 文档导航

| 文档 | 用途 |
|---|---|
| [01-current-status.md](01-current-status.md) | 版本、工作树、完成度与待交付项 |
| [02-architecture.md](02-architecture.md) | 持久异步任务链路、队列、状态机和恢复边界 |
| [03-api-contracts.md](03-api-contracts.md) | BB/SC 下游契约和站内图片 API |
| [04-billing-and-idempotency.md](04-billing-and-idempotency.md) | 计费不变量、混合尺寸、固定账单与幂等 |
| [05-storage-and-retention.md](05-storage-and-retention.md) | OSS、统一对象引用、签名、配额和保留策略 |
| [06-deployment-and-configuration.md](06-deployment-and-configuration.md) | `185/186/187/188` 迁移、配置、部署和回滚 |
| [07-testing-and-validation.md](07-testing-and-validation.md) | 已知测试证据、完整复验命令和验收矩阵 |
| [08-known-risks-and-next-steps.md](08-known-risks-and-next-steps.md) | P0 风险、产品缺口和后续优先级 |
| [09-ai-handoff-checklist.md](09-ai-handoff-checklist.md) | 新电脑或下一位 AI 的无缝接手步骤 |
| [10-image-workbench.md](10-image-workbench.md) | Key 分组驱动的实时/异步工作台 |
| [11-image-library-object-model.md](11-image-library-object-model.md) | 服务端个人图库与对象引用模型 |
| [12-moderated-plaza-and-migration.md](12-moderated-plaza-and-migration.md) | 审核广场、举报、旧广场迁移和维护 Worker |
| [13-local-development-runbook.md](13-local-development-runbook.md) | Windows/Docker 本地前后端运行、联调和故障排查 |
| [14-production-deployment-runbook.md](14-production-deployment-runbook.md) | Fork 镜像生产部署、HTTPS、OSS、备份、升级和回滚 |

持久异步下游请求/响应示例以 [../docs/DURABLE_ASYNC_IMAGE_API.md](../docs/DURABLE_ASYNC_IMAGE_API.md) 为权威来源。旧 Redis 24 小时异步接口仍看 [../docs/ASYNC_IMAGE_TASKS.md](../docs/ASYNC_IMAGE_TASKS.md)。原始需求文档 `docs/图片生成新功能请求说明.md` 只用于理解来源，不是最终上线契约。

发生差异时的真值顺序：

```text
数据库迁移与当前代码
  > docs/DURABLE_ASYNC_IMAGE_API.md（持久异步公共协议）
  > wiki-new 专题（架构与交接）
  > readmenew.md（摘要）
  > 早期聊天和原始需求草稿
```

## 两层系统关系

```text
下游客户端
  -> 原同步 API / 旧 Redis 异步 API / 新持久异步 BB、SC API

站内用户（实时）
  -> 图片工作台能力接口
  -> Key 当前分组决定实时执行
  -> 结果只保存在本机 IndexedDB（默认不入 OSS / 不入个人图库）
  -> 用户显式「投稿审核」（仅元数据）
  -> 管理员审核
  -> approved_pending_sync
  -> 用户再次上线「同步至图片广场」（此时才上传 OSS 并 published）

站内用户（异步 / 已归档图库）
  -> Key 当前分组决定异步执行
  -> 任务结果入 OSS，并可归档到私有个人图库
  -> 图库资产可走既有 publications 投稿审核
  -> 已批准图片广场
```

`185_async_image_tasks.sql` 负责持久异步任务；`186_image_library_and_plaza_moderation.sql` 负责统一对象、图库、投稿、举报、事件、Outbox、清理和旧广场迁移；`187_async_image_upload_reservations.sql` 负责 SC 上传 attempt/reservation/URL alias；`188_plaza_submission_deferred_upload.sql` 负责本机持图延期投稿队列。不要把四者回滚、清理或所有权规则混为一套。

## 绝对不能破坏的约束

1. BB、SC 只是下游方言；上游平台始终由 API Key 当前分组决定。
2. 旧同步接口、旧 Redis `/async` 接口和新持久异步接口保持独立，不因工作台或新开关改变旧响应体。
3. 工作台不允许用户手工选实时/异步；必须在提交前重新读取 Key 能力。
4. 实时与异步失败不能互相回退，避免重复生成和重复费用。
5. 所有新图片默认私有；只有显式投稿并审核通过的资产才进入公开广场。
6. **实时本机投稿在审核前不得占用 OSS**；只有 `approved_pending_sync` 后用户同步才上传并发布。
7. 图库归档失败不能重新调用上游、改变生成成功状态或产生第二笔费用。
8. 异步上游成功后固定账单；后处理重试不重新计算价格，不重复扣费。
9. `execution_unknown` 禁止自动重调；再次生成必须创建新任务号。
10. 对象可能被异步结果、图库和有效投稿共同引用；删除前必须检查全部引用。
11. 数据库不保存预签名 URL，不向普通用户泄露内部 user ID、对象 key、上游账号或完整错误。
12. 旧广场内容未经过新校验和审核，升级时先隐藏，再迁为私有和待审；绝不能直接继续公开。
13. SC 上传必须经过 PostgreSQL 两阶段 admission；幂等重放只能重签同一对象，不能因数据库/OSS故障 fail open 或漏计配额。
14. 上传 object intent 必须先于 OSS 持久化；Put 最大 600 秒；失败 intent 至少间隔十分钟二次删除且清理前继续计入容量。
15. URL alias 始终复核 Key 所有权，每输入对象最多 128 个且过期记录保留为墓碑；客户端文件名不得进入 object key。
16. OSS 对象 key 由服务端按 UTC 年/月/日分区生成，客户端路径不得拼入 key。
17. 视频不在本轮范围。

## 快速定位

后端主要入口：

```text
backend/migrations/185_async_image_tasks.sql
backend/migrations/186_image_library_and_plaza_moderation.sql
backend/migrations/187_async_image_upload_reservations.sql
backend/migrations/188_plaza_submission_deferred_upload.sql
backend/internal/handler/durable_async_image_*.go
backend/internal/service/async_image_upload.go
backend/internal/repository/async_image_upload_repo.go
backend/internal/handler/image_workbench_handler.go
backend/internal/handler/image_library_handler.go
backend/internal/handler/image_plaza_handler.go
backend/internal/service/image_workbench.go
backend/internal/service/image_library.go
backend/internal/service/image_plaza_submission.go
backend/internal/service/image_library_maintenance.go
backend/internal/service/image_plaza_helpers.go
backend/internal/repository/image_library_repo.go
backend/internal/repository/image_plaza_submission_repo.go
```

前端主要入口：

```text
frontend/src/features/image-workflow/
frontend/src/features/image-workflow/submissionBlobStore.ts
frontend/src/views/user/ImageWorkbenchView.vue
frontend/src/views/user/ImageLibraryView.vue
frontend/src/views/user/ImagePlazaView.vue
frontend/src/views/user/AsyncImageTasksView.vue
frontend/src/views/admin/ImageModerationView.vue
frontend/src/api/imageWorkbench.ts
frontend/src/api/imageLibrary.ts
frontend/src/api/imagePlaza.ts
```

开始工作前执行：

```bash
git status --short --branch
git remote -v
git log -5 --oneline --decorate
git describe --tags --always --dirty
```

`origin` 应指向 `JasonWangJie/sub2api`，`upstream` 应指向 `Wei-Shaw/sub2api`。不要推送到 `upstream`，不要对共享分支使用 force push。
