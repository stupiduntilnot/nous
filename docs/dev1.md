# Builtin Tools 开发计划（pi-mono 风格）

目标：按 `pi-mono` 的 builtin 工具语义，实现 `read/bash/edit/write/grep/find/ls`，并保持“每个工具一个独立提交（实现+测试）”。

约束：
- 传输层保持现有 `UDS + NDJSON`，不变。
- Tool 调用入口保持现有 Core tool loop，不改协议。
- 默认启用全部 builtin tools；仍可通过 `set_active_tools` 收敛可见集合。

## Step 0: 基线与契约
- 任务：
  - 在文档中固定 builtin tool 名称与最小参数契约（先最小可用，再迭代）。
  - 增加工具注册入口（Core 启动时注册 builtin tools）。
- 验收：
  - `go test ./...` 全绿。
  - 启动 Core 后执行 `set_active_tools read` 不报 `tool_not_registered`。

## Step 1: `read` tool（先做）
- 任务：
  - 新增 `read`：读取文本文件。
  - 参数：`path`(required), `offset`(optional), `limit`(optional)。
  - 行为：支持相对/绝对路径、偏移与行数限制，错误返回标准化字符串错误。
- 验收：
  - 单测覆盖：
    - 读取成功
    - 相对路径解析
    - offset/limit 生效
    - 文件不存在报错
    - 目录路径报错
  - 引擎集成测试覆盖一次 `tool_call(name=read)` 成功路径。

## Step 2: `ls` tool
- 任务：实现目录列举（最小字段：name/type/size）。
- 验收：
  - 单测覆盖空目录、普通文件、子目录、不存在路径。
  - 引擎工具调用测试通过。

## Step 3: `find` tool
- 任务：实现递归查找（基于 glob/子串，先做子串）。
- 验收：
  - 单测覆盖深度限制、命中/未命中、非法参数。

## Step 4: `grep` tool
- 任务：实现文本搜索（按行返回命中，含行号）。
- 验收：
  - 单测覆盖大小写、多命中、无命中、二进制文件保护。

## Step 5: `write` tool
- 任务：实现文件写入（覆盖/新建，最小模式）。
- 验收：
  - 单测覆盖新建、覆盖、父目录不存在。

## Step 6: `edit` tool
- 任务：实现最小编辑（基于 old/new 替换）。
- 验收：
  - 单测覆盖替换成功、old 不存在、重复匹配冲突。

## Step 7: `bash` tool
- 任务：实现受限命令执行（超时、工作目录、stdout/stderr 截断）。
- 验收：
  - 单测覆盖成功、超时、非零退出码、输出截断。

## Step 8: 文档与端到端验收
- 任务：
  - 更新 `docs/req.md`/`docs/dev.md`（若需要）与手动测试文档。
  - 增加脚本验证典型工具链：`read -> grep -> edit -> write`。
- 验收：
  - `make test` 通过。
  - 手动 smoke 通过并保留日志到 `artifacts/`。

## 提交策略
- 每个 step 至少 1 个 commit。
- commit 模板：
  - `feat(tools): add <tool> builtin`
  - `test(tools): cover <tool> edge cases`
  - 若同一步改动小，可合并为一个 commit：`feat(tools): add <tool> builtin with tests`
