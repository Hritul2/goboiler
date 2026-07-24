BINARY  := goboiler
OUT_DIR := dist
MAIN    := ./cmd

.PHONY: dev build run tidy fmt fmt-check lint install-hooks test test-v test-cover

dev:
	go run $(MAIN)

build:
	@mkdir -p $(OUT_DIR)
	go build -o $(OUT_DIR)/$(BINARY) $(MAIN)

run: build
	./$(OUT_DIR)/$(BINARY)

tidy:
	go mod tidy

fmt:
	go fmt ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

lint:
	golangci-lint run

# Run once after cloning: points git at the committed hook script (native
# git, no pre-commit/lefthook framework needed for a pure-Go repo).
install-hooks:
	git config core.hooksPath .githooks

test:
	go test ./internal/...

test-v:
	go test ./internal/... -v

test-cover:
	go test ./internal/... -coverprofile=coverage.out
	go tool cover -func=coverage.out
