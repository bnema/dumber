# Tasks: Migrate to WebKitGTK 6.0

**Input**: Design documents from `/specs/004-migrate-to-webkitgtk/`
**Prerequisites**: plan.md (required), research.md, data-model.md, contracts/

## Execution Flow (main)
```
1. Load plan.md from feature directory
   → Loaded: WebKitGTK 6.0 migration plan
   → Extract: Go 1.25.1, WebKitGTK 6.0, GTK4, no TDD
2. Load optional design documents:
   → data-model.md: RenderingConfig model
   → contracts/: webkit-api.yaml, rendering-config.yaml
   → research.md: API migration mappings
3. Generate tasks by category:
   → Setup: Build config, dependencies
   → Core: Migrate webkit files incrementally
   → Integration: Rendering config, CLI flags
   → Verification: Test GPU acceleration
4. Apply task rules:
   → Different files = mark [P] for parallel
   → Same file = sequential (no [P])
   → No TDD per user request
5. Number tasks sequentially (T001, T002...)
6. Generate dependency graph
7. Create parallel execution examples
8. Validate task completeness:
   → All webkit files migrated?
   → Rendering config implemented?
   → GPU acceleration verified?
9. Return: SUCCESS (tasks ready for execution)
```

## Format: `[ID] [P?] Description`
- **[P]**: Can run in parallel (different files, no dependencies)
- Include exact file paths in descriptions

## Path Conventions
- **Single project**: `pkg/webkit/` for WebKit bindings
- **Config**: `internal/config/` for rendering configuration
- **CLI**: `internal/cli/` for command-line flags

## Phase 3.1: Setup & Build Configuration
- [ ] T001 Update pkg-config in all CGO files from webkit2gtk-4.0 to webkitgtk-6.0
- [ ] T002 Update GTK pkg-config from gtk+-3.0 to gtk4 in all CGO files
- [ ] T003 Update JavaScript pkg-config from javascriptcoregtk-4.0 to javascriptcoregtk-6.0
- [ ] T004 Update Makefile build flags for GTK4/WebKitGTK 6.0
- [ ] T005 Verify build environment has GTK4 and WebKitGTK 6.0 installed

## Phase 3.2: Rendering Configuration
- [ ] T006 [P] Add RenderingConfig struct to pkg/webkit/settings.go
- [ ] T007 [P] Add RenderingMode constants and validation to internal/config/config.go
- [ ] T008 [P] Add --rendering-mode CLI flag to internal/cli/browse.go
- [ ] T009 [P] Add DUMBER_RENDERING_MODE environment variable support to internal/config/config.go

## Phase 3.3: Core WebKit Migration - Build Files
- [ ] T010 [P] Migrate pkg/webkit/build_flags_cgo.go to WebKitGTK 6.0 headers
- [ ] T011 [P] Migrate pkg/webkit/build_cgo_stub.go build tags for GTK4
- [ ] T012 [P] Update pkg/webkit/build_flags_stub.go for new build configuration

## Phase 3.4: Core WebKit Migration - Main Components
- [ ] T013 Migrate pkg/webkit/webview_cgo.go - Update WebView creation to use g_object_new instead of webkit_web_view_new_with_context
- [ ] T014 Migrate pkg/webkit/webview_cgo.go - Replace WebKitWebContext with WebKitNetworkSession
- [ ] T015 Migrate pkg/webkit/webview_cgo.go - Add hardware acceleration policy settings
- [ ] T016 Migrate pkg/webkit/window_cgo.go - Update gtk_window_new() without GTK_WINDOW_TOPLEVEL
- [ ] T017 Migrate pkg/webkit/window_cgo.go - Replace gtk_container_add with gtk_window_set_child
- [ ] T018 Migrate pkg/webkit/window_cgo.go - Update gtk_widget_destroy to gtk_window_destroy

## Phase 3.5: Core WebKit Migration - Event Handling
- [ ] T019 Migrate pkg/webkit/keyboard_cgo.go - Replace GdkEventKey with GtkEventController
- [ ] T020 Update pkg/webkit/keyboard_cgo.go - Migrate key-press-event to key-pressed signal
- [ ] T021 Migrate button-press-event to GtkGestureClick in pkg/webkit/webview_cgo.go

## Phase 3.6: Feature Migration
- [ ] T022 [P] Update pkg/webkit/script_cgo.go for GTK4 compatibility
- [ ] T023 [P] Update pkg/webkit/zoom_cgo.go for GTK4 compatibility
- [ ] T024 [P] Update pkg/webkit/devtools_cgo.go for GTK4 compatibility
- [ ] T025 [P] Update pkg/webkit/loop_cgo.go for GTK4 main loop
- [ ] T026 [P] Update pkg/webkit/scheme_cgo.go for new URI scheme handling

## Phase 3.7: Integration & Services
- [ ] T027 Update services/browser_service.go to pass RenderingConfig to WebView
- [ ] T028 Update services/config_service.go to handle rendering mode configuration
- [ ] T029 Add GPU detection logic to determine auto mode in pkg/webkit/webview.go

## Phase 3.8: Testing & Verification
- [ ] T030 Create test for GPU acceleration detection in tests/integration/test_gpu_rendering.go
- [ ] T031 Create test for CPU fallback in tests/integration/test_cpu_fallback.go
- [ ] T032 Update existing integration tests for GTK4 compatibility
- [ ] T033 Manual test: Verify browser starts with GTK4/WebKitGTK 6.0
- [ ] T034 Manual test: Verify GPU acceleration with WebGL content
- [ ] T035 Manual test: Verify CPU fallback with --rendering-mode=cpu

## Phase 3.9: Documentation & Cleanup
- [ ] T036 [P] Update README.md with new build requirements
- [ ] T037 [P] Document rendering mode configuration in docs/
- [ ] T038 Remove deprecated WebKit2GTK 4.0 compatibility code
- [ ] T039 Update CLAUDE.md to reflect completed migration

## Dependencies
- Build config (T001-T005) must complete first
- Rendering config (T006-T009) can run in parallel
- Core migration (T010-T026) depends on build config
- T013-T015 must be sequential (same file)
- T016-T018 must be sequential (same file)
- Integration (T027-T029) depends on core migration
- Testing (T030-T035) depends on all implementation
- Documentation (T036-T039) can start anytime

## Parallel Example
```bash
# Launch T006-T009 together (different files):
Task: "Add RenderingConfig struct to pkg/webkit/settings.go"
Task: "Add RenderingMode constants to internal/config/config.go"
Task: "Add --rendering-mode CLI flag to internal/cli/browse.go"
Task: "Add DUMBER_RENDERING_MODE env var to internal/config/config.go"

# Launch T022-T026 together (independent files):
Task: "Update pkg/webkit/script_cgo.go for GTK4"
Task: "Update pkg/webkit/zoom_cgo.go for GTK4"
Task: "Update pkg/webkit/devtools_cgo.go for GTK4"
Task: "Update pkg/webkit/loop_cgo.go for GTK4"
Task: "Update pkg/webkit/scheme_cgo.go for URI handling"
```

## Notes
- No TDD per user request - migrate first, test after
- Focus on incremental file-by-file migration
- Test GPU acceleration after each major component
- Commit after each task to track progress
- Use quickstart.md for verification steps

## Critical Migration Points
1. **WebView Creation**: Must update to g_object_new pattern
2. **Network Session**: Replace all WebContext usage
3. **Event Handling**: GTK4 uses controllers not direct events
4. **Container API**: gtk_container_add no longer exists
5. **Hardware Acceleration**: Must configure policy correctly