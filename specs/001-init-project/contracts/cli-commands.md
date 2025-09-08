# CLI Commands Contract: Project Initialization

**Date**: 2025-01-09  
**Context**: Command-line interface specification for project initialization

## Root Command

### `dumber init`
**Purpose**: Initialize the project with all dependencies

**Syntax**:
```bash
dumber init [OPTIONS]
```

**Options**:
- `--dry-run`: Show what would be done without executing
- `--force`: Overwrite existing files if present
- `--verbose, -v`: Show detailed progress information
- `--go-version`: Specify Go version requirement (default: auto-detect)

**Examples**:
```bash
# Basic initialization
dumber init

# Preview mode
dumber init --dry-run

# Force reinitialize existing project
dumber init --force --verbose

# Specify Go version
dumber init --go-version=1.21
```

**Exit Codes**:
- `0`: Success
- `1`: General error (invalid arguments, etc.)
- `2`: Dependency installation failed
- `3`: Tool installation failed
- `4`: Project already initialized (without --force)

**Output Format**:
```
✓ Initializing Go module: dumber
✓ Installing CLI dependencies: cobra, viper
✓ Installing database dependencies: sqlite3, sqlc
✓ Installing validation: go-playground/validator
✓ Installing testing: testify
✓ Installing Wails v3-alpha...
✓ Creating project structure...
✓ Generating configuration files...
✓ Verifying installation...

Project initialized successfully!

Next steps:
  1. Review generated configs/
  2. Run: go build ./cmd/dumber
  3. Start development: dumber --help
```

## Subcommands

### `dumber init deps`
**Purpose**: Install only dependencies without project structure

**Syntax**:
```bash
dumber init deps [OPTIONS]
```

**Options**:
- `--only`: Install specific dependency category (cli|db|validation|testing|wails)
- `--skip`: Skip specific dependency category
- `--latest`: Use @latest for all dependencies (default)
- `--check`: Verify dependencies without installing

**Examples**:
```bash
# Install only CLI dependencies
dumber init deps --only=cli

# Install everything except Wails
dumber init deps --skip=wails

# Check current dependency status
dumber init deps --check
```

### `dumber init structure`
**Purpose**: Create project directory structure only

**Syntax**:
```bash
dumber init structure [OPTIONS]
```

**Options**:
- `--template`: Use specific structure template (default|minimal)
- `--preview`: Show directory tree without creating

**Examples**:
```bash
# Create full project structure
dumber init structure

# Preview directory layout
dumber init structure --preview

# Minimal structure (no example files)
dumber init structure --template=minimal
```

### `dumber init config`
**Purpose**: Generate configuration files only

**Syntax**:
```bash
dumber init config [OPTIONS]
```

**Options**:
- `--type`: Generate specific config (sqlc|wails|all)
- `--overwrite`: Replace existing config files

**Examples**:
```bash
# Generate all config files
dumber init config

# Only SQLC configuration
dumber init config --type=sqlc

# Replace existing configs
dumber init config --overwrite
```

## Status and Information Commands

### `dumber init status`
**Purpose**: Show initialization status

**Syntax**:
```bash
dumber init status [OPTIONS]
```

**Options**:
- `--format`: Output format (text|json|yaml)
- `--detailed`: Show dependency versions and paths

**Output** (text format):
```
Project Status: ✓ Initialized

Dependencies:
  ✓ CLI (cobra v1.8.0, viper v1.18.2)
  ✓ Database (sqlite3 v1.14.19, sqlc v1.25.0)
  ✓ Validation (validator v10.16.0)
  ✓ Testing (testify v1.8.4)
  ✗ Wails (not installed)

Structure:
  ✓ cmd/, internal/, migrations/, configs/
  ✗ Missing: tests/integration/

Build Status: ✓ Ready (go build succeeds)
```

### `dumber init verify`
**Purpose**: Verify complete installation

**Syntax**:
```bash
dumber init verify [OPTIONS]
```

**Options**:
- `--fix`: Attempt to fix detected issues
- `--report`: Generate detailed verification report

**Exit Codes**:
- `0`: All verifications passed
- `1`: Some issues detected but not critical
- `2`: Critical issues prevent development

## Error Handling

### Common Error Scenarios

**Dependency Installation Failures**:
```bash
$ dumber init
✗ Installing database dependencies: sqlite3
  Error: CGO_ENABLED=1 required for sqlite3
  Suggestion: Run 'export CGO_ENABLED=1' and try again
  Exit Code: 2
```

**System Dependency Missing**:
```bash
$ dumber init
✗ Installing Wails v3-alpha
  Error: libwebkit2gtk-4.0-dev not found
  Suggestion: Install with 'sudo apt install libwebkit2gtk-4.0-dev'
  Exit Code: 3
```

**Project Already Exists**:
```bash
$ dumber init
✗ Project already initialized
  Use --force to reinitialize or run 'dumber init status'
  Exit Code: 4
```

## Integration with Other Systems

### CI/CD Integration
Commands designed for automated environments:
```bash
# Non-interactive initialization
dumber init --verbose > init.log 2>&1

# Verification in CI
dumber init verify --report=ci-report.json
```

### Development Workflow Integration
```bash
# Quick setup for new contributors
git clone <repo> && cd <repo>
dumber init
go test ./...
```

### IDE Integration
Status command provides machine-readable output:
```bash
dumber init status --format=json | jq '.dependencies.wails.status'
```

This CLI contract supports the initialization phase while providing flexibility for different development scenarios and debugging needs.