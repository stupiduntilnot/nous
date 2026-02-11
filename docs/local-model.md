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

当前项目现在有两条清晰路径：

1. `--provider openai`：严格 OpenAI 语义（只认标准 `tool_calls`）
2. `--provider ollama`：本地 OpenAI-compatible 语义（兼容文本 JSON tool call 回退）
3. `--provider gemini`
4. `--api-base` 可指定 provider endpoint

因此开发阶段推荐：

1. 本地模型统一走 `--provider ollama`
- 默认目标是 Ollama 的 OpenAI-compatible endpoint
- 同时兼容部分本地模型“把 tool call 作为文本输出”的情况

2. 云模型只做回归验证
- 日常开发默认本地模型，降低成本

## 4. 最小可用操作（Ollama 示例）

### 4.1 安装 Ollama（开发机）

macOS（Homebrew）：

```bash
brew install ollama
```

Linux：

```bash
curl -fsSL https://ollama.com/install.sh | sh
```

安装验收：

```bash
ollama --version
```

### 4.2 启动本地模型服务与下载模型

```bash
ollama serve
ollama pull qwen2.5-coder:7b
```

可选验收：

```bash
ollama list
```

### 4.3 启动 Core（连接本地服务）

注意：`api-base` 支持两种写法（都会归一化）：`http://127.0.0.1:11434` 或 `http://127.0.0.1:11434/v1`。

```bash
go run ./cmd/core \
  --socket /tmp/pi-core.sock \
  --provider ollama \
  --model qwen2.5-coder:7b \
  --api-base http://127.0.0.1:11434
```

### 4.4 发送请求

```bash
go run ./cmd/corectl --socket /tmp/pi-core.sock prompt "用一句话介绍你自己"
```

或者直接跑本地链路验收脚本：

```bash
make e2e-local
```

## 5. 什么时候需要真实云 API Key

只有在以下场景才必须提供云 key：

1. 你要测试 `--provider gemini`（需要 `GEMINI_API_KEY`）
2. 你要测试 `--provider openai`（需要真实 `OPENAI_API_KEY`）

如果只是本地模型开发，`--provider ollama` 默认不要求你显式提供 key（内部用占位值）。

## 6. 建议的开发策略

1. 日常开发：`--provider ollama`（本地模型）
2. 里程碑回归：云模型（Gemini/OpenAI）做小样本验证
3. 最终发布前：云模型 + 本地模型都跑一遍 `e2e-smoke`
