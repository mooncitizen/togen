.PHONY: check test build generate acceptance fmt

check: fmt
	go vet ./...
	go test ./...

fmt:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

test:
	go test ./...

build:
	go build -o bin/togen ./cmd/togen

generate:
	go run ./internal/schema/cmd

acceptance: build
	bash scripts/acceptance.sh
