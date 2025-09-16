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
    omniboxStore.setFaded(true);
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
    class="mt-2 max-h-[50vh] overflow-auto border-t border-[#333]"
    role="listbox"
    aria-label="Search suggestions"
  >
    {#each suggestions as suggestion, index (suggestion.url)}
      {@const { domain, path } = parseUrl(suggestion.url)}
      {@const isSelected = index === selectedIndex}

      <div
        id="omnibox-item-{index}"
        class="px-2.5 py-2 flex gap-2.5 items-center cursor-pointer
               border-b border-[#2a2a2a] last:border-b-0
               {isSelected ? 'bg-[#0a0a0a]' : ''}"
        role="option"
        tabindex={isSelected ? 0 : -1}
        aria-selected={isSelected}
        onmouseenter={() => handleItemMouseEnter(index)}
        onclick={() => handleItemClick(suggestion)}
        onkeydown={(e) => handleItemKeyDown(e, suggestion)}
      >
        <!-- Favicon chip -->
        {#if suggestion.favicon}
          <div
            class="flex-shrink-0 w-5 h-5 rounded-full
                   bg-[#ccc] border border-black/12
                   shadow-sm flex items-center justify-center"
          >
            <img
              src={suggestion.favicon}
              alt=""
              class="w-4 h-4 filter brightness-105 contrast-103"
              loading="lazy"
              onerror={handleFaviconError}
              style="image-rendering: -webkit-optimize-contrast;"
            />
          </div>
        {/if}

        <!-- URL text with gradient fade -->
        <div
          class="flex-1 min-w-0 flex gap-2 items-center
                 whitespace-nowrap overflow-hidden
                 [mask-image:linear-gradient(90deg,black_85%,transparent_100%)]
                 [-webkit-mask-image:linear-gradient(90deg,black_85%,transparent_100%)]"
        >
          <!-- Domain -->
          <span class="text-[#e6e6e6] opacity-95">
            {domain}
          </span>

          <!-- Separator -->
          <span class="text-[#777]">
            {' | '}
          </span>

          <!-- Path -->
          <span class="text-[#99aadd]">
            {path || '/'}
          </span>
        </div>
      </div>
    {/each}
  </div>
{/if}

