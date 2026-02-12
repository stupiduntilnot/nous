# pi-mono Semantic Parity Matrix

This table tracks semantic compatibility targets against `pi-mono` runtime behavior.

## Commands

| Semantic | pi-mono | nous target | Status |
|---|---|---|---|
| `prompt` starts/continues an agent run | yes | same | aligned |
| `steer` injects mid-run user intent | yes | same | aligned |
| `follow_up` queues work after current run | yes | same | aligned |
| `abort` cancels in-flight processing | yes | same | aligned |
| `set_active_tools` updates model-visible tool set | yes (runtime-specific timing) | same intent | aligned-with-note |
| `new_session` resets context | yes | same | aligned |
| `switch_session` switches persisted context | yes | same | aligned |
| `branch_session` forks context from a parent session | yes (session-tree semantics) | same | aligned |
| `extension_command` invokes registered extension command handlers | runtime extension APIs exist, wire shape differs | same intent | aligned-with-note |

## Event Lifecycle

| Semantic | pi-mono | nous target | Status |
|---|---|---|---|
| agent lifecycle (`agent_start/end`) | yes | same | aligned |
| turn lifecycle (`turn_start/end`) | yes | same | aligned |
| message lifecycle (`message_start/update/end`) | yes | same | aligned |
| tool lifecycle (`tool_execution_start/update/end`) | yes | same | aligned |
| queue semantics (`steer/follow_up`) | yes | same | aligned |

## Explicit Compatibility Notes

1. Transport differs by design: this project uses UDS + NDJSON (pi-mono exposes multiple runtime modes).
2. Core semantics are the compatibility target; UI/process topology is not.
