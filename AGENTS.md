# AI Workbench 开发约束

## 仓库定位

- `backend/` 是个人 AI 工作台 API 与浏览器 BFF，使用 Go、SQLite，并调用 OpenAI Compatible 模型接口。
- `frontend/` 是独立工作台，使用 Vue 3、TypeScript、Vite 和 Element Plus。
- 独立前端支持内部账号和 People OAuth 登录；统一 `admin-ui` 复用已有 Permission 登录令牌。
- 模型 API Key 只能在后端加密保存和使用，不得返回前端或写入日志。

## 开发要求

- 对话、提示词和普通用户用量必须按当前用户隔离；模型连接由内部管理员统一维护并供所有用户使用，管理员用量概览汇总全站数据。
- 后端修改后运行 `gofmt` 和 `go test ./...`；前端修改后运行 `npm run typecheck`、`npm test` 和 `npm run build`。
- 本目录是独立 Git 仓库。验证后必须提交并推送当前分支。
