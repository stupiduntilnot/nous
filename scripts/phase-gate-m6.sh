#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_file() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    echo "[m6-gate] missing required file: $path" >&2
    exit 1
  fi
}

echo "[m6-gate] verify milestone 6 docs/artifacts"
require_file docs/plan-milestone6.md
require_file docs/protocol-openapi-like.json
require_file docs/tui-protocol-v1-contract.md
require_file docs/example-protocol-events-prompt-tool.ndjson
require_file docs/example-protocol-events-runtime-tool-sequence.ndjson
require_file docs/example-protocol-events-live-run-control.ndjson

echo "[m6-gate] run m6-1 provider streaming checks"
go test ./internal/provider -run 'OpenAI|Stream|Retry|Abort|Contract' -count=1

echo "[m6-gate] run m6-2 core runtime streaming checks"
go test ./internal/core -run 'Engine|Runtime|Event|MessageUpdate|Golden' -count=1

echo "[m6-gate] run m6-3 ipc stream/backpressure checks"
go test ./internal/ipc -run 'Event|Stream|Backpressure|Trace' -count=1

echo "[m6-gate] run m6-4 cli stream-first checks"
go test ./cmd/corectl -run 'Prompt|Async|Stream|Timeout' -count=1
go test ./internal/ipc -run 'Prompt|Stream|Trace' -count=1

echo "[m6-gate] run m6-5 protocol/docs checks"
go test ./internal/protocol -run 'Spec|Examples|Requirements|ADR' -count=1

echo "[m6-gate] run full test suite"
go test ./... -count=1

if [[ "${M6_GATE_LIVE_SMOKE:-0}" == "1" ]]; then
  if [[ -z "${OPENAI_API_KEY:-}" ]]; then
    echo "[m6-gate] M6_GATE_LIVE_SMOKE=1 but OPENAI_API_KEY is not set" >&2
    exit 1
  fi

  SOCKET="/tmp/nous-core-m6-live.sock"
  MODEL="${MODEL:-gpt-4o-mini}"
  API_BASE="${API_BASE:-https://api.openai.com/v1}"

  echo "[m6-gate] run live streaming smoke (model=$MODEL socket=$SOCKET)"
  rm -f "$SOCKET" "$SOCKET.events"

  cleanup() {
    if [[ -n "${CORE_PID:-}" ]]; then
      kill "$CORE_PID" 2>/dev/null || true
      wait "$CORE_PID" 2>/dev/null || true
    fi
    rm -f "$SOCKET" "$SOCKET.events"
  }
  trap cleanup EXIT

  OPENAI_API_KEY="$OPENAI_API_KEY" go run ./cmd/core \
    --socket "$SOCKET" \
    --provider openai \
    --model "$MODEL" \
    --api-base "$API_BASE" \
    --workdir "$ROOT_DIR" >/tmp/nous-core-m6-live.log 2>&1 &
  CORE_PID=$!

  for _ in {1..200}; do
    if [[ -S "$SOCKET" && -S "$SOCKET.events" ]]; then
      break
    fi
    sleep 0.02
  done
  if [[ ! -S "$SOCKET" || ! -S "$SOCKET.events" ]]; then
    echo "[m6-gate] core sockets not ready" >&2
    exit 1
  fi

  OUT=$(go run ./cmd/corectl --socket "$SOCKET" --request-timeout 120s prompt_stream "Answer with three short numbered points about streaming events.")
  COUNT=$(printf '%s\n' "$OUT" | rg -c '"type":"message_update"' || true)
  if [[ "$COUNT" -lt 2 ]]; then
    echo "[m6-gate] expected at least 2 message_update events in live stream, got $COUNT" >&2
    echo "$OUT" >&2
    exit 1
  fi

  echo "[m6-gate] live streaming smoke passed (message_update count=$COUNT)"
else
  echo "[m6-gate] skip live smoke (set M6_GATE_LIVE_SMOKE=1 to enable)"
fi

echo "[m6-gate] milestone 6 gate passed"
