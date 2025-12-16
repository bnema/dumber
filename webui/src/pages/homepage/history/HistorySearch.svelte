<script lang="ts">
  import { onMount } from 'svelte';
  import { homepageState } from '../state.svelte';
  import { handleSearchInput } from '../keyboard';

  interface Props {
    placeholder?: string;
    autofocus?: boolean;
  }

  let { placeholder = 'Search history...', autofocus = false }: Props = $props();

  let inputRef = $state<HTMLInputElement | null>(null);

  // Auto-focus on mount if requested
  onMount(() => {
    if (autofocus && inputRef) {
      inputRef.focus();
    }
  });

  const handleInput = (e: Event) => {
    const target = e.target as HTMLInputElement;
    handleSearchInput(target.value);
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    switch (e.key) {
      case 'Escape':
        e.preventDefault();
        e.stopPropagation();
        if (homepageState.historySearchQuery) {
          // First Esc: clear search
          homepageState.setHistorySearchQuery('');
        } else {
          // Second Esc (or if no query): blur input
          inputRef?.blur();
        }
        break;
      case 'ArrowDown':
      case 'j':
        if (e.key === 'j' && !e.ctrlKey) break; // Only Ctrl+j or ArrowDown
        e.preventDefault();
        homepageState.focusNext();
        inputRef?.blur();
        break;
      case 'ArrowUp':
      case 'k':
        if (e.key === 'k' && !e.ctrlKey) break; // Only Ctrl+k or ArrowUp
        e.preventDefault();
        // Don't move focus - already at top
        break;
    }
  };

  // Focus input when focusedIndex becomes -1 (user pressed up from first item)
  $effect(() => {
    if (homepageState.focusedIndex === -1 && homepageState.activePanel === 'history') {
      inputRef?.focus();
    }
  });

  const clearSearch = () => {
    homepageState.setHistorySearchQuery('');
    inputRef?.focus();
  };
</script>

<div class="history-search">
  <div class="search-icon"></div>
  <input
    bind:this={inputRef}
    type="text"
    class="search-input"
    {placeholder}
    value={homepageState.historySearchQuery}
    oninput={handleInput}
    onkeydown={handleKeyDown}
  />
  {#if homepageState.historySearching}
    <div class="search-spinner"></div>
  {:else if homepageState.historySearchQuery}
    <button
      class="clear-btn"
      type="button"
      onclick={clearSearch}
      title="Clear search (Esc)"
    >
      ×
    </button>
  {:else}
    <kbd class="search-hint">/</kbd>
  {/if}
</div>

<style>
  .history-search {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.5rem 0.75rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: var(--background);
    transition: border-color 150ms ease;
  }

  .history-search:focus-within {
    border-color: var(--primary, #4ade80);
  }

  .search-icon::before {
    content: '';
    font-size: 0.85rem;
    color: var(--muted-foreground);
  }

  .search-input {
    flex: 1;
    border: none;
    background: transparent;
    color: var(--foreground);
    font-family: inherit;
    font-size: 0.8rem;
    outline: none;
  }

  .search-input::placeholder {
    color: var(--muted-foreground);
    letter-spacing: 0.05em;
  }

  .search-spinner {
    width: 14px;
    height: 14px;
    border-width: 2px;
    border-style: solid;
    border-color: var(--border);
    border-top-color: var(--primary, #4ade80);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .clear-btn {
    width: 20px;
    height: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: transparent;
    color: var(--muted-foreground);
    font-size: 0.9rem;
    cursor: pointer;
    transition: all 100ms ease;
  }

  .clear-btn:hover {
    color: var(--foreground);
    border-color: var(--foreground);
  }

  .search-hint {
    font-size: 0.6rem;
    padding: 0.15rem 0.35rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: color-mix(in srgb, var(--card) 30%, transparent);
    color: var(--muted-foreground);
  }
</style>
