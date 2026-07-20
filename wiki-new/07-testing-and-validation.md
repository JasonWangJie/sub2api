# 测试与验收记录

## 最近验证快照

日期：`2026-07-21`。

功能代码与自动化测试基本完成，但尚未完成带真实云厂商凭证的生产验收。下面只记录实际执行过的结果，不把未执行项目写成通过。

## 后端已通过

在 `backend/` 执行：

```bash
go test ./... -run '^$'
go test ./... -skip '^TestContentModerationRuntimeSnapshotRefreshFailureKeepsStaleConfig$' -count=1
go vet ./internal/service ./internal/repository ./internal/handler
go test -tags unit ./internal/repository -run SettlesSoftDeletedKey
go test -tags unit ./internal/service -run TestGatewayServicePrepareRecordUsage_SimpleModeCreatesNotBillableCommand
```

这些命令覆盖全包编译、全量 Go 测试（排除一个既有 Windows 时钟随机用例）、关键包 vet、软删除 Key 结算和 simple 模式固定账单。

Wire 已重新生成并通过编译：

```bash
go run github.com/google/wire/cmd/wire@v0.7.0 ./cmd/server
```

## 前端已通过

在 `frontend/` 执行：

```bash
pnpm exec eslint src/features/async-image-tasks src/views/admin/GroupsView.vue src/views/admin/BackupView.vue src/views/admin/groupsAsyncImage.ts src/views/admin/__tests__/groupsAsyncImage.spec.ts src/router/index.ts src/components/layout/AppSidebar.vue
pnpm typecheck
pnpm test:run -- src/features/async-image-tasks/__tests__/api.spec.ts src/views/admin/__tests__/groupsAsyncImage.spec.ts
pnpm build
```

目标测试结果为 2 个测试文件、9 个测试通过；生产构建完成，记录为 960 个模块转换成功。

## 格式检查已通过

```bash
git diff --check
gofmt -l <本次所有 Go 文件>
```

没有 Go 格式遗漏。Windows 工作区只出现 `.gitignore` 和 `backend/go.sum` 的 LF/CRLF 提示，不是 `diff --check` 错误。

## 已知无关测试问题

### Go 随机失败

完整 `go test ./...` 在 Windows 可能随机失败：

```text
TestContentModerationRuntimeSnapshotRefreshFailureKeepsStaleConfig
```

该既有测试使用 `1ns` TTL；Windows 连续 `time.Now()` 可能落在同一时钟刻度。它与异步生图无关，但后续应把测试改成确定性时钟或合理 TTL，再恢复无排除全量运行。

### 前端既有失败

完整前端测试有两个既有失败，位于：

```text
src/api/__tests__/admin.system.rollback.spec.ts
```

测试仍期望 `post` 只有两个参数，现有实现已经传第三个 `{ timeout: 900000 }`。这不是异步生图回归；后续应更新测试期望并重新跑完整套件。

## 尚未完成

- 七牛、阿里、腾讯真实凭证的上传、HEAD/read、删除契约测试。
- 真实 `custom_s3` 兼容服务的升级回归。
- Gemini BB/SC 与 OpenAI generations/edits 的生产或准生产端到端调用。
- 真实余额、订阅、Key/账号额度与 1K/2K/4K/数量的逐笔核账。
- 桌面和移动视口的浏览器截图验收。
- 预签名 URL 实际到期、CDN URL、对象清理和 provider 切换演练。

本轮浏览器控制工具报错：

```text
codex/sandbox-state-meta: missing field sandboxPolicy
```

因此不能声称任务页面、分组表单和存储配置页已完成视觉截图验收。

## 上线前测试矩阵

| 维度 | 必测值 |
|---|---|
| 协议 | Gemini BB、Gemini SC、OpenAI generations、OpenAI edits JSON、OpenAI edits multipart |
| 图片来源 | 纯文本、HTTPS 参考图、data URI、SC 上传 |
| 规格 | 1K、2K、4K、允许/拒绝 0.5K、比例 auto 边界、多图 |
| 所有权 | 同 Key 成功、同用户其他 Key 404、其他用户 404、管理员可见 |
| 幂等 | 相同 key/相同请求复用、不同请求 409、队列重复投递 |
| 计费 | 余额、订阅、simple、倍率、额度、改价后重试、重复 Apply |
| 恢复 | 重启、stale queued、stale invoking、OSS 部分失败、账务超时、日志补写 |
| 安全 | SSRF、DNS rebinding、重定向内网、伪 MIME、超限、路径注入、错误脱敏 |
| UI | 用户/管理员列表详情、筛选、时间线、缩略图、移动无溢出 |

## 提交前最小复验

```bash
cd backend
go test ./... -run '^$'
cd ../frontend
pnpm typecheck
cd ..
git diff --check
```

若其中任一步失败，不要提交或推送；先判断是本次回归、环境问题还是上面已记录的既有问题，并在交接文档追加证据。
