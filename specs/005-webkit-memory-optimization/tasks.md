# Tasks: WebKit Memory Optimization

**Input**: Design documents from `/specs/005-webkit-memory-optimization/`
**Prerequisites**: plan.md (required), research.md, data-model.md, contracts/

## Execution Flow (main)
```
1. Load plan.md from feature directory
   → Extract: Go 1.25.1 + CGO, WebKit2GTK 6.0, memory optimization approach
2. Load optional design documents:
   → data-model.md: MemoryConfig, MemoryStats, ProcessMemoryInfo, WebViewMemoryManager → model tasks
   → contracts/webkit_memory_api.go: 4 interfaces → contract test tasks
   → quickstart.md: 5 validation scenarios → integration test tasks
3. Generate tasks by category:
   → Setup: CGO configuration, WebKit dependencies, linting
   → Tests: contract tests, integration tests for memory reduction
   → Core: memory configuration, stats tracking, WebKit integration
   → Integration: process monitoring, GC management, recycling logic
   → Polish: unit tests, performance validation, documentation
4. Apply task rules:
   → Different files = mark [P] for parallel
   → WebKit integration = sequential (shared CGO context)
   → Tests before implementation (TDD)
5. Number tasks sequentially (T001, T002...)
6. Generate dependency graph
7. Create parallel execution examples
8. Validate: All contracts tested, all entities modeled, all scenarios covered
9. Return: SUCCESS (tasks ready for execution)
```

## Format: `[ID] [P?] Description`
- **[P]**: Can run in parallel (different files, no dependencies)
- Include exact file paths in descriptions

## Path Conventions
- **Single project**: `pkg/webkit/` for WebKit bindings enhancement
- Tests in `tests/contract/`, `tests/integration/`, `tests/unit/`
- Memory optimization files in `pkg/webkit/` package

## Phase 3.1: Setup
- [ ] T001 Verify WebKit2GTK 6.0+ dependencies and CGO build environment
- [ ] T002 Configure webkit_cgo build tags in existing project structure
- [ ] T003 [P] Update linting rules for CGO memory safety patterns

## Phase 3.2: Tests First (TDD) ⚠️ MUST COMPLETE BEFORE 3.3
**CRITICAL: These tests MUST be written and MUST FAIL before ANY implementation**
- [ ] T004 [P] Contract test WebKitMemoryAPI in tests/contract/test_webkit_memory_api_test.go
- [ ] T005 [P] Contract test MemoryConfigAPI in tests/contract/test_memory_config_api_test.go
- [ ] T006 [P] Contract test MemoryStatsAPI in tests/contract/test_memory_stats_api_test.go
- [ ] T007 [P] Contract test WebViewLifecycleAPI in tests/contract/test_webview_lifecycle_api_test.go
- [ ] T008 [P] Integration test memory reduction scenario in tests/integration/test_memory_reduction_test.go
- [ ] T009 [P] Integration test memory stability scenario in tests/integration/test_memory_stability_test.go
- [ ] T010 [P] Integration test process recycling scenario in tests/integration/test_process_recycling_test.go
- [ ] T011 [P] Integration test memory monitoring scenario in tests/integration/test_memory_monitoring_test.go
- [ ] T012 [P] Integration test optimization presets scenario in tests/integration/test_optimization_presets_test.go

## Phase 3.3: Core Implementation (ONLY after tests are failing)
- [ ] T013 [P] MemoryConfig struct with validation in pkg/webkit/memory_config.go
- [ ] T014 [P] ProcessMemoryInfo struct and parsing logic in pkg/webkit/process_memory.go
- [ ] T015 [P] Optimization preset configurations in pkg/webkit/memory_presets.go
- [ ] T016 WebViewMemoryManager singleton in pkg/webkit/memory_manager.go
- [ ] T017 WebKit memory pressure settings integration in pkg/webkit/webview_cgo.go
- [ ] T018 Cache model configuration in pkg/webkit/webview_cgo.go
- [ ] T019 JavaScript garbage collection triggers in pkg/webkit/webview_cgo.go
- [ ] T020 Page cache and offline cache controls in pkg/webkit/webview_cgo.go
- [ ] T021 Memory statistics tracking in pkg/webkit/webview_cgo.go

## Phase 3.4: Integration
- [ ] T022 Process memory monitoring via /proc filesystem in pkg/webkit/memory_manager.go
- [ ] T023 Memory pressure callback handling in pkg/webkit/webview_cgo.go
- [ ] T024 Periodic garbage collection scheduler in pkg/webkit/webview_cgo.go
- [ ] T025 Process recycling recommendation logic in pkg/webkit/memory_manager.go
- [ ] T026 Memory event logging and monitoring in pkg/webkit/memory_manager.go

## Phase 3.5: Polish
- [ ] T027 [P] Unit tests for MemoryConfig validation in tests/unit/test_memory_config_test.go
- [ ] T028 [P] Unit tests for ProcessMemoryInfo parsing in tests/unit/test_process_memory_test.go
- [ ] T029 [P] Unit tests for optimization presets in tests/unit/test_memory_presets_test.go
- [ ] T030 Performance validation: 40-60% memory reduction benchmark
- [ ] T031 [P] Update CLAUDE.md with memory optimization context
- [ ] T032 Memory safety review for CGO pointer management
- [ ] T033 Execute quickstart.md validation scenarios

## Dependencies
- Setup (T001-T003) before all other tasks
- Contract tests (T004-T007) before any implementation
- Integration tests (T008-T012) before implementation
- T013-T015 (data models) before T016-T021 (WebKit integration)
- T017-T021 (WebKit changes) are sequential (shared webview_cgo.go file)
- T022-T026 (integration features) depend on T016-T021
- Polish (T027-T033) after all core implementation

## Parallel Example
```
# Launch contract tests together (T004-T007):
Task: "Contract test WebKitMemoryAPI in tests/contract/test_webkit_memory_api_test.go"
Task: "Contract test MemoryConfigAPI in tests/contract/test_memory_config_api_test.go" 
Task: "Contract test MemoryStatsAPI in tests/contract/test_memory_stats_api_test.go"
Task: "Contract test WebViewLifecycleAPI in tests/contract/test_webview_lifecycle_api_test.go"

# Launch integration tests together (T008-T012):
Task: "Integration test memory reduction in tests/integration/test_memory_reduction_test.go"
Task: "Integration test memory stability in tests/integration/test_memory_stability_test.go"
Task: "Integration test process recycling in tests/integration/test_process_recycling_test.go"

# Launch data model implementations together (T013-T015):
Task: "MemoryConfig struct with validation in pkg/webkit/memory_config.go"
Task: "ProcessMemoryInfo struct and parsing in pkg/webkit/process_memory.go"
Task: "Optimization preset configurations in pkg/webkit/memory_presets.go"
```

## Notes
- WebKit2GTK 6.0 memory pressure APIs are the core technical dependency
- CGO memory safety requires careful pointer management and finalizers
- Tests must use real WebKit processes for accurate memory measurements
- Memory reduction target: 400MB → 150-250MB (40-60% reduction)
- All WebKit integration changes modify shared webview_cgo.go file (sequential)

## Task Generation Rules
*Applied during main() execution*

1. **From Contracts**:
   - WebKitMemoryAPI → T004 contract test, T017-T021 implementation
   - MemoryConfigAPI → T005 contract test, T013 implementation
   - MemoryStatsAPI → T006 contract test, T021 implementation
   - WebViewLifecycleAPI → T007 contract test, T025 implementation
   
2. **From Data Model**:
   - MemoryConfig entity → T013 model creation task [P]
   - ProcessMemoryInfo entity → T014 model creation task [P]
   - WebViewMemoryManager entity → T016 manager implementation
   - MemoryStats → integrated into WebView (T021)
   
3. **From Quickstart Scenarios**:
   - Memory reduction validation → T008 integration test [P]
   - Memory stability testing → T009 integration test [P]
   - Process recycling testing → T010 integration test [P]
   - Memory monitoring testing → T011 integration test [P]
   - Optimization presets testing → T012 integration test [P]

4. **Ordering**:
   - Setup → Contract tests → Integration tests → Models → WebKit integration → Polish
   - Data models before WebKit integration
   - WebKit integration tasks sequential due to shared file

## Validation Checklist
*GATE: Checked by main() before returning*

- [x] All contracts have corresponding tests (T004-T007)
- [x] All entities have model tasks (T013-T016)
- [x] All tests come before implementation (T004-T012 before T013+)
- [x] Parallel tasks truly independent (different files, no shared dependencies)
- [x] Each task specifies exact file path
- [x] No task modifies same file as another [P] task (webview_cgo.go tasks are sequential)
- [x] Memory reduction goal (40-60%) addressed in T008, T030
- [x] All quickstart scenarios covered (T008-T012, T033)
- [x] CGO memory safety addressed (T032)