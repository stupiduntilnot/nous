.PHONY: build test lint ci release-gate phase-gate milestone2-gate milestone3-gate milestone4-gate milestone5-gate milestone6-gate start start-small start-medium start-large small medium large list-openai-models e2e-pingpong e2e-smoke e2e-local e2e-session e2e-extension e2e-protocol-compat e2e-tui e2e-tui-evidence

SOCKET ?= /tmp/nous-core.sock
API_BASE ?= https://api.openai.com/v1
WORKDIR ?= $(CURDIR)
OPENAI_MODEL_SMALL ?= gpt-4o-mini
OPENAI_MODEL_MEDIUM ?= gpt-4o
OPENAI_MODEL_LARGE ?= gpt-5.2-codex

START_SIZE := $(firstword $(filter small medium large,$(MAKECMDGOALS)))
START_MODEL := $(OPENAI_MODEL_MEDIUM)
ifeq ($(START_SIZE),small)
START_MODEL := $(OPENAI_MODEL_SMALL)
endif
ifeq ($(START_SIZE),large)
START_MODEL := $(OPENAI_MODEL_LARGE)
endif

build:
	mkdir -p bin
	go build -o bin/nous-core ./cmd/core
	go build -o bin/nous-ctl ./cmd/corectl
	go build -o bin/nous-tui ./cmd/tui

start: build
	@test -n "$$OPENAI_API_KEY" || (echo "OPENAI_API_KEY is required" >&2; exit 1)
	@echo "starting nous-core (size=$(if $(START_SIZE),$(START_SIZE),medium) model=$(START_MODEL) socket=$(SOCKET) workdir=$(WORKDIR))"
	OPENAI_API_KEY="$$OPENAI_API_KEY" ./bin/nous-core --socket "$(SOCKET)" --provider openai --model "$(START_MODEL)" --api-base "$(API_BASE)" --workdir "$(WORKDIR)"

start-small:
	$(MAKE) start small

start-medium:
	$(MAKE) start medium

start-large:
	$(MAKE) start large

small medium large:
	@:

list-openai-models:
	@test -n "$$OPENAI_API_KEY" || (echo "OPENAI_API_KEY is required" >&2; exit 1)
	@if command -v jq >/dev/null 2>&1; then \
		curl -sS https://api.openai.com/v1/models -H "Authorization: Bearer $$OPENAI_API_KEY" | jq -r '.data[].id' | sort; \
	else \
		echo "jq not found; printing raw JSON model list"; \
		curl -sS https://api.openai.com/v1/models -H "Authorization: Bearer $$OPENAI_API_KEY"; \
	fi

test:
	./scripts/phase-gate.sh
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

release-gate:
	$(MAKE) ci
	$(MAKE) e2e-tui-evidence

phase-gate:
	./scripts/phase-gate.sh

milestone2-gate:
	./scripts/phase-gate-m2.sh

milestone3-gate:
	./scripts/phase-gate-m3.sh

milestone4-gate:
	./scripts/phase-gate-m4.sh

milestone5-gate:
	./scripts/phase-gate-m5.sh

milestone6-gate:
	./scripts/phase-gate-m6.sh

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
