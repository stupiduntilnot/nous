#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_file() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    echo "[m3-gate] missing required file: $path" >&2
    exit 1
  fi
}

echo "[m3-gate] verify milestone 3 docs/protocol artifacts"
require_file docs/plan-milestone3.md
require_file docs/dev-milestone3.md
require_file docs/protocol-openapi-like.json
require_file docs/example-protocol-commands.ndjson
require_file docs/example-protocol-responses.ndjson
require_file docs/example-protocol-events-live-run-control.ndjson
require_file scripts/local-smoke.sh

echo "[m3-gate] run phase A parity tests"
go test ./internal/core -run 'TestCommandLoopCoordinatesSingleRunLifecycleAcrossQueuedTurns|TestPromptWithinExternalRunEmitsSingleRunLifecycle|TestSteerQueuedSkipsRemainingToolCallsInSameAssistantMessage|TestCommandLoopSteerModeAllBatchesQueuedMessages|TestCommandLoopFollowUpModeAllBatchesQueuedMessages|TestPromptWithExecutionTextUsesExecutionPayloadButPersistsInput' -count=1
go test ./internal/ipc -run 'TestQueuedFollowUpKeepsSingleRunLifecycleEvents|TestSteerDuringFirstToolSkipsRemainingToolCalls|TestAsyncPromptRunControlHonorsQueueModeAllOverIPC|TestAsyncPromptUsesSessionContextAndPersistsRawInput' -count=1

echo "[m3-gate] run phase B parity tests"
go test ./internal/core -run 'TestEngineAppliesTransformContextBeforeConvertToLLM|TestEngineDefaultConvertFiltersCustomMessages|TestPromptEmitsStatusEventsForProviderDoneMetadata' -count=1
go test ./internal/provider -run 'TestAdapterContractMockText|TestAdapterContractOpenAIText|TestAdapterContractGeminiText|TestOpenAIAdapterDoneIncludesUsageAndStopReason|TestGeminiAdapterDoneIncludesUsageAndStopReason|TestOpenAIAdapterUsesStructuredMessages' -count=1

echo "[m3-gate] run phase C parity tests"
go test ./internal/session -run 'TestNormalizeMessageChainBackfillsLegacyIDsAndParents|TestBuildMessagePathByLeaf|TestBuildMessageContextSupportsLegacySessionLines|TestAppendMessageToAssignsIDAndParent|TestBuildMessageContextFromLeaf' -count=1
go test ./internal/extension -run 'TestBeforeAgentStartAndTurnStartHooksInvoked|TestSessionBeforeHooksCanCancel' -count=1
go test ./internal/ipc -run 'TestSessionPreHooksCanCancelSwitchAndBranch' -count=1

echo "[m3-gate] run phase D parity tests"
go test ./internal/ipc -run 'TestGetStateReportsQueueModesAndPendingCountsOverIPC|TestGetMessagesReturnsSessionHistoryOverIPC|TestDispatchDoesNotReturnNotImplementedForKnownCommands' -count=1
go test ./internal/protocol -run 'TestProtocolSchemaValidation|TestCommandPayloadRequirementsCoverAllCommands|TestProtocolExamplesCommandsNDJSON|TestProtocolExamplesResponsesNDJSON|TestProtocolExamplesEventsNDJSON|TestResponseExamplesCoveredByResponseRequirements|TestResponseRequirementsHaveSuccessExamples' -count=1
go test ./cmd/corectl -run 'TestParseArgs|TestParseArgsGetMessagesOptionalSessionID' -count=1

echo "[m3-gate] run full test suite"
go test ./... -count=1

if [[ -n "${OPENAI_API_KEY:-}" ]]; then
  echo "[m3-gate] run live OpenAI smoke"
  ./scripts/local-smoke.sh
else
  echo "[m3-gate] OPENAI_API_KEY is not set; skipping live smoke"
fi

echo "[m3-gate] milestone 3 gate passed"
