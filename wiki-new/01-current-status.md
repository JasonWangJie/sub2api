# 当前状态与完成度

## 版本和仓库

| 项目 | 当前值 |
|---|---|
| 发布版本 | `0.1.162` |
| 开发前 HEAD | `6a57b47d724bad12e5c3ed3e039cd4d33ea03897` |
| 开发分支 | `main` |
| 用户 Fork | `origin = https://github.com/JasonWangJie/sub2api.git` |
| 原作者仓库 | `upstream = https://github.com/Wei-Shaw/sub2api.git` |
| 数据库迁移 | `backend/migrations/185_async_image_tasks.sql` |

本次不修改 `backend/cmd/server/VERSION`。交付后的完整版本必须同时报告版本文件、`git describe` 和提交 SHA，不能只报 `0.1.162`。

## 实施状态

| 范围 | 状态 | 说明 |
|---|---|---|
| 分组异步生图开关 | 已完成 | `allow_async_image_generation` 默认关闭，仅 Gemini/OpenAI 且普通生图开启时有效 |
| 7 条下游接口 | 已完成 | Gemini BB、OpenAI BB、Gemini SC 提交/上传/查询 |
| Gemini 执行 | 已完成 | 标准 `generateContent` 文生图、图生图和全部 inline image 提取 |
| OpenAI 执行 | 已完成 | 标准 generations/edits，兼容 JSON URL 和 multipart |
| 持久任务与事件 | 已完成 | PostgreSQL 任务、结果、事件、Outbox、暂存和输入对象 |
| 可靠队列与恢复 | 已完成 | Redis ready/delayed/active/inflight、租约、心跳、CAS 和启动恢复 |
| 计费 | 已完成 | 复用现有规则，固定 Prepare/Apply 命令，幂等扣费和用量日志补写 |
| OSS | 已完成 | 七牛、阿里、腾讯、`custom_s3`，真实上传/HEAD/删除探测 |
| 保留清理 | 已完成 | 输入默认 24 小时，任务和结果默认 90 天 |
| 用户任务中心 | 已完成 | 查看本人所有 Key 的任务和图片 |
| 管理员任务中心 | 已完成 | 全站筛选、详情、时间线和仅后处理续跑 |
| 自动化验证 | 已完成 | Go 编译、测试、vet；前端 lint、类型、目标测试和构建 |
| 真云厂商契约验收 | 待完成 | 缺少七牛/阿里/腾讯真实测试凭证 |
| 浏览器视觉验收 | 待完成 | 本轮浏览器控制工具不可用，未完成桌面/移动截图检查 |

## 主要改动边界

后端改动包括 Group Ent 字段、管理 DTO/服务、API Key 缓存快照、新路由、任务 Handler/Worker、队列和任务 Repository、计费 Prepare/Apply、存储抽象、运行配置、迁移与 Wire 生成文件。

前端改动包括分组表单开关、备份页的图片存储/异步运行参数、用户与管理员任务中心、路由、侧栏、中英文文案、API 和类型。

权威接口文档是 `docs/DURABLE_ASYNC_IMAGE_API.md`。`docs/ASYNC_IMAGE_TASKS.md` 描述的是旧 Redis 24 小时任务，两套接口和数据不可混用。

## 已修复的关键风险

- 结果泄露：普通用户在账务确认前看不到图片 URL；管理员仍可诊断存储故障。
- 长任务误恢复：Worker 每 15 秒刷新 Redis 和 PostgreSQL 心跳，恢复更新带 `updated_at` 条件。
- 旧 Gemini 路由能力泄漏：图片捕获由 Worker 私有 context capability 开启，下游 JSON 无法伪造。
- Key 软删除逃费：结算可加载 tombstone Key；软删除 Key 仍不能通过正常鉴权。
- Gemini 规格计费错误：使用生成图片的真实宽高选择档位；允许的 `0.5K` 进入现有 `1K` 档。
- 重试预算串扰：`storage_retry_count` 和 `billing_retry_count` 独立，`retry_count` 只保留为总计。
- simple 模式：生成明确的 `not_billable` 固定命令，不扣费但仍写用量日志。

## 当前未承诺的能力

- 视频生成或视频任务。
- 用户取消、删除任务。
- 对 `execution_unknown` 原任务重新调用上游。
- 自动迁移旧 Redis 异步任务。
- SC 方言调用 OpenAI。
- 多套 OSS 配置并行解析历史对象。

## 完成定义

代码层面的计划范围已经完成。生产上线要再满足以下条件：真实存储凭证的上传/读取/删除契约测试通过，测试环境完成 Gemini/OpenAI 端到端生成和账务核对，桌面与移动任务页面完成视觉验收，并验证部署环境的 `public_base_url`、反向代理和预签名链接。
