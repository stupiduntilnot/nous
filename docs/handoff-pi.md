# handoff-pi: pi-mono technical handoff

This document is a deeper dump of what matters in `../pi-mono`, intended for implementation and parity work.
It complements `docs/study-pi.md`, which is intentionally high-level.

## 1. Repo shape and package roles

`pi-mono` is a TypeScript monorepo. The core packages for runtime semantics are:

- `../pi-mono/packages/ai`
  - Unified provider/model layer and streaming event API (`stream`, `streamSimple`).
  - Defines shared message and event types used by upper layers.
- `../pi-mono/packages/agent`
  - Core agent runtime loop (`Agent`, `agentLoop`, tool execution, queue semantics).
  - Emits lifecycle events for UI and persistence consumers.
- `../pi-mono/packages/coding-agent`
  - Product shell around `Agent` with session manager (JSONL tree), extension system and hooks, built-in coding tools, and interactive/print/RPC modes.

Other packages (`tui`, `web-ui`, `mom`, `pods`) are adjacent products/libraries, not the core semantic source for coding-agent behavior.

## 2. One-minute runtime model

Primary flow for coding-agent:

1. User input enters `AgentSession.prompt()`.
2. `AgentSession` may expand slash skills/templates and route streaming-time inputs into steer/follow-up queues.
3. `Agent.prompt()` (from `packages/agent`) starts `_runLoop`.
4. `agentLoop()` drives turns:
   - stream assistant response,
   - execute tool calls sequentially,
   - inject tool results,
   - poll steering/follow-up queues,
   - converge only when no tool calls and no queued work.
5. `AgentSession` listens to emitted events and persists them into session JSONL tree entries.

Key files:

- `../pi-mono/packages/agent/src/agent.ts`
- `../pi-mono/packages/agent/src/agent-loop.ts`
- `../pi-mono/packages/coding-agent/src/core/agent-session.ts`

## 3. Agent loop semantics (source truth)

`agentLoop` is the main semantic anchor.

Files:
- `../pi-mono/packages/agent/src/agent-loop.ts`
- `../pi-mono/packages/agent/src/types.ts`

Important behavior:

1. Lifecycle events:
- Starts with `agent_start`, then `turn_start`.
- Emits `message_start/update/end` for assistant streaming.
- Emits `tool_execution_start/update/end` for each tool call.
- Emits `turn_end` after assistant + tool phase.
- Emits `agent_end` only on convergence or terminal error/abort.

2. Turn structure:
- One turn corresponds to one assistant message plus zero or more tool results.
- `turn_end` includes `{ message, toolResults }`.

3. Tool execution:
- Tool calls in one assistant message are executed in order.
- Each tool result is emitted as a `toolResult` message (`message_start/end`) and appended to context.

4. Convergence:
- Inner loop continues while:
  - assistant produced tool calls, or
  - pending steering messages exist.
- Outer loop then checks follow-up queue.
- Agent stops only when both loops have no pending work.

## 4. Queue semantics (steer/follow-up) in detail

Queue logic is distributed across:

- `../pi-mono/packages/agent/src/agent.ts` (queue storage + mode)
- `../pi-mono/packages/agent/src/agent-loop.ts` (poll timing + control flow)
- `../pi-mono/packages/coding-agent/src/core/agent-session.ts` (user-facing behavior)

Semantics:

1. Steering:
- High priority, intended to interrupt current run progress.
- Checked:
  - before loop start,
  - after each turn,
  - and critically after each tool execution inside a multi-tool assistant message.
- If steering appears mid multi-tool batch, remaining tool calls are skipped and converted to synthetic error tool results (`Skipped due to queued user message.`).

2. Follow-up:
- Lower priority, checked only when loop would otherwise stop.
- If available, injected and processing continues with new turn(s).

3. Modes:
- `one-at-a-time` (default): dequeue one queued message per poll.
- `all`: dequeue the whole queue at once.
- Configured separately for steering and follow-up.

## 5. Message model and LLM boundary

`Agent` intentionally separates app-level message space from provider-compatible LLM message space.

Files:
- `../pi-mono/packages/agent/src/types.ts`
- `../pi-mono/packages/coding-agent/src/core/messages.ts`
- `../pi-mono/packages/agent/src/agent-loop.ts`

Concepts:

1. `AgentMessage` can include non-LLM messages (custom app messages).
2. Before each model call:
- optional `transformContext(messages)` runs on `AgentMessage[]`,
- required `convertToLlm(messages)` maps to provider-ready `Message[]`.
3. This allows custom extension messages/UI messages without polluting provider payloads.

## 6. Tool model and built-in tool set

Files:
- `../pi-mono/packages/agent/src/types.ts` (`AgentTool`)
- `../pi-mono/packages/coding-agent/src/core/tools/index.ts`
- `../pi-mono/packages/coding-agent/src/core/sdk.ts`

Facts:

1. Default coding tool set is `[read, bash, edit, write]`.
2. Additional built-ins include `grep`, `find`, `ls`.
3. Tool execution supports streaming partial updates via callback, surfaced as `tool_execution_update`.
4. Active tools are controlled in coding-agent runtime and affect what is exposed in model context/system prompt construction.

## 7. AgentSession: orchestration layer

`AgentSession` is the most important file in coding-agent.

File:
- `../pi-mono/packages/coding-agent/src/core/agent-session.ts`

Responsibilities:

1. Prompt handling:
- slash command execution,
- skill and prompt-template expansion,
- input event interception by extensions,
- streaming-time queue behavior selection (`steer` or `followUp`).

2. Event-driven state handling:
- subscribes to `Agent` events,
- tracks queued message display state,
- persists session entries,
- coordinates compaction and retry logic.

3. Runtime wiring:
- constructs model registry/settings/session manager/resource loader integration,
- builds tool registry and extension wrapping pipeline.

## 8. RPC mode contract (headless interface)

Files:
- `../pi-mono/packages/coding-agent/docs/rpc.md`
- `../pi-mono/packages/coding-agent/src/modes/rpc/rpc-mode.ts`
- `../pi-mono/packages/coding-agent/src/modes/rpc/rpc-types.ts`

Key points:

1. Transport: JSON lines over stdin/stdout.
2. Responses:
- immediate command response envelope (`type: "response"`),
- async runtime events stream separately.
3. `prompt` while streaming requires explicit `streamingBehavior`:
- `steer` or `followUp`.
4. Dedicated queue commands exist:
- `steer`, `follow_up`, `abort`.
5. RPC exposes more control than minimal nous IPC today (model ops, settings ops, queue mode ops, etc.).

## 9. Session persistence model

Files:
- `../pi-mono/packages/coding-agent/docs/session.md`
- `../pi-mono/packages/coding-agent/src/core/session-manager.ts`

Model:

1. JSONL file with versioned format (current v3).
2. Tree semantics via `id` + `parentId` on entries.
3. Supports in-place branching and tree navigation.
4. Message entries include extended message types beyond plain user/assistant/toolResult.
5. Additional entry types capture model changes, thinking-level changes, compaction, branch summary, labels, custom extension state.

## 10. Extension system surface

Files:
- `../pi-mono/packages/coding-agent/src/core/extensions/types.ts`
- `../pi-mono/packages/coding-agent/src/core/extensions/runner.ts`
- `../pi-mono/packages/coding-agent/src/core/extensions/wrapper.ts`

High-value hooks/events:

- `input`
- `before_agent_start`
- `tool_call`
- `tool_result`
- `turn_start`
- `turn_end`
- session lifecycle hooks such as `session_before_switch`, `session_before_fork`, `session_before_compact`, `session_before_tree`.

Extensions can:

1. register tools and commands,
2. mutate or block tool calls/results,
3. intercept and transform input,
4. contribute custom messages and UI integrations.

## 11. Provider abstraction and streaming API

Files:
- `../pi-mono/packages/ai/src/types.ts`
- `../pi-mono/packages/ai/src/stream.ts`

Core ideas:

1. Provider-specific adapters normalize into common assistant streaming events.
2. `streamSimple` is the main entry used by `packages/agent`.
3. Unified content model includes text/thinking/toolCall blocks, usage accounting, and stop reasons.
4. OAuth + API-key providers both supported through model registry and auth storage in coding-agent.

## 12. Suggested reading order for new contributors

For core semantics first:

1. `../pi-mono/packages/agent/src/types.ts`
2. `../pi-mono/packages/agent/src/agent-loop.ts`
3. `../pi-mono/packages/agent/src/agent.ts`
4. `../pi-mono/packages/agent/test/agent-loop.test.ts`

Then product orchestration:

1. `../pi-mono/packages/coding-agent/src/core/agent-session.ts`
2. `../pi-mono/packages/coding-agent/src/core/session-manager.ts`
3. `../pi-mono/packages/coding-agent/src/core/extensions/*`
4. `../pi-mono/packages/coding-agent/src/modes/rpc/*`

Then provider internals:

1. `../pi-mono/packages/ai/src/types.ts`
2. `../pi-mono/packages/ai/src/stream.ts`
3. provider implementations under `../pi-mono/packages/ai/src/providers/*`

## 13. Parity-oriented notes for nous

If using pi-mono as semantic reference:

1. Do not model steer/follow-up as only run-boundary queue checks.
- pi-mono checks steering after each tool execution and can skip remaining tool calls in the same assistant message.

2. Preserve explicit AgentMessage -> LLM Message conversion boundary.
- This is important for extension/custom message types.

3. Keep session tree semantics in mind if aiming for coding-agent parity.
- pi-mono sessions are not simple linear transcript files.

4. Distinguish transport parity from runtime semantic parity.
- pi-mono RPC transport differs from nous UDS+NDJSON; semantic equivalence does not require identical transport.

---

If this handoff becomes stale, refresh it by diffing:

- `../pi-mono/packages/agent/src/*`
- `../pi-mono/packages/coding-agent/src/core/*`
- `../pi-mono/packages/coding-agent/src/modes/rpc/*`
- `../pi-mono/packages/ai/src/*`
