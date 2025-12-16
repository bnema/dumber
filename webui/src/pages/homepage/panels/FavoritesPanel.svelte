<script lang="ts">
  import { homepageState } from '../state.svelte';
  import {
    navigateTo,
    setFavoriteShortcut,
    setFavoriteFolder,
    assignTag,
    removeTag,
    createFolder,
    createTag
  } from '../messaging';
  import type { Favorite } from '../types';

  import { FolderList, TagCloud, FavoriteGrid, FavoriteEditor } from '../favorites';

  let editorOpen = $state(false);
  let editingFavorite = $state<Favorite | null>(null);

  // Get filtered favorites based on folder and tag selection
  const displayedFavorites = $derived(() => {
    let result = homepageState.favorites;

    // Filter by folder
    const folderId = homepageState.selectedFolderId;
    if (folderId !== null) {
      if (folderId === -1) {
        // Show unfiled
        result = result.filter(f => f.folder_id === null);
      } else {
        result = result.filter(f => f.folder_id === folderId);
      }
    }

    // Filter by tags
    const tagIds = homepageState.selectedTagIds;
    if (tagIds.length > 0) {
      result = result.filter(f =>
        f.tags?.some(t => tagIds.includes(t.id))
      );
    }

    return result;
  });

  const handleSelectFavorite = (fav: Favorite) => {
    navigateTo(fav.url);
  };

  const handleEditFavorite = (fav: Favorite) => {
    editingFavorite = fav;
    editorOpen = true;
  };

  const handleSaveFavorite = async (updates: Partial<Favorite>) => {
    if (!updates.id) return;

    // Update shortcut key
    if (updates.shortcut_key !== undefined) {
      await setFavoriteShortcut(updates.id, updates.shortcut_key);
    }

    // Update folder
    if (updates.folder_id !== undefined) {
      await setFavoriteFolder(updates.id, updates.folder_id);
    }

    // Title update would need a separate handler - for now just update local state
    if (updates.title !== undefined && editingFavorite) {
      homepageState.updateFavorite({
        ...editingFavorite,
        ...updates
      });
    }
  };

  const handleDeleteFavorite = (id: number) => {
    // This would call a delete API - for now just update local state
    homepageState.deleteFavorite(id);
  };

  const handleAssignTag = async (favoriteId: number, tagId: number) => {
    await assignTag(favoriteId, tagId);
  };

  const handleRemoveTag = async (favoriteId: number, tagId: number) => {
    await removeTag(favoriteId, tagId);
  };

  const handleSelectFolder = (id: number | null) => {
    homepageState.selectFolder(id);
  };

  const handleToggleTag = (id: number) => {
    homepageState.toggleTag(id);
  };

  const handleClearTagSelection = () => {
    homepageState.clearTagSelection();
  };

  const handleCreateFolder = async (name: string) => {
    await createFolder({ name });
  };

  const handleCreateTag = async (name: string, color: string) => {
    await createTag({ name, color });
  };

  const closeEditor = () => {
    editorOpen = false;
    editingFavorite = null;
  };
</script>

<div class="favorites-panel">
  <div class="panel-sidebar">
    <FolderList
      folders={homepageState.folders}
      favorites={homepageState.favorites}
      selectedFolderId={homepageState.selectedFolderId}
      onSelectFolder={handleSelectFolder}
      onCreateFolder={handleCreateFolder}
    />

    <TagCloud
      tags={homepageState.tags}
      selectedTagIds={homepageState.selectedTagIds}
      onToggleTag={handleToggleTag}
      onClearSelection={handleClearTagSelection}
      onCreateTag={handleCreateTag}
    />
  </div>

  <div class="panel-main">
    <div class="panel-header">
      <span class="header-title">
        {#if homepageState.selectedFolderId === null}
          ALL FAVORITES
        {:else if homepageState.selectedFolderId === -1}
          UNFILED
        {:else}
          {homepageState.folders.find(f => f.id === homepageState.selectedFolderId)?.name ?? 'FOLDER'}
        {/if}
      </span>
      <span class="header-count">{displayedFavorites().length}</span>
    </div>

    <div class="panel-content">
      {#if homepageState.favoritesLoading}
        <div class="loading-state">
          <span class="loading-spinner"></span>
          <span class="loading-text">LOADING FAVORITES...</span>
        </div>
      {:else}
        <FavoriteGrid
          favorites={displayedFavorites()}
          onSelectFavorite={handleSelectFavorite}
          onEditFavorite={handleEditFavorite}
        />
      {/if}
    </div>
  </div>

  {#if editorOpen && editingFavorite}
    <FavoriteEditor
      favorite={editingFavorite}
      folders={homepageState.folders}
      tags={homepageState.tags}
      onSave={handleSaveFavorite}
      onDelete={handleDeleteFavorite}
      onClose={closeEditor}
      onAssignTag={handleAssignTag}
      onRemoveTag={handleRemoveTag}
    />
  {/if}
</div>

<style>
  .favorites-panel {
    display: grid;
    grid-template-columns: 200px 1fr;
    gap: 0.75rem;
    padding: 0.5rem 1rem;
  }

  .panel-sidebar {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .panel-main {
    display: flex;
    flex-direction: column;
    min-height: 0;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: color-mix(in srgb, var(--background) 95%, var(--card) 5%);
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.6rem 0.85rem;
    border-bottom: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 80%, var(--card) 20%);
  }

  .header-title {
    font-size: 0.68rem;
    font-weight: 600;
    letter-spacing: 0.12em;
    color: var(--foreground);
  }

  .header-count {
    font-size: 0.6rem;
    padding: 0.15rem 0.4rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: var(--background);
    color: var(--muted-foreground);
  }

  .panel-content {
    padding: 0.75rem;
  }

  .loading-state {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    padding: 2rem;
    color: var(--muted-foreground);
  }

  .loading-spinner {
    width: 24px;
    height: 24px;
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

  .loading-text {
    font-size: 0.68rem;
    letter-spacing: 0.12em;
    animation: pulse 1.5s ease-in-out infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 0.5; }
    50% { opacity: 1; }
  }

  /* Responsive: stack on smaller screens */
  @media (max-width: 768px) {
    .favorites-panel {
      grid-template-columns: 1fr;
      grid-template-rows: auto 1fr;
    }

    .panel-sidebar {
      flex-direction: row;
      gap: 0.5rem;
      overflow-x: auto;
    }

    .panel-sidebar > :global(*) {
      flex-shrink: 0;
      min-width: 180px;
    }
  }
</style>
