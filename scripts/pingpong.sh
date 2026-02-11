#!/usr/bin/env bash
set -euo pipefail

SOCKET="${1:-/tmp/pi-core-e2e.sock}"
rm -f "$SOCKET"

cleanup() {
  if [[ -n "${CORE_PID:-}" ]]; then
    kill "$CORE_PID" 2>/dev/null || true
    wait "$CORE_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET"
}
trap cleanup EXIT

go run ./cmd/core --socket "$SOCKET" >/tmp/pi-core-e2e.log 2>&1 &
CORE_PID=$!

for _ in {1..100}; do
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
if [[ "$OUT" != "pong" ]]; then
  echo "unexpected output: $OUT" >&2
  exit 1
fi

echo "ping-pong ok"
