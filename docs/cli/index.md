# CLI Commands

Dumber provides commands for browser management, history, sessions, and diagnostics.

## Commands

| Command | Description |
|---------|-------------|
| `dumber browse` | Launch the graphical browser |
| `dumber dmenu` | Launcher integration for rofi/fuzzel |
| `dumber history` | Browse and manage history |
| `dumber sessions` | Manage browser sessions |
| `dumber config` | Manage configuration |
| `dumber doctor` | Check runtime requirements |
| `dumber setup` | Setup desktop integration |
| `dumber update` | Check for and install updates |
| `dumber logs` | View application logs |
| `dumber crashes` | Inspect unexpected-close reports |
| `dumber purge` | Remove data and configuration |
| `dumber about` | Show version information |
| `dumber gen-docs` | Generate documentation from CLI commands |
| `dumber completion` | Generate shell completions |

## Command Reference

### browse

Launch the graphical browser.

```bash
dumber browse [url]
```

### dmenu

Launcher integration for rofi/fuzzel.

```bash
dumber dmenu [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--interactive` | `-i` | Use interactive TUI mode |
| `--select` | | Process selection from stdin |
| `--days` | | Number of days of history to show |
| `--most-visited` | `-m` | Sort by visit count instead of recency |

**Examples:**
```bash
dumber dmenu | rofi -dmenu -show-icons | dumber dmenu --select
dumber dmenu | fuzzel --dmenu | dumber dmenu --select
dumber dmenu --interactive
dumber dmenu --days 7 --most-visited
```

### history

Browse and manage history.

```bash
dumber history [flags]
dumber history stats
dumber history clear
```

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |
| `--max` | Maximum entries to show (default: 50) |

**Subcommands:**
- `stats` - Show history statistics
- `clear` - Interactive history cleanup

### sessions

Manage browser sessions.

```bash
dumber sessions
dumber sessions list [flags]
dumber sessions restore <session-id>
dumber sessions delete <session-id>
```

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `list` | List saved sessions |
| `restore <id>` | Restore a saved session |
| `delete <id>` | Delete a saved session |

**list flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |
| `--limit` | Maximum sessions to show (default: 20) |

### config

Manage configuration.

```bash
dumber config open
dumber config status
dumber config migrate [flags]
dumber config schema [flags]
```

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `open` | Open config file in editor |
| `status` | Show config status and migration availability |
| `migrate` | Add missing default settings |
| `schema` | Show all config keys with types and values |

**migrate flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--yes` | `-y` | Skip confirmation prompt |

**schema flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |

### doctor

Check runtime requirements and diagnose issues.

```bash
dumber doctor [flags]
```

| Flag | Description |
|------|-------------|
| `--runtime` | Only run runtime checks (GTK4/WebKitGTK) |
| `--media` | Only run media checks (GStreamer/VA-API) |

### setup

Setup desktop integration.

```bash
dumber setup install
dumber setup default
```

| Subcommand | Description |
|------------|-------------|
| `install` | Install desktop file to ~/.local/share/applications/ |
| `default` | Set dumber as the default web browser |

### update

Check for and install updates.

```bash
dumber update [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--force` | `-f` | Force reinstall (skips version check) |

### logs

View application logs.

```bash
dumber logs [session]
dumber logs clear [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--follow` | `-f` | Follow log output in real-time |
| `--lines` | `-n` | Number of lines to show (default: 50) |

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `clear` | Clear old log files |

**clear flags:**

| Flag | Description |
|------|-------------|
| `--all` | Remove all session logs |

### crashes

Inspect crash reports generated automatically after unexpected closes.

```bash
dumber crashes
dumber crashes show <report|latest>
dumber crashes issue <report|latest>
```

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `show <report|latest>` | Show full crash report markdown |
| `issue <report|latest>` | Print GitHub-ready issue section |

### purge

Remove dumber data and configuration.

```bash
dumber purge [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--force` | `-f` | Remove all items without prompting |

### about

Show version and build information.

```bash
dumber about
```

### gen-docs

Generate documentation (man pages or markdown) from CLI commands.

```bash
dumber gen-docs [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--output` | `-o` | Output directory for generated docs |
| `--format` | `-f` | Output format: man, markdown (default: man) |

**Supported formats:**
- `man` - Unix manual pages (groff format)
- `markdown` - Markdown files (for websites/wikis)

**Examples:**
```bash
dumber gen-docs                     # Install man pages to ~/.local/share/man/man1/
dumber gen-docs --format markdown   # Generate markdown docs
dumber gen-docs --output ./man      # Generate to local directory
```

## Environment Variables

Configuration can be overridden via environment variables:

```bash
DUMBER_LOG_LEVEL=debug dumber browse
DUMBER_RENDERING_MODE=cpu dumber browse
```

Pattern: `DUMBER_SECTION_KEY` (uppercase, underscores)
