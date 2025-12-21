<script lang="ts">
  import type { DomainStat } from '../types';

  interface Props {
    domains?: DomainStat[];
    selectedDomain?: string | null;
    onSelectDomain?: (domain: string | null) => void;
    onOpenCleanup?: () => void;
  }

  let {
    domains = [],
    selectedDomain = null,
    onSelectDomain,
    onOpenCleanup
  }: Props = $props();

  const topDomains = $derived(domains.slice(0, 5));
</script>

<div class="history-filters">
  <div class="filter-section">
    <span class="filter-label">FILTER</span>
    <div class="filter-chips">
      <button
        class="filter-chip"
        class:active={selectedDomain === null}
        type="button"
        onclick={() => onSelectDomain?.(null)}
      >
        ALL
      </button>
      {#each topDomains as domain (domain.domain)}
        <button
          class="filter-chip"
          class:active={selectedDomain === domain.domain}
          type="button"
          onclick={() => onSelectDomain?.(domain.domain)}
          title="{domain.page_count} pages, {domain.total_visits} visits"
        >
          {domain.domain.replace('www.', '').slice(0, 12)}
        </button>
      {/each}
    </div>
  </div>

  <div class="filter-actions">
    <button
      class="cleanup-btn"
      type="button"
      onclick={() => onOpenCleanup?.()}
      title="Clear history (D)"
    >
      <span class="cleanup-icon"></span>
      <span>CLEAR</span>
    </button>
  </div>
</div>

<style>
  .history-filters {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.5rem 1rem;
  }

  .filter-section {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex: 1;
    min-width: 0;
  }

  .filter-label {
    font-size: 0.6rem;
    font-weight: 600;
    color: var(--muted-foreground);
    letter-spacing: 0.12em;
    flex-shrink: 0;
  }

  .filter-chips {
    display: flex;
    gap: 0.35rem;
    overflow-x: auto;
    scrollbar-width: none;
  }

  .filter-chips::-webkit-scrollbar {
    display: none;
  }

  .filter-chip {
    padding: 0.3rem 0.55rem;
    font-size: 0.6rem;
    font-weight: 500;
    letter-spacing: 0.08em;
    color: var(--muted-foreground);
    background: transparent;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    cursor: pointer;
    transition: all 100ms ease;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .filter-chip:hover {
    color: var(--foreground);
    border-color: var(--foreground);
  }

  .filter-chip.active {
    color: var(--primary-foreground, var(--background));
    background: var(--primary);
    border-color: var(--primary);
  }

  .filter-actions {
    display: flex;
    gap: 0.35rem;
    flex-shrink: 0;
  }

  .cleanup-btn {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.35rem 0.6rem;
    font-size: 0.6rem;
    font-weight: 500;
    letter-spacing: 0.08em;
    color: var(--muted-foreground);
    background: transparent;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    cursor: pointer;
    transition: all 100ms ease;
  }

  .cleanup-btn:hover {
    color: var(--destructive);
    border-color: var(--destructive);
  }

  .cleanup-icon::before {
    content: '';
    font-size: 0.75rem;
  }
</style>
