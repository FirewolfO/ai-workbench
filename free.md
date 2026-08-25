# AI Workbench 免费模型方案

更新时间：2026-08-25

## 当前结论

- 现有 `chatgpt` 连接已从 AI Workbench 后端完成模型列表和对话实测，可以继续作为主连接。
- 现有 `chatgpt-a` 的域名已无 DNS 记录，后端、宿主机和公共 DNS 都无法解析。该问题不能靠修改容器 DNS 修复，应替换服务地址或停用此连接。
- 推荐新增 Groq 作为免费主备，Google Gemini 作为第二主备，OpenRouter 免费路由作为低频兜底。
- “启用”只表示管理员允许使用；只有“测试连接”成功的连接才会标记为“可使用”并出现在聊天模型列表中。

## 可直接接入的方案

| 优先级 | 服务 | API Base URL | 建议模型 ID | 免费能力与限制 |
| --- | --- | --- | --- | --- |
| 1 | Groq | `https://api.groq.com/openai/v1` | `openai/gpt-oss-120b` | Free Plan 当前为 30 RPM、1,000 RPD、8,000 TPM、200,000 TPD；速度快，适合日常对话和推理。 |
| 2 | Google Gemini | `https://generativelanguage.googleapis.com/v1beta/openai` | `gemini-3.5-flash-lite` | Stable 模型，标准请求的免费层输入和输出均免费；具体速率以 AI Studio 当前项目页面为准。免费层数据可能用于改进 Google 产品。 |
| 3 | OpenRouter | `https://openrouter.ai/api/v1` | `openrouter/free` | 自动选择当时可用的免费模型。未购买至少 10 美元额度时通常为 50 次免费请求/日，容量和具体模型会变化，不适合作为唯一生产连接。 |
| 4 | Hugging Face | `https://router.huggingface.co/v1` | 在 Inference Playground 选择支持 Chat Completion 的模型 ID | 免费账号当前每月只有 0.10 美元试用额度，适合验证和应急，不适合持续使用。 |
| 本地 | Ollama | 见下方容器配置 | `gpt-oss:20b` 或本机已拉取的模型 | 无 API 费用，但占用本机 CPU/GPU、内存和磁盘；性能取决于硬件。 |

以上服务均提供 OpenAI Compatible 的 `GET /models` 和 `POST /chat/completions` 路径，符合当前工作台的连接测试要求。模型和额度会变化，配置时应先在服务控制台确认模型仍对当前账号开放。

## 推荐落地顺序

1. 保留已经实测可用的内部 `chatgpt` 连接。
2. 注册 Groq，创建 API Key，按表格填写连接后点击“测试连接”。成功后应显示“可使用”。
3. 在 Google AI Studio 创建 API Key，优先配置 `gemini-3.5-flash-lite`，作为 Groq 的独立上游备份。
4. 需要更多模型选择时再配置 OpenRouter。`openrouter/free` 的实际模型不固定，不应依赖某个模型特有的输出格式。
5. `chatgpt-a` 在服务方提供新域名之前保持不可用或直接停用，不要通过 hosts 文件猜测 IP。

## 容器连接规则

容器中的 `127.0.0.1` 和 `localhost` 指向容器自身，不是宿主机。因此 Ollama 或其他模型服务运行在宿主机时，AI Workbench 的 Base URL 应填写：

```text
http://host.docker.internal:11434/v1
```

仓库的 `compose.yaml` 已为后端配置 `host.docker.internal:host-gateway`。如果模型服务也在同一个 Compose 网络中，应改用服务名，例如：

```text
http://ollama:11434/v1
```

如果模型服务只监听宿主机 `127.0.0.1`，容器仍然无法访问；需要让模型服务监听 Docker 网桥可达地址，并通过主机防火墙限制来源。不要为了省事直接把未鉴权的 Ollama 端口暴露到公网。

## 工作台连接判定

一次“测试连接”会同时验证：

1. Base URL 能访问并返回 JSON 模型列表。
2. API Key 能通过认证。
3. 默认模型能完成一次最小对话请求。

测试成功后，连接记录最近检测时间和延迟并标记为“可使用”。测试失败后，连接记录失败原因、标记为“连接失败”，并立即从普通用户的模型选择列表中移除。修改 Base URL、默认模型或 API Key 会清除旧检测结果，必须重新测试。

## 官方资料

- [Groq OpenAI Compatibility](https://console.groq.com/docs/openai)
- [Groq Free Plan Rate Limits](https://console.groq.com/docs/rate-limits)
- [Google Gemini OpenAI Compatibility](https://ai.google.dev/gemini-api/docs/openai)
- [Google Gemini API Pricing](https://ai.google.dev/gemini-api/docs/pricing)
- [OpenRouter Free Models Router](https://openrouter.ai/docs/cookbook/get-started/free-models-router-playground)
- [OpenRouter FAQ](https://openrouter.ai/docs/faq)
- [Hugging Face Inference Providers](https://huggingface.co/docs/inference-providers/index)
- [Hugging Face Pricing](https://huggingface.co/docs/inference-providers/pricing)
- [Ollama OpenAI Compatibility](https://docs.ollama.com/api/openai-compatibility)
