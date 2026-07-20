# 架构、状态机与恢复

## 主链路

```text
下游请求
  -> API Key 鉴权、平台/分组/普通生图/异步开关检查
  -> PostgreSQL 同事务创建 task + initial event + outbox
  -> Outbox 发布到 Redis ready 队列
  -> Worker 租约 + 数据库 CAS 抢占
  -> 再加载用户、Key、分组、订阅和账号上下文
  -> 现有 Gemini/OpenAI 账号调度与上游执行链路
  -> 原子保存图片暂存字节 + 实际规格 + 固定账单命令
  -> OSS 上传并保存稳定对象引用
  -> 同步 Apply 固定账单和用量日志
  -> succeeded 后向普通用户暴露图片 URL
```

PostgreSQL 是任务事实来源。Redis 只负责投递、延迟、活跃租约和心跳；Redis 数据丢失时，恢复扫描会根据 PostgreSQL 的可恢复状态重新入队。

## 数据表

| 表 | 作用 |
|---|---|
| `async_image_tasks` | 所有权、协议、平台、模型、状态、规格、费用、重试、时间和脱敏错误 |
| `async_image_results` | OSS provider/bucket/object key、MIME、字节、checksum、宽高 |
| `async_image_input_objects` | SC 参考图上传对象、所有权、URL 哈希和过期时间 |
| `async_image_task_inputs` | 活动任务与参考图的引用关系，防止输入被提前清理 |
| `async_image_staging_objects` | 上游已成功但尚未完成 OSS/账务的短期图片字节 |
| `async_image_events` | 阶段时间线和状态变更 |
| `async_image_outbox` | 与任务同事务写入的可靠投递记录 |

完整字段和约束见 `backend/migrations/185_async_image_tasks.sql`。

## 内部状态机

```text
queued -> invoking -> upstream_succeeded -> uploading -> billing_pending -> succeeded
                 \-> failed
                 \-> execution_unknown
                                      uploading -> storage_failed
                               billing_pending -> billing_failed
任何已到保留期的终态 -> expired/清理
```

实际执行可能跳过只用于展示的中间状态，但必须遵守以下语义：

| 状态 | 含义 | 是否允许自动再次调用上游 |
|---|---|---|
| `queued` | 已持久化，等待 Worker | 是，尚未发出上游请求 |
| `invoking` | 已抢占并正在调用上游 | 否，除非能确认请求从未发出 |
| `upstream_succeeded` | 图片和固定账单已持久化 | 否 |
| `uploading` | 正在上传 OSS | 否，只重试存储 |
| `billing_pending` | 存储完成，等待账务确认 | 否，只重试账务/日志 |
| `succeeded` | 存储和账务均完成 | 否 |
| `failed` | 调用上游前或有明确失败响应 | 否；新请求应创建新任务 |
| `execution_unknown` | 请求可能已到达上游但结果未持久化 | 绝对禁止 |
| `storage_failed` | 图片暂存存在但存储重试耗尽 | 否；管理员可续跑后处理 |
| `billing_failed` | 固定账单 Apply 或用量日志失败 | 否；管理员可续跑后处理 |
| `expired` | 已到保留期 | 否 |

BB 将内部中间态渲染为 `queued/processing`，成功为 `succeeded`；SC 渲染为 `pending/processing/completed`。不要直接把内部状态字段拼进公共响应。

## 并发与心跳

- Worker 使用数据库版本号 CAS，避免多个实例同时主动执行同一任务。
- Redis inflight 租约和 PostgreSQL `updated_at` 均由心跳刷新；当前实现约每 15 秒刷新一次。
- 恢复扫描只接管超过租约阈值的任务，并在转移时再次检查旧 `updated_at`，避免把仍在运行的长任务改成不确定状态。
- `storage_retry_count` 与 `billing_retry_count` 各自消耗独立预算；`retry_count` 是汇总观察字段。
- 服务启动和周期恢复都会处理未发布 Outbox、延迟任务、失效租约和可恢复的 PostgreSQL 状态。

## 崩溃边界

标准 Gemini/OpenAI 同步生图没有可依赖的上游幂等保证。最危险的窗口是“请求已经发出，但进程还没把上游结果写进 PostgreSQL”。该窗口恢复为 `execution_unknown`，目的是防止静默生成第二次和产生第二笔上游成本。

上游成功后，图片字节、真实尺寸和固定账单命令在进入后处理前持久化。此后进程重启只会继续上传和账务，不会重新生成。暂存数据完成上传后尽快删除，异常残留由保留任务清理。

## 请求数据与错误

- 规范化请求体跨越 Repository 边界前加密保存，不保存原始 API Key。
- Worker 通过 API Key ID 重新加载上下文。
- 任务终态清除完整请求载荷，只保留请求哈希、可配置的脱敏提示词摘要和脱敏错误。
- 公共接口与用户页面不得展示最终账号、内部对象 key、账单载荷或详细上游错误；管理员详情用于运维诊断。
