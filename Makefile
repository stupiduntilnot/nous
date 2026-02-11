.PHONY: build test lint ci phase-gate e2e-pingpong e2e-smoke e2e-local e2e-session e2e-extension e2e-protocol-compat e2e-tui e2e-tui-evidence

build:
	go build ./...

test:
	go test ./...

lint:
	go vet ./...

ci:
	go vet ./...
	go test ./...
	./scripts/phase-gate.sh
	./scripts/pingpong.sh
	./scripts/local-smoke.sh
	./scripts/session-smoke.sh
	./scripts/extension-smoke.sh
	./scripts/protocol-compat-smoke.sh
	./scripts/tui-smoke.sh
	./scripts/smoke.sh

phase-gate:
	./scripts/phase-gate.sh

e2e-pingpong:
	./scripts/pingpong.sh

e2e-smoke:
	./scripts/smoke.sh

e2e-local:
	./scripts/local-smoke.sh

e2e-session:
	./scripts/session-smoke.sh

e2e-extension:
	./scripts/extension-smoke.sh

e2e-protocol-compat:
	./scripts/protocol-compat-smoke.sh

e2e-tui:
	./scripts/tui-smoke.sh

e2e-tui-evidence:
	./scripts/tui-evidence.sh
