# Nous Milestone 3 Plan

## 1. Goal

Milestone 3 goal: close the highest-impact **core runtime parity gaps** between `nous` and `pi-mono` (`packages/ai`, `packages/agent`, `packages/coding-agent`), while keeping the transport contract stable.

This milestone is focused on core behavior, not a new TUI framework.

## 2. Inputs

1. Nous current state:
`docs/plan-milestone2.md`, `docs/dev-milestone2.md`, `internal/core/*`, `internal/provider/*`, `internal/session/*`, `internal/ipc/*`.
2. pi-mono parity references:
`../pi-mono/packages/agent/src/agent.ts`, `../pi-mono/packages/agent/src/agent-loop.ts`, `../pi-mono/packages/agent/test/agent-loop.test.ts`, `../pi-mono/packages/ai/src/stream.ts`, `../pi-mono/packages/coding-agent/src/core/agent-session.ts`, `../pi-mono/packages/coding-agent/src/core/session-manager.ts`, `../pi-mono/packages/coding-agent/docs/rpc.md`.

## 3. Highest-Priority Gaps

1. Run/turn semantics are not yet truly agent-loop parity:
   `nous` still executes prompt/steer/follow-up as separate `Engine.Prompt` calls and does not support mid-tool steer interruption semantics equivalent to pi-mono.
2. Async prompt path does not apply session context reconstruction in the same way as sync flow.
3. Queue mode controls (`one-at-a-time` vs `all`) are missing in core/RPC.
4. Message model and provider boundary are still string-first; no `transformContext` + `convertToLlm` style boundary.
5. Provider stream normalization is minimal compared to pi-ai (usage/stop-reason/content-block fidelity).
6. Session model is still much simpler than pi-mono session tree semantics.

## 4. Priority Tasks

### P0: Agent Loop Semantics Parity (must-have)

#### M3-A1 Single-run lifecycle parity

Deliver:
1. Introduce a run coordinator so one logical run emits one `agent_start` and one `agent_end`.
2. Preserve per-turn `turn_start/turn_end` while processing prompt + queued steer/follow-up within the same run.
3. Keep deterministic single-writer state.

Exit criteria:
1. Integration test shows one `agent_start`/`agent_end` for a run with multiple turns.
2. Event order remains deterministic under concurrent command injection.

#### M3-A2 Mid-tool steering interruption parity

Deliver:
1. Poll steering queue between tool executions inside a turn.
2. If steering arrives, skip remaining tool calls in that assistant message.
3. Emit synthetic skipped tool results (pi-style behavior) and continue next turn with queued steer message.

Exit criteria:
1. Test equivalent to pi `agent-loop` skipped-tool behavior passes.
2. Replay/golden fixture documents event sequence for interrupted multi-tool turn.

#### M3-A3 Queue mode parity (`one-at-a-time` / `all`)

Deliver:
1. Add steer/follow-up dequeue mode settings in core.
2. Add RPC commands for setting and reading queue modes.
3. Ensure queue drain behavior matches selected mode.

Exit criteria:
1. Unit tests for both modes on steer and follow-up queues.
2. IPC tests verify mode changes are reflected in run behavior.

#### M3-A4 Async prompt context parity

Deliver:
1. Ensure async `prompt(wait:false)` uses the same session-derived context reconstruction as sync path.
2. Remove semantic drift between sync/async prompt execution paths.

Exit criteria:
1. Session continuation tests pass for async path.
2. Same conversation history yields equivalent model input in sync and async modes.

### P1: Message/Provider Parity (high value)

#### M3-B1 AgentMessage boundary parity

Deliver:
1. Evolve internal message model from plain text role/message into richer typed messages.
2. Add explicit `transformContext` then `convertToLlm` boundary before provider call.
3. Keep extension/custom messages out of provider payload unless mapped by converter.

Exit criteria:
1. Tests prove custom/internal message types can exist in context without breaking provider calls.
2. Provider requests are built only from converter output.

#### M3-B2 Provider stream normalization v3

Deliver:
1. Normalize provider outputs to structured stream events with stop reason and usage propagation.
2. Tighten OpenAI/Gemini adapters around one shared semantic contract.
3. Keep tool-call/tool-result reinjection behavior provider-agnostic.

Exit criteria:
1. Cross-adapter semantic contract tests cover usage/stop reason/tool calls.
2. No provider-specific behavior leaks into core loop logic.

### P1: Session/Extension Core Parity

#### M3-C1 Session tree semantics roadmap step

Deliver:
1. Add entry identity (`id`, `parent_id`) to session entries for branchable context traversal.
2. Extend stored entry types beyond plain messages (minimum: model/thinking/custom metadata markers).
3. Keep migration compatibility from current schema.

Exit criteria:
1. Migration tests from schema v1/v2 pass.
2. Branch/replay tests validate deterministic context reconstruction by leaf.

#### M3-C2 Extension hook surface parity

Deliver:
1. Add missing high-value lifecycle hooks (`before_agent_start`, `turn_start`) in addition to existing hooks.
2. Add session lifecycle pre-hooks needed for parity (`session_before_switch`, `session_before_fork` minimal set).
3. Keep hook failure isolation + warning observability semantics.

Exit criteria:
1. Hook ordering/isolation tests cover new hook points.
2. Session pre-hook cancellation behavior is integration-tested.

### P2: RPC/Core Operability Parity

#### M3-D1 Core state RPCs for parity debugging

Deliver:
1. Add `get_state` and `get_messages` parity-style read commands.
2. Expose queue mode state and pending counts.
3. Keep command/response schemas documented and validated.

Exit criteria:
1. IPC contract tests for new commands.
2. Protocol examples and schema validation updated.

#### M3-D2 Milestone 3 gate

Deliver:
1. Add `scripts/phase-gate-m3.sh` and `make milestone3-gate`.
2. Gate includes parity-critical tests (A1-A4, B1-B2, C1-C2, D1).

Exit criteria:
1. Gate runs green locally and in CI.

## 5. Execution Order

1. Phase A (M3-A1..A4) - stabilize loop semantics first.
2. Phase B (M3-B1..B2) - enforce message/provider boundary.
3. Phase C (M3-C1..C2) - session and extension parity surface.
4. Phase D (M3-D1..D2) - RPC observability and final gate.

## 6. Non-goals

1. New TUI framework migration.
2. Full visual UX parity with pi interactive mode.
3. Full coding-agent feature matrix (compaction UX, extension UI widgets, retry orchestration).

## 7. Completion Criteria

Milestone 3 is complete when:

1. Run lifecycle and queue semantics match pi-mono core expectations (single run lifecycle, mid-tool steer interruption, queue modes).
2. Async and sync prompt paths are context-equivalent.
3. Message/provider boundary supports typed context and conversion hooks.
4. Session schema advances toward branchable tree semantics with migration safety.
5. Extension hook surface covers required runtime/session lifecycle parity points.
6. `make milestone3-gate` passes.
