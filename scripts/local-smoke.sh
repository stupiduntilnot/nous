#!/usr/bin/env bash
set -euo pipefail

SOCKET="${1:-/tmp/nous-core-local.sock}"
MODEL="${MODEL:-gpt-4o-mini}"
API_BASE="${API_BASE:-https://api.openai.com/v1}"
API_KEY="${OPENAI_API_KEY:-}"

if [[ -z "$API_KEY" ]]; then
  echo "OPENAI_API_KEY is required for local-smoke" >&2
  exit 1
fi

rm -f "$SOCKET"

cleanup() {
  if [[ -n "${CORE_PID:-}" ]]; then
    kill "$CORE_PID" 2>/dev/null || true
    wait "$CORE_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET"
}
trap cleanup EXIT

OPENAI_API_KEY="$API_KEY" go run ./cmd/core \
  --socket "$SOCKET" \
  --provider openai \
  --model "$MODEL" \
  --api-base "$API_BASE" >/tmp/nous-core-local.log 2>&1 &
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
  echo "unexpected openai output: $OUT" >&2
  exit 1
}
echo "$OUT" | rg -q '"session_id"' || {
  echo "missing session_id in output: $OUT" >&2
  exit 1
}

echo "openai smoke ok"
