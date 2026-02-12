#!/usr/bin/env bash
set -euo pipefail

SOCKET="${1:-/tmp/nous-core-proto-compat.sock}"
rm -f "$SOCKET"

cleanup() {
  if [[ -n "${CORE_PID:-}" ]]; then
    kill "$CORE_PID" 2>/dev/null || true
    wait "$CORE_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET"
}
trap cleanup EXIT

go run ./cmd/core --socket "$SOCKET" >/tmp/nous-core-proto-compat.log 2>&1 &
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

# Preferred payload: session_id
BRANCH_NEW=$(go run ./cmd/corectl --socket "$SOCKET" branch "$PARENT_ID")
echo "$BRANCH_NEW" | rg -q '"session_id"' || { echo "branch(session_id) missing session_id: $BRANCH_NEW" >&2; exit 1; }

# Backward-compatible payload: parent_id via raw NDJSON command.
BRANCH_OLD=$(printf '{"v":"1","id":"compat-branch","type":"branch_session","payload":{"parent_id":"%s"}}\n' "$PARENT_ID" | nc -U "$SOCKET")
echo "$BRANCH_OLD" | rg -q '"ok":true' || { echo "branch(parent_id) should be accepted: $BRANCH_OLD" >&2; exit 1; }
echo "$BRANCH_OLD" | rg -q '"parent_id":"' || { echo "branch(parent_id) response missing parent_id: $BRANCH_OLD" >&2; exit 1; }

echo "protocol compat smoke ok"
