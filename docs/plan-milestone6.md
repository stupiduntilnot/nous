# Nous Milestone 6 Plan

## 1. Goal

Milestone 6 goal: implement true streaming communication from provider to core to IPC clients, so users see incremental assistant output instead of one late full chunk.

Architecture stance:
1. Streaming transport/parsing is provider-specific (OpenAI in this milestone).
2. Event semantics are provider-agnostic via shared `provider.Event` contract.
3. Core/IPC consume streaming events generically, independent of provider transport details.

Focus:
1. Core/runtime/protocol parity first.
2. Keep protocol `v=1` backward compatible (additive only).
3. Keep TUI minimal; only compatibility updates.

## 2. Scope Inputs

1. `internal/provider/openai.go`
2. `internal/provider/openai_compat.go`
3. `internal/core/engine.go`
4. `internal/ipc/server.go`
5. `cmd/corectl/main.go`
6. `docs/tui-protocol-v1-contract.md`

## 3. Atomic Steps

### Step M6-1: OpenAI true streaming adapter

Implement:
1. Replace one-shot OpenAI response handling with true streaming transport.
2. Parse incremental provider chunks and emit multiple `provider.EventTextDelta` events.
3. Preserve tool-call behavior while streaming (assemble partial tool-call arguments safely).
4. Preserve retry/abort semantics from Milestone 5; no hidden behavior regressions.
5. Remove or rename `internal/provider/openai_compat.go` once the new streaming adapter fully covers current behavior.
6. Keep `internal/provider/types.go` contract stable so core/IPC do not depend on OpenAI wire specifics.

Implement for test:
1. `internal/provider/http_adapters_test.go`: mock streaming endpoint and assert multiple deltas are emitted in order.
2. `internal/provider/semantic_contract_test.go`: assert event ordering remains valid (`start -> delta/tool/status -> done|error`).
3. Add negative tests for malformed stream frames and premature EOF.

Verified:
1. `go test ./internal/provider -run 'OpenAI|Stream|Retry|Abort|Contract'`
2. `go test ./internal/provider`

Commit:
1. `m6-1 provider/openai: implement true streaming event adapter`

### Step M6-2: Core incremental message runtime semantics

Implement:
1. Ensure each provider delta maps to one runtime `message_update` event while preserving final accumulated output.
2. Keep existing turn/run lifecycle invariants unchanged.
3. Keep tool loop semantics unchanged when deltas and tool calls interleave.

Implement for test:
1. `internal/core/engine_test.go`: assert multiple `message_update` events produce expected final answer text.
2. `internal/core/runtime_events_test.go`: assert legal transition sequence under high-frequency updates.
3. `internal/core/events_golden_test.go`: add/refresh streaming golden case.

Verified:
1. `go test ./internal/core -run 'Engine|Runtime|Event|MessageUpdate|Golden'`
2. `go test ./internal/core`

Commit:
1. `m6-2 core/runtime: support high-frequency incremental message updates`

### Step M6-3: IPC event-stream backpressure hardening

Implement:
1. Remove silent-drop behavior for subscriber overflow during high-rate streaming.
2. Add explicit overflow policy (block with bound, disconnect slow subscriber, or surface warning) with deterministic behavior.
3. Keep non-streaming behavior and existing event ordering guarantees.

Implement for test:
1. `internal/ipc/event_stream_test.go`: high-volume delta stream does not silently lose events under expected load.
2. `internal/ipc/server_test.go`: slow-consumer behavior follows explicit overflow policy.
3. `internal/ipc/trace_test.go`: trace capture remains valid with many deltas.

Verified:
1. `go test ./internal/ipc -run 'Event|Stream|Backpressure|Trace'`
2. `go test ./internal/ipc`

Commit:
1. `m6-3 ipc: harden event stream delivery for token streaming load`

### Step M6-4: CLI stream-first operability

Implement:
1. Keep `prompt_async` as default recommended path for long-running prompts.
2. Add a stream-follow command in `nous-ctl` (or equivalent UX) that sends async prompt and prints run events until `agent_end`.
3. Keep sync prompt for compatibility; improve timeout/error hints for long model responses.

Implement for test:
1. `cmd/corectl/main_test.go`: command parsing and stream-follow output behavior.
2. `internal/ipc/client_test.go`: async prompt + follow stream utility integration checks.
3. Optional smoke script for manual operator validation.

Verified:
1. `go test ./cmd/corectl -run 'Prompt|Async|Stream|Timeout'`
2. `go test ./internal/ipc -run 'Prompt|Stream|Trace'`
3. `go test ./cmd/corectl ./internal/ipc`

Commit:
1. `m6-4 cli/ipc: add stream-follow prompt flow and long-run timeout guidance`

### Step M6-5: Protocol/docs/examples streaming contract

Implement:
1. Document that one assistant message may produce many `message_update` events.
2. Clarify client handling expectations for partial deltas and finalization on `message_end`/`turn_end`.
3. Update NDJSON examples to include multi-delta streaming cases.

Implement for test:
1. Update protocol examples under `docs/example-protocol-events-*.ndjson`.
2. Keep `docs/protocol-openapi-like.json` backward compatible; update optional field docs only if needed.
3. Ensure protocol/doc consistency tests remain green.

Verified:
1. `go test ./internal/protocol -run 'Spec|Examples|Requirements|ADR'`
2. `go test ./internal/protocol`

Commit:
1. `m6-5 docs/protocol: codify streaming message_update contract`

### Step M6-6: Milestone 6 gate

Implement:
1. Add `scripts/phase-gate-m6.sh` and `make milestone6-gate`.
2. Gate includes M6-1..M6-5 parity-critical tests.
3. Add optional live OpenAI smoke (`M6_GATE_LIVE_SMOKE=1`) that verifies incremental streaming behavior.

Implement for test:
1. Deterministic local gate without network.
2. Optional live smoke asserts at least two `message_update` events on a long answer prompt.

Verified:
1. `./scripts/phase-gate-m6.sh`
2. `make milestone6-gate`
3. `M6_GATE_LIVE_SMOKE=1 ./scripts/phase-gate-m6.sh` (when API key available)
4. `go test ./...`

Commit:
1. `m6-6 build: add milestone6 streaming gate and optional live smoke`

## 4. Non-goals

1. Product TUI framework redesign.
2. Non-OpenAI provider expansion.
3. Breaking protocol changes (`v=2`) during this milestone.

## 5. Done Criteria

Milestone 6 is complete when:
1. Steps `M6-1`..`M6-6` are implemented and committed.
2. `go test ./...` is green.
3. `make milestone6-gate` is green.
4. Optional live smoke validates incremental streaming behavior against real OpenAI API.
