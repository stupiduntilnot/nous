# ADR-0001: IPC Transport and Wire Protocol

## Status
Accepted

## Context
The project needs a stable local IPC mechanism for a headless core and minimal TUI client on Linux/macOS. We also need easy debugging and deterministic framing.

## Decision
1. Transport is Unix Domain Socket (UDS) only for current scope.
2. Wire protocol framing is NDJSON: one JSON object per line.
3. stdio transport is explicitly out of scope for current MVP.

## Consequences
1. Clients connect through a socket path (e.g. `/tmp/pi-core.sock`).
2. Message boundaries are newline-delimited; parsers read line-by-line.
3. Future transports may be added later, but must preserve protocol semantics.
