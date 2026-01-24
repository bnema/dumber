// ═══════════════════════════════════════════════════════════════════════════════
// Homepage WebKit Bridge
// Communication layer between frontend and Go backend via WebKit message handlers
// ═══════════════════════════════════════════════════════════════════════════════

import type {
  MessageType,
  HistoryEntry,
  Folder,
  Tag,
  Favorite,
  DomainStat,
  Analytics,
  HistoryCleanupRange,
  FolderCreateRequest,
  FolderUpdateRequest,
  TagCreateRequest,
  TagUpdateRequest,
} from './types';
import { homepageState } from './state.svelte';

// ═══════════════════════════════════════════════════════════════════════════════
// REQUEST TRACKING
// ═══════════════════════════════════════════════════════════════════════════════

// Pending requests store resolvers for different response types.
// The resolve function accepts `unknown` since requests with different
// response types share the same map. Type safety is maintained at the
// sendMessage<T> API level.
interface PendingRequest {
  resolve: (value: unknown) => void;
  reject: (error: Error) => void;
  timeout: ReturnType<typeof setTimeout>;
}

const pendingRequests = new Map<string, PendingRequest>();
const REQUEST_TIMEOUT_MS = 10_000;

let callbacksInitialized = false;

// ═══════════════════════════════════════════════════════════════════════════════
// WEBKIT BRIDGE UTILITIES
// ═══════════════════════════════════════════════════════════════════════════════

function getWebKitBridge(): DumberBridge | null {
  const bridge = window.webkit?.messageHandlers?.dumber;
  if (bridge && typeof bridge.postMessage === 'function') {
    return bridge;
  }
  return null;
}

function getWebViewId(): number {
  return window.__dumber_webview_id ?? 0;
}

function generateRequestId(type: string): string {
  return `${type}_${Date.now()}_${Math.random().toString(36).slice(2, 9)}`;
}

// ═══════════════════════════════════════════════════════════════════════════════
// CALLBACK INITIALIZATION
// ═══════════════════════════════════════════════════════════════════════════════

function initializeCallbacks(): void {
  if (callbacksInitialized) return;

  // Generic response handler - accepts a single Response object from Go backend
  window.__dumber_homepage_response = (response) => {
    const request = pendingRequests.get(response.requestId);
    if (!request) return;

    clearTimeout(request.timeout);
    pendingRequests.delete(response.requestId);

    if (response.success) {
      request.resolve(response.data);
    } else {
      request.reject(new Error(response.error ?? 'Unknown error'));
    }
  };

  // Legacy callbacks for backward compatibility
  // These can receive either raw data or a Response object
  window.__dumber_history_timeline = (dataOrResponse) => {
    handleLegacyOrResponseCallback(dataOrResponse);
  };

  window.__dumber_folders = (dataOrResponse) => {
    handleLegacyOrResponseCallback(dataOrResponse);
  };

  window.__dumber_tags = (dataOrResponse) => {
    handleLegacyOrResponseCallback(dataOrResponse);
  };

  window.__dumber_favorites = (dataOrResponse) => {
    handleLegacyOrResponseCallback(dataOrResponse);
  };

  window.__dumber_analytics = (dataOrResponse) => {
    handleLegacyOrResponseCallback(dataOrResponse);
  };

  window.__dumber_domain_stats = (dataOrResponse) => {
    handleLegacyOrResponseCallback(dataOrResponse);
  };

  window.__dumber_history_search_results = (dataOrResponse) => {
    handleLegacyOrResponseCallback(dataOrResponse);
  };

  window.__dumber_history_deleted = (dataOrResponse) => {
    handleLegacyOrResponseCallback(dataOrResponse);
  };

  window.__dumber_history_cleared = (dataOrResponse) => {
    handleLegacyOrResponseCallback(dataOrResponse);
  };

  window.__dumber_error = (errorOrResponse) => {
    // Handle both formats: direct error string or Response object with error
    if (typeof errorOrResponse === 'string') {
      // Legacy format: direct error string
      console.error('[messaging] Legacy error:', errorOrResponse);
    } else if (errorOrResponse && typeof errorOrResponse === 'object') {
      // Response object format
      const request = pendingRequests.get(errorOrResponse.requestId ?? 'default');
      if (request) {
        clearTimeout(request.timeout);
        pendingRequests.delete(errorOrResponse.requestId ?? 'default');
        request.reject(new Error(errorOrResponse.error ?? 'Unknown error'));
      }
    }
  };

  // Listen for favorites changes from omnibox (e.g., when user toggles favorite)
  document.addEventListener('dumber:favorites-changed', () => {
    console.log('[homepage] Favorites changed event received, refreshing...');
    fetchFavorites();
  });

  callbacksInitialized = true;
}

// Handle both legacy raw data and new Response object format
function handleLegacyOrResponseCallback(dataOrResponse: unknown): void {
  // Check if it's a Response object (has requestId and success fields)
  const response = dataOrResponse as { requestId?: string; success?: boolean; data?: unknown; error?: string };
  if (response && typeof response === 'object' && 'requestId' in response && 'success' in response) {
    // New Response object format
    const request = pendingRequests.get(response.requestId || 'default');
    if (request) {
      clearTimeout(request.timeout);
      pendingRequests.delete(response.requestId || 'default');
      if (response.success) {
        request.resolve(response.data);
      } else {
        request.reject(new Error(response.error || 'Unknown error'));
      }
    }
  } else {
    // Legacy format: raw data (for backward compatibility)
    // Try to find any pending request (not ideal but maintains compatibility)
    console.warn('[messaging] Received legacy callback format without requestId');
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// CORE SEND FUNCTION
// ═══════════════════════════════════════════════════════════════════════════════

async function sendMessage<T = unknown>(
  type: MessageType,
  payload?: Record<string, unknown>
): Promise<T> {
  initializeCallbacks();

  const bridge = getWebKitBridge();
  if (!bridge) {
    throw new Error('WebKit message handler not available');
  }

  const requestId = generateRequestId(type);

  return new Promise<T>((resolve, reject) => {
    const timeout = setTimeout(() => {
      pendingRequests.delete(requestId);
      reject(new Error(`Request timeout: ${type}`));
    }, REQUEST_TIMEOUT_MS);

    // Cast resolve to accept unknown - type safety is maintained at API level
    // since sendMessage<T> returns Promise<T> and callers expect typed responses
    pendingRequests.set(requestId, {
      resolve: resolve as (value: unknown) => void,
      reject,
      timeout,
    });

    try {
      bridge.postMessage({
        type,
        webviewId: getWebViewId(),
        payload: { requestId, ...payload },
      });
    } catch (error) {
      clearTimeout(timeout);
      pendingRequests.delete(requestId);
      reject(error);
    }
  });
}

// ═══════════════════════════════════════════════════════════════════════════════
// HISTORY API
// ═══════════════════════════════════════════════════════════════════════════════

export async function fetchHistoryTimeline(
  limit: number = 50,
  offset: number = 0
): Promise<HistoryEntry[]> {
  homepageState.setHistoryLoading(true);
  try {
    const data = await sendMessage<HistoryEntry[]>('history_timeline', {
      limit,
      offset,
    });
    const entries = Array.isArray(data) ? data : [];

    if (offset === 0) {
      homepageState.setHistory(entries);
    } else {
      homepageState.appendHistory(entries);
    }

    homepageState.setHistoryOffset(offset + entries.length);
    homepageState.setHasMoreHistory(entries.length === limit);

    return entries;
  } catch (error) {
    console.error('[messaging] fetchHistoryTimeline error:', error);
    if (offset === 0) {
      homepageState.setHistory([]);
    }
    homepageState.setHasMoreHistory(false);
    return [];
  } finally {
    homepageState.setHistoryLoading(false);
  }
}

export async function searchHistoryFTS(query: string): Promise<HistoryEntry[]> {
  if (!query.trim()) {
    homepageState.setHistorySearchResults([]);
    return [];
  }

  homepageState.setHistorySearching(true);
  try {
    const data = await sendMessage<HistoryEntry[]>('history_search_fts', {
      query,
      limit: 100,
    });
    const results = Array.isArray(data) ? data : [];
    homepageState.setHistorySearchResults(results);
    return results;
  } catch (error) {
    console.error('[messaging] searchHistoryFTS error:', error);
    homepageState.setHistorySearchResults([]);
    return [];
  } finally {
    homepageState.setHistorySearching(false);
  }
}

export async function deleteHistoryEntry(id: number): Promise<void> {
  try {
    await sendMessage('history_delete_entry', { id });
    homepageState.deleteHistoryEntry(id);
  } catch (error) {
    console.error('[messaging] deleteHistoryEntry error:', error);
    throw error;
  }
}

export async function deleteHistoryByRange(range: HistoryCleanupRange): Promise<void> {
  try {
    await sendMessage('history_delete_range', { range });
    // Refresh history after deletion
    await fetchHistoryTimeline();
  } catch (error) {
    console.error('[messaging] deleteHistoryByRange error:', error);
    throw error;
  }
}

export async function clearAllHistory(): Promise<void> {
  try {
    await sendMessage('history_clear_all');
    homepageState.clearHistory();
  } catch (error) {
    console.error('[messaging] clearAllHistory error:', error);
    throw error;
  }
}

export async function deleteHistoryByDomain(domain: string): Promise<void> {
  try {
    await sendMessage('history_delete_domain', { domain });
    // Refresh history after deletion
    await fetchHistoryTimeline();
  } catch (error) {
    console.error('[messaging] deleteHistoryByDomain error:', error);
    throw error;
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// FOLDERS API
// ═══════════════════════════════════════════════════════════════════════════════

export async function fetchFolders(): Promise<Folder[]> {
  try {
    const data = await sendMessage<Folder[]>('folder_list');
    const folders = Array.isArray(data) ? data : [];
    homepageState.setFolders(folders);
    return folders;
  } catch (error) {
    console.error('[messaging] fetchFolders error:', error);
    homepageState.setFolders([]);
    return [];
  }
}

export async function createFolder(req: FolderCreateRequest): Promise<Folder> {
  const folder = await sendMessage<Folder>('folder_create', { ...req });
  homepageState.addFolder(folder);
  return folder;
}

export async function updateFolder(req: FolderUpdateRequest): Promise<void> {
  await sendMessage('folder_update', { ...req });
  homepageState.updateFolder({
    ...homepageState.folders.find(f => f.id === req.id)!,
    name: req.name,
    icon: req.icon ?? null,
  });
}

export async function deleteFolder(id: number): Promise<void> {
  await sendMessage('folder_delete', { id });
  homepageState.deleteFolder(id);
}

// ═══════════════════════════════════════════════════════════════════════════════
// TAGS API
// ═══════════════════════════════════════════════════════════════════════════════

export async function fetchTags(): Promise<Tag[]> {
  try {
    const data = await sendMessage<Tag[]>('tag_list');
    const tags = Array.isArray(data) ? data : [];
    homepageState.setTags(tags);
    return tags;
  } catch (error) {
    console.error('[messaging] fetchTags error:', error);
    homepageState.setTags([]);
    return [];
  }
}

export async function createTag(req: TagCreateRequest): Promise<Tag> {
  const tag = await sendMessage<Tag>('tag_create', { ...req });
  homepageState.addTag(tag);
  return tag;
}

export async function updateTag(req: TagUpdateRequest): Promise<void> {
  await sendMessage('tag_update', { ...req });
  const existing = homepageState.tags.find(t => t.id === req.id);
  if (existing) {
    homepageState.updateTag({
      ...existing,
      name: req.name ?? existing.name,
      color: req.color ?? existing.color,
    });
  }
}

export async function deleteTag(id: number): Promise<void> {
  await sendMessage('tag_delete', { id });
  homepageState.deleteTag(id);
}

export async function assignTag(favoriteId: number, tagId: number): Promise<void> {
  await sendMessage('tag_assign', { favorite_id: favoriteId, tag_id: tagId });
  // Update local state
  const favorite = homepageState.favorites.find(f => f.id === favoriteId);
  const tag = homepageState.tags.find(t => t.id === tagId);
  if (favorite && tag) {
    homepageState.updateFavorite({
      ...favorite,
      tags: [...(favorite.tags || []), tag],
    });
  }
}

export async function removeTag(favoriteId: number, tagId: number): Promise<void> {
  await sendMessage('tag_remove', { favorite_id: favoriteId, tag_id: tagId });
  // Update local state
  const favorite = homepageState.favorites.find(f => f.id === favoriteId);
  if (favorite) {
    homepageState.updateFavorite({
      ...favorite,
      tags: favorite.tags?.filter(t => t.id !== tagId),
    });
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// FAVORITES API
// ═══════════════════════════════════════════════════════════════════════════════

export async function fetchFavorites(): Promise<Favorite[]> {
  homepageState.setFavoritesLoading(true);
  try {
    const data = await sendMessage<Favorite[]>('favorite_list');
    const favorites = Array.isArray(data) ? data : [];
    homepageState.setFavorites(favorites);
    return favorites;
  } catch (error) {
    console.error('[messaging] fetchFavorites error:', error);
    homepageState.setFavorites([]);
    return [];
  } finally {
    homepageState.setFavoritesLoading(false);
  }
}

export async function setFavoriteShortcut(
  favoriteId: number,
  shortcutKey: number | null
): Promise<void> {
  await sendMessage('favorite_set_shortcut', {
    favorite_id: favoriteId,
    shortcut_key: shortcutKey,
  });
  // Update local state
  const favorite = homepageState.favorites.find(f => f.id === favoriteId);
  if (favorite) {
    // Clear shortcut from any other favorite that had this key
    if (shortcutKey !== null) {
      for (const f of homepageState.favorites) {
        if (f.shortcut_key === shortcutKey && f.id !== favoriteId) {
          homepageState.updateFavorite({ ...f, shortcut_key: null });
        }
      }
    }
    homepageState.updateFavorite({ ...favorite, shortcut_key: shortcutKey });
  }
}

export async function getFavoriteByShortcut(shortcutKey: number): Promise<Favorite | null> {
  try {
    const data = await sendMessage<Favorite | null>('favorite_get_by_shortcut', {
      shortcut_key: shortcutKey,
    });
    return data;
  } catch (error) {
    console.error('[messaging] getFavoriteByShortcut error:', error);
    return null;
  }
}

export async function setFavoriteFolder(
  favoriteId: number,
  folderId: number | null
): Promise<void> {
  await sendMessage('favorite_set_folder', {
    favorite_id: favoriteId,
    folder_id: folderId,
  });
  // Update local state
  const favorite = homepageState.favorites.find(f => f.id === favoriteId);
  if (favorite) {
    homepageState.updateFavorite({ ...favorite, folder_id: folderId });
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// ANALYTICS API
// ═══════════════════════════════════════════════════════════════════════════════

export async function fetchAnalytics(): Promise<Analytics | null> {
  homepageState.setAnalyticsLoading(true);
  try {
    const data = await sendMessage<Analytics>('history_analytics');
    homepageState.setAnalytics(data);
    return data;
  } catch (error) {
    console.error('[messaging] fetchAnalytics error:', error);
    homepageState.setAnalytics(null);
    return null;
  } finally {
    homepageState.setAnalyticsLoading(false);
  }
}

export async function fetchDomainStats(limit: number = 20): Promise<DomainStat[]> {
  try {
    const data = await sendMessage<DomainStat[]>('history_domain_stats', { limit });
    const stats = Array.isArray(data) ? data : [];
    homepageState.setDomainStats(stats);
    return stats;
  } catch (error) {
    console.error('[messaging] fetchDomainStats error:', error);
    homepageState.setDomainStats([]);
    return [];
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// INITIALIZATION
// ═══════════════════════════════════════════════════════════════════════════════

export async function initializeHomepage(): Promise<void> {
  // Initialize callbacks first
  initializeCallbacks();

  // Fetch all initial data in parallel
  await Promise.all([
    fetchHistoryTimeline(),
    fetchFolders(),
    fetchTags(),
    fetchFavorites(),
    fetchAnalytics(),
  ]);
}

// ═══════════════════════════════════════════════════════════════════════════════
// NAVIGATION
// ═══════════════════════════════════════════════════════════════════════════════

export function navigateTo(url: string): void {
  window.location.href = url;
}
