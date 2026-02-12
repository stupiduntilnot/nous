# Nous

## What & Why
Nous is a Go implementation of a headless agent runtime inspired by `pi-mono` core semantics.

This repo focuses on:
1. Stable core runtime semantics (`agent_run` / `turn` / `tool_call` / `tool_result`).
2. Thin clients (CLI/TUI) over a fixed local IPC contract (`UDS + NDJSON`).
3. Testable, replayable event flow and deterministic behavior.

Why this repo exists:
1. Reproduce core agent behavior first, then extend safely.
2. Keep provider differences inside adapters, not in core logic.
3. Make milestones verifiable via scripts and gates.

## Build, Test, Run

### Build
```bash
make build
```

Binaries:
- `bin/nous-core`
- `bin/nousctl`
- `bin/nous-tui`

### Test
```bash
go test ./...
make phase-gate
```

Full gate (requires `OPENAI_API_KEY` in env):
```bash
source ~/.zshrc
make release-gate
```

### Run
Start core:
```bash
./bin/nous-core --socket /tmp/nous-core.sock --provider mock
```

Ping with CLI:
```bash
./bin/nousctl --socket /tmp/nous-core.sock ping
```

Start TUI:
```bash
./bin/nous-tui /tmp/nous-core.sock
```

Use OpenAI provider:
```bash
source ~/.zshrc
./bin/nous-core --socket /tmp/nous-core.sock --provider openai --model gpt-4o-mini
```
