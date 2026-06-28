# Systemviews WASM CRUD and Architecture Plan

This note captures the parity target for the `feat/systemviews-wasm` branch after merging `main`.

## Current branch baseline

- Worktree: `.worktrees/feat/systemviews-wasm`
- Branch: `feat/systemviews-wasm`
- Merge baseline: `d84415e5` (`Merge branch 'main' into feat/systemviews-wasm`)
- Vendor baseline: `go mod vendor` has been run in the worktree and `go list -mod=vendor ./...` succeeds there.
- Root-checkout vendoring diagnostics were corrected separately with `go mod vendor`; branch validation should still use `-mod=vendor` after dependency changes.

## Clean architecture contract

Keep the dependency direction explicit:

| Layer | Role for systemviews | Must not do |
| --- | --- | --- |
| `internal/domain` | Pure entities/value objects and pure helpers such as history range parsing. | Import ports, usecases, adapters, GTK/CEF/WebKit, or config packages. |
| `internal/application/usecase` | Own favorites/history/config/keybinding behavior and validation. | Import infrastructure adapters or UI packages. |
| `internal/application/port` | Define the systemview-facing contracts and DTOs. | Depend on CEF/WebKit/SQLite/config concrete types. |
| `internal/infrastructure/*` | Implement handlers, scheme serving, bridge transports, persistence, config payload readers. | Put UI workflow or business rules here. |
| `internal/ui/systemviews` | Render and orchestrate WASM UI through ports. | Call repositories, config package, CEF/WebKit, or SQLite directly. |
| `internal/ui/dispatcher` / coordinators | Convert native actions into pane/tab/navigation orchestration. | Bypass usecases for domain state changes. |

Preferred direction for new work:

1. Add behavior to existing usecases first when it is business logic.
2. Expose it through application ports.
3. Implement WebUI/systemview handlers in `internal/infrastructure/handlers`.
4. Consume those handlers from `internal/infrastructure/systemviewsbridge`.
5. Render and bind user interactions in `internal/ui/systemviews`.

## Old Svelte UI CRUD matrix

### History (`webui/src/pages/homepage/*`)

| Operation | Old Svelte surface | Backend message/API | Current WASM status | Gap |
| --- | --- | --- | --- | --- |
| Read timeline | `HistoryPanel.svelte`, `HistoryTimeline.svelte`, `HistoryItem.svelte` | `history_timeline` | Present, but bare list only (`internal/ui/systemviews/history_view.go`). | Add pagination, loading/error/empty states, row metadata, favicons, visit counts. |
| Read/search | `HistorySearch.svelte`, `keyboard.ts` local fuzzy + FTS | `history_search_fts` | Bridge client supports it. | Add search UI, debounce/local filtering, FTS fallback, visible result count. |
| Read filters/stats | `HistoryFilters.svelte`, `AnalyticsPanel.svelte` | `history_domain_stats`, `history_analytics` | Bridge client supports both; config route renders keybinding/config summaries only. | Add domain chips, stats summary, analytics cards/charts or text equivalent. |
| Create | Browser navigation writes history outside the systemview UI. | Existing history repository/usecases. | Not a user-facing CRUD action. | No manual create needed. |
| Update | Not supported in old UI. | None. | None. | Keep out of scope unless a future title-edit requirement appears. |
| Delete one | `HistoryItem.svelte`, `keyboard.ts` | `history_delete_entry` | Bridge client supports it; UI not interactive. | Add row delete action with confirmation and refresh. |
| Delete range/all | `HistoryCleanup.svelte`, `keyboard.ts` | `history_delete_range`, `history_clear_all` | Bridge client supports range; route UI not interactive. | Add cleanup modal/actions for hour/day/week/month/all. |
| Delete domain | `keyboard.ts`, `HistoryFilters.svelte` | `history_delete_domain` | Bridge client supports it. | Add domain-action UI and confirmation. |
| Open row | `navigateTo(entry.url)` | Browser navigation | Static links exist. | Add keyboard open and active-row focus management. |

### Favorites and tags (`webui/src/pages/homepage/favorites/*`)

| Operation | Old Svelte surface | Backend message/API | Current WASM status | Gap |
| --- | --- | --- | --- | --- |
| Read favorites | `FavoritesPanel.svelte`, `FavoriteGrid.svelte`, `FavoriteCard.svelte` | `favorite_list` | Present, but bare link list (`favorites_view.go`) with tags-first data. | Add rich rows/cards, favicons, shortcut badges, and tag metadata. |
| Create favorite | Old homepage hints imply adding/pinning from history; native omnibox toggle exists outside this page. | `ManageFavoritesUseCase.Add` exists, but no homepage/systemview message. | Missing from `port.HomepageFavorites`/`SystemviewFavoritesService` and handlers. | Add `favorite_create` handler/bridge and UI. |
| Update favorite | `FavoriteEditor.svelte` locally updates title, shortcut, and tags. | `favorite_set_shortcut`, `tag_assign`, `tag_remove`; no title update handler. | Partial bridge support. | Add `favorite_update` for title/favicon/url policy; keep shortcut/tag specialized calls or fold into update after design. |
| Delete favorite | `FavoriteEditor.svelte` local-only delete placeholder. | `ManageFavoritesUseCase.Remove` exists, but no homepage/systemview message. | Missing. | Add `favorite_delete` handler/bridge/UI. |
| Read tags | `TagCloud.svelte` | `tag_list` | Present as static list. | Add color chips and selection. |
| Create tag | `TagCloud.svelte` inline create | `tag_create` | Bridge and handler exist. | Add UI with color picker/palette. |
| Update tag | Handler exists (`tag_update`). | `tag_update` | Bridge exists. | Add rename/color edit UI. |
| Delete tag | Handler exists (`tag_delete`). | `tag_delete` | Bridge exists. | Add confirm UI and refresh assignments. |
| Assign/remove tag | `FavoriteEditor.svelte` | `tag_assign`, `tag_remove` | Bridge exists. | Add editor UI. |

### Config and keybindings (`webui/src/pages/ConfigPage.svelte`, `webui/src/pages/config/*`)

| Operation | Old Svelte surface | Backend message/API | Current WASM status | Gap |
| --- | --- | --- | --- | --- |
| Read config | `ConfigPage.svelte` | `/api/config` | Bridge supports direct API and route renders flattened values. | Replace flattened read-only view with grouped editable forms. |
| Read defaults | Reset dialog in `ConfigPage.svelte` | `/api/config/default` | Bridge supports direct API. | Add reset-to-default flow. |
| Save config | `save_config` via native bridge | `save_config` | Bridge supports it. Existing app config watcher hot-reloads appearance/keybindings/engine settings. | Add form save flow, validation, success/error UI, refresh after save. |
| Appearance update | Fonts, UI scale, color scheme, palettes | `save_config` | Payload shape exists in `SystemviewConfigPayload`. | Add form controls and palette validation. |
| Search shortcuts CRUD | `ShortcutsTable.svelte` | Saved inside config payload | Missing interactive UI. | Add table/list editing and `%s`/`{query}` validation. |
| Performance update | Performance tab | `save_config` | Payload shape exists; CEF gating needed. | Add profile/custom fields and restart warning. |
| Keybindings read | `KeybindingsTab.svelte` | `get_keybindings` | Route renders read-only keybinding groups. | Add editable keybinding UI. |
| Keybindings update | `KeyCaptureModal.svelte` | `set_keybinding`, `reset_keybinding`, `reset_all_keybindings` | Bridge supports message types. | Add capture/reset UI and conflict display. |

## Native shortcut and split-pane target

Add first-class global actions:

| Action | Default key | Target behavior |
| --- | --- | --- |
| `toggle_history_systemview` | `ctrl+h` | Split active pane to the right with `dumb://history`, focus an existing history systemview pane, or close it if already active. |
| `toggle_favorites_systemview` | Unset by default. | Split/focus/toggle `dumb://favorites`. |
| `toggle_config_systemview` | Unset by default; user can bind in config. | Split/focus/toggle `dumb://config`. |

Implementation path:

1. Add input actions in `internal/ui/input/shortcuts.go` and config defaults/schema in `internal/infrastructure/config`.
2. Map the actions in global shortcuts and keybinding DTOs.
3. Add a coordinator method such as `WorkspaceCoordinator.SplitURL(ctx, SplitRight, "dumb://history")` so dispatcher does not manipulate pane internals.
4. Use existing `ManagePanesUseCase.Split` with `SplitPaneInput.InitialURL` to keep domain/usecase clean.
5. Add tests in input, dispatcher, config defaults/schema, and workspace coordinator.

## Completed branch notes

- Rendering now uses typed `github.com/a-h/templ` components under `internal/ui/systemviews/*.templ`; generated `*_templ.go` files are committed alongside source templates.
- `make build-systemviews` runs `go tool templ generate -path internal/ui/systemviews -include-version=false` before building the WASM runtime.
- History supports search, domain filters, pagination, analytics/domain summaries, single/range/domain deletion, row open actions, alerts, and keyboard navigation.
- Favorites supports create/update/delete for favorites and tags; tag filters; shortcut editing; tag assignment/removal; alerts; and keyboard navigation.
- Config supports editable appearance, search defaults, search shortcut CRUD, performance profile/custom values, keybinding set/reset/reset-all, success/error states, and refreshes the config payload/theme after save.
- Native global actions use `toggle_*_systemview` names and open/focus/close History/Favorites/Config system views through the coordinator/right-split path.
- Trust-boundary hardening includes templ HTML escaping, URL scheme sanitization for rendered links, sanitized CSS color use for tag swatches, serialized DOM action handling, and route-level error states.

## Implementation notes

- Do not resurrect `webui/`; the branch intentionally removed Svelte assets.
- Keep WASM UI code small and stateful. Avoid pushing business rules into HTML string helpers.
- Prefer explicit bridge request methods over ad-hoc message strings in UI code.
- Every destructive action needs confirmation and a visible refresh/error path.
- Every feature should remain usable by keyboard because these pages are browser management surfaces, not marketing pages.
