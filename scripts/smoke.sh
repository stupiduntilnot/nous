#!/usr/bin/env bash
set -euo pipefail

SOCKET="${1:-/tmp/nous-core-smoke.sock}"
rm -f "$SOCKET"

cleanup() {
  if [[ -n "${CORE_PID:-}" ]]; then
    kill "$CORE_PID" 2>/dev/null || true
    wait "$CORE_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET"
}
trap cleanup EXIT

go run ./cmd/core --socket "$SOCKET" >/tmp/nous-core-smoke.log 2>&1 &
CORE_PID=$!

for _ in {1..200}; do
  if [[ -S "$SOCKET" ]]; then
    break
  fi
  sleep 0.02
done
if [[ ! -S "$SOCKET" ]]; then
  echo "core socket not ready: $SOCKET" >&2
  exit 1
fi

OUT=$(go run ./cmd/corectl --socket "$SOCKET" ping)
[[ "$OUT" == "pong" ]] || { echo "unexpected ping output: $OUT" >&2; exit 1; }

PROMPT_OUT=$(go run ./cmd/corectl --socket "$SOCKET" prompt "hello smoke")
echo "$PROMPT_OUT" | rg -q '"output"' || { echo "prompt output missing: $PROMPT_OUT" >&2; exit 1; }
echo "$PROMPT_OUT" | rg -q '"session_id"' || { echo "prompt output missing session_id: $PROMPT_OUT" >&2; exit 1; }

NEW_OUT=$(go run ./cmd/corectl --socket "$SOCKET" new)
SESSION_ID=$(echo "$NEW_OUT" | rg '"session_id"' | sed -E 's/.*"session_id": "([^"]+)".*/\1/')
[[ -n "$SESSION_ID" ]] || { echo "failed to parse session id: $NEW_OUT" >&2; exit 1; }

go run ./cmd/corectl --socket "$SOCKET" switch "$SESSION_ID" >/dev/null

PROMPT2_OUT=$(go run ./cmd/corectl --socket "$SOCKET" prompt "hello second smoke")
echo "$PROMPT2_OUT" | rg -q "\"session_id\": \"$SESSION_ID\"" || {
  echo "prompt missing/incorrect session_id: $PROMPT2_OUT" >&2
  exit 1
}

echo "smoke ok"
