<!--
  Omnibox Suggestions Component

  Displays search suggestions with favicons and URL highlighting
-->
<script lang="ts">
  import { omniboxStore } from './stores.svelte.ts';
  import { omniboxBridge } from './messaging';

  // Reactive state
  let suggestions = $derived(omniboxStore.suggestions);
  let favorites = $derived(omniboxStore.favorites);
  let viewMode = $derived(omniboxStore.viewMode);
  let selectedIndex = $derived(omniboxStore.selectedIndex);
  let inputValue = $derived(omniboxStore.inputValue);
  let hasContent = $state(false);

  // Debug effect to log favorites changes
  $effect(() => {
    console.log('[OmniboxSuggestions] Favorites updated, count:', favorites.length);
  });

  // Computed: Get current list based on view mode with local filtering for favorites
  let currentList = $derived.by(() => {
    const list = viewMode === 'history' ? suggestions : favorites;

    // In favorites view with input, filter locally
    if (viewMode === 'favorites' && inputValue && inputValue.trim()) {
      const query = inputValue.toLowerCase().trim();
      return list.filter(fav =>
        fav.url.toLowerCase().includes(query) ||
        (fav.title && fav.title.toLowerCase().includes(query))
      );
    }

    return list;
  });

  // Update hasContent - always show in omnibox mode to display header
  $effect(() => {
    hasContent = omniboxStore.mode === 'omnibox';
  });

  // Check if a URL is in favorites
  function isFavorited(url: string): boolean {
    return favorites.some(fav => fav.url === url);
  }

  // Handle suggestion item mouse enter
  function handleItemMouseEnter(index: number) {
    omniboxStore.setSelectedIndex(index);
    omniboxStore.setFaded(false);
    scrollToSelection();
  }

  // Handle suggestion item click
  function handleItemClick(suggestion: any) {
    omniboxBridge.navigate(suggestion.url);
    omniboxStore.close();
  }

  // Handle keyboard activation on suggestion item
  function handleItemKeyDown(event: KeyboardEvent, suggestion: any) {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      handleItemClick(suggestion);
    }
  }

  // Scroll list to show selected item
  function scrollToSelection() {
    const selectedItem = document.getElementById(`omnibox-item-${selectedIndex}`);
    if (selectedItem && selectedItem.scrollIntoView) {
      try {
        selectedItem.scrollIntoView({ block: 'nearest' });
      } catch {
        selectedItem.scrollIntoView();
      }
    }
  }

  // Parse URL for display
  function parseUrl(url: string) {
    try {
      const urlObj = new URL(url, window.location.href);
      return {
        domain: urlObj.hostname,
        path: (urlObj.pathname || '') + (urlObj.search || '') + (urlObj.hash || '')
      };
    } catch {
      return {
        domain: url || '',
        path: ''
      };
    }
  }

  // Handle favicon error
  function handleFaviconError(event: Event) {
    const target = event.target as HTMLImageElement;
    const chip = target.parentElement;
    if (chip) {
      chip.style.display = 'none';
    }
  }

  // Watch for selection changes to scroll
  $effect(() => {
    if (selectedIndex >= 0) {
      scrollToSelection();
    }
  });
</script>

{#if hasContent}
  <div
    id="omnibox-list"
    class="suggestion-list"
    role="listbox"
    aria-label="Search suggestions"
  >
    <!-- View mode indicator -->
    <div class="view-mode-header">
      <span class={viewMode === 'history' ? 'view-tab active' : 'view-tab'}>
        History ({suggestions.length})
      </span>
      <span class={viewMode === 'favorites' ? 'view-tab active' : 'view-tab'}>
        Favorites ({favorites.length})
      </span>
      <span class="view-hint">Tab to switch</span>
    </div>

    {#if currentList.length === 0}
      <!-- Empty state -->
      {#if viewMode === 'favorites'}
        <!-- Always show empty state for favorites -->
        <div class="empty-state">
          <span class="empty-icon">⭐</span>
          <span class="empty-text">No favorites yet</span>
          <span class="empty-hint">Press Space on any item in History to add it here</span>
        </div>
      {:else if inputValue && inputValue.trim()}
        <!-- Only show "No results found" for history when user has typed something -->
        <div class="empty-state">
          <span class="empty-icon">🔍</span>
          <span class="empty-text">No results found</span>
        </div>
      {/if}
    {:else}
      {#each currentList as item, index (item.url)}
        {@const { domain, path } = parseUrl(item.url)}
        {@const isSelected = index === selectedIndex}
        {@const favicon = item.favicon || item.favicon_url || ''}
        {@const isFav = viewMode === 'history' && isFavorited(item.url)}

        <div
          id="omnibox-item-{index}"
          class:suggestion-item={true}
          class:selected={isSelected}
          class:favorited={isFav}
          role="option"
          tabindex={isSelected ? 0 : -1}
          aria-selected={isSelected}
          onmouseenter={() => handleItemMouseEnter(index)}
          onclick={() => handleItemClick(item)}
          onkeydown={(e) => handleItemKeyDown(e, item)}
        >
        <!-- Favicon chip -->
        {#if favicon}
          <div class="suggestion-favicon">
            <img
              src={favicon}
              alt=""
              class="suggestion-favicon-img"
              loading="lazy"
              onerror={handleFaviconError}
            />
          </div>
        {/if}

        <!-- URL text with gradient fade -->
        <div class="suggestion-text">
          <!-- Domain -->
          <span class="suggestion-domain">
            {domain}
          </span>

          <!-- Separator -->
          <span class="suggestion-separator">
            {' | '}
          </span>

          <!-- Path -->
          <span class="suggestion-path">
            {path || '/'}
          </span>
        </div>
      </div>
    {/each}
    {/if}
  </div>
{/if}



<style>
  .suggestion-list {
    margin-top: 0.5rem;
    max-height: 50vh;
    overflow-y: auto;
    border-top: 1px solid var(--dynamic-border);
    background: var(--dynamic-bg);
    border-radius: 0 0 2px 2px;
  }

  .view-mode-header {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    padding: 0.5rem 0.85rem;
    border-bottom: 1px solid var(--dynamic-border);
    background: color-mix(in srgb, var(--dynamic-bg) 95%, var(--dynamic-text));
    font-family: 'Fira Sans', system-ui, -apple-system, 'Segoe UI', 'Ubuntu', 'Cantarell', sans-serif;
  }

  .view-tab {
    padding: 0.25rem 0.75rem;
    border-radius: 2px;
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--dynamic-muted);
    transition: all 100ms ease;
  }

  .view-tab.active {
    color: var(--dynamic-accent);
    background: color-mix(in srgb, var(--dynamic-accent) 15%, transparent);
  }

  .view-hint {
    margin-left: auto;
    font-size: 0.75rem;
    color: var(--dynamic-muted);
    opacity: 0.7;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 2rem 1rem;
    gap: 0.5rem;
    color: var(--dynamic-muted);
    text-align: center;
  }

  .empty-icon {
    font-size: 2rem;
    opacity: 0.5;
  }

  .empty-text {
    font-size: 0.95rem;
    font-weight: 500;
    color: var(--dynamic-text);
    opacity: 0.7;
  }

  .empty-hint {
    font-size: 0.8rem;
    opacity: 0.6;
    max-width: 300px;
  }

  .suggestion-item {
    display: flex;
    gap: 0.75rem;
    align-items: center;
    padding: 0.85rem 0.85rem;
    border-bottom: 1px solid color-mix(in srgb, var(--dynamic-border) 50%, transparent);
    cursor: pointer;
    transition: background-color 100ms ease, border-left-color 100ms ease;
    letter-spacing: normal;
    border-left: 3px solid transparent;
    position: relative;
    font-family: 'Fira Sans', system-ui, -apple-system, 'Segoe UI', 'Ubuntu', 'Cantarell', sans-serif;
  }

  .suggestion-item:last-child {
    border-bottom: none;
  }

  /* Favorited items get a permanent yellow left border */
  .suggestion-item.favorited {
    border-left-color: #f59e0b;
  }

  .suggestion-item.selected,
  .suggestion-item:hover,
  .suggestion-item:focus-visible {
    background: color-mix(in srgb, var(--dynamic-accent) 15%, var(--dynamic-bg));
    border-left-color: var(--dynamic-accent);
    color: var(--dynamic-text);
    outline: none;
  }

  /* Keep yellow border even when selected/hover for favorited items */
  .suggestion-item.favorited.selected,
  .suggestion-item.favorited:hover,
  .suggestion-item.favorited:focus-visible {
    border-left-color: #f59e0b;
  }

  .suggestion-item.selected {
    font-weight: 500;
  }

  .suggestion-item .suggestion-domain {
    color: var(--dynamic-text);
    opacity: 0.95;
  }

  .suggestion-item .suggestion-separator {
    color: var(--dynamic-muted);
  }

  .suggestion-item .suggestion-path {
    color: var(--dynamic-muted);
  }

  .suggestion-favicon {
    width: 24px;
    height: 24px;
    border: none;
    border-radius: 3px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    flex-shrink: 0;
    overflow: hidden;
  }

  .suggestion-favicon-img {
    width: 24px;
    height: 24px;
    object-fit: contain;
    image-rendering: auto;
  }

  .suggestion-text {
    flex: 1;
    min-width: 0;
    display: flex;
    gap: 0.75rem;
    align-items: center;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    mask-image: linear-gradient(90deg, black 85%, transparent 100%);
    -webkit-mask-image: linear-gradient(90deg, black 85%, transparent 100%);
  }
</style>
