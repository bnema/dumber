# Implementation Plan: Native WebKit Browser Backend

**Branch**: `003-webkit2gtk-migration` | **Date**: 2025-01-21 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/home/brice/dev/projects/dumb-browser/specs/003-webkit2gtk-migration/spec.md`

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
Replace Wails framework with direct WebKit2GTK bindings to gain full control over browser engine capabilities, maintain all existing functionality while removing framework limitations, and create custom C Go bindings in a pkg/ package for potential future extraction.

## Technical Context
**Language/Version**: Go 1.25.1 + C bindings for WebKit2GTK  
**Primary Dependencies**: WebKit2GTK, GTK3, CGO for C bindings  
**Storage**: SQLite (maintain existing database schema and data)  
**Testing**: Go testing + WebKit integration tests  
**Target Platform**: Linux (Wayland/X11) - focus on GTK-based systems
**Project Type**: single - native desktop application  
**Performance Goals**: <500ms startup time, <100MB baseline memory, maintain existing performance  
**Constraints**: Remove all Wails dependencies, preserve user data, maintain keyboard shortcuts and browser features  
**Scale/Scope**: Single browser window, existing CLI functionality, WebKit2GTK reference clone for development

## Constitution Check
*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Simplicity**:
- Projects: 1 (single native browser application)
- Using framework directly? ✅ (Direct WebKit2GTK, removing Wails wrapper)
- Single data model? ✅ (Maintain existing SQLite schema)
- Avoiding patterns? ✅ (Direct C bindings, no unnecessary abstraction layers)

**Architecture**:
- EVERY feature as library? ✅ (pkg/ package for WebKit bindings, reusable)
- Libraries listed: pkg/webkit (WebKit2GTK Go bindings), existing internal packages
- CLI per library: Maintain existing CLI, add webkit package testing commands
- Library docs: llms.txt format planned for pkg/webkit

**Testing (NON-NEGOTIABLE)**:
- RED-GREEN-Refactor cycle enforced? ✅ (Contract tests for WebKit bindings first)
- Git commits show tests before implementation? ✅ (Will create failing WebKit tests)
- Order: Contract→Integration→E2E→Unit strictly followed? ✅ 
- Real dependencies used? ✅ (Real WebKit2GTK, actual browser engine)
- Integration tests for: WebKit bindings, browser functionality, keyboard shortcuts
- FORBIDDEN: Implementation before test, skipping RED phase

**Observability**:
- Structured logging included? ✅ (Continue using slog)
- Frontend logs → backend? ✅ (WebKit console logs to Go backend)
- Error context sufficient? ✅ (WebKit error propagation to Go)

**Versioning**:
- Version number assigned? Next version after current (maintain semver)
- BUILD increments on every change? ✅ 
- Breaking changes handled? ✅ (Migration from Wails to native, preserve user data)

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

**Structure Decision**: Option 1 (Single project) - Native desktop application with pkg/ for WebKit bindings

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
- Generate tasks from WebKit bindings contract (webkit-bindings-api.yaml)
- Create pkg/webkit package structure and CGO bridge tasks [P]
- WebKit2GTK reference repository setup and analysis tasks
- Contract tests for each WebKit API function [P]
- Integration tests for browser features (navigation, zoom, shortcuts)
- Migration tasks to remove Wails dependencies sequentially
- Performance validation and data preservation verification tasks

**Ordering Strategy**:
- TDD order: Contract tests before WebKit bindings implementation
- Dependency order: CGO bridge → WebKit API → Browser integration → Wails removal
- Parallel tasks: Independent WebKit functions, separate test files [P]
- Sequential migration: Preserve existing functionality throughout transition

**Estimated Output**: 30-35 numbered, ordered tasks focusing on WebKit migration in tasks.md

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
- [x] Complexity deviations documented (none required)

---
*Based on Constitution v2.1.1 - See `/memory/constitution.md`*