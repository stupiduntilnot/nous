.PHONY: build test lint e2e-pingpong e2e-smoke

build:
	go build ./...

test:
	go test ./...

lint:
	go vet ./...

e2e-pingpong:
	./scripts/pingpong.sh

e2e-smoke:
	./scripts/smoke.sh
