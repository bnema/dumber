# Home page

`dumb://home` is Dumber's start page. It shows built-in widgets (search, clock, favorites, top sites, recent history), bookmark groups and service cards for your own services. Service cards can have a live API widget, an embedded panel and action buttons.

## Files

| File | Purpose |
|------|---------|
| `~/.config/dumber/home.toml` | Page layout. Without it, a default layout is used. **Open config** on the page creates a starter file. |
| `~/.config/dumber/user.css` | Your CSS for every `dumb://` page. |

Both files reload while the browser runs: save the file and open pages update. When `home.toml` has a syntax or validation error, the page keeps the last valid layout and lists the errors with their key path.

Two settings in `config.toml` open the page:

```toml
[workspace]
startup_url = "dumb://home"  # first window when no URL is given and no session is restored
new_pane_url = "dumb://home" # new panes and tabs
```

## home.toml reference

Unknown keys are errors. Text values are trimmed.

### `[settings]`

| Key | Type | Default | Rules |
|-----|------|---------|-------|
| `title` | string | `"Home"` | Empty means the default |
| `columns` | int | `3` | `1`-`6` |

### `[[widgets]]`

Built-in widgets, shown at the top in file order. Without `home.toml` the default is search, clock (24h), favorites, top sites and recent (limit 8 each).

| Key | Type | Applies to | Default | Rules |
|-----|------|-----------|---------|-------|
| `type` | string | all | - | `search`, `clock`, `favorites`, `top_sites`, `recent` |
| `format` | string | `clock` | `"24h"` | `24h` or `12h` |
| `limit` | int | `favorites`, `top_sites`, `recent` | `8` | `1`-`50` |
| `tag` | string | `favorites` | - | Show only favorites with this tag |

### `[[bookmarks]]`

| Key | Type | Rules |
|-----|------|-------|
| `group` | string | Required |
| `links` | array of `{ name, url }` | `name` required, `url` must be `http(s)://` |

### `[[groups]]` and `[[groups.services]]`

| Key | Type | Rules |
|-----|------|-------|
| `groups.name` | string | Required |
| `id` | string | Required, `[a-z0-9_-]{1,64}`, unique across all groups |
| `name` | string | Required |
| `url` | string | Optional, `http(s)://`; the card links to it |
| `description` | string | Optional |
| `icon` | string | Optional path relative to `~/.config/dumber/`, no `..`, extension `.svg`, `.png`, `.ico` or `.webp`. Without it, the icon is discovered from `url`. |

### `[groups.services.widget]`

| Key | Type | Applies to | Default | Rules |
|-----|------|-----------|---------|-------|
| `type` | string | all | - | `api` or `embed` |
| `url` | string | all | - | `http(s)://`; see [Embeds](#embeds) for embed limits |
| `headers` | table | `api` | - | Not allowed on `embed` |
| `refresh` | duration | `api` | `"60s"` | At least `5s` |
| `timeout` | duration | `api` | `"10s"` | Above `0`, at most `60s` |
| `display` | string | `api` | `"stats"` | `stats`, `list`, `bar` |
| `fields` | array | `api` | - | At least one, unless `template` is set |
| `items` | string | `api` | - | Path to an array, only with `display = "list"` |
| `template` | string | `api` | - | HTML template, at most 16 KiB. Replaces `display`. Not allowed on `embed` |
| `height` | int (px) | `embed` | `240` | `80`-`2000` |

Durations use Go syntax: `30s`, `5m`, `1h30m`.

Each field is `{ label, path, format }`. `label` and `path` are required, and `format` defaults to `text`.

### `[[groups.services.actions]]`

| Key | Type | Default | Rules |
|-----|------|---------|-------|
| `id` | string | - | Required, `[a-z0-9_-]{1,64}`, unique per service |
| `label` | string | - | Required |
| `method` | string | `"POST"` | `GET`, `POST`, `PUT`, `PATCH`, `DELETE` |
| `url` | string | - | `http(s)://` |
| `headers` | table | - | |
| `body` | string | - | Request body |
| `confirm` | string | - | When set, the page asks this question before sending |
| `inputs` | array | - | Values typed by the user, see [Actions](#actions) |

## API widgets

Dumber requests `url` (GET, at most 2 MiB, User-Agent `dumber-home`). It follows at most 3 redirects, and only within the same scheme, host and port: a redirect to another origin or from https to http is an error, so headers and bodies never leave the configured service. Dumber then parses the JSON and shows each field.

**Field paths** use dot keys and `[n]` indexes, starting from the JSON root:

| Path | Reads |
|------|-------|
| `MovieCount` | `{"MovieCount": 42}` |
| `data.items[0].count` | nested objects and arrays |
| `[2].name` | a root array |
| `"a.b".c` | a key that contains dots |

**Formats:**

| Format | Example |
|--------|---------|
| `text` | value as-is |
| `number` | `1,234,567.89` (at most 2 decimals) |
| `percent` | `0.42` and `42` both show `42%` (values up to 1 are ratios) |
| `bytes` | `1.5 KiB`, `3.2 GiB` |
| `duration` | seconds as `1h 2m`, `3d 4h` |

**Lists:** with `display = "list"`, set `items` to the path of an array. Each element becomes a row, and field paths are read from the element. Without `items`, a JSON root array is used.

```toml
[groups.services.widget]
type = "api"
url = "https://example.local/api/jobs"
display = "list"
items = "data.jobs"
fields = [ { label = "Job", path = "name" }, { label = "Done", path = "progress", format = "percent" } ]
```

**Templates:** `template` renders your own HTML with Go `html/template` syntax. It receives `.data` (the parsed JSON) and `.fields` (formatted field values by label). Helpers: `format "number" .data.x` (any field format), `default "n/a" .data.x`, `join ", " .data.list`. Dumber renders it in the browser process (at most 2 s and 64 KiB of output) and sanitizes the result with an allowlist:

- Kept: basic text, list, table and layout tags (`div`, `span`, `p`, `strong`, `ul`, `table`, `h1`-`h6`, `progress`, `meter`, ...), `a` and `img` with `http(s)` URLs (and `data:image/*` for images), `class`, `title`, `data-*` attributes and a few inline styles (`color`, `background-color`, `width`, `height`, `display`, ...).
- Removed: `script`, `style`, `iframe`, forms, `svg`, event handlers, `id`, `name`, `data-sv-*`, and any `javascript:` or `url(...)` value.

A template error shows `template error: ...` on the card, and the template is not run again until you edit that widget. `{{ range }}` over a number (such as `{{ range 10 }}` or `{{ range len .data.x }}`) is rejected; range over arrays from `.data` instead.

```toml
[groups.services.widget]
type = "api"
url = "https://example.local/api/stats"
fields = [ { label = "Movies", path = "MovieCount", format = "number" } ]
template = """
<strong>{{ index .fields "Movies" }}</strong> movies
{{ if .data.Updating }}<em>updating</em>{{ end }}
"""
```

**Secrets:** write `{{env.NAME}}` in `url`, `headers` and action `url`, `headers` and `body`. Dumber replaces it with the environment variable `NAME` of the browser process. A missing variable is an error that names the variable. Keep tokens in the environment, not in `home.toml`.

## Embeds

`embed` shows `url` in an iframe, `height` pixels tall. The embedded site must allow framing: some services send `X-Frame-Options` or `frame-ancestors` and refuse to be embedded.

- **URL:** absolute `http(s)://` only, without user name or password. Dumber's internal host `dumber.invalid` and its subdomains are refused, including look-alike spellings (the host is IDNA-mapped like a browser does, and hosts with `%` escapes are refused).
- **Sandbox:** `allow-scripts allow-same-origin allow-forms allow-popups`, with `referrerpolicy="no-referrer"`. The embed runs as its own site but cannot navigate the home page.
- **Isolation:** an embed never reaches Dumber's page bridge or internal API, cannot navigate itself or a popup to a `dumb://` page, and cannot open one in a popup.
- **Local network:** embeds may reach LAN and private addresses. Dumber grants this to `dumb://` pages only; regular sites must ask (see [Permissions](../reference/permissions.md)).
- **Reloads:** saving `home.toml` keeps every unchanged embed running with its loaded page. An embed reloads only when its URL or height changes, or when services are reordered.

## Actions

Each action is a button on the service card. With `confirm`, the page asks for confirmation first. The browser process sends the request and the page only shows whether it succeeded. Only one request per action runs at a time.

**Inputs** ask the user for values before sending. Each input is `{ name, label, placeholder }`. `name` matches `[A-Za-z_][A-Za-z0-9_]{0,63}` and is unique per action, `label` is required and `placeholder` is optional. Use `{{input.NAME}}` in two places only:

- `url`, where the value is URL-encoded.
- A JSON `body`, inside a string literal such as `body = '{"query": "{{input.query}}"}'`, where the value is JSON-escaped. The body must be valid JSON once inputs are filled in.

Inputs in headers, in a non-JSON body, or outside a JSON string are rejected when `home.toml` loads, and so is an input that is not declared. Typed values are inserted as plain text: `{{env.X}}` typed by a user is never expanded.

```toml
[[groups.services.actions]]
id = "search"
label = "Search"
url = "https://example.local/api/search?q={{input.query}}"
inputs = [ { name = "query", label = "Title", placeholder = "Movie title" } ]
```

## Service icons

Each service card shows a 128px icon:

1. `icon`, a local file relative to `~/.config/dumber/` (`.svg`, `.png`, `.ico`, `.webp`).
2. Otherwise Dumber reads the page at `url` and picks the best candidate: SVG icon, then web app manifest icons, then `apple-touch-icon`, then the favicon (`/favicon.ico` last).
3. When nothing works, the card shows the first letter of the service name.

Discovery runs in the browser process. On private and LAN addresses it only follows the service's own host and port, so links, manifests and redirects cannot send it to other machines on your network.

## Security model

- `home.toml` and `user.css` cannot run JavaScript.
- Secrets stay in the browser process. The page never sees headers, action URLs, action bodies or API widget URLs; it only receives formatted field values, sanitized template HTML and action results.
- Requests may target private and LAN addresses, so you can reach your own services.
- Requests carry no cookies.

## Styling with user.css

`~/.config/dumber/user.css` is loaded after the built-in styles on every `dumb://` page.

**Theme variables** (set on `:root`, follow the browser theme):

| Group | Variables |
|-------|-----------|
| Colors | `--sv-background`, `--sv-surface`, `--sv-surface-variant`, `--sv-text`, `--sv-muted`, `--sv-muted-text`, `--sv-accent`, `--sv-accent-ink`, `--sv-accent-text`, `--sv-success`, `--sv-danger`, `--sv-danger-text`, `--sv-selection`, `--sv-hover`, `--sv-hover-strong`, `--sv-chip` |
| Borders | `--sv-border`, `--sv-hairline`, `--sv-content-border`, `--sv-panel-border` |
| Radii | `--sv-radius-panel`, `--sv-radius-control`, `--sv-radius-row`, `--sv-radius-chip` |
| Type | `--sv-font-sans`, `--sv-font-mono`, `--sv-font-size`, `--sv-header-font` |
| Effects | `--sv-float-shadow`, `--sv-ease` |

**Page classes:** the page root has `sv-route-<page>` (`sv-route-home`, `sv-route-history`, `sv-route-favorites`, `sv-route-config`), so you can target one page.

**Home classes:**

| Part | Classes |
|------|---------|
| Page | `sv-home`, `sv-home-region` (`sv-home-region-groups` for the service area), `sv-home-header`, `sv-home-title`, `sv-home-open-config`, `sv-home-section-title`, `sv-home-muted` |
| Widgets | `sv-home-widget`, `sv-home-widget-<type>`, `sv-home-clock`, `sv-home-clock-time`, `sv-home-clock-date`, `sv-home-search`, `sv-home-search-input`, `sv-home-suggestions`, `sv-home-suggestion`, `sv-home-rows`, `sv-home-row`, `sv-home-row-title`, `sv-home-row-url`, `sv-home-loading`, `sv-home-skeleton` |
| Bookmarks | `sv-home-bookmarks`, `sv-home-bookmark-group` |
| Services | `sv-home-groups`, `sv-home-group`, `sv-home-grid`, `sv-home-service`, `sv-home-service-tile`, `sv-home-service-link`, `sv-home-service-icon`, `sv-home-service-icon-has-image`, `sv-home-service-glyph`, `sv-home-service-text`, `sv-home-service-name`, `sv-home-service-description`, `sv-home-service-widget`, `sv-home-service-actions`, `sv-home-service-embed`, `sv-home-embed` |
| API widget | `sv-home-svc`, `sv-home-svc-display-<display>`, `sv-home-svc-loading`, `sv-home-svc-stale`, `sv-home-svc-stale-badge`, `sv-home-svc-error`, `sv-home-svc-stats`, `sv-home-svc-field`, `sv-home-svc-label`, `sv-home-svc-value`, `sv-home-svc-list`, `sv-home-svc-row`, `sv-home-svc-cell`, `sv-home-svc-bars`, `sv-home-svc-bar`, `sv-home-svc-track`, `sv-home-svc-fill`, `sv-home-svc-html` |
| Actions | `sv-home-svc-actions`, `sv-home-svc-action`, `sv-home-svc-chip`, `sv-home-svc-confirm`, `sv-home-svc-confirm-text`, `sv-home-svc-form`, `sv-home-svc-form-buttons` |
| Errors | `sv-home-errors`, `sv-home-errors-title`, `sv-home-error-list`, `sv-home-error`, `sv-home-error-path` |

Layout variables: `--sv-home-columns` (from `settings.columns`) on `sv-home-groups`, and `--sv-home-bar` (fill percent) on each `sv-home-svc-bar`.

Example:

```css
:root { --sv-accent: #8aadf4; }
.sv-route-home .sv-home-service { border-radius: 4px; }
```

## Examples

Ready-made snippets are in [`examples/`](./examples/). API paths in them are examples: check them against each service's API docs and version.

- [Jellyfin](./examples/jellyfin.toml): library counts
- [Uptime Kuma](./examples/uptime-kuma.toml): status page heartbeat
- [AdGuard Home](./examples/adguard.toml): query stats and a protection toggle
- [Portainer](./examples/portainer-actions.toml): container restart action
- [Grafana](./examples/grafana-embed.toml): an embedded panel

## Known limits

- Embedded sites run in a third-party iframe, so their login cookies are usually blocked. For Grafana, only anonymous or public panels work.
- API widgets read JSON only.
- HTTPS certificates are verified. A service with a self-signed certificate gets the letter icon: set a local `icon` instead.
- A failed icon lookup is remembered for 10 minutes. Editing the service's `url` or `icon` retries at once.
