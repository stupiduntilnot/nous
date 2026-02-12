# Nous Milestone 3 Dev Plan

## 1. Purpose

This document turns `docs/plan-milestone3.md` into an execution checklist.

Execution rule:
1. One atomic step per round.
2. Implement -> test -> fix -> test.
3. Commit only after the step passes.
4. Keep TUI work minimal in Milestone 3 (no UX redesign).

## 2. Scope Guardrails

1. Milestone 3 focus is core parity (`internal/core`, `internal/ipc`, `internal/provider`, `internal/session`, `internal/extension`, protocol docs/tests).
2. TUI changes are allowed only when needed to keep compatibility with core/protocol changes.
3. No broad TUI UX optimization in this milestone.

## 3. Current Status

Completed from Milestone 2 (baseline):
1. Non-blocking prompt + live event stream.
2. Structured provider message request contract (legacy fields removed).
3. Session schema v2 typed message decoding.
4. Extension lifecycle hooks (`run_start`, `run_end`) with warning isolation.
5. TUI stream-first path + pending/progress visibility.

Milestone 3 pending:
1. A1 single-run lifecycle parity.
2. A2 mid-tool steering interruption parity.
3. A3 queue mode parity.
4. A4 async prompt context parity.
5. B1 typed message boundary (`transformContext`/`convertToLlm`).
6. B2 provider stream normalization v3.
7. C1 session tree semantics roadmap step.
8. C2 extension hook surface parity.
9. D1 RPC/core state read commands.
10. D2 milestone gate (`phase-gate-m3`).

## 4. Atomic Steps

### Step M3-A1: Single-run lifecycle coordinator

Scope:
1. Ensure one logical run emits exactly one `agent_start` and one `agent_end`.
2. Keep multi-turn behavior for prompt + queued messages inside the same run.

Unit tests to implement/update:
1. `internal/core/command_loop_test.go`: one logical run with queued `steer`/`follow_up` emits one run lifecycle.
2. `internal/core/runtime_events_test.go`: event sequence preserves turn boundaries under one run.
3. `internal/ipc/client_test.go`: async prompt + injected control commands still produce single run lifecycle.

Validation:
1. `go test ./internal/core -run 'Run|CommandLoop|Event'`
2. `go test ./internal/ipc -run 'Async|RunControl|Event'`
3. `go test ./...`

Commit message:
1. `m3-a1 core: enforce single-run lifecycle across queued turns`

### Step M3-A2: Mid-tool steer interruption semantics

Scope:
1. Poll steer queue between tool executions in a turn.
2. Skip remaining tool calls when steer arrives; emit synthetic skipped tool results.

Unit tests to implement/update:
1. `internal/core/engine_tool_test.go`: multi-tool assistant turn interrupted by steer skips remaining calls.
2. `internal/core/events_semantic_replay_test.go`: replay fixture includes skipped tool result semantics.
3. `internal/ipc/client_test.go`: mid-run steer timing test validates deterministic interrupt ordering.

Validation:
1. `go test ./internal/core -run 'Tool|Steer|Loop|Replay'`
2. `go test ./internal/ipc -run 'steer|follow|abort|order'`
3. `go test ./...`

Commit message:
1. `m3-a2 core: add mid-tool steer interruption and skipped tool results`

### Step M3-A3: Queue mode parity (`one-at-a-time` / `all`)

Scope:
1. Add steer/follow-up queue mode state in core.
2. Add IPC commands to set/read queue modes.

Unit tests to implement/update:
1. `internal/core/command_loop_test.go`: dequeue semantics for both modes and both queues.
2. `internal/ipc/command_coverage_test.go`: new mode commands parsed and dispatched.
3. `internal/ipc/client_test.go`: mode switches affect observed turn consumption.

Validation:
1. `go test ./internal/core -run 'Queue|Steer|Follow'`
2. `go test ./internal/ipc -run 'set_steering_mode|set_follow_up_mode|get_state'`
3. `go test ./...`

Commit message:
1. `m3-a3 ipc/core: add steer and follow-up queue mode controls`

### Step M3-A4: Async prompt session-context parity

Scope:
1. Ensure `prompt(wait:false)` builds context with session history consistently.
2. Align async/sync prompt context semantics.

Unit tests to implement/update:
1. `internal/ipc/client_test.go`: async prompt carries prior session history in model input.
2. `internal/ipc/headless_process_test.go`: sync/async context equivalence coverage.
3. `internal/session/manager_test.go`: context reconstruction remains deterministic across branch/switch.

Validation:
1. `go test ./internal/ipc -run 'AsyncPrompt|Session|Switch|Context'`
2. `go test ./internal/session -run 'Context|Recover|Build'`
3. `go test ./...`

Commit message:
1. `m3-a4 ipc/session: align async prompt with session context reconstruction`

### Step M3-B1: Typed message boundary + transforms

Scope:
1. Introduce typed message pipeline in core.
2. Add `transformContext` then `convertToLlm` boundary before provider call.

Unit tests to implement/update:
1. `internal/core/engine_test.go` and/or new `internal/core/messages_transform_test.go`: transform then convert order is enforced.
2. `internal/core/engine_extension_test.go`: custom/internal messages do not leak to provider unless converter maps them.
3. `internal/provider/request_format_test.go`: provider request built strictly from converted messages.

Validation:
1. `go test ./internal/core -run 'Message|Transform|Convert|Engine'`
2. `go test ./internal/provider -run 'Request|Format|Semantic'`
3. `go test ./...`

Commit message:
1. `m3-b1 core/provider: add transform and llm-conversion message boundary`

### Step M3-B2: Provider normalization v3

Scope:
1. Normalize OpenAI/Gemini stream contract (tool calls, stop reason, usage surface).
2. Keep provider-specific parsing isolated to adapters.

Unit tests to implement/update:
1. `internal/provider/semantic_contract_test.go`: stop reason + usage parity assertions.
2. `internal/provider/http_adapters_test.go`: adapter-specific parsing maps to normalized events.
3. `internal/core/engine_error_events_test.go`: normalized provider errors/warnings propagate consistently.

Validation:
1. `go test ./internal/provider -run 'Semantic|OpenAI|Gemini|Contract'`
2. `go test ./internal/core -run 'Provider|Runtime|Parity'`
3. `go test ./...`

Commit message:
1. `m3-b2 provider: normalize stream contract with stop reason and usage`

### Step M3-C1: Session tree semantics step

Scope:
1. Add entry identity and parent linkage for branchable context traversal.
2. Add migration-safe decoding from existing sessions.

Unit tests to implement/update:
1. `internal/session/manager_test.go`: tree-linked traversal by leaf id.
2. `internal/session/entries_test.go`: decode compatibility for legacy + new schema.
3. `internal/ipc/client_test.go`: branch/switch continuation reads correct path context.

Validation:
1. `go test ./internal/session -run 'Schema|Migration|Branch|Context|Recover'`
2. `go test ./internal/ipc -run 'session|branch|switch'`
3. `go test ./...`

Commit message:
1. `m3-c1 session: add tree-linked entries with migration compatibility`

### Step M3-C2: Extension lifecycle parity surface

Scope:
1. Add missing core lifecycle hooks (`before_agent_start`, `turn_start`).
2. Add session pre-hooks (`session_before_switch`, `session_before_fork`) with cancellation semantics.

Unit tests to implement/update:
1. `internal/extension/manager_test.go`: new hooks registration/order/isolation.
2. `internal/core/engine_extension_test.go`: run/turn hook invocation around real turns.
3. `internal/ipc/client_test.go`: session pre-hook cancellation blocks switch/fork safely.

Validation:
1. `go test ./internal/extension -run 'Hook|Order|Isolation|Session'`
2. `go test ./internal/core -run 'Extension|Hook|Lifecycle'`
3. `go test ./internal/ipc -run 'session_before|switch|fork'`
4. `go test ./...`

Commit message:
1. `m3-c2 extension/ipc: add lifecycle and session pre-hook parity`

### Step M3-D1: RPC parity read surface

Scope:
1. Add `get_state` and `get_messages` commands.
2. Include queue modes and pending counts in state response.
3. Update protocol docs/examples/tests.

Unit tests to implement/update:
1. `internal/ipc/command_coverage_test.go`: command routing for `get_state` and `get_messages`.
2. `internal/ipc/client_test.go`: response payload schema assertions for state/messages.
3. `internal/protocol/spec_validation_test.go` + `internal/protocol/examples_validation_test.go`: schema/example parity.

Validation:
1. `go test ./internal/protocol -run 'Spec|Examples|Requirements'`
2. `go test ./internal/ipc -run 'get_state|get_messages|command_coverage'`
3. `go test ./...`

Commit message:
1. `m3-d1 protocol/ipc: add get_state and get_messages parity commands`

### Step M3-D2: Milestone 3 gate

Scope:
1. Add `scripts/phase-gate-m3.sh` and `make milestone3-gate`.
2. Gate all parity-critical tests from A1..D1.

Validation:
1. `./scripts/phase-gate-m3.sh`
2. `make milestone3-gate`
3. `go test ./...`

Commit message:
1. `m3-d2 build: add milestone3 parity gate script and make target`

## 5. Step Template (repeat per round)

1. Implement one step only.
2. Run targeted tests for touched packages.
3. Fix failures.
4. Run `go test ./...`.
5. Commit with step-specific message.
6. Proceed to next step.

## 6. Definition of Done

Milestone 3 done means:
1. Steps M3-A1 through M3-D2 are complete and committed.
2. `go test ./...` is green.
3. `make milestone3-gate` is green.
4. Core parity goals are met without broad TUI UX work.

## 7. Live API Verification (required)

Policy:
1. Unit/integration tests are required for every step.
2. For steps touching provider/runtime behavior (minimum: `M3-A4`, `M3-B1`, `M3-B2`, `M3-D2`), run a real OpenAI smoke check when `OPENAI_API_KEY` is available.

Required live check command:
1. `./scripts/local-smoke.sh`

Equivalent manual flow (if script needs debugging):
1. Launch core with OpenAI provider.
2. Send a real prompt with `corectl` (or `coreutil` alias if configured).
3. Verify successful output and `session_id` in response.

Merge gate expectation:
1. PR/merge-ready state requires both `go test ./...` and live OpenAI smoke evidence (or explicit note why unavailable).
