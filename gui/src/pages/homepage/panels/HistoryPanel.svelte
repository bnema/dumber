<script lang="ts">
  import { homepageState } from '../state.svelte';
  import {
    fetchHistoryTimeline,
    deleteHistoryEntry,
    deleteHistoryByRange,
    clearAllHistory,
    fetchDomainStats,
    navigateTo
  } from '../messaging';
  import type { HistoryEntry, HistoryCleanupRange } from '../types';

  import { HistoryTimeline, HistorySearch, HistoryFilters, HistoryCleanup } from '../history';

  let cleanupOpen = $state(false);
  let selectedDomain = $state<string | null>(null);

  // Filter entries by domain if selected
  const displayedEntries = $derived(() => {
    if (!selectedDomain) {
      return homepageState.historySearchQuery
        ? homepageState.historySearchResults
        : homepageState.history;
    }

    const entries = homepageState.historySearchQuery
      ? homepageState.historySearchResults
      : homepageState.history;

    return entries.filter(entry => {
      try {
        return new URL(entry.url).hostname === selectedDomain;
      } catch {
        return false;
      }
    });
  });

  // Recompute timeline from filtered entries
  const displayedTimeline = $derived(() => {
    const entries = displayedEntries();
    if (entries.length === 0) return [];

    const groups = new Map<string, HistoryEntry[]>();
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);

    for (const entry of entries) {
      const date = new Date(entry.last_visited);
      const dateKey = date.toISOString().split('T')[0] ?? '';
      if (!groups.has(dateKey)) {
        groups.set(dateKey, []);
      }
      groups.get(dateKey)!.push(entry);
    }

    const result: { date: string; label: string; entries: HistoryEntry[] }[] = [];
    for (const [dateKey, items] of groups) {
      const date = new Date(dateKey);
      let label: string;

      if (date >= today) {
        label = 'Today';
      } else if (date >= yesterday) {
        label = 'Yesterday';
      } else {
        label = date.toLocaleDateString(undefined, {
          weekday: 'short',
          month: 'short',
          day: 'numeric',
        });
      }

      result.push({ date: dateKey, label, entries: items });
    }

    return result.sort((a, b) => b.date.localeCompare(a.date));
  });

  const handleSelectEntry = (entry: HistoryEntry) => {
    navigateTo(entry.url);
  };

  const handleDeleteEntry = async (entry: HistoryEntry) => {
    await deleteHistoryEntry(entry.id);
  };

  const handleLoadMore = async () => {
    if (homepageState.hasMoreHistory && !homepageState.historyLoading) {
      await fetchHistoryTimeline(50, homepageState.historyOffset);
    }
  };

  const handleCleanup = async (range: HistoryCleanupRange) => {
    if (range === 'all') {
      await clearAllHistory();
    } else {
      await deleteHistoryByRange(range);
    }
    cleanupOpen = false;
  };

  const handleSelectDomain = (domain: string | null) => {
    selectedDomain = domain;
    homepageState.setFocusedIndex(0);
  };

  // Fetch domain stats on mount
  $effect(() => {
    fetchDomainStats(10);
  });
</script>

<div class="history-panel">
  <div class="panel-toolbar">
    <HistorySearch placeholder="Search history (FTS)..." autofocus={true} />
  </div>

  <HistoryFilters
    domains={homepageState.domainStats}
    selectedDomain={selectedDomain}
    onSelectDomain={handleSelectDomain}
    onOpenCleanup={() => cleanupOpen = true}
  />

  <div class="panel-content">
    {#if homepageState.historyLoading && homepageState.history.length === 0}
      <div class="loading-state">
        <span class="loading-spinner"></span>
        <span class="loading-text">LOADING HISTORY...</span>
      </div>
    {:else}
      <HistoryTimeline
        groups={displayedTimeline()}
        onSelectEntry={handleSelectEntry}
        onDeleteEntry={handleDeleteEntry}
        onLoadMore={handleLoadMore}
      />
    {/if}
  </div>

  {#if cleanupOpen}
    <HistoryCleanup
      onCleanup={handleCleanup}
      onClose={() => cleanupOpen = false}
    />
  {/if}
</div>

<style>
  .history-panel {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.5rem 1rem;
  }

  .panel-toolbar {
    flex-shrink: 0;
  }

  .panel-content {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--dynamic-border);
    background: color-mix(in srgb, var(--dynamic-bg) 95%, var(--dynamic-surface) 5%);
  }

  .loading-state {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    color: var(--dynamic-muted);
  }

  .loading-spinner {
    width: 24px;
    height: 24px;
    border: 2px solid var(--dynamic-border);
    border-top-color: var(--dynamic-accent, #4ade80);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .loading-text {
    font-size: 0.68rem;
    letter-spacing: 0.12em;
    animation: pulse 1.5s ease-in-out infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 0.5; }
    50% { opacity: 1; }
  }
</style>
