<script lang="ts">
  import type { Tag } from '../types';

  interface Props {
    tags: Tag[];
    selectedTagIds?: number[];
    onToggleTag?: (id: number) => void;
    onCreateTag?: (name: string, color: string) => Promise<void>;
    onClearSelection?: () => void;
  }

  let {
    tags,
    selectedTagIds = [],
    onToggleTag,
    onCreateTag,
    onClearSelection
  }: Props = $props();

  // Inline creation state
  let isCreating = $state(false);
  let newTagName = $state('');
  let inputRef = $state<HTMLInputElement | null>(null);

  // Predefined tag colors
  const tagColors = [
    '#6b7280', '#ef4444', '#f59e0b', '#22c55e',
    '#3b82f6', '#8b5cf6', '#ec4899', '#14b8a6'
  ];

  const getRandomColor = () => tagColors[Math.floor(Math.random() * tagColors.length)] ?? '#6b7280';

  const startCreating = () => {
    isCreating = true;
    newTagName = '';
    setTimeout(() => inputRef?.focus(), 0);
  };

  const cancelCreating = () => {
    isCreating = false;
    newTagName = '';
  };

  const submitCreate = async () => {
    const name = newTagName.trim();
    if (!name || !onCreateTag) {
      cancelCreating();
      return;
    }
    try {
      await onCreateTag(name, getRandomColor());
      cancelCreating();
    } catch (e) {
      console.error('Failed to create tag:', e);
    }
  };

  const handleKeydown = (e: KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      submitCreate();
    } else if (e.key === 'Escape') {
      cancelCreating();
    }
  };

  const isSelected = (id: number) => selectedTagIds.includes(id);
</script>

<div class="tag-cloud">
  <div class="tag-header">
    <span class="header-label">TAGS</span>
    <div class="header-actions">
      {#if selectedTagIds.length > 0}
        <button
          class="clear-btn"
          type="button"
          onclick={() => onClearSelection?.()}
          title="Clear selection"
        >
          CLEAR
        </button>
      {/if}
      {#if isCreating}
        <button
          class="add-tag-btn cancel"
          type="button"
          onclick={cancelCreating}
          title="Cancel"
        >
          ×
        </button>
      {:else}
        <button
          class="add-tag-btn"
          type="button"
          onclick={startCreating}
          title="Create tag"
        >
          +
        </button>
      {/if}
    </div>
  </div>

  {#if isCreating}
    <div class="create-input-row">
      <input
        bind:this={inputRef}
        bind:value={newTagName}
        onkeydown={handleKeydown}
        onblur={cancelCreating}
        class="create-input"
        type="text"
        placeholder="Tag name..."
        maxlength="30"
      />
    </div>
  {/if}

  <div class="tag-chips">
    {#if tags.length === 0}
      <span class="no-tags">No tags yet</span>
    {:else}
      {#each tags as tag (tag.id)}
        <button
          class="tag-chip"
          class:selected={isSelected(tag.id)}
          type="button"
          onclick={() => onToggleTag?.(tag.id)}
          style="--tag-color: {tag.color}"
        >
          <span class="tag-dot"></span>
          <span class="tag-name">{tag.name}</span>
        </button>
      {/each}
    {/if}
  </div>
</div>

<style>
  .tag-cloud {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.6rem 0.75rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: color-mix(in srgb, var(--background) 92%, var(--card) 8%);
  }

  .tag-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .header-label {
    font-size: 0.6rem;
    font-weight: 600;
    color: var(--muted-foreground);
    letter-spacing: 0.12em;
  }

  .header-actions {
    display: flex;
    gap: 0.35rem;
  }

  .clear-btn {
    padding: 0.2rem 0.4rem;
    font-size: 0.55rem;
    font-weight: 500;
    letter-spacing: 0.08em;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: transparent;
    color: var(--muted-foreground);
    cursor: pointer;
    transition: all 100ms ease;
  }

  .clear-btn:hover {
    color: var(--foreground);
    border-color: var(--foreground);
  }

  .add-tag-btn {
    width: 20px;
    height: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.9rem;
    font-weight: 500;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: transparent;
    color: var(--muted-foreground);
    cursor: pointer;
    transition: all 100ms ease;
  }

  .add-tag-btn:hover {
    color: var(--foreground);
    border-color: var(--foreground);
  }

  .add-tag-btn.cancel {
    color: var(--muted-foreground);
  }

  .add-tag-btn.cancel:hover {
    color: #ef4444;
    border-color: #ef4444;
  }

  .create-input-row {
    margin-bottom: 0.35rem;
  }

  .create-input {
    width: 100%;
    padding: 0.35rem 0.5rem;
    font-size: 0.68rem;
    font-family: inherit;
    color: var(--foreground);
    background: var(--background);
    border-width: 1px;
    border-style: solid;
    border-color: var(--primary, #4ade80);
    outline: none;
  }

  .create-input::placeholder {
    color: var(--muted-foreground);
  }

  .tag-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .no-tags {
    font-size: 0.65rem;
    color: var(--muted-foreground);
    font-style: italic;
    padding: 0.25rem 0;
  }

  .tag-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.25rem 0.5rem;
    font-size: 0.62rem;
    font-weight: 500;
    letter-spacing: 0.05em;
    border-width: 1px;
    border-style: solid;
    border-color: var(--tag-color);
    background: transparent;
    color: var(--foreground);
    cursor: pointer;
    transition: all 100ms ease;
  }

  .tag-chip:hover {
    background: color-mix(in srgb, var(--tag-color) 15%, transparent);
  }

  .tag-chip.selected {
    background: color-mix(in srgb, var(--tag-color) 25%, transparent);
    border-width: 2px;
    padding: calc(0.25rem - 1px) calc(0.5rem - 1px);
  }

  .tag-dot {
    width: 6px;
    height: 6px;
    background: var(--tag-color);
    border-radius: 50%;
  }

  .tag-name {
    text-transform: lowercase;
  }
</style>
