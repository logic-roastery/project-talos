BINARY_NAME=talos
BUILD_DIR=bin
CMD_DIR=./cmd/talos
DATA_DIR=./data

# Go parameters
GO=go
GOBUILD=$(GO) build
GOTEST=$(GO) test
GOVET=$(GO) vet
GOMOD=$(GO) mod

# Build flags
LDFLAGS=-s -w
BUILD_FLAGS=-ldflags "$(LDFLAGS)"

# Dev environment
DEV_PORT=4001
DEV_DOCKER_HOST=unix:///var/run/docker.sock
DEV_DOCKER_NETWORK=talos
DEV_DB_PATH=$(DATA_DIR)/talos.db
DEV_SESSION_SECRET=dev-secret-$(shell date +%s)
DEV_ENCRYPTION_KEY=ZGV2LWxvY2FsLXRlc3Qta2V5LTMyLWJ5dGVzLXBhZCE=

.PHONY: all build run test vet clean tidy dev dev-fresh dev-stop fmt lint ps help

all: clean build

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)

run: build
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME)

dev:
	@echo "Starting dev server on port $(DEV_PORT)..."
	@lsof -ti:$(DEV_PORT) | xargs kill -9 2>/dev/null || true
	@mkdir -p $(DATA_DIR)
	@TALOS_PORT=$(DEV_PORT) \
	 TALOS_SESSION_SECRET=$(DEV_SESSION_SECRET) \
	 TALOS_ENCRYPTION_KEY=$(DEV_ENCRYPTION_KEY) \
	 TALOS_DOCKER_HOST=$(DEV_DOCKER_HOST) \
	 TALOS_DOCKER_NETWORK=$(DEV_DOCKER_NETWORK) \
	 TALOS_DATABASE_PATH=$(DEV_DB_PATH) \
	 $(GO) run $(CMD_DIR)

dev-watch:
	@which air >/dev/null 2>&1 || { echo "Install air: go install github.com/air-verse/air@latest"; exit 1; }
	@mkdir -p $(DATA_DIR)
	@TALOS_PORT=$(DEV_PORT) \
	 TALOS_SESSION_SECRET=$(DEV_SESSION_SECRET) \
	 TALOS_ENCRYPTION_KEY=$(DEV_ENCRYPTION_KEY) \
	 TALOS_DOCKER_HOST=$(DEV_DOCKER_HOST) \
	 TALOS_DOCKER_NETWORK=$(DEV_DOCKER_NETWORK) \
	 TALOS_DATABASE_PATH=$(DEV_DB_PATH) \
	 air

dev-fresh:
	@echo "Resetting dev database..."
	@rm -f $(DEV_DB_PATH)
	@$(MAKE) dev

dev-stop:
	@echo "Stopping dev server..."
	@lsof -ti:$(DEV_PORT) | xargs kill -9 2>/dev/null || true
	@echo "Stopped."

test:
	@echo "Running tests..."
	$(GOTEST) -v -race ./...

test-short:
	@echo "Running short tests..."
	$(GOTEST) -short ./...

test-cover:
	@echo "Running tests with coverage..."
	$(GOTEST) -race -coverprofile=$(DATA_DIR)/coverage.out ./...
	$(GO) tool cover -html=$(DATA_DIR)/coverage.out -o $(DATA_DIR)/coverage.html
	@echo "Coverage report: $(DATA_DIR)/coverage.html"

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

ps:
	@docker ps --filter "label=managed-by=talos" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

help:
	@echo "Available targets:"
	@echo "  all        - Clean and build"
	@echo "  build      - Build binary"
	@echo "  run        - Build and run"
	@echo "  dev        - Kill existing, start dev server on port $(DEV_PORT)"
	@echo "  dev-watch  - Auto-reload on file changes (requires air)"
	@echo "  dev-fresh  - Reset database and start dev server"
	@echo "  dev-stop   - Stop the dev server"
	@echo "  test       - Run all tests with race detection"
	@echo "  test-short - Run short tests only"
	@echo "  test-cover - Run tests with HTML coverage report"
	@echo "  vet        - Run go vet"
	@echo "  clean      - Remove build artifacts"
	@echo "  tidy       - Tidy go modules"
	@echo "  fmt        - Format code"
	@echo "  lint       - Run vet and fmt"
	@echo "  ps         - List Talos-managed Docker containers"
	@echo "  help       - Show this help"
