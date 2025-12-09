# Makefile for dumber (Clean Architecture - puregotk)

.PHONY: build build-frontend test lint clean install-tools dev generate help init

# Load local overrides from .env.local if present (Makefile syntax)
ifneq (,$(wildcard .env.local))
include .env.local
export
endif

# Variables
BINARY_NAME=dumber
MAIN_PATH=./cmd/dumber
DIST_DIR=dist

# Detect number of CPU cores for parallel compilation
NPROCS?=$(shell nproc 2>/dev/null || echo 1)

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
build: build-frontend ## Build the application (pure Go, no CGO)
	@echo "Building $(BINARY_NAME) $(VERSION) using $(NPROCS) cores..."
	@mkdir -p $(DIST_DIR)
	go build -p $(NPROCS) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build successful! Binary: $(DIST_DIR)/$(BINARY_NAME)"

build-frontend: ## Build Svelte GUI with Tailwind CSS and main-world script
	@echo "Building Svelte GUI with Tailwind CSS..."
	@cd gui && npm install --silent && npm run build
	@echo "GUI build complete"

build-quick: ## Build without frontend (faster for backend development)
	@echo "Building $(BINARY_NAME) $(VERSION) (quick, no frontend)..."
	@mkdir -p $(DIST_DIR)
	go build -p $(NPROCS) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build successful! Binary: $(DIST_DIR)/$(BINARY_NAME)"

# Development targets
dev: ## Run in development mode
	@echo "Running in development mode..."
	go run $(MAIN_PATH)

run: build-quick ## Build and run
	@echo "Running $(BINARY_NAME)..."
	$(DIST_DIR)/$(BINARY_NAME)

generate: ## Generate code (SQLC)
	@echo "Generating code with SQLC..."
	sqlc generate
	@echo "Code generation complete"

# Testing
test: ## Run tests
	@echo "Running tests..."
	go test -v ./...

test-race: ## Run tests with race detection
	@echo "Running tests with race detection..."
	go test -race -v ./...

test-cover: ## Run tests with coverage
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Linting
lint: ## Run golangci-lint
	@echo "Running golangci-lint..."
	golangci-lint run

lint-fix: ## Run golangci-lint with --fix
	@echo "Running golangci-lint with --fix..."
	golangci-lint run --fix

# Format code
fmt: ## Format Go code with gofmt
	@echo "Formatting Go code..."
	go fmt ./...

# Tools installation
install-tools: ## Install development tools
	@echo "Installing development tools..."
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Tools installed successfully"

# Cleanup
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -rf $(DIST_DIR)
	rm -f $(BINARY_NAME)
	rm -rf gui/dist gui/node_modules
	rm -f assets/gui/gui.min.js assets/gui/homepage.min.js assets/gui/homepage.css assets/gui/main-world.min.js assets/gui/color-scheme.js
	rm -f coverage.out coverage.html
	go clean -cache
	go clean -testcache

# Project initialization
init: install-tools ## Initialize project dependencies and tools
	@echo "Initializing project..."
	go mod tidy
	@echo "Project initialized! Run 'make help' for available commands."

# Check setup
check: ## Check that all tools and dependencies are working
	@echo "Checking project setup..."
	@echo "Go version:"
	@go version
	@echo "\nSQLC version:"
	@sqlc version
	@echo "\nGolangci-lint version:"
	@golangci-lint version
	@echo "\nBuilding project..."
	@$(MAKE) build-quick
	@echo "\nRunning tests..."
	@$(MAKE) test
	@echo "\nAll checks passed!"

# Native release targets
.PHONY: release-snapshot release

release-snapshot: build-frontend ## Build snapshot using goreleaser
	@echo "Building snapshot with goreleaser..."
	goreleaser release --snapshot --clean

release: ## Create full release using goreleaser
	@echo "Building release with goreleaser..."
	GITHUB_TOKEN=$$(gh auth token) goreleaser release --clean
