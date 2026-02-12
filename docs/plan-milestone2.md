# Nous Milestone 2 Plan

## 1. Scope and inputs

This plan is derived from:

1. Nous Milestone 1 requirements and execution docs:
`docs/req.md`, `docs/dev-milestone1.md`, `docs/protocol-pi-mono-semantic-matrix.md`.
2. pi-mono runtime behavior and architecture:
`docs/study-pi.md`, `docs/handoff-pi.md`, and `../pi-mono/packages/{ai,agent,coding-agent}`.

## 2. Updated milestone goal

Milestone 2 goal is to reach semantics-first parity on runtime control and state modeling.

Important clarification:
1. Prompt behavior target is pi-mono RPC style: command response returns immediately, events stream asynchronously.
2. This does not require the internal core `Prompt()` function signature to be async fire-and-forget.
3. The required outcome is transport/runtime controllability during active runs (`steer`, `follow_up`, `abort`), with deterministic event flow.

## 3. Priority order

### P0.1 IPC run control and live event streaming

Problem:
1. Current IPC prompt path is sync/aggregated (returns final output in command response).
2. This prevents true mid-run control semantics.

Deliver:
1. `prompt` behaves as non-blocking command and returns immediate accepted response (pi-mono RPC style).
2. Live event stream channel for run lifecycle events.
3. Correlation fields for request/run/turn.
4. Mid-run `steer`, `follow_up`, `abort` works reliably.

Exit criteria:
1. Integration tests for concurrent command injection while run is active.
2. E2E script: prompt(non-blocking) -> steer -> follow_up -> abort.

### P0.2 Core loop v2 with structured messages

Problem:
1. Current loop still depends on prompt-string patching (`Tool results:`).
2. This is brittle and limits parity/extensibility.

Deliver:
1. Internal typed message model (`user`, `assistant`, `tool_result` minimum).
2. Structured reinjection of tool results into next turn context.
3. Remove prompt-string augmentation as primary loop mechanism.
4. Preserve single-writer deterministic state progression.

Exit criteria:
1. Tests verify tool_result reinjection without string concatenation.
2. Golden/replay event tests remain stable.

### P0.3 Provider adapter v2 normalization

Problem:
1. Current provider request contract (`Prompt`, `ToolResults []string`) is too coarse.
2. Core behavior risks provider-specific leakage.

Deliver:
1. Structured provider request contract (messages/tools/settings).
2. Normalized provider event surface (`text_delta`, `tool_call`, `done`, `error`).
3. OpenAI first, then Gemini on same normalized contract.
4. Stable adapter API for future codex/claude work.

Exit criteria:
1. Cross-adapter semantic contract tests (mock/openai/gemini).
2. Provider parsing differences isolated in adapter tests.

### P1.1 Session model upgrade

Deliver:
1. Session records include typed messages and run metadata.
2. Branch/switch/new preserve reconstructable context semantics.
3. Corrupt-line tolerant recovery remains.

Exit criteria:
1. Migration-safe tests from M1 sessions.
2. E2E branch + continue behavior matches expected context.

### P1.2 Extension runtime v2

Deliver:
1. Expanded hook lifecycle around run/turn/message/tool boundaries.
2. Deterministic transform/block/mutate semantics.
3. Error isolation and observability for hook failures.

Exit criteria:
1. Hook ordering + block/mutate tests.
2. Extension E2E against active-run scenarios.

### P2 TUI stream-first UX

Deliver:
1. TUI consumes live event stream (not only final response payload).
2. Queue visibility for pending steer/follow-up.
3. Clear run/turn/tool progress display.

Exit criteria:
1. Updated `tui-smoke` for stream semantics.
2. Updated `tui-evidence` with injected steer/follow-up timeline.

## 4. Execution phases

### Phase A: IPC control plane

1. Add non-blocking prompt command behavior and live event delivery.
2. Unify run control path (avoid sync-only bypass).
3. Validate mid-run command timing behavior.

Gate A:
1. Protocol examples updated for non-blocking prompt and event stream.
2. Concurrency integration tests green.

### Phase B: Core + adapter semantics

1. Introduce typed message state in core.
2. Migrate tool loop to structured reinjection.
3. Upgrade provider adapter contracts.

Gate B:
1. Structured loop tests green.
2. Adapter contract tests green.

### Phase C: Session + extension parity

1. Session schema/record upgrade with migration support.
2. Extension lifecycle expansion and safety controls.

Gate C:
1. Session compatibility tests green.
2. Extension lifecycle E2E green.

### Phase D: TUI UX alignment

1. Switch TUI to streaming-first rendering.
2. Improve queue and progress UX.

Gate D:
1. TUI smoke/evidence updated and green.

## 5. Deliverables

1. Milestone 2 requirements doc (`docs/req-milestone2.md` or M2 section in `docs/req.md`).
2. Protocol spec update (`docs/protocol-openapi-like.json`).
3. New NDJSON examples:
   1. non-blocking prompt accepted
   2. live run events
   3. mid-run steer/follow_up/abort
4. M2 gate script (`scripts/phase-gate-m2.sh`) and make target (`make milestone2-gate`).
5. Updated E2E scripts for non-blocking/streaming behavior.

## 6. Non-goals

1. DAG/parallel tool scheduling.
2. Full pi interactive UI parity in M2.
3. Enterprise auth/multi-tenant policy systems.
4. Full provider matrix parity in one milestone.

## 7. Completion criteria

Milestone 2 is complete when:

1. IPC supports non-blocking prompt command + live event streaming.
2. Mid-run `steer`, `follow_up`, `abort` behavior is deterministic and tested.
3. Core loop runs on structured message context with typed tool_result reinjection.
4. Provider adapters implement normalized structured contracts.
5. Session and extension layers support new semantics without regressions.
6. `make milestone2-gate` passes locally and in CI.
