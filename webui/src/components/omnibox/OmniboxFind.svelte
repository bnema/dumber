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

<div id="find-list" class="find-list" role="listbox" aria-label="Find results">
  <!-- Header with match count -->
  <div class="find-header">
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
        class={isSelected ? 'find-item selected' : 'find-item'}
        role="option"
        tabindex="-1"
        aria-selected={isSelected}
        onmouseenter={() => handleItemMouseEnter(index)}
        onclick={() => handleItemClick(index)}
        onkeydown={(e) => e.key === 'Enter' && handleItemClick(index)}
      >
        <!-- Match context -->
        <div class="find-context">
          {match.context || ''}
        </div>
      </div>
    {/each}
  {/if}
</div>

<style>
  .find-list {
    margin-top: 0.75rem;
    max-height: 50vh;
    overflow-y: auto;
    border-top: 1px solid var(--dynamic-border);
    background: color-mix(in srgb, var(--dynamic-bg) 94%, var(--dynamic-surface) 6%);
  }

  .find-header {
    padding: 0.65rem 0.85rem;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--dynamic-muted);
    border-bottom: 1px dashed var(--dynamic-border);
  }

  .find-item {
    padding: 0.65rem 0.85rem;
    border-bottom: 1px dashed var(--dynamic-border);
    cursor: pointer;
    transition: background-color 120ms ease, color 120ms ease;
  }

  .find-item:last-child {
    border-bottom: none;
  }

  .find-item.selected,
  .find-item:hover,
  .find-item:focus-visible {
    background: color-mix(in srgb, var(--dynamic-bg) 75%, var(--dynamic-surface) 25%);
    color: var(--dynamic-text);
    outline: none;
  }

  .find-context {
    color: var(--dynamic-text);
    font-size: 0.75rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
