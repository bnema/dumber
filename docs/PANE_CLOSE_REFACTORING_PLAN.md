# Detailed Pane Close Refactoring Plan

## Problem Analysis Summary

The current implementation has **THREE CRITICAL BUGS**:
1. **Multiple Pane Closure**: Sibling identification logic at lines 920-928 incorrectly identifies siblings
2. **Invalid Widget Promotion**: Lines 1042-1090 operate on destroyed widgets
3. **Race Conditions**: Widget cleanup happens before promotion completes

## PHASE 1: New Simplified Architecture

### 1.1 Create New File Structure
```
workspace_pane_close_v2.go      # New simplified close logic
workspace_pane_close_legacy.go  # Rename current implementation
workspace_pane_close_test.go    # Comprehensive tests
```

### 1.2 Core Data Structure Changes
```go
// Add to paneNode struct
type paneNode struct {
    // ... existing fields ...

    // New field to track widget validity
    widgetValid bool  // Set to false when widget destroyed
}
```

## PHASE 2: Detailed New closePane Implementation

### 2.1 Main Close Function (50-70 lines total)
```go
func (wm *WorkspaceManager) closePaneV2(node *paneNode) (*paneNode, error) {
    // STEP 1: Basic validation (5 lines)
    if node == nil || !node.isLeaf {
        return nil, errors.New("invalid close target")
    }

    // STEP 2: Handle special cases (15 lines)
    // 2a. Stacked pane
    if node.parent != nil && node.parent.isStacked {
        return wm.closeStackedPaneSimple(node)
    }

    // 2b. Last pane
    if wm.countPanes() == 1 {
        wm.cleanupAndExit(node)
        return nil, nil
    }

    // 2c. Root pane with others
    if node == wm.root {
        return wm.promoteNewRoot(node)
    }

    // STEP 3: Standard close - promote sibling (30 lines)
    parent := node.parent
    sibling := wm.getSibling(node)
    grandparent := parent.parent

    // Critical: Update tree structure FIRST
    sibling.parent = grandparent
    if grandparent != nil {
        if grandparent.left == parent {
            grandparent.left = sibling
        } else {
            grandparent.right = sibling
        }
    } else {
        wm.root = sibling
    }

    // STEP 4: Update GTK widgets (using GTK4 auto-unparenting)
    if grandparent != nil && grandparent.container != nil {
        grandparent.container.Execute(func(gpPtr uintptr) error {
            sibling.container.Execute(func(sibPtr uintptr) error {
                // GTK4 automatically unparents old child when setting new
                if grandparent.left == sibling {
                    webkit.PanedSetStartChild(gpPtr, sibPtr)
                } else {
                    webkit.PanedSetEndChild(gpPtr, sibPtr)
                }
                return nil
            })
            return nil
        })
    } else if sibling == wm.root {
        // Sibling is new root
        sibling.container.Execute(func(sibPtr uintptr) error {
            wm.window.SetChild(sibPtr)
            return nil
        })
    }

    // STEP 5: Cleanup (10 lines)
    wm.cleanupPane(node)
    wm.cleanupPane(parent) // Parent paned is no longer needed

    // STEP 6: Update focus
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

#### promoteNewRoot - Handle Root Replacement
```go
func (wm *WorkspaceManager) promoteNewRoot(oldRoot *paneNode) (*paneNode, error) {
    // Find first non-root pane by traversing tree
    var newRoot *paneNode

    // Try left subtree first
    if oldRoot.left != nil {
        newRoot = oldRoot.left
    } else if oldRoot.right != nil {
        newRoot = oldRoot.right
    } else {
        // No other panes - shouldn't happen
        return nil, errors.New("no replacement root found")
    }

    // Detach new root from its parent
    if newRoot.parent != nil {
        sibling := wm.getSibling(newRoot)
        parent := newRoot.parent

        // Promote sibling to parent's position
        if parent.parent != nil {
            // Has grandparent - attach sibling to it
            grand := parent.parent
            sibling.parent = grand
            if grand.left == parent {
                grand.left = sibling
            } else {
                grand.right = sibling
            }

            // Update GTK widget
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
    }

    // Make newRoot the root
    newRoot.parent = nil
    wm.root = newRoot

    // Attach to window
    newRoot.container.Execute(func(ptr uintptr) error {
        wm.window.SetChild(ptr)
        return nil
    })

    // Cleanup old root
    wm.cleanupPane(oldRoot)

    return newRoot, nil
}
```

#### cleanupPane - Safe Cleanup
```go
func (wm *WorkspaceManager) cleanupPane(node *paneNode) {
    if node == nil || node.widgetValid == false {
        return // Already cleaned
    }

    // Mark as invalid immediately
    node.widgetValid = false

    // Clean up pane resources
    if node.pane != nil {
        node.pane.CleanupFromWorkspace(wm)
        if node.pane.webView != nil {
            node.pane.webView.Destroy()
        }
    }

    // Invalidate container
    if node.container != nil {
        node.container.Invalidate()
        node.container = nil
    }

    // Clear tree pointers
    node.parent = nil
    node.left = nil
    node.right = nil
}
```

## PHASE 3: Stack Container Simplification

### 3.1 Simplified Stack Close
```go
func (wm *WorkspaceManager) closeStackedPaneSimple(node *paneNode) (*paneNode, error) {
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
        wm.cleanupPane(stack)

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
    wm.cleanupPane(node)

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
1. **Week 1**: Implement new functions alongside old ones
2. **Week 2**: Add comprehensive logging to both paths
3. **Week 3**: A/B test with feature flag
4. **Week 4**: Full migration after stability proven

### 5.2 Testing Scenarios
```go
func TestClosePaneScenarios(t *testing.T) {
    tests := []struct{
        name string
        setup func() *paneNode
        target string  // path to target node
        expectPanes int
        expectRoot string
    }{
        {
            name: "close_right_child_simple_split",
            // A[B,C] -> close C -> A becomes B
        },
        {
            name: "close_middle_nested_split",
            // A[B[D,E],C] -> close B -> A[sibling(D,E),C]
        },
        {
            name: "close_in_stack",
            // Stack[A,B,C] -> close B -> Stack[A,C]
        },
        {
            name: "close_last_in_stack",
            // A[Stack[B,C],D] -> close C -> A[B,D]
        },
    }
}
```

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
log.Printf("[CLOSE] Starting: node=%p parent=%p sibling=%p",
    node, node.parent, sibling)

// After tree update:
log.Printf("[CLOSE] Tree updated: new_root=%p promoted=%p",
    wm.root, promoted)

// After GTK update:
log.Printf("[CLOSE] GTK updated: widget=%#x parent=%#x",
    widgetPtr, parentPtr)
```

## Expected Results

### Before (Current Issues):
- 400+ lines of complex code
- Segfaults on complex layouts
- Multiple panes closed incorrectly
- Widget corruption errors

### After (New Implementation):
- ~150 lines total
- Clean GTK4 lifecycle adherence
- Predictable single-pane closure
- No widget corruption

## Implementation Order
1. Implement `closePaneV2` function
2. Implement helper functions
3. Add comprehensive logging
4. Create test suite
5. Run side-by-side with flag
6. Migrate after proven stable

## Key Files to Modify

1. `workspace_types.go:8` - Add `widgetValid bool` field to `paneNode`
2. `workspace_pane_ops.go:707` - Current `closePane` function (rename to `closePaneLegacy`)
3. **NEW** `workspace_pane_close_v2.go` - Simplified implementation
4. **NEW** `workspace_pane_close_test.go` - Comprehensive test suite
5. `stacked_panes.go` - Simplify `CloseStackedPane` function

## Critical Success Metrics

1. **Stability**: No segfaults in any close scenario
2. **Correctness**: Only requested pane closes (never multiple)
3. **Performance**: Close operation completes in <50ms
4. **Maintainability**: Code is <200 lines and easily understood