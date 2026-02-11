# Go 版 Pi Agent 开发分解（可执行步骤清单）

本文按 `docs/req.md` 拆解为最小可执行步骤。  
目标：每一步都可以独立完成、独立测试、独立验收。
逐步骤验收细则见 `docs/acceptance.md`（该文档为验收主文档）。

---

## 0. 联通性 MVP（进入 P0 前）

### 0.1 实现最小 Core（仅 ping-pong）
- 任务：
  - 启动 UDS server
  - 支持最小命令：`ping`
  - 返回最小响应：`pong`
- 完成标准：
  - Core 在无业务逻辑情况下可稳定收发 NDJSON 消息
- 验证：
  - 集成测试：发送 `ping`，收到 `pong`

### 0.2 实现最小 CLI client（非 TUI）
- 任务：
  - 通过 UDS 连接 Core
  - 发送单条 NDJSON 命令并输出响应
- 完成标准：
  - 命令行可直接完成 ping-pong
- 验证：
  - 手工：`corectl ping` 输出 `pong`
  - 黑盒：CI 中跑一条 ping-pong 测试

### 0.3 实现最小 TUI MVP（只为联调）
- 任务：
  - 单输入框 + 单日志区
  - 支持发送 `ping`，展示 `pong`
  - 支持显示连接状态（connected/disconnected）
- 完成标准：
  - TUI 能通过 UDS 与 Core 完成最小交互闭环
- 验证：
  - 手工测试：输入 ping，界面看到 pong

### 0.4 0 阶段门禁
- 完成标准：
  - Core + CLI + TUI 三者均通过 ping-pong
  - 协议与 transport 的基本稳定性已验证
- 验证：
  - `go test ./...` 通过
  - 最小联调脚本通过

---

## A. 项目骨架与工程约束

### A1. 初始化 Go 工程与目录结构
- 任务：
  - 创建模块（`go mod init`）
  - 建立目录：`cmd/`, `internal/core/`, `internal/ipc/`, `internal/tui/`, `internal/provider/`, `internal/session/`, `internal/extension/`, `pkg/protocol/`
- 完成标准：
  - 代码可编译，空程序可运行
- 验证：
  - `go test ./...`
  - `go build ./...`

### A2. 建立统一 lint/test 脚本
- 任务：
  - 增加 `Makefile` 或脚本：`test`, `lint`, `build`
  - 接入 `go vet`（可选再加 `staticcheck`）
- 完成标准：
  - 一条命令可跑全量检查
- 验证：
  - `make test`
  - `make lint`

### A3. 定义错误码与日志规范
- 任务：
  - 统一错误类型（含 code/message/cause）
  - 统一结构化日志字段（run_id/turn_id/message_id/tool_call_id）
- 完成标准：
  - 核心模块输出结构化日志
- 验证：
  - 单测校验错误序列化
  - 手动运行检查日志字段是否齐全

---

## B. 协议与模型（先定契约）

### B0. 固化协议决策（ADR）
- 任务：
  - 记录并冻结两项决策：
    - transport：仅 `UDS`
    - wire protocol：`NDJSON`
  - 明确本期不实现 `stdio` transport
- 完成标准：
  - ADR/文档落地，团队对协议无歧义
- 验证：
  - 文档审查通过（`docs/req.md` 与 `dev.md` 一致）

### B0.5 产出 OpenAPI 风格协议规范（语义对齐 pi-mono）
- 任务：
  - 新增协议规范文档（JSON/YAML 均可），采用 OpenAPI 风格描述命令、响应、事件结构
  - 在规范中明确：wire framing 为 NDJSON（一行一个 JSON message）
  - 建立“语义对齐矩阵”：逐项映射 `pi-mono` 的命令/响应/事件语义（尤其是 `prompt/steer/follow_up/abort` 与生命周期事件）
  - 明确本项目对 `pi-mono` 的兼容边界（哪些是完全一致，哪些是显式差异）
- 完成标准：
  - 规范文件可被机器读取（schema 可用于校验）
  - 对齐矩阵完整覆盖本期协议面
- 验证：
  - 协议校验测试：使用规范校验命令/响应/事件样例
  - 兼容性测试：回放 `pi-mono` 语义样例，断言本项目输出事件序列与预期一致

### B1. 定义命令协议（IPC 入站）
- 任务：
  - 定义命令：`prompt`, `steer`, `follow_up`, `abort`, `set_active_tools`, `new_session`, `switch_session`
  - 每个命令含 `request_id`
- 完成标准：
  - 命令 JSON 能反序列化并通过 schema 校验
- 验证：
  - 协议单测（合法/非法 payload）

### B2. 定义事件协议（IPC 出站）
- 任务：
  - 定义事件：
    - `agent_start/agent_end`
    - `turn_start/turn_end`
    - `message_start/message_update/message_end`
    - `tool_execution_start/tool_execution_update/tool_execution_end`
    - `error/warning/status`
  - 添加版本字段 `protocol_version`
- 完成标准：
  - 所有事件可稳定序列化
- 验证：
  - Golden tests 校验 JSON 输出

### B3. 定义消息模型
- 任务：
  - `UserMessage`, `AssistantMessage`, `ToolResultMessage`
  - `AssistantMessage` 支持 `text/thinking/toolCall`
- 完成标准：
  - 模型可用于持久化和回放
- 验证：
  - 序列化/反序列化单测

### B4. 定义 Tool 模型
- 任务：
  - `tool registry` 与 `active tools` 分离
  - Tool 输入输出、详情结构统一
- 完成标准：
  - 可以注册、查询、激活、禁用工具
- 验证：
  - 单测覆盖：
    - 注册冲突
    - 激活不存在工具
    - active 集合切换

---

## C. Core 状态机（P0 核心）

### C1. 实现 Run/Turn 状态机骨架（单写者）
- 任务：
  - 单 goroutine 主循环
  - 状态：idle/running/aborting
  - Run 生命周期事件
- 完成标准：
  - 能触发 `agent_start -> agent_end`
- 验证：
  - 状态机单测（合法/非法状态迁移）

### C2. 实现消息生命周期事件
- 任务：
  - 用户消息事件：`message_start/end`
  - 助手消息事件：`start/update/end`
- 完成标准：
  - 事件顺序稳定、可预测
- 验证：
  - 顺序断言单测（事件数组精确比对）

### C3. 接入 ProviderAdapter 接口（先 mock）
- 任务：
  - 抽象 `Stream(ctx, context) -> event stream`
  - 先做 fake provider，输出固定 text/toolcall
- 完成标准：
  - 不依赖外部 API 也能跑通 loop
- 验证：
  - E2E 单测：`prompt -> assistant text`

### C4. 实现 Tool 顺序执行循环
- 任务：
  - 读取 assistant 内 tool calls
  - 顺序执行
  - 发出 tool execution 三段事件
- 完成标准：
  - `ToolResultMessage` 回灌后进入下一 Turn
- 验证：
  - E2E 单测：`prompt -> toolcall -> toolResult -> second turn`

### C5. 实现 `steer` 队列语义
- 任务：
  - 运行中可插入 steer
  - 在 tool 边界投递
  - 插入后可中断剩余 tool（按当前需求）
- 完成标准：
  - steer 能生效并改变后续行为
- 验证：
  - 并发测试：运行中发送 steer，断言事件序列与结果

### C6. 实现 `followUp` 队列语义
- 任务：
  - 当前 run 即将结束时投递 follow_up
- 完成标准：
  - follow_up 不打断当前流程
- 验证：
  - 单测断言 follow_up 在 run 尾部触发下一轮

### C7. 实现 `abort` 语义与取消传播
- 任务：
  - `context.Context` 贯穿 provider/tool
  - abort 后状态回到 idle
- 完成标准：
  - abort 后不再有新工具执行
- 验证：
  - 超时/取消测试，断言停止原因与事件

### C8. 实现 active tools 动态切换
- 任务：
  - 命令可更新 active tool 集
  - 下一次模型请求仅发送 active 集
- 完成标准：
  - 模型请求工具列表可观测
- 验证：
  - 单测检查请求 payload tools 集合

---

## D. Session 持久化与恢复（P0 核心）

### D1. 定义 Session 文件格式（JSONL）
- 任务：
  - 头部 + entry
  - entry 含 `id/parent_id/timestamp/type`
- 完成标准：
  - 文件可 append，可读取
- 验证：
  - 文件读写单测

### D2. 实现追加写与崩溃恢复
- 任务：
  - append-only
  - 对坏行容错（跳过非法行）
- 完成标准：
  - 中断后可恢复最近有效状态
- 验证：
  - 故障注入测试（截断/损坏行）

### D3. 实现会话恢复与切换
- 任务：
  - `new_session`
  - `switch_session`
- 完成标准：
  - 可恢复消息上下文并继续运行
- 验证：
  - E2E：首轮写入 -> 重启 -> 继续 prompt

### D4. 实现基本分支能力（最小）
- 任务：
  - 基于 `parent_id` 继续新分支
- 完成标准：
  - 可以从历史节点继续对话
- 验证：
  - 单测构造树并验证上下文构建

---

## E. Extension MVP（P0 范围内最小闭环）

### E1. 定义 Extension 接口
- 任务：
  - 注册 Tool
  - 注册 Command
  - 注册 Hook：`input/tool_call/tool_result/turn_end`
- 完成标准：
  - 可加载一个内置 demo extension
- 验证：
  - 单测：hook 被调用且可修改行为

### E2. 实现 input hook（transform/handled）
- 任务：
  - 支持输入改写和短路处理
- 完成标准：
  - 输入可被 extension 改写
- 验证：
  - 单测断言改写后 message 内容

### E3. 实现 tool_call/tool_result hook
- 任务：
  - tool_call 可拦截（可拒绝）
  - tool_result 可改写结果
- 完成标准：
  - 可实现策略控制（如 denylist）
- 验证：
  - 单测覆盖 block/transform 两路径

---

## F. IPC 层（P1）

### F1. 实现 UDS transport（NDJSON framing）
- 任务：
  - 使用 Unix Domain Socket 建立命令/事件双向消息流
  - 采用按行读取 NDJSON 作为消息边界
  - 请求响应关联 `request_id`
- 完成标准：
  - 外部进程可控制 core
- 验证：
  - 集成测试：通过 UDS 发送 NDJSON 命令并读取事件流
  - 手工调试：`nc -U /tmp/<socket>.sock`

### F2. 实现超时与错误响应规范
- 任务：
  - 命令级 timeout
  - 标准错误返回
- 完成标准：
  - 调用方可稳定处理失败
- 验证：
  - 超时测试、非法命令测试

### F3. 实现 Headless 运行入口
- 任务：
  - `cmd/core` 启动 headless server
- 完成标准：
  - 无 TUI 可独立运行
- 验证：
  - 黑盒测试：启动进程 + UDS/NDJSON 命令驱动

---

## G. 最小 TUI（P2）

### G1. 选型并接入第三方 TUI 库
- 任务：
  - 确定库（如 bubbletea/tcell 等）
  - 仅实现最小输入输出面板
- 完成标准：
  - TUI 能连通 IPC
- 验证：
  - 手工测试：发送 prompt，看到事件与结果

### G2. 事件渲染最小闭环
- 任务：
  - 渲染 message/tool/状态
  - 支持输入 `prompt/steer/follow_up/abort`
- 完成标准：
  - 可观察完整 agent run
- 验证：
  - 手工测试脚本 + 截图/录屏验收

### G3. 会话操作最小闭环
- 任务：
  - 新建/切换 session
- 完成标准：
  - 前端可控制 session 生命周期
- 验证：
  - 手工测试：切换后上下文变化正确

---

## H. Provider 最小接入

### H1. 实现一个最小可用 provider
- 任务：
  - 接入一个 API（优先易测）
  - 映射统一 assistant 事件
- 完成标准：
  - 真实 API 下可跑通 tool call 回路
- 验证：
  - 集成测试（可用 mock + 可选真实 key）

### H2. Provider 能力差异处理（最小）
- 任务：
  - 不支持能力时返回标准错误
- 完成标准：
  - 不发生 panic/不一致状态
- 验证：
  - 异常路径单测

---

## I. 端到端验收场景（与需求文档对齐）

### I1. 标准链路验收
- 场景：
  - `prompt -> tool call -> tool result -> final answer`
- 验证：
  - 自动化 E2E 测试通过

### I2. steer/followUp 验收
- 场景：
  - 运行中 steer 纠偏
  - follow_up 排队执行
- 验证：
  - 事件顺序与最终输出符合预期

### I3. Session 恢复验收
- 场景：
  - 退出后重启恢复会话继续执行
- 验证：
  - 上下文与历史一致

### I4. Headless 验收
- 场景：
  - 无 TUI 进程下通过 IPC 完成完整 run
- 验证：
  - 黑盒脚本通过

---

## J. 每阶段交付物清单

### J1. Phase 1（Core）
- 交付：
  - 状态机、消息模型、顺序 tool loop、session、extension MVP
- 门禁：
  - `go test ./...` 全绿
  - 核心 E2E（无 UI）全绿

### J2. Phase 2（IPC + TUI）
- 交付：
  - IPC 协议实现、最小 TUI
- 门禁：
  - Headless 黑盒测试通过
  - 手工链路验证通过

### J3. Phase 3（增强）
- 交付：
  - active tools 策略增强、扩展生态增强
- 门禁：
  - 回归测试通过

---

## K. 执行约束（防偏航）

1. 不提前做 DAG 并发调度。
2. 不提前做复杂 UI。
3. 任何新能力优先走 extension/config 注入，不硬编码到 core。
4. 每完成一个步骤，必须补对应测试或验收脚本。
