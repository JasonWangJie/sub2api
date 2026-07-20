# 下游 API 契约与兼容边界

完整请求/响应示例见 [../docs/DURABLE_ASYNC_IMAGE_API.md](../docs/DURABLE_ASYNC_IMAGE_API.md)。本页只记录开发时不能改变的契约。

## 固定接口

| 方言 | 平台/用途 | 方法与路径 | 提交状态 |
|---|---|---|---|
| BB | Gemini 文生图/图生图 | `POST /v1/chat/completions_gm` | HTTP `202` |
| BB | OpenAI 文生图 | `POST /v1/images/generations_oa` | HTTP `202` |
| BB | OpenAI 图生图 | `POST /v1/images/edits_oa` | HTTP `202` |
| BB | 查询 | `GET /v1/images/tasks_async/{task_id}` | HTTP `200` |
| SC | 上传 Gemini 参考图 | `POST /v1/uploads/images_sc` | HTTP `200` |
| SC | Gemini 文生图/图生图 | `POST /v1/images/generations_sc` | HTTP `200`，响应体 `code: 200` |
| SC | 查询 | `GET /v1/tasks_sc/{task_id}` | HTTP `200` |

接口路径固定，不做配置化。只通过 `async_image.public_base_url` 配置提交响应里的绝对 `query_url` 和 `Location`。

## 开关与路由

新任务必须同时满足：

1. API Key 有效且已绑定分组。
2. 分组平台与路径匹配。
3. `allow_image_generation=true`。
4. `allow_async_image_generation=true`。
5. 图片存储配置可用。
6. 现有用户、订阅/余额、Key/账号额度、并发和资格检查通过。

新开关默认 `false`。关闭普通生图或切换到非 Gemini/OpenAI 平台时，表单和服务端都要把它关闭。新接口未授权时返回 `403`，不回退同步。

以下旧接口不得受新开关影响：

```text
POST /v1/chat/completions
POST /v1/images/generations
POST /v1/images/edits
POST /v1/images/generations/async
POST /v1/images/edits/async
GET  /v1/images/tasks/{task_id}
```

## BB 与 SC 不能合并

BB、SC 的字段、HTTP 状态和任务状态词不同。它们共享内部任务模型，但查询 Handler 必须分别渲染，不能返回一个包含双方字段的“兼容超集”。

- BB 提交返回 `id/task_id/object/status/query_url` 或 OpenAI 图片方言的 `task_id/query_url`。
- SC 提交保持文档定义的 `code/data` 包装。
- BB 查询使用 `queued/processing/succeeded/failed` 语义。
- SC 查询使用 `pending/processing/completed` 语义。
- 只有最终成功时才返回 OSS 图片地址。

## 所有权与幂等

- 公共查询必须携带创建任务的同一 API Key。
- 同一用户的另一个 Key 也返回 `404`；登录后的用户任务中心才按 user ID 聚合本人所有 Key。
- 管理员接口通过站内管理员鉴权访问全站任务。
- `Idempotency-Key` 作用域是同一 API Key。相同 key 和相同请求哈希返回原任务；相同 key 配不同请求返回 `409`。
- 请求哈希包含协议、平台、路径和请求内容。客户端重试应原样重发 JSON，避免字段顺序或空白差异形成冲突。

## Gemini 映射

- BB `extra_body.google.image_config.image_size/aspect_ratio` 映射到 `generationConfig.imageConfig`。
- Worker 私有能力强制非流式，并加入 `responseModalities: ["TEXT", "IMAGE"]`。
- BB `messages[].content[].image_url` 和 SC `image_urls[]` 转成 Gemini `inlineData`。
- 从 `candidates[].content.parts[].inlineData` 提取全部图片，不只取第一张。
- 默认允许 `1K/2K/4K`。`0.5K` 只有模型命中 `async_image.gemini_half_k_models` 时透传，否则返回参数错误。
- `aspect_ratio=auto/自动` 只对有参考图的图生图有效；文生图不得静默接受。

图片捕获能力由 Worker 的私有 context 值开启。不要改回“看到请求 JSON 里有 image_config 就开启”，否则旧 `/v1/chat/completions` 会被下游绕过兼容边界。

## OpenAI 映射

- `_oa` 路径复用现有 `ParseOpenAIImagesRequest` 与原有 generations/edits 执行逻辑。
- generations 使用 OpenAI 标准文生图请求。
- edits 同时兼容文档的 `images[].image_url` JSON 和现有 multipart。
- 尺寸、数量、质量等原生参数不做近似转换。
- SC 首版只允许 Gemini；不要把 SC 的 `4K/16:9` 映射成 OpenAI 近似尺寸，否则会导致行为和计费档位失真。

## 参考图安全边界

远程参考图只接受 HTTPS 或受限 data URI，并限制 MIME、单图字节、像素、总超时和重定向次数。初始 DNS 和每次重定向都必须拒绝内网、回环、链路本地和保留地址。任何重构都必须保留 DNS rebinding 和重定向到内网的防护。
