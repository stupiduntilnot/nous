# 本地模型接入说明（面向当前 Go Agent）

## 1. 先澄清概念

你对“本地模型是一个二进制文件”的理解，部分正确，但不完整。

1. 模型权重通常是文件（例如 `gguf`、`safetensors`）。
2. Agent 进程通常不会直接把权重文件读进来做推理。
3. 常见做法是：
- 先启动一个本地推理服务（例如 Ollama、LM Studio、vLLM）。
- 再让 Agent 通过 HTTP API 调这个服务。

所以通信关系是：

`TUI -> Core(UDS+NDJSON) -> ProviderAdapter(HTTP) -> Local Model Server`

## 2. pi-mono 是怎么支持本地模型的（源码结论）

结论：`pi` 不是“内嵌本地推理引擎”，而是通过 provider 配置把本地服务当成一个 API provider 来用。

关键证据：

1. `packages/ai/README.md`
- 明确写了支持 “Any OpenAI-compatible API: Ollama, vLLM, LM Studio, etc.”

2. `packages/coding-agent/docs/models.md`
- 明确给了 `~/.pi/agent/models.json` 的本地示例（`ollama`、`baseUrl`、`api=openai-completions`）。

3. `packages/coding-agent/src/core/model-registry.ts`
- 解析 `models.json` 的 `providers[].baseUrl/api/apiKey/models`，并合并到运行时模型注册表。
- 说明 pi 的机制是“配置 provider endpoint”，不是“加载模型文件到 agent 进程”。

## 3. 我们这个 Go 项目该怎么做

当前项目已经有：

1. `cmd/core --provider mock|openai|gemini`
2. `--api-base` 可指定 provider endpoint
3. `openai` adapter 走 `/v1/chat/completions`

因此开发阶段推荐：

1. 本地模型统一走 `openai-compatible` 路径
- Ollama / LM Studio / vLLM 都可先对接成 `--provider openai`

2. 云模型只做回归验证
- 日常开发默认本地模型，降低成本

## 4. 最小可用操作（Ollama 示例）

### 4.1 启动本地模型服务

```bash
ollama serve
ollama pull qwen2.5-coder:7b
```

### 4.2 启动 Core（连接本地服务）

注意：当前代码会请求 `<api-base>/v1/chat/completions`，所以 `api-base` 传主机根地址，不要再额外带 `/v1`。

```bash
OPENAI_API_KEY=dummy \
go run ./cmd/core \
  --socket /tmp/pi-core.sock \
  --provider openai \
  --model qwen2.5-coder:7b \
  --api-base http://127.0.0.1:11434
```

### 4.3 发送请求

```bash
go run ./cmd/corectl --socket /tmp/pi-core.sock prompt "用一句话介绍你自己"
```

## 5. 什么时候需要真实云 API Key

只有在以下场景才必须提供云 key：

1. 你要测试 `--provider gemini`（需要 `GEMINI_API_KEY`）
2. 你要测试 OpenAI 云端（需要真实 `OPENAI_API_KEY`）

如果只是本地模型开发，`openai-compatible` 通常用占位 key（如 `dummy`）即可。

## 6. 建议的开发策略

1. 日常开发：本地模型（Ollama/LM Studio/vLLM）
2. 里程碑回归：云模型（Gemini/OpenAI）做小样本验证
3. 最终发布前：云模型 + 本地模型都跑一遍 `e2e-smoke`
