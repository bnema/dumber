# Data Model: WebKit2GTK Native Browser Backend

**Feature**: Native WebKit Browser Backend  
**Date**: 2025-01-21  
**Status**: Complete

## Core Entities

### WebKit Browser Instance
**Purpose**: Main browser engine wrapper managing WebKitWebView and GTK window
**Key Attributes**:
- WebView: WebKitWebView instance for rendering
- Window: GTK ApplicationWindow container  
- Settings: WebKitSettings for browser configuration
- UserContentManager: WebKitUserContentManager for JavaScript injection
- KeybindingController: GTK event handler for keyboard shortcuts

**Relationships**:
- Contains one WebKitWebView instance
- Manages one GTK ApplicationWindow
- References shared HistoryManager and ZoomManager
- Interfaces with existing SQLite database through unchanged schema

### WebKit Bindings Interface  
**Purpose**: CGO wrapper providing Go interface to WebKit2GTK C APIs
**Key Attributes**:
- WebViewHandle: C pointer to WebKitWebView
- WindowHandle: C pointer to GtkApplicationWindow
- SettingsHandle: C pointer to WebKitSettings  
- CallbackRegistry: Go callback functions for WebKit signals
- ErrorContext: Error propagation from C to Go

**State Transitions**:
1. Uninitialized → C library loaded → GTK initialized → WebKit configured → Ready
2. Ready → Loading URL → Content loaded → Interactive
3. Interactive → Navigation event → Loading URL (cycle)
4. Any state → Error → Error handling → Recovery attempt

### Migration State Manager
**Purpose**: Manages transition from Wails to WebKit implementation  
**Key Attributes**:
- MigrationMode: Flag indicating Wails/WebKit/Dual mode
- DataPreservation: User data backup and restore mechanisms
- FeatureParity: Validation checklist for Wails feature equivalence
- PerformanceMetrics: Comparison data between implementations

**Validation Rules**:
- All existing user data must be preserved during migration
- Browser functionality must maintain 100% feature parity
- Performance must meet or exceed Wails baseline metrics
- Keyboard shortcuts must function identically to current implementation

## Data Flow Architecture

### Browser Lifecycle
```
1. Application Start
   ├── Load pkg/webkit bindings
   ├── Initialize GTK application
   ├── Create WebKitWebView instance  
   ├── Configure browser settings
   ├── Setup keyboard event handlers
   └── Load initial URL or homepage

2. URL Navigation  
   ├── Validate URL through existing parser service
   ├── Load URL in WebKitWebView
   ├── Update history database (unchanged schema)
   ├── Configure zoom level from database
   └── Inject global keyboard scripts via UserContentManager

3. Keyboard Events
   ├── GTK captures key events
   ├── Route to appropriate handler (navigation/zoom/copy)
   ├── Execute WebKit operation or JavaScript injection
   └── Update database if needed (zoom persistence)
```

### C/Go Interface Boundaries
```
Go Layer (pkg/webkit):
├── WebView struct with Go methods
├── Event handling with Go channels  
├── Error handling with Go error types
└── Memory management with Go finalizers

CGO Bridge:
├── C function wrappers for WebKit APIs
├── Callback function registration
├── Memory allocation/deallocation
└── Type conversion (C ↔ Go)

C Layer (WebKit2GTK):
├── WebKitWebView widget management
├── GTK event loop integration
├── WebKit signals and callbacks
└── Native browser engine operations
```

## Database Integration

### Preserved Schema
**Status**: No changes to existing SQLite schema
- History table: Maintain current structure and data
- Zoom settings table: Continue using domain-based zoom storage  
- All SQLC generated queries: Unchanged interface
- Database migrations: No new migrations required

### New Database Interactions
**WebKit-specific data**: Store as configuration, not database entities
- WebKit settings: Managed through WebKitSettings API
- User content scripts: Stored as embedded resources, not database
- Browser state: Memory-only, restored through WebKit APIs

## Error Handling Model

### Error Propagation Chain
```
WebKit C Error → CGO Error Conversion → Go Error → User Notification

Error Categories:
├── Initialization Errors: WebKit library loading, GTK setup
├── Navigation Errors: Invalid URLs, network failures, load timeouts
├── Script Injection Errors: UserContentManager failures, CSP violations  
├── Keyboard Binding Errors: GTK event registration, handler failures
└── Performance Errors: Memory limits, CPU constraints, timeout violations
```

### Recovery Strategies
- Initialization failures: Fallback error dialog, graceful exit
- Navigation errors: Error page display, retry mechanisms
- Script failures: Silent failure with logging, functionality degradation
- Memory errors: Garbage collection, resource cleanup, restart recommendation

## Performance Data Model

### Metrics Collection
**Startup Performance**:
- Library loading time: WebKit2GTK and GTK initialization
- WebView creation time: Widget setup and configuration
- First paint time: Initial browser window display

**Runtime Performance**:  
- Memory usage: WebKit process + GTK overhead + Go runtime
- Navigation speed: URL load time, DOM ready time
- Event handling latency: Keyboard shortcut response time

**Comparison Baseline**:
- Current Wails metrics as performance targets
- Performance regression detection through automated testing
- Resource usage monitoring for constitutional compliance (<100MB baseline)

---

**Data Model Status**: ✅ Complete - All entities defined, ready for contract generation