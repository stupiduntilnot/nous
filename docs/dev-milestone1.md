# Nous Milestone 1 开发执行手册（单文件版）

本文是 Milestone 1 的唯一执行文档，目标是让任意 Agent 从零实现并验收通过。

## 0. Milestone 1 定义

Milestone 1 完成条件：
1. Headless Core + 最小 TUI 可用。
2. IPC 固定为 UDS + NDJSON。
3. Core 具备 pi 风格顺序 loop（含 tool-call -> tool-result -> 下一轮收敛）。
4. Session / Extension / Active tools 可用。
5. 内置工具 `read/bash/edit/write/grep/find/ls` 全部可用。
6. `make release-gate` 通过。

当前 provider 分层：
- 当前可用：`mock/openai/gemini`。
- 当前验收主链路：`openai`。
- 后续目标：`codex/claude/gemini` 语义对齐。

## 1. 开发约束

1. Core 采用单写者状态机；除 I/O 外，不并发写 run/turn/session 状态。
2. 协议先行：先稳定命令/响应/事件语义，再扩功能。
3. 每步都必须有：手工验证 + 单元测试 + 集成测试。
4. 每步完成后建议提交一个 commit，便于回滚。

## 1.1 协议最小命令与事件清单（固定字面量）

命令：
- `ping`
- `prompt`
- `steer`
- `follow_up`
- `abort`
- `set_active_tools`
- `new_session`
- `switch_session`
- `branch_session`
- `extension_command`

事件：
- `agent_start` / `agent_end`
- `turn_start` / `turn_end`
- `message_start` / `message_update` / `message_end`
- `tool_execution_start` / `tool_execution_update` / `tool_execution_end`
- `status` / `warning` / `error`

## 2. Step-by-step（从零到通过 Milestone 1）

### Step 01: 初始化工程骨架
- 目标：建立 Go module、目录结构、基础 Makefile。
- 重要说明：从一开始就把 `build/test/lint` 入口固定，避免后续脚本分裂。
- 手工测试：
  - `go env GOMOD`
  - `go build ./...`
- 单元测试要求：
  - 至少一个空包 smoke test，保证测试框架可运行。
- 集成测试要求：
  - `make build`、`make test`、`make lint` 可执行。
- 常见问题：
  - 在仓库根目录直接 `go test`（不是 `go test ./...`）会报 `no Go files`。

### Step 02: 定义协议与 ADR
- 目标：固定 `UDS + NDJSON`，并产出机器可读协议文件与样例。
- 重要说明：先定契约，再实现 server/client。
- 手工测试：
  - 打开协议文件确认命令、响应、事件三类结构完整。
- 单元测试要求：
  - schema 校验测试。
  - examples NDJSON 每行可解析测试。
- 集成测试要求：
  - 样例回放与事件顺序一致性测试。
- 常见问题：
  - NDJSON 必须“一行一个 JSON 对象”，不能多行拼接一个对象。

### Step 03: 最小 Core + Ping/Pong
- 目标：Core 监听 UDS，支持 `ping` 命令。
- 重要说明：先实现最小闭环，不要先上模型。
- 手工测试：
  - `echo '{"id":"1","type":"ping"}' | nc -U /tmp/nous-core.sock`
- 单元测试要求：
  - 命令解析测试。
  - 标准错误结构测试（code/message/cause）。
- 集成测试要求：
  - `TestCorePingPong`。

### Step 04: 最小 CLI + 最小 TUI
- 目标：`corectl`/TUI 能完成 ping-pong。
- 重要说明：TUI 只做薄客户端。
- 手工测试：
  - `go run ./cmd/corectl --socket /tmp/nous-core.sock ping`
  - `go run ./cmd/tui /tmp/nous-core.sock` 后输入 `ping`
- 单元测试要求：
  - TUI 命令解析测试。
- 集成测试要求：
  - `scripts/pingpong.sh`、`scripts/tui-smoke.sh`。

### Step 05: Runtime 状态机与事件生命周期
- 目标：实现 `idle/running/aborting` 及 agent/turn/message 事件序列。
- 重要说明：所有状态迁移在单 goroutine 里串行推进。
- 手工测试：
  - 发一个 `prompt`，观察事件顺序：`agent_start -> turn_start -> ... -> turn_end -> agent_end`。
- 单元测试要求：
  - 合法/非法迁移测试。
  - golden 事件顺序测试。
- 集成测试要求：
  - prompt 基础链路测试（不依赖真实 provider）。
- 常见问题：
  - 如果多 goroutine 直接写 runtime，事件顺序容易漂移。

### Step 06: Provider Adapter 基线
- 目标：接入 `mock/openai/gemini`，OpenAI 为主验收链路。
- 重要说明：provider 只是 adapter，不进入 Core 语义层。
- 手工测试：
  - `source ~/.zshrc`
  - `OPENAI_API_KEY` 可见后执行 `corectl prompt`。
- 单元测试要求：
  - provider factory 测试（已知 provider/未知 provider）。
  - HTTP adapter 响应解析测试。
- 集成测试要求：
  - `scripts/local-smoke.sh`。
- 常见问题：
  - 未加载环境变量会导致 `make ci` 在 local-smoke 阶段失败。

### Step 07: 顺序 Tool Loop 收敛
- 目标：同一 run 内循环执行 tool calls，直到收敛或 abort。
- 重要说明：不能“只调一次 tool 就结束”。
- 手工测试：
  - 用提示词让模型先调工具再总结，确认出现 `await_next` 后继续下一轮。
- 单元测试要求：
  - `tool_call -> tool_result -> next provider call`。
  - pending steer/follow_up 时不得提前结束。
- 集成测试要求：
  - 事件回放与语义一致性测试。
- 常见问题：
  - 初版容易漏掉“无 await_next 但有 tool calls”场景，导致提前收敛。

### Step 08: steer / follow_up / abort
- 目标：实现队列语义与取消传播。
- 重要说明：`abort` 是唯一允许强制提前终止 run 的命令。
- 手工测试：
  - 运行中发 `steer`、`follow_up`、`abort` 观察事件。
- 单元测试要求：
  - 三类语义分别有独立测试。
- 集成测试要求：
  - IPC 层命令调度与状态恢复测试。

### Step 09: Session 持久化与分支
- 目标：JSONL append-only、恢复、切换、分支。
- 重要说明：损坏行容错恢复必须实现。
- 手工测试：
  - `new` / `switch` / `branch` 命令全链路。
- 单元测试要求：
  - 追加、读取、损坏行恢复、parent 关系。
- 集成测试要求：
  - `scripts/session-smoke.sh`。

### Step 10: Extension MVP
- 目标：注册 Tool/Command/Hook（input/tool_call/tool_result/turn_end）。
- 重要说明：扩展逻辑不应破坏 core loop 语义。
- 手工测试：
  - `corectl ext echo '{"text":"hello"}'`
- 单元测试要求：
  - 注册、执行、hook mutate/block 测试。
- 集成测试要求：
  - `scripts/extension-smoke.sh`。

### Step 11: 内置工具全量实现
- 目标：`read/bash/edit/write/grep/find/ls` 完整可用。
- 重要说明：`tool registry` 与 `active tools` 必须分离。
- 手工测试：
  - `set_active_tools read` 后触发 read。
  - `set_active_tools` 空参数可清空 active set。
- 单元测试要求：
  - 每个工具覆盖成功路径 + 参数错误 + 边界错误。
  - `bash` 覆盖 timeout/非零退出码/截断。
- 集成测试要求：
  - 提示词触发工具真实调用（至少 read + bash）。
- 常见问题：
  - `read` 参数名不兼容（如 `file_path/filePath/path`）会导致 `read_invalid_path`。
  - 本地模型可能输出非预期 tool arguments，需在 validation 层做别名归一化。

### Step 12: bash 对齐到 pi 风格语义
- 目标：bash 截断/超时/错误文案与 pi 风格一致。
- 重要说明：
  - 默认 tail 截断。
  - 截断后写 full output 到临时文件。
- 手工测试：
  - 执行大输出命令，确认提示包含 full output path。
- 单元测试要求：
  - line limit / bytes limit / timeout / exit code。
- 集成测试要求：
  - 用真实 prompt 触发 bash，确认 `tool_execution_update` 可见。

### Step 13: 文档一致性与协议一致性
- 目标：需求、开发、协议文件与实现一致。
- 重要说明：文档漂移会直接导致后续 agent 实现误判。
- 手工测试：
  - 检查 provider 现状与目标分层描述一致。
- 单元测试要求：
  - protocol/doc consistency tests。
- 集成测试要求：
  - `scripts/protocol-compat-smoke.sh`。

### Step 14: 最终门禁
- 目标：完成回归并生成证据。
- 重要说明：必须先 `source ~/.zshrc` 再跑需要 OpenAI 的门禁。
- 手工测试：
  - `make ci`
  - `make e2e-tui-evidence`
  - `make release-gate`
- 单元测试要求：
  - `go test ./...` 全绿。
- 集成测试要求：
  - `scripts/pingpong.sh`
  - `scripts/smoke.sh`
  - `scripts/protocol-compat-smoke.sh`
  - `scripts/tui-smoke.sh`
  - `scripts/local-smoke.sh`
  - `scripts/session-smoke.sh`
  - `scripts/extension-smoke.sh`
  - `scripts/tui-evidence.sh`（产物：`artifacts/tui-evidence-*.log`）

## 3. 已发生问题与处理策略（按归属分类）

### 协议层
1. 问题：路径文档与 schema 路径不一致导致 gate 失败。
- 处理：固定文档文件名并写一致性测试。

### Runtime 层
1. 问题：曾出现“执行一次 tool call 即结束 run”。
- 处理：引入收敛条件（无新 tool calls 且无 pending messages）并加测试。

### Tool 层
1. 问题：`read` 出现 `read_invalid_path`。
- 处理：补齐参数别名归一化与 validation。
2. 问题：`bash` 输出截断语义不一致。
- 处理：改为 2000 行/50KB tail 截断 + full output 文件提示。

### Provider 层
1. 问题：本地模型链路稳定性不足，影响开发效率。
- 处理：Milestone 1 验收主链路固定 OpenAI，local model 后置。

### 工程流程
1. 问题：在仓库根目录运行 `go test` 导致 `no Go files`。
- 处理：统一使用 `go test ./...`。
2. 问题：环境变量未注入导致 `make ci` 失败。
- 处理：在手册中明确 `source ~/.zshrc` 前置步骤。

## 4. 交付清单（Milestone 1）

1. 可运行二进制：`bin/nous-core`、`bin/nousctl`、`bin/nous-tui`。
2. 协议规范文件与 NDJSON 样例。
3. 全量单测 + 关键集成脚本。
4. 门禁命令：`make release-gate`。
