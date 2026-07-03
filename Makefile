BINARY  := goboiler
OUT_DIR := dist
MAIN    := ./cmd

.PHONY: dev build run tidy fmt test test-v test-cover

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

test:
	go test ./internal/...

test-v:
	go test ./internal/... -v

test-cover:
	go test ./internal/... -coverprofile=coverage.out
	go tool cover -func=coverage.out
