# Nous Milestone 5 Plan

## 1. Goal

Milestone 5 goal: close the next core parity gaps vs `pi-mono` after Milestone 4, while keeping protocol `v=1` backward compatible.

Focus:
1. Core/runtime parity only.
2. No product-TUI UX work.
3. Additive protocol/API changes only.

## 2. Scope Inputs

1. `../pi-mono/packages/agent/src/agent-loop.ts`
2. `../pi-mono/packages/ai/src/*`
3. `../pi-mono/packages/coding-agent/src/core/compaction/*`
4. `../pi-mono/packages/coding-agent/src/core/agent-session.ts`
5. Current nous baseline: `docs/plan-milestone4.md`, `scripts/phase-gate-m4.sh`

## 3. Atomic Steps

### Step M5-1: Context compaction baseline

Implement:
1. Add core compaction service (`internal/core/compaction.go`) with deterministic summarization interface.
2. Add manual IPC command `compact_session` (session-scoped) with optional user instruction.
3. Persist compaction marker entry in session history.
4. Mirror pi-mono trigger semantics:
   - manual trigger (`/compact` equivalent),
   - auto trigger on threshold after successful turn (compact only, no auto-retry),
   - auto trigger on overflow error (compact then auto-retry once policy allows).
5. Keep compaction non-proactive (do not interrupt mid-turn), matching current pi-mono behavior.

Implement for test:
1. `internal/core/compaction_test.go`: deterministic summary behavior and bounds.
2. `internal/ipc/client_test.go`: `compact_session` command success/error paths.
3. `internal/session/manager_test.go`: compaction marker persistence + replay compatibility.

Verified:
1. `go test ./internal/core -run 'Compaction|Summary'`
2. `go test ./internal/ipc -run 'compact_session|Session'`
3. `go test ./internal/session -run 'Compaction|Schema|Recover'`
4. `go test ./...`

Commit:
1. `m5-1 core/ipc/session: add manual session compaction baseline`

### Step M5-2: Session tree navigation commands

Implement:
1. Add `set_leaf` command to set active branch leaf for current session.
2. Add `get_tree` command returning branchable message graph metadata (`id`, `parent_id`, role, snippet).
3. Ensure subsequent prompts use active leaf when `leaf_id` is omitted.

Implement for test:
1. `internal/ipc/client_test.go`: `set_leaf` affects prompt context selection.
2. `internal/session/manager_test.go`: active-leaf tracking and deterministic path reconstruction.
3. `internal/protocol/spec_validation_test.go`: command/response schema updates.

Verified:
1. `go test ./internal/ipc -run 'set_leaf|get_tree|Leaf|Context'`
2. `go test ./internal/session -run 'Leaf|Tree|Context'`
3. `go test ./internal/protocol -run 'Spec|Examples|Requirements'`
4. `go test ./...`

Commit:
1. `m5-2 protocol/ipc/session: add tree navigation commands and active leaf semantics`

### Step M5-3: Rich message block model

Implement:
1. Extend internal message model to support typed content blocks (text/tool/thinking).
2. Extend provider normalization boundary to preserve block typing where available.
3. Keep backward compatibility by rendering unknown blocks as text fallback.
4. Protocol stance:
   - no breaking change to protocol `v=1` required for core-internal block support,
   - if block payloads are exposed over IPC, add optional fields only (backward-compatible schema update).

Implement for test:
1. `internal/core/messages_transform_test.go`: block transform + conversion ordering.
2. `internal/provider/semantic_contract_test.go`: block parity contract checks.
3. `internal/core/engine_test.go`: fallback rendering behavior for unsupported blocks.

Verified:
1. `go test ./internal/core -run 'Message|Block|Transform|Convert'`
2. `go test ./internal/provider -run 'Semantic|Contract|Block'`
3. `go test ./...`

Commit:
1. `m5-3 core/provider: add rich message block model with backward fallback`

### Step M5-4: Provider retry/abort resilience

Implement:
1. Add retry policy for retryable provider failures with bounded delay/jitter.
2. Normalize aborted outcomes distinctly from generic errors.
3. Emit status/warning telemetry for retries and abort reasons.

Implement for test:
1. `internal/provider/http_adapters_test.go`: retryable failure mapping.
2. `internal/core/engine_error_events_test.go`: retry status + abort reason propagation.
3. `internal/ipc/event_stream_test.go`: deterministic event order under retry then success.

Verified:
1. `go test ./internal/provider -run 'Retry|Abort|Contract'`
2. `go test ./internal/core -run 'Provider|Retry|Abort|Runtime'`
3. `go test ./internal/ipc -run 'Event|Retry|Abort'`
4. `go test ./...`

Commit:
1. `m5-4 provider/core: add retry and abort resilience semantics`

### Step M5-5: Incremental tool progress contract

Implement:
1. Add optional incremental tool progress callback API in tool executor path.
2. Emit multiple `tool_execution_update` events for long-running tools.
3. Keep single-result tools unchanged.
4. Dataflow clarification:
   - progress updates are emitted on the tool-execution path (tool -> agent/core -> event stream),
   - this is not model-token streaming and does not require model/provider protocol changes.

Implement for test:
1. `internal/core/engine_tool_test.go`: multi-update tool execution sequence.
2. `internal/ipc/event_stream_test.go`: tool progress updates visible on event socket.
3. Built-in tool test(s) with synthetic progress source.

Verified:
1. `go test ./internal/core -run 'Tool|Progress|Execution'`
2. `go test ./internal/ipc -run 'tool_execution_update|Event'`
3. `go test ./...`

Commit:
1. `m5-5 core/ipc: add incremental tool progress event contract`

### Step M5-6: Milestone 5 gate

Implement:
1. Add `scripts/phase-gate-m5.sh` and `make milestone5-gate`.
2. Include parity-critical checks for M5-1..M5-5.
3. Keep optional live smoke controlled by env (`M5_GATE_LIVE_SMOKE=1`).

Implement for test:
1. Gate script deterministic run in local/CI without network.
2. Optional live smoke stage using existing `scripts/local-smoke.sh`.

Verified:
1. `./scripts/phase-gate-m5.sh`
2. `make milestone5-gate`
3. `M5_GATE_LIVE_SMOKE=1 ./scripts/phase-gate-m5.sh` (when API key available)
4. `go test ./...`

Commit:
1. `m5-6 build: add milestone5 gate with optional live smoke`

## 4. Non-goals

1. Product TUI framework selection or UX redesign.
2. Extension UI widgets parity.
3. Non-essential provider additions unrelated to parity gaps above.

## 5. Done Criteria

Milestone 5 is complete when:
1. Steps `M5-1`..`M5-6` are implemented and committed.
2. `go test ./...` is green.
3. `make milestone5-gate` is green.
4. Optional live smoke path is executable and documented.
