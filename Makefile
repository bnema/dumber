# Makefile for dumber

.PHONY: build build-frontend test lint clean install-tools dev generate help check init build-static

# Load local overrides from .env.local if present (Makefile syntax)
ifneq (,$(wildcard .env.local))
include .env.local
export
endif

# Variables
BINARY_NAME=dumber
MAIN_PATH=.
DIST_DIR=dist

# Local caches to avoid $HOME permission issues (override via .env.local)
GOMODCACHE?=$(CURDIR)/tmp/go-mod
GOCACHE?=$(CURDIR)/tmp/go-cache
GOTMPDIR?=$(CURDIR)/tmp
GOENV=GOMODCACHE=$(GOMODCACHE) GOCACHE=$(GOCACHE) GOTMPDIR=$(GOTMPDIR)

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
build: build-frontend ## Build the application (frontend assets, then Go binary)
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(DIST_DIR) tmp tmp/go-cache tmp/go-mod
	$(GOENV) CGO_ENABLED=1 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME) $(MAIN_PATH)

build-frontend: ## Build TypeScript frontend
	@echo "Building TypeScript frontend..."
	@cd frontend && npm install --silent && npm run build
	@echo "Frontend build complete"

build-static: ## Build static binary (CGO disabled, CLI-only functionality)
	@echo "Building static $(BINARY_NAME) $(VERSION) (CLI-only)..."
	@mkdir -p $(DIST_DIR) tmp tmp/go-cache tmp/go-mod
	$(GOENV) CGO_ENABLED=0 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-static $(MAIN_PATH)

# GUI build with WebKitGTK 6.0 (GTK4)
.PHONY: build-gui run-gui
build-gui: build-frontend ## Build GUI binary with native WebKitGTK 6.0 (requires GTK4/WebKitGTK 6 dev packages)
	@echo "Building $(BINARY_NAME) (GUI, webkit_cgo)…"
	@mkdir -p $(DIST_DIR) tmp tmp/go-cache tmp/go-mod
	$(GOENV) CGO_ENABLED=1 go build $(LDFLAGS) -tags=webkit_cgo -o $(DIST_DIR)/$(BINARY_NAME) $(MAIN_PATH)

run-gui: ## Run the GUI with native WebKitGTK 6.0 (requires GTK4/WebKitGTK 6 dev packages)
	@echo "Running GUI (webkit_cgo)…"
	@mkdir -p tmp tmp/go-cache tmp/go-mod
	$(GOENV) CGO_ENABLED=1 go run -tags=webkit_cgo $(MAIN_PATH)

.PHONY: check-webkit
check-webkit: ## Check system has GTK4/WebKitGTK 6.0/JavaScriptCore 6.0
	@echo "Checking pkg-config versions..."
	@which pkg-config >/dev/null 2>&1 || (echo "pkg-config not found" && exit 1)
	@echo "gtk4:            $$(pkg-config --modversion gtk4 2>/dev/null || echo not found)"
	@echo "webkitgtk-6.0:    $$(pkg-config --modversion webkitgtk-6.0 2>/dev/null || echo not found)"
	@echo "javascriptcoregtk-6.0: $$(pkg-config --modversion javascriptcoregtk-6.0 2>/dev/null || echo not found)"

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
	@mkdir -p tmp tmp/go-cache tmp/go-mod
	$(GOENV) CGO_ENABLED=0 go test -v ./...

test-race: ## Run tests with race detection
	@echo "Running tests with race detection..."
	@mkdir -p tmp tmp/go-cache tmp/go-mod
	$(GOENV) CGO_ENABLED=1 go test -race -v ./...

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
	@$(GOENV) go version
	@echo "\nSQLC version:"
	@$(GOENV) sqlc version
	@echo "\nGolangci-lint version:"
	@$(GOENV) golangci-lint version
	@echo "\nBuilding project..."
	@$(MAKE) build
	@echo "\nTesting built binary..."
	@$(DIST_DIR)/$(BINARY_NAME) version
	@echo "\nRunning tests..."
	@$(MAKE) test
	@echo "\n✅ All checks passed! Project is ready for development."
