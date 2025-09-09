# Quickstart: Project Initialization

**Goal**: Validate that project initialization works correctly and all dependencies are properly installed.

## Prerequisites Verification

Before running quickstart, verify system requirements:

```bash
# Check Go version (1.25+ required)
go version
# Expected: go version go1.25.x linux/amd64

# Check system packages for Wails (Arch Linux)
pacman -Q webkit2gtk gtk3
# Expected: webkit2gtk-4.0 and gtk3 packages listed

# For Ubuntu/Debian:
# dpkg -l | grep -E "(webkit2gtk|libgtk-3)"

# Note: CGO only needed for Wails, not for database layer
```

## Step 1: Project Initialization

Navigate to project root and initialize:

```bash
cd /home/brice/dev/projects/dumber

# Initialize with verbose output to see progress
dumber init --verbose
```

**Expected Output**:
```
✓ Initializing Go module: dumber
✓ Installing CLI dependencies: cobra, viper
✓ Installing database dependencies: ncruces/go-sqlite3, sqlc
✓ Installing validation: go-playground/validator
✓ Installing testing: testify
✓ Installing Wails v3-alpha... (requires CGO)
✓ Creating project structure...
✓ Generating configuration files...
✓ Verifying installation...

Project initialized successfully!

Next steps:
  1. Review generated configs/
  2. Run: go build ./cmd/dumber
  3. Start development: dumber --help
```

## Step 2: Verify Installation

Check that all components are correctly installed:

```bash
# Verify project status
dumber init status

# Expected output:
# Project Status: ✓ Initialized
# Dependencies: ✓ All installed
# Structure: ✓ Complete
# Build Status: ✓ Ready
```

Detailed verification:

```bash
# Check go.mod exists and is valid
test -f go.mod && echo "✓ go.mod exists"
grep -q "module dumber" go.mod && echo "✓ Module name correct"

# Verify dependencies are downloadable
go mod download && echo "✓ Dependencies resolved"

# Check tools are available
sqlc version && echo "✓ SQLC available"
wails version && echo "✓ Wails available"
```

## Step 3: Test CGO-Free Components

Most components can be tested without CGO enabled:

```bash
# Test CGO-free database layer
export CGO_ENABLED=0
go build ./internal/db && echo "✓ Database layer builds CGO-free"
go test ./internal/db && echo "✓ Database tests pass CGO-free"

# Test CLI components
go build ./internal/cli && echo "✓ CLI builds CGO-free" 
go test ./internal/cli && echo "✓ CLI tests pass CGO-free"

# Test URL parser
go build ./internal/parser && echo "✓ Parser builds CGO-free"
go test ./internal/parser && echo "✓ Parser tests pass CGO-free"
```

## Step 4: Test Wails Integration (CGO Required)

Test the full application with Wails:

```bash
# Enable CGO for Wails
export CGO_ENABLED=1

# Test full application build
go build ./cmd/dumber && echo "✓ Full build successful"

# Clean up test binary
rm -f dumber
```

## Step 5: Test Project Structure

Verify the correct directory structure was created:

```bash
# Check main directories exist
test -d cmd && echo "✓ cmd/ directory"
test -d internal && echo "✓ internal/ directory" 
test -d migrations && echo "✓ migrations/ directory"
test -d configs && echo "✓ configs/ directory"
test -d tests && echo "✓ tests/ directory"

# Check internal structure
test -d internal/cli && echo "✓ internal/cli/"
test -d internal/db && echo "✓ internal/db/"
test -d internal/parser && echo "✓ internal/parser/"
test -d internal/browser && echo "✓ internal/browser/"
```

## Step 6: Test Configuration Generation

Verify that configuration files were generated correctly:

```bash
# Check SQLC configuration
test -f configs/sqlc.yaml && echo "✓ SQLC config exists"
sqlc compile -f configs/sqlc.yaml && echo "✓ SQLC config valid"

# Check basic project files exist (if generated)
test -f cmd/dumber/main.go && echo "✓ Main entry point exists"
```

## Step 7: Test Module Dependencies

Verify the updated SQLite dependency:

```bash
# Check ncruces/go-sqlite3 is in go.mod (not mattn)
grep -q "github.com/ncruces/go-sqlite3" go.mod && echo "✓ Using CGO-free SQLite driver"
! grep -q "github.com/mattn/go-sqlite3" go.mod && echo "✓ Not using CGO SQLite driver"

# Test that database works CGO-free
export CGO_ENABLED=0
go run -c "import _ 'github.com/ncruces/go-sqlite3'" 2>/dev/null && echo "✓ SQLite driver loads CGO-free"
```

## Step 8: Validation Tests

Run tests with different CGO settings:

```bash
# Test CGO-free components
export CGO_ENABLED=0
go test ./internal/cli ./internal/parser ./internal/db && echo "✓ CGO-free tests pass"

# Test full application with CGO
export CGO_ENABLED=1  
go test ./... && echo "✓ All tests pass with CGO"

# Test with race detection (requires CGO)
go test -race ./... && echo "✓ No race conditions detected"
```

## Troubleshooting Common Issues

### Issue: Missing WebKit2GTK (Wails only)
**Symptom**: Wails installation fails with webkit error
**Solution** (Arch Linux):
```bash
sudo pacman -S webkit2gtk gtk3
dumber init deps --only=wails
```

### Issue: CGO disabled for Wails
**Symptom**: Wails build fails with CGO error  
**Solution**:
```bash
export CGO_ENABLED=1
go build ./cmd/dumber
```

### Issue: Wrong SQLite driver
**Symptom**: CGO errors from mattn/go-sqlite3
**Verification**:
```bash
# Should show ncruces, not mattn
grep sqlite3 go.mod
```

### Issue: Network timeout
**Symptom**: Dependency download timeouts
**Solution**:
```bash
go env -w GOPROXY=proxy.golang.org,direct
go env -w GOSUMDB=sum.golang.org
dumber init deps
```

## Success Criteria

After completing all steps, you should have:

- ✅ Valid Go module with CGO-free SQLite driver (`ncruces/go-sqlite3`)
- ✅ All CLI, parser, database components buildable with `CGO_ENABLED=0`
- ✅ Full application buildable with `CGO_ENABLED=1` (for Wails)
- ✅ Project directory structure created per constitution
- ✅ Configuration files generated and valid
- ✅ All tools (sqlc, wails) available and functional
- ✅ Clean separation: CGO only needed for browser layer

## Development Workflow Benefits

This CGO strategy provides several advantages:

1. **Fast development cycle**: Most development can happen CGO-free
2. **Better testing**: Database and CLI logic testable without CGO setup
3. **Cleaner builds**: Only final integration requires CGO
4. **Better performance**: `ncruces/go-sqlite3` is faster than `mattn/go-sqlite3`

## Next Steps

After successful initialization:

1. **Start with CGO-free components**: CLI structure, URL parsing, database schema
2. **Test incrementally**: Each component works independently
3. **Integrate with Wails last**: Final step requires CGO for browser integration
4. **Development workflow**: Most work can be done with `CGO_ENABLED=0`

The project is now ready for feature development with optimal CGO separation.