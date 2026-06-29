BINARY  := goboiler
OUT_DIR := dist
MAIN    := ./cmd

.PHONY: dev build run tidy fmt

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
