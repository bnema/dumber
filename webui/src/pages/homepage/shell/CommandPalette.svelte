<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { homepageState } from '../state.svelte';
  import { navigateTo } from '../messaging';
  import type { CommandPaletteItem } from '../types';
  import { Search, SearchX, History, Star, BarChart3, Trash2, Globe } from '@lucide/svelte';

  // Input element reference
  let inputRef: HTMLInputElement | null = $state(null);
  let listRef: HTMLDivElement | null = $state(null);

  // Local state for command palette
  let query = $state('');
  let selectedIndex = $state(0);

  // Build command items from available actions
  const staticCommands: CommandPaletteItem[] = [
    {
      id: 'nav:history',
      label: 'Go to History',
      description: 'View browsing history timeline',
      icon: History,
      shortcut: 'g h',
      action: () => {
        homepageState.setActivePanel('history');
        homepageState.closeCommandPalette();
      },
    },
    {
      id: 'nav:favorites',
      label: 'Go to Favorites',
      description: 'View bookmarks and folders',
      icon: Star,
      shortcut: 'g f',
      action: () => {
        homepageState.setActivePanel('favorites');
        homepageState.closeCommandPalette();
      },
    },
    {
      id: 'nav:analytics',
      label: 'Go to Analytics',
      description: 'View browsing statistics',
      icon: BarChart3,
      shortcut: 'g a',
      action: () => {
        homepageState.setActivePanel('analytics');
        homepageState.closeCommandPalette();
      },
    },
    {
      id: 'action:cleanup',
      label: 'Clean History',
      description: 'Remove history entries by time range',
      icon: Trash2,
      action: () => {
        homepageState.closeCommandPalette();
        homepageState.openCleanupModal();
      },
    },
  ];

  // Fuzzy match function
  function fuzzyMatch(text: string, pattern: string): { match: boolean; score: number } {
    if (!pattern) return { match: true, score: 0 };

    const lower = text.toLowerCase();
    const pLower = pattern.toLowerCase();

    // Exact substring match scores highest
    if (lower.includes(pLower)) {
      return { match: true, score: 100 - lower.indexOf(pLower) };
    }

    // Fuzzy character matching
    let pIdx = 0;
    let score = 0;
    let consecutive = 0;

    for (let i = 0; i < lower.length && pIdx < pLower.length; i++) {
      if (lower[i] === pLower[pIdx]) {
        pIdx++;
        consecutive++;
        score += consecutive * 2;
      } else {
        consecutive = 0;
      }
    }

    return { match: pIdx === pLower.length, score };
  }

  // Build filtered items based on query
  const filteredItems = $derived.by(() => {
    const results: Array<{ item: CommandPaletteItem; score: number }> = [];

    // Add static commands
    for (const cmd of staticCommands) {
      const labelMatch = fuzzyMatch(cmd.label, query);
      const descMatch = cmd.description ? fuzzyMatch(cmd.description, query) : { match: false, score: 0 };

      if (labelMatch.match || descMatch.match) {
        results.push({
          item: cmd,
          score: Math.max(labelMatch.score, descMatch.score * 0.8),
        });
      }
    }

    // Add history items as navigable commands
    for (const entry of homepageState.history.slice(0, 50)) {
      const titleMatch = fuzzyMatch(entry.title || '', query);
      const urlMatch = fuzzyMatch(entry.url, query);

      if (titleMatch.match || urlMatch.match) {
        results.push({
          item: {
            id: `history:${entry.id}`,
            label: entry.title || getDomain(entry.url),
            description: entry.url,
            icon: Globe,
            action: () => {
              navigateTo(entry.url);
            },
          },
          score: Math.max(titleMatch.score, urlMatch.score * 0.7),
        });
      }
    }

    // Add favorites as navigable commands
    for (const fav of homepageState.favorites) {
      const titleMatch = fuzzyMatch(fav.title || '', query);
      const urlMatch = fuzzyMatch(fav.url, query);

      if (titleMatch.match || urlMatch.match) {
        results.push({
          item: {
            id: `fav:${fav.id}`,
            label: fav.title || getDomain(fav.url),
            description: fav.url,
            icon: Star,
            shortcut: fav.shortcut_key ? `${fav.shortcut_key}` : undefined,
            action: () => {
              navigateTo(fav.url);
            },
          },
          score: Math.max(titleMatch.score, urlMatch.score * 0.7) + 10, // Boost favorites
        });
      }
    }

    // Sort by score and return items
    results.sort((a, b) => b.score - a.score);
    return results.map(r => r.item).slice(0, 20);
  });

  function getDomain(url: string): string {
    try {
      return new URL(url).hostname;
    } catch {
      return url;
    }
  }

  // Reset selection when query changes
  $effect(() => {
    void query; // Track query changes
    selectedIndex = 0;
  });

  // Scroll selected item into view
  $effect(() => {
    if (listRef) {
      const selected = listRef.querySelector(`[data-index="${selectedIndex}"]`);
      selected?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
  });

  function handleKeydown(e: KeyboardEvent) {
    switch (e.key) {
      case 'ArrowDown':
      case 'j':
        if (e.key === 'j' && !e.ctrlKey) break;
        e.preventDefault();
        selectedIndex = Math.min(selectedIndex + 1, filteredItems.length - 1);
        break;

      case 'ArrowUp':
      case 'k':
        if (e.key === 'k' && !e.ctrlKey) break;
        e.preventDefault();
        selectedIndex = Math.max(selectedIndex - 1, 0);
        break;

      case 'n':
        if (e.ctrlKey) {
          e.preventDefault();
          selectedIndex = Math.min(selectedIndex + 1, filteredItems.length - 1);
        }
        break;

      case 'p':
        if (e.ctrlKey) {
          e.preventDefault();
          selectedIndex = Math.max(selectedIndex - 1, 0);
        }
        break;

      case 'Enter':
        e.preventDefault();
        executeSelected();
        break;

      case 'Escape':
        e.preventDefault();
        homepageState.closeCommandPalette();
        break;
    }
  }

  function executeSelected() {
    const item = filteredItems[selectedIndex];
    if (item) {
      item.action();
    }
  }

  function handleItemClick(index: number) {
    selectedIndex = index;
    executeSelected();
  }

  function handleOverlayInteraction(event: MouseEvent | KeyboardEvent) {
    if (event instanceof KeyboardEvent) {
      if (event.key === 'Escape' || event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        homepageState.closeCommandPalette();
      }
      return;
    }

    if (event.currentTarget === event.target) {
      homepageState.closeCommandPalette();
    }
  }

  onMount(async () => {
    await tick();
    inputRef?.focus();
  });
</script>

<div
  class="palette-overlay"
  role="button"
  tabindex="0"
  aria-label="Close command palette"
  onclick={handleOverlayInteraction}
  onkeydown={handleOverlayInteraction}
>
  <div class="palette-container" onclick={(e) => e.stopPropagation()} role="presentation">
    <!-- Search Header -->
    <div class="palette-header">
      <div class="search-row">
        <span class="search-icon">
          <Search size={16} strokeWidth={2} />
        </span>
        <input
          bind:this={inputRef}
          bind:value={query}
          type="text"
          class="search-input"
          placeholder="Search commands, history, favorites..."
          spellcheck="false"
          autocomplete="off"
          onkeydown={handleKeydown}
        />
        <div class="search-meta">
          <span class="result-count">{filteredItems.length}</span>
          <kbd class="esc-hint">esc</kbd>
        </div>
      </div>
    </div>

    <!-- Results List -->
    <div class="palette-results" bind:this={listRef}>
      {#if filteredItems.length === 0}
        <div class="empty-results">
          <span class="empty-icon">
            <SearchX size={24} strokeWidth={1.5} />
          </span>
          <span class="empty-text">No matches found</span>
        </div>
      {:else}
        {#each filteredItems as item, index (item.id)}
          <button
            class="result-item"
            class:selected={index === selectedIndex}
            data-index={index}
            type="button"
            onclick={() => handleItemClick(index)}
            onmouseenter={() => selectedIndex = index}
          >
            <span class="item-icon">
              {#if typeof item.icon === 'function'}
                <item.icon size={16} strokeWidth={2} />
              {:else if item.icon}
                {item.icon}
              {/if}
            </span>
            <div class="item-content">
              <span class="item-label">{item.label}</span>
              {#if item.description}
                <span class="item-description">{item.description}</span>
              {/if}
            </div>
            {#if item.shortcut}
              <kbd class="item-shortcut">{item.shortcut}</kbd>
            {/if}
          </button>
        {/each}
      {/if}
    </div>

    <!-- Footer Hints -->
    <div class="palette-footer">
      <div class="hint-group">
        <kbd></kbd><kbd></kbd>
        <span>navigate</span>
      </div>
      <div class="hint-group">
        <kbd></kbd>
        <span>select</span>
      </div>
      <div class="hint-group">
        <kbd>esc</kbd>
        <span>close</span>
      </div>
    </div>
  </div>
</div>

<style>
  .palette-overlay {
    position: fixed;
    inset: 0;
    z-index: 200;
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 12vh;
    background: rgb(0 0 0 / 0.65);
    backdrop-filter: blur(8px);
    animation: overlay-in 150ms ease;
  }

  @keyframes overlay-in {
    from {
      opacity: 0;
      backdrop-filter: blur(0);
    }
  }

  .palette-container {
    width: 100%;
    max-width: 580px;
    margin: 0 1rem;
    display: flex;
    flex-direction: column;
    background: var(--card);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    box-shadow:
      0 0 0 1px color-mix(in srgb, var(--border) 30%, transparent),
      0 32px 64px -16px rgb(0 0 0 / 0.7);
    animation: palette-in 200ms cubic-bezier(0.16, 1, 0.3, 1);
  }

  @keyframes palette-in {
    from {
      opacity: 0;
      transform: scale(0.96) translateY(-12px);
    }
  }

  /* Header */
  .palette-header {
    padding: 0;
    border-bottom: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 70%, transparent);
  }

  .search-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.85rem 1rem;
  }

  .search-icon {
    font-size: 1rem;
    color: var(--primary, #4ade80);
    flex-shrink: 0;
  }

  .search-input {
    flex: 1;
    background: transparent;
    border: none;
    outline: none;
    font-size: 0.95rem;
    font-family: inherit;
    color: var(--foreground);
    caret-color: var(--primary, #4ade80);
  }

  .search-input::placeholder {
    color: var(--muted-foreground);
    opacity: 0.7;
  }

  .search-meta {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .result-count {
    font-size: 0.7rem;
    color: var(--muted-foreground);
    letter-spacing: 0.05em;
  }

  .esc-hint {
    padding: 0.2rem 0.4rem;
    font-size: 0.65rem;
    color: var(--muted-foreground);
    background: var(--background);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
  }

  /* Results */
  .palette-results {
    max-height: 360px;
    overflow-y: auto;
    overscroll-behavior: contain;
  }

  .empty-results {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 2.5rem 1rem;
    color: var(--muted-foreground);
  }

  .empty-icon {
    font-size: 1.5rem;
    opacity: 0.5;
  }

  .empty-text {
    font-size: 0.8rem;
    letter-spacing: 0.05em;
  }

  .result-item {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.7rem 1rem;
    background: transparent;
    border: none;
    border-bottom: 1px solid color-mix(in srgb, var(--border) 40%, transparent);
    cursor: pointer;
    text-align: left;
    transition: background 80ms ease;
  }

  .result-item:last-child {
    border-bottom: none;
  }

  .result-item:hover {
    background: color-mix(in srgb, var(--card) 80%, var(--background) 20%);
  }

  .result-item.selected {
    background: color-mix(in srgb, var(--primary, #4ade80) 12%, transparent);
    box-shadow: inset 2px 0 0 var(--primary, #4ade80);
  }

  .item-icon {
    font-size: 1rem;
    color: var(--muted-foreground);
    flex-shrink: 0;
    width: 1.5rem;
    text-align: center;
  }

  .result-item.selected .item-icon {
    color: var(--primary, #4ade80);
  }

  .item-content {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .item-label {
    font-size: 0.85rem;
    color: var(--foreground);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .item-description {
    font-size: 0.72rem;
    color: var(--muted-foreground);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .item-shortcut {
    padding: 0.2rem 0.45rem;
    font-size: 0.65rem;
    color: var(--muted-foreground);
    background: var(--background);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    flex-shrink: 0;
  }

  .result-item.selected .item-shortcut {
    border-color: color-mix(in srgb, var(--primary, #4ade80) 40%, var(--border) 60%);
    color: var(--foreground);
  }

  /* Footer */
  .palette-footer {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 1.5rem;
    padding: 0.65rem 1rem;
    border-top: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 70%, transparent);
  }

  .hint-group {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.68rem;
    color: var(--muted-foreground);
  }

  .hint-group kbd {
    padding: 0.15rem 0.35rem;
    font-size: 0.62rem;
    background: var(--background);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
  }

  /* Scrollbar */
  .palette-results::-webkit-scrollbar {
    width: 6px;
  }

  .palette-results::-webkit-scrollbar-track {
    background: transparent;
  }

  .palette-results::-webkit-scrollbar-thumb {
    background: var(--border);
  }

  .palette-results::-webkit-scrollbar-thumb:hover {
    background: var(--muted-foreground);
  }

  @media (max-width: 480px) {
    .palette-overlay {
      padding-top: 2rem;
      align-items: flex-start;
    }

    .palette-container {
      max-height: calc(100vh - 4rem);
    }

    .palette-footer {
      flex-wrap: wrap;
      gap: 0.75rem;
    }
  }
</style>
