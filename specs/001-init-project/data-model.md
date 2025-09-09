# Data Model: Project Initialization

**Date**: 2025-01-09  
**Context**: Data structures for dumber project initialization

## Core Entities

### 1. Go Module Configuration
**Purpose**: Represents the initialized Go module and its metadata

**Structure**:
```go
type ModuleConfig struct {
    Name         string `validate:"required"`           // "dumber"
    Version      string `validate:"required,semver"`    // "v0.1.0"
    GoVersion    string `validate:"required"`           // Go version requirement
    Dependencies []Dependency `validate:"dive"`
}

type Dependency struct {
    ImportPath string `validate:"required,uri"`        // e.g. "github.com/spf13/cobra"
    Version    string `validate:"required"`            // "@latest" or specific version
    Type       DependencyType `validate:"required"`    // runtime, dev, tool
}

type DependencyType int
const (
    RuntimeDep DependencyType = iota  // Core application dependencies
    DevDep                           // Development/testing tools  
    ToolDep                          // CLI tools (sqlc, wails)
)
```

**Validation Rules**:
- Module name must be valid Go identifier
- Version must follow semantic versioning
- All dependency paths must be valid import paths
- No circular dependencies allowed

### 2. Project Structure
**Purpose**: Defines the directory layout and file organization

**Structure**:
```go
type ProjectStructure struct {
    RootPath     string `validate:"required,dir"`
    Directories  []Directory `validate:"dive"`
    ConfigFiles  []ConfigFile `validate:"dive"`
    InitialFiles []InitialFile `validate:"dive"`
}

type Directory struct {
    Path        string `validate:"required"`           // relative to root
    Purpose     string `validate:"required"`           // description
    Required    bool                                   // must be created
}

type ConfigFile struct {
    Path        string `validate:"required"`           // relative to root
    Template    string `validate:"required"`           // template name
    Variables   map[string]string                      // template variables
}

type InitialFile struct {
    Path     string `validate:"required"`              // relative to root
    Content  string                                    // file content
    Type     FileType `validate:"required"`           // go, sql, yaml, etc.
}
```

### 3. Dependency Installation Status
**Purpose**: Tracks installation progress and success/failure states

**Structure**:
```go
type InstallationStatus struct {
    ModuleName   string `validate:"required"`
    Dependencies []DependencyStatus `validate:"dive"`
    Tools        []ToolStatus `validate:"dive"`
    Overall      StatusType `validate:"required"`
    Timestamp    time.Time `validate:"required"`
    Errors       []InstallError
}

type DependencyStatus struct {
    ImportPath   string `validate:"required"`
    Version      string
    Status       StatusType `validate:"required"`
    InstallTime  time.Duration                        // how long to install
    Error        *InstallError
}

type ToolStatus struct {
    Name         string `validate:"required"`          // "sqlc", "wails"
    Command      string `validate:"required"`          // installation command
    Status       StatusType `validate:"required"`
    Version      string                                // installed version
    Error        *InstallError
}

type StatusType int
const (
    NotStarted StatusType = iota
    InProgress
    Success
    Failed
    Skipped
)

type InstallError struct {
    Message     string `validate:"required"`
    Code        int
    Suggestion  string                                 // helpful hint for user
}
```

## State Transitions

### Module Initialization Flow
```
Empty Directory → Module Init → Dependency Resolution → Tool Installation → Verification → Complete
                     ↓              ↓                      ↓                ↓
                  [go.mod]      [go.mod updated]      [tools available]  [build succeeds]
```

### Dependency Installation States
```
NotStarted → InProgress → Success
             ↓
             Failed (with retry option)
```

## Relationships

1. **ModuleConfig** contains multiple **Dependencies**
2. **ProjectStructure** defines where **ConfigFiles** are placed
3. **InstallationStatus** tracks the progress of **ModuleConfig** setup
4. Each **Directory** in **ProjectStructure** serves specific **Dependencies**

## Validation Constraints

### Business Rules
1. **Module name uniqueness**: No conflicts with existing Go modules in workspace
2. **Dependency compatibility**: All dependencies must support the same Go version
3. **Tool availability**: Required tools (sqlc, wails) must be installable
4. **Directory permissions**: All target directories must be writable
5. **Disk space**: Adequate space for dependencies and generated files

### Technical Constraints
1. **Go version**: Must support generics (Go 1.18+)
2. **CGO requirement**: SQLite driver requires CGO_ENABLED=1
3. **System dependencies**: Wails requires WebKit2GTK development headers
4. **Network access**: Dependency download requires internet connectivity

## Integration Points

### With CLI Commands
- `dumber init` → Creates **ModuleConfig** and **ProjectStructure**
- `dumber deps install` → Updates **InstallationStatus**
- `dumber deps verify` → Validates all **Dependencies**

### With Configuration Files
- **go.mod**: Generated from **ModuleConfig**
- **sqlc.yaml**: Generated from database **Dependencies**
- **wails.json**: Generated from Wails **Dependencies**

### With Testing Framework
- Integration tests validate each **StatusType** transition
- Contract tests verify **ConfigFile** template generation
- Unit tests cover **Dependency** validation logic

This data model supports the initialization phase while preparing for the next development phases (CLI implementation, database setup, browser integration).