# OpenAI 账号卡 429

## 配置与入口

普通 OpenAI OAuth、Setup Token 文本账号可以在账号编辑中启用「卡 429」，默认关闭。配置存放在 `extra.openai_429_mode_enabled`，值只能是布尔值，不新增数据库列。

批量编辑提供「不修改 / 启用 / 关闭」，勾选账号和按筛选结果修改共用原批量接口。只要目标包含 API Key、Spark 影子或其他独立额度池账号，整批在写入前拒绝，错误中包含账号 ID 和名称。单独开关更新保留其他配置。

覆盖 Responses（包括 compact）、聊天兼容转发和 WebSocket 每轮实际推理调用。图片和 Spark 请求不会进入文本计数；返回图片的响应也不会重置文本计数。

`openai_429_mode_state` 和 `openai_429_mode_generation` 由服务端维护，普通编辑传入这两个字段会被剔除。完整编辑在账号行锁内保留最新运行状态，并拒绝过期的配置代次。

## 调度与状态

| 阶段 | 行为 |
| --- | --- |
| 普通调度 | 沿用原调度方式 |
| 集中调度 | 5 小时或 7 天窗口有效已用比例达到 90%，按比例降序、原优先级、账号 ID 排序，使用原并发上限 |
| 等待在途请求 | 达到 100% 或真实调用明确返回额度耗尽 429；拒绝新增调用，处理在途结果并等待原并发槽释放 |
| 自动请求 | 同一账号由一个租约持有者串行发送固定短文本 |
| 等待真实请求 | 连续 9 次或本轮预算耗尽后，通过 Redis 原子操作发放一个真实请求名额 |
| 真实请求处理中 | 该名额已领取，其他真实调用和自动请求均等待 |
| 停调 | 第 10 次连续 429 后停止推理探测，等待额度恢复 |

额度快照有效期为 5 分钟；过期、缺失、非有限数值、已过重置时间的窗口不会触发集中调度。两个窗口耗尽时按较晚的可靠重置时间恢复。分组、模型、权限、成本、健康及必要的上下文绑定仍按现有规则检查；集中调度优先于普通会话粘性。

首次真实额度 429 计为第 1 次，再自动补 8 次即可到达 9。真实成功或自动成功将连续计数清零。自动轮次最多 30 次或 30 秒，成功不会刷新该预算；预算用尽保留计数并交回一次真实请求。非 429 错误、超时、取消或不完整响应会中断连续序列并退出本轮。已有并发响应先全部处理，最后结果决定下一阶段。

自动请求使用已映射的上游模型、账号认证、代理和原请求构造链，只携带固定短文本。没有模型记录时等待真实请求。自动请求直接调用上游传输，不进入用户计费、余额扣减或用户成功统计；记录独立次数、耗时、结果和可解析的上游 token 用量。

HTTP 200 或 SSE `[DONE]` 本身不算成功；必须解析到完整有效终态。每次实际上游调用有独立票据，响应关闭、错误处理和重复终态不能重复计数。WebSocket 已建立连接也会逐轮检查最新开关和准入。

## 一致性与恢复

Redis 使用账号键 `openai:429-mode:{账号 ID}` 和 Lua 比较交换，原子维护计数、在途票据、执行租约、真实请求名额和版本。停调投影写入 PostgreSQL `extra`，与调度 outbox 共用事务。服务重启或 Redis 键丢失时可从持久化停调恢复；过期票据不能被迟到结果重新使用。

Redis 故障时启用账号的新文本调用暂停，调度器可以尝试其他账号。持久化故障同样不发放新的准入。恢复、关闭及手动解除会切换配置代次，旧代次结果不会重新启动流程。

重置时间不可靠时，每 5 分钟查询额度，并确认所有已知耗尽窗口均恢复后放行。自动推理轮次有 30 秒期限；运行中关闭会取消探测，跨实例通过定期复查开关和代次取消。服务启动扫描启用账号，退出先取消并等待协调服务，再关闭依赖。

启用期间不执行原额度阈值暂停和自动重置卡决策，但保留原配置。已排队的自动重置卡任务在取得执行权后再次读取开关。只接管可确认的额度暂停；数据库和 Redis 均条件清理，保护随后出现的凭据和健康限制。管理员原有停调继续生效。

## 关闭时的影响边界

账号未配置该字段或值为 `false` 时，HTTP Responses、聊天兼容请求沿禁用快速路径执行，不读取请求体、不访问本协调器的 Redis，也不额外读取账号。系统没有任何启用账号时，普通调度在列举候选账号前直接返回；若只有部分账号启用，集中调度也只考虑这些账号，其他账号继续使用原会话粘性、429、额度阈值、自动重置卡、健康、计费和并发逻辑。

协调服务仍会在进程启动时及每 30 秒通过现有 `accounts.extra` GIN 索引查询一次启用账号列表，用于多实例发现开关变化；查询期间不持有请求侧内存锁。已建立 WebSocket 为满足逐轮读取最新开关的约定，每个文本 `response.create` 会额外读取一次最新账号配置。自动重置卡真正执行前也会额外读取一次账号，以防排队期间开启本功能。以上读取不改写业务状态。

API Key、Spark 影子、图片和独立额度池始终绕过本功能。Setup Token 的额度查询现在可读，但仍不允许消费重置卡；这是额度管理接口能力扩展，不改变其文本转发、计费或原限流结果。

## 本地验收

后端测试覆盖配置及批量预检、90% / 100% 边界、双窗口、快照过期、原并发与粘性约束、真实 429 + 自动 8 次 + 单次真实请求、成功重累计、30 次 / 实际 30 秒预算、并发票据竞争、迟到结果、WebSocket 多轮、运行中关闭、持久化停调重建、未知重置查询、图片 / Spark 隔离和自动重置卡抑制。

模拟上游只在本机测试中运行，无真实账号凭据；数据库持久化和 outbox 使用 SQL mock 验证，Redis 协议及 Lua 使用 miniredis 验证。它们不替代真实 PostgreSQL / Redis 集群、浏览器交互和真实上游验收。

在 `backend` 执行：

```powershell
go generate ./cmd/server
go test -tags unit ./internal/service ./internal/repository ./cmd/server -run 'Test(OpenAI429Mode|AdminOpenAI429Mode|OpenAIQuotaAutoReset|ProvideCleanup)' -count=1
go test -tags unit -short ./internal/service ./internal/repository ./internal/handler ./internal/handler/admin ./cmd/server -run 'Test(.*OpenAI.*|.*Account.*Scheduling.*|.*SchedulingThreshold.*|.*BulkUpdateAccounts.*|.*ClearRateLimit.*|ProvideCleanup)' -count=1
go build ./cmd/server
```

在 `frontend` 执行：

```powershell
pnpm exec vitest run src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/BulkEditAccountModal.spec.ts src/components/account/__tests__/OpenAI429ModeStatus.spec.ts src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts src/views/admin/__tests__/AccountsView.selectAllResults.spec.ts
pnpm exec vue-tsc --noEmit
pnpm run build
```

另执行 `git diff --check`。当前安装的 golangci-lint 2.9.0 不兼容项目 Go 1.27 的导出数据，不能将其报告视为静态检查通过。

真实账号是否能够超额成功调用尚未实测；本地模拟状态机通过不能证明该结论。Fork 安装、升级、Release CI 和发行身份配置保持不变。
