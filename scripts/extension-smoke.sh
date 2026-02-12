#!/usr/bin/env bash
set -euo pipefail

SOCKET="${1:-/tmp/nous-core-extension.sock}"
rm -f "$SOCKET"

cleanup() {
  if [[ -n "${CORE_PID:-}" ]]; then
    kill "$CORE_PID" 2>/dev/null || true
    wait "$CORE_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET"
}
trap cleanup EXIT

go run ./cmd/core --socket "$SOCKET" --enable-demo-extension >/tmp/nous-core-extension.log 2>&1 &
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

OUT=$(go run ./cmd/corectl --socket "$SOCKET" ext echo '{"text":"hello"}')
echo "$OUT" | rg -q '"echo": "hello"' || {
  echo "unexpected extension echo output: $OUT" >&2
  exit 1
}

if go run ./cmd/corectl --socket "$SOCKET" ext unknown >/tmp/nous-core-extension-missing.log 2>&1; then
  echo "expected missing extension command to fail" >&2
  exit 1
fi
rg -q "extension_command_not_found" /tmp/nous-core-extension-missing.log || {
  echo "missing expected error marker for unknown extension command" >&2
  cat /tmp/nous-core-extension-missing.log >&2 || true
  exit 1
}

echo "extension smoke ok"
