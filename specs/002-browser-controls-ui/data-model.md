# Data Model: Browser Controls UI

## Entities

### 1. ZoomLevel
Represents domain-specific zoom preferences for persistent user experience.

**Fields**:
- `Domain` (string, primary key): Website domain (e.g., "github.com", "stackoverflow.com")
- `ZoomFactor` (float64): Zoom multiplier (0.3 to 5.0 range, default 1.0)
- `UpdatedAt` (time.Time): Timestamp of last zoom level change

**Validation Rules**:
- Domain must be valid hostname format
- ZoomFactor must be within Firefox range: 0.30 ≤ zoom ≤ 5.00
- ZoomFactor must match standard Firefox zoom levels (30%, 50%, 67%, 80%, 90%, 100%, 110%, 120%, 133%, 150%, 170%, 200%, 240%, 300%, 400%, 500%)
- UpdatedAt automatically set on creation/update

**Relationships**:
- One-to-one with website domains
- No foreign key relationships (standalone entity)

### 2. BrowserState (Extended)
Existing entity extended with zoom and navigation state management.

**New Fields**:
- `CurrentZoomLevel` (float64): Active zoom level for current page
- `NavigationHistory` ([]HistoryEntry): Back/forward navigation stack
- `HistoryIndex` (int): Current position in navigation history

**State Transitions**:
- Zoom In: CurrentZoomLevel → Next higher Firefox zoom level
- Zoom Out: CurrentZoomLevel → Next lower Firefox zoom level  
- Navigate Back: HistoryIndex decrements (if > 0)
- Navigate Forward: HistoryIndex increments (if < len(NavigationHistory)-1)
- URL Change: Update CurrentZoomLevel from ZoomLevel persistence

**Validation Rules**:
- CurrentZoomLevel follows same rules as ZoomLevel.ZoomFactor
- HistoryIndex must be valid array index (0 ≤ index < len(NavigationHistory))
- NavigationHistory maintained as bounded array (configurable max size)

### 3. KeyboardEvent (Transient)
Represents keyboard input events for processing.

**Fields**:
- `Key` (string): Key identifier ("=", "-", "c")
- `Modifiers` ([]string): Active modifier keys (["ctrl"], ["ctrl", "shift"])
- `Timestamp` (time.Time): Event occurrence time

**Validation Rules**:
- Key must be non-empty string
- Modifiers must be valid modifier keys ("ctrl", "shift", "alt", "meta")
- Timestamp must be recent (within reasonable event processing window)

**State Transitions**:
- Ctrl+Plus → ZoomIn action
- Ctrl+Minus → ZoomOut action  
- Ctrl+Shift+C → CopyURL action

### 4. MouseEvent (Transient)
Represents mouse button events for navigation.

**Fields**:
- `Button` (int): Mouse button identifier (3=back, 4=forward)
- `EventType` (string): Event type ("mousedown", "mouseup")
- `Timestamp` (time.Time): Event occurrence time

**Validation Rules**:
- Button must be 3 (back) or 4 (forward)
- EventType must be valid mouse event type
- Only "mousedown" events trigger navigation actions

**State Transitions**:
- Button 3 mousedown → NavigateBack action
- Button 4 mousedown → NavigateForward action

## Database Schema Changes

### New Table: zoom_levels
```sql
CREATE TABLE zoom_levels (
    domain TEXT PRIMARY KEY,
    zoom_factor REAL NOT NULL DEFAULT 1.0 CHECK(zoom_factor >= 0.3 AND zoom_factor <= 5.0),
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_zoom_levels_updated_at ON zoom_levels(updated_at);
```

### SQLC Queries
```sql
-- name: GetZoomLevel :one
SELECT zoom_factor FROM zoom_levels WHERE domain = ?;

-- name: SetZoomLevel :exec
INSERT OR REPLACE INTO zoom_levels (domain, zoom_factor, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP);

-- name: CleanupOldZoomLevels :exec
DELETE FROM zoom_levels WHERE updated_at < date('now', '-30 days');
```

## Data Flow

### Zoom Operations
1. User triggers zoom event → JavaScript captures keyboard event
2. Frontend calls backend `ZoomIn()` or `ZoomOut()` method
3. Backend calculates next valid Firefox zoom level
4. Backend updates BrowserState.CurrentZoomLevel
5. Backend persists to database via `SetZoomLevel`
6. Frontend applies CSS zoom styling
7. Optional: Visual feedback notification

### Navigation Operations  
1. User triggers mouse back/forward → JavaScript captures mouse event
2. Frontend calls backend `NavigateBack()` or `NavigateForward()` method
3. Backend validates navigation history bounds
4. Backend updates BrowserState.HistoryIndex
5. Backend returns target URL from history
6. Frontend navigates WebView to target URL

### URL Copy Operation
1. User triggers Ctrl+Shift+C → JavaScript captures keyboard event
2. Frontend calls backend `CopyCurrentURL()` method  
3. Backend retrieves current URL from BrowserState
4. Backend executes clipboard tool chain (wlcopy → xclip → xsel)
5. Backend returns success/failure status
6. Optional: Visual feedback notification

## Validation Strategy

### Input Validation
- All user inputs validated at service layer entry points
- Zoom levels constrained to Firefox standard values
- Navigation bounds checking prevents array index errors
- URL validation for copy operations

### Data Integrity
- SQLite constraints enforce zoom level ranges
- Database transactions for atomic state updates
- Error handling for clipboard tool failures
- Graceful degradation when tools unavailable

### Performance Considerations
- Zoom level persistence debounced to prevent excessive DB writes
- Navigation history bounded to prevent memory growth
- Database cleanup for old zoom preferences
- Prepared statements for repeated queries

## Error Handling

### Domain Errors
- Invalid zoom level → clamp to nearest valid value
- Empty navigation history → silent no-op
- Invalid clipboard tool → try next in fallback chain
- Malformed URL → log error, return failure status

### System Errors
- Database unavailable → continue with in-memory state only
- Clipboard tools unavailable → user notification, graceful degradation
- WebView navigation failure → restore previous state
- Event processing errors → log and continue

All data models maintain constitutional principles of simplicity and performance while providing robust user experience for browser control operations.