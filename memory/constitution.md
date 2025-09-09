# Dumb Browser Constitution

## Core Principles

### I. Speed First (NON-NEGOTIABLE)
- Fast startup time is paramount - no feature worth compromising speed
- Single binary with embedded assets - no external dependencies at runtime
- WebKit2GTK for rendering - full compliance without reinventing wheels
- Minimal UI overhead - focus on web content, not chrome

### II. Universal Launcher Compatibility
- Support any dmenu-compatible launcher (fuzzel, rofi, tofi, wofi, bemenu)
- Standard stdin/stdout protocol - no launcher-specific integrations
- Clean text-based format for history/suggestions
- Pipeline-friendly: `dumber --dmenu | launcher | dumber --dmenu`

### III. Smart URL Handling
- Direct URL detection: `reddit.com` → navigate directly
- Search shortcuts: `g: golang tutorial` → Google search  
- History-first search: `reddit` → check history before web search
- No ambiguity - clear rules for input interpretation

### IV. Privacy-Conscious History
- Local SQLite database only - no cloud sync
- Configurable retention periods and cleanup
- Visit counting for intelligent suggestions
- User controls data - easy to clear/export

### V. Wayland Native
- Built for modern Linux desktop - sway, hyprland, etc.
- WebKit2GTK provides native Wayland integration
- Single instance handling for launcher integration
- Minimal resource usage - no background services

## Technical Standards

### Stack Requirements
- **Backend**: Go + Wails([v3-alpha](https://github.com/wailsapp/wails/tree/v3.0.0-alpha.28)) + WebKit2GTK + SQLite
- **CLI**: Cobra + Viper for configuration
- **Database**: SQLC for type-safe SQL generation
- **Validation**: Input sanitization libraries ([go-playground/validator](https://github.com/go-playground/validator))
- **Build**: Single static binary with embedded frontend
- **Dependencies**: WebKit2GTK + GTK3 only (system packages)

### Performance Targets
- Startup time: < 500ms from cold start
- Memory usage: < 100MB baseline + WebKit overhead
- History search: < 50ms for 1000+ entries
- Single instance lock: < 10ms response time

### Quality Gates
- All database queries must use SQLC generated code
- All user input must be validated and sanitized
- Error handling: proper Go error chains, no panics in user paths 
- Logging: structured logging for debugging, minimal in production (Slog for logging in text mode)

## Development Workflow

### Implementation Order
1. Core CLI structure (Cobra + Viper)
2. Database schema + SQLC integration
3. URL parser + search engine logic with fuzzy finder algorithm
4. History management (CRUD operations)
5. Wails browser integration
6. Dmenu compatibility layer
7. Configuration system

### Testing Requirements
- Unit tests for URL parsing logic
- Integration tests for database operations
- Manual testing with fuzzel integration
- Cross-launcher compatibility verification

## Constraints

### Simplicity Mandate
- KISS principle - no over-engineering
- Clean Code philosophy throughout
- Single responsibility per module
- Minimal configuration surface

### Security Requirements
- No credential storage or session management
- Input sanitization for all user data
- SQL injection prevention via SQLC
- No network requests except web browsing

## Governance

This constitution defines the project's core identity and technical direction. All implementation decisions must align with these principles, prioritizing speed and simplicity above feature richness.

**Version**: 1.0.0 | **Ratified**: 2025-01-09 | **Last Amended**: 2025-01-09