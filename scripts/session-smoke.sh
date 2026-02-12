#!/usr/bin/env bash
set -euo pipefail

SOCKET="${1:-/tmp/nous-core-session.sock}"
rm -f "$SOCKET"

cleanup() {
  if [[ -n "${CORE_PID:-}" ]]; then
    kill "$CORE_PID" 2>/dev/null || true
    wait "$CORE_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET"
}
trap cleanup EXIT

go run ./cmd/core --socket "$SOCKET" >/tmp/nous-core-session.log 2>&1 &
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

PARENT_JSON=$(go run ./cmd/corectl --socket "$SOCKET" new)
PARENT_ID=$(echo "$PARENT_JSON" | rg '"session_id"' | sed -E 's/.*"session_id": "([^"]+)".*/\1/')
[[ -n "$PARENT_ID" ]] || { echo "failed to parse parent session id: $PARENT_JSON" >&2; exit 1; }

go run ./cmd/corectl --socket "$SOCKET" prompt "parent-turn" >/dev/null

BRANCH_JSON=$(go run ./cmd/corectl --socket "$SOCKET" branch "$PARENT_ID")
BRANCH_ID=$(echo "$BRANCH_JSON" | rg '"session_id"' | sed -E 's/.*"session_id": "([^"]+)".*/\1/')
[[ -n "$BRANCH_ID" ]] || { echo "failed to parse branch session id: $BRANCH_JSON" >&2; exit 1; }
echo "$BRANCH_JSON" | rg -q "\"parent_id\": \"$PARENT_ID\"" || {
  echo "branch output missing parent_id: $BRANCH_JSON" >&2
  exit 1
}

go run ./cmd/corectl --socket "$SOCKET" switch "$BRANCH_ID" >/dev/null
OUT=$(go run ./cmd/corectl --socket "$SOCKET" prompt "branch-turn")
echo "$OUT" | rg -q '"session_id"' || { echo "missing session_id in prompt output: $OUT" >&2; exit 1; }
echo "$OUT" | rg -q "parent-turn" || { echo "branch prompt did not include parent context: $OUT" >&2; exit 1; }

echo "session smoke ok"
