#!/usr/bin/env bash
set -euo pipefail

SOCKET="${1:-/tmp/pi-core-local.sock}"
MODEL="${MODEL:-qwen2.5-coder:7b}"
API_BASE="${API_BASE:-http://127.0.0.1:11434}"
API_KEY="${OLLAMA_API_KEY:-ollama}"

rm -f "$SOCKET"

cleanup() {
  if [[ -n "${CORE_PID:-}" ]]; then
    kill "$CORE_PID" 2>/dev/null || true
    wait "$CORE_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET"
}
trap cleanup EXIT

OLLAMA_API_KEY="$API_KEY" go run ./cmd/core \
  --socket "$SOCKET" \
  --provider ollama \
  --model "$MODEL" \
  --api-base "$API_BASE" >/tmp/pi-core-local.log 2>&1 &
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

OUT=$(go run ./cmd/corectl --socket "$SOCKET" --request-timeout 30s prompt "Reply with exactly: core-local-ok")
echo "$OUT" | rg -qi '"output": "core-local-ok"' || {
  echo "unexpected local model output: $OUT" >&2
  exit 1
}
echo "$OUT" | rg -q '"session_id"' || {
  echo "missing session_id in output: $OUT" >&2
  exit 1
}

echo "local smoke ok"
