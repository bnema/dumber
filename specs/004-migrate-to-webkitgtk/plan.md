# Implementation Plan: Migrate to WebKitGTK 6.0

**Branch**: `004-migrate-to-webkitgtk` | **Date**: 2025-01-21 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/004-migrate-to-webkitgtk/spec.md`

## Execution Flow (/plan command scope)
```
1. Load feature spec from Input path
   → Successfully loaded spec for WebKitGTK 6.0 migration
2. Fill Technical Context (scan for NEEDS CLARIFICATION)
   → Project Type: Single (Go browser application)
   → Structure Decision: Option 1 (single project)
   → Open clarifications remain in spec (FR-008, FR-011, FR-012)
3. Evaluate Constitution Check section below
   → Adjust for remaining clarifications; document deviations
   → Update Progress Tracking: Initial Constitution Check
4. Execute Phase 0 → research.md (complete)
   → Research WebKit API differences and migration path
5. Execute Phase 1 → contracts, data-model.md, quickstart.md (complete)
6. Re-evaluate Constitution Check section
   → Design aligned with constitution principles; clarifications still pending
   → Update Progress Tracking: Post-Design Constitution Check
7. Plan Phase 2 → Describe task generation approach (DO NOT create tasks.md) (complete)
8. Generate tasks via /tasks (exists: tasks.md)
9. Track Phase 4 implementation progress (in progress)
```

## Summary
Migrate the dumber browser from WebKit2GTK 4.0 (GTK3) to WebKitGTK 6.0 (GTK4) to enable GPU rendering through Vulkan layer. The migration will involve incrementally updating the pkg/webkit package files while maintaining browser functionality. A global rendering configuration will be added to allow users to choose between CPU and GPU rendering, with GPU enabled by default when supported.

## Technical Context
**Language/Version**: Go 1.25.1  
**Primary Dependencies**: WebKitGTK 6.0, GTK4, libsoup3, SQLite3  
**Storage**: SQLite3 for history/settings, WebKitWebsiteDataManager for browser data  
**Testing**: Go tests, contract tests for WebKit bindings  
**Target Platform**: Linux (Wayland-first, X11 compatible)  
**Project Type**: single - Go browser application  
**Performance Goals**: < 500ms startup, improved GPU rendering performance  
**Constraints**: Maintain all existing functionality, graceful fallback for non-Vulkan systems  
**Scale/Scope**: ~15 files in pkg/webkit to migrate, ~3000 LOC

**User-Provided Context**: No TDD for this, we will start by checking the WebKit API differences and migrate pkg/webkit files one by one

## Current Snapshot (repo state)
- Build flags already target GTK4/WebKitGTK 6.0 in `pkg/webkit/*_cgo.go` ✓
- Core files present: `webview_cgo.go`, `window_cgo.go`, `script_cgo.go`, `scheme_cgo.go`, `zoom_cgo.go`, `keyboard_cgo.go`, `devtools_cgo.go`, `loop_cgo.go`, plus Go wrappers (`*.go`) ✓
- Migration underway in `webview_cgo.go` (GTK4 APIs, evaluate_javascript) ✓
- Spec still has [NEEDS CLARIFICATION] for performance targets and timeline ✗

## Constitution Check
*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Simplicity**:
- Projects: 1 (single Go browser project) ✓
- Using framework directly? YES (direct CGO bindings to WebKitGTK) ✓
- Single data model? YES (existing models unchanged) ✓
- Avoiding patterns? YES (no unnecessary abstractions) ✓

**Architecture**:
- EVERY feature as library? pkg/webkit is already a library ✓
- Libraries listed: pkg/webkit (WebKit2GTK Go bindings)
- CLI per library: Browser CLI with browse, history, dmenu commands ✓
- Library docs: Will update with new API documentation

**Testing (Modified per user request)**:
- User specified: "No TDD for this" - migration-focused approach
- Will add tests after migration for regression prevention
- Contract tests for new WebKit bindings
- Integration tests for browser functionality

**Observability**:
- Structured logging included? YES (slog) ✓
- Frontend logs → backend? N/A (native application)
- Error context sufficient? YES ✓

**Versioning**:
- Version number assigned? Will increment after migration
- BUILD increments on every change? YES ✓
- Breaking changes handled? API compatibility layer during migration

## Project Structure

### Documentation (this feature)
```
specs/004-migrate-to-webkitgtk/
├── plan.md              # This file (/plan command output)
├── research.md          # Phase 0 output (/plan command)
├── data-model.md        # Phase 1 output (/plan command)
├── quickstart.md        # Phase 1 output (/plan command)
├── contracts/           # Phase 1 output (/plan command)
└── tasks.md             # Phase 2 output (/tasks command - NOT created by /plan)
```

### Source Code (repository root)
```
# Option 1: Single project (SELECTED)
pkg/
└── webkit/              # WebKitGTK Go bindings (migration target)
    ├── webview_cgo.go   # WebView implementation (GTK4/WebKit 6)
    ├── window_cgo.go    # Window management (GTK4)
    ├── script_cgo.go    # JavaScript evaluation/injection
    ├── scheme_cgo.go    # Custom URI scheme handler
    ├── zoom_cgo.go      # Zoom control
    ├── keyboard_cgo.go  # Keyboard handling (controllers)
    ├── devtools_cgo.go  # Inspector
    ├── loop_cgo.go      # Main loop helpers
    ├── settings.go      # High-level settings glue
    └── *.go             # Non-CGO wrappers (window.go, zoom.go, script.go, ...)

internal/
├── cli/                 # CLI entry points (browse, history, dmenu)
├── config/              # App config (extend for rendering)
└── db/                  # SQLite persistence

tests/
├── contract/            # Contract tests for WebKit bindings
├── integration/         # End-to-end validation
└── unit/                # Unit tests
```

**Structure Decision**: Option 1 (Single project) - maintaining existing structure

## Phase 0: Outline & Research
1. **Extract unknowns from Technical Context**:
   - WebKitGTK 6.0 API changes from 4.0
   - GTK4 migration requirements
   - Vulkan renderer configuration
   - CGO binding updates needed
   - Build system changes (pkg-config)

2. **Generate and dispatch research agents**:
   ```
   Task: "Research WebKitGTK 6.0 API breaking changes"
   Task: "Find GTK3 to GTK4 migration patterns for CGO"
   Task: "Research Vulkan enablement in WebKitGTK 6.0"
   Task: "Identify deprecated WebKit2GTK 4.0 APIs"
   ```

3. **Consolidate findings** in `research.md`

**Output**: research.md with migration path defined

## Phase 1: Design & Contracts
*Prerequisites: research.md complete*

1. **API Mapping** → `data-model.md`:
   - Map WebKit2GTK 4.0 APIs to WebKitGTK 6.0 equivalents
   - Document removed/replaced functions
   - Identify new required APIs

2. **Generate migration contracts**:
   - Define compatibility layer interfaces
   - Create migration checkpoints
   - Output to `/contracts/`

3. **Migration test scenarios**:
   - Test each webkit file migration
   - Verify GPU acceleration
   - Fallback testing

4. **Create quickstart guide**:
   - Build requirements for GTK4/WebKitGTK 6.0
   - Verification steps
   - Performance testing

5. **Update CLAUDE.md**:
   - Add WebKitGTK 6.0 context
   - GTK4 migration notes
   - Recent changes

**Output**: data-model.md, /contracts/*, quickstart.md, CLAUDE.md updates

## File‑By‑File Migration Plan (grounded in repo)
- `pkg/webkit/build_flags_cgo.go` / `build_cgo_stub.go` / `build_flags_stub.go`: ensure pkg-config switched to `webkitgtk-6.0 gtk4 javascriptcoregtk-6.0` (present; verify includes and tags)
- `pkg/webkit/webview_cgo.go`: use `g_object_new` patterns as needed; ensure `webkit_web_view_evaluate_javascript` (async) usage; connect notify signals via GTK4; migrate any deprecated UCM message handlers.
- `pkg/webkit/window_cgo.go`: use `gtk_window_new()`, `gtk_window_set_child()`, and `gtk_window_destroy()`; remove `gtk_container_add()` remnants.
- `pkg/webkit/keyboard_cgo.go`: replace GdkEvent handlers with `GtkEventControllerKey` (key-pressed) and gestures for mouse input.
- `pkg/webkit/zoom_cgo.go` / `zoom.go`: confirm zoom ranges and GTK4 APIs for zoom if used; keep high-level validation in Go.
- `pkg/webkit/script_cgo.go` / `script.go`: standardize JS evaluation via `webkit_web_view_evaluate_javascript`.
- `pkg/webkit/scheme_cgo.go`: verify `WebKitURISchemeRequest` handling and memory ownership semantics on GTK4/WebKit 6.
- `pkg/webkit/devtools_cgo.go`: validate inspector APIs on WebKit 6.
- `pkg/webkit/loop_cgo.go`: update any deprecated main-loop helpers.
- `internal/config/*`: add `RenderingConfig` and `RenderingMode` with env/flag wiring.
- `internal/cli/browse.go`: add `--rendering-mode`, `--disable-gpu`, `--debug-gpu`.
- `services/*`: plumb rendering config into WebView creation.

## Phase 2: Task Planning Approach
*This section describes what the /tasks command will do - DO NOT execute during /plan*

**Task Generation Strategy**:
- Analyze each file in pkg/webkit for required changes
- Create migration task for each file
- Order by dependency (build files first, then core, then features)
- Add verification tasks after each major component

**Ordering Strategy**:
1. Build configuration (pkg-config, CGO flags)
2. Core types and structures
3. Window and WebView creation
4. Feature-specific files (zoom, script, keyboard)
5. Integration testing
6. Performance verification

**Estimated Output**: 20-25 migration tasks in tasks.md

**IMPORTANT**: This phase is executed by the /tasks command, NOT by /plan

## Phase 3+: Future Implementation
*These phases are beyond the scope of the /plan command*

**Phase 3**: Task execution (/tasks command creates tasks.md)  
**Phase 4**: Implementation (execute migration tasks)  
**Phase 5**: Validation (verify GPU acceleration, run performance tests)

## Risks & Mitigations
- Driver variability: prefer ON_DEMAND policy by default; expose override flags; add CPU fallback on init failures.
- API drift: adhere to contracts in `contracts/webkit-api.yaml`; gate changes behind compile tags where helpful.
- Performance regressions: provide quickstart validation and capture metrics; keep rollback point per task.

## Complexity Tracking
*Fill ONLY if Constitution Check has violations that must be justified*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| No TDD for migration | User-specified approach | TDD would slow down API exploration during migration |

## Progress Tracking
*This checklist is updated during execution flow*

**Phase Status**:
- [x] Phase 0: Research complete (/plan command)
- [x] Phase 1: Design complete (/plan command)
- [x] Phase 2: Task planning complete (/plan command - describe approach only)
- [x] Phase 3: Tasks generated (/tasks command)
- [ ] Phase 4: Implementation in progress
- [ ] Phase 5: Validation passed

**Gate Status**:
- [x] Initial Constitution Check: PASS
- [x] Post-Design Constitution Check: PASS (performance targets pending)
- [ ] All NEEDS CLARIFICATION resolved (see spec FR-008, FR-011, FR-012)
- [x] Complexity deviations documented

## Next Actions
- Verify and finalize build flags and includes across `pkg/webkit/*_cgo.go`.
- Align `internal/config` and `internal/cli` with `RenderingConfig` and flags.
- Complete event/gesture migration in `keyboard_cgo.go` and related handlers.
- Add hardware acceleration policy mapping during WebView initialization.

---
*Based on Constitution v1.0.0 - See `/memory/constitution.md`*
