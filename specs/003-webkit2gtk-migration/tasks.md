# Tasks: WebKit2GTK Native Browser Migration

**Input**: Design documents from `/specs/003-webkit2gtk-migration/`
**Prerequisites**: plan.md (✓), research.md (✓), data-model.md (✓), contracts/ (✓), quickstart.md (✓)

## Phase 3.1: Setup

- [ ] T001 Clone WebKit2GTK reference repository to /home/brice/dev/clone/webkit2gtk-reference
- [ ] T002 Install WebKit2GTK and GTK3 development dependencies on system  
- [x] T003 [P] Create pkg/webkit package structure with basic Go files
- [x] T004 [P] Configure CGO build flags for WebKit2GTK and GTK3 in pkg/webkit
- [x] T005 [P] Setup linting and formatting tools for CGO code

## Phase 3.2: Tests First (TDD) ⚠️ MUST COMPLETE BEFORE 3.3

**CRITICAL: These tests MUST be written and MUST FAIL before ANY implementation**

- [x] T006 [P] Contract test NewWebView function in tests/contract/test_webview_creation.go
- [x] T007 [P] Contract test LoadURL navigation in tests/contract/test_navigation.go  
- [x] T008 [P] Contract test RegisterKeyboardShortcut in tests/contract/test_keyboard_shortcuts.go
- [x] T009 [P] Contract test InjectScript functionality in tests/contract/test_script_injection.go
- [x] T010 [P] Contract test SetZoom/GetZoom operations in tests/contract/test_zoom_control.go
- [x] T011 [P] Contract test Show/Hide/Destroy lifecycle in tests/contract/test_lifecycle.go
- [x] T012 [P] Integration test basic WebKit browser window in tests/integration/test_webkit_basic.go
- [x] T013 [P] Integration test google.fr navigation in tests/integration/test_google_navigation.go
- [x] T014 [P] Integration test keyboard shortcuts on external sites in tests/integration/test_external_shortcuts.go
- [x] T015 [P] Integration test zoom persistence with database in tests/integration/test_zoom_persistence.go

## Phase 3.3: Core Implementation (ONLY after tests are failing)

- [x] T016 [P] WebView struct and basic CGO wrapper in pkg/webkit/webview.go
- [x] T017 [P] GTK Application window management in pkg/webkit/window.go  
- [x] T018 [P] WebKit settings configuration in pkg/webkit/settings.go
- [x] T019 [P] Keyboard event handling with GTK in pkg/webkit/keyboard.go
- [x] T020 [P] JavaScript injection via UserContentManager in pkg/webkit/script.go
- [x] T021 [P] Zoom control WebKit API wrapper in pkg/webkit/zoom.go
- [x] T022 NewWebView constructor function
- [x] T023 LoadURL navigation implementation
- [x] T024 RegisterKeyboardShortcut GTK event binding
- [x] T025 InjectScript WebKit UserContentManager integration
- [x] T026 SetZoom/GetZoom WebKit API calls
- [x] T027 Show/Hide/Destroy lifecycle management
- [x] T028 CGO memory management and finalizers
- [ ] T029 Error handling and C to Go error propagation

## Phase 3.4: Integration

- [x] T030 WebKit browser service integration in services/browser_service.go
- [x] T031 Replace Wails ExecJS with WebKit InjectScript in services/browser_service.go
- [x] T032 Integrate WebKit zoom with existing database storage in services/browser_service.go
- [x] T033 Update main.go to use WebKit browser instead of Wails
- [x] T034 Remove Wails periodic script injection, use WebKit UserContentManager
- [x] T035 Update CLI commands to work with WebKit backend
- [x] T036 Preserve existing keyboard shortcut behavior through GTK events

## Phase 3.5: Migration & Polish

- [ ] T037 [P] Performance tests: startup time <500ms in tests/unit/test_performance.go
- [ ] T038 [P] Performance tests: memory usage <100MB baseline in tests/unit/test_memory.go  
- [ ] T039 [P] Unit tests for WebKit error handling in tests/unit/test_error_handling.go
- [ ] T040 [P] Unit tests for CGO memory management in tests/unit/test_memory_management.go
- [x] T041 Data preservation validation: existing history and zoom settings
- [x] T042 Remove Wails dependencies from go.mod and go.sum
- [ ] T043 Remove frontend TypeScript build system (frontend/ directory)
- [ ] T044 Remove Wails references from documentation and README
- [ ] T045 [P] Update pkg/webkit documentation in llms.txt format
- [ ] T046 Run quickstart.md validation scenarios
- [ ] T047 Performance comparison: WebKit vs Wails baseline metrics

## Dependencies

- Setup (T001-T005) before tests (T006-T015)
- Tests (T006-T015) before implementation (T016-T029) 
- T016 blocks T022, T030, T033
- T017 blocks T027, T033
- T018 blocks T022, T026
- T019 blocks T024, T036
- T020 blocks T025, T031
- T021 blocks T026, T032
- Core implementation (T016-T029) before integration (T030-T036)
- Integration (T030-T036) before migration (T042-T044)
- Implementation before polish (T037-T047)

## Parallel Example

```
# Launch T006-T011 together (contract tests):
Task: "Contract test NewWebView function in tests/contract/test_webview_creation.go"
Task: "Contract test LoadURL navigation in tests/contract/test_navigation.go"
Task: "Contract test RegisterKeyboardShortcut in tests/contract/test_keyboard_shortcuts.go"
Task: "Contract test InjectScript functionality in tests/contract/test_script_injection.go"
Task: "Contract test SetZoom/GetZoom operations in tests/contract/test_zoom_control.go"
Task: "Contract test Show/Hide/Destroy lifecycle in tests/contract/test_lifecycle.go"

# Launch T016-T021 together (core CGO wrappers):
Task: "WebView struct and basic CGO wrapper in pkg/webkit/webview.go"
Task: "GTK Application window management in pkg/webkit/window.go"
Task: "WebKit settings configuration in pkg/webkit/settings.go"
Task: "Keyboard event handling with GTK in pkg/webkit/keyboard.go"
Task: "JavaScript injection via UserContentManager in pkg/webkit/script.go"
Task: "Zoom control WebKit API wrapper in pkg/webkit/zoom.go"
```

## Notes

- [P] tasks = different files, no dependencies
- Verify all contract tests fail before implementing
- Commit after each task completion
- WebKit2GTK reference repository provides C API examples
- CGO memory management critical for stability
- Preserve all existing user data during migration

## Task Generation Rules

*Applied during main() execution*

1. **From Contracts**:
   - Each Go function in webkit-bindings-api.yaml → contract test task [P]
   - Each WebKit API operation → implementation task
   
2. **From Data Model**:
   - WebKit Browser Instance → WebView/Window/Settings tasks [P]
   - WebKit Bindings Interface → CGO wrapper tasks [P]
   - Migration State Manager → data preservation tasks
   
3. **From User Stories**:
   - google.fr navigation → integration test [P]
   - Keyboard shortcuts → integration test [P]
   - Zoom persistence → integration test [P]

4. **Ordering**:
   - Setup → Contract Tests → CGO Wrappers → API Functions → Integration → Migration → Polish
   - Dependencies block parallel execution

## Validation Checklist

*GATE: Checked by main() before returning*

- [x] All WebKit API contracts have corresponding tests
- [x] All data model entities have implementation tasks  
- [x] All tests come before implementation (TDD enforced)
- [x] Parallel tasks truly independent ([P] = different files)
- [x] Each task specifies exact file path
- [x] No task modifies same file as another [P] task
- [x] Migration preserves user data (history, zoom settings)
- [x] Performance requirements addressed (<500ms, <100MB)
