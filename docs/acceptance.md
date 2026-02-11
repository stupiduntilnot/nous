# 验收标准清单（对应 `dev.md`）

本文是逐步骤验收主文档。  
每个步骤都必须满足：`通过条件` + `验证方式`。

---

## 0. 联通性 MVP

### 0.1 最小 Core（ping-pong）
- 通过条件：
  - Core 可监听 UDS 路径并稳定处理 `ping`
  - 返回 `pong`（含 `id` 对应）
- 验证方式：
  - 自动化：集成测试 `TestCorePingPong`
  - 手工：`echo '{"id":"1","type":"ping"}' | nc -U /tmp/pi-core.sock`

### 0.2 最小 CLI client
- 通过条件：
  - CLI 能连接 UDS 并收到 `pong`
- 验证方式：
  - 自动化：黑盒测试 `TestCorectlPing`
  - 手工：`corectl ping --socket /tmp/pi-core.sock`

### 0.3 最小 TUI MVP
- 通过条件：
  - 输入 `ping` 后 UI 日志显示 `pong`
  - 连接状态可显示
- 验证方式：
  - 手工：启动 Core + TUI，执行一次 ping
  - 证据：截图或录屏

### 0.4 0 阶段门禁
- 通过条件：
  - 0.1~0.3 全部通过
- 验证方式：
  - `go test ./...`
  - 手工联调脚本通过

---

## A. 项目骨架与工程约束

### A1 初始化工程结构
- 通过条件：
  - 模块与目录结构存在，空程序可编译
- 验证方式：
  - `go build ./...`
  - `go test ./...`

### A2 lint/test 脚本
- 通过条件：
  - 一条命令跑完检查
- 验证方式：
  - `make test`
  - `make lint`

### A3 错误码与日志规范
- 通过条件：
  - 错误结构统一（code/message/cause）
  - 关键日志字段齐全
- 验证方式：
  - 单测 `TestErrorEnvelope`
  - 手工检查日志包含 `run_id/turn_id/message_id/tool_call_id`

---

## B. 协议与模型

### B0 协议决策 ADR
- 通过条件：
  - 明确写入：`UDS + NDJSON`
- 验证方式：
  - 文档审查：`docs/req.md`、`dev.md`、本文件一致

### B0.5 OpenAPI 风格协议规范（语义对齐 pi-mono）
- 通过条件：
  - 存在可机器读取的协议规范文件（OpenAPI 风格）
  - 规范明确 NDJSON framing 规则
  - 存在 `pi-mono` 语义对齐矩阵（命令/响应/事件）
  - 兼容边界有明确说明（完全一致项 vs 差异项）
- 验证方式：
  - 协议样例校验测试：`TestProtocolSchemaValidation`
  - 语义兼容测试：`TestPiMonoSemanticCompatibility`
  - 样例回放：基于 `pi-mono` 采样的 JSONL corpus 回放并断言事件序列

### B1 命令协议
- 通过条件：
  - 命令反序列化与 schema 校验通过
  - 非法 payload 返回标准错误
- 验证方式：
  - 单测：
    - `TestCommandDecodeValid`
    - `TestCommandDecodeInvalid`

### B2 事件协议
- 通过条件：
  - 事件 JSON 稳定，含版本字段
- 验证方式：
  - Golden tests：`testdata/events/*.golden.json`

### B3 消息模型
- 通过条件：
  - 三类消息可稳定序列化/反序列化
- 验证方式：
  - 单测 `TestMessageRoundTrip`

### B4 Tool 模型与 active 集
- 通过条件：
  - registry 与 active set 分离
  - 激活/禁用行为可预测
- 验证方式：
  - 单测：
    - `TestToolRegistryCollision`
    - `TestActiveToolsSwitch`
    - `TestActiveToolsIgnoreUnknown`

---

## C. Core 状态机

### C1 Run/Turn 状态机骨架
- 通过条件：
  - 状态迁移合法且可重复
- 验证方式：
  - 单测 `TestStateTransitions`

### C2 消息生命周期事件
- 通过条件：
  - 事件顺序严格符合定义
- 验证方式：
  - 单测 `TestMessageEventOrdering`

### C3 ProviderAdapter mock 接入
- 通过条件：
  - 不依赖外部 API 可跑完整回合
- 验证方式：
  - E2E 单测 `TestPromptWithFakeProvider`

### C4 顺序 tool loop
- 通过条件：
  - tool 调用顺序执行，回灌后进入下一 turn
- 验证方式：
  - E2E 单测 `TestToolCallSequentialLoop`

### C5 steer 语义
- 通过条件：
  - 运行中 steer 可生效并改变后续执行
- 验证方式：
  - 单测 `TestSteerInterruptsFlow`

### C6 follow_up 语义
- 通过条件：
  - follow_up 不打断当前流程，只在尾部执行
- 验证方式：
  - 单测 `TestFollowUpAfterRunEnd`

### C7 abort 语义
- 通过条件：
  - abort 后停止执行并回到 idle
- 验证方式：
  - 单测 `TestAbortPropagation`

### C8 active tools 动态切换
- 通过条件：
  - 下一次模型请求仅携带当前 active set
- 验证方式：
  - 单测 `TestModelRequestUsesActiveTools`

---

## D. Session 持久化与恢复

### D1 Session JSONL 格式
- 通过条件：
  - 文件格式可持续 append/read
- 验证方式：
  - 单测 `TestSessionAppendRead`

### D2 崩溃恢复
- 通过条件：
  - 损坏行不致崩溃，可恢复到最近有效状态
- 验证方式：
  - 单测 `TestSessionRecoveryFromCorruption`

### D3 会话恢复与切换
- 通过条件：
  - 能恢复上下文并继续执行
- 验证方式：
  - E2E `TestResumeAndSwitchSession`

### D4 最小分支能力
- 通过条件：
  - 从历史节点继续，分支上下文正确
- 验证方式：
  - 单测 `TestBranchContextBuild`

---

## E. Extension MVP

### E1 Extension 接口
- 通过条件：
  - 可注册 Tool/Command/Hook
- 验证方式：
  - 单测 `TestExtensionRegistration`

### E2 input hook
- 通过条件：
  - 支持 `transform` 与 `handled`
- 验证方式：
  - 单测 `TestInputHookTransform`
  - 单测 `TestInputHookHandled`

### E3 tool_call/tool_result hook
- 通过条件：
  - 支持拦截与结果改写
- 验证方式：
  - 单测 `TestToolCallBlockedByHook`
  - 单测 `TestToolResultModifiedByHook`

---

## F. IPC 层（UDS + NDJSON）

### F1 UDS transport
- 通过条件：
  - UDS 上可稳定收发 NDJSON
  - request_id 关联正确
- 验证方式：
  - 集成测试 `TestUDSCommandResponse`
  - 手工：`nc -U /tmp/pi-core.sock`

### F2 超时与错误响应
- 通过条件：
  - timeout 与非法命令返回标准错误
- 验证方式：
  - 单测 `TestCommandTimeout`
  - 单测 `TestInvalidCommandResponse`

### F3 Headless 运行入口
- 通过条件：
  - 无 TUI 独立运行并可被外部控制
- 验证方式：
  - 黑盒 `TestHeadlessCoreProcess`

---

## G. 最小 TUI

### G1 TUI 选型接入
- 通过条件：
  - TUI 能连接 UDS 并保持稳定
- 验证方式：
  - 手工连接测试

### G2 事件渲染闭环
- 通过条件：
  - 可展示 message/tool/status
  - 支持 `prompt/steer/follow_up/abort`
- 验证方式：
  - 手工场景脚本逐项验证

### G3 会话操作闭环
- 通过条件：
  - 可触发新建/切换会话
- 验证方式：
  - 手工测试 + 日志校验

---

## H. Provider 最小接入

### H1 单 provider 接入
- 通过条件：
  - 真实 provider 下可跑完整闭环
- 验证方式：
  - 集成测试（mock 必跑，真实 key 可选跑）

### H2 差异处理
- 通过条件：
  - 能力不支持时返回标准错误，不破坏状态机
- 验证方式：
  - 单测 `TestProviderCapabilityErrorPath`

---

## I. 端到端验收

### I1 标准链路
- 通过条件：
  - `prompt -> tool call -> tool result -> final answer`
- 验证方式：
  - E2E `TestFullAgentFlow`

### I2 steer/followUp
- 通过条件：
  - 两种队列语义均符合定义
- 验证方式：
  - E2E `TestQueueSemantics`

### I3 Session 恢复
- 通过条件：
  - 重启后可续跑
- 验证方式：
  - E2E `TestRestartAndResume`

### I4 Headless
- 通过条件：
  - 无 TUI 可完成完整 run
- 验证方式：
  - 黑盒 `TestHeadlessRun`

---

## J. 阶段门禁

### J1 Phase 1（Core）
- 门禁：
  - C/D/E 全部通过
  - `go test ./...` 全绿

### J2 Phase 2（IPC + TUI）
- 门禁：
  - F/G 通过
  - UDS 联调脚本通过

### J3 Phase 3（增强）
- 门禁：
  - I 全部通过
  - 回归测试通过
