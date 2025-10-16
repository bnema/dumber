# Makefile for dumber

.PHONY: build build-frontend test lint clean install-tools dev generate help check init build-static build-no-gui

# Load local overrides from .env.local if present (Makefile syntax)
ifneq (,$(wildcard .env.local))
include .env.local
export
endif

# Variables
BINARY_NAME=dumber
MAIN_PATH=.
DIST_DIR=dist

# Detect number of CPU cores for parallel compilation
NPROCS?=$(shell nproc 2>/dev/null || echo 1)

# Local caches to avoid $HOME permission issues (override via .env.local)
# GOMODCACHE?=$(CURDIR)/tmp/go-mod
# GOCACHE?=$(CURDIR)/tmp/go-cache
# GOTMPDIR?=$(CURDIR)/tmp
# GOENV=GOMODCACHE=$(GOMODCACHE) GOCACHE=$(GOCACHE) GOTMPDIR=$(GOTMPDIR)
GOENV=

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
build: build-frontend ## Build the application with GUI (frontend assets, then Go binary with WebKitGTK)
	@echo "Building $(BINARY_NAME) $(VERSION) with GUI using $(NPROCS) cores..."
	@mkdir -p $(DIST_DIR) tmp tmp/go-cache tmp/go-mod
	$(GOENV) CGO_ENABLED=1 go build -p $(NPROCS) $(LDFLAGS) -tags=webkit_cgo -o $(DIST_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "✅ Build successful! Binary: $(DIST_DIR)/$(BINARY_NAME)"

build-frontend: ## Build Svelte GUI with Tailwind CSS and main-world script
	@echo "Building Svelte GUI with Tailwind CSS and main-world script..."
	@cd gui && npm install --silent && npm run build
	@echo "GUI build complete"

build-no-gui: build-frontend ## Build binary without GUI (CGO disabled, CLI-only functionality)
	@echo "Building $(BINARY_NAME) $(VERSION) (CLI-only, no GUI) using $(NPROCS) cores..."
	@mkdir -p $(DIST_DIR) tmp tmp/go-cache tmp/go-mod
	$(GOENV) CGO_ENABLED=0 go build -p $(NPROCS) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-no-gui $(MAIN_PATH)
	@echo "✅ Build successful! Binary: $(DIST_DIR)/$(BINARY_NAME)-no-gui"

build-static: build-no-gui ## Alias for build-no-gui (backward compatibility)
	@echo "Note: build-static is deprecated, use build-no-gui instead"

# GUI build with WebKitGTK 6.0 (GTK4)
.PHONY: build-gui run-gui
build-gui: build ## Alias for default build (backward compatibility)
	@echo "Note: build-gui is now the default 'build' target"

run-gui: ## Run the GUI with native WebKitGTK 6.0 (requires GTK4/WebKitGTK 6 dev packages)
	@echo "Running GUI (webkit_cgo) using $(NPROCS) cores…"
	@mkdir -p tmp tmp/go-cache tmp/go-mod
	$(GOENV) CGO_ENABLED=1 go run -p $(NPROCS) -tags=webkit_cgo $(MAIN_PATH)

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

# Format code
fmt: ## Format Go code with gofmt
	@echo "Formatting Go code with gofmt..."
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
	rm -f $(BINARY_NAME)  # Remove any old binaries in root
	rm -rf gui/dist gui/node_modules
	rm -f assets/gui/gui.min.js assets/gui/homepage.min.js assets/gui/homepage.css assets/gui/main-world.min.js assets/gui/color-scheme.js
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

# Native release targets
.PHONY: release-snapshot release

release-snapshot: build-frontend ## Build snapshot using native goreleaser (no git tags required)
	@echo "Building snapshot with goreleaser..."
	goreleaser release --snapshot --clean

release: ## Create full release (amd64 only) using native goreleaser
	@echo "Building release with goreleaser..."
	GITHUB_TOKEN=$$(gh auth token) goreleaser release --clean
