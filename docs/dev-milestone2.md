# Nous Milestone 2 Dev Plan

## 1. Purpose

This document turns `docs/plan-milestone2.md` into an execution checklist with atomic steps, validation commands, and commit expectations.

Execution rule:
1. One atomic step per round.
2. Implement -> test -> fix -> test.
3. Commit after the step passes.

## 2. Current Status

Completed steps:
1. A1: non-blocking `prompt` (`wait:false`) accepted with `run_id`.
2. A2: live runtime event stream over `<socket>.events`.
3. A3: IPC run-control integration tests (`prompt -> steer/follow_up/abort`) and deterministic ordering checks.
4. A4: protocol fixtures updated for non-blocking/live control and `scripts/phase-gate-m2.sh` + `make milestone2-gate`.
5. B1: core/provider structured message request path (typed roles, structured reinjection for tool results).
6. B2: cross-adapter semantic contract tests (mock/openai/gemini).
7. C1: async-run session persistence pinned to prompt-origin session (safe against mid-run session switch).
8. C2: extension run lifecycle hooks (`run_start`, `run_end`) integrated into engine pipeline.
9. D1: TUI prompt path switched to stream-first rendering via live event socket.

Pending steps:
1. B3: remove remaining legacy provider request fields (`Prompt`, `ToolResults`) as primary contract.
2. C3: session schema v2 with explicit typed entry model and migration compatibility checks.
3. C4: extension hook error isolation semantics and observability improvements.
4. D2: TUI queue visibility for pending steer/follow_up during active run.
5. D3: richer run/turn/tool progress UX and evidence updates.

## 3. Atomic Step Template

For each pending step:
1. Scope:
   `one concrete behavior change`.
2. Code change:
   `minimum set of files`.
3. Validation:
   `go test` for touched packages, then `go test ./...`, then `make milestone2-gate`.
4. Commit:
   `single-purpose message`.

## 4. Validation Commands

1. `go test ./...`
2. `make milestone2-gate`
3. `./scripts/tui-smoke.sh`

## 5. Gate Notes

Current M2 gate enforces:
1. Async prompt and live run-control IPC tests.
2. Protocol fixture consistency.
3. Structured loop/provider contract checks.

Before marking Milestone 2 complete, add pending-step checks into `scripts/phase-gate-m2.sh`.
