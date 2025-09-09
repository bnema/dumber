# dumb-browser Development Guidelines

Auto-generated from all feature plans. Last updated: 2025-09-09

## Active Technologies
- Go 1.25.1 + Wails v3-alpha.28, Cobra, Viper, SQLite3 (002-browser-controls-ui)

## Project Structure
```
src/
├── models/        # Data models and entities
├── services/      # Business logic services  
├── cli/          # Command line interface
└── lib/          # Shared libraries

tests/
├── contract/     # Contract tests
├── integration/  # Integration tests
└── unit/         # Unit tests

frontend/
└── dist/         # Embedded WebView assets
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
- Constitutional simplicity principles

## Recent Changes
- 002-browser-controls-ui: Added keyboard/mouse controls, zoom persistence, clipboard integration

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->