# 部署与配置

## 上线前置条件

- PostgreSQL 和 Redis 可用，并已备份生产数据库。
- 应用加密密钥在换机/扩容后保持一致，否则排队任务的加密请求无法解密。
- 至少一个 Gemini 或 OpenAI 分组具有可用账号和正确的现有图片计费配置。
- 七牛、阿里、腾讯或 `custom_s3` 的桶、region、凭证和网络权限完整。
- 对外 API Base URL、反向代理和 HTTPS 已确定。

## 数据库迁移

迁移文件：

```text
backend/migrations/185_async_image_tasks.sql
```

它新增 Group 开关和 7 张异步生图表。正常 Sub2API 启动流程会执行待处理迁移；部署后仍应在数据库确认迁移已应用，并确认所有新表、索引和 check constraint 存在。

升级前务必备份。迁移只新增字段和表，不删除旧 Redis 任务或旧接口数据。不要为了回滚代码直接删除新表；先停止新提交和 Worker，确认没有待结算任务，再制定数据保留方案。

## 首次配置顺序

1. 配置 `image_storage` 的 provider、endpoint/region、bucket、凭证、prefix 和公开 CDN 地址。
2. 在后台执行真实连接测试；确认 upload、HEAD/read、delete 全部成功。
3. 配置 `async_image.public_base_url`、并发、超时、重试、下载限制、签名和保留期。
4. 重启服务，使 `worker_concurrency` 按新值启动。
5. 在目标 Gemini/OpenAI 分组先开启普通图片生成，再开启异步生图。
6. 使用测试 API Key 分别跑提交、轮询、OSS 查看和费用核对。
7. 验证用户任务中心与管理员任务中心。

存储未启用或凭证不完整时，新任务必须拒绝，不能接受后再永久卡在上传阶段。

## 默认运行参数

| 配置 | 默认值 | 说明 |
|---|---:|---|
| `public_base_url` | 空 | 空时返回相对查询路径 |
| `worker_concurrency` | `4` | 单实例 Worker 数；修改后重启 |
| `worker_lease_seconds` | `120` | Redis/PG 租约失效阈值 |
| `recovery_interval_seconds` | `30` | 恢复扫描间隔 |
| `execution_timeout_seconds` | `900` | 单次上游调用超时 |
| `storage_retry_attempts` | `5` | OSS 后处理最大尝试次数 |
| `billing_retry_attempts` | `10` | 固定账单最大尝试次数 |
| `retry_backoff_seconds` | `30` | 后处理重试间隔 |
| `download_max_bytes` | `33554432` | 单张参考图 32 MiB |
| `download_timeout_seconds` | `30` | HTTPS 下载总超时 |
| `download_max_redirects` | `3` | 每跳重新做 SSRF 检查 |
| `signed_url_expiry_seconds` | `3600` | 动态结果链接有效期 |
| `input_retention_hours` | `24` | SC 输入保留期 |
| `task_retention_days` | `90` | 任务记录保留期 |
| `result_retention_days` | `90` | OSS 结果保留期 |
| `gemini_half_k_models` | `[]` | 精确名称或末尾 `*` 前缀白名单 |
| `prompt_preview_enabled` | `true` | 保存脱敏提示摘要 |
| `prompt_preview_max_chars` | `160` | 摘要最大字符数 |

完整注释见 `deploy/config.example.yaml`。后台数据库设置优先于文件和环境变量。Worker 数在进程启动时确定；修改并发后必须重启。其他运行参数会在任务执行或循环中重新加载。

## 构建

前端：

```bash
cd frontend
pnpm install
pnpm build
```

后端嵌入前端：

```bash
cd ../backend
VERSION="$(./scripts/resolve-version.sh)"
go build -tags embed -ldflags="-X main.Version=${VERSION}" -o sub2api ./cmd/server
```

若部署机没有本机 Go，使用项目 Docker 构建环境执行同等构建和测试。不要以缺少本机命令为理由省略后端验证。

只有修改 Wire provider 或构造函数时才重新生成：

```bash
cd backend
go run github.com/google/wire/cmd/wire@v0.7.0 ./cmd/server
```

## 多实例

`worker_concurrency` 是每个实例的并发，总并发等于所有实例之和。上线前按上游账号容量、数据库连接数和 OSS 吞吐计算总量。数据库 CAS 和 Redis 租约负责互斥，但不会替管理员限制集群总 Worker 数。

## 上线验收最小闭环

至少完成四条生成链：

```text
BB Gemini 文生图/图生图
SC Gemini 文生图/图生图和上传参考图
BB OpenAI generations
BB OpenAI edits（JSON URL 和 multipart）
```

每条都检查：提交状态、同 Key 查询、跨 Key 404、最终 OSS 图片、真实尺寸、图片数量、UsageLog、余额/订阅扣减、任务实际费用和幂等重试。

## 回滚注意

- 先关闭所有分组的异步生图开关，阻止新任务。
- 等待 `queued/invoking/uploading/billing_pending` 清空或人工处理。
- 不要删除 staging 或固定账单，否则可能造成无法完成结算。
- 旧同步和旧 Redis 异步路径独立，可继续服务。
- 若必须回退二进制，保留新表和对象，待恢复兼容版本后继续处理。
- 切换 OSS 配置前阅读 [05-storage-and-retention.md](05-storage-and-retention.md) 的历史对象风险。
