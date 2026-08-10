# ANL API

`anlapi` is ANL API's self-hosted AI API gateway and usage-management platform. It unifies different upstream providers behind OpenAI-compatible endpoints and provides account, group, API key, usage, billing, and administration workflows for personal deployments, internal teams, and further customization.

[在线控制台](https://api.anlmc.top) | [中文说明](README_CN.md) | [部署文档](deploy/README.md)

![Go](https://img.shields.io/badge/Go-1.26.5-00ADD8?logo=go&logoColor=white)
![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15+-4169E1?logo=postgresql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-7+-DC382D?logo=redis&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)
![License](https://img.shields.io/badge/License-LGPL--3.0-blue)

> ANL API 是独立维护的项目名称和代码仓库。它基于 [Sub2API](https://github.com/Wei-Shaw/sub2api) 进行二次开发，并不代表上游项目或任何模型供应商的官方产品。

The current `1.0.11` release selectively tracks compatibility, security, and Codex stability fixes from Sub2API `v0.1.173` while retaining ANL-specific payment, image-generation, account-isolation, and user-level concurrency behavior.

## 项目定位

ANL API 面向需要统一接入 AI 能力的部署者：管理员在后台配置合法的渠道或账号、分组和计费规则，用户通过 API Key 调用允许的模型，并在控制台查看余额和用量记录。

项目仓库只包含源码、配置模板、迁移文件和部署示例，不包含生产数据库、OAuth 凭据、支付密钥、服务器密码或真实用户数据。

## 当前能力

### API 网关

- 提供 OpenAI 兼容的 `chat`、`responses`、`models`、`embeddings`、图像和流式请求入口。
- 支持不同上游类型的统一路由、失败切换和请求/响应处理。
- 支持 Codex 客户端相关请求；客户端的 `fast` 意图可按兼容路径透传给上游，由上游决定是否支持。
- 支持长耗时图像任务的异步提交与轮询（需要按 [异步图像任务文档](docs/ASYNC_IMAGE_TASKS.md) 配置对象存储）。

### DeepSeek V4

ANL API provides a dedicated DeepSeek setup path while keeping the client-facing API OpenAI-compatible:

- In the admin console, choose the **DeepSeek** shortcut when creating an API-key account. The official base URL (`https://api.deepseek.com`) and supported model candidates are prefilled; the upstream key remains server-side and is not exposed through the client API.
- The built-in V4 models are `deepseek-v4-flash` and `deepseek-v4-pro`. You can still use a public alias and map it to an upstream model through the normal account/group configuration.
- Clients can continue to call `POST /v1/chat/completions`. The gateway selects the upstream protocol by model capability: V4 Flash uses the official Responses route, while V4 Pro uses Chat Completions.
- Reasoning controls accept the standard `reasoning_effort` form and the supported nested provider-options form. Explicit client values are preserved and are subject to the group/API-key policy configured by the administrator.
- DeepSeek cache-hit usage, including `prompt_cache_hit_tokens`, is parsed into the normalized usage record and billed with the configured cache-read rate. This is usage and billing optimization, not response-content caching.

### OpenAI Realtime / Live

为 OpenAI 分组启用 Live 后，可通过 OpenAI 风格别名创建 WebRTC 会话。该别名复用现有 Live 请求格式：

```bash
curl -i https://your-domain.example/v1/realtime/sessions \
  -H "Authorization: Bearer $ANL_API_KEY" \
  -F 'sdp=<offer.sdp' \
  -F 'session={"model":"gpt-live"}'
```

响应体为 SDP answer，`Location` 响应头包含 `call_id`。使用同一 API Key 连接控制 WebSocket：

```bash
wscat -c 'wss://your-domain.example/v1/realtime?call_id=call_123' \
  -H "Authorization: Bearer $ANL_API_KEY"
```

原有 `POST /v1/live` 与 `GET /v1/live/:call_id` 路径继续可用。

### 账号、渠道与分组

- 管理员可以按账号类型、渠道和分组组织可用上游。
- 支持公开、私有、归属和共享等账号池调度边界，具体权限以后台配置和当前版本实现为准。
- 支持为不同用途配置模型分组，包括图像能力分组；可用模型、参数和上游限制仍以实际账号及供应商能力为准。
- 支持 OAuth 账号与普通 API Key/渠道的管理和隔离路径，凭据不会通过 README 或示例配置公开。

### 用户控制台

- 用户注册、登录、余额与充值流程。
- 创建和管理 API Key，并为 Key 配置允许的分组路由。
- 在控制台查看 Key 用量、请求记录和按时间汇总的消耗。
- 由服务端按用户账户执行请求并发控制；不会在 README 中虚构固定倍率、永久免费额度或上游可用性承诺。

### 管理后台

- 用户、账号、渠道、分组、API Key、订阅、支付和用量管理。
- 图像相关账号和分组的管理入口，以及请求审计、风险控制和系统设置。
- 支持按部署需要启用支付、邮件、对象存储、内容审查和 OAuth 等可选模块。

## ANL 定制界面

这些界面截图使用演示数据，用于说明 ANL API 在用户端和管理端的定制方向。截图已做脱敏处理，余额、请求量、Token、价格、模型统计、账号和图表数据均不代表生产环境，不包含真实凭据或用户数据。

### 用户端：账户与用量仪表盘

用户可以在一个页面查看账户状态、请求用量、Token 使用趋势、模型分布和平台消费概况，适合个人或团队快速了解调用情况。

<p align="center">
  <img src="assets/screenshots/anlapi-user-dashboard-demo.png" alt="anlapi 用户端账户与用量仪表盘脱敏演示截图" width="100%">
</p>

### 管理端：运营与用量仪表盘

管理员可以集中查看 API Key、账号、用户、Token、模型分布和请求趋势等运营指标，便于进行渠道、用量和系统运行管理。

<p align="center">
  <img src="assets/screenshots/anlapi-admin-dashboard-demo.png" alt="anlapi 管理端运营与用量仪表盘脱敏演示截图" width="100%">
</p>

## Quick start

After deployment, the usual setup flow is:

1. In the admin console, create an upstream account. For the official DeepSeek API, use the **DeepSeek** shortcut, enter the upstream API key, and keep the prefilled official base URL unless you intentionally use another compatible endpoint.
2. Create or select a group, then configure the public model names and the accounts/models that group may use.
3. Create a user API key and grant it access to the group. The user key is the only credential a client needs to call ANL API; upstream credentials stay in the server-side account configuration.
4. Call the OpenAI-compatible endpoint:

```bash
curl https://your-domain.example/v1/chat/completions \
  -H "Authorization: Bearer $ANL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [{"role": "user", "content": "Hello"}],
    "reasoning_effort": "high",
    "stream": false
  }'
```

The actual model list, routing permissions, billing rate, and upstream availability are controlled by the deployment administrator and the configured provider accounts.

## 技术栈

- 后端：Go 1.26.5、Gin、Ent、PostgreSQL、Redis
- 前端：Vue 3、TypeScript、Pinia、Vue Router、Tailwind CSS、Vite
- 测试：Go test、Vitest、`vue-tsc`、ESLint
- 部署：Docker Compose 或 Linux systemd，生产环境建议将 PostgreSQL 和 Redis 持久化到应用容器之外

## 仓库结构

```text
.
├── backend/              # Go 后端、迁移、服务、处理器和仓储层
├── frontend/             # Vue 3 管理端与用户端控制台
├── deploy/               # Docker、systemd 和配置模板
├── docs/                 # 集成、支付、图像任务和运维文档
├── assets/               # 项目静态资源
├── tools/                # 开发与安全检查工具
├── Makefile              # 构建和测试入口
└── Dockerfile            # 应用镜像构建文件
```

## 环境要求

- Go 1.26.5
- Node.js 20 或更高版本
- pnpm 9 或更高版本
- PostgreSQL 15 或更高版本
- Redis 7 或更高版本
- Docker 与 Docker Compose（推荐用于部署）

## 快速部署

生产或长期运行环境建议先阅读完整的 [部署文档](deploy/README.md)。一个基于本地目录持久化的 Docker Compose 示例：

```bash
git clone https://github.com/ANL-694/anlapi.git
cd anlapi/deploy
cp .env.example .env
# 编辑 .env，至少设置数据库密码和固定的安全密钥
chmod 600 .env
docker compose -f docker-compose.local.yml up -d
docker compose -f docker-compose.local.yml logs -f anlapi
```

首次部署时，应用会根据环境变量初始化数据库和管理员账号。正式对外提供服务前，请配置反向代理、TLS、可信代理地址、数据库备份和日志策略。

源码开发方式：

```bash
pnpm --dir frontend install
pnpm --dir frontend run dev

cd backend
go run ./cmd/server
```

详细配置以 [`deploy/config.example.yaml`](deploy/config.example.yaml)、[`deploy/.env.example`](deploy/.env.example) 和 `deploy/README.md` 为准。不要把生产配置复制到仓库，也不要把真实凭据填入示例文件。

## 常用检查

在仓库根目录执行：

```bash
make build
make test
```

也可以分别执行：

```bash
cd backend
go test ./...

cd ../frontend
pnpm run test:run
pnpm run typecheck
pnpm run i18n:audit:strict
```

发布前建议额外运行仓库提供的安全扫描，并检查 Git 暂存区中没有本地配置、数据库导出、日志或凭据文件。

## 安全与合规

- 只接入你有权使用的账号、渠道和供应商接口，并遵守相关服务条款。
- 不要提交 API Key、OAuth token、支付密钥、数据库密码、JWT 密钥或服务器凭据。
- 生产环境使用强管理员密码，限制后台访问，并为 PostgreSQL、Redis 和对象存储建立独立备份策略。
- `/api/*`、`/v1/*`、流式接口和网关请求不应被 CDN 缓存；反向代理应正确转发 WebSocket 和长连接。
- 模型价格、可用性、额度、响应时间和图像参数取决于管理员配置及实际上游服务，仓库不对第三方服务作稳定性或额度保证。
- 使用者应自行确认所在国家或地区的法律法规、数据处理要求和上游服务协议。

## 相关文档

- [部署与运维](deploy/README.md)
- [异步图像任务](docs/ASYNC_IMAGE_TASKS.md)
- [支付接入](docs/PAYMENT.md)
- [管理员支付接口](docs/ADMIN_PAYMENT_INTEGRATION_API.md)
- [官方更新与合并流程](docs/OFFICIAL_UPDATE_AND_DEPLOY_CN.md)
- [中文说明](README_CN.md)

## 许可证与上游

本项目遵循仓库中的 [LGPL-3.0 许可证](LICENSE)。ANL API 基于以下项目进行二次开发：

- [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api)
- [PIXEL-API/PixelAPI](https://github.com/PIXEL-API/PixelAPI)

请同时阅读各上游项目的许可证、贡献协议和第三方依赖许可。
