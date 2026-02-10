# CLI Style Alignment Plan (Missing Commands)

This plan targets CLI commands/subcommands that currently do not use `internal/cli/styles` renderers and also do not use Charmbracelet TUI packages (`lipgloss`, `bubbles`, `bubbletea`) in their execution path.

Excluded by request: `help`, `completion`, `gen-docs`, `browse`.

## Scope

Commands/subcommands to align:

- `dumber config open`
- `dumber crashes show <report|latest>`
- `dumber crashes issue <report|latest>`
- `dumber sessions list`
- `dumber sessions restore <session-id>`
- `dumber sessions delete <session-id>`

Non-goals:

- No changes to use cases, repositories, or adapters.
- No changes to output formats for `--json` flags.

## Approach

1. Add small, focused renderers under `internal/cli/styles/` for these command outputs.
2. Update the Cobra handlers in `internal/cli/cmd/` to use those renderers for:
   - Success messages
   - “not found / nothing to do” messages
   - User-facing errors (print styled message and return `nil`, consistent with existing commands like `config status`)
3. Keep “raw content” outputs raw where users likely pipe/copy them (crash report markdown), and only style surrounding framing text.

## Proposed Renderer Additions

- `internal/cli/styles/sessions_cli.go`
  - `type SessionsCLIRenderer struct { theme *Theme }`
  - `RenderListHeader(limit int) string`
  - `RenderListEmpty() string`
  - `RenderListRow(info entity.SessionInfo) string` (status icon + counts + relative time styling)
  - `RenderRestoreStarted(sessionID string) string`
  - `RenderDeleteOK(sessionID string) string`
  - `RenderError(err error) string`

- `internal/cli/styles/crashes_cli.go`
  - `type CrashesCLIRenderer struct { theme *Theme }`
  - `RenderShowHeader(id string, generatedAt string) string` (optional)
  - `RenderIssueHeader(id string) string` (optional, default off if it would break copy/paste)
  - `RenderError(err error) string`

- `internal/cli/styles/config_cli.go` (or extend existing `config.go`)
  - Add `RenderOpening(path, editor string) string`
  - Add `RenderOpenHint(path string) string` (e.g., when file missing)
  - Reuse `RenderError(err)` from `ConfigRenderer`

## Command Changes

- `internal/cli/cmd/config.go`
  - In `runConfigOpen`:
    - Use `styles.NewConfigRenderer(app.Theme)` (requires `GetApp()`/theme).
    - Print styled “opening …” message (optional).
    - For user-facing failures (missing config, missing editor), print renderer error/hint and return `nil` so Cobra doesn’t print a second unstyled error.

- `internal/cli/cmd/crashes.go`
  - In `runCrashesShow` / `runCrashesIssue`:
    - Use a new `CrashesCLIRenderer` for framing/errors.
    - Keep the markdown body as-is on stdout.

- `internal/cli/cmd/sessions.go`
  - In `runSessionsList`:
    - Keep `--json` identical.
    - Replace plain tabwriter output with themed lines (or a boxed, fixed-width table) produced by `SessionsCLIRenderer`.
  - In `runSessionsRestore` / `runSessionsDelete`:
    - Replace plain `fmt.Printf` success lines with themed status lines using icons from `internal/cli/styles/icons.go`.
    - Style user-facing errors similarly (print and return `nil`).

## Tests

Add renderer unit tests (no bubbletea runtime needed):

- `internal/cli/styles/sessions_cli_test.go`
- `internal/cli/styles/crashes_cli_test.go`
- `internal/cli/styles/config_cli_test.go` (if adding new config-open rendering)

Test strategy:

- Use `styles.NewTheme(config.DefaultConfig())` to build a theme.
- Assert key substrings are present (icons, command nouns, session IDs), avoiding strict ANSI comparisons.

## Verification

- `go test ./...`
- `make lint`
- Manual smoke:
  - `go run ./cmd/dumber sessions list`
  - `go run ./cmd/dumber sessions delete <id>`
  - `go run ./cmd/dumber crashes show latest`

