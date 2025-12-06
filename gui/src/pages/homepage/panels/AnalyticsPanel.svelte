<script lang="ts">
  import { homepageState } from '../state.svelte';
  import { fetchAnalytics } from '../messaging';

  // Fetch analytics on mount
  $effect(() => {
    if (!homepageState.analytics) {
      fetchAnalytics();
    }
  });

  const formatNumber = (n: number): string => {
    if (n >= 1000) {
      return `${(n / 1000).toFixed(1)}k`;
    }
    return n.toString();
  };
</script>

<div class="analytics-panel">
  {#if homepageState.analyticsLoading}
    <div class="loading-state">
      <span class="loading-spinner"></span>
      <span class="loading-text">LOADING ANALYTICS...</span>
    </div>
  {:else if homepageState.analytics}
    <div class="stats-grid">
      <div class="stat-card">
        <span class="stat-value">{formatNumber(homepageState.analytics.stats.total_entries)}</span>
        <span class="stat-label">TOTAL ENTRIES</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">{formatNumber(homepageState.analytics.stats.total_visits)}</span>
        <span class="stat-label">TOTAL VISITS</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">{homepageState.analytics.stats.unique_days}</span>
        <span class="stat-label">DAYS ACTIVE</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">{homepageState.domainStats.length}</span>
        <span class="stat-label">UNIQUE DOMAINS</span>
      </div>
    </div>

    <div class="section">
      <div class="section-header">
        <span class="section-title">TOP DOMAINS</span>
      </div>
      <div class="domain-list">
        {#each homepageState.domainStats.slice(0, 10) as domain, i (domain.domain)}
          <div class="domain-item">
            <span class="domain-rank">{i + 1}</span>
            <span class="domain-name">{domain.domain}</span>
            <span class="domain-stats">
              <span class="domain-pages">{domain.page_count} pages</span>
              <span class="domain-visits">{domain.total_visits} visits</span>
            </span>
          </div>
        {/each}
      </div>
    </div>

    {#if homepageState.analytics.hourly_distribution.length > 0}
      <div class="section">
        <div class="section-header">
          <span class="section-title">ACTIVITY BY HOUR</span>
        </div>
        <div class="hourly-chart">
          {#each homepageState.analytics.hourly_distribution as hour (hour.hour)}
            {@const maxVisits = Math.max(...homepageState.analytics.hourly_distribution.map(h => h.visit_count))}
            {@const heightPercent = maxVisits > 0 ? (hour.visit_count / maxVisits) * 100 : 0}
            <div class="hour-bar" title="{hour.hour}:00 - {hour.visit_count} visits">
              <div class="bar-fill" style="height: {heightPercent}%"></div>
              <span class="hour-label">{hour.hour}</span>
            </div>
          {/each}
        </div>
      </div>
    {/if}
  {:else}
    <div class="empty-state">
      <span class="empty-icon"></span>
      <span class="empty-text">NO ANALYTICS DATA</span>
      <span class="empty-hint">Browse some sites to generate analytics</span>
    </div>
  {/if}
</div>

<style>
  .analytics-panel {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 0.5rem 1rem;
  }

  .loading-state,
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

  .loading-text,
  .empty-text {
    font-size: 0.68rem;
    font-weight: 600;
    letter-spacing: 0.12em;
  }

  .empty-icon {
    font-size: 1.5rem;
    opacity: 0.5;
  }

  .empty-hint {
    font-size: 0.65rem;
    letter-spacing: 0.06em;
    opacity: 0.7;
  }

  .stats-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: 0.5rem;
  }

  .stat-card {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    padding: 0.85rem;
    border: 1px solid var(--dynamic-border);
    background: color-mix(in srgb, var(--dynamic-bg) 92%, var(--dynamic-surface) 8%);
  }

  .stat-value {
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--dynamic-text);
    letter-spacing: -0.02em;
  }

  .stat-label {
    font-size: 0.6rem;
    font-weight: 600;
    color: var(--dynamic-muted);
    letter-spacing: 0.12em;
  }

  .section {
    border: 1px solid var(--dynamic-border);
    background: color-mix(in srgb, var(--dynamic-bg) 92%, var(--dynamic-surface) 8%);
  }

  .section-header {
    padding: 0.5rem 0.75rem;
    border-bottom: 1px solid var(--dynamic-border);
    background: color-mix(in srgb, var(--dynamic-bg) 80%, var(--dynamic-surface) 20%);
  }

  .section-title {
    font-size: 0.6rem;
    font-weight: 600;
    color: var(--dynamic-muted);
    letter-spacing: 0.12em;
  }

  .domain-list {
    display: flex;
    flex-direction: column;
  }

  .domain-item {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 0.75rem;
    border-bottom: 1px solid color-mix(in srgb, var(--dynamic-border) 40%, transparent);
  }

  .domain-item:last-child {
    border-bottom: none;
  }

  .domain-rank {
    width: 1.5rem;
    font-size: 0.65rem;
    font-weight: 600;
    color: var(--dynamic-muted);
    text-align: center;
  }

  .domain-name {
    font-size: 0.75rem;
    color: var(--dynamic-text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .domain-stats {
    display: flex;
    gap: 0.75rem;
    font-size: 0.6rem;
    color: var(--dynamic-muted);
  }

  .hourly-chart {
    display: flex;
    align-items: flex-end;
    gap: 2px;
    height: 80px;
    padding: 0.75rem;
  }

  .hour-bar {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    height: 100%;
  }

  .bar-fill {
    width: 100%;
    background: var(--dynamic-accent, #4ade80);
    margin-top: auto;
    min-height: 2px;
    transition: height 300ms ease;
  }

  .hour-label {
    font-size: 0.5rem;
    color: var(--dynamic-muted);
    margin-top: 0.25rem;
  }

  .hour-bar:nth-child(odd) .hour-label {
    visibility: hidden;
  }
</style>
