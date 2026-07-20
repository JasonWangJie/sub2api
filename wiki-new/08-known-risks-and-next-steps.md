# 已知风险与后续工作

## P0：生产启用前必须完成

1. 使用真实 PostgreSQL、Redis 和测试 OSS 跑完整 staging/恢复/清理链路。
2. 用七牛、阿里、腾讯真实凭证分别验证 upload、HEAD/read、presign/public URL 和 delete。
3. 跑通 Gemini BB、Gemini SC、OpenAI generations、OpenAI edits JSON/multipart，并核对全部图片。
4. 对 1K/2K/4K、数量、倍率、余额、订阅、Key/账号额度做生产配置抽样核账。
5. 演练迁移、应用回滚、服务重启和加密密钥备份/恢复。
6. 在桌面和移动尺寸检查分组表单、存储配置、用户任务中心、管理员任务中心和图片预览。
7. 验证真实签名过期时间、90 天清理策略的缩短测试和失败重试。

## P0 风险说明

### 切换 OSS 后旧对象不可解析

系统当前只有一个全局 provider/bucket 配置，存储实现会拒绝与当前配置不匹配的旧 ObjectRef。直接切换可能让历史图片无法查看和清理。生产切换前必须保留旧配置到对象清空、迁移对象，或先实现历史凭证 resolver。

### execution_unknown 不能自动恢复

上游没有可靠幂等保证。请求发出后、结果入库前崩溃会进入 `execution_unknown`。自动重调可能生成第二张图并产生第二次上游成本，因此当前必须人工判断。若决定再生成，应创建新任务，并明确风险。

### PostgreSQL staging 容量

生成结果在 OSS 成功前短期存储在 PostgreSQL `BYTEA`。高并发 4K 多图会放大磁盘、WAL、备份和副本压力。上线前建立容量指标和告警，并验证 24 小时 staging 清理。

### 多实例并发

配置是每实例 Worker 数。扩容应用实例会线性提高总上游并发和 OSS 吞吐，可能越过账号或厂商限制。部署系统需要显式计算总 Worker 数。

### 密钥连续性

排队任务的请求载荷加密保存。迁移机器或重建容器时若更换加密密钥，旧任务无法解密执行。密钥备份和轮换策略必须在生产启用前确定。

## P1：稳定性和运维增强

- 实现历史 provider/bucket -> 凭证的 resolver，或提供对象迁移工具。
- 增加 ready/delayed/inflight、状态数量、最老任务年龄、账务失败和清理积压指标与告警。
- 对每个崩溃点做故障注入，验证不会重复调用上游和重复扣费。
- 为 `execution_unknown` 增加管理员“创建新任务”流程，要求二次确认并产生新 task ID。
- 修复 Windows `1ns` TTL 随机测试，恢复不带排除的 `go test ./...`。
- 更新 `admin.system.rollback.spec.ts` 两个既有期望，恢复完整前端测试绿色。
- 完成 Playwright 或浏览器工具的桌面/移动截图回归。
- 决定是否把 `0.5K` 像素上限等当前代码常量开放为受控配置。
- 增加各云厂商按环境变量启用的可选真实凭证契约测试。

## P2：产品能力候选

- 用户主动取消尚未开始的 queued 任务。
- 合规的任务删除与审计保留策略。
- 更细的任务中心导出、告警和运营统计。
- 多存储配置与按对象历史配置解析。
- 视频异步任务。视频需要单独协议、计费、存储和状态设计，不能直接复用图片接口命名。

这些不是当前交付遗漏，而是明确未包含的后续范围。实现前应先写新方案和验收标准。

## 上游同步高冲突区域

以后合并 `upstream/main` 时重点检查：

```text
backend/ent/schema/group.go 及生成的 ent/group* 文件
backend/internal/handler/gateway_handler_chat_completions.go
backend/internal/handler/openai_images.go
backend/internal/service/gateway_usage_billing.go
backend/internal/service/openai_gateway_usage.go
backend/internal/service/image_storage*.go
backend/internal/server/routes/{gateway,admin,user}.go
backend/internal/handler/wire.go
backend/cmd/server/wire_gen.go
frontend/src/views/admin/{GroupsView,BackupView}.vue
frontend/src/router/index.ts
frontend/src/components/layout/AppSidebar.vue
```

冲突解决后必须重新生成 Ent/Wire（若 schema 或 provider 变化），并回归旧同步、旧 Redis 异步和新持久异步三套路径。

## 发布决策

当前保留 `VERSION=0.1.162`。生产验收完成后再决定是否升级版本并新增 Fork 自己的 release notes/tag。不要修改或复用原作者的 `v0.1.162` 标签。
