.PHONY: build test lint e2e-pingpong

build:
	go build ./...

test:
	go test ./...

lint:
	go vet ./...

e2e-pingpong:
	./scripts/pingpong.sh
