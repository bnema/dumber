# Implementation Plan: WebKit Memory Optimization

**Branch**: `005-webkit-memory-optimization` | **Date**: 2025-01-10 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/005-webkit-memory-optimization/spec.md`

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
**Primary Requirement**: Reduce WebKit browser memory usage from ~400MB to 150-250MB per instance (40-60% reduction) while maintaining browser stability and essential functionality.

**Technical Approach**: Implement WebKit memory pressure settings, optimize cache models, add periodic JavaScript garbage collection, enable process recycling, and provide configurable memory optimization presets for different usage scenarios.

## What’s Implemented So Far
Based on current branch diffs (uncommitted changes):
- Config: Added `MemoryConfig` to `pkg/webkit/settings.go` with thresholds, cache model, GC interval, page/offline cache toggles, monitoring, and recycling.
- WebKit integration (`pkg/webkit/webview_cgo.go`):
  - Applies `WebKitMemoryPressureSettings` (limit, thresholds, poll interval) to default `WebContext`.
  - Sets cache model (DocumentViewer/WebBrowser/Primary).
  - Toggles page cache and offline app cache.
  - Adds periodic JavaScript GC (ticker) and manual cleanup.
  - Tracks memory stats (page loads, last GC), exposes `GetMemoryStats()`.
  - Counts page loads and logs when recycle threshold is reached.
  - Hooks WebViews into a global memory manager when monitoring enabled.
- Memory manager (`pkg/webkit/memory.go`):
  - Scans `/proc` for WebKit-related processes, parses memory (`VmRSS`, `VmSize`, `VmPeak`).
  - Provides totals, detailed info, and recycling recommendation helpers.
  - Global manager lifecycle with background monitoring ticker.
- Presets and validation (`pkg/webkit/memory_example.go`): Memory-optimized, Balanced, High-performance configs and `ValidateMemoryConfig`.

Observations:
- Implementation aligns with spec goals: limits, cache tuning, GC, monitoring, and recycling are in place.
- Tests exist but are placeholders; contract/integration coverage for memory features is still missing.

## Technical Context
**Language/Version**: Go 1.25.1 with CGO (WebKit2GTK 6.0 bindings)
**Primary Dependencies**: WebKit2GTK 6.0, GTK4, JavaScriptCoreGTK 6.0, GLib/GObject  
**Storage**: Configuration files (memory settings), /proc filesystem (memory monitoring)  
**Testing**: Go testing framework, contract tests for WebKit memory APIs, integration tests with real WebKit processes  
**Target Platform**: Linux (Wayland/X11) with WebKit2GTK 6.0+ system packages
**Project Type**: single (WebKit browser enhancement)  
**Performance Goals**: 40-60% memory reduction (400MB → 150-250MB), <2s garbage collection impact, <500ms startup overhead  
**Constraints**: Maintain browser stability, preserve essential web functionality, no performance degradation >20%  
**Scale/Scope**: Single WebView instances, multiple browser windows, up to 100+ page loads per process lifecycle

## Constitution Check
*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Simplicity**:
- Projects: 1 (enhancing existing webkit package - no new projects)
- Using framework directly? YES (WebKit2GTK C APIs via CGO)
- Single data model? YES (MemoryConfig and MemoryStats structs)  
- Avoiding patterns? YES (direct WebKit API calls, no abstraction layers)

**Architecture**:
- EVERY feature as library? PARTIAL (webkit package is library, memory features enhance it)
- Libraries listed: pkg/webkit (WebKit2GTK Go bindings + memory optimization)
- CLI per library: N/A (browser features, not CLI commands)
- Library docs: Will update CLAUDE.md with memory optimization context

**Testing (NON-NEGOTIABLE)**:
- RED-GREEN-Refactor cycle enforced? YES (tests for memory limits, GC triggers, monitoring)
- Git commits show tests before implementation? YES (TDD approach planned)
- Order: Contract→Integration→E2E→Unit strictly followed? YES
- Real dependencies used? YES (actual WebKit processes, real memory monitoring)
- Integration tests for: YES (memory pressure settings, process recycling, GC effectiveness)
- FORBIDDEN: Implementation before test, skipping RED phase

**Observability**:
- Structured logging included? YES (using Go log package with memory monitoring flags)
- Frontend logs → backend? N/A (single process browser)
- Error context sufficient? YES (memory pressure events, GC failures, recycling triggers)

**Versioning**:
- Version number assigned? 005 (feature number, follows existing pattern)
- BUILD increments on every change? YES (follows project build process)
- Breaking changes handled? YES (backwards compatible config with defaults)

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
pkg/webkit/          # WebKit2GTK Go bindings + memory features
  - settings.go      # Config + MemoryConfig (added)
  - webview.go       # WebView public API
  - webview_cgo.go   # CGO integration (memory pressure, cache model, GC)
  - memory.go        # Memory manager (/proc, totals, recycling)
  - memory_example.go# Presets + validation helpers

tests/
  contract/          # Contract tests (to add)
  integration/       # Integration tests (to add)
  unit/              # Unit tests (placeholders present)

internal/, services/ # Existing project modules (unchanged)
frontend/            # Ancillary assets (unchanged)
```

**Structure Decision**: Single Go project — enhance `pkg/webkit` with memory features; tests live under `tests/*`.

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
- Contract tests from webkit_memory_api.go interfaces (5-6 tasks)
- Entity implementation tasks from data-model.md (4-5 tasks)
- Integration tests from quickstart.md scenarios (8-10 tasks)
- Memory optimization implementation tasks (6-8 tasks)

**Specific Task Categories**:
1. **Contract Tests** [P]: WebKitMemoryAPI, MemoryConfigAPI, MemoryStatsAPI, WebViewLifecycleAPI
2. **Data Model** [P]: MemoryConfig validation, MemoryStats tracking, ProcessMemoryInfo parsing
3. **WebKit Integration**: Memory pressure settings, cache model configuration, GC triggers
4. **Memory Monitoring**: Process memory tracking, statistics collection, recycling logic
5. **Configuration**: Preset management, validation rules, settings persistence
6. **Integration Tests**: Memory reduction validation, stability testing, performance benchmarks

**Ordering Strategy**:
- TDD order: Contract tests → Integration tests → Implementation → Unit tests
- Dependencies: MemoryConfig → WebKit integration → Memory monitoring → Statistics
- Parallel tasks [P]: Independent contract tests, separate entity implementations
- Sequential tasks: WebKit integration depends on configuration, monitoring depends on integration

**Estimated Output**: 22-28 numbered, ordered tasks in tasks.md

**IMPORTANT**: This phase is executed by the /tasks command, NOT by /plan

## Gaps & Next Steps
- Tests: Add contract tests for memory APIs and integration tests to verify 40–60% reduction and stability.
- CLI/Config wiring: Expose flags/env for presets and limits; persist user choices.
- Safeguards: Handle absence of memory APIs at runtime; version-checks where needed.
- Observability: Ensure structured logs; optionally add an API to fetch `MemoryStats`.
- CGO hygiene: Audit ownership/unref and goroutine shutdown paths; finalizeizers as needed.
- Benchmarks: Script baseline vs optimized memory and performance impact.
- Docs: Quickstart verification steps are present; add examples for presets and flags.

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
- [x] Phase 3: Tasks generated (tasks.md present)
- [ ] Phase 4: Implementation complete
- [ ] Phase 5: Validation passed

**Gate Status**:
- [x] Initial Constitution Check: PASS
- [x] Post-Design Constitution Check: PASS
- [x] All NEEDS CLARIFICATION resolved
- [ ] Complexity deviations documented

---
*Based on Constitution v2.1.1 - See `/memory/constitution.md`*
