# TUI Integration Contract: NDJSON Protocol v1 (Frozen)

## 1. Scope

This document freezes the wire contract for product-TUI work against `nous` core.

Status:
1. Protocol version is `v=1`.
2. Transport is `UDS + NDJSON`.
3. Command/event names and required payload keys are frozen for Milestone 4.

Source of truth:
1. `internal/protocol/types.go`
2. `docs/protocol-openapi-like.json`

## 2. Stream-First Runtime Contract

TUI must use stream-first flow as default:
1. Send `prompt` with `{"wait": false}`.
2. Read runtime events from the event socket.
3. Drive run/turn/message/tool UX from events, not sync response payload.

Sync `prompt(wait:true)` is compatibility fallback only and should not be the primary UI path.

## 3. Frozen Command Surface

Core commands required by product TUI:
1. `prompt`
2. `steer`
3. `follow_up`
4. `abort`
5. `get_state`
6. `get_messages`
7. `new_session`
8. `switch_session`
9. `branch_session`
10. `set_active_tools`
11. `set_steering_mode`
12. `set_follow_up_mode`
13. `extension_command`
14. `ping`

Required payloads and response payload requirements are locked in `docs/protocol-openapi-like.json`.

## 4. Event Semantics TUI Must Respect

Lifecycle invariants:
1. One logical run emits one `agent_start` and one `agent_end`.
2. One run may contain multiple turns (`turn_start`/`turn_end`) due to queued steer/follow-up.
3. Message stream uses `message_start` -> `message_update` -> `message_end`.
4. Tool stream uses `tool_execution_start` -> `tool_execution_update` -> `tool_execution_end`.

Queue/runtime semantics:
1. `steer` has priority over `follow_up` for next turn dequeue.
2. Queue modes (`one-at-a-time` / `all`) affect how queued text is consumed.
3. Mid-tool steer may cause skipped tool calls in current assistant message.

## 5. State and History Read APIs

`get_state` is required for runtime HUD and reconnect recovery:
1. `run_state`, `run_id`, `session_id`
2. `steering_mode`, `follow_up_mode`
3. `pending_counts.steer`, `pending_counts.follow_up`

`get_messages` is required for session transcript/state restore:
1. Default active session if `session_id` omitted.
2. Optional `leaf_id` for branch-path retrieval.

## 6. TUI Compatibility Rules

1. Treat unknown payload fields as forward-compatible extras.
2. Do not assume sync `result.events` is present in stream-first operation.
3. Correlate run-scoped UI state via `run_id`.
4. Show error envelopes (`ok=false`) with `error.code`, `error.message`, optional `error.cause`.
5. File-tool relative paths are resolved by core against its startup `--workdir`, not client process cwd.
6. For cross-process consistency, TUI should send absolute paths or ensure core `--workdir` is explicitly configured.

## 7. Product-TUI Acceptance Checklist

1. Async prompt path (`wait:false`) is default and fully functional.
2. Real-time rendering from event stream covers run/turn/message/tool lifecycle.
3. `steer`, `follow_up`, `abort` controls work during active run.
4. Queue mode controls are user-visible and applied via IPC commands.
5. Session controls (`new/switch/branch`) work and preserve context behavior.
6. Reconnect path can recover UI state using `get_state` and `get_messages`.
7. Contract tests or fixtures validate wire shapes against `docs/protocol-openapi-like.json`.
