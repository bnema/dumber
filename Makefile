# Makefile for dumber

.PHONY: build build-frontend test lint clean install-tools dev generate help

# Variables
BINARY_NAME=dumber
MAIN_PATH=.
DIST_DIR=dist

# Version information from git
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Linker flags
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Default target
help: ## Show this help message
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: build-frontend ## Build the Wails app (builds frontend, then wails3 build)
	@echo "Building $(BINARY_NAME) $(VERSION) with Wails..."
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=1 wails3 build

build-frontend: ## Build TypeScript frontend
	@echo "Building TypeScript frontend..."
	@cd frontend && npm ci --silent && npm run build
	@echo "Frontend build complete"

build-static: ## Build static binary (CGO disabled, CLI-only functionality)
	@echo "Building static $(BINARY_NAME) $(VERSION) (CLI-only)..."
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-static $(MAIN_PATH)

# Development targets
dev: ## Run in development mode
	@echo "Running in development mode..."
	go run $(MAIN_PATH)

generate: ## Generate code (SQLC)
	@echo "Generating code with SQLC..."
	sqlc generate
	@echo "Code generation complete"

# Testing
test: ## Run tests
	@echo "Running tests..."
	CGO_ENABLED=0 go test -v ./...

test-race: ## Run tests with race detection
	@echo "Running tests with race detection..."
	CGO_ENABLED=1 go test -race -v ./...

# Linting
lint: ## Run golangci-lint
	@echo "Running golangci-lint..."
	golangci-lint run

lint-fix: ## Run golangci-lint with --fix
	@echo "Running golangci-lint with --fix..."
	golangci-lint run --fix

# Tools installation
install-tools: ## Install development tools
	@echo "Installing development tools..."
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/wailsapp/wails/v3/cmd/wails3@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Tools installed successfully"

# Cleanup
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -rf $(DIST_DIR)
	rm -f $(BINARY_NAME)  # Remove any old binaries in root
	rm -rf frontend/dist frontend/node_modules
	go clean -cache
	go clean -testcache

# Project initialization 
init: install-tools ## Initialize project dependencies and tools
	@echo "Project dependencies and tools installed!"
	@echo "Ready for development. Run 'make help' for available commands."

# Check setup
check: ## Check that all tools and dependencies are working
	@echo "Checking project setup..."
	@echo "Go version:"
	@go version
	@echo "\nSQLC version:"
	@sqlc version
	@echo "\nWails version:"
	@wails3 version
	@echo "\nGolangci-lint version:"
	@golangci-lint version
	@echo "\nBuilding project..."
	@make build
	@echo "\nTesting built binary..."
	@$(DIST_DIR)/$(BINARY_NAME) version
	@echo "\nRunning tests..."
	@make test
	@echo "\n✅ All checks passed! Project is ready for development."
