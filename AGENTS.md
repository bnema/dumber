# Clean Architecture

## Layers

> **Note:** This document describes conceptual architecture layers. The actual project structure uses `internal/` prefixes (e.g., `internal/application/usecase/`, `internal/application/port/`, `internal/infrastructure/`) to organize the code.

| Layer | Contains | Depends On |
|-------|----------|------------|
| `app/` | Entry points, CLI/TUI, request handling | usecase |
| `usecase/` | Business logic, orchestration | domain, boundaries |
| `domain/` | Entities, value objects, utils | nothing |
| `boundaries/in` | Inbound interfaces (services) | domain |
| `boundaries/out` | Outbound interfaces (repos, clients) | domain |
| `adapters/` | Interface implementations | boundaries, domain |

## Rules

- **app**: Parse input, call usecase, format output. No business logic.
- **usecase**: All business logic. Use boundaries for I/O.
- **domain**: Pure functions only. No I/O.
- **boundaries**: Interfaces only. Mock with `mockery generate`.

## Anti-patterns strictly forbidden

```text
❌ app doing business logic
❌ app calling adapters directly
❌ usecase importing adapters
❌ domain importing anything
```

## Commands

