# Research: Browser Controls UI

## Research Tasks

### 1. Wails v3 Keyboard Event Handling
**Task**: Research keyboard event handling in Wails v3-alpha for desktop applications

**Decision**: Use Wails context menu and event system with JavaScript frontend integration
**Rationale**: 
- Wails v3 provides native keyboard event handling through WebView2/WebKit integration
- Frontend JavaScript can capture keyboard events and call backend Go methods via context
- Maintains separation between UI (JavaScript) and business logic (Go services)
- No additional dependencies required beyond existing Wails framework

**Alternatives considered**:
- Direct GTK keyboard hooks: Rejected due to complexity and Wails abstraction
- OS-level global hotkeys: Rejected as not needed (app-specific shortcuts only)
- Third-party hotkey libraries: Rejected to maintain constitution's simplicity principle

### 2. Mouse Button Event Handling (Back/Forward)
**Task**: Research mouse button 4/5 (back/forward) event handling in WebView context

**Decision**: Use JavaScript mouse event listeners with event.button detection
**Rationale**:
- Standard web platform approach using addEventListener('mousedown')
- event.button === 3 (back) and event.button === 4 (forward) detection
- Works consistently across WebKit2GTK implementations
- No platform-specific code required

**Alternatives considered**:
- GTK mouse event capture: Rejected due to Wails abstraction layer
- X11/Wayland direct event handling: Rejected as breaks single-binary principle

### 3. Zoom Level Management and Persistence
**Task**: Research zoom level implementation patterns and domain-based persistence

**Decision**: Use WebView zoom API with SQLite domain-based storage
**Rationale**:
- WebView provides native zoom functionality (setZoomFactor)
- Firefox zoom levels provide proven UX pattern (30%-500% range)
- SQLite table for domain → zoom_level mapping enables per-site persistence
- Existing database infrastructure can be extended

**Alternatives considered**:
- CSS transform scaling: Rejected due to layout issues and non-standard behavior
- Browser-level zoom only: Rejected as doesn't persist per-domain
- Configuration file storage: Rejected as database is faster for lookups

### 4. Linux Clipboard Integration (wlcopy fallback chain)
**Task**: Research Linux clipboard tool availability and fallback patterns

**Decision**: Implement command execution chain: wlcopy → xclip → xsel
**Rationale**:
- wlcopy is Wayland-native (primary target environment)
- xclip and xsel provide X11 compatibility
- exec.Command provides clean abstraction for tool detection
- Graceful degradation maintains usability across Linux variants

**Alternatives considered**:
- Go clipboard libraries: Rejected due to additional dependencies
- D-Bus clipboard access: Rejected as complex and not universally available
- Only wlcopy support: Rejected as limits platform compatibility

### 5. WebView Zoom API Integration
**Task**: Research WebView zoom control APIs in Wails v3 context

**Decision**: Use frontend JavaScript with backend coordination for zoom state
**Rationale**:
- document.body.style.zoom CSS property for immediate visual feedback
- Backend Go service manages zoom level persistence and bounds checking
- Maintains constitutional separation between UI and business logic
- No direct WebView API dependency required

**Alternatives considered**:
- Direct WebView SetZoom API: Not available in Wails v3-alpha abstraction
- Pure CSS zoom transforms: Rejected due to layout calculation issues

## Key Technical Findings

### Firefox Zoom Levels
Standard zoom progression: 30%, 50%, 67%, 80%, 90%, 100%, 110%, 120%, 133%, 150%, 170%, 200%, 240%, 300%, 400%, 500%

### Event Handling Architecture
```
Frontend (JavaScript) → Wails Context → Backend (Go) → Services → Database
     ↑                                                              ↓
  UI Events                                              State Persistence
```

### Clipboard Tool Detection Pattern
```go
var clipboardTools = []string{"wlcopy", "xclip", "xsel"}
for _, tool := range clipboardTools {
    if _, err := exec.LookPath(tool); err == nil {
        // Use this tool
        break
    }
}
```

### Database Schema Extension
New table: `zoom_levels`
```sql
CREATE TABLE zoom_levels (
    domain TEXT PRIMARY KEY,
    zoom_factor REAL NOT NULL DEFAULT 1.0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Risks and Mitigation

1. **Risk**: WebView zoom behavior inconsistency across platforms
   **Mitigation**: Use CSS zoom with bounds checking, test on target platforms

2. **Risk**: Mouse button events not captured in WebView
   **Mitigation**: Implement JavaScript event delegation on document level

3. **Risk**: Clipboard tools unavailable on target system
   **Mitigation**: Graceful degradation with user notification

4. **Risk**: Zoom persistence performance impact
   **Mitigation**: Debounce zoom level saves, use SQLite prepared statements

## Implementation Dependencies

- No new external dependencies required
- Leverage existing: Wails v3, SQLite, standard Go library
- Frontend JavaScript for event capture
- Backend Go services for state management
- Database schema extension for zoom persistence

All research findings support constitutional principles of speed, simplicity, and single-binary architecture.