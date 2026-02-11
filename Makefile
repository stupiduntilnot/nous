.PHONY: build test lint ci e2e-pingpong e2e-smoke e2e-local e2e-session

build:
	go build ./...

test:
	go test ./...

lint:
	go vet ./...

ci:
	go vet ./...
	go test ./...
	./scripts/local-smoke.sh
	./scripts/session-smoke.sh
	./scripts/smoke.sh

e2e-pingpong:
	./scripts/pingpong.sh

e2e-smoke:
	./scripts/smoke.sh

e2e-local:
	./scripts/local-smoke.sh

e2e-session:
	./scripts/session-smoke.sh
