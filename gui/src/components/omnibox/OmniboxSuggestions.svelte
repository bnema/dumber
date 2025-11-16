<!--
  Omnibox Suggestions Component

  Displays search suggestions with favicons and URL highlighting
-->
<script lang="ts">
  import { omniboxStore } from './stores.svelte.ts';
  import { omniboxBridge } from './messaging';

  // Reactive state
  let suggestions = $derived(omniboxStore.suggestions);
  let selectedIndex = $derived(omniboxStore.selectedIndex);
  let hasContent = $state(false);

  // Update hasContent when suggestions change
  $effect(() => {
    hasContent = omniboxStore.mode === 'omnibox' && suggestions.length > 0;
  });

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
    {#each suggestions as suggestion, index (suggestion.url)}
      {@const { domain, path } = parseUrl(suggestion.url)}
      {@const isSelected = index === selectedIndex}

      <div
        id="omnibox-item-{index}"
        class={isSelected ? 'suggestion-item selected' : 'suggestion-item'}
        role="option"
        tabindex={isSelected ? 0 : -1}
        aria-selected={isSelected}
        onmouseenter={() => handleItemMouseEnter(index)}
        onclick={() => handleItemClick(suggestion)}
        onkeydown={(e) => handleItemKeyDown(e, suggestion)}
      >
        <!-- Favicon chip -->
        {#if suggestion.favicon}
          <div class="suggestion-favicon">
            <img
              src={suggestion.favicon}
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

  .suggestion-item {
    display: flex;
    gap: 0.75rem;
    align-items: center;
    padding: 0.65rem 0.85rem;
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

  .suggestion-item.selected,
  .suggestion-item:hover,
  .suggestion-item:focus-visible {
    background: color-mix(in srgb, var(--dynamic-accent) 15%, var(--dynamic-bg));
    border-left-color: var(--dynamic-accent);
    color: var(--dynamic-text);
    outline: none;
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
