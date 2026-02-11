#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

require_file() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    echo "[gate] missing required file: $path" >&2
    exit 1
  fi
}

require_test() {
  local pkg="$1"
  local name="$2"
  local listed
  listed="$(go test "$pkg" -list .)"
  if ! printf '%s\n' "$listed" | grep -qx "$name"; then
    echo "[gate] missing required test: $pkg $name" >&2
    exit 1
  fi
}

echo "[gate] verify protocol docs/artifacts"
require_file docs/req.md
require_file docs/dev.md
require_file docs/protocol/openapi-like.json
require_file docs/protocol/pi-mono-semantic-matrix.md
require_file docs/protocol/examples/commands.ndjson
require_file docs/protocol/examples/responses.ndjson
require_file docs/protocol/examples/events_prompt_tool.ndjson
require_file docs/protocol/examples/events_runtime_tool_sequence.ndjson
require_file scripts/local-smoke.sh
require_file scripts/session-smoke.sh
require_file scripts/extension-smoke.sh
require_file scripts/protocol-compat-smoke.sh
require_file scripts/tui-smoke.sh
require_file scripts/tui-evidence.sh
require_file scripts/smoke.sh
require_file scripts/pingpong.sh
rg -q 'UDS \+ NDJSON' docs/req.md docs/dev.md

echo "[gate] verify critical test inventory"
require_test ./internal/core TestStateTransitions
require_test ./internal/core TestEventSequenceGoldenPromptBasic
require_test ./internal/core TestRuntimeEventSequenceMatchesProtocolExample
require_test ./internal/core TestPromptEmitsErrorEventOnProviderError
require_test ./internal/core TestCommandLoopSteerPreemptsFollowUps
require_test ./internal/ipc TestCorePingPong
require_test ./internal/ipc TestSteerPreemptsFollowUpOverIPC
require_test ./internal/ipc TestAsyncPromptAutoCreatesSessionAndReturnsSessionID
require_test ./internal/ipc TestDispatchAsyncPromptAcceptedIncludesSessionID
require_test ./internal/ipc TestDispatchAcceptsProtocolCommandExamplePayloadShapes
require_test ./internal/ipc TestDispatchResponsesSatisfySpecPayloadRequirements
require_test ./internal/protocol TestProtocolSchemaValidation
require_test ./internal/protocol TestProtocolExamplesCommandsNDJSON
require_test ./internal/protocol TestProtocolExamplesResponsesNDJSON
require_test ./cmd/tui TestRenderResultRendersStatusWarningErrorEvents

echo "[gate] run targeted phase checks"
go test ./internal/core ./internal/session ./internal/extension ./internal/provider
go test ./internal/ipc ./internal/protocol ./cmd/core ./cmd/corectl ./cmd/tui

echo "[gate] phase checks passed"
