#!/usr/bin/env bash
set -euo pipefail

SOCKET="${1:-/tmp/pi-core-tui-smoke.sock}"
rm -f "$SOCKET"

cleanup() {
  if [[ -n "${CORE_PID:-}" ]]; then
    kill "$CORE_PID" 2>/dev/null || true
    wait "$CORE_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET"
}
trap cleanup EXIT

go run ./cmd/core --socket "$SOCKET" >/tmp/pi-core-tui-smoke.log 2>&1 &
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

OUT=$(printf 'ping\nnew\nstatus\nquit\n' | go run ./cmd/tui "$SOCKET")

echo "$OUT" | rg -q 'status: connected' || { echo "tui did not report connected status" >&2; echo "$OUT" >&2; exit 1; }
echo "$OUT" | rg -q 'ok: type=pong' || { echo "tui ping did not return pong" >&2; echo "$OUT" >&2; exit 1; }
echo "$OUT" | rg -q 'ok: type=session' || { echo "tui new did not return session payload" >&2; echo "$OUT" >&2; exit 1; }
echo "$OUT" | rg -q 'session: sess-' || { echo "tui did not show active session id" >&2; echo "$OUT" >&2; exit 1; }

echo "tui smoke ok"
