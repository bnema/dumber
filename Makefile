# Makefile for dumber (Clean Architecture - puregotk)

.PHONY: build build-frontend test lint clean install-tools dev generate help init check-docs man flatpak-deps flatpak-build flatpak-install flatpak-run flatpak-clean stress-omnibox-callbacks verify-purego

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
	CGO_ENABLED=0 go build -p $(NPROCS) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build successful! Binary: $(DIST_DIR)/$(BINARY_NAME)"

build-frontend: ## Build homepage and error pages
	@echo "Building webui pages (homepage + error)..."
	@cd webui && npm install --silent && npm run build
	@echo "Frontend build complete"

build-quick: ## Build without frontend (faster for backend development)
	@echo "Building $(BINARY_NAME) $(VERSION) (quick, no frontend)..."
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 go build -p $(NPROCS) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME) $(MAIN_PATH)
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

mocks: ## Generate mock implementations with mockery
	@echo "Generating mocks with mockery..."
	mockery
	@echo "Mock generation complete"

# Testing
test: ## Run tests
	@echo "Running tests..."
	go test -v ./...

check-docs: ## Verify README/docs match code
	@go run ./dev/check-docs.go --fail-on-error

test-race: ## Run tests with race detection
	@echo "Running tests with race detection..."
	go test -race -v ./...

test-cover: ## Run tests with coverage
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

stress-omnibox-callbacks: ## Run placeholder omnibox callback stress harness
	GOFLAGS=-mod=mod CGO_ENABLED=0 go run ./scripts/stress_omnibox_callbacks.go

verify-purego: ## Ensure callback path stays cgo/export free
	bash ./scripts/verify_purego_only.sh

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
	rm -rf webui/dist webui/node_modules
	rm -f assets/webui/homepage.min.js assets/webui/error.min.js assets/webui/config.min.js assets/webui/index.html assets/webui/error.html assets/webui/config.html assets/webui/style.css
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
	@echo "\nVerifying purego-only constraints..."
	@$(MAKE) verify-purego
	@echo "\nAll checks passed!"

# Documentation
man: build-quick ## Install man pages to ~/.local/share/man/man1/
	@echo "Installing man pages..."
	$(DIST_DIR)/$(BINARY_NAME) gen-docs

# Native release targets
.PHONY: release-snapshot release

release-snapshot: build-frontend ## Build snapshot using goreleaser
	@echo "Building snapshot with goreleaser..."
	goreleaser release --snapshot --clean

release: ## Create full release using goreleaser
	@echo "Building release with goreleaser..."
	GITHUB_TOKEN=$$(gh auth token) goreleaser release --clean

# Flatpak targets
flatpak-deps: ## Install Flatpak build dependencies (Arch)
	@echo "Installing Flatpak build dependencies..."
	sudo pacman -S --needed flatpak flatpak-builder
	flatpak remote-add --if-not-exists --user flathub https://dl.flathub.org/repo/flathub.flatpakrepo
	flatpak install --user -y flathub org.gnome.Platform//48 org.gnome.Sdk//48
	flatpak install --user -y flathub org.freedesktop.Sdk.Extension.golang//24.08
	flatpak install --user -y flathub org.freedesktop.Sdk.Extension.node20//24.08
	@echo "Flatpak dependencies installed!"

flatpak-build: ## Build Flatpak bundle locally
	@echo "Building Flatpak bundle..."
	flatpak-builder --force-clean --user --repo=flatpak-repo flatpak-build dev.bnema.Dumber.yml
	flatpak build-bundle flatpak-repo dumber.flatpak dev.bnema.Dumber
	@echo "Flatpak bundle created: dumber.flatpak"

flatpak-install: ## Install Flatpak locally for testing
	@echo "Installing Flatpak locally..."
	flatpak-builder --force-clean --user --install flatpak-build dev.bnema.Dumber.yml
	@echo "Flatpak installed! Run with: make flatpak-run"

flatpak-run: ## Run the installed Flatpak
	flatpak run dev.bnema.Dumber

flatpak-clean: ## Clean Flatpak build artifacts
	@echo "Cleaning Flatpak build artifacts..."
	rm -rf flatpak-build flatpak-repo .flatpak-builder dumber.flatpak
	@echo "Flatpak artifacts cleaned!"
