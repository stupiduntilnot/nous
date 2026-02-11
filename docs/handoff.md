# Handoff Notes

## 1. Current Decisions (Frozen)

1. Core architecture is headless-first; TUI is a thin client for validation.
2. Transport is **UDS only** (no stdio in current scope).
3. Wire protocol is **NDJSON** (one JSON object per line).
4. Runtime model is **pi-style sequential loop** (single-writer state machine; no DAG tool scheduling in MVP).
5. Priority order is **P0 > P1 > P2**:
- `P0`: Core runtime
- `P1`: IPC protocol/transport
- `P2`: minimal TUI

## 2. Documentation Created/Updated

1. `docs/req.md`
- Requirements updated to reflect UDS+NDJSON and Core-first direction.

2. `docs/dev.md`
- Unified execution document: development steps + acceptance criteria in one file.
- Includes pre-P0 connectivity MVP (Core ping-pong, CLI ping, TUI ping).
- Includes protocol-spec step: OpenAPI-style schema + semantic parity with `pi-mono`.

## 3. Recent Commits

1. `d435253` - `docs: finalize requirements with UDS+NDJSON decisions`
2. `a0bed35` - `docs: add development plan and acceptance checklist`
3. `9aa68c8` - `docs: add pi framework study notes`
4. `4c610c1` - `docs: refine core-first Go pi clone requirements`

## 4. Repo State

1. Working tree is clean (no pending changes in `/Users/kuang/code/oh-my-agent`).
2. All active planning docs are under `docs/`.

## 5. Codex Config Change Applied

Added profile in `~/.codex/config.toml`:

```toml
[profiles.unattended]
approval_policy = "never"
sandbox_mode = "danger-full-access"
```

Backup file exists at `~/.codex/config.toml.bak`.

## 6. Resume Instructions (After Restart)

1. Restart Codex with unattended profile:
```bash
codex -p unattended -C /Users/kuang/code/oh-my-agent
```

2. Optionally resume prior conversation:
```bash
codex resume --last
```

3. On reconnect, start from:
- `docs/req.md`
- `docs/dev.md`
