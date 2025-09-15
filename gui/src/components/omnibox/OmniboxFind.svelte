<!--
  Omnibox Find Component

  Displays find results and match navigation
-->
<script lang="ts">
  import { omniboxStore } from './stores.svelte.ts';
  import { revealMatch } from './find';

  // Reactive state
  let matches = $derived(omniboxStore.matches);
  let selectedIndex = $derived(omniboxStore.selectedIndex);
  let hasContent = $state(false);

  // Update hasContent when matches change
  $effect(() => {
    hasContent = omniboxStore.mode === 'find' && matches.length > 0;
  });

  // Computed total count
  let totalMatches = $derived(matches.length);

  // Handle match item mouse enter
  function handleItemMouseEnter(index: number) {
    omniboxStore.setSelectedIndex(index);
    omniboxStore.setFaded(true);
    scrollToSelection();
  }

  // Handle match item click
  function handleItemClick(index: number) {
    omniboxStore.setSelectedIndex(index);
    revealMatch(index);
    omniboxStore.close();
  }

  // Scroll list to show selected item
  function scrollToSelection() {
    const selectedItem = document.getElementById(`find-item-${selectedIndex}`);
    if (selectedItem && selectedItem.scrollIntoView) {
      try {
        selectedItem.scrollIntoView({ block: 'nearest' });
      } catch {
        selectedItem.scrollIntoView();
      }
    }
  }

  // Watch for selection changes to scroll
  $effect(() => {
    if (selectedIndex >= 0) {
      scrollToSelection();
    }
  });
</script>

<div
  id="find-list"
  class="mt-2 max-h-[50vh] overflow-auto border-t border-[#333]"
  role="listbox"
  aria-label="Find results"
>
  <!-- Header with match count -->
  <div
    class="px-2.5 py-1.5 text-xs text-[#bbb]
           border-b border-[#2a2a2a]"
  >
    {#if totalMatches > 0}
      {totalMatches} match{totalMatches === 1 ? '' : 'es'}
    {:else}
      No matches
    {/if}
  </div>

  <!-- Match results -->
  {#if hasContent}
    {#each matches as match, index (index)}
      {@const isSelected = index === selectedIndex}

      <div
        id="find-item-{index}"
        class="px-2.5 py-2 cursor-pointer
               border-b border-[#2a2a2a] last:border-b-0
               {isSelected ? 'bg-[#0a0a0a]' : ''}"
        role="option"
        tabindex="-1"
        aria-selected={isSelected}
        onmouseenter={() => handleItemMouseEnter(index)}
        onclick={() => handleItemClick(index)}
      >
        <!-- Match context -->
        <div
          class="text-[#ddd] text-sm
                 whitespace-nowrap overflow-hidden text-ellipsis"
        >
          {match.context || ''}
        </div>
      </div>
    {/each}
  {/if}
</div>

