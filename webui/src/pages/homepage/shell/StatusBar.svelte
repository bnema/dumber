<script lang="ts">
  import { homepageState } from '../state.svelte';
  import { History, Star, BarChart3, Terminal, Search, Folder, Tags, Command } from '@lucide/svelte';

  // Derived status indicators
  const panelLabel = $derived({
    history: 'HISTORY',
    favorites: 'FAVORITES',
    analytics: 'ANALYTICS',
  }[homepageState.activePanel]);

  const panelIcon = $derived({
    history: History,
    favorites: Star,
    analytics: BarChart3,
  }[homepageState.activePanel]);

  // Loading states
  const isLoading = $derived(
    homepageState.historyLoading ||
    homepageState.favoritesLoading ||
    homepageState.analyticsLoading
  );

  // Current list stats
  const listStats = $derived.by(() => {
    switch (homepageState.activePanel) {
      case 'history':
        if (homepageState.historySearchQuery) {
          return {
            count: homepageState.historySearchResults.length,
            label: 'results',
            filtered: true,
          };
        }
        return {
          count: homepageState.history.length,
          label: 'entries',
          filtered: false,
        };
      case 'favorites': {
        const filtered = homepageState.filteredFavorites;
        const total = homepageState.favorites.length;
        return {
          count: filtered.length,
          label: filtered.length === total ? 'items' : `of ${total}`,
          filtered: filtered.length !== total,
        };
      }
      case 'analytics':
        return {
          count: homepageState.domainStats.length,
          label: 'domains',
          filtered: false,
        };
      default:
        return { count: 0, label: 'items', filtered: false };
    }
  });

  // Position in list
  const positionInfo = $derived.by(() => {
    const length = homepageState.currentListLength;
    if (length === 0) return null;

    const pos = homepageState.focusedIndex + 1;
    return { current: pos, total: length };
  });

  // Mode indicator
  const modeIndicator = $derived.by(() => {
    if (homepageState.commandPaletteOpen) {
      return { icon: Terminal, label: 'COMMAND', color: 'accent' };
    }
    if (homepageState.historySearchQuery) {
      return { icon: Search, label: 'SEARCH', color: 'warning' };
    }
    if (homepageState.selectedFolderId !== null) {
      const folder = homepageState.folders.find(f => f.id === homepageState.selectedFolderId);
      return { icon: Folder, label: folder?.name || 'FOLDER', color: 'info' };
    }
    if (homepageState.selectedTagIds.length > 0) {
      return { icon: Tags, label: `${homepageState.selectedTagIds.length} TAGS`, color: 'info' };
    }
    return null;
  });
</script>

<footer class="status-bar">
  <!-- Left Section: Mode & Panel -->
  <div class="status-left">
    <!-- Pending Key Prefix Indicator -->
    {#if homepageState.pendingKeyPrefix}
      <div class="status-chip prefix-chip" data-color="warning">
        <span class="chip-icon">
          <Command size={12} strokeWidth={2} />
        </span>
        <kbd class="prefix-key">{homepageState.pendingKeyPrefix}</kbd>
        <span class="prefix-waiting">...</span>
      </div>
    {/if}

    <!-- Mode Indicator -->
    {#if modeIndicator}
      {@const ModeIcon = modeIndicator.icon}
      <div class="status-chip mode-chip" data-color={modeIndicator.color}>
        <span class="chip-icon">
          <ModeIcon size={12} strokeWidth={2} />
        </span>
        <span class="chip-text">{modeIndicator.label}</span>
      </div>
    {/if}

    <!-- Panel Indicator -->
    <div class="status-chip panel-chip">
      <span class="chip-icon">
        {#if panelIcon}
          {@const PanelIcon = panelIcon}
          <PanelIcon size={12} strokeWidth={2} />
        {/if}
      </span>
      <span class="chip-text">{panelLabel}</span>
    </div>

    <!-- Loading Indicator -->
    {#if isLoading}
      <div class="status-chip loading-chip">
        <span class="loading-spinner"></span>
        <span class="chip-text">SYNC</span>
      </div>
    {/if}
  </div>

  <!-- Center Section: Stats -->
  <div class="status-center">
    <div class="stats-group">
      <span class="stats-count" class:filtered={listStats.filtered}>
        {listStats.count}
      </span>
      <span class="stats-label">{listStats.label}</span>
    </div>

    {#if positionInfo}
      <div class="position-group">
        <span class="position-current">{positionInfo.current}</span>
        <span class="position-sep">/</span>
        <span class="position-total">{positionInfo.total}</span>
      </div>
    {/if}
  </div>

  <!-- Right Section: Quick Hints -->
  <div class="status-right">
    <div class="hint-item">
      <kbd>/</kbd>
      <span>cmd</span>
    </div>
    <div class="hint-item">
      <kbd>j</kbd><kbd>k</kbd>
      <span>nav</span>
    </div>
    <div class="hint-item">
      <kbd>?</kbd>
      <span>help</span>
    </div>
  </div>
</footer>

<style>
  .status-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.5rem 0.85rem;
    font-size: 0.68rem;
    font-family: ui-monospace, 'Fira Code', 'Cascadia Code', Menlo, Monaco, Consolas, monospace;
    letter-spacing: 0.05em;
    border-top: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 92%, var(--card) 8%);
    color: var(--muted-foreground);
  }

  /* Sections */
  .status-left,
  .status-center,
  .status-right {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  .status-left {
    flex: 1;
    justify-content: flex-start;
  }

  .status-center {
    flex: 0 0 auto;
    gap: 1rem;
  }

  .status-right {
    flex: 1;
    justify-content: flex-end;
    gap: 0.85rem;
  }

  /* Status Chips */
  .status-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.25rem 0.5rem;
    background: color-mix(in srgb, var(--card) 60%, transparent);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
  }

  .chip-icon {
    display: flex;
    align-items: center;
  }

  .chip-text {
    font-weight: 500;
    text-transform: uppercase;
  }

  /* Mode chip colors */
  .mode-chip[data-color="accent"] {
    background: color-mix(in srgb, var(--primary, #4ade80) 15%, transparent);
    border-color: color-mix(in srgb, var(--primary, #4ade80) 40%, var(--border) 60%);
    color: var(--primary, #4ade80);
  }

  .mode-chip[data-color="warning"] {
    background: color-mix(in srgb, var(--warning) 15%, transparent);
    border-color: color-mix(in srgb, var(--warning) 40%, var(--border) 60%);
    color: var(--warning);
  }

  .mode-chip[data-color="info"] {
    background: color-mix(in srgb, var(--info) 15%, transparent);
    border-color: color-mix(in srgb, var(--info) 40%, var(--border) 60%);
    color: var(--info);
  }

  /* Pending prefix indicator */
  .prefix-chip {
    animation: pulse-border 1s ease-in-out infinite;
  }

  .prefix-chip[data-color="warning"] {
    background: color-mix(in srgb, var(--warning) 15%, transparent);
    border-color: color-mix(in srgb, var(--warning) 50%, var(--border) 50%);
    color: var(--warning);
  }

  .prefix-key {
    font-size: 0.7rem;
    font-weight: 700;
    padding: 0.1rem 0.25rem;
    background: color-mix(in srgb, var(--warning) 25%, transparent);
    border: 1px solid color-mix(in srgb, var(--warning) 40%, var(--border) 60%);
    font-family: ui-monospace, 'Fira Code', 'Cascadia Code', Menlo, Monaco, Consolas, monospace;
  }

  .prefix-waiting {
    font-size: 0.65rem;
    opacity: 0.8;
    animation: blink 0.6s ease-in-out infinite;
  }

  @keyframes pulse-border {
    0%, 100% { border-color: color-mix(in srgb, var(--warning) 50%, var(--border) 50%); }
    50% { border-color: var(--warning); }
  }

  @keyframes blink {
    0%, 100% { opacity: 0.4; }
    50% { opacity: 1; }
  }

  .panel-chip {
    color: var(--foreground);
  }

  .panel-chip .chip-icon {
    color: var(--primary, #4ade80);
  }

  /* Loading */
  .loading-chip {
    color: var(--muted-foreground);
  }

  .loading-spinner {
    width: 0.7rem;
    height: 0.7rem;
    border: 1.5px solid var(--border);
    border-top-color: var(--primary, #4ade80);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  /* Stats */
  .stats-group {
    display: flex;
    align-items: baseline;
    gap: 0.35rem;
  }

  .stats-count {
    font-weight: 600;
    color: var(--foreground);
    font-variant-numeric: tabular-nums;
  }

  .stats-count.filtered {
    color: var(--primary, #4ade80);
  }

  .stats-label {
    color: var(--muted-foreground);
  }

  .position-group {
    display: flex;
    align-items: center;
    gap: 0.15rem;
    padding: 0.2rem 0.45rem;
    background: var(--background);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    font-variant-numeric: tabular-nums;
  }

  .position-current {
    font-weight: 600;
    color: var(--foreground);
  }

  .position-sep {
    color: var(--border);
  }

  .position-total {
    color: var(--muted-foreground);
  }

  /* Hints */
  .hint-item {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    color: var(--muted-foreground);
  }

  .hint-item kbd {
    padding: 0.1rem 0.3rem;
    font-size: 0.6rem;
    background: var(--background);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    color: var(--foreground);
  }

  .hint-item span {
    font-size: 0.62rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  /* Responsive */
  @media (max-width: 640px) {
    .status-bar {
      flex-wrap: wrap;
      gap: 0.5rem;
    }

    .status-left,
    .status-right {
      flex: 0 0 auto;
    }

    .status-center {
      order: -1;
      width: 100%;
      justify-content: center;
      padding-bottom: 0.35rem;
      border-bottom: 1px solid color-mix(in srgb, var(--border) 50%, transparent);
    }

    .hint-item span {
      display: none;
    }
  }

  @media (max-width: 480px) {
    .status-right {
      display: none;
    }

    .status-left {
      flex: 1;
    }
  }
</style>
