# Tasks: Project Initialization with Dependencies

**Input**: Design documents from `/home/brice/dev/projects/dumber/specs/001-init-project/`
**Prerequisites**: plan.md, research.md, data-model.md, contracts/

## Execution Flow (main)
Project initialization requires setting up Go module, installing dependencies with `go get @latest`, creating directory structure, and verifying installation. Tasks follow TDD principles with contract tests before implementation.

## Format: `[ID] [P?] Description`
- **[P]**: Can run in parallel (different files, no dependencies)
- Include exact file paths in descriptions

## Path Conventions
Single project structure: `cmd/`, `internal/`, `tests/` at repository root per constitution

## Phase 3.1: Setup and Structure

- [ ] T001 Initialize Go module with `go mod init dumber` in project root
- [ ] T002 Create project directory structure: `cmd/dumber/`, `internal/{cli,db,parser,browser}/`, `migrations/`, `configs/`, `tests/{contract,integration,unit}/`
- [ ] T003 [P] Configure Git repository with .gitignore for Go projects

## Phase 3.2: Dependency Installation

- [ ] T004 [P] Install CLI dependencies: `go get github.com/spf13/cobra@latest github.com/spf13/viper@latest`
- [ ] T005 [P] Install CGO-free database dependency: `go get github.com/ncruces/go-sqlite3@latest`
- [ ] T006 [P] Install validation dependency: `go get github.com/go-playground/validator/v10@latest`
- [ ] T007 [P] Install testing dependency: `go get github.com/stretchr/testify@latest`
- [ ] T008 Install SQLC tool: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`
- [ ] T009 Install Wails v3-alpha tool: `go install github.com/wailsapp/wails/v3/cmd/wails@v3.0.0-alpha.28`

## Phase 3.3: Tests First (TDD) ⚠️ MUST COMPLETE BEFORE 3.4

**CRITICAL: These tests MUST be written and MUST FAIL before ANY implementation**

- [ ] T010 [P] Contract test for ModuleConfig validation in `tests/contract/test_module_config.go`
- [ ] T011 [P] Contract test for CLI init command in `tests/contract/test_cli_init.go`
- [ ] T012 [P] Contract test for dependency installation in `tests/contract/test_deps_install.go`
- [ ] T013 [P] Integration test for full project initialization in `tests/integration/test_full_init.go`
- [ ] T014 [P] Integration test for CGO-free database build in `tests/integration/test_cgo_free_build.go`
- [ ] T015 [P] Integration test for Wails build (CGO required) in `tests/integration/test_wails_build.go`

## Phase 3.4: Core Implementation (ONLY after tests are failing)

- [ ] T016 [P] ModuleConfig model in `internal/models/module.go`
- [ ] T017 [P] ProjectStructure model in `internal/models/structure.go`
- [ ] T018 [P] InstallationStatus model in `internal/models/status.go`
- [ ] T019 CLI root command setup in `cmd/dumber/main.go`
- [ ] T020 CLI init command implementation in `internal/cli/init.go`
- [ ] T021 Dependency installer service in `internal/services/installer.go`
- [ ] T022 Project structure creator in `internal/services/structure.go`
- [ ] T023 Configuration file generator in `internal/services/config.go`

## Phase 3.5: Configuration Files

- [ ] T024 [P] Generate SQLC configuration template in `configs/sqlc.yaml`
- [ ] T025 [P] Generate Wails configuration template in `configs/wails.json`
- [ ] T026 [P] Create initial database migration in `migrations/001_initial.sql`

## Phase 3.6: Verification and Integration

- [ ] T027 Build verification with CGO_ENABLED=0 for core components
- [ ] T028 Build verification with CGO_ENABLED=1 for Wails integration
- [ ] T029 Dependency resolution verification with `go mod download`
- [ ] T030 Tool availability verification: `sqlc version && wails version`

## Phase 3.7: Polish and Documentation

- [ ] T031 [P] Unit tests for validation logic in `tests/unit/test_validation.go`
- [ ] T032 [P] Unit tests for config generation in `tests/unit/test_config_gen.go`
- [ ] T033 [P] Error handling and logging implementation
- [ ] T034 Update quickstart.md with actual execution results
- [ ] T035 Run full quickstart validation script

## Dependencies

**Critical Path**:
- Setup (T001-T003) before dependencies (T004-T009)
- Dependencies before tests (T010-T015)
- Tests (T010-T015) before implementation (T016-T023)
- Core implementation before config files (T024-T026)
- Everything before verification (T027-T030)

**Specific Blockers**:
- T001 blocks T004-T009 (need go.mod first)
- T016-T018 block T021-T023 (need models for services)
- T020 blocks T027-T030 (need CLI for verification)

## Parallel Execution Examples

### Install Dependencies Concurrently
```bash
# Launch T004-T007 together (different packages):
Task: "Install CLI dependencies: go get github.com/spf13/cobra@latest github.com/spf13/viper@latest"
Task: "Install CGO-free database dependency: go get github.com/ncruces/go-sqlite3@latest"  
Task: "Install validation dependency: go get github.com/go-playground/validator/v10@latest"
Task: "Install testing dependency: go get github.com/stretchr/testify@latest"
```

### Write Tests Concurrently
```bash
# Launch T010-T015 together (different test files):
Task: "Contract test for ModuleConfig validation in tests/contract/test_module_config.go"
Task: "Contract test for CLI init command in tests/contract/test_cli_init.go"
Task: "Contract test for dependency installation in tests/contract/test_deps_install.go"
Task: "Integration test for full project initialization in tests/integration/test_full_init.go"
Task: "Integration test for CGO-free database build in tests/integration/test_cgo_free_build.go"
Task: "Integration test for Wails build (CGO required) in tests/integration/test_wails_build.go"
```

### Create Models Concurrently
```bash
# Launch T016-T018 together (different model files):
Task: "ModuleConfig model in internal/models/module.go"
Task: "ProjectStructure model in internal/models/structure.go"
Task: "InstallationStatus model in internal/models/status.go"
```

## CGO Optimization Strategy

**CGO-Free Development** (T004-T007, T010-T014, T016-T023):
- Most tasks can run with `CGO_ENABLED=0`
- Database layer uses ncruces/go-sqlite3 (CGO-free)
- CLI and parser components don't need CGO
- Faster development cycle for majority of work

**CGO Required** (T009, T015, T028):
- Only Wails installation and integration requires `CGO_ENABLED=1`
- WebKit2GTK bindings need CGO for native rendering
- Clear separation allows independent development

## Notes

- [P] tasks = different files, no dependencies
- Verify tests fail before implementing (TDD enforcement)
- Use `go get @latest` for all dependencies per user requirements
- Commit after each task completion
- CGO-free SQLite driver (ncruces) allows faster development cycle
- Only Wails final integration requires CGO

## Task Generation Rules Applied

1. **From Contracts**:
   - cli-commands.md → T011, T020 (CLI init command)
   - go-module.yaml → T010, T012 (module and dependency contracts)

2. **From Data Model**:
   - ModuleConfig → T016 (model creation)
   - ProjectStructure → T017 (model creation)
   - InstallationStatus → T018 (model creation)

3. **From Research Decisions**:
   - CGO-free SQLite → T005, T014 (specific driver selection)
   - Wails v3-alpha → T009, T015 (version-specific installation)

4. **From Quickstart Scenarios**:
   - Full initialization → T013 (integration test)
   - Build verification → T027, T028 (CGO separation tests)
   - Tool availability → T030 (verification scenario)

## Validation Checklist

- [x] All contracts have corresponding tests (T010-T012)
- [x] All entities have model tasks (T016-T018)
- [x] All tests come before implementation (T010-T015 before T016-T023)
- [x] Parallel tasks truly independent ([P] tasks use different files)
- [x] Each task specifies exact file path
- [x] No task modifies same file as another [P] task
- [x] TDD principles enforced (tests must fail before implementation)
- [x] CGO optimization strategy clearly defined
- [x] Dependencies properly ordered and documented