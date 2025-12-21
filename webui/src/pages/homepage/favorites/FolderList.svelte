<script lang="ts">
  import type { Folder, Favorite } from '../types';

  interface Props {
    folders: Folder[];
    favorites: Favorite[];
    selectedFolderId?: number | null;
    onSelectFolder?: (id: number | null) => void;
    onCreateFolder?: (name: string) => Promise<void>;
  }

  let {
    folders,
    favorites,
    selectedFolderId = null,
    onSelectFolder,
    onCreateFolder
  }: Props = $props();

  // Inline creation state
  let isCreating = $state(false);
  let newFolderName = $state('');
  let inputRef = $state<HTMLInputElement | null>(null);

  const startCreating = () => {
    isCreating = true;
    newFolderName = '';
    // Focus input after it renders
    setTimeout(() => inputRef?.focus(), 0);
  };

  const cancelCreating = () => {
    isCreating = false;
    newFolderName = '';
  };

  const submitCreate = async () => {
    const name = newFolderName.trim();
    if (!name || !onCreateFolder) {
      cancelCreating();
      return;
    }
    try {
      await onCreateFolder(name);
      cancelCreating();
    } catch (e) {
      console.error('Failed to create folder:', e);
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

  // Count favorites per folder
  const getFolderCount = (folderId: number | null): number => {
    if (folderId === null) {
      return favorites.length;
    }
    return favorites.filter(f => f.folder_id === folderId).length;
  };

  // Count unfiled favorites
  const unfiledCount = $derived(favorites.filter(f => f.folder_id === null).length);
</script>

<div class="folder-list">
  <div class="folder-header">
    <span class="header-label">FOLDERS</span>
    {#if isCreating}
      <button
        class="add-folder-btn cancel"
        type="button"
        onclick={cancelCreating}
        title="Cancel"
      >
        ×
      </button>
    {:else}
      <button
        class="add-folder-btn"
        type="button"
        onclick={startCreating}
        title="Create folder"
      >
        +
      </button>
    {/if}
  </div>

  {#if isCreating}
    <div class="create-input-row">
      <input
        bind:this={inputRef}
        bind:value={newFolderName}
        onkeydown={handleKeydown}
        onblur={cancelCreating}
        class="create-input"
        type="text"
        placeholder="Folder name..."
        maxlength="50"
      />
    </div>
  {/if}

  <div class="folder-items">
    <!-- All favorites -->
    <button
      class="folder-item"
      class:active={selectedFolderId === null}
      type="button"
      onclick={() => onSelectFolder?.(null)}
    >
      <span class="folder-icon"></span>
      <span class="folder-name">All</span>
      <span class="folder-count">{favorites.length}</span>
    </button>

    <!-- User folders -->
    {#each folders as folder (folder.id)}
      <button
        class="folder-item"
        class:active={selectedFolderId === folder.id}
        type="button"
        onclick={() => onSelectFolder?.(folder.id)}
      >
        <span class="folder-icon">{folder.icon || ''}</span>
        <span class="folder-name">{folder.name}</span>
        <span class="folder-count">{getFolderCount(folder.id)}</span>
      </button>
    {/each}

    <!-- Unfiled -->
    {#if unfiledCount > 0 && folders.length > 0}
      <button
        class="folder-item unfiled"
        class:active={selectedFolderId === -1}
        type="button"
        onclick={() => onSelectFolder?.(-1)}
      >
        <span class="folder-icon"></span>
        <span class="folder-name">Unfiled</span>
        <span class="folder-count">{unfiledCount}</span>
      </button>
    {/if}
  </div>
</div>

<style>
  .folder-list {
    display: flex;
    flex-direction: column;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: color-mix(in srgb, var(--background) 92%, var(--card) 8%);
  }

  .folder-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.5rem 0.75rem;
    border-bottom: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 80%, var(--card) 20%);
  }

  .header-label {
    font-size: 0.6rem;
    font-weight: 600;
    color: var(--muted-foreground);
    letter-spacing: 0.12em;
  }

  .add-folder-btn {
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

  .add-folder-btn:hover {
    color: var(--foreground);
    border-color: var(--foreground);
  }

  .add-folder-btn.cancel {
    color: var(--muted-foreground);
  }

  .add-folder-btn.cancel:hover {
    color: var(--destructive);
    border-color: var(--destructive);
  }

  .create-input-row {
    padding: 0.4rem 0.5rem;
    border-bottom: 1px solid var(--border);
    background: color-mix(in srgb, var(--primary, #4ade80) 8%, var(--background) 92%);
  }

  .create-input {
    width: 100%;
    padding: 0.35rem 0.5rem;
    font-size: 0.7rem;
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

  .folder-items {
    display: flex;
    flex-direction: column;
  }

  .folder-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.75rem;
    text-align: left;
    background: transparent;
    border: none;
    border-bottom: 1px solid color-mix(in srgb, var(--border) 40%, transparent);
    cursor: pointer;
    transition: background-color 100ms ease;
  }

  .folder-item:last-child {
    border-bottom: none;
  }

  .folder-item:hover {
    background: color-mix(in srgb, var(--card) 30%, transparent);
  }

  .folder-item.active {
    background: color-mix(in srgb, var(--card) 50%, transparent);
  }

  .folder-item.active::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 2px;
    background: var(--primary, #4ade80);
  }

  .folder-item {
    position: relative;
  }

  .folder-icon {
    font-size: 0.8rem;
    width: 1.2rem;
    text-align: center;
    color: var(--muted-foreground);
  }

  .folder-item.active .folder-icon {
    color: var(--foreground);
  }

  .folder-name {
    flex: 1;
    font-size: 0.72rem;
    color: var(--foreground);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .folder-item.unfiled .folder-name {
    font-style: italic;
    color: var(--muted-foreground);
  }

  .folder-count {
    font-size: 0.6rem;
    padding: 0.1rem 0.35rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: var(--background);
    color: var(--muted-foreground);
  }
</style>
