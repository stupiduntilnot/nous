#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_file() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    echo "[m4-gate] missing required file: $path" >&2
    exit 1
  fi
}

echo "[m4-gate] verify milestone 4 docs/artifacts"
require_file docs/plan-milestone4.md
require_file docs/tui-protocol-v1-contract.md
require_file docs/runtime-trace.md
require_file docs/protocol-openapi-like.json
require_file docs/example-protocol-commands.ndjson
require_file docs/example-protocol-responses.ndjson

echo "[m4-gate] protocol freeze checks"
go test ./internal/protocol -run 'TestProtocolSchemaValidation|TestCommandPayloadRequirementsCoverAllCommands|TestProtocolExamplesCommandsNDJSON|TestProtocolExamplesResponsesNDJSON|TestProtocolExamplesEventsNDJSON|TestResponseExamplesCoveredByResponseRequirements|TestResponseRequirementsHaveSuccessExamples' -count=1

echo "[m4-gate] run m4-1 stream-first checks"
go test ./cmd/corectl -run 'TestParseArgs|TestParseArgsPromptAsyncSetsWaitFalse' -count=1
go test ./internal/ipc -run 'TestPromptCommandWithWaitFalseAcceptedOverIPC|TestPromptWaitFalseStreamsEventsOverEventSocket' -count=1

echo "[m4-gate] run m4-2 trace/replay checks"
go test ./internal/ipc -run 'TestCaptureRunTraceAndReplayValidation|TestValidateRunTraceRejectsMismatchedRun' -count=1

echo "[m4-gate] run m4-3 leaf session checks"
go test ./internal/ipc -run 'TestPromptWaitIncludesLeafPathContext|TestAsyncPromptWithLeafPersistsParentLinkage' -count=1
go test ./internal/session -run 'TestAppendMessageToResolvedHonorsExplicitParent|TestBuildMessageContextFromLeaf' -count=1

echo "[m4-gate] run m4-4 extension timeout checks"
go test ./internal/extension -run 'TestHookTimeoutReturnsExtensionTimeoutError|TestToolTimeoutReturnsHandledTimeoutError' -count=1
go test ./internal/core -run 'TestEngineIsolatesInputHookTimeoutAsWarning|TestEngineIsolatesExtensionToolTimeoutAsToolError' -count=1
go test ./cmd/core -run 'TestConfigureExtensionTimeouts|TestConfigureExtensionTimeoutsRejectsNegativeValues' -count=1

echo "[m4-gate] run full test suite"
go test ./... -count=1

if [[ "${M4_GATE_LIVE_SMOKE:-0}" == "1" ]]; then
  if [[ -z "${OPENAI_API_KEY:-}" ]]; then
    echo "[m4-gate] M4_GATE_LIVE_SMOKE=1 but OPENAI_API_KEY is not set" >&2
    exit 1
  fi
  echo "[m4-gate] run live OpenAI smoke"
  ./scripts/local-smoke.sh
else
  echo "[m4-gate] skip live smoke (set M4_GATE_LIVE_SMOKE=1 to enable)"
fi

echo "[m4-gate] milestone 4 gate passed"
