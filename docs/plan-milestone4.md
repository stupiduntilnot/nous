# Nous Milestone 4 Plan

## 1. Goal

Milestone 4 goal: harden core runtime operability and developer safety while keeping NDJSON protocol `v=1` stable for external product-TUI work.

Milestone 4 focus is core/runtime quality, not TUI UX redesign.

## 2. Inputs

1. Stable protocol contract: `docs/tui-protocol-v1-contract.md`, `docs/protocol-openapi-like.json`, `internal/protocol/types.go`.
2. Milestone 3 baseline: `docs/plan-milestone3.md`, `docs/dev-milestone3.md`, `scripts/phase-gate-m3.sh`.
3. Core packages: `internal/core`, `internal/ipc`, `internal/session`, `internal/extension`, `internal/provider`.

## 3. Priorities

### P0: Runtime Contract Hardening

#### M4-A1 Stream-first default, sync prompt as compatibility shim

Problem:
1. Sync `prompt(wait:true)` still exists and can be misused as primary interaction path.

Deliver:
1. Keep sync path backward compatible.
2. Make stream-first semantics explicit in code/docs/tests.
3. Add guardrails so new features do not depend on sync result payload.

Exit criteria:
1. IPC tests assert async (`wait:false`) path is canonical.
2. No new runtime behavior is only exposed via sync response body.

#### M4-A2 Event trace export + deterministic replay

Problem:
1. Debugging parity/runtime issues is slower without first-class run traces.

Deliver:
1. Add trace export utility for run-scoped events (NDJSON artifact).
2. Add deterministic replay test harness from captured trace.
3. Document trace format and replay workflow.

Exit criteria:
1. At least one end-to-end replay test passes from exported fixture.
2. Debug workflow can reproduce run ordering issues from artifact only.

### P1: Session and Extension Robustness

#### M4-B1 Leaf-aware prompt execution API

Problem:
1. Session tree exists, but prompt execution from explicit leaf context is not yet first-class in IPC.

Deliver:
1. Add IPC/API support to execute prompt with explicit `leaf_id` context selection.
2. Persist resulting messages with correct parent linkage.
3. Keep default behavior unchanged when `leaf_id` is not provided.

Exit criteria:
1. IPC tests prove prompt-from-leaf reconstructs correct path context.
2. Session tests prove deterministic parent/leaf linkage after branching.

#### M4-B2 Extension isolation with timeout/resource policy

Problem:
1. Extension hooks/tools can still degrade run stability if they block too long.

Deliver:
1. Add configurable timeout policy for extension hook/tool execution.
2. Convert timeout/failure to isolated warning/error envelopes without corrupting runtime state.
3. Add observability fields for timeout/rejection reasons.

Exit criteria:
1. Timeout tests cover hook and extension tool paths.
2. Runtime remains responsive and state-consistent under extension delays.

### P1: Build/CI Reliability

#### M4-C1 Gate split and smoke policy

Problem:
1. Local and CI flows need clearer separation between deterministic tests and live-provider checks.

Deliver:
1. Add milestone 4 gate script with deterministic core checks.
2. Keep live OpenAI smoke as optional-but-supported stage via explicit env toggle.
3. Ensure protocol freeze checks are enforced in gate.

Exit criteria:
1. `make milestone4-gate` passes without network when smoke toggle is off.
2. Live smoke stage can be enabled in CI/local with one env flag.

## 4. Execution Order

1. M4-A1 stream-first hardening.
2. M4-A2 trace export/replay.
3. M4-B1 leaf-aware prompt API.
4. M4-B2 extension timeout/resource isolation.
5. M4-C1 gate split + smoke policy.

## 5. Non-goals

1. Product-level TUI framework migration.
2. Visual UX redesign in `cmd/tui`.
3. New provider families beyond current adapters.

## 6. Completion Criteria

Milestone 4 is complete when:
1. Protocol `v=1` compatibility remains intact for external TUI.
2. Stream-first runtime path is clearly canonical and protected by tests.
3. Trace export/replay is usable for debugging regressions.
4. Leaf-aware prompt API is supported and tested.
5. Extension timeout isolation is implemented and verified.
6. Milestone 4 gate is green, with deterministic and optional live-smoke stages.
