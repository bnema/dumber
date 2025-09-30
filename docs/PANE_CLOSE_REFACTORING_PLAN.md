# Detailed Pane Close Refactoring Plan

## Problem Analysis Summary

The current implementation in `internal/app/browser/workspace_pane_ops.go:707`+ exhibits **three interlocked failure modes**:
1. **Multiple Pane Closure**: The promotion branch (around `workspace_pane_ops.go:990`) iterates over invalid parent/child references, causing both siblings to be detached when `wm.app.panes` still counts them. This yields double-close behaviour during rapid shortcuts.
2. **Invalid Widget Promotion**: The root promotion path (roughly `workspace_pane_ops.go:878-948`) manipulates widgets after `Cleanup()` invalidates their `SafeWidget` guard, which leads to sporadic `GTK_IS_WIDGET` assertions.
3. **Race Conditions**: Cleanup (`ensureCleanup`) and hover-detach run while asynchronous `IdleAdd` callbacks still reference the old nodes, so hover/focus reattachment runs against freed memory during reproduction of issue #1429.

> Baseline reproduction: trigger repeated `closePane` via Ctrl+W on a layout with nested root + stack; log spew shows `[workspace] close aborted` alongside panics in `ensureSafeWidget`.

### Environment Constraints
- `stackedPaneManager.CloseStackedPane` is tightly coupled to `wm.stackedPaneManager`, so any refactor must either reimplement its logic or provide compatibility shims.
- `BulletproofClosePane` wraps `closePane`, retrying on specific errors; changes to panic/zero-return semantics must preserve that contract.
- Hover/focus controller lifecycles (`wm.ensureHover`, `focusStateMachine.attachGTKController`) must remain intact after tree restructuring.

### Preconditions & Guardrails
1. Capture baseline behaviour with `go test ./tests/workspace/...` (currently incomplete; we will add targeted tests in Phase 5).
2. Add temporary structured logging with `log.Printf("[pane-close] ...")` gated behind an environment toggle to avoid production noise.
3. Ensure unit test scaffolding can construct `WorkspaceManager` with fake `webkit` bindings (see `tests/browser/fakes`).

## PHASE 0: Stabilise & Instrument

1. Wrap `BulletproofClosePane` with structured logging (log key: `pane-close-stage`) guarded by `wm.debugInstrumentation` flag.
2. Add `WorkspaceDiagnostics` helper capturing tree snapshots (`wm.dumpTreeState`) before and after `closePane`.
3. Add regression harness in `tests/browser/workspace_close_test.go` that constructs a representative tree and exercises edge cases (root-only, nested split, stack + split).
4. Verify reproduction steps still fail prior to refactor to ensure harness is meaningful.

## PHASE 1: New Simplified Architecture

### 1.1 Keep File Layout, Replace Implementation In-Place
```
internal/app/browser/workspace_pane_ops.go   # Replace existing closePane implementation with new logic
internal/app/browser/workspace_types.go      # Extend paneNode metadata
```

### 1.2 Core Data Structure Changes
```go
// Add to paneNode struct
type paneNode struct {
    // ... existing fields ...

    widgetValid bool       // Guard flagged before GTK destruction
    cleanupGeneration uint // Helps assert that asynchronous callbacks skip stale nodes
}

// Add to WorkspaceManager
type WorkspaceManager struct {
    // ... existing fields ...

    cleanupCounter uint
}
```

## PHASE 2: Detailed New closePane Implementation

### 2.1 Main Close Function (60-80 lines total)
```go
func (wm *WorkspaceManager) closePane(node *paneNode) (*paneNode, error) {
    ctx := wm.beginClose(node)
    defer ctx.finish()

    // STEP 1: Basic validation (quick fail)
    if ctx.err != nil {
        return nil, ctx.err
    }

    // STEP 2: Handle stacked panes via compatibility shim
    if node.parent != nil && node.parent.isStacked {
        return wm.closeStackedPaneCompat(node)
    }

    // STEP 3: Handle trivial exit cases
    if ctx.remaining == 1 {
        return wm.cleanupAndExit(node)
    }
    if node == wm.root {
        return wm.promoteNewRoot(ctx, node)
    }

    // STEP 4: Promote sibling in-place
    parent := node.parent
    sibling := wm.getSibling(node)
    grandparent := parent.parent
    wm.ensureWidgets(grandparent, parent, sibling)

    wm.promoteSibling(grandparent, parent, sibling)

    // STEP 5: GTK updates leverage auto-unparenting
    wm.swapContainers(grandparent, sibling)

    // STEP 6: Cleanup & focus
    wm.cleanupPane(node, ctx.Generation())
    wm.decommissionParent(parent, ctx.Generation())
    wm.setFocusToLeaf(sibling)

    return sibling, nil
}
```

### 2.2 Helper Functions with Exact Implementation

#### getSibling - CORRECTED LOGIC
```go
func (wm *WorkspaceManager) getSibling(node *paneNode) *paneNode {
    if node.parent == nil {
        return nil
    }
    // FIXED: Simple, clear sibling identification
    if node.parent.left == node {
        return node.parent.right
    }
    return node.parent.left
}
```

#### beginClose / closeContext
```go
type closeContext struct {
    wm         *WorkspaceManager
    target     *paneNode
    remaining  int
    err        error
    generation uint
}

func (wm *WorkspaceManager) beginClose(node *paneNode) closeContext {
    ctx := closeContext{wm: wm, target: node, generation: wm.nextCleanupGeneration()}
    switch {
    case wm == nil:
        ctx.err = errors.New("workspace manager nil")
    case node == nil || !node.isLeaf:
        ctx.err = errors.New("invalid close target")
    case node.pane == nil || node.pane.webView == nil:
        ctx.err = errors.New("close target missing webview")
    default:
        ctx.remaining = len(wm.app.panes)
    }
    return ctx
}

func (ctx closeContext) Generation() uint { return ctx.generation }

func (ctx closeContext) finish() {
    if ctx.err == nil {
        ctx.wm.updateMainPane()
    }
}
```

#### ensureWidgets
```go
func (wm *WorkspaceManager) ensureWidgets(nodes ...*paneNode) {
    for _, n := range nodes {
        if n == nil || n.container == nil {
            continue
        }
        if n.container.IsValid() {
            continue
        }
        ptr := n.container.Ptr()
        n.container = wm.widgetRegistry.Recover(ptr, n.container.typeInfo)
    }
}
```

#### promoteSibling
```go
func (wm *WorkspaceManager) promoteSibling(grand *paneNode, parent *paneNode, sibling *paneNode) {
    if grand == nil {
        wm.root = sibling
        sibling.parent = nil
        return
    }
    sibling.parent = grand
    if grand.left == parent {
        grand.left = sibling
    } else {
        grand.right = sibling
    }
}
```

#### swapContainers
```go
func (wm *WorkspaceManager) swapContainers(grand *paneNode, sibling *paneNode) {
    if grand == nil {
        wm.attachRoot(sibling)
        return
    }
    grand.container.Execute(func(gPtr uintptr) error {
        sibling.container.Execute(func(sPtr uintptr) error {
            if grand.left == sibling {
                webkit.PanedSetStartChild(gPtr, sPtr)
            } else {
                webkit.PanedSetEndChild(gPtr, sPtr)
            }
            return nil
        })
        return nil
    })
}
```

#### decommissionParent
```go
func (wm *WorkspaceManager) decommissionParent(parent *paneNode, generation uint) {
    if parent == nil {
        return
    }
    wm.cleanupPane(parent, generation)
}
```

#### nextCleanupGeneration
```go
func (wm *WorkspaceManager) nextCleanupGeneration() uint {
    wm.cleanupCounter++
    return wm.cleanupCounter
}
```

#### attachRoot
```go
func (wm *WorkspaceManager) attachRoot(root *paneNode) {
    if root == nil || root.container == nil || wm.window == nil {
        return
    }
    root.container.Execute(func(ptr uintptr) error {
        wm.window.SetChild(ptr)
        webkit.WidgetQueueAllocate(ptr)
        webkit.WidgetShow(ptr)
        return nil
    })
}
```

#### promoteNewRoot - Handle Root Replacement
```go
func (wm *WorkspaceManager) promoteNewRoot(ctx closeContext, oldRoot *paneNode) (*paneNode, error) {
    candidate := wm.findReplacementRoot(oldRoot)
    if candidate == nil {
        return wm.cleanupAndExit(oldRoot)
    }

    sibling := wm.getSibling(candidate)
    if sibling != nil {
        wm.promoteSibling(candidate.parent.parent, candidate.parent, sibling)
    }

    candidate.parent = nil
    wm.root = candidate

    wm.attachRoot(candidate)

    wm.cleanupPane(oldRoot, ctx.Generation())
    return candidate, nil
}
```

#### cleanupPane - Safe Cleanup
```go
func (wm *WorkspaceManager) cleanupPane(node *paneNode, generation uint) {
    if node == nil {
        return
    }
    if !node.widgetValid {
        return
    }

    node.widgetValid = false
    node.cleanupGeneration = generation

    if node.pane != nil {
        node.pane.CleanupFromWorkspace(wm)
        if node.pane.webView != nil {
            node.pane.webView.Destroy()
        }
    }

    if node.container != nil {
        node.container.Invalidate()
        node.container = nil
    }

    node.parent = nil
    node.left = nil
    node.right = nil
}
```

## PHASE 3: Stack Container Simplification

### 3.1 Compatibility Layer for Stacked Panes
```go
func (wm *WorkspaceManager) closeStackedPaneCompat(node *paneNode) (*paneNode, error) {
    stack := node.parent

    // Find index
    index := -1
    for i, pane := range stack.stackedPanes {
        if pane == node {
            index = i
            break
        }
    }

    if index == -1 {
        return nil, errors.New("pane not in stack")
    }

    // Remove from array
    stack.stackedPanes = append(
        stack.stackedPanes[:index],
        stack.stackedPanes[index+1:]...
    )

    // If only one pane left, unstack it
    if len(stack.stackedPanes) == 1 {
        remaining := stack.stackedPanes[0]

        // Replace stack with remaining pane in tree
        remaining.parent = stack.parent
        if stack.parent != nil {
            if stack.parent.left == stack {
                stack.parent.left = remaining
            } else {
                stack.parent.right = remaining
            }

            // Update GTK widgets
            stack.parent.container.Execute(func(pPtr uintptr) error {
                remaining.container.Execute(func(rPtr uintptr) error {
                    if stack.parent.left == remaining {
                        webkit.PanedSetStartChild(pPtr, rPtr)
                    } else {
                        webkit.PanedSetEndChild(pPtr, rPtr)
                    }
                    return nil
                })
                return nil
            })
        } else {
            // Stack was root
            wm.root = remaining
            remaining.container.Execute(func(ptr uintptr) error {
                wm.window.SetChild(ptr)
                return nil
            })
        }

        // Cleanup stack container
        wm.cleanupPane(stack, wm.nextCleanupGeneration())

        return remaining, nil
    }

    // Update active index if needed
    if stack.activeStackIndex >= len(stack.stackedPanes) {
        stack.activeStackIndex = len(stack.stackedPanes) - 1
    }

    // Remove widget from GTK box
    stack.container.Execute(func(boxPtr uintptr) error {
        node.container.Execute(func(nodePtr uintptr) error {
            webkit.BoxRemove(boxPtr, nodePtr)
            return nil
        })
        return nil
    })

    // Show new active pane
    if stack.activeStackIndex >= 0 {
        active := stack.stackedPanes[stack.activeStackIndex]
        active.container.Execute(func(ptr uintptr) error {
            webkit.WidgetShow(ptr)
            return nil
        })
    }

    // Cleanup closed pane
    wm.cleanupPane(node, wm.nextCleanupGeneration())

    return stack, nil
}
```

## PHASE 4: GTK4 Widget Lifecycle Rules

### 4.1 Key GTK4 Rules to Follow
1. **Auto-unparenting**: When calling `gtk_paned_set_start_child(paned, new_child)`, GTK4 automatically:
   - Unparents the old child (if any)
   - Unparents new_child from its current parent (if any)
   - Sets new_child as the start child

2. **Widget Validity**: Always check `GTK_IS_WIDGET(widget)` before operations

3. **Reference Counting**: GTK4 uses reference counting - widgets are destroyed when ref count reaches 0

### 4.2 Safe Widget Operations Pattern
```go
// NEVER do this:
webkit.WidgetUnparent(widget)  // Manual unparent
webkit.PanedSetStartChild(paned, widget)  // Then set

// ALWAYS do this:
webkit.PanedSetStartChild(paned, widget)  // GTK handles unparenting

// For boxes:
webkit.BoxRemove(box, widget)  // Removes and unparents
// OR
webkit.BoxAppend(box, widget)  // Auto-unparents from old parent
```

## PHASE 5: Migration Strategy

### 5.1 Step-by-Step Migration
1. Land instrumentation (`beginClose` logging, `dumpTreeState`) behind `DebugPaneClose` flag.
2. Extend data structures (`widgetValid`, `cleanupGeneration`, `cleanupCounter`).
3. Introduce helper scaffolding (`closeContext`, `ensureWidgets`, `promoteSibling`, `swapContainers`).
4. Replace core `closePane` logic and keep compatibility helpers stubbed.
5. Rewrite stacked close handling to call `closeStackedPaneCompat` while keeping `StackedPaneManager` API surface intact.
6. Add regression tests + golden tree dumps; run `go test ./tests/browser/...` and `make build`.
7. Remove verbose logging, keep assertions + generation checks.

## PHASE 6: Validation & Safety

### 6.1 Add Assertions
```go
// Add to every widget operation:
if !webkit.WidgetIsValid(ptr) {
    return fmt.Errorf("invalid widget %#x", ptr)
}

// Before tree operations:
if err := wm.validateTreeConsistency(); err != nil {
    return nil, fmt.Errorf("tree inconsistent before close: %w", err)
}
```

### 6.2 Comprehensive Logging
```go
// Before operation:
log.Printf("[pane-close] start node=%p parent=%p sibling=%p",
    node, node.parent, sibling)

// After tree update:
log.Printf("[pane-close] tree updated new_root=%p promoted=%p gen=%d",
    wm.root, promoted, ctx.Generation())

// After GTK update:
log.Printf("[pane-close] gtk updated widget=%#x parent=%#x",
    widgetPtr, parentPtr)
```

## Expected Results

### Before (Current Issues):
- 400+ lines of defensive code
- Segfaults on complex layouts
- Multiple panes closed incorrectly
- Widget corruption errors

### After (New Implementation):
- ≤200 lines of cohesive logic
- Clean GTK4 lifecycle adherence
- Predictable single-pane closure
- Deterministic hover/focus cleanup using generations

## Implementation Order
1. Land instrumentation + regression harness (Phase 0)
2. Introduce data-structure additions (`widgetValid`, generation counter)
3. Implement new `closePane` + helper set in-place (Phase 2)
4. Wire stacked pane compatibility shim (Phase 3)
5. Run tests (`go test ./tests/browser/...`) and `make build`
6. Trim temporary logging once confidence is regained

## Key Files to Modify

1. `internal/app/browser/workspace_types.go` - Add `widgetValid` and `cleanupGeneration`
2. `internal/app/browser/workspace_manager.go` - Introduce `cleanupCounter` + helper
3. `internal/app/browser/workspace_pane_ops.go` - Replace `closePane`, add helpers, call into compat shim
4. `internal/app/browser/stacked_panes.go` - Point manager to `closeStackedPaneCompat`
5. `tests/browser/workspace_close_test.go` - Add regression scenarios

## Critical Success Metrics

1. **Stability**: No segfaults in any close scenario
2. **Correctness**: Only requested pane closes (never multiple)
3. **Performance**: Close operation completes in <50ms
4. **Maintainability**: Code is <200 lines and easily understood
