# Manual Test Checklist

## 0. Environment

- [ ] `ollama list` includes `qwen2.5-coder:7b`
- [ ] Core starts on `/tmp/pi-core.sock` with local model config

```bash
OPENAI_API_KEY=dummy go run ./cmd/core \
  --socket /tmp/pi-core.sock \
  --provider openai \
  --model qwen2.5-coder:7b \
  --api-base http://127.0.0.1:11434 \
  --command-timeout 5s \
  --enable-demo-extension
```

## 1. Basic IPC

- [ ] `corectl ping` returns `pong`

```bash
go run ./cmd/corectl --socket /tmp/pi-core.sock --request-timeout 5s ping
```

## 2. Prompt Flow

- [ ] synchronous prompt returns `output/events/session_id`
- [ ] async prompt returns accepted payload (`{"command":"prompt","session_id":"..."}`)
- [ ] async prompt without pre-created session still returns non-empty `session_id`
- [ ] tool-loop continuation includes `status` event with non-empty `message`
- [ ] unknown/blocked tool path includes `warning` event with `code/message`
- [ ] provider failure path returns `provider_error` and carries `cause` when available

```bash
go run ./cmd/corectl --socket /tmp/pi-core.sock --request-timeout 5s prompt "say hello"
go run ./cmd/corectl --socket /tmp/pi-core.sock --request-timeout 5s prompt_async "say hello"
```

## 3. Session Flow

- [ ] `new` returns session id
- [ ] `switch` can switch to existing session
- [ ] `branch` (`session_id`) returns new session id and parent id
- [ ] raw `branch_session` with legacy `parent_id` is still accepted

```bash
go run ./cmd/corectl --socket /tmp/pi-core.sock new
go run ./cmd/corectl --socket /tmp/pi-core.sock switch <session_id>
go run ./cmd/corectl --socket /tmp/pi-core.sock branch <session_id>
printf '{"v":"1","id":"compat-branch","type":"branch_session","payload":{"parent_id":"<session_id>"}}\n' | nc -U /tmp/pi-core.sock
```

## 4. Extension Command Path

- [ ] `ext echo {"text":"hello"}` returns extension result payload
- [ ] `ext` on missing command returns `command_rejected`

```bash
go run ./cmd/corectl --socket /tmp/pi-core.sock ext echo '{"text":"hello"}'
go run ./cmd/corectl --socket /tmp/pi-core.sock ext hello
```

## 5. Tool Controls

- [ ] `set_active_tools` with unknown tool returns `command_rejected`
- [ ] `set_active_tools` without args clears active tool set

```bash
go run ./cmd/corectl --socket /tmp/pi-core.sock set_active_tools unknown_tool
go run ./cmd/corectl --socket /tmp/pi-core.sock set_active_tools
```

## 6. TUI Flow

- [ ] TUI connects and shows `status: connected`
- [ ] `status` command shows current session (`session: ...` or `session: (none)`)
- [ ] TUI command parsing works for `prompt/prompt_async/new/switch/branch/set_active_tools/ext`
- [ ] `prompt_async` accepted payload contains `session_id`, and subsequent `status` shows that session
- [ ] TUI renders `status/warning/error` lines from sync prompt events

```bash
go run ./cmd/tui /tmp/pi-core.sock
```

## 7. Automated Regression (recommended before release)

- [ ] `make phase-gate` passes
- [ ] `make ci` passes

```bash
make phase-gate
make ci
```
