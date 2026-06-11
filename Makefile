# Makefile for dumber (Clean Architecture - puregotk)

.PHONY: build build-systemviews generate-systemviews build-quick install-local test lint staticcheck clean install-tools install-golangci-lint install-staticcheck dev generate help init man flatpak-deps flatpak-build flatpak-install flatpak-run flatpak-clean stress-omnibox-callbacks verify-purego verify-generated check

# Load local overrides from .env.local if present (Makefile syntax)
ifneq (,$(wildcard .env.local))
include .env.local
export
endif

# Variables
BINARY_NAME=dumber
MAIN_PATH=./cmd/dumber
DIST_DIR=dist
LOCAL_BIN_DIR?=$(HOME)/.local/bin
TOOL_BIN_DIR?=$(shell go env GOPATH)/bin
GOLANGCI_LINT_VERSION?=v2.12.2
STATICCHECK_VERSION?=v0.7.0

# Detect number of CPU cores for parallel compilation
NPROCS?=$(shell nproc 2>/dev/null || echo 1)

# Version information from git
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go flags. NATIVE_GOFLAGS can add native-only hardening such as
# -buildmode=pie without breaking js/wasm systemviews builds.
GOFLAGS?=-mod=mod
NATIVE_GOFLAGS?=$(GOFLAGS)
WASM_GOFLAGS?=$(GOFLAGS)

# Linker flags
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Disable optimizations and inlining for ALL packages. purego callbacks
# create mixed Go/C stack frames; the Go runtime (stack growth, GC,
# scheduler) cannot reliably traverse optimised frames across these
# boundaries. -N prevents register-only variables and stack slot reuse
# that corrupt the runtime's stack walker. -l prevents inlining across
# package boundaries. Both are required — -l alone is insufficient.
GCFLAGS=-gcflags 'all=-N -l'

# Default target
help: ## Show this help message
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: build-systemviews ## Build the application (pure Go, no CGO)
	@echo "Building $(BINARY_NAME) $(VERSION) using $(NPROCS) cores..."
	@mkdir -p $(DIST_DIR)
	GOFLAGS="$(NATIVE_GOFLAGS)" CGO_ENABLED=0 go build -buildvcs=false -p $(NPROCS) $(GCFLAGS) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@rm -f $(DIST_DIR)/cef-helper
	@echo "Build successful! Binary: $(DIST_DIR)/$(BINARY_NAME)"

generate-systemviews: ## Generate Go code from systemviews templ components
	@echo "Generating systemviews templ components..."
	go tool templ generate -path internal/ui/systemviews -include-version=false
	@echo "Systemviews templ generation complete"

build-systemviews: generate-systemviews ## Build the WASM systemviews runtime
	@echo "Building systemviews wasm assets..."
	@command -v brotli >/dev/null 2>&1 || { echo "Error: brotli is required to build compressed systemviews assets. Install brotli and retry."; exit 1; }
	@mkdir -p assets/systemviews
	@cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" assets/systemviews/wasm_exec.js
	GOFLAGS="$(WASM_GOFLAGS)" GOOS=js GOARCH=wasm go build -buildvcs=false -ldflags="-s -w" -o assets/systemviews/systemviews.wasm ./cmd/systemviews
	brotli -f -o assets/systemviews/systemviews.wasm.br assets/systemviews/systemviews.wasm
	@echo "Systemviews build complete"

build-quick: ## Build quickly for backend development
	@echo "Building $(BINARY_NAME) $(VERSION) (quick)..."
	@mkdir -p $(DIST_DIR)
	GOFLAGS="$(NATIVE_GOFLAGS)" CGO_ENABLED=0 go build -buildvcs=false -p $(NPROCS) $(GCFLAGS) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@rm -f $(DIST_DIR)/cef-helper
	@echo "Build successful! Binary: $(DIST_DIR)/$(BINARY_NAME)"

install-local: build-quick ## Install dumber to ~/.local/bin atomically
	@echo "Installing $(BINARY_NAME) to $(LOCAL_BIN_DIR)..."
	@mkdir -p $(LOCAL_BIN_DIR)
	@tmp_dumber="$$(mktemp '$(LOCAL_BIN_DIR)/.dumber.tmp.XXXXXX')"; \
	trap 'rm -f "$$tmp_dumber"' EXIT INT TERM; \
	install -m 0755 $(DIST_DIR)/$(BINARY_NAME) "$$tmp_dumber"; \
	mv -f "$$tmp_dumber" $(LOCAL_BIN_DIR)/$(BINARY_NAME); \
	removed_stale=0; \
	if [ -e $(LOCAL_BIN_DIR)/cef-helper ]; then rm -f $(LOCAL_BIN_DIR)/cef-helper && removed_stale=1; fi; \
	trap - EXIT INT TERM; \
	echo "Installed: $(LOCAL_BIN_DIR)/$(BINARY_NAME)"; \
	if [ "$$removed_stale" -eq 1 ]; then echo "Removed stale: $(LOCAL_BIN_DIR)/cef-helper"; fi

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
	GOFLAGS=-mod=mod go test -v ./...

test-race: ## Run tests with race detection
	@echo "Running tests with race detection..."
	GOFLAGS=-mod=mod go test -race -v ./...

test-cover: ## Run tests with coverage
	@echo "Running tests with coverage..."
	GOFLAGS=-mod=mod go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

stress-omnibox-callbacks: ## Run placeholder omnibox callback stress harness
	GOFLAGS=-mod=mod CGO_ENABLED=0 go run ./scripts/stress_omnibox_callbacks.go

verify-purego: ## Ensure callback path stays cgo/export free
	bash ./scripts/verify_purego_only.sh

verify-generated: ## Verify generated systemviews artifacts are committed
	@echo "Verifying generated systemviews artifacts..."
	@git diff --exit-code -- assets/systemviews internal/ui/systemviews || { \
		echo "Generated systemviews artifacts are out of date. Run 'make build-systemviews' and commit the result."; \
		exit 1; \
	}

# Linting
lint: ## Run golangci-lint
	@echo "Running golangci-lint..."
	golangci-lint run

lint-fix: ## Run golangci-lint with --fix
	@echo "Running golangci-lint with --fix..."
	golangci-lint run --fix

staticcheck: ## Run Staticcheck
	@echo "Running staticcheck..."
	staticcheck ./...

# Format code
fmt: ## Format Go code with gofmt
	@echo "Formatting Go code..."
	go fmt ./...

# Tools installation
install-golangci-lint: ## Install pinned golangci-lint
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION) to $(TOOL_BIN_DIR)..."
	@mkdir -p $(TOOL_BIN_DIR)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TOOL_BIN_DIR) $(GOLANGCI_LINT_VERSION)

install-staticcheck: ## Install pinned Staticcheck with the active Go toolchain
	@echo "Installing staticcheck $(STATICCHECK_VERSION)..."
	go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)

install-tools: install-golangci-lint install-staticcheck ## Install development tools
	@echo "Installing development tools..."
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	@echo "Tools installed successfully"

# Cleanup
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -rf $(DIST_DIR)
	rm -f $(BINARY_NAME)
	rm -f assets/systemviews/wasm_exec.js assets/systemviews/systemviews.wasm assets/systemviews/systemviews.wasm.br
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

release-snapshot: build-systemviews ## Build snapshot using goreleaser
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
