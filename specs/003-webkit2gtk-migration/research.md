# Research: WebKit2GTK Native Bindings Migration

**Feature**: Native WebKit Browser Backend  
**Date**: 2025-01-21  
**Status**: Complete

## WebKit2GTK Integration Patterns

### Decision: CGO-based WebKit2GTK bindings
**Rationale**: 
- Direct access to WebKit2GTK C APIs provides full control over browser engine
- CGO allows seamless integration of C libraries with Go code
- Performance critical for <500ms startup requirement
- Maintains compatibility with existing GTK-based Linux desktop environments

**Alternatives considered**:
- Keeping Wails: Rejected due to framework limitations and lack of direct engine control
- gotk3/webkit2: Existing Go bindings but lack modern WebKit2GTK features needed
- Embedding Chromium: Too heavyweight, violates constitution's speed-first principle

## C Go Bindings Architecture

### Decision: pkg/webkit package structure
**Rationale**:
- Isolates WebKit-specific code for future extraction as separate repository
- Follows Go package conventions with clear API boundaries  
- Enables independent testing and versioning of WebKit bindings
- Supports constitution requirement for library-based architecture

**Alternatives considered**:
- Internal package only: Rejected, doesn't support future repo extraction goal
- Multiple WebKit packages: Over-engineering, violates simplicity principle
- Direct main package integration: Tight coupling, harder to test and maintain

## WebKit2GTK Reference Implementation

### Decision: Clone WebKit2GTK repository to /home/brice/dev/clone
**Rationale**:
- Provides official API documentation and examples for C bindings
- Reference implementations for browser features (navigation, zoom, JS execution)
- Understanding WebKit widget lifecycle and event handling patterns
- Source of truth for WebKit signal/callback patterns needed for keyboard shortcuts

**Research findings**:
- WebKit2GTK provides WebKitWebView widget for rendering
- WebKitSettings for browser configuration and feature toggles
- WebKitUserContentManager for JavaScript injection (replaces current Wails script injection)
- GTK key binding system for keyboard shortcuts (Alt+arrows, Ctrl+shortcuts)

## Migration Strategy

### Decision: Gradual replacement maintaining data compatibility
**Rationale**:
- Constitution requires preserving user data (history, bookmarks, zoom settings)
- Existing SQLite schema and SQLC generated code can be maintained
- Allows A/B testing of WebKit vs Wails implementations
- Minimizes user disruption during migration

**Migration approach**:
1. Create pkg/webkit with basic WebView rendering
2. Implement keyboard shortcuts through GTK event handling
3. Replace Wails window with GTK ApplicationWindow + WebKitWebView
4. Migrate script injection from Wails ExecJS to WebKit UserContentManager
5. Remove Wails dependencies and frontend build system

## Performance Considerations

### Decision: Direct WebKit widget integration
**Rationale**:
- WebKit2GTK designed for embedding in native applications
- Eliminates JavaScript bridge overhead present in Wails
- Direct GTK event loop integration for better responsiveness
- Memory management handled by GObject reference counting

**Performance expectations**:
- Startup time improvement due to removing Wails initialization overhead
- Memory reduction by eliminating embedded web frontend  
- Better keyboard shortcut responsiveness through direct GTK events
- Improved page loading performance with direct WebKit configuration

## Documentation Sources

### Decision: Multi-source documentation approach
**Rationale**:
- WebKit2GTK official docs: API reference and C examples
- GNOME Web (Epiphany) source: Real-world WebKit2GTK usage patterns  
- CGO best practices: Memory management and C/Go interface design
- GTK documentation: Window management and keyboard event handling

**Key resources identified**:
- WebKit2GTK API Reference: https://webkitgtk.org/reference/webkit2gtk/stable/
- Epiphany browser source: Example of WebKit2GTK browser implementation
- CGO documentation: Memory safety and performance best practices
- GTK4 migration guides: Future-proofing for eventual GTK4 support

## Risk Mitigation

### Decision: Incremental implementation with fallback plan
**Rationale**:
- Large architectural change requires careful validation
- User data preservation is non-negotiable requirement
- Performance regressions would violate constitution speed-first principle

**Mitigation strategies**:
- Maintain parallel Wails implementation during development
- Comprehensive integration testing before removing Wails
- Database migration scripts with rollback capability
- Performance benchmarking against current Wails implementation
- Feature parity validation through existing test suite

---

**Research Status**: ✅ Complete - All technical unknowns resolved, ready for Phase 1 design