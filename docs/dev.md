# Go 版 Pi Agent 开发计划（开发 + 验收一体）

本文是唯一执行文档：每个步骤都包含两部分。
1. `任务`：要实现什么
2. `验收`：如何判定完成（命令/测试/人工检查）

当前原则：
- 先工程初始化，再做联通原型，再做 Core 主能力
- 协议固定为 `UDS + NDJSON`
- 实现顺序按 `P0 > P1 > P2`

---

## A. 工程初始化（必须先完成）

### A0. 本地模型运行时准备（推荐）
- 任务：
  - 安装本地推理服务 `Ollama`（开发默认）
  - 拉取至少一个本地模型（如 `qwen2.5-coder:7b`）
- 验收：
  - `ollama --version` 可执行
  - `ollama pull qwen2.5-coder:7b` 成功
  - `ollama list` 中可见已下载模型

### A1. 初始化 Go module 与目录结构
- 任务：
  - 初始化 `go.mod`
  - 创建目录：
    - `cmd/core`
    - `internal/core`
    - `internal/ipc`
    - `internal/protocol`
    - `internal/session`
    - `internal/extension`
    - `internal/provider`
    - `internal/tui`
- 验收：
  - `go env GOMOD` 指向当前项目 `go.mod`
  - `go build ./...` 通过
  - `go test ./...` 可运行（即使暂无测试）

### A2. 建立统一命令入口
- 任务：
  - 增加 `Makefile`（至少 `build/test/lint`）
  - 接入 `go vet`
- 验收：
  - `make build` 成功
  - `make test` 成功
  - `make lint` 成功

### A3. 定义错误与日志基本规范
- 任务：
  - 统一错误结构（`code/message/cause`）
  - 统一日志上下文字段（`run_id/turn_id/message_id/tool_call_id`）
- 验收：
  - 最少 1 个单测覆盖错误序列化
  - 运行最小程序时日志字段可见

---

## B. 协议定义（先定契约再写逻辑）

### B1. 固化协议 ADR（UDS + NDJSON）
- 任务：
  - 在文档中明确：
    - transport 仅 `UDS`
    - wire protocol 为 `NDJSON`（一行一个 JSON）
    - 本期不实现 `stdio`
- 验收：
  - `docs/req.md` 与本文一致，无冲突描述

### B2. 产出 OpenAPI 风格协议规范（语义对齐 pi-mono）
- 任务：
  - 新增协议规范文件（YAML/JSON）
  - 定义命令/响应/事件 schema
  - 编写语义对齐矩阵：逐项映射 `pi-mono` 语义
  - 标记差异边界（若有）
- 验收：
  - 存在可机器读取规范文件
  - 存在对齐矩阵
  - 协议样例可通过 schema 校验测试
  - 语义兼容测试通过（样例回放事件序列一致）

### B3. 定义命令与事件最小集合
- 任务：
  - 命令：`ping`, `prompt`, `steer`, `follow_up`, `abort`, `set_active_tools`, `new_session`, `switch_session`, `branch_session`
  - 事件：
    - `agent_start/agent_end`
    - `turn_start/turn_end`
    - `message_start/message_update/message_end`
    - `tool_execution_start/tool_execution_update/tool_execution_end`
    - `status/warning/error`
  - 所有消息包含版本字段与请求关联字段
- 验收：
  - 协议单测：
    - 合法 payload 可解析
    - 非法 payload 返回标准错误
  - 事件 Golden tests 通过

---

## C. 联通性 MVP（在 Core 复杂功能前完成）

### C1. 最小 Core：UDS + NDJSON + ping/pong
- 任务：
  - Core 监听 UDS 路径
  - 收到 `ping` 返回 `pong`
- 验收：
  - 集成测试：`TestCorePingPong`
  - 手工：`echo '{"id":"1","type":"ping"}' | nc -U /tmp/pi-core.sock`

### C2. 最小 CLI client（非 TUI）
- 任务：
  - 实现 `corectl ping --socket <path>`
- 验收：
  - 黑盒测试：`TestCorectlPing`
  - 手工：命令行输出 `pong`

### C3. 最小 TUI MVP（仅联调）
- 任务：
  - 一个输入框 + 一个日志区域
  - 输入 `ping`，显示 `pong`
  - 显示连接状态
- 验收：
  - 人工联调通过（Core+TUI）
  - 保留截图或录屏作为验收证据

### C4. 联通阶段门禁
- 任务：
  - 汇总 C1~C3
- 验收：
  - `go test ./...` 全绿
  - Core + CLI + TUI 都能完成 ping-pong

---

## D. Core P0：状态机与运行语义

### D1. Run/Turn 状态机骨架（单写者）
- 任务：
  - 主循环单 goroutine 推进状态
  - 状态最小集：`idle/running/aborting`
- 验收：
  - 状态迁移单测通过（合法与非法路径）

### D2. 消息生命周期事件
- 任务：
  - 实现 `message_start/update/end`（assistant 支持 update）
- 验收：
  - 事件顺序断言测试通过

### D3. ProviderAdapter mock 接入
- 任务：
  - 先接入 fake provider，支持返回文本与 tool call
- 验收：
  - 不依赖真实 API 的 E2E 可跑通一轮

### D3.5 本地模型 Provider 接入（开发默认）
- 任务：
  - 增加 OpenAI-compatible provider 接入能力（用于 Ollama/LM Studio/vLLM）
  - Core 支持 `provider/model/api-base` 启动参数
  - 补充本地模型接入文档（`docs/local-model.md`）
- 验收：
  - 本地模型服务启动后，`corectl prompt` 可拿到模型响应
  - 不依赖云端 API key 也可完成开发链路联调（占位 key 允许）

### D4. 顺序 Tool loop
- 任务：
  - 读取 assistant tool calls 并顺序执行
  - 回灌 `ToolResultMessage` 后进入下一 turn
- 验收：
  - E2E：`prompt -> toolcall -> toolresult -> next turn` 通过

### D5. steer / follow_up / abort 语义
- 任务：
  - `steer`：中途纠偏
  - `follow_up`：尾部排队
  - `abort`：取消传播与状态复位
- 验收：
  - 三类语义各有单测
  - 并发情境下无顺序漂移

### D6. active tools 机制
- 任务：
  - 分离 `tool registry` 与 `active tools`
  - 模型请求只发送 active 集
- 验收：
  - 单测验证 payload 工具集合准确

---

## E. Core P0：Session 与 Extension

### E1. Session JSONL 与恢复
- 任务：
  - append-only 持久化
  - 损坏行容错恢复
  - `new_session/switch_session`
- 验收：
  - 文件读写/恢复/切换测试通过

### E2. Session 分支最小能力
- 任务：
  - 基于 `id/parent_id` 支持从历史继续
- 验收：
  - 分支上下文构建测试通过

### E3. Extension MVP
- 任务：
  - 支持注册 Tool/Command/Hook
  - 必须支持 hooks：`input/tool_call/tool_result/turn_end`
- 验收：
  - 注册与调用测试通过
  - `input transform/handled` 测试通过
  - `tool_call block`、`tool_result mutate` 测试通过

---

## F. P1：IPC 层完善

### F1. UDS 命令-响应链路
- 任务：
  - request_id 关联
  - 错误返回标准化
- 验收：
  - UDS 集成测试通过

### F2. timeout 与异常处理
- 任务：
  - 命令级超时
  - 非法命令/内部错误稳定返回
- 验收：
  - timeout 与错误路径单测通过

### F3. Headless 启动入口
- 任务：
  - 可独立运行 Core（无 TUI）
- 验收：
  - 黑盒进程测试通过

---

## G. P2：最小 TUI（仅验证 Core）

### G1. 事件渲染最小闭环
- 任务：
  - 展示 message/tool/status
  - 输入支持 `prompt/steer/follow_up/abort`
- 验收：
  - 人工场景脚本逐项通过

### G2. 会话操作最小闭环
- 任务：
  - 支持新建/切换会话
- 验收：
  - 人工验证 + 日志验证通过

---

## H. 阶段门禁（发布前检查）

### H1. Phase 1（Core）
- 验收：
  - D + E 全部通过
  - `go test ./...` 通过

### H2. Phase 2（IPC + TUI）
- 验收：
  - F + G 全部通过
  - UDS 联调脚本通过

### H3. 回归门禁
- 验收：
  - 全量回归通过
  - 文档与实现一致（协议无漂移）
