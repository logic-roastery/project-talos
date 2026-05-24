BINARY_NAME=talos
BUILD_DIR=bin
CMD_DIR=./cmd/talos

# Go parameters
GO=go
GOBUILD=$(GO) build
GOTEST=$(GO) test
GOVET=$(GO) vet
GOMOD=$(GO) mod

# Build flags
LDFLAGS=-s -w
BUILD_FLAGS=-ldflags "$(LDFLAGS)"

.PHONY: all build run test vet clean tidy dev help

all: clean build

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)

run: build
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME)

dev:
	@echo "Running in development mode..."
	@lsof -ti:4001 | xargs kill -9 2>/dev/null || true
	@TALOS_PORT=4001 TALOS_SESSION_SECRET=dev-secret $(GO) run $(CMD_DIR)

test:
	@echo "Running tests..."
	$(GOTEST) -v -race ./...

test-short:
	@echo "Running short tests..."
	$(GOTEST) -short ./...

vet:
	@echo "Running go vet..."
	$(GOVET) ./...

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)

tidy:
	@echo "Tidying modules..."
	$(GOMOD) tidy

fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

lint: vet fmt
	@echo "Linting complete"

help:
	@echo "Available targets:"
	@echo "  all        - Clean and build"
	@echo "  build      - Build binary"
	@echo "  run        - Build and run"
	@echo "  dev        - Run in development mode"
	@echo "  test       - Run all tests with race detection"
	@echo "  test-short - Run short tests only"
	@echo "  vet        - Run go vet"
	@echo "  clean      - Remove build artifacts"
	@echo "  tidy       - Tidy go modules"
	@echo "  fmt        - Format code"
	@echo "  lint       - Run vet and fmt"
	@echo "  help       - Show this help"
