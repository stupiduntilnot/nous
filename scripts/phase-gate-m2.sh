#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_file() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    echo "[m2-gate] missing required file: $path" >&2
    exit 1
  fi
}

echo "[m2-gate] verify milestone 2 protocol artifacts"
require_file docs/protocol-openapi-like.json
require_file docs/example-protocol-commands.ndjson
require_file docs/example-protocol-responses.ndjson
require_file docs/example-protocol-commands-live-run-control.ndjson
require_file docs/example-protocol-events-live-run-control.ndjson
require_file docs/example-protocol-responses-live-run-control.ndjson

echo "[m2-gate] run async prompt + run-control tests"
go test ./internal/ipc -run 'TestPromptCommandWithWaitFalseAcceptedOverIPC|TestPromptWaitFalseStreamsEventsOverEventSocket|TestAsyncPromptRunControlAcceptsSteerFollowUpAbort|TestAsyncPromptRunControlSteerPreemptsFollowUpOverIPC' -count=1

echo "[m2-gate] run protocol fixture consistency tests"
go test ./internal/protocol -run 'TestProtocolExamplesCommandsNDJSON|TestProtocolExamplesResponsesNDJSON|TestProtocolExamplesEventsNDJSON|TestResponseExamplesCoveredByResponseRequirements|TestResponseRequirementsHaveSuccessExamples' -count=1

echo "[m2-gate] run structured loop + provider contract tests"
go test ./internal/core -run 'TestAwaitNextTurnLoopsWithToolResults|TestToolCallWithoutAwaitStillContinuesNextTurn|TestEngineAppliesInputHookBeforeProviderCall|TestEngineIsolatesLifecycleHookErrorsAsWarnings' -count=1
go test ./internal/provider -run 'TestResolvePromptPrefersStructuredMessages|TestOpenAIAdapterUsesStructuredMessages|TestAdapterContractMockText|TestAdapterContractOpenAIText|TestAdapterContractGeminiText' -count=1
go test ./cmd/tui -run 'TestParseInputPromptIsNonBlocking|TestRunQueueStateLifecycle|TestRunQueueStateIgnoresMismatchedRun|TestRenderResultRendersProgressFieldsForRunTurnTool' -count=1

echo "[m2-gate] milestone 2 gate passed"
