# Nous Milestone 4 Plan

## 1. Goal

Milestone 4 goal: harden core runtime operability while keeping NDJSON protocol `v=1` stable for the separate product-TUI implementation.

Scope focus:
1. Core/runtime behavior and tooling.
2. Protocol-compatible additions only (no breaking wire changes).
3. Existing in-repo TUI remains dev MVP.

## 2. Atomic Steps

### Step M4-1: Stream-first operational hardening

Implement:
1. Add a stream-first CLI path (`corectl prompt_async`) that sends `prompt` with `wait:false`.
2. Keep existing sync prompt behavior as compatibility path.
3. Update usage/help text to make async flow explicit.

Implement for test:
1. `cmd/corectl/main_test.go`:
`prompt_async` parsing and payload assertions.
2. `internal/ipc/client_test.go`:
stream-first prompt acceptance/run-id behavior remains green.

Verified:
1. `go test ./cmd/corectl -run 'ParseArgs'`
2. `go test ./internal/ipc -run 'PromptCommandWithWaitFalse|PromptWaitFalseStreamsEventsOverEventSocket'`
3. `go test ./...`

Commit:
1. `m4-1 cli: add explicit stream-first prompt_async path`

### Step M4-2: Event trace export and deterministic replay

Implement:
1. Add IPC client utilities to capture run-scoped events from event socket.
2. Add NDJSON trace read/write helpers.
3. Add deterministic replay validator for trace ordering.
4. Add `corectl trace <run_id>` command to export trace NDJSON.
5. Document trace workflow in `docs/runtime-trace.md`.

Implement for test:
1. `internal/ipc/trace_test.go`:
trace capture + NDJSON roundtrip + replay validation.
2. `cmd/corectl/main_test.go`:
`trace` arg parsing coverage.

Verified:
1. `go test ./internal/ipc -run 'Trace|Replay'`
2. `go test ./cmd/corectl -run 'ParseArgs'`
3. `go test ./...`

Commit:
1. `m4-2 ipc/corectl: add run trace export and replay utilities`

### Step M4-3: Leaf-aware prompt execution

Implement:
1. Add optional `leaf_id` on `prompt` payload.
2. Build prompt context from `BuildMessageContextFromLeaf` when `leaf_id` provided.
3. Persist new user/assistant entries with correct parent linkage from selected leaf.
4. Preserve existing behavior when `leaf_id` absent.

Implement for test:
1. `internal/ipc/client_test.go`:
leaf-aware prompt context and parent-link persistence.
2. `internal/session/manager_test.go`:
resolved append helper and parent linkage checks.
3. Protocol docs/examples update for `prompt.leaf_id` optional field.

Verified:
1. `go test ./internal/ipc -run 'Leaf|Prompt|Session|Context'`
2. `go test ./internal/session -run 'Append|Leaf|Context'`
3. `go test ./internal/protocol -run 'Spec|Examples|Requirements'`
4. `go test ./...`

Commit:
1. `m4-3 ipc/session: add leaf-aware prompt context and persistence`

### Step M4-4: Extension timeout isolation policy

Implement:
1. Add configurable extension hook/tool timeouts in `extension.Manager`.
2. Add core flags to configure extension timeout values.
3. Isolate extension timeout failures in engine as warnings/tool_error output instead of run-stopping errors where safe.

Implement for test:
1. `internal/extension/manager_test.go`:
timeout behavior for hook/tool execution.
2. `internal/core/engine_extension_test.go`:
timeout isolation warnings and non-fatal tool timeout behavior.
3. `cmd/core/main_test.go` (or equivalent):
timeout flag parsing/validation.

Verified:
1. `go test ./internal/extension -run 'Timeout|Hook|Tool'`
2. `go test ./internal/core -run 'Extension|Timeout|Hook|Tool'`
3. `go test ./cmd/core -run 'Flag|Timeout'`
4. `go test ./...`

Commit:
1. `m4-4 core/extension: add timeout policy and isolation semantics`

### Step M4-5: Milestone 4 gate

Implement:
1. Add `scripts/phase-gate-m4.sh` and `make milestone4-gate`.
2. Gate deterministic tests for M4-1..M4-4.
3. Add optional live smoke stage controlled by env (`M4_GATE_LIVE_SMOKE=1`).

Implement for test:
1. Gate script self-validation in local runs.
2. `Makefile` target wiring.

Verified:
1. `./scripts/phase-gate-m4.sh`
2. `make milestone4-gate`
3. `M4_GATE_LIVE_SMOKE=1 ./scripts/phase-gate-m4.sh` when `OPENAI_API_KEY` is set.
4. `go test ./...`

Commit:
1. `m4-5 build: add milestone4 gate with optional live smoke`

## 3. Non-goals

1. Product-level TUI framework migration.
2. UX redesign in `cmd/tui`.
3. Breaking protocol changes to existing `v=1` command/event surface.

## 4. Done Criteria

Milestone 4 is done when:
1. Steps `M4-1` through `M4-5` are implemented and committed.
2. `go test ./...` is green.
3. `make milestone4-gate` is green.
4. Live smoke path is executable when credentials are present.
