BIN_DIR := bin
BIN_NAME := claude

.PHONY: build test fmt lint

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BIN_NAME) ./cmd/claude

test:
	go test ./...

fmt:
	go fmt ./...

lint:
	@echo "golangci-lint not configured yet"
