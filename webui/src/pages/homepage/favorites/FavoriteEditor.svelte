<script lang="ts">
  import type { Favorite, Folder, Tag } from '../types';

  interface Props {
    favorite: Favorite;
    folders: Folder[];
    tags: Tag[];
    onSave: (updates: Partial<Favorite>) => void;
    onDelete: (id: number) => void;
    onClose: () => void;
    onAssignTag: (favoriteId: number, tagId: number) => void;
    onRemoveTag: (favoriteId: number, tagId: number) => void;
  }

  const props: Props = $props();
  const { folders, tags, onSave, onDelete, onClose, onAssignTag, onRemoveTag } = props;
  // Capture initial values for form state (component remounts for different favorites)
  const initialFavorite = props.favorite;

  // Form state
  let title = $state(initialFavorite.title || '');
  let folderId = $state<number | null>(initialFavorite.folder_id);
  let shortcutKey = $state<number | null>(initialFavorite.shortcut_key);
  let confirmDelete = $state(false);

  // Track current favorite for reactive operations
  const favorite = $derived(props.favorite);

  // Get favorite's current tags
  const favoriteTags = $derived(favorite.tags || []);
  const hasTag = (tagId: number) => favoriteTags.some(t => t.id === tagId);

  const handleSave = () => {
    onSave({
      id: favorite.id,
      title: title.trim() || null,
      folder_id: folderId,
      shortcut_key: shortcutKey,
    });
    onClose();
  };

  const handleDelete = () => {
    if (!confirmDelete) {
      confirmDelete = true;
      return;
    }
    onDelete(favorite.id);
    onClose();
  };

  const handleTagClick = (tagId: number) => {
    if (hasTag(tagId)) {
      onRemoveTag(favorite.id, tagId);
    } else {
      onAssignTag(favorite.id, tagId);
    }
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    switch (e.key) {
      case 'Escape':
        e.preventDefault();
        if (confirmDelete) {
          confirmDelete = false;
        } else {
          onClose();
        }
        break;
      case 'Enter':
        if (e.ctrlKey || e.metaKey) {
          e.preventDefault();
          handleSave();
        }
        break;
    }
  };

  const handleOverlayClick = (e: Event) => {
    if (e.target === e.currentTarget) {
      onClose();
    }
  };
</script>

<svelte:window onkeydown={handleKeyDown} />

<div
  class="editor-overlay"
  onclick={handleOverlayClick}
  onkeydown={(e) => { if (e.key === 'Escape') handleOverlayClick(e); }}
  role="button"
  tabindex="0"
  aria-label="Close editor"
>
  <div class="editor-modal">
    <div class="modal-header">
      <span class="modal-icon"></span>
      <span class="modal-title">EDIT FAVORITE</span>
      <button class="close-btn" type="button" onclick={onClose}>
        <kbd>Esc</kbd>
      </button>
    </div>

    <div class="modal-body">
      <!-- URL (readonly) -->
      <div class="form-group">
        <span class="form-label" id="url-label">URL</span>
        <div class="url-display" aria-labelledby="url-label">{favorite.url}</div>
      </div>

      <!-- Title -->
      <div class="form-group">
        <label class="form-label" for="fav-title">TITLE</label>
        <input
          id="fav-title"
          type="text"
          class="form-input"
          bind:value={title}
          placeholder="Custom title (optional)"
        />
      </div>

      <!-- Folder -->
      <div class="form-group">
        <label class="form-label" for="fav-folder">FOLDER</label>
        <select id="fav-folder" class="form-select" bind:value={folderId}>
          <option value={null}>— No folder —</option>
          {#each folders as folder (folder.id)}
            <option value={folder.id}>
              {folder.icon || ''} {folder.name}
            </option>
          {/each}
        </select>
      </div>

      <!-- Shortcut Key -->
      <div class="form-group">
        <label class="form-label" for="fav-shortcut">QUICK ACCESS KEY</label>
        <select id="fav-shortcut" class="form-select" bind:value={shortcutKey}>
          <option value={null}>— None —</option>
          {#each [1, 2, 3, 4, 5, 6, 7, 8, 9] as key}
            <option value={key}>{key}</option>
          {/each}
        </select>
        <span class="form-hint">Press 1-9 on homepage to open directly</span>
      </div>

      <!-- Tags -->
      <div class="form-group">
        <span class="form-label" id="tags-label">TAGS</span>
        <div class="tag-selector" role="group" aria-labelledby="tags-label">
          {#if tags.length === 0}
            <span class="no-tags">No tags available</span>
          {:else}
            {#each tags as tag (tag.id)}
              <button
                class="tag-chip"
                class:selected={hasTag(tag.id)}
                type="button"
                onclick={() => handleTagClick(tag.id)}
                style="--tag-color: {tag.color}"
              >
                <span class="tag-dot"></span>
                <span class="tag-name">{tag.name}</span>
              </button>
            {/each}
          {/if}
        </div>
      </div>
    </div>

    <div class="modal-footer">
      {#if confirmDelete}
        <div class="confirm-delete">
          <span class="confirm-text">Delete this favorite?</span>
          <button class="btn btn-cancel" type="button" onclick={() => confirmDelete = false}>
            CANCEL
          </button>
          <button class="btn btn-danger" type="button" onclick={handleDelete}>
            DELETE
          </button>
        </div>
      {:else}
        <button class="btn btn-danger-outline" type="button" onclick={handleDelete}>
           DELETE
        </button>
        <div class="footer-right">
          <button class="btn btn-secondary" type="button" onclick={onClose}>
            CANCEL
          </button>
          <button class="btn btn-primary" type="button" onclick={handleSave}>
            SAVE
            <kbd>Ctrl+Enter</kbd>
          </button>
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .editor-overlay {
    position: fixed;
    inset: 0;
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgb(0 0 0 / 0.75);
    backdrop-filter: blur(4px);
    animation: fade-in 150ms ease;
  }

  @keyframes fade-in {
    from { opacity: 0; }
  }

  .editor-modal {
    width: 100%;
    max-width: 480px;
    margin: 1rem;
    background: var(--card);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    box-shadow: 0 24px 48px -12px rgb(0 0 0 / 0.6);
    animation: modal-in 150ms ease;
  }

  @keyframes modal-in {
    from {
      opacity: 0;
      transform: scale(0.96) translateY(-8px);
    }
  }

  .modal-header {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 80%, transparent);
  }

  .modal-icon::before {
    content: '';
    font-size: 0.9rem;
    color: var(--primary, #4ade80);
  }

  .modal-title {
    flex: 1;
    font-size: 0.72rem;
    font-weight: 600;
    letter-spacing: 0.1em;
    color: var(--foreground);
  }

  .close-btn {
    padding: 0.25rem 0.5rem;
    background: transparent;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    cursor: pointer;
  }

  .close-btn kbd {
    font-size: 0.6rem;
    color: var(--muted-foreground);
  }

  .close-btn:hover {
    border-color: var(--foreground);
  }

  .close-btn:hover kbd {
    color: var(--foreground);
  }

  .modal-body {
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .form-label {
    font-size: 0.6rem;
    font-weight: 600;
    letter-spacing: 0.12em;
    color: var(--muted-foreground);
  }

  .url-display {
    font-size: 0.72rem;
    color: var(--foreground);
    padding: 0.5rem 0.65rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: var(--background);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .form-input,
  .form-select {
    font-family: inherit;
    font-size: 0.75rem;
    padding: 0.5rem 0.65rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: var(--background);
    color: var(--foreground);
    outline: none;
    transition: border-color 100ms ease;
  }

  .form-input:focus,
  .form-select:focus {
    border-color: var(--primary, #4ade80);
  }

  .form-input::placeholder {
    color: var(--muted-foreground);
  }

  .form-hint {
    font-size: 0.6rem;
    color: var(--muted-foreground);
    letter-spacing: 0.05em;
  }

  .tag-selector {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .no-tags {
    font-size: 0.65rem;
    color: var(--muted-foreground);
    font-style: italic;
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

  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.75rem 1rem;
    border-top: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 60%, transparent);
  }

  .footer-right {
    display: flex;
    gap: 0.5rem;
  }

  .btn {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.85rem;
    font-size: 0.68rem;
    font-weight: 500;
    letter-spacing: 0.08em;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    cursor: pointer;
    transition: all 100ms ease;
  }

  .btn kbd {
    font-size: 0.55rem;
    padding: 0.1rem 0.3rem;
    border: 1px solid;
    opacity: 0.6;
  }

  .btn-secondary {
    background: transparent;
    color: var(--muted-foreground);
  }

  .btn-secondary:hover {
    color: var(--foreground);
    border-color: var(--foreground);
  }

  .btn-primary {
    background: var(--primary, #4ade80);
    color: var(--background);
    border-color: var(--primary, #4ade80);
  }

  .btn-primary:hover {
    filter: brightness(1.1);
  }

  .btn-danger-outline {
    background: transparent;
    color: var(--muted-foreground);
  }

  .btn-danger-outline:hover {
    color: var(--destructive);
    border-color: var(--destructive);
  }

  .btn-danger {
    background: var(--destructive);
    color: var(--destructive-foreground);
    border-color: color-mix(in srgb, var(--destructive) 80%, black 20%);
  }

  .btn-danger:hover {
    background: color-mix(in srgb, var(--destructive) 85%, white 15%);
  }

  .btn-cancel {
    background: transparent;
    color: var(--muted-foreground);
  }

  .btn-cancel:hover {
    color: var(--foreground);
    border-color: var(--foreground);
  }

  .confirm-delete {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
  }

  .confirm-text {
    flex: 1;
    font-size: 0.72rem;
    color: var(--destructive);
    font-weight: 500;
  }
</style>
