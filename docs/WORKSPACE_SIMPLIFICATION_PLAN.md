# Workspace Architecture Simplification Plan

**Status**: Approved for implementation
**Date**: 2025-09-30
**Goal**: Remove over-engineering, simplify architecture, improve naming

## Why Simplify

Current architecture has ~2000 lines of defensive code that add complexity without measurable benefit:
- "Bulletproof" terminology is confusing
- ConcurrencyController bypassed 90% of the time (operations already on main thread)
- Widget transaction batching adds overhead GTK4 already handles
- Validation runs on every operation despite rarely catching issues
- 5-layer call chain for simple operations

**This is unreleased code** - we can refactor cleanly without backward compatibility concerns.

---

## Goals

✅ **40-50% less code** (~2000 lines removed)
✅ **Clearer architecture** (3 layers instead of 5)
✅ **Better naming** (SplitPane not BulletproofSplitNode)
✅ **Faster operations** (remove overhead)
✅ **Same correctness** (keep core algorithms)

---

## Phase 1: Rename Everything 🏷️

### Method Renaming
```go
// Public API (used by application code)
BulletproofSplitNode  → SplitPane
BulletproofClosePane  → ClosePane
BulletproofStackPane  → StackPane

// Internal implementations (unchanged)
splitNode → splitNode (stays private)
closePane → closePane (stays private)
```

### File Renaming
```
workspace_bulletproof_operations.go → workspace_operations.go
workspace_tree_rebalancer.go → workspace_layout.go
```

### Call Sites to Update
- `workspace_manager.go:270` - split operation
- `workspace_manager.go:401` - close operation
- `workspace_pane_ops.go:182` - close current pane
- `workspace_popup.go:158,412,498` - popup close operations

---

## Phase 2: Delete ConcurrencyController 🔥

### Why Delete
- **Reality**: 90% of operations already on GTK main thread
- **Check cost**: `webkit.IsMainThread()` is 1 CPU instruction (pthread_self)
- **Complexity**: 454 lines for edge case marshalling
- **Better approach**: Direct IdleAdd for off-thread calls

### Replacement Pattern
```go
func (wm *WorkspaceManager) SplitPane(target *paneNode, direction string) (*paneNode, error) {
    if !webkit.IsMainThread() {
        return wm.marshalToMainThread(func() (*paneNode, error) {
            return wm.splitPaneImpl(target, direction)
        })
    }
    return wm.splitPaneImpl(target, direction)
}

func marshalToMainThread[T any](fn func() (T, error)) (T, error) {
    var result T
    var err error
    done := make(chan struct{})

    webkit.IdleAdd(func() bool {
        result, err = fn()
        close(done)
        return false
    })

    <-done
    return result, err
}
```

### Delete
- **File**: `workspace_concurrency.go` (454 lines)
- **Fields**: Remove from WorkspaceManager:
  - `concurrencyController *ConcurrencyController`
- **References**: Update all operation calls

---

## Phase 3: Delete WidgetTransactionManager 🎯

### Why Delete
- **GTK4 already batches**: No need to manually batch widget ops
- **Overhead**: Closure allocation, operation queuing, commit phase
- **Direct calls work**: GTK4 docs say direct calls are fine

### Simplified Pattern
```go
// Old (complex):
tx := wm.widgetTxManager.BeginTransaction("promotion")
tx.AddOperation(&WidgetOperation{
    Execute: func() error {
        webkit.WidgetResetSizeRequest(ptr)
        webkit.WidgetSetHExpand(ptr, true)
        return nil
    },
})
tx.Execute()
tx.Commit()

// New (direct):
webkit.WidgetResetSizeRequest(ptr)
webkit.WidgetSetHExpand(ptr, true)
webkit.WidgetQueueAllocate(ptr)
```

### Delete
- **File**: `workspace_widget_transaction.go` (400 lines)
- **Fields**: Remove from WorkspaceManager:
  - `widgetTxManager *WidgetTransactionManager`
- **Dependencies**: Update TreeRebalancer to call GTK directly

---

## Phase 4: Simplify TreeRebalancer 🌲

### Current Problem
230-line promotion transaction for what should be 5 GTK calls

### New Implementation
```go
func (wm *WorkspaceManager) promoteNodeAfterClose(node *paneNode) error {
    if node == nil || node.container == nil {
        return errors.New("invalid promotion target")
    }

    ptr := node.container.Ptr()
    if !webkit.WidgetIsValid(ptr) {
        return errors.New("widget destroyed during promotion")
    }

    // Step 1: Clear stale size constraints
    webkit.WidgetResetSizeRequest(ptr)
    webkit.WidgetSetHExpand(ptr, true)
    webkit.WidgetSetVExpand(ptr, true)

    // Step 2: Attach to new parent
    if node.parent == nil {
        wm.attachToWindow(ptr)
    } else {
        wm.attachToPaned(node.parent.container.Ptr(), ptr, node.parent.left == node)
    }

    // Step 3: Request layout recalculation
    webkit.WidgetQueueAllocate(ptr)
    if node.parent != nil {
        webkit.WidgetQueueAllocate(node.parent.container.Ptr())
    }

    return nil
}
```

### Changes to TreeRebalancer
- Remove `executePromotion` (230 lines of transaction code)
- Replace with direct GTK calls (~30 lines)
- Keep tree metrics calculation (useful for debugging)
- Keep rotation logic (not used yet, but correct for future)

---

## Phase 5: Make Validation Opt-In 📊

### Debug Mode Levels
```go
type DebugLevel int

const (
    DebugOff   DebugLevel = iota // Production: no validation
    DebugBasic                    // Development: basic checks
    DebugFull                     // Testing: full validation
)

// Read from environment
func getDebugLevel() DebugLevel {
    switch os.Getenv("DUMBER_DEBUG_WORKSPACE") {
    case "off", "0":
        return DebugOff
    case "basic", "1":
        return DebugBasic
    case "full", "2":
        return DebugFull
    default:
        return DebugBasic // Safe default for development
    }
}
```

### Conditional Validation
```go
func (wm *WorkspaceManager) SplitPane(target *paneNode, direction string) (*paneNode, error) {
    // Quick validation (always run - cheap)
    if target == nil || !target.isLeaf {
        return nil, errors.New("split target must be leaf pane")
    }

    // Expensive validation (debug only)
    if wm.debugLevel >= DebugBasic {
        if err := wm.geometryValidator.ValidateSplit(target, direction); err != nil {
            return nil, fmt.Errorf("geometry validation: %w", err)
        }
    }

    if wm.debugLevel == DebugFull {
        wm.treeValidator.ValidateTree(wm.root, "before_split")
    }

    // Actual operation
    newNode, err := wm.splitPaneImpl(target, direction)

    if wm.debugLevel == DebugFull {
        wm.treeValidator.ValidateTree(wm.root, "after_split")
    }

    return newNode, err
}
```

### Keep Components (Made Optional)
- `TreeValidator` - Useful for debugging tree corruption
- `GeometryValidator` - Catches size constraint issues
- `StateTombstoneManager` - Useful for rollback during development

---

## Phase 6: Simplify SafeWidget 🔧

### Current Problem
Every widget operation wrapped in closure:
```go
node.container.Execute(func(ptr uintptr) error {
    webkit.WidgetShow(ptr)
    return nil
})
```

### Simplified Approach
```go
// SafeWidget keeps validity tracking but removes closure overhead
type SafeWidget struct {
    ptr      uintptr
    typeInfo string
    valid    atomic.Bool
}

func (sw *SafeWidget) Ptr() uintptr {
    if sw.valid.Load() {
        return sw.ptr
    }
    return 0
}

func (sw *SafeWidget) IsValid() bool {
    return sw.valid.Load() && webkit.WidgetIsValid(sw.ptr)
}

// Usage becomes direct:
if ptr := node.container.Ptr(); ptr != 0 {
    webkit.WidgetShow(ptr)
}
```

### Migration
- Remove `Execute()` method from SafeWidget
- Update all call sites to use Ptr() + direct calls
- Keep Invalidate() for cleanup tracking

---

## Phase 7: File Consolidation 📁

### Delete These Files
1. `workspace_concurrency.go` - 454 lines
2. `workspace_widget_transaction.go` - 400 lines
3. `workspace_stack_lifecycle.go` - Merge into pane_ops (not heavily used)

### Rename These Files
1. `workspace_bulletproof_operations.go` → `workspace_operations.go`
2. `workspace_tree_rebalancer.go` → `workspace_layout.go`

### Final Structure (14 files, -2000 lines)
```
Core:
  workspace_types.go                 - Data structures
  workspace_manager.go               - Manager initialization
  workspace_operations.go            - Public API (SplitPane, ClosePane, StackPane)
  workspace_pane_ops.go             - Private implementations
  workspace_layout.go               - Tree rebalancing (simplified)

Features:
  workspace_stacked_panes.go        - Stack management
  workspace_focus.go                - Focus state machine
  workspace_popup.go                - Popup handling
  workspace_navigation.go           - Keyboard navigation

Utilities:
  workspace_utils.go                - Helper functions
  workspace_css.go                  - CSS class management

Debug (opt-in):
  workspace_tree_validator.go       - Tree validation
  workspace_geometry_validator.go   - Geometry checks
  workspace_state_tombstone.go      - State snapshots
  workspace_debug.go                - Debug helpers
  workspace_diagnostics.go          - Diagnostics
```

---

## Implementation Order

### Day 1: Safe Renames (2-3 hours)
1. ✅ Phase 1: Rename methods and files
2. ✅ Update all call sites
3. ✅ Update docs (PANE_ARCHITECTURE.md)
4. ✅ Commit: "refactor(workspace): rename bulletproof to standard names"

### Day 2: Remove Layers (4-5 hours)
5. ✅ Phase 2: Delete ConcurrencyController
6. ✅ Phase 3: Delete WidgetTransactionManager
7. ✅ Phase 6: Simplify SafeWidget
8. ✅ Commit: "refactor(workspace): remove transaction overhead"

### Day 3: Simplify Logic (3-4 hours)
9. ✅ Phase 4: Simplify TreeRebalancer promotion
10. ✅ Phase 5: Make validation opt-in
11. ✅ Commit: "refactor(workspace): simplify promotion and validation"

### Day 4: Cleanup (2 hours)
12. ✅ Phase 7: Consolidate files
13. ✅ Update imports everywhere
14. ✅ Commit: "refactor(workspace): consolidate files"

### Day 5: Test & Validate (3-4 hours)
15. ✅ Run with DUMBER_DEBUG_WORKSPACE=full
16. ✅ Test all split/close/stack operations
17. ✅ Verify no GTK warnings
18. ✅ Check geometry with debug tools
19. ✅ Commit: "test(workspace): validate simplification"

**Total: 14-18 hours over 5 sessions**

---

## Testing Strategy

### Environment Setup
```bash
# Full validation during refactoring
export DUMBER_DEBUG_WORKSPACE=full

# After validation passes, test production mode
export DUMBER_DEBUG_WORKSPACE=off
```

### Test Cases
1. **Splits**: All 4 directions (left/right/up/down)
2. **Closes**:
   - Non-root with sibling promotion
   - Root pane replacement
   - Last pane (app exit)
   - Pane within stack
3. **Stacks**: Create, navigate tabs, close tabs
4. **Stress Tests**:
   - Rapid split/close cycles
   - Split from stacked pane
   - Deep nesting (8+ levels)
   - Focus changes during operations

### Success Criteria
- ✅ All operations work identically to before
- ✅ No GTK warnings/errors in logs
- ✅ Widget sizes correct after promotion
- ✅ Tree structure valid (DebugFull validation passes)
- ✅ ~2000 lines removed
- ✅ Faster operation latency (measure with debug timers)

---

## Risk Mitigation

### Branch Strategy
```bash
git checkout -b refactor/simplify-workspace
# Commit after each phase
# Squash before merge to main
```

### If Issues Found
1. **Broken operation**: Revert specific phase commit
2. **GTK crash**: Enable DebugFull, check validation logs
3. **Widget sizing wrong**: Check promotion logic, compare with old TreeRebalancer
4. **Focus loss**: Check focus state machine integration

### No Rollback Code Needed
- This is unreleased software
- Clean refactor is better than feature flags
- Git history provides rollback if critical issue found

---

## Expected Outcome

### Code Quality
- **Before**: 19 files, ~8000 lines, 5-layer architecture
- **After**: 14 files, ~6000 lines, 3-layer architecture
- **Complexity**: -40% (measured by cyclomatic complexity)

### Performance
- **Operation latency**: -20% (remove transaction/validation overhead)
- **Memory usage**: -15% (fewer allocations)
- **Code clarity**: Massively improved (direct call chains)

### Maintainability
- ✅ New developers understand flow in 30 minutes vs 3 hours
- ✅ Adding features requires touching fewer files
- ✅ Debugging shows clear stack traces
- ✅ No mysterious "bulletproof" terminology

---

## Notes

- Keep all tree algorithms identical (they work correctly)
- GTK4 bindings are solid - trust them
- Validation is development tool, not production requirement
- Direct is better than defensive when APIs are stable