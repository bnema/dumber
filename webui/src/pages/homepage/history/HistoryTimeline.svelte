<script lang="ts">
  import type { TimelineGroup, HistoryEntry } from '../types';
  import { homepageState } from '../state.svelte';
  import HistoryItem from './HistoryItem.svelte';

  interface Props {
    groups: TimelineGroup[];
    onSelectEntry?: (entry: HistoryEntry) => void;
    onDeleteEntry?: (entry: HistoryEntry) => void;
    onLoadMore?: () => void;
  }

  let { groups, onSelectEntry, onDeleteEntry, onLoadMore }: Props = $props();

  let containerRef = $state<HTMLElement | null>(null);
  let sentinelRef = $state<HTMLElement | null>(null);

  // Flatten groups to get global index for keyboard navigation
  const flatEntries = $derived(() => {
    const entries: HistoryEntry[] = [];
    for (const group of groups) {
      entries.push(...group.entries);
    }
    return entries;
  });

  // Track which entry is focused based on global focusedIndex
  const getFocusedEntry = () => {
    const entries = flatEntries();
    return entries[homepageState.focusedIndex] ?? null;
  };

  // Track previous focused index to only scroll on actual navigation
  let prevFocusedIndex = $state(-1);

  // Scroll focused item into view when focusedIndex changes via keyboard navigation
  $effect(() => {
    // Skip if panel not active or command palette is open
    if (!containerRef || homepageState.activePanel !== 'history') return;
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

    const focusedEntry = getFocusedEntry();
    if (!focusedEntry) return;

    // Find the focused element by entry ID
    const el = containerRef.querySelector(`[data-entry-id="${focusedEntry.id}"]`);
    if (el) {
      el.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
  });

  // Setup infinite scroll observer
  $effect(() => {
    if (!sentinelRef || !onLoadMore) return;

    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (entry?.isIntersecting && homepageState.hasMoreHistory && !homepageState.historyLoading) {
          onLoadMore();
        }
      },
      { threshold: 0.1, rootMargin: '100px' }
    );

    observer.observe(sentinelRef);

    return () => observer.disconnect();
  });
</script>

<div class="history-timeline" bind:this={containerRef}>
  {#if groups.length === 0}
    <div class="empty-state">
      <span class="empty-icon"></span>
      <span class="empty-text">NO HISTORY RECORDED</span>
      <span class="empty-hint">Start browsing to see your history here</span>
    </div>
  {:else}
    {#each groups as group (group.date)}
      <div class="timeline-group">
        <div class="group-header">
          <span class="group-label">{group.label}</span>
          <span class="group-count">{group.entries.length}</span>
        </div>
        <div class="group-entries">
          {#each group.entries as entry (entry.id)}
            <HistoryItem
              {entry}
              focused={getFocusedEntry()?.id === entry.id}
              onSelect={onSelectEntry}
              onDelete={onDeleteEntry}
            />
          {/each}
        </div>
      </div>
    {/each}

    {#if homepageState.hasMoreHistory}
      <div class="load-more-sentinel" bind:this={sentinelRef}>
        {#if homepageState.historyLoading}
          <span class="loading-text">LOADING...</span>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .history-timeline {
    display: flex;
    flex-direction: column;
  }

  .empty-state {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 2rem;
    color: var(--muted-foreground);
    text-align: center;
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
    font-size: 0.7rem;
    letter-spacing: 0.08em;
    opacity: 0.7;
  }

  .timeline-group {
    border-bottom-width: 1px;
    border-bottom-style: solid;
    border-bottom-color: var(--border);
  }

  .timeline-group:last-child {
    border-bottom: none;
  }

  .group-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding: 0.5rem 0.85rem;
    background: color-mix(in srgb, var(--background) 80%, var(--card) 20%);
    border-bottom-width: 1px;
    border-bottom-style: solid;
    border-bottom-color: color-mix(in srgb, var(--border) 50%, transparent);
    position: sticky;
    top: 0;
    z-index: 1;
  }

  .group-label {
    font-size: 0.68rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    color: var(--foreground);
  }

  .group-count {
    font-size: 0.6rem;
    padding: 0.15rem 0.4rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: var(--background);
    color: var(--muted-foreground);
  }

  .group-entries {
    display: flex;
    flex-direction: column;
  }

  .load-more-sentinel {
    padding: 0.75rem;
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 40px;
  }

  .loading-text {
    font-size: 0.65rem;
    color: var(--muted-foreground);
    letter-spacing: 0.12em;
    animation: pulse 1.5s ease-in-out infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 0.4; }
    50% { opacity: 1; }
  }
</style>
