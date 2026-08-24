# AI Workbench

企业内部个人 AI 工作台，包含 Go 后端和 Vue 3 独立前端，并原生集成到 `admin-ui`。

## 常用能力

- 管理员统一维护 OpenAI Compatible 模型连接，支持连接测试、启停与默认模型配置
- 模型 API Key 仅在后端使用 AES-GCM 加密保存，接口只返回是否已配置
- 多轮对话、系统提示词、模型切换、快速/中等/高三档推理、停止生成、自动命名、搜索、置顶、重命名和删除
- 图片及文件附件随单次消息发送，模型调用读取后即从服务器删除原文件
- Markdown 回答预览、回答复制、响应延迟及 Token 用量展示
- 个人提示词库，支持分类、搜索、收藏、使用次数及一键带入对话
- 前沿项目发现，按 AI 项目、Skill 与插件检索 GitHub，并综合社区规模、活跃度与成熟度排序
- 普通用户查看个人用量，管理员查看全部用户汇总用量
- 内部账号、People OAuth 与 Permission 三种身份入口；对话和提示词按身份 ID 隔离
- Android 15/16 原生客户端

## 登录与集成

独立工作台支持内部账号和 People OAuth 2.0 授权码模式登录。浏览器只把授权码交给 AI Workbench 后端，OAuth Client Secret 不进入前端。

SQLite 首次初始化会创建内部管理员 `admin`，初始密码为 `admin123!`。管理员可在“用户管理”中添加、停用、删除普通用户或重设密码；新增用户未指定密码时默认使用 `用户名@123`。生产部署后应立即修改管理员密码。普通用户看不到用户管理和模型连接页面，也不能调用对应管理接口。

`admin-ui` 不会再次跳转 People 登录，而是把已有 Permission Access Token 交给 AI Workbench 后端验证。People 与 Permission 身份使用用户名作为数据主键，内部账号使用带来源前缀的独立主键，避免不同身份来源之间的数据串用。

首次启用前需要重启一次 People 后端，使其幂等预置下面的 OAuth 客户端：

```text
Client ID: ai-workbench-ui
Client Secret: ai-workbench-local-client-secret-change-me
Redirect URI: http://localhost:5181/oauth/callback
```

生产环境必须同时替换 People 和 AI Workbench 中的 Client Secret，并保证两端配置一致。

## 本地运行

前置服务：Gateway Runtime、People、Permission。People Open API 默认通过 Gateway 的 `http://127.0.0.1:8082/api/open/people` 访问。

```bash
npm --prefix frontend install
./start.sh
```

服务地址：

- 独立 AI Workbench：`http://localhost:5181`
- AI Workbench API：`http://localhost:8087`
- 统一 Admin UI：`http://localhost:5178/ai-workbench/chat`

需要服务在退出终端后继续运行时，安装用户级常驻服务：

```bash
./install-user-service.sh
```

安装后 API 与独立前端会随用户服务管理器自动启动，异常退出时自动重启。查看状态和日志可使用 `systemctl --user status ai-workbench-api ai-workbench-ui` 与 `journalctl --user -u ai-workbench-api -u ai-workbench-ui`。

默认配置位于 `backend/.env.example` 和 `frontend/.env.example`。工作区默认使用 `http://10.251.237.216:5177/oauth/authorize` 作为 People 登录入口；仅在浏览器与 People 都运行于同一台机器时，才应将 `AI_WORKBENCH_PEOPLE_AUTHORIZE_URL` 覆盖为 `http://localhost:5177/oauth/authorize`。还可在 `backend/.env` 覆盖模型密钥加密主密钥、OAuth Client Secret、允许来源和数据库路径。`AI_WORKBENCH_ENCRYPTION_KEY` 轮换前必须迁移已有模型密钥，否则旧密文将无法解密。

## Docker 与公网部署

仓库根目录的 `compose.yaml` 会构建后端和 Nginx 前端。后端不直接暴露宿主机端口，Nginx 仅监听 `127.0.0.1:18082` 并将 `/api/` 同源反代到后端。SQLite、上传暂存目录和内部账号保存在 `workbench-data` volume。

```bash
export AI_WORKBENCH_ENCRYPTION_KEY='替换为至少 32 字符的随机值'
export AI_WORKBENCH_PEOPLE_CLIENT_SECRET='与 People OAuth 客户端一致的密钥'
docker compose up -d --build
curl http://127.0.0.1:18082/health
```

当前生产入口为 `https://ai.lxvb.top`，Cloudflare Tunnel 源站指向 `http://127.0.0.1:18082`。该入口与 terminal、iStore Admin、SSH 和 IM 使用不同主机名及端口，部署不需要修改它们的现有路由。

## Android 客户端

`android/` 是固定访问 `https://ai.lxvb.top` 的原生 WebView 客户端，包含系统文件选择、返回导航、下载、外部链接和 Safe Browsing。它使用 `compileSdk 36`、`targetSdk 36`、`minSdk 35`，覆盖 Android 15 和 Android 16。

```bash
cd android
./gradlew assembleDebug
```

构建需要 JDK 17 和 Android SDK Platform 36，调试 APK 输出到 `app/build/outputs/apk/debug/app-debug.apk`。

## 每日 AI 情报

后端每天按 `AI_WORKBENCH_NEWS_TIMEZONE`（默认 `Asia/Shanghai`）在 `AI_WORKBENCH_NEWS_REFRESH_HOUR`（默认上午 10 点）自动拉取一次最近 `AI_WORKBENCH_NEWS_LOOKBACK_HOURS`（默认 24 小时）内发布的新热点；当天失败时每 15 分钟重试，服务在 10 点后启动且当天尚未成功时会立即补拉。页面仍支持手动同步。人物动态继续按 `AI_WORKBENCH_CONTENT_REFRESH_HOURS`（默认 24 小时）检查更新。

- AI 热点使用各机构公开订阅源：[OpenAI News RSS](https://openai.com/news/rss.xml)、[Google AI RSS](https://blog.google/technology/ai/rss/)、[Hugging Face Blog Feed](https://huggingface.co/blog/feed.xml) 与 [arXiv cs.AI RSS](https://export.arxiv.org/rss/cs.AI)。文章按原文 URL 去重，收藏按 People 用户隔离；页面会使用当前用户启用的模型批量生成简短中文概要并缓存，未配置模型时保留来源原始摘要。
- 大佬动态默认关注 Codex 产品负责人 Tibor Blaho（`@btibor91`），并允许每位用户维护自己的 X 关注列表。接入遵循 X API v2 的 [User Lookup](https://docs.x.com/x-api/users/lookup/introduction) 与 [Get Posts](https://docs.x.com/x-api/users/get-posts) 接口。
- X Bearer Token 仅通过后端环境变量 `AI_WORKBENCH_X_BEARER_TOKEN` 提供，不会返回浏览器或写入日志。未配置时 AI 热点仍可正常工作，人物动态页面会显示配置状态。

## 前沿项目

“前沿项目”通过 GitHub Repository Search 实时检索 AI 项目、Skill 和插件，支持关键词、语言、活跃时间与排序筛选。默认推荐分综合 Star、Fork、最近推送和开源协议、主题、主页、讨论区等成熟度信号，不以单一 Star 数代替项目质量。

默认榜单每天按 `AI_WORKBENCH_FRONTIER_TIMEZONE`（默认 `Asia/Shanghai`）在 `AI_WORKBENCH_FRONTIER_REFRESH_HOUR`（默认 11 点）自动拉取 AI 项目、Skill 和插件三类结果并写入 SQLite。服务在 11 点后启动且当天尚未成功时会立即补拉，失败时保留上一版完整快照并每 15 分钟重试。自定义关键词或筛选条件仍会实时查询 GitHub。

未配置 `AI_WORKBENCH_GITHUB_TOKEN` 时仍可使用 GitHub 公开搜索额度；生产环境建议配置服务端 Token 以获得更稳定的额度。Token 只由后端使用，不会返回浏览器或写入日志。相同实时筛选结果会缓存 5 分钟，上游短暂异常时可在一小时内回退到缓存结果。

## 模型连接

模型服务需兼容以下接口：

```text
GET  {baseUrl}/models
POST {baseUrl}/chat/completions
```

云端 OpenAI Compatible 服务通常需要 API Key；Ollama 等内网模型可留空。只有内部管理员能够创建和修改连接，普通用户只能获取已启用连接的名称与默认模型。生产环境仍应通过网络策略限制后端可访问的模型服务地址。

## 验证

```bash
cd backend
gofmt -w ./cmd ./internal
go test ./...

cd ../frontend
npm run typecheck
npm test
npm run build

cd ../android
./gradlew tasks
```
