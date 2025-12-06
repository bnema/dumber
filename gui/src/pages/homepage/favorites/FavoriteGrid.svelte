<script lang="ts">
  import type { Favorite } from '../types';
  import { homepageState } from '../state.svelte';
  import FavoriteCard from './FavoriteCard.svelte';

  interface Props {
    favorites: Favorite[];
    onSelectFavorite?: (fav: Favorite) => void;
    onEditFavorite?: (fav: Favorite) => void;
  }

  let { favorites, onSelectFavorite, onEditFavorite }: Props = $props();

  let containerRef = $state<HTMLElement | null>(null);

  const getFocusedFavorite = () => {
    return favorites[homepageState.focusedIndex] ?? null;
  };

  // Track previous focused index to only scroll on actual navigation
  let prevFocusedIndex = $state(-1);

  // Scroll focused item into view when focusedIndex changes via keyboard navigation
  $effect(() => {
    // Skip if panel not active or command palette is open
    if (!containerRef || homepageState.activePanel !== 'favorites') return;
    if (homepageState.commandPaletteOpen) return;

    const currentIndex = homepageState.focusedIndex;

    // Only scroll if index actually changed (not on initial render or modal close)
    if (prevFocusedIndex === currentIndex) return;
    if (prevFocusedIndex === -1) {
      // First render - just track, don't scroll
      prevFocusedIndex = currentIndex;
      return;
    }

    prevFocusedIndex = currentIndex;

    const focusedFav = getFocusedFavorite();
    if (!focusedFav) return;

    // Find the focused element by favorite ID
    const el = containerRef.querySelector(`[data-favorite-id="${focusedFav.id}"]`);
    if (el) {
      el.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
  });
</script>

<div class="favorite-grid" bind:this={containerRef}>
  {#if favorites.length === 0}
    <div class="empty-state">
      <span class="empty-icon"></span>
      <span class="empty-text">NO FAVORITES</span>
      <span class="empty-hint">Pin sites from history to add them here</span>
    </div>
  {:else}
    {#each favorites as favorite (favorite.id)}
      <FavoriteCard
        {favorite}
        focused={getFocusedFavorite()?.id === favorite.id}
        onSelect={onSelectFavorite}
        onEdit={onEditFavorite}
      />
    {/each}
  {/if}
</div>

<style>
  .favorite-grid {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    flex: 1;
    min-height: 0;
    overflow-y: auto;
  }

  .empty-state {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 2rem;
    color: var(--dynamic-muted);
    text-align: center;
    border: 1px dashed var(--dynamic-border);
    background: color-mix(in srgb, var(--dynamic-bg) 95%, var(--dynamic-surface) 5%);
  }

  .empty-icon {
    font-size: 1.5rem;
    opacity: 0.5;
  }

  .empty-text {
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.12em;
  }

  .empty-hint {
    font-size: 0.68rem;
    letter-spacing: 0.06em;
    opacity: 0.7;
  }
</style>
