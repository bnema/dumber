# Tasks: Browser Controls UI

**Input**: Design documents from `/home/brice/dev/projects/dumb-browser/specs/002-browser-controls-ui/`
**Prerequisites**: plan.md, research.md, data-model.md, contracts/browser-controls-api.yaml

## Feature Summary
Implement keyboard and mouse navigation controls for the Wails-based browser: zoom in/out with Ctrl+/-, back/forward navigation with mouse buttons, URL copying with Ctrl+Shift+C, and dynamic window title updates with SQLite persistence.

## Path Conventions
Single project structure (desktop application):
- **Source**: `src/models/`, `src/services/`, `src/cli/`, `src/lib/`
- **Tests**: `tests/contract/`, `tests/integration/`, `tests/unit/`
- **Frontend**: `frontend/src/`
- **Database**: SQLite with SQLC generated queries

## Phase 3.1: Setup

- [ ] T001 Create zoom_levels database table with SQLite schema migration in `src/db/migrations/003_add_zoom_levels.sql`
- [ ] T002 Generate SQLC queries for zoom level operations in `src/db/queries/zoom_levels.sql`  
- [ ] T003 [P] Add frontend JavaScript event handlers in `frontend/src/events/keyboard.js`
- [ ] T004 [P] Add frontend mouse event handlers in `frontend/src/events/mouse.js`

## Phase 3.2: Tests First (TDD) ⚠️ MUST COMPLETE BEFORE 3.3

**CRITICAL: These tests MUST be written and MUST FAIL before ANY implementation**

### Contract Tests [P]
- [ ] T005 [P] Contract test POST /zoom/in endpoint in `tests/contract/zoom_in_test.go`
- [ ] T006 [P] Contract test POST /zoom/out endpoint in `tests/contract/zoom_out_test.go`
- [ ] T007 [P] Contract test POST /navigation/back endpoint in `tests/contract/navigation_back_test.go`
- [ ] T008 [P] Contract test POST /navigation/forward endpoint in `tests/contract/navigation_forward_test.go`
- [ ] T009 [P] Contract test POST /clipboard/copy-url endpoint in `tests/contract/clipboard_copy_test.go`
- [ ] T010 [P] Contract test GET /zoom/get-level endpoint in `tests/contract/zoom_get_test.go`

### Integration Tests [P]
- [ ] T011 [P] Integration test zoom persistence across domains in `tests/integration/zoom_persistence_test.go`
- [ ] T012 [P] Integration test keyboard event handling in `tests/integration/keyboard_events_test.go`
- [ ] T013 [P] Integration test mouse navigation in `tests/integration/mouse_navigation_test.go`
- [ ] T014 [P] Integration test clipboard tool fallback chain in `tests/integration/clipboard_fallback_test.go`
- [ ] T015 [P] Integration test window title updates in `tests/integration/window_title_test.go`

## Phase 3.3: Core Implementation (ONLY after tests are failing)

### Data Models [P]
- [ ] T016 [P] ZoomLevel model with validation in `src/models/zoom_level.go`
- [ ] T017 [P] BrowserState model extensions in `src/models/browser_state.go`
- [ ] T018 [P] KeyboardEvent model for event processing in `src/models/keyboard_event.go`
- [ ] T019 [P] MouseEvent model for navigation in `src/models/mouse_event.go`

### Core Services [P]
- [ ] T020 [P] ZoomService with Firefox zoom levels in `src/services/zoom_service.go`
- [ ] T021 [P] NavigationService with history management in `src/services/navigation_service.go`
- [ ] T022 [P] ClipboardService with tool fallback in `src/services/clipboard_service.go`
- [ ] T023 [P] WindowTitleService for dynamic updates in `src/services/window_title_service.go`

### Wails Service Layer
- [ ] T024 BrowserControlService main service in `src/services/browser_control_service.go`
- [ ] T025 ZoomIn method implementation with domain persistence
- [ ] T026 ZoomOut method implementation with bounds checking
- [ ] T027 NavigateBack method with history validation
- [ ] T028 NavigateForward method with forward history
- [ ] T029 CopyCurrentURL method with clipboard tool detection
- [ ] T030 GetZoomLevel method for domain lookup

### Database Layer
- [ ] T031 ZoomLevelRepository with SQLC integration in `src/repositories/zoom_level_repository.go`
- [ ] T032 Database transaction management for zoom operations
- [ ] T033 Cleanup job for old zoom level entries

## Phase 3.4: Integration

### Frontend Integration
- [ ] T034 Wire keyboard events to Wails backend calls in `frontend/src/events/keyboard.js`
- [ ] T035 Wire mouse events to navigation methods in `frontend/src/events/mouse.js`
- [ ] T036 CSS zoom application with visual feedback in `frontend/src/styles/zoom.css`
- [ ] T037 Window title update integration with page changes

### Backend Integration
- [ ] T038 Register BrowserControlService with Wails context in `src/app.go`
- [ ] T039 Database migration execution on startup
- [ ] T040 Error handling and logging for all service methods
- [ ] T041 Service method validation and bounds checking

## Phase 3.5: Polish

### Unit Tests [P]
- [ ] T042 [P] Unit tests for Firefox zoom level validation in `tests/unit/zoom_levels_test.go`
- [ ] T043 [P] Unit tests for clipboard tool detection in `tests/unit/clipboard_tools_test.go`
- [ ] T044 [P] Unit tests for navigation history bounds in `tests/unit/navigation_bounds_test.go`
- [ ] T045 [P] Unit tests for domain validation in `tests/unit/domain_validation_test.go`

### Performance & Documentation
- [ ] T046 Performance test: zoom operations < 100ms response in `tests/performance/zoom_perf_test.go`
- [ ] T047 Performance test: navigation < 200ms response in `tests/performance/nav_perf_test.go`
- [ ] T048 Memory usage test: < 10MB additional usage over 10 minutes
- [ ] T049 Update README with new keyboard shortcuts and features
- [ ] T050 Run quickstart.md validation scenarios

## Dependencies

**Critical Dependencies:**
- Database setup (T001-T002) blocks all repository tasks (T031-T033)
- All tests (T005-T015) MUST complete and FAIL before implementation (T016-T041)
- Models (T016-T019) block services (T020-T023)
- Services block Wails integration (T024-T030)
- Core implementation blocks frontend integration (T034-T041)

**Parallel Groups:**
- Setup frontend events: T003, T004
- Contract tests: T005-T010
- Integration tests: T011-T015
- Data models: T016-T019
- Core services: T020-T023
- Unit tests: T042-T045

## Parallel Execution Examples

```bash
# Phase 3.1 Setup (parallel frontend)
Task: "Add frontend JavaScript event handlers in frontend/src/events/keyboard.js"
Task: "Add frontend mouse event handlers in frontend/src/events/mouse.js"

# Phase 3.2 Contract Tests (all parallel)
Task: "Contract test POST /zoom/in endpoint in tests/contract/zoom_in_test.go"
Task: "Contract test POST /zoom/out endpoint in tests/contract/zoom_out_test.go" 
Task: "Contract test POST /navigation/back endpoint in tests/contract/navigation_back_test.go"
Task: "Contract test POST /navigation/forward endpoint in tests/contract/navigation_forward_test.go"
Task: "Contract test POST /clipboard/copy-url endpoint in tests/contract/clipboard_copy_test.go"
Task: "Contract test GET /zoom/get-level endpoint in tests/contract/zoom_get_test.go"

# Phase 3.2 Integration Tests (all parallel)
Task: "Integration test zoom persistence across domains in tests/integration/zoom_persistence_test.go"
Task: "Integration test keyboard event handling in tests/integration/keyboard_events_test.go"
Task: "Integration test mouse navigation in tests/integration/mouse_navigation_test.go"
Task: "Integration test clipboard tool fallback chain in tests/integration/clipboard_fallback_test.go"
Task: "Integration test window title updates in tests/integration/window_title_test.go"

# Phase 3.3 Data Models (all parallel)
Task: "ZoomLevel model with validation in src/models/zoom_level.go"
Task: "BrowserState model extensions in src/models/browser_state.go"
Task: "KeyboardEvent model for event processing in src/models/keyboard_event.go"
Task: "MouseEvent model for navigation in src/models/mouse_event.go"

# Phase 3.3 Core Services (all parallel)
Task: "ZoomService with Firefox zoom levels in src/services/zoom_service.go"
Task: "NavigationService with history management in src/services/navigation_service.go"
Task: "ClipboardService with tool fallback in src/services/clipboard_service.go"
Task: "WindowTitleService for dynamic updates in src/services/window_title_service.go"
```

## Firefox Zoom Levels Reference
Standard zoom progression: 30%, 50%, 67%, 80%, 90%, 100%, 110%, 120%, 133%, 150%, 170%, 200%, 240%, 300%, 400%, 500%

## Clipboard Tool Fallback Chain
1. `wlcopy` (Wayland native)
2. `xclip` (X11 primary)
3. `xsel` (X11 fallback)

## Database Schema
```sql
CREATE TABLE zoom_levels (
    domain TEXT PRIMARY KEY,
    zoom_factor REAL NOT NULL DEFAULT 1.0 CHECK(zoom_factor >= 0.3 AND zoom_factor <= 5.0),
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_zoom_levels_updated_at ON zoom_levels(updated_at);
```

## Validation Checklist
- [x] All contracts have corresponding tests (T005-T010)
- [x] All entities have model tasks (T016-T019)  
- [x] All tests come before implementation (Phase 3.2 before 3.3)
- [x] Parallel tasks truly independent (different files)
- [x] Each task specifies exact file path
- [x] No task modifies same file as another [P] task

## Success Criteria
- All quickstart.md test scenarios pass
- Performance targets met (< 100ms zoom, < 200ms navigation)
- Memory usage within constitutional limits (< 10MB additional)
- Cross-platform clipboard compatibility maintained
- No regressions in existing browser functionality

**Total Tasks**: 50 tasks across 5 phases
**Estimated Completion**: 2-3 days with parallel execution
**Critical Path**: Database setup → Tests → Models → Services → Integration