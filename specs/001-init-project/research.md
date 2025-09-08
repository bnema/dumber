# Research: Project Initialization Dependencies

**Date**: 2025-01-09  
**Context**: Resolve technical unknowns for dumb-browser project initialization

## Research Questions Resolved

### 1. Go Module Initialization
**Decision**: Use `go mod init dumb-browser`
**Rationale**: 
- Simple module name matching the binary name
- Local development focus (no public import path needed)
- Follows Go module naming conventions

**Alternatives considered**: 
- `github.com/user/dumb-browser` - rejected (unnecessary complexity for local tool)
- `dumb-browser/v1` - rejected (premature versioning)

### 2. Wails v3-Alpha Compatibility
**Decision**: Use Wails v3.0.0-alpha.28 (latest available)
**Rationale**:
- Constitution specifically requires v3-alpha
- Alpha version provides newest WebKit2GTK integration
- Active development branch with Wayland improvements
- Breaking changes acceptable for new project

**Alternatives considered**:
- Wails v2 stable - rejected (constitution mandates v3-alpha)
- Wait for v3 stable - rejected (delays development unnecessarily)

**Integration approach**:
```bash
go install github.com/wailsapp/wails/v3/cmd/wails@v3.0.0-alpha.28
```

### 3. CLI Framework Selection
**Decision**: Cobra v1.8+ with Viper v1.18+
**Rationale**:
- Constitution requirement: "Cobra + Viper for configuration"
- Mature, stable libraries with excellent Go ecosystem integration
- Perfect for dmenu/rofi integration via stdin/stdout
- Built-in flag parsing and configuration management

**Key packages**:
- `github.com/spf13/cobra@latest`
- `github.com/spf13/viper@latest`

### 4. Database Stack
**Decision**: SQLite with ncruces/go-sqlite3 (CGO-free) + SQLC v1.25+
**Rationale**:
- Constitution requirement: "SQLite + SQLC for type-safe SQL"
- `github.com/ncruces/go-sqlite3` is CGO-free (better performance, fewer build issues)
- Only Wails needs CGO, not database layer - cleaner separation
- SQLC generates type-safe Go code from SQL schemas
- Perfect for local history storage

**Key packages**:
- `github.com/ncruces/go-sqlite3@latest` (CGO-free SQLite driver)
- Install SQLC CLI tool: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`

**Alternatives considered**:
- `github.com/mattn/go-sqlite3` - rejected (requires CGO unnecessarily)
- Pure Go implementations - rejected (ncruces is the modern standard)

### 5. Input Validation
**Decision**: go-playground/validator v10.16+
**Rationale**:
- Constitution requirement
- Industry standard for Go struct validation
- URL validation capabilities for browser input
- Tag-based validation (clean code)

**Key package**:
- `github.com/go-playground/validator/v10@latest`

### 6. Testing Framework
**Decision**: Standard Go testing + testify v1.8+
**Rationale**:
- Standard library first approach (constitution: simplicity)
- testify for better assertions and test organization
- No need for heavyweight testing frameworks

**Key package**:
- `github.com/stretchr/testify@latest`

### 7. Project Directory Structure
**Decision**: Follow constitution implementation order
```
/
├── cmd/                 # CLI entry point
├── internal/
│   ├── cli/            # Cobra commands
│   ├── db/             # SQLC generated code + migrations
│   ├── parser/         # URL parsing logic
│   └── browser/        # Wails integration
├── migrations/         # SQL schema files
├── configs/           # Configuration files (SQLC, Wails)
└── tests/             # Integration tests
```

**Rationale**:
- `internal/` prevents external imports (Go convention)
- Matches constitution implementation order
- Separates concerns clearly
- Easy to test individual components

### 8. Build Configuration
**Decision**: Standard Go build with Wails embedding
**Rationale**:
- Single binary requirement from constitution
- Wails handles frontend asset embedding
- Standard go build for CLI-only functionality

## Dependency Installation Order

Based on research, the installation sequence should be:

1. **Initialize Go module**
   ```bash
   go mod init dumb-browser
   ```

2. **Install CLI dependencies**
   ```bash
   go get github.com/spf13/cobra@latest
   go get github.com/spf13/viper@latest
   ```

3. **Install database dependencies**
   ```bash
   go get github.com/ncruces/go-sqlite3@latest
   go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
   ```

4. **Install validation**
   ```bash
   go get github.com/go-playground/validator/v10@latest
   ```

5. **Install testing tools**
   ```bash
   go get github.com/stretchr/testify@latest
   ```

6. **Install Wails (last - most complex)**
   ```bash
   go install github.com/wailsapp/wails/v3/cmd/wails@v3.0.0-alpha.28
   # Wails will handle its own module dependencies during init
   ```

## Risk Assessment

**Low Risk**:
- Standard Go dependencies (Cobra, Viper, SQLite)
- Well-established patterns

**Medium Risk**:
- Wails v3-alpha stability (alpha software)
- WebKit2GTK system dependency availability
- CGO requirement for Wails (but not for database layer)

**Mitigation**:
- Test Wails installation immediately after dependency setup
- Verify WebKit2GTK availability in integration tests
- Document CGO requirement clearly (only for final Wails build)
- Database layer can be developed/tested CGO-free independently

## Next Phase Requirements

All technical unknowns resolved. Phase 1 can proceed with:
- Data model design (SQLite schema)
- Contract definitions (CLI commands)
- Integration test scenarios
- Configuration file generation

**Status**: ✅ Complete - All NEEDS CLARIFICATION items resolved