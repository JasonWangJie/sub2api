# Sub2API

**AI API 网关平台 — 订阅配额分发与统一接入**

[官方 README](README.md) · [中文](README_CN.md) · [日本語](README_JA.md) · [开发指南](DEV_GUIDE.md) · [**Wiki 文档**](wiki/Home.md)

---

## 这是什么

Sub2API 把上游 AI 订阅账号（Claude / OpenAI / Gemini / Grok / Antigravity 等）统一成可分发的 API 网关。

管理员在后台接入上游账号、配置分组与调度策略；终端用户拿到平台下发的 API Key，用兼容官方协议的方式调用模型。平台负责：

- 鉴权与会话
- 账号调度与粘性会话
- Token 级用量统计与计费
- 并发 / 速率限制
- 请求转发与故障切换
- 管理后台与自助充值

典型用途：团队内部分发订阅配额、自建中转站、对接 Claude Code / Codex / Gemini CLI 等工具。

---

## ⚠️ 使用前须知

- 使用本项目可能违反 Anthropic 等上游服务商的服务条款，风险由使用者自行承担。
- 请在符合当地法律法规的前提下使用；禁止用于违法用途。
- 项目仅供技术学习与研究；作者不对封号、中断、数据丢失等损失负责。
- 开发者未授权任何商业运营；基于本项目的商业行为与官方无关。

许可证：[LGPL-3.0](LICENSE)（或更高版本）

---

## 核心能力

| 能力 | 说明 |
|------|------|
| 多账号管理 | OAuth / API Key 等多种上游账号接入 |
| API Key 分发 | 为用户生成、管理 Key，绑定分组与权限 |
| 精确计费 | Token 级用量追踪、成本计算、余额/订阅 |
| 智能调度 | 按分组/平台选账号，支持粘性会话与故障切换 |
| 限流与并发 | 用户级、账号级并发与 RPM/Token 限制 |
| 内置支付 | EasyPay、支付宝、微信、Stripe 等自助充值 |
| 管理后台 | 监控、运维看板、风控、公告、数据备份等 |
| 协议兼容 | Anthropic Messages、OpenAI Chat/Responses、Gemini v1beta 等 |

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go（当前 `go.mod` 为 1.26.x），Gin，Ent ORM，Google Wire |
| 前端 | Vue 3.4+，Vite，Pinia，Vue Router，TailwindCSS，pnpm |
| 数据 | PostgreSQL 15+ |
| 缓存/队列 | Redis 7+ |
| 部署 | Docker Compose / 二进制 + systemd / Apple container |

---

## 系统架构（概念）

```
客户端 / CLI / IDE 插件
        │  API Key
        ▼
┌───────────────────────────────────────┐
│              Sub2API Gateway          │
│  /v1/*  /v1beta/*  /antigravity/*     │
│  鉴权 → 分组 → 选账号 → 转发 → 计费   │
└───────────────────────────────────────┘
        │                    │
        ▼                    ▼
   PostgreSQL              Redis
   (用户/账号/用量)     (限流/会话/队列)
        │
        ▼
┌───────────────────────────────────────┐
│           管理后台 (Vue SPA)          │
│  嵌入同一二进制（-tags embed）或分离  │
└───────────────────────────────────────┘
```

请求主路径：

1. 客户端携带 `sk-...` 访问网关路径  
2. 中间件完成 API Key 鉴权、分组校验、限流  
3. 按分组平台选择上游账号并转发  
4. 流式/非流式回写客户端，异步落库用量并扣费  

---

## 仓库结构

```
sub2api/
├── backend/                      # Go 后端
│   ├── cmd/server/               # 主进程入口
│   ├── cmd/jwtgen/               # JWT 工具
│   ├── ent/schema/               # Ent 数据模型定义
│   ├── migrations/               # 数据库迁移
│   ├── resources/                # 模型定价等静态资源
│   └── internal/
│       ├── config/               # 配置加载
│       ├── handler/              # HTTP 处理器（网关/认证/支付等）
│       ├── service/              # 业务逻辑（调度、计费、账号…）
│       ├── repository/           # 数据访问
│       ├── server/               # 路由、中间件、启动装配
│       │   └── routes/           # auth / gateway / admin / payment / user
│       ├── payment/              # 支付通道实现
│       ├── setup/                # 首次安装向导
│       ├── middleware/           # 鉴权、限流等
│       └── web/                  # 前端静态资源嵌入
│
├── frontend/                     # Vue 3 管理端 / 用户端
│   └── src/
│       ├── api/                  # 后端 API 封装
│       ├── views/admin/          # 管理页面（账号、分组、运维…）
│       ├── views/user/           # 用户页面（Key、用量、充值…）
│       ├── views/auth/           # 登录注册 / OAuth 回调
│       ├── stores/               # Pinia 状态
│       ├── router/               # 路由
│       └── components/           # 通用组件
│
├── deploy/                       # 部署脚本与 Compose 配置
├── docs/                         # 支付、异步图片等专题文档
├── skills/                       # Agent Skill（管理端 CLI）
├── tools/                        # 辅助工具
├── assets/                       # 文档图片等
├── Dockerfile                    # 镜像构建
└── DEV_GUIDE.md                  # 本地开发注意事项
```

### 后端分层

| 目录 | 职责 |
|------|------|
| `handler` | 解析请求、协议适配、调用 service |
| `service` | 核心业务：网关转发、账号调度、计费、用户配额 |
| `repository` + `ent` | 持久化 |
| `server/routes` | 路由注册与中间件编排 |
| `payment` | 充值订单与支付回调 |

### 前端分区

| 区域 | 内容 |
|------|------|
| `views/admin` | 仪表盘、账号/分组/渠道、用量、兑换码、支付方案、运维 Ops、风控、设置 |
| `views/user` | API Key、用量、订阅、充值、推广、批量图片等 |
| `views/auth` | 登录注册、找回密码、OAuth（GitHub/Google/微信/钉钉/OIDC/LinuxDo） |

---

## 网关协议一览

平台按 **分组（Group）绑定的平台类型** 自动路由。常用入口：

| 路径 | 用途 |
|------|------|
| `POST /v1/messages` | Anthropic Messages（Claude Code 常用） |
| `POST /v1/chat/completions` | OpenAI Chat Completions |
| `POST /v1/responses` | OpenAI Responses（Codex 等） |
| `GET /v1/models` | 模型列表（Codex 可带 `client_version`） |
| `POST /v1/embeddings` | Embeddings（OpenAI 分组） |
| `POST /v1/images/*` | 图像生成/编辑；含异步任务与批量接口 |
| `POST /v1/videos/*` | 视频相关（Grok 分组） |
| `GET|POST /v1beta/models...` | Gemini 原生兼容 |
| `/antigravity/v1/messages` | Antigravity Claude |
| `/antigravity/v1beta/` | Antigravity Gemini |
| `/backend-api/codex/*` | Codex 直连别名 |
| `GET /v1/sub2api/billing` | Key 侧计费信息 |

管理与业务 API 前缀：`/api/v1/...`（认证、用户 Key、用量、管理端、支付等）。

### Antigravity 示例

```bash
export ANTHROPIC_BASE_URL="http://localhost:8080/antigravity"
export ANTHROPIC_AUTH_TOKEN="sk-xxx"
```

开启混合调度后，通用 `/v1/messages`、`/v1beta/` 也可调度 Antigravity 账号。  
注意：Anthropic Claude 与 Antigravity Claude **不要在同一上下文混用**，请用分组隔离。

### Sora 状态

当前 Sora 相关能力因上游/媒体链路问题 **暂不可用**，生产环境请勿依赖。相关配置项仅为预留。

---

## 数据模型（核心实体）

Ent schema 中的主要概念：

| 实体 | 含义 |
|------|------|
| `User` | 平台用户（余额、并发、角色） |
| `APIKey` | 用户对外调用凭证 |
| `Account` | 上游订阅/API 账号 |
| `Group` | 调度分组（平台、模型范围、倍率） |
| `UsageLog` | Token 用量与费用记录 |
| `RedeemCode` / `PromoCode` | 兑换码、优惠码 |
| `SubscriptionPlan` / `UserSubscription` | 订阅套餐 |
| `PaymentOrder` | 支付订单 |
| `Proxy` | 出站代理 |
| `TLSFingerprintProfile` | TLS 指纹配置 |
| `ChannelMonitor` | 渠道可用性监控 |

---

## 运行模式

| 模式 | 说明 |
|------|------|
| `standard`（默认） | 完整 SaaS：计费、余额、订阅等 |
| `simple` | 隐藏 SaaS 能力、跳过计费；适合个人/内网。生产需同时设 `SIMPLE_MODE_CONFIRM=true` |

环境变量：`RUN_MODE=simple`

---

## 快速部署

### 1. Docker Compose（推荐）

```bash
mkdir -p sub2api-deploy && cd sub2api-deploy
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/docker-deploy.sh | bash
docker compose up -d
docker compose logs -f sub2api
```

浏览器访问：`http://<服务器IP>:8080`

| Compose 文件 | 数据存放 | 适用 |
|--------------|----------|------|
| `docker-compose.local.yml` | 本地目录 | 生产、易备份迁移（推荐） |
| `docker-compose.yml` | Docker 命名卷 | 快速试用 |

详情见 [deploy/README.md](deploy/README.md)、[deploy/DOCKER.md](deploy/DOCKER.md)。

### 2. 二进制一键安装（Linux）

前置：PostgreSQL 15+、Redis 7+、root。

```bash
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/install.sh | sudo bash
sudo systemctl start sub2api
sudo systemctl enable sub2api
```

首次打开 `http://<IP>:8080` 走设置向导（库、Redis、管理员）。

### 3. 源码编译

```bash
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api

# 前端
cd frontend && pnpm install && pnpm run build
# 产物输出到 backend/internal/web/dist/

# 后端（嵌入前端）
cd ../backend
VERSION="$(./scripts/resolve-version.sh)"
go build -tags embed -ldflags="-X main.Version=${VERSION}" -o sub2api ./cmd/server

# 建议不要先手写 config.yaml，直接运行以触发 setup 向导
./sub2api
```

> `-tags embed` 才会把前端打进二进制。  
> 管理员只能通过 setup 向导创建；预置 `config.yaml` 会跳过向导导致无法登录。

配置模板：`deploy/config.example.yaml`

### 4. Apple container（macOS 本地）

见 [deploy/APPLE_CONTAINER.md](deploy/APPLE_CONTAINER.md)。

---

## 本地开发

```bash
# 后端
cd backend
go run ./cmd/server

# 前端（另开终端）
cd frontend
pnpm install
pnpm run dev
```

修改 `backend/ent/schema` 后：

```bash
cd backend
go generate ./ent
go generate ./cmd/server
```

常用测试：

```bash
cd backend && go test -tags=unit ./...
cd frontend && pnpm test:run
```

前端包管理请使用 **pnpm**（不要用 npm，以免 lock / node_modules 冲突）。更多坑点见 [DEV_GUIDE.md](DEV_GUIDE.md)。

---

## Nginx / 反向代理注意

若经 Nginx 反代并配合 Codex CLI，需在 `http` 块开启：

```nginx
underscores_in_headers on;
```

否则含下划线的头（如 `session_id`）会被丢弃，粘性会话失效。

明文端口默认支持 **h2c**，并保留 HTTP/1.1（WebSocket / 旧客户端）。Caddy 示例见官方 README。

---

## 文档索引

| 文档 | 内容 |
|------|------|
| [docs/PAYMENT_CN.md](docs/PAYMENT_CN.md) | 支付配置（中文） |
| [docs/PAYMENT.md](docs/PAYMENT.md) | Payment setup (EN) |
| [docs/ASYNC_IMAGE_TASKS.md](docs/ASYNC_IMAGE_TASKS.md) | 异步图片任务 |
| [docs/BATCH_IMAGE_MVP.md](docs/BATCH_IMAGE_MVP.md) | 批量图片 |
| [deploy/DATAMANAGEMENTD_CN.md](deploy/DATAMANAGEMENTD_CN.md) | 数据管理守护进程 |
| [skills/sub2api-admin](skills/sub2api-admin/SKILL.md) | 管理端 CLI Skill |

生态：移动端控制台 [sub2api-mobile](https://github.com/ckken/sub2api-mobile)

---

## 安全建议（摘要）

- 生产环境使用强随机 `JWT_SECRET`、`TOTP_ENCRYPTION_KEY`、数据库密码  
- 收紧 `security.url_allowlist`，生产避免明文 HTTP 上游  
- 配置 `cors.allowed_origins`、`server.trusted_proxies`  
- 建议 CDN/WAF + 服务端限流双层防护  
- 关注 `gateway.upstream_response_read_max_bytes` 等响应大小上限  

---

## 贡献与反馈

上游仓库：[Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api)

Issue / PR 欢迎；提交前请跑通本地单测与 lint。前端变更请同步 `pnpm-lock.yaml`。

---

## Fork 二次开发与上游同步

本仓库是从 `Wei-Shaw/sub2api` Fork 后进行二次开发的版本。当前 Git 远程约定如下：

| 远程名 | 仓库 | 用途 |
|--------|------|------|
| `origin` | `JasonWangJie/sub2api` | 自己的 Fork，提交完成后推送到这里 |
| `upstream` | `Wei-Shaw/sub2api` | 原作者仓库，只用于获取和合并官方更新 |

可以随时用下面的命令确认配置：

```bash
git remote -v
git branch -vv
git status
```

### 日常修改并推送到自己的 Fork

开始修改前，建议先同步最新代码，再创建功能分支。分支名可按实际功能调整：

```bash
git switch main
git fetch upstream
git merge upstream/main
git push origin main

git switch -c feat/image-plaza
```

完成修改后检查、测试、提交并推送：

```bash
git status
git diff

# 按实际改动选择文件；提交前确认不要包含密钥、配置和临时文件
git add <文件或目录>
git commit -m "新增图片广场功能"
git push -u origin feat/image-plaza
```

功能分支确认无误后，可以在 GitHub 上向自己 Fork 的 `main` 发起 Pull Request；也可以在本地合并：

```bash
git switch main
git merge --no-ff feat/image-plaza
git push origin main
```

本项目的提交信息统一使用**简体中文**。

### 原作者更新后，同步到本地和自己的 Fork

同步前必须先处理工作区中的未提交修改。推荐直接提交：

```bash
git status
git add <文件或目录>
git commit -m "保存当前开发进度"
```

如果修改还不适合提交，可以临时储藏，包括未跟踪文件：

```bash
git stash push -u -m "同步上游前临时保存"
```

然后更新本地 `main`，合并原作者的 `main`，并推送到自己的 Fork：

```bash
git switch main
git fetch upstream --prune
git merge upstream/main

# 按项目实际情况执行测试
cd backend
go test -tags=unit ./...
cd ../frontend
pnpm test:run
cd ..

git push origin main
```

如果之前使用了 `stash`，在上游合并完成后恢复修改：

```bash
git stash pop
```

恢复时也可能产生冲突，需要按下一节的方法处理。确认修改完整后，再正常 `add`、`commit` 和 `push`。

### 合并冲突的处理方法

执行 `git merge upstream/main` 后如果出现冲突：

```bash
git status
```

打开 `both modified` 标记的文件，找到以下冲突标记，结合本地定制和上游新逻辑决定最终内容，并删除所有标记：

```text
冲突开始：<<<<<<< HEAD
本地代码
=======
上游代码
冲突结束：>>>>>>> upstream/main
```

解决并测试后完成合并：

```bash
git add <已解决的文件>
git commit
git push origin main
```

`git commit` 打开编辑器时，保留或改成简体中文合并说明即可。如果冲突处理有误、尚未提交，并且想放弃本次上游合并：

```bash
git merge --abort
```

不要使用 `git reset --hard` 处理普通同步问题，否则容易丢失本地修改。

### 在功能分支开发期间获取上游更新

先让本地 `main` 跟上 upstream，再把它合并进当前功能分支：

```bash
git switch main
git fetch upstream --prune
git merge upstream/main
git push origin main

git switch feat/image-plaza
git merge main
```

解决冲突并测试后，提交合并结果，再推送功能分支：

```bash
git push origin feat/image-plaza
```

### 推荐的固定同步命令

当工作区干净、当前开发内容已经提交后，日常同步通常只需要：

```bash
git switch main
git fetch upstream --prune
git merge upstream/main
git push origin main
```

这里刻意使用 `merge` 而不是对已共享分支执行 `rebase`：`merge` 会保留完整分叉与合并记录，也不需要强制推送，更适合自己的 Fork 已经存在长期定制提交的情况。不要使用 `git push --force` 覆盖 `main`。

---

## 二次开发提交日志

### 2026-07-20：新增图片工作台、图片广场及界面定制

- 提交主题：`新增图片工作台、图片广场及界面定制`
- 上游基线：`fa402b909`（项目版本 `0.1.161`，位于 `v0.1.161` 标签之后）
- 后端：新增图片广场数据表、Repository、Service、Handler、用户路由及 Wire 依赖注入。
- 前端：新增图片工作台、图片广场、API 封装、本地画廊状态管理、路由、侧边栏入口和中英文文案。
- 界面：调整首页、登录页、认证布局、站点字体、Logo 和相关视觉样式。
- 文档：补充项目 Wiki、Fork 长期维护流程和上游同步说明。
- 工具：新增本地 MiniRedis 和管理员初始化工具；管理员工具仅通过环境变量读取连接信息与凭据。
- 同步记录：合并上游时保留新的 `ProvideBatchImageHandler` 注入方式，并将定制界面的默认 Logo 更新为 `/logo.svg`。

---

<div align="center">

若本项目对你有帮助，欢迎 Star 支持。

</div>
