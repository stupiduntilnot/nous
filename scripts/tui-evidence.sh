#!/usr/bin/env bash
set -euo pipefail

SOCKET="${1:-/tmp/pi-core-tui-evidence.sock}"
ARTIFACT_DIR="${2:-artifacts}"
mkdir -p "$ARTIFACT_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUT_FILE="$ARTIFACT_DIR/tui-evidence-$STAMP.log"

rm -f "$SOCKET"

cleanup() {
  if [[ -n "${CORE_PID:-}" ]]; then
    kill "$CORE_PID" 2>/dev/null || true
    wait "$CORE_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET"
}
trap cleanup EXIT

go run ./cmd/core --socket "$SOCKET" >/tmp/pi-core-tui-evidence.log 2>&1 &
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

OUT=$(printf 'ping\nprompt_async hello-from-evidence\nstatus\nquit\n' | go run ./cmd/tui "$SOCKET")
printf '%s\n' "$OUT" | tee "$OUT_FILE" >/dev/null

echo "$OUT" | rg -q 'status: connected' || { echo "tui evidence missing connected status" >&2; exit 1; }
echo "$OUT" | rg -q 'ok: type=pong' || { echo "tui evidence missing pong" >&2; exit 1; }
echo "$OUT" | rg -q 'ok: type=accepted payload=.*command:prompt' || { echo "tui evidence missing prompt accepted" >&2; exit 1; }
echo "$OUT" | rg -q 'session_id:sess-' || { echo "tui evidence missing accepted session_id" >&2; exit 1; }
echo "$OUT" | rg -q 'session: sess-' || { echo "tui evidence missing active session" >&2; exit 1; }

echo "tui evidence saved: $OUT_FILE"
