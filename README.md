# AI Workbench

企业内部个人 AI 工作台，包含 Go 后端和 Vue 3 独立前端，并原生集成到 `admin-ui`。

## 常用能力

- 多个 OpenAI Compatible 模型连接，支持连接测试、启停与默认模型配置
- 模型 API Key 仅在后端使用 AES-GCM 加密保存，接口只返回是否已配置
- 多轮对话、系统提示词、模型切换、自动命名、搜索、置顶、重命名和删除
- Markdown 回答预览、回答复制、响应延迟及 Token 用量展示
- 个人提示词库，支持分类、搜索、收藏、使用次数及一键带入对话
- 个人用量概览和最近对话
- 所有业务数据严格按 People 用户名隔离

## 登录与集成

独立工作台通过 People OAuth 2.0 授权码模式登录。浏览器只把授权码交给 AI Workbench 后端，OAuth Client Secret 不进入前端。

`admin-ui` 不会再次跳转 People 登录，而是把已有 Permission Access Token 交给 AI Workbench 后端验证。两个入口最终都使用 People 用户名作为个人数据主键，因此模型连接、提示词和对话完全共享。

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

默认配置位于 `backend/.env.example` 和 `frontend/.env.example`。可在 `backend/.env` 覆盖模型密钥加密主密钥、People 地址、OAuth Client Secret、允许来源和数据库路径。`AI_WORKBENCH_ENCRYPTION_KEY` 轮换前必须迁移已有模型密钥，否则旧密文将无法解密。

## 模型连接

模型服务需兼容以下接口：

```text
GET  {baseUrl}/models
POST {baseUrl}/chat/completions
```

云端 OpenAI Compatible 服务通常需要 API Key；Ollama 等内网模型可留空。生产环境应通过网络策略限制后端可访问的模型服务地址，并只允许受信任员工创建连接。

## 验证

```bash
cd backend
gofmt -w ./cmd ./internal
go test ./...

cd ../frontend
npm run typecheck
npm test
npm run build
```
