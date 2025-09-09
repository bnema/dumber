# Implementation Plan: Project Initialization with Dependencies

**Branch**: `001-init-project` | **Date**: 2025-01-09 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/home/brice/dev/projects/dumber/specs/001-init-project/spec.md`

## Execution Flow (/plan command scope)
```
1. Load feature spec from Input path
   → If not found: ERROR "No feature spec at {path}"
2. Fill Technical Context (scan for NEEDS CLARIFICATION)
   → Detect Project Type from context (web=frontend+backend, mobile=app+api)
   → Set Structure Decision based on project type
3. Evaluate Constitution Check section below
   → If violations exist: Document in Complexity Tracking
   → If no justification possible: ERROR "Simplify approach first"
   → Update Progress Tracking: Initial Constitution Check
4. Execute Phase 0 → research.md
   → If NEEDS CLARIFICATION remain: ERROR "Resolve unknowns"
5. Execute Phase 1 → contracts, data-model.md, quickstart.md, agent-specific template file (e.g., `CLAUDE.md` for Claude Code, `.github/copilot-instructions.md` for GitHub Copilot, or `GEMINI.md` for Gemini CLI).
6. Re-evaluate Constitution Check section
   → If new violations: Refactor design, return to Phase 1
   → Update Progress Tracking: Post-Design Constitution Check
7. Plan Phase 2 → Describe task generation approach (DO NOT create tasks.md)
8. STOP - Ready for /tasks command
```

**IMPORTANT**: The /plan command STOPS at step 7. Phases 2-4 are executed by other commands:
- Phase 2: /tasks command creates tasks.md
- Phase 3-4: Implementation execution (manual or via tools)

## Summary
Initialize a Go project with all required dependencies for the dumber application, following the constitution's technical stack: Go + Wails v3-alpha + WebKit2GTK + SQLite with CLI (Cobra + Viper), database (SQLC), and validation (go-playground/validator) components. Use `go get @latest` for all dependencies.

## Technical Context
**User Details**: We need to add all deps using go get @latest
**Language/Version**: Go (latest compatible with Wails v3-alpha)
**Primary Dependencies**: Wails v3-alpha, Cobra, Viper, SQLC, go-playground/validator, SQLite driver
**Storage**: SQLite (local embedded database)
**Testing**: Go standard testing + testify for enhanced assertions
**Target Platform**: Linux Wayland (sway, hyprland) - primary focus
**Project Type**: single (desktop application with CLI + GUI components)
**Performance Goals**: < 500ms startup time, < 100MB baseline memory
**Constraints**: Single binary, WebKit2GTK system dependency only
**Scale/Scope**: Personal productivity tool, local history management (1000+ entries)

## Constitution Check
*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Simplicity**:
- Projects: 1 (single desktop app - dumber)
- Using framework directly? YES (Wails, Cobra, Viper directly)
- Single data model? YES (SQLite schema matches Go structs via SQLC)
- Avoiding patterns? YES (no unnecessary abstractions for initialization)

**Architecture**:
- EVERY feature as library? YES (CLI, parser, database, browser as separate packages)
- Libraries planned: cli (cobra commands), db (sqlc queries), parser (URL handling), browser (Wails integration)
- CLI per library: main CLI with subcommands (--dmenu, history, config)
- Library docs: llms.txt format will be generated post-implementation

**Testing (NON-NEGOTIABLE)**:
- RED-GREEN-Refactor cycle enforced? YES (tests for module initialization first)
- Git commits show tests before implementation? YES (test dependency resolution before implementation)
- Order: Contract→Integration→E2E→Unit strictly followed? YES
- Real dependencies used? YES (actual Go modules, SQLite DB)
- Integration tests for: module loading, dependency compatibility, build success
- FORBIDDEN: Implementation before test, skipping RED phase

**Observability**:
- Structured logging included? YES (slog for debugging initialization)
- Frontend logs → backend? N/A for initialization phase
- Error context sufficient? YES (proper error chains for dependency failures)

**Versioning**:
- Version number assigned? YES (v0.1.0 - initial development)
- BUILD increments on every change? YES (following semantic versioning)
- Breaking changes handled? N/A for initial setup

## Project Structure

### Documentation (this feature)
```
specs/[###-feature]/
├── plan.md              # This file (/plan command output)
├── research.md          # Phase 0 output (/plan command)
├── data-model.md        # Phase 1 output (/plan command)
├── quickstart.md        # Phase 1 output (/plan command)
├── contracts/           # Phase 1 output (/plan command)
└── tasks.md             # Phase 2 output (/tasks command - NOT created by /plan)
```

### Source Code (repository root)
```
# Option 1: Single project (DEFAULT)
src/
├── models/
├── services/
├── cli/
└── lib/

tests/
├── contract/
├── integration/
└── unit/

# Option 2: Web application (when "frontend" + "backend" detected)
backend/
├── src/
│   ├── models/
│   ├── services/
│   └── api/
└── tests/

frontend/
├── src/
│   ├── components/
│   ├── pages/
│   └── services/
└── tests/

# Option 3: Mobile + API (when "iOS/Android" detected)
api/
└── [same as backend above]

ios/ or android/
└── [platform-specific structure]
```

**Structure Decision**: Option 1 - Single project (desktop application with CLI and GUI components)

## Phase 0: Outline & Research
1. **Extract unknowns from Technical Context** above:
   - For each NEEDS CLARIFICATION → research task
   - For each dependency → best practices task
   - For each integration → patterns task

2. **Generate and dispatch research agents**:
   ```
   For each unknown in Technical Context:
     Task: "Research {unknown} for {feature context}"
   For each technology choice:
     Task: "Find best practices for {tech} in {domain}"
   ```

3. **Consolidate findings** in `research.md` using format:
   - Decision: [what was chosen]
   - Rationale: [why chosen]
   - Alternatives considered: [what else evaluated]

**Output**: research.md with all NEEDS CLARIFICATION resolved

## Phase 1: Design & Contracts
*Prerequisites: research.md complete*

1. **Extract entities from feature spec** → `data-model.md`:
   - Entity name, fields, relationships
   - Validation rules from requirements
   - State transitions if applicable

2. **Generate API contracts** from functional requirements:
   - For each user action → endpoint
   - Use standard REST/GraphQL patterns
   - Output OpenAPI/GraphQL schema to `/contracts/`

3. **Generate contract tests** from contracts:
   - One test file per endpoint
   - Assert request/response schemas
   - Tests must fail (no implementation yet)

4. **Extract test scenarios** from user stories:
   - Each story → integration test scenario
   - Quickstart test = story validation steps

5. **Update agent file incrementally** (O(1) operation):
   - Run `/scripts/update-agent-context.sh [claude|gemini|copilot]` for your AI assistant
   - If exists: Add only NEW tech from current plan
   - Preserve manual additions between markers
   - Update recent changes (keep last 3)
   - Keep under 150 lines for token efficiency
   - Output to repository root

**Output**: data-model.md, /contracts/*, failing tests, quickstart.md, agent-specific file

## Phase 2: Task Planning Approach
*This section describes what the /tasks command will do - DO NOT execute during /plan*

**Task Generation Strategy**:
- Load `/templates/tasks-template.md` as base
- Generate tasks from Phase 1 design docs (contracts, data model, quickstart)
- Prioritize CGO-free components first (faster development cycle)
- Each CLI command contract → contract test + implementation task
- Module initialization → dependency installation tasks
- Project structure → directory creation + config generation tasks
- Integration tests for each success criterion in quickstart

**Ordering Strategy**:
- TDD order: Tests before implementation 
- CGO-free first: CLI, parser, database (can develop with CGO_ENABLED=0)
- Wails integration last: Full application build (requires CGO_ENABLED=1)
- Dependency order: Go module → dependencies → structure → config → verification
- Mark [P] for parallel execution (independent dependency installations)

**Key Task Categories**:
1. **Module Setup Tasks**: go mod init, dependency installation
2. **Structure Tasks**: Directory creation, config file generation  
3. **Verification Tasks**: Build tests, tool availability checks
4. **Integration Tasks**: Full quickstart validation

**CGO Optimization**:
- Most tasks can run with CGO_ENABLED=0 for faster iteration
- Only final integration tests require CGO_ENABLED=1
- Clear separation allows parallel development of CGO-free components

**Estimated Output**: 15-20 numbered, ordered tasks in tasks.md (streamlined for initialization focus)

**IMPORTANT**: This phase is executed by the /tasks command, NOT by /plan

## Phase 3+: Future Implementation
*These phases are beyond the scope of the /plan command*

**Phase 3**: Task execution (/tasks command creates tasks.md)  
**Phase 4**: Implementation (execute tasks.md following constitutional principles)  
**Phase 5**: Validation (run tests, execute quickstart.md, performance validation)

## Complexity Tracking
*Fill ONLY if Constitution Check has violations that must be justified*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |


## Progress Tracking
*This checklist is updated during execution flow*

**Phase Status**:
- [x] Phase 0: Research complete (/plan command)
- [x] Phase 1: Design complete (/plan command)  
- [x] Phase 2: Task planning complete (/plan command - describe approach only)
- [ ] Phase 3: Tasks generated (/tasks command)
- [ ] Phase 4: Implementation complete
- [ ] Phase 5: Validation passed

**Gate Status**:
- [x] Initial Constitution Check: PASS
- [x] Post-Design Constitution Check: PASS  
- [x] All NEEDS CLARIFICATION resolved
- [x] Complexity deviations documented (none - simplicity maintained)

---
*Based on Constitution v2.1.1 - See `/memory/constitution.md`*