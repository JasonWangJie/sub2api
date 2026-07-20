# Sub2API 异步生图二次开发交接 Wiki

本目录只记录 `JasonWangJie/sub2api` Fork 的持久化异步生图二次开发，不替代原项目 `README*.md`、`docs/` 或 `wiki/`。新的开发者或 AI 应先读本页，再按任务打开对应专题。

这是仓库内 Wiki，会随 `origin/main` 和 `git clone` 到达新电脑。GitHub 网站的 Wiki 标签页使用独立的 `sub2api.wiki.git` 仓库，本次不向那个独立仓库发布，也不应误报为已更新 GitHub Wiki。

## 当前结论

- 发布版本文件：`backend/cmd/server/VERSION = 0.1.162`，本次没有主动升级版本号。
- 开发基线：`6a57b47d7`，即合并原作者 `upstream/main` 后的本地 `main`。
- 精确交付提交：运行 `git rev-parse HEAD`。文档与功能在同一交付提交中，因此不硬编码自引用的最终 SHA。
- 功能状态：计划内代码已实现，后端和前端自动化验证已通过。
- 生产验收状态：尚未使用七牛、阿里、腾讯真实凭证逐厂商联调，也尚未完成桌面/移动浏览器截图验收。
- 明确不在范围：视频、取消任务、删除任务、在原任务号上重新调用上游。

不要把“代码完成”误写成“生产验收完成”。上线前仍应完成 [08-known-risks-and-next-steps.md](08-known-risks-and-next-steps.md) 中的 P0 项。

## 文档导航

| 文档 | 用途 |
|---|---|
| [01-current-status.md](01-current-status.md) | 完成度、文件范围、版本与 Git 状态 |
| [02-architecture.md](02-architecture.md) | 请求链路、持久化、队列、Worker 和状态机 |
| [03-api-contracts.md](03-api-contracts.md) | 7 条下游接口、BB/SC 语义和兼容边界 |
| [04-billing-and-idempotency.md](04-billing-and-idempotency.md) | 计费不变量、固定账单、幂等与故障处理 |
| [05-storage-and-retention.md](05-storage-and-retention.md) | OSS 厂商、结果链接、参考图安全和清理策略 |
| [06-deployment-and-configuration.md](06-deployment-and-configuration.md) | 迁移、配置、构建、上线和回滚前检查 |
| [07-testing-and-validation.md](07-testing-and-validation.md) | 已执行测试、已知无关失败和验收清单 |
| [08-known-risks-and-next-steps.md](08-known-risks-and-next-steps.md) | 风险、限制和后续优先级 |
| [09-ai-handoff-checklist.md](09-ai-handoff-checklist.md) | 下一台电脑或下一位 AI 的无缝接手步骤 |

对外调用示例、完整请求体和 BB/SC 返回体以 [../docs/DURABLE_ASYNC_IMAGE_API.md](../docs/DURABLE_ASYNC_IMAGE_API.md) 为权威来源。原有 Redis 异步接口仍看 [../docs/ASYNC_IMAGE_TASKS.md](../docs/ASYNC_IMAGE_TASKS.md)。

发生文档差异时的真值优先级是：数据库迁移和实际代码 > `docs/DURABLE_ASYNC_IMAGE_API.md` > 本交接 Wiki > `readmenew.md` 摘要。`docs/图片生成新功能请求说明.md` 是需求来源，不是最终上线契约。

## 绝对不能破坏的约束

1. BB、SC 只是下游方言，不是上游供应商；真正的上游由 API Key 当前分组决定。
2. 新开关只控制新接口。旧同步接口和旧 `/async` 接口不得自动切换到新逻辑。
3. 上游成功后才按现有规则准备账单；一旦准备，重试不得按新价格重新计算。
4. `billing_failed`、`storage_failed` 只续跑后处理，不得再次调用 Gemini/OpenAI。
5. `execution_unknown` 禁止自动重调。需要重新生成时必须创建新任务号，并接受可能产生第二次上游费用。
6. 普通用户只有在 OSS 结果已持久化且账务为 `succeeded` 或 `not_billable` 时才能看结果链接。
7. 公共查询必须使用提交任务的同一 API Key；其他 Key 返回 `404`，不能泄露任务存在性。
8. 不要把对象存储的预签名 URL写入任务表；只保存 provider、bucket 和 object key 等稳定引用。

## 快速定位

后端入口主要在：

```text
backend/internal/handler/durable_async_image_handler.go
backend/internal/handler/durable_async_image_worker.go
backend/internal/handler/async_image_task_center_handler.go
backend/internal/service/async_image_task.go
backend/internal/service/async_image_protocol.go
backend/internal/service/prepared_usage_billing.go
backend/internal/repository/async_image_task_repo.go
backend/internal/repository/async_image_queue.go
backend/migrations/185_async_image_tasks.sql
```

前端入口主要在：

```text
frontend/src/features/async-image-tasks/
frontend/src/views/admin/GroupsView.vue
frontend/src/views/admin/BackupView.vue
frontend/src/views/admin/groupsAsyncImage.ts
frontend/src/router/index.ts
frontend/src/components/layout/AppSidebar.vue
```

开始工作前先执行：

```bash
git status --short --branch
git remote -v
git log -5 --oneline --decorate
git describe --tags --always --dirty
```

`origin` 应是 `JasonWangJie/sub2api`，`upstream` 应是 `Wei-Shaw/sub2api`。任何推送都显式使用 `git push origin HEAD:main`，不要推送到 `upstream`。
