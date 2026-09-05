.PHONY: check test build build-cli ui ui-deps ui-test ui-check generate acceptance fmt

check: fmt
	go vet ./...
	go test ./...

fmt:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

test:
	go test ./...

ui-deps:
	pnpm --dir ui install --frozen-lockfile

ui: ui-deps
	pnpm --dir ui build
	rm -rf internal/server/dist/*
	cp -R ui/dist/. internal/server/dist/

ui-test: ui-deps
	pnpm --dir ui test

ui-check: ui-deps
	pnpm --dir ui check

build-cli:
	go build -o bin/togen ./cmd/togen

build: ui build-cli

generate:
	go run ./internal/schema/cmd

acceptance: build-cli
	bash scripts/acceptance.sh
