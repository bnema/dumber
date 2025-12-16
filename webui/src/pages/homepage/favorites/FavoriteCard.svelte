<script lang="ts">
  import type { Favorite } from '../types';

  interface Props {
    favorite: Favorite;
    focused?: boolean;
    onSelect?: (fav: Favorite) => void;
    onEdit?: (fav: Favorite) => void;
  }

  let { favorite, focused = false, onSelect, onEdit }: Props = $props();

  const getDomain = (url: string): string => {
    try {
      return new URL(url).hostname.replace('www.', '');
    } catch {
      return url;
    }
  };

  const getDisplayTitle = (fav: Favorite): string => {
    const trimmed = fav.title?.trim();
    if (trimmed) return trimmed;
    return getDomain(fav.url) || fav.url;
  };

  const handleSelect = () => {
    onSelect?.(favorite);
  };

  const handleEdit = (e: Event) => {
    e.stopPropagation();
    onEdit?.(favorite);
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    switch (e.key) {
      case 'Enter':
        handleSelect();
        break;
      case 'e':
        onEdit?.(favorite);
        break;
    }
  };
</script>

<div
  class="favorite-card"
  class:focused
  role="button"
  tabindex="0"
  data-favorite-id={favorite.id}
  onclick={handleSelect}
  onkeydown={handleKeyDown}
>
  <div class="card-favicon">
    {#if favorite.favicon_url}
      <img
        src={favorite.favicon_url}
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
    {:else}
      <div class="favicon-fallback"></div>
    {/if}
    {#if favorite.shortcut_key}
      <div class="shortcut-badge">{favorite.shortcut_key}</div>
    {/if}
  </div>

  <div class="card-content">
    <span class="card-title">{getDisplayTitle(favorite)}</span>
    <span class="card-domain">{getDomain(favorite.url)}</span>
  </div>

  {#if favorite.tags && favorite.tags.length > 0}
    <div class="card-tags">
      {#each favorite.tags.slice(0, 2) as tag (tag.id)}
        <span class="tag-chip" style="--tag-color: {tag.color}">
          {tag.name}
        </span>
      {/each}
      {#if favorite.tags.length > 2}
        <span class="tag-more">+{favorite.tags.length - 2}</span>
      {/if}
    </div>
  {/if}

  <div class="card-actions">
    <button
      class="action-btn"
      type="button"
      onclick={handleEdit}
      title="Edit (e)"
    >

    </button>
  </div>
</div>

<style>
  .favorite-card {
    display: grid;
    grid-template-columns: auto 1fr auto auto;
    align-items: center;
    gap: 0.75rem;
    padding: 0.65rem 0.85rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: color-mix(in srgb, var(--background) 92%, var(--card) 8%);
    transition: all 100ms ease;
    cursor: pointer;
  }

  .favorite-card:hover,
  .favorite-card.focused {
    background: color-mix(in srgb, var(--card) 40%, var(--background) 60%);
    border-color: color-mix(in srgb, var(--border) 50%, var(--foreground) 50%);
  }

  .favorite-card.focused {
    outline: 1px solid var(--primary, #4ade80);
    outline-offset: -1px;
  }

  .card-favicon {
    position: relative;
    width: 32px;
    height: 32px;
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
    width: 18px;
    height: 18px;
    object-fit: contain;
  }

  .favicon-fallback {
    width: 18px;
    height: 18px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--muted-foreground);
  }

  .favicon-fallback::before {
    content: '';
    font-size: 0.9rem;
  }

  .shortcut-badge {
    position: absolute;
    bottom: -4px;
    right: -4px;
    width: 14px;
    height: 14px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.55rem;
    font-weight: 700;
    color: var(--background);
    background: var(--primary, #4ade80);
    border-width: 1px;
    border-style: solid;
    border-color: var(--background);
  }

  .card-content {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
  }

  .card-title {
    font-size: 0.8rem;
    font-weight: 500;
    color: var(--foreground);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .card-domain {
    font-size: 0.65rem;
    color: var(--muted-foreground);
    letter-spacing: 0.06em;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .card-tags {
    display: flex;
    align-items: center;
    gap: 0.3rem;
  }

  .tag-chip {
    font-size: 0.55rem;
    font-weight: 500;
    padding: 0.15rem 0.4rem;
    background: color-mix(in srgb, var(--tag-color) 20%, transparent);
    border-width: 1px;
    border-style: solid;
    border-color: var(--tag-color);
    color: var(--tag-color);
    letter-spacing: 0.05em;
  }

  .tag-more {
    font-size: 0.55rem;
    color: var(--muted-foreground);
    padding: 0.15rem 0.3rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
  }

  .card-actions {
    display: flex;
    gap: 0.3rem;
    opacity: 0;
    transition: opacity 100ms ease;
  }

  .favorite-card:hover .card-actions,
  .favorite-card.focused .card-actions {
    opacity: 1;
  }

  .action-btn {
    width: 24px;
    height: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.8rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: transparent;
    color: var(--muted-foreground);
    cursor: pointer;
    transition: all 100ms ease;
  }

  .action-btn:hover {
    color: var(--foreground);
    border-color: var(--foreground);
    background: color-mix(in srgb, var(--card) 50%, transparent);
  }
</style>
