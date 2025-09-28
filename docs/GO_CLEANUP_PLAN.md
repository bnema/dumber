# Go-Idiomatic Cleanup Plan (2025 Standards)

## Overview
This document outlines the plan to refactor the browser package to follow Go best practices and idioms as of 2025, focusing on simplicity and maintainability.

## Core Principles (Based on Current Go Best Practices)
1. **"Accept interfaces, return structs"** - Define interfaces where they're used
2. **Small, focused interfaces** - 1-3 methods max (like io.Reader, io.Writer)
3. **No "manager" suffix** - Go prefers descriptive type names
4. **Avoid premature abstraction** - Only create interfaces when needed
5. **Keep it simple** - Go values simplicity over clever abstractions

## Phase 1: Simplify File Structure (Following stdlib patterns)

### Current Problem:
- 15 `workspace_*` files (Java-style naming)
- Too many abstractions without clear benefit
- "Manager" suffix everywhere (not idiomatic)

### New Structure (12 files total):
```
browser/
├── doc.go           # Package documentation
├── browser.go       # BrowserApp and main types
├── pane.go         # paneNode, BrowserPane types
├── tree.go         # Tree operations (split, close, navigation)
├── focus.go        # Focus handling (simplified)
├── widget.go       # Widget wrapping and safety
├── stack.go        # Stacked pane operations
├── validation.go   # Tree and geometry validation
├── state.go        # State capture/restore
├── shortcuts.go    # Keyboard shortcuts
├── webview.go      # WebView helpers
└── popup.go        # Popup handling
```

## Phase 2: Define Minimal Interfaces (Only Where Needed)

### In pane.go (consumer-defined):
```go
// Viewer is what a pane needs from a webview
type Viewer interface {
    LoadURL(string) error
    Widget() uintptr
}

// Focusable is what can receive focus
type Focusable interface {
    Focus() error
    IsFocused() bool
}
```

### In tree.go (for testing):
```go
// TreeWalker visits nodes in the tree
type TreeWalker interface {
    Visit(*paneNode) error
}
```

**NO MEGA-INTERFACES** - Avoid TreeManager, FocusManager, etc.

## Phase 3: Rename Types (Remove "Manager" Suffix)

### Renames:
- `WorkspaceManager` → `Workspace`
- `FocusStateMachine` → `FocusController`
- `StackedPaneManager` → `StackController`
- `TreeValidator` → `Validator`
- `GeometryValidator` → (merge into Validator)
- `WidgetTransactionManager` → `WidgetTx`
- `StateTombstoneManager` → `StateCapture`

## Phase 4: Consolidate by Functionality

### tree.go (~1500 lines):
Merge:
- workspace_pane_ops.go (core operations)
- workspace_tree_rebalancer.go (rebalancing)
- workspace_navigation.go (tree navigation)

### focus.go (~600 lines):
Simplify focus_state_machine.go:
- Remove complex state machine
- Use simple focused pointer + validation
- Move CSS to widget.go

### widget.go (~700 lines):
Merge:
- Current widget.go
- workspace_widget_transaction.go
- Widget parts from workspace_utils.go

### validation.go (~800 lines):
Merge:
- workspace_tree_validator.go
- workspace_geometry_validator.go
- Validation from workspace_bulletproof_operations.go

### state.go (~600 lines):
Simplify workspace_state_tombstone.go:
- Remove complex tombstone system
- Simple state snapshots

## Phase 5: Simplify Workspace Type

### Current WorkspaceManager (too complex):
```go
type WorkspaceManager struct {
    // 20+ fields, multiple mutexes, nested managers
}
```

### New Workspace (simplified):
```go
type Workspace struct {
    app    *BrowserApp
    root   *paneNode
    focus  *paneNode        // Currently focused pane
    views  map[*webkit.WebView]*paneNode

    mu     sync.RWMutex     // Single mutex
}

// Methods directly on Workspace, no delegation
func (w *Workspace) Split(target *paneNode, dir Direction) (*paneNode, error)
func (w *Workspace) Close(target *paneNode) error
func (w *Workspace) Focus(target *paneNode) error
```

## Phase 6: Remove Overengineering

### Remove:
- Complex state machine for focus (use simple pointer)
- Transaction system (use defer for cleanup)
- Concurrency controller (use sync.Mutex)
- Multiple validation layers (validate once)
- Bulletproof wrappers (handle errors normally)

### Simplify to:
```go
// Instead of complex transactions
func (w *Workspace) Split(target *paneNode, dir Direction) (*paneNode, error) {
    w.mu.Lock()
    defer w.mu.Unlock()

    // Validate once
    if err := w.validate(); err != nil {
        return nil, err
    }

    // Do the split
    newNode := w.doSplit(target, dir)

    // Update focus
    w.focus = newNode

    return newNode, nil
}
```

## Benefits of This Approach:

1. **Go-idiomatic**: Follows stdlib patterns
2. **Simpler**: 40% less code, easier to understand
3. **No over-abstraction**: Interfaces only where needed
4. **Clear ownership**: Each file has single responsibility
5. **Testable**: Small interfaces easy to mock
6. **Maintainable**: Less indirection, clearer flow

## What We're NOT Doing:
- Creating interface for everything
- Using "Manager" suffix
- Complex dependency injection
- Abstracting GTK (until actually needed)
- Multiple layers of validation

## Implementation Order:
1. Rename files (remove workspace_ prefix)
2. Merge related files by functionality
3. Simplify types (remove Manager suffix)
4. Remove unnecessary abstractions
5. Test that everything still works

This follows Go's philosophy: **"Clear is better than clever"**

## File Consolidation Map

### Current → New:
```
workspace_manager.go           → browser.go (merge with existing)
workspace_types.go            → pane.go (merge with existing)
workspace_pane_ops.go         → tree.go
workspace_tree_rebalancer.go  → tree.go (merge)
workspace_navigation.go       → tree.go (merge)
focus_state_machine.go        → focus.go (simplified)
widget.go                     → widget.go (keep, expand)
workspace_widget_transaction.go → widget.go (merge)
workspace_utils.go            → widget.go (merge widget parts)
workspace_tree_validator.go   → validation.go
workspace_geometry_validator.go → validation.go (merge)
workspace_bulletproof_operations.go → validation.go (merge)
workspace_state_tombstone.go  → state.go (simplified)
workspace_stack_lifecycle.go  → state.go (merge)
stacked_panes.go              → stack.go (rename)
workspace_css.go              → widget.go (merge CSS parts)
workspace_popup.go            → popup.go (rename)
workspace_concurrency.go      → REMOVE (use standard sync)
shortcuts.go                  → shortcuts.go (keep)
window_shortcuts.go           → shortcuts.go (merge)
webview.go                    → webview.go (keep)
interfaces.go                 → REMOVE (define interfaces where used)
```

### Result: 24 files → 12 files (50% reduction)