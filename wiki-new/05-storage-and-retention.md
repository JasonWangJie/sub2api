# 对象存储、参考图与保留策略

## 存储模型

全站只有一个当前图片对象存储配置，支持：

| provider | 厂商/用途 |
|---|---|
| `qiniu` | 七牛云 Kodo 的 S3 兼容接口 |
| `aliyun` | 阿里云 OSS 的 S3 兼容接口 |
| `tencent` | 腾讯云 COS 的 S3 兼容接口 |
| `custom_s3` | 兼容既有 AWS S3、R2、MinIO 和其他 S3 服务 |

厂商模式提供 endpoint 预设，也允许显式高级覆盖；七牛/阿里/腾讯必须填写有效 region。完整模板见 `deploy/config.example.yaml` 的 `image_storage`。

保存结果返回稳定元数据：

```text
provider / bucket / object_key / content_type / bytes / checksum / width / height
```

数据库不保存会过期的预签名 URL。查询时若配置 `image_storage.public_base_url`，生成公开 CDN 地址；否则按 `async_image.signed_url_expiry_seconds` 动态签名。SC 的 `expires_at` 必须对应本次实际签名到期时间。

## 配置和连接探测

- 后台保存的数据库设置优先于 config/env，并可用于即时更新运行参数。
- 启用存储配置前必须真实执行小对象上传、HEAD/读取验证和删除。
- 任一步失败都不能把配置保存为可用状态。
- 后台入口位于“备份 / 异步生图存储”，API 为 `/api/v1/admin/backups/image-storage`，测试接口为 `/api/v1/admin/backups/image-storage/test`。
- 生产凭证优先通过 `IMAGE_STORAGE_ACCESS_KEY_ID`、`IMAGE_STORAGE_SECRET_ACCESS_KEY` 或受控密钥系统注入，不要提交到 Git。

## 重要的历史对象风险

当前对象解析器会校验记录中的 provider/bucket 是否与全站当前配置一致。直接把当前配置从一个 provider 或 bucket 切到另一个后，旧任务对象可能无法签名、查看或自动清理。

生产环境切换前必须选择一种方案：

1. 保持旧 provider/bucket 配置，直到旧对象全部超过保留期并清理。
2. 先迁移对象并原子更新数据库引用。
3. 后续实现按历史 provider/bucket 查找旧凭证的 resolver。

不要在没有迁移方案时直接覆盖生产存储配置。

## 参考图安全

BB 远程参考图与 SC 上传都经过限制：

- 只允许 HTTPS 或受限 data URI。
- 初始 DNS、实际 dial IP 和每次重定向重新检查地址。
- 拒绝内网、回环、链路本地和保留网段。
- 校验声明 MIME、响应 MIME 和实际解码格式。
- 默认单图上限 32 MiB、总超时 30 秒、最多 3 次重定向。
- 当前解码像素上限为代码默认 8000 万像素，尚未开放运行时配置。
- SC 上传对象绑定 API Key 所有权，外部 URL 只保存 SHA-256 哈希用于匹配。

活动任务通过 `async_image_task_inputs` 引用输入对象，清理器不会提前删除仍被活动任务使用的参考图。

## 对象前缀

`image_storage.prefix` 是根前缀，参考图、生成结果和连接探测对象在不同子前缀下保存。对象 key 由服务端生成，不能拼接下游提供的原始文件路径；文件名只作为脱敏元数据。

## 默认保留期

| 数据 | 默认值 | 配置 |
|---|---:|---|
| SC 参考图 | 24 小时 | `async_image.input_retention_hours` |
| 上游成功 staging | 最长 24 小时 | 代码恢复安全窗 |
| 任务记录 | 90 天 | `async_image.task_retention_days` |
| 生成结果对象 | 90 天 | `async_image.result_retention_days` |
| 动态签名 | 3600 秒 | `async_image.signed_url_expiry_seconds` |

清理循环当前约每 15 分钟运行、单批 100 条、claim 租约 30 分钟。流程先删除 OSS 对象，再删除结果或输入数据库行，最后删除满足条件的终态任务。删除失败会释放 claim 供后续重试。

## 容量注意事项

上游成功图片会短期以 `BYTEA` 存在 PostgreSQL staging 中，以保证进程重启不重新生成。大图并发会增加数据库容量、WAL、备份和复制压力。生产环境应监控：

- `async_image_staging_objects` 总字节和最老记录年龄。
- PostgreSQL 磁盘、WAL 和副本延迟。
- OSS 上传失败率和延迟。
- `storage_failed` 数量与清理积压。
- 输入、结果和任务的过期清理延迟。
