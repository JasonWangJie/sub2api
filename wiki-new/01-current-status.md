# 当前状态与完成度

更新时间：`2026-07-22`。本页记录的是交接时工作树状态，不是发布公告。

## 版本和仓库

| 项目 | 当前值 |
|---|---|
| 发布版本 | `0.1.162` |
| 本轮开发基线 | `51b083d374decf811ac88f8b0194165db9a8ba79` |
| 基线描述 | `v0.1.162-4-g51b083d37` |
| 当前及后续默认分支 | `main` |
| 已合并上游 | `upstream/main 5a8d6c4e41e38f05cea4164e6ff03443fc0f6923` |
| 上游合并提交 | `433cf0096` |
| 合并后代码验证提交 | `6412b5eb7` |
| 用户 Fork | `origin = https://github.com/JasonWangJie/sub2api.git` |
| 原作者仓库 | `upstream = https://github.com/Wei-Shaw/sub2api.git` |
| 持久异步迁移 | `backend/migrations/185_async_image_tasks.sql` |
| 图库/审核迁移 | `backend/migrations/186_image_library_and_plaza_moderation.sql` |
| SC 上传安全迁移 | `backend/migrations/187_async_image_upload_reservations.sql` |
| 功能代码主线合并提交 | `a9d23973d352c9923eccdaf789ffd2598d9d0ffe` |
| 合并提交描述 | `v0.1.162-52-ga9d23973d` |
| 功能分支远端 | 已推送 `origin/feat/image-workflow-library-moderation` |
| 功能分支 CI | `BLOCKED`：Fork Actions 页面显示 `Enable Actions`，历史运行数为 0 |
| 合并并推送 `origin/main` | `COMPLETED`：用户明确要求直接交付主线，已非强制合并并推送 |

本轮不主动修改 `backend/cmd/server/VERSION`。最终报告不能只写 `0.1.162`，必须同时写完整 SHA、`git describe`、推送目标和 CI 结果。

## 工作树中已经存在的实现

| 范围 | 代码状态 | 说明 |
|---|---|---|
| 持久异步任务中心 | 已存在 | 保留上一轮 Gemini BB、OpenAI BB、Gemini SC、PostgreSQL/Redis Worker、固定账单和 OSS 结果 |
| 工作台能力接口 | 已存在 | 按 API Key 所属用户、分组、平台和开关返回 `capability_version`、模式、协议、模型和参数 |
| Key 分组模式矩阵 | 已存在 | OpenAI/Gemini 实时或异步，Grok 仅实时，Antigravity/其他平台不可用 |
| Gemini 实时图片计费采集 | 已存在 | 图片意图、权限/并发门禁、全部 `inlineData`、真实宽高和输出数量 |
| 混合尺寸计费 | 已存在 | 按每张实际规格档位求和，不再按最大规格乘总数量 |
| 服务端个人图库 | 已存在 | OSS 归档、私有默认、游标分页、查看、编辑、软删除、投稿和撤回 |
| 异步结果自动归档 | 已存在 | 成功任务写归档 Outbox；任务结果与图库复用对象身份，归档幂等 |
| 统一对象引用 | 已存在 | `image_storage_objects` 供异步结果、图库和投稿共享；删除检查活动引用 |
| 审核广场与举报 | 已存在 | `pending_review`、批准、拒绝、下架、恢复、撤回、举报处理和审计事件 |
| 严格图片校验 | 已存在 | PNG/JPEG/WebP 完整容器、实际 MIME、字节/像素限制和常见 polyglot 防护 |
| 旧广场安全迁移 | 已存在 | 先隐藏旧公开记录，再校验、OSS 归档、转私有和待审；危险数据隔离计数 |
| 图库维护 Worker | 已存在 | PostgreSQL 租约/心跳、Outbox、过期清理、对象删除恢复和旧数据迁移 |
| 管理员配置 | 已存在 | 统一 OSS、异步参数、图库保留、条目/容量/单图限制、签名和限频 |
| 用户与管理页面 | 已存在 | 工作台、图库、审核广场、举报、清理/迁移状态和视觉/可访问性调整 |
| 批量审核 | 已存在 | 静态优先的批量 API、管理端批量批准/拒绝 UI、逐项结果和审计状态转换 |
| 旧删除兼容 | 已存在 | 历史数字 ID、`imgpub_*` 投稿 ID 和 `img_*` 资产 ID 均映射到安全撤回/软删除 |
| 异步归档恢复 | 已存在 | 启动回填升级前成功任务；永久错误终止重排，临时错误才延迟重试 |
| 存储身份保护 | 已存在 | 非 `deleted` 对象存在时拒绝 provider/bucket/endpoint/region/path-style 身份变更 |
| 进程关闭 | 已存在 | server cleanup 同时等待持久异步 Handler 与图库维护 Worker 的 `Stop()` |
| SC 上传 admission | 已存在 | body 前滚动限频，解码/OSS 前幂等与 Key 字节 reservation，PostgreSQL 故障 fail closed |
| SC 上传恢复 | 已存在 | deterministic intent、300/600 秒 Put 超时、至少间隔十分钟二次 Delete、128 alias 上限/墓碑、failed intent 配额和文件名净化 |

“已存在”只表示代码已经在当前工作树中，不等于已经通过完整测试、CI 或生产验收。

## 本地证据与剩余交付门禁

| 门禁 | 状态 | 完成证据要求 |
|---|---|---|
| 最新 Go 格式化和生成代码 | `PASSED POST-MERGE 2026-07-22` | `gofmt`、`go generate ./cmd/server` 通过且生成代码无差异 |
| 后端全量测试 | `PASSED POST-MERGE 2026-07-22` | Go 1.26.5：交汇定向测试；强制 unit `277.9s`、默认全包 `204.4s`、server build 通过 |
| PostgreSQL 集成与恢复测试 | `PENDING` | `185/186/187`、两阶段 admission、多 Worker、stale lease、Outbox、intent/OSS 部分失败、对象引用、迁移幂等 |
| 前端全量门禁 | `PASSED POST-MERGE 2026-07-22` | frozen install、ESLint、typecheck、189 files/1277 tests、974 modules build 通过 |
| 浏览器验收 | `BLOCKED BY TOOLING` | Vite :3000 正常；内置浏览器缺失 sandboxPolicy 元数据，历史 Chrome 10 场景不冒充当前结果 |
| 首页工作台图片资源 | `PASSED 2026-07-22` | WebP `79,374` 字节，首页加载成功 |
| 真实 OSS 厂商契约 | `PENDING` | 七牛、阿里、腾讯逐一 upload、HEAD/read、签名/公开 URL、delete |
| 真实上游与计费核对 | `PENDING` | Gemini/OpenAI/Grok 实时与异步、余额/订阅/倍率/额度逐笔核对 |
| 逻辑拆分提交 | `PASSED` | 主体三笔、`f16c2106a` SC 安全、`81ac080ed` 文档、`433cf0096` 上游合并、`6412b5eb7` 锁文件修复 |
| Fork CI | `BLOCKED` | 工作流文件 active 且监听 push，但 Fork 尚未 Enable Actions；启用后在 `main` 明确触发并核对 |
| 合并 `origin/main` | `PASSED 2026-07-22` | 用户明确覆盖原 CI 等待顺序；功能代码以 `a9d23973d` 非强制合并，最终远端 SHA 见交付报告或实际 `origin/main` |

## 明确未承诺或尚未完成的产品范围

- 视频生成、视频任务或视频广场。
- 实时与异步模式的手工切换或失败互相回退。
- 对 `execution_unknown` 的原任务自动重调。
- 用户取消或删除持久异步任务。
- SC 方言调用 OpenAI。
- 多套 OSS 凭证同时解析历史对象。

## 上线完成定义

只有同时满足以下条件，才可以把状态改为“生产验收完成”：

1. `185`、`186`、`187` 在真实 PostgreSQL 备份副本上迁移成功，SC admission/intent 可恢复，旧公开内容立即隐藏且迁移可恢复。
2. 最新完整后端和前端门禁通过，Fork CI 全绿。
3. 实时/异步四象限、Grok 实时、图库归档失败和异步幂等计费均完成端到端核对。
4. 三家 OSS 的上传、查看、签名过期、对象共享引用和删除完成真实凭证验证。
5. 五种视口及键盘/屏幕阅读器关键路径无阻塞问题。
6. 已拆分提交并推送功能分支，随后非强制合并到 `origin/main`。
7. 本页所有必须门禁的 `PENDING` 已替换为可复核证据。
