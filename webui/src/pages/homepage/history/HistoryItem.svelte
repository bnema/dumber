<script lang="ts">
  import type { HistoryEntry } from '../types';

  interface Props {
    entry: HistoryEntry;
    focused?: boolean;
    onSelect?: (entry: HistoryEntry) => void;
    onDelete?: (entry: HistoryEntry) => void;
  }

  let { entry, focused = false, onSelect, onDelete }: Props = $props();

  let deleting = $state(false);

  const getDomain = (url: string): string => {
    try {
      return new URL(url).hostname;
    } catch {
      return url;
    }
  };

  const getFaviconUrl = (item: HistoryEntry): string => {
    // Use provided favicon_url if available
    if (item.favicon_url) return item.favicon_url;
    // Construct DuckDuckGo favicon URL from domain
    const domain = getDomain(item.url).replace(/^www\./, '');
    return `https://icons.duckduckgo.com/ip3/${encodeURIComponent(domain)}.ico`;
  };

  const getDisplayTitle = (item: HistoryEntry): string => {
    const trimmed = item.title?.trim();
    if (trimmed) return trimmed;
    return getDomain(item.url) || item.url;
  };

  const formatTime = (timestamp: string): string => {
    try {
      const date = new Date(timestamp);
      const now = new Date();
      const diff = now.getTime() - date.getTime();

      const minutes = Math.floor(diff / (1000 * 60));
      const hours = Math.floor(diff / (1000 * 60 * 60));

      if (minutes < 1) return 'now';
      if (minutes < 60) return `${minutes}m`;
      if (hours < 24) return `${hours}h`;

      return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
    } catch {
      return '';
    }
  };

  const handleSelect = () => {
    onSelect?.(entry);
  };

  const handleDelete = (e: Event) => {
    e.stopPropagation();
    deleting = true;
    onDelete?.(entry);
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleSelect();
    } else if (e.key === 'd' || e.key === 'x') {
      handleDelete(e);
    }
  };
</script>

<div
  class="history-item"
  class:focused
  class:deleting
  role="button"
  tabindex="0"
  data-entry-id={entry.id}
  onclick={handleSelect}
  onkeydown={handleKeyDown}
>
  <div class="item-favicon">
    <img
      src={getFaviconUrl(entry)}
      alt=""
      class="favicon-img"
      onerror={(e) => {
        const target = e.target as HTMLImageElement;
        target.style.display = 'none';
        const fallback = target.nextElementSibling as HTMLElement;
        if (fallback) fallback.style.display = 'flex';
      }}
    />
    <div class="favicon-fallback" style="display: none;"></div>
  </div>

  <div class="item-content">
    <div class="item-row">
      <span class="item-title">{getDisplayTitle(entry)}</span>
      <span class="item-time">{formatTime(entry.last_visited)}</span>
    </div>
    <div class="item-url">{entry.url}</div>
  </div>

  <div class="item-actions">
    {#if entry.visit_count > 1}
      <span class="visit-count">{entry.visit_count}x</span>
    {/if}
    <button
      class="action-btn delete-btn"
      type="button"
      onclick={handleDelete}
      title="Delete entry (d)"
    >
      <span aria-hidden="true">×</span>
    </button>
  </div>
</div>

<style>
  .history-item {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 0.75rem;
    padding: 0.55rem 0.85rem;
    border-bottom-width: 1px;
    border-bottom-style: solid;
    border-bottom-color: color-mix(in srgb, var(--border) 50%, transparent);
    transition: background-color 100ms ease;
    cursor: pointer;
  }

  .history-item:last-child {
    border-bottom: none;
  }

  .history-item:hover,
  .history-item.focused {
    background: color-mix(in srgb, var(--card) 40%, transparent);
  }

  .history-item.focused {
    outline: 1px solid var(--primary, #4ade80);
    outline-offset: -1px;
  }

  .history-item.deleting {
    opacity: 0.3;
    pointer-events: none;
  }

  .item-favicon {
    width: 28px;
    height: 28px;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: var(--background);
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .favicon-img {
    width: 16px;
    height: 16px;
    object-fit: contain;
  }

  .favicon-fallback {
    width: 16px;
    height: 16px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--muted-foreground);
  }

  .favicon-fallback::before {
    content: '';
    font-size: 0.75rem;
  }

  .item-content {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
  }

  .item-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .item-title {
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--foreground);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .item-time {
    font-size: 0.65rem;
    color: var(--muted-foreground);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .item-url {
    font-size: 0.68rem;
    color: var(--muted-foreground);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .item-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .visit-count {
    font-size: 0.6rem;
    color: var(--muted-foreground);
    padding: 0.15rem 0.35rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: var(--background);
  }

  .action-btn {
    width: 24px;
    height: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: transparent;
    color: var(--muted-foreground);
    cursor: pointer;
    transition: all 100ms ease;
    opacity: 0;
  }

  .history-item:hover .action-btn,
  .history-item.focused .action-btn {
    opacity: 1;
  }

  .action-btn:hover {
    color: var(--foreground);
    border-color: var(--foreground);
    background: color-mix(in srgb, var(--card) 50%, transparent);
  }

  .delete-btn:hover {
    color: #ef4444;
    border-color: #ef4444;
  }
</style>
