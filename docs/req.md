# Go 版 Pi Agent 复刻需求文档（v0.1）

## 1. 项目目标

本项目目标是用 Go 语言复刻 `pi agent` 的核心运行结构，形成一个可运行、可扩展的 Agent 系统基础框架。

目标是“运行结构一比一复刻优先”，而不是先做差异化创新。

---

## 2. 核心要求（必须满足）

1. 架构必须是 `Headless`核心 + `TUI` 前端模式。
2. 采用 Headless Core 的理由：未来若需要支持 Web 前端，Core 无需修改即可复用。
3. 复刻 `pi agent` 的核心运行语义（Agent loop、消息模型、Tool 执行模型、事件流模型、会话模型、扩展模型），以运行语义对齐为第一优先级。
4. 明确边界：对齐的是 Core 运行语义，不是 PI 的进程拓扑或 UI 实现细节。

---

## 3. 范围定义

### 3.1 本期范围（In Scope）

1. Headless Core Runtime（无 UI）。
2. TUI Client（调用 Core Runtime）。
3. Tool 模型与执行循环（默认顺序执行）。
4. 消息与事件模型（含 streaming message lifecycle）。
5. Session 持久化与恢复。
6. Provider Adapter 抽象层（先实现最小可用 provider）。
7. 扩展机制最小版本（注册 Tool、命令、事件 hook）。
8. TUI 仅作为 Core 验证前端，优先使用成熟第三方库，以尽量少代码实现可用界面。

### 3.2 非本期范围（Out of Scope）

1. DAG 并行 Tool 调度（后续优化）。
2. 多租户/企业级权限系统（先定义边界，不完整实现）。
3. 全量 provider 兼容（先做少量关键 provider： codex，claude code，gemini）。
4. 复杂前端能力（高级 UI、视觉优化、复杂交互系统）不在本期范围。

---

### 3.3 开发优先级（P0-P2）

1. `P0` Core Runtime
- Agent loop / Turn / Message / Tool lifecycle
- Session 持久化与恢复
- 扩展机制最小闭环

2. `P1` IPC/RPC 协议层
- 命令-事件协议
- 请求响应关联、取消、超时
- Headless 独立运行能力

3. `P2` 最小 TUI
- 仅用于验证 Core 行为
- 低代码量实现，优先复用成熟 TUI 库

---

## 4. 目标架构

### 4.1 分层

1. `Core Runtime (Headless)`
- Agent loop
- Context 构造
- Tool 调度执行
- Session 管理
- Event Bus
- Provider Adapter
  - 本地模型优先通过 OpenAI-compatible endpoint 接入（如 Ollama/LM Studio/vLLM）

2. `IPC Interface Layer`
- Core 对外控制协议（命令 + 事件）
- 支持本地进程间通信
- 预留 Task Pipeline 适配点

3. `TUI Client`
- 通过 IPC 与 Core 通信
- 只做展示与交互，不承载业务执行逻辑

### 4.2 关键原则

1. 单一执行事实源：只有 Core Runtime 执行业务逻辑。
2. UI 薄层化：TUI 仅消费事件、发送命令。
3. 协议稳定优先：先固定内部消息与事件协议，再扩展能力。
4. 可观测优先：每个 Turn、Tool、Message 都必须可追踪。
5. 架构自由原则：通信方式可自定义，但 Core 语义必须稳定对齐目标。
6. 开发成本优先：开发阶段默认优先使用本地模型，云模型仅用于回归验证。

---

### 4.3 并发与执行模型决策（已定）

1. 本期采用 PI 风格顺序 loop（默认串行 Tool 执行）。
2. Core 使用“单写者状态机”原则：
- 一个主循环负责状态推进与事件提交顺序。
- 避免多 goroutine 并发写会话状态导致语义漂移。
3. Go 并发能力仅用于边界层（可选）：
- I/O 执行、外部调用可并发。
- 结果必须回到主循环按序提交。
4. 暂不引入 DAG 并行调度；相关设计仅保留扩展点。

---

## 5. 运行结构复刻要求（与 Pi 对齐）

必须复刻以下结构关系：

1. `AgentRun`（agent_start -> agent_end）
2. `Turn`（turn_start -> turn_end）
3. `AssistantMessage`（可包含 text/thinking/toolCall）
4. `ToolExecutionPhase`（tool_execution_start/update/end）
5. `ToolResultMessage` 回灌后进入下一 Turn
6. 消息生命周期事件：message_start/message_update/message_end
7. 队列语义：`steer` 与 `followUp`

### 5.1 Tool Loop 收敛条件（强约束）

1. 单次 `prompt` 不是“只跑一轮模型 + 一次工具调用”，而是一个完整 Agent Run。
2. Core 必须在同一 Run 内循环执行，直到满足收敛条件才允许 `turn_end/agent_end`。
3. 每轮循环处理顺序：
- 先向模型请求 assistant 输出
- 若 assistant 含 tool calls，则按顺序执行所有 tool calls
- 将 `tool_result` 回灌上下文后继续下一轮模型请求
4. 仅在以下条件同时满足时收敛：
- 当前 assistant 输出不再包含新的 tool calls
- 没有待处理的队列输入（如 `steer/follow_up` 形成的 pending messages）
5. 违反上述收敛条件属于实现错误（例如“执行一次 tool call 就结束”）。
6. `abort` 可打断循环并强制结束当前 Run；除 `abort` 外不得提前终止。

---

## 6. 通信与协议要求

### 6.1 IPC（本期）

0. 传输与线协议（已定）
- transport：仅支持 `Unix Domain Socket (UDS)`。
- wire protocol：使用 `NDJSON`（一行一个 JSON 消息）。

1. 命令通道
- `prompt`
- `steer`
- `follow_up`
- `abort`
- `set_active_tools`
- `new_session` / `switch_session` / `branch_session`
- `extension_command`

2. 事件通道
- agent_start / agent_end
- turn_start / turn_end
- message_start / message_update / message_end
- tool_execution_start / tool_execution_update / tool_execution_end
- error / warning / status

3. 协议要求
- 明确消息类型与版本字段
- 支持请求-响应关联 ID
- 支持取消与超时语义

### 6.2 Task Pipeline（预留）

1. Core 必须保留替换 IPC transport 的能力（在不破坏现有 UDS + NDJSON 协议语义前提下演进）。
2. 不把 transport 与业务逻辑耦合。
3. 允许未来把命令与事件映射到异步任务队列。
4. 该层属于本项目自主设计，不要求保持与 PI 一致。

---

## 7. Tool 与调度要求

1. 先实现顺序 Tool 执行。
2. `tool registry` 与 `active tools` 必须分离。
3. 每次 Turn 请求模型时，只发送当前 active tools。
4. 在运行中支持切换 active tools（用于 Scenario phase 控制）。
5. 预留 Tool 元数据位（为未来 DAG 并发做准备）：
- sideEffectFree
- locks
- mustSerial

---

## 8. 扩展机制要求（最小可用）

1. 支持注册：
- Tool
- Command
- Event Hook（至少 input/tool_call/tool_result/turn_end）

2. 支持运行时策略控制：
- setActiveTools
- 读写 Scenario 状态
- 触发用户确认（后续 UI 实现）

3. 支持外部配置注入（不要把业务约束硬编码在 Go 代码）：
- system-level prompt
- scenario-level prompt/skill
- environment-level config

---

## 9. 阶段计划（建议）

### Phase 1：Core Skeleton

1. 建立 Agent loop、Message/Event 模型、Session 模型。
2. 完成最小 provider adapter 与单 provider 接入（优先 OpenAI-compatible 本地模型链路）。
3. 完成顺序 Tool 执行与 ToolResult 回灌。
4. 落实单写者顺序状态机（并发能力不进入 Core 主语义）。

### Phase 2：IPC + TUI

1. 定义 IPC 协议并实现客户端 SDK。
2. 实现 TUI 客户端（最小交互 + 事件展示）。
3. 跑通完整链路：prompt -> tool call -> result -> final answer。

### Phase 3：Extension MVP

1. 支持 Tool/Command/Hook 注册。
2. 实现 `steer` / `followUp` 队列语义。
3. 实现 active tools 动态切换。

---

## 10. 验收标准（MVP）

满足以下条件即视为第一阶段可用：

1. 可以通过 TUI 输入任务并触发完整 Agent run。
2. 可以观察到标准事件流（agent/turn/message/tool）。
3. Tool 可以被模型调用并返回 ToolResult 后继续下一 Turn。
4. 可以在运行中通过 `steer` 插入纠偏消息。
5. 可以按 Scenario phase 切换 active tools。
6. Session 可持久化、可恢复。
7. Headless 模式可通过 IPC 独立运行，无需 TUI。
8. 可通过本地模型服务（Ollama/LM Studio/vLLM）完成端到端通信，无需云端 API。

---

## 11. 风险与约束

1. 结构复刻优先将降低早期开发速度。
2. Provider 差异会带来适配复杂度。
3. 若 IPC 协议前期不稳定，会反复影响 TUI 层。
4. 过早引入并发 DAG 会显著增加复杂度，应后置。
5. 若过早使用多 goroutine 并发写 Core 状态，事件顺序与可观测性会退化。

---

## 12. 决策原则

1. 先正确，再优化。
2. 先对齐运行结构，再做能力扩展。
3. 所有新功能优先通过 Extension/配置注入，不直接硬编码到 Core。
4. 开发资源分配遵循 `P0 > P1 > P2`。
