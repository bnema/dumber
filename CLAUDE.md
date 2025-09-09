# dumb-browser Development Guidelines

Auto-generated from all feature plans. Last updated: 2025-01-21

## Active Technologies  
- Go 1.25.1 + WebKit2GTK (native), Cobra, Viper, SQLite3 (003-webkit2gtk-migration)
- CGO bindings for WebKit2GTK C API integration

## Project Structure
```
pkg/
└── webkit/       # WebKit2GTK Go bindings (extractable)

src/
├── models/       # Data models and entities
├── services/     # Business logic services  
├── cli/         # Command line interface
└── lib/         # Shared libraries

tests/
├── contract/    # Contract tests for WebKit bindings
├── integration/ # Integration tests with WebKit2GTK
└── unit/        # Unit tests
```

## Commands
```bash
# Testing and linting
make test
make lint

# Build application
make build

# Development
make dev

# Generate code (SQLC)
make generate

# Database operations
sqlite3 ~/.config/dumber/browser.db
```

## Code Style
Go 1.25.1: Follow standard Go conventions with gofmt
- Use structured logging (slog)
- Proper error handling (no panics)
- SQLC for type-safe database queries  
- CGO memory management with Go finalizers
- Constitutional simplicity principles

## Recent Changes
- 003-webkit2gtk-migration: Migrating from Wails to native WebKit2GTK with custom CGO bindings
- 002-browser-controls-ui: Added keyboard/mouse controls, zoom persistence, clipboard integration

## WebKit2GTK Integration
- Reference repository: /home/brice/dev/clone/webkit2gtk-reference
- pkg/webkit: Reusable WebKit2GTK Go bindings
- Direct GTK event handling for keyboard shortcuts
- WebKitUserContentManager for script injection

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->