# Runtime Trace Workflow (NDJSON)

This guide describes how to capture and replay run-scoped runtime traces without changing protocol `v=1`.

## 1. Capture a run trace

1. Start core and submit an async prompt.
2. Read the returned `run_id` from accepted response.
3. Capture event stream for that run and write NDJSON.

Example:
```bash
# submit async prompt
corectl prompt_async "analyze this"
# => {"command":"prompt","run_id":"run-12",...}

# capture run trace until agent_end
corectl trace run-12 > artifacts/run-12.trace.ndjson
```

`corectl trace` listens on `<socket>.events`, filters by `run_id`, and writes one event envelope per line.

## 2. Replay/validate trace in tests

Milestone 4 adds reusable helpers in `internal/ipc/trace.go`:
1. `ReadTraceNDJSON`
2. `WriteTraceNDJSON`
3. `ValidateRunTrace`

Use these helpers in tests to:
1. round-trip trace fixtures,
2. validate lifecycle ordering,
3. reproduce runtime sequencing issues from artifacts.

## 3. What validation checks

`ValidateRunTrace` asserts:
1. all events match the target `run_id`,
2. exactly one `agent_start` and one `agent_end`,
3. no events after `agent_end`,
4. balanced turn/message/tool lifecycle boundaries,
5. no message/tool updates outside active message/tool scopes.
