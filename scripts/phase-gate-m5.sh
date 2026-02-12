#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_file() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    echo "[m5-gate] missing required file: $path" >&2
    exit 1
  fi
}

echo "[m5-gate] verify milestone 5 docs/artifacts"
require_file docs/plan-milestone5.md
require_file docs/protocol-openapi-like.json
require_file docs/example-protocol-commands.ndjson
require_file docs/example-protocol-responses.ndjson
require_file scripts/local-smoke.sh

echo "[m5-gate] protocol compatibility checks"
go test ./internal/protocol -run 'TestProtocolSchemaValidation|TestCommandPayloadRequirementsCoverAllCommands|TestProtocolExamplesCommandsNDJSON|TestProtocolExamplesResponsesNDJSON|TestProtocolExamplesEventsNDJSON|TestResponseExamplesCoveredByResponseRequirements|TestResponseRequirementsHaveSuccessExamples' -count=1

echo "[m5-gate] run m5-1 compaction checks"
go test ./internal/core -run 'Compaction|Summary' -count=1
go test ./internal/ipc -run 'compact_session|Session' -count=1
go test ./internal/session -run 'Compaction|Schema|Recover' -count=1

echo "[m5-gate] run m5-2 tree navigation checks"
go test ./internal/ipc -run 'set_leaf|get_tree|Leaf|Context' -count=1
go test ./internal/session -run 'Leaf|Tree|Context' -count=1
go test ./internal/protocol -run 'Spec|Examples|Requirements' -count=1

echo "[m5-gate] run m5-3 rich message block checks"
go test ./internal/core -run 'Message|Block|Transform|Convert|Fallback' -count=1
go test ./internal/provider -run 'Semantic|Contract|Block|Structured' -count=1

echo "[m5-gate] run m5-4 retry/abort resilience checks"
go test ./internal/provider -run 'Retry|Abort|Contract' -count=1
go test ./internal/core -run 'Provider|Retry|Abort|Runtime' -count=1
go test ./internal/ipc -run 'Event|Retry|Abort' -count=1

echo "[m5-gate] run m5-5 tool progress checks"
go test ./internal/core -run 'Tool|Progress|Execution' -count=1
go test ./internal/ipc -run 'tool_execution_update|Event' -count=1

echo "[m5-gate] run full test suite"
go test ./... -count=1

if [[ "${M5_GATE_LIVE_SMOKE:-0}" == "1" ]]; then
  if [[ -z "${OPENAI_API_KEY:-}" ]]; then
    echo "[m5-gate] M5_GATE_LIVE_SMOKE=1 but OPENAI_API_KEY is not set" >&2
    exit 1
  fi
  echo "[m5-gate] run live OpenAI smoke"
  ./scripts/local-smoke.sh
else
  echo "[m5-gate] skip live smoke (set M5_GATE_LIVE_SMOKE=1 to enable)"
fi

echo "[m5-gate] milestone 5 gate passed"
