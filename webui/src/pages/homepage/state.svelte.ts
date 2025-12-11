// ═══════════════════════════════════════════════════════════════════════════════
// Homepage Global State (Svelte 5 Runes)
// Single module-level $state with exported store object
// ═══════════════════════════════════════════════════════════════════════════════

import type {
  HistoryEntry,
  TimelineGroup,
  DomainStat,
  DailyVisitCount,
  HourlyDistribution,
  Favorite,
  Folder,
  Tag,
  Analytics,
  PanelType,
  HistoryCleanupRange,
} from './types';

// ═══════════════════════════════════════════════════════════════════════════════
// HISTORY STATE
// ═══════════════════════════════════════════════════════════════════════════════

let history = $state<HistoryEntry[]>([]);
let timeline = $state<TimelineGroup[]>([]);
let historyLoading = $state(true);
let historyOffset = $state(0);
let hasMoreHistory = $state(true);
let historySearchQuery = $state('');
let historySearchResults = $state<HistoryEntry[]>([]);
let historySearching = $state(false);

// ═══════════════════════════════════════════════════════════════════════════════
// FAVORITES STATE
// ═══════════════════════════════════════════════════════════════════════════════

let favorites = $state<Favorite[]>([]);
let folders = $state<Folder[]>([]);
let tags = $state<Tag[]>([]);
let favoritesLoading = $state(true);
let selectedFolderId = $state<number | null>(null);
let selectedTagIds = $state<number[]>([]);
let editingFavorite = $state<Favorite | null>(null);

// ═══════════════════════════════════════════════════════════════════════════════
// ANALYTICS STATE
// ═══════════════════════════════════════════════════════════════════════════════

let analytics = $state<Analytics | null>(null);
let analyticsLoading = $state(true);
let domainStats = $state<DomainStat[]>([]);
let dailyVisits = $state<DailyVisitCount[]>([]);
let hourlyDistribution = $state<HourlyDistribution[]>([]);

// ═══════════════════════════════════════════════════════════════════════════════
// UI STATE
// ═══════════════════════════════════════════════════════════════════════════════

let activePanel = $state<PanelType>('history');
let commandPaletteOpen = $state(false);
let commandPaletteQuery = $state('');
let focusedIndex = $state(-1); // -1 means no item focused (focus on search input)
let confirmModalOpen = $state(false);
let confirmModalMessage = $state('');
let confirmModalAction = $state<(() => void) | null>(null);
let cleanupModalOpen = $state(false);
let cleanupRange = $state<HistoryCleanupRange>('hour');
let pendingKeyPrefix = $state<string | null>(null);

// ═══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════════

function groupHistoryByDate(entries: HistoryEntry[]): TimelineGroup[] {
  const groups = new Map<string, HistoryEntry[]>();
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);

  for (const entry of entries) {
    const date = new Date(entry.last_visited);
    const dateKey = date.toISOString().split('T')[0] ?? '';
    if (!groups.has(dateKey)) {
      groups.set(dateKey, []);
    }
    groups.get(dateKey)!.push(entry);
  }

  const result: TimelineGroup[] = [];
  for (const [dateKey, items] of groups) {
    const date = new Date(dateKey);
    let label: string;

    if (date >= today) {
      label = 'Today';
    } else if (date >= yesterday) {
      label = 'Yesterday';
    } else {
      label = date.toLocaleDateString(undefined, {
        weekday: 'short',
        month: 'short',
        day: 'numeric',
      });
    }

    result.push({ date: dateKey, label, entries: items });
  }

  return result.sort((a, b) => b.date.localeCompare(a.date));
}

// ═══════════════════════════════════════════════════════════════════════════════
// EXPORTED STORE
// ═══════════════════════════════════════════════════════════════════════════════

export const homepageState = {
  // ─────────────────────────────────────────────────────────────────────────────
  // History Getters
  // ─────────────────────────────────────────────────────────────────────────────
  get history() { return history; },
  get timeline() { return timeline; },
  get historyLoading() { return historyLoading; },
  get historyOffset() { return historyOffset; },
  get hasMoreHistory() { return hasMoreHistory; },
  get historySearchQuery() { return historySearchQuery; },
  get historySearchResults() { return historySearchResults; },
  get historySearching() { return historySearching; },

  // ─────────────────────────────────────────────────────────────────────────────
  // Favorites Getters
  // ─────────────────────────────────────────────────────────────────────────────
  get favorites() { return favorites; },
  get folders() { return folders; },
  get tags() { return tags; },
  get favoritesLoading() { return favoritesLoading; },
  get selectedFolderId() { return selectedFolderId; },
  get selectedTagIds() { return selectedTagIds; },
  get editingFavorite() { return editingFavorite; },

  // ─────────────────────────────────────────────────────────────────────────────
  // Analytics Getters
  // ─────────────────────────────────────────────────────────────────────────────
  get analytics() { return analytics; },
  get analyticsLoading() { return analyticsLoading; },
  get domainStats() { return domainStats; },
  get dailyVisits() { return dailyVisits; },
  get hourlyDistribution() { return hourlyDistribution; },

  // ─────────────────────────────────────────────────────────────────────────────
  // UI Getters
  // ─────────────────────────────────────────────────────────────────────────────
  get activePanel() { return activePanel; },
  get commandPaletteOpen() { return commandPaletteOpen; },
  get commandPaletteQuery() { return commandPaletteQuery; },
  get focusedIndex() { return focusedIndex; },
  get confirmModalOpen() { return confirmModalOpen; },
  get confirmModalMessage() { return confirmModalMessage; },
  get cleanupModalOpen() { return cleanupModalOpen; },
  get cleanupRange() { return cleanupRange; },
  get pendingKeyPrefix() { return pendingKeyPrefix; },

  // ─────────────────────────────────────────────────────────────────────────────
  // Computed Properties
  // ─────────────────────────────────────────────────────────────────────────────
  get filteredFavorites(): Favorite[] {
    let result = favorites;

    // Filter by folder
    if (selectedFolderId !== null) {
      result = result.filter(f => f.folder_id === selectedFolderId);
    }

    // Filter by tags
    if (selectedTagIds.length > 0) {
      result = result.filter(f =>
        f.tags?.some(t => selectedTagIds.includes(t.id))
      );
    }

    return result;
  },

  get currentList(): HistoryEntry[] | Favorite[] {
    switch (activePanel) {
      case 'history':
        return historySearchQuery ? historySearchResults : history;
      case 'favorites':
        return this.filteredFavorites;
      default:
        return [];
    }
  },

  get currentListLength(): number {
    return this.currentList.length;
  },

  // ─────────────────────────────────────────────────────────────────────────────
  // History Actions
  // ─────────────────────────────────────────────────────────────────────────────
  setHistory(items: HistoryEntry[]) {
    history = items;
    timeline = groupHistoryByDate(items);
  },

  appendHistory(items: HistoryEntry[]) {
    history = [...history, ...items];
    timeline = groupHistoryByDate(history);
  },

  setHistoryLoading(loading: boolean) {
    historyLoading = loading;
  },

  setHistoryOffset(offset: number) {
    historyOffset = offset;
  },

  setHasMoreHistory(hasMore: boolean) {
    hasMoreHistory = hasMore;
  },

  deleteHistoryEntry(id: number) {
    history = history.filter(h => h.id !== id);
    timeline = groupHistoryByDate(history);
    historySearchResults = historySearchResults.filter(h => h.id !== id);
  },

  clearHistory() {
    history = [];
    timeline = [];
    historySearchResults = [];
    historyOffset = 0;
    hasMoreHistory = true;
  },

  setHistorySearchQuery(q: string) {
    historySearchQuery = q;
    if (!q) {
      historySearchResults = [];
    }
  },

  setHistorySearchResults(results: HistoryEntry[]) {
    historySearchResults = results;
  },

  setHistorySearching(searching: boolean) {
    historySearching = searching;
  },

  // ─────────────────────────────────────────────────────────────────────────────
  // Favorites Actions
  // ─────────────────────────────────────────────────────────────────────────────
  setFavorites(items: Favorite[]) {
    favorites = items;
  },

  setFolders(items: Folder[]) {
    folders = items;
  },

  setTags(items: Tag[]) {
    tags = items;
  },

  setFavoritesLoading(loading: boolean) {
    favoritesLoading = loading;
  },

  selectFolder(id: number | null) {
    selectedFolderId = id;
    focusedIndex = 0;
  },

  toggleTag(id: number) {
    if (selectedTagIds.includes(id)) {
      selectedTagIds = selectedTagIds.filter(t => t !== id);
    } else {
      selectedTagIds = [...selectedTagIds, id];
    }
    focusedIndex = 0;
  },

  clearTagSelection() {
    selectedTagIds = [];
    focusedIndex = 0;
  },

  setEditingFavorite(fav: Favorite | null) {
    editingFavorite = fav;
  },

  updateFavorite(updated: Favorite) {
    favorites = favorites.map(f => f.id === updated.id ? updated : f);
  },

  deleteFavorite(id: number) {
    favorites = favorites.filter(f => f.id !== id);
  },

  addFolder(folder: Folder) {
    folders = [...folders, folder];
  },

  updateFolder(updated: Folder) {
    folders = folders.map(f => f.id === updated.id ? updated : f);
  },

  deleteFolder(id: number) {
    folders = folders.filter(f => f.id !== id);
    // Clear folder selection if deleted folder was selected
    if (selectedFolderId === id) {
      selectedFolderId = null;
    }
    // Update favorites that were in this folder
    favorites = favorites.map(f =>
      f.folder_id === id ? { ...f, folder_id: null } : f
    );
  },

  addTag(tag: Tag) {
    tags = [...tags, tag];
  },

  updateTag(updated: Tag) {
    tags = tags.map(t => t.id === updated.id ? updated : t);
  },

  deleteTag(id: number) {
    tags = tags.filter(t => t.id !== id);
    selectedTagIds = selectedTagIds.filter(t => t !== id);
    // Remove tag from all favorites
    favorites = favorites.map(f => ({
      ...f,
      tags: f.tags?.filter(t => t.id !== id),
    }));
  },

  // ─────────────────────────────────────────────────────────────────────────────
  // Analytics Actions
  // ─────────────────────────────────────────────────────────────────────────────
  setAnalytics(data: Analytics | null) {
    analytics = data;
    if (data) {
      domainStats = data.top_domains ?? [];
      dailyVisits = data.daily_visits ?? [];
      hourlyDistribution = data.hourly_distribution ?? [];
    }
  },

  setAnalyticsLoading(loading: boolean) {
    analyticsLoading = loading;
  },

  setDomainStats(stats: DomainStat[]) {
    domainStats = stats;
  },

  // ─────────────────────────────────────────────────────────────────────────────
  // UI Actions
  // ─────────────────────────────────────────────────────────────────────────────
  setActivePanel(panel: PanelType) {
    activePanel = panel;
    focusedIndex = -1; // Clear focus, let search input be focused
    // Clear search when switching panels
    historySearchQuery = '';
    historySearchResults = [];
  },

  openCommandPalette() {
    commandPaletteOpen = true;
    commandPaletteQuery = '';
  },

  closeCommandPalette() {
    commandPaletteOpen = false;
    commandPaletteQuery = '';
  },

  setCommandPaletteQuery(q: string) {
    commandPaletteQuery = q;
  },

  setFocusedIndex(i: number) {
    focusedIndex = Math.max(-1, i);
  },

  focusNext() {
    const maxIndex = this.currentListLength - 1;
    if (maxIndex < 0) return;
    // If unfocused (-1), go to first item
    if (focusedIndex === -1) {
      focusedIndex = 0;
    } else {
      focusedIndex = Math.min(focusedIndex + 1, maxIndex);
    }
  },

  focusPrev() {
    // If at first item or unfocused, stay at -1 (return to search)
    if (focusedIndex <= 0) {
      focusedIndex = -1;
    } else {
      focusedIndex = focusedIndex - 1;
    }
  },

  focusFirst() {
    focusedIndex = 0;
  },

  focusLast() {
    focusedIndex = Math.max(0, this.currentListLength - 1);
  },

  clearFocus() {
    focusedIndex = -1;
  },

  // Confirmation modal
  showConfirm(message: string, action: () => void) {
    confirmModalMessage = message;
    confirmModalAction = action;
    confirmModalOpen = true;
  },

  confirmAction() {
    confirmModalAction?.();
    confirmModalOpen = false;
    confirmModalAction = null;
    confirmModalMessage = '';
  },

  cancelConfirm() {
    confirmModalOpen = false;
    confirmModalAction = null;
    confirmModalMessage = '';
  },

  // Cleanup modal
  openCleanupModal() {
    cleanupModalOpen = true;
    cleanupRange = 'hour';
  },

  closeCleanupModal() {
    cleanupModalOpen = false;
  },

  setCleanupRange(range: HistoryCleanupRange) {
    cleanupRange = range;
  },

  // Keyboard prefix state (for 'g' and 'D' prefix indicators)
  setPendingKeyPrefix(prefix: string | null) {
    pendingKeyPrefix = prefix;
  },

  // ─────────────────────────────────────────────────────────────────────────────
  // Reset
  // ─────────────────────────────────────────────────────────────────────────────
  reset() {
    // History
    history = [];
    timeline = [];
    historyLoading = true;
    historyOffset = 0;
    hasMoreHistory = true;
    historySearchQuery = '';
    historySearchResults = [];
    historySearching = false;

    // Favorites
    favorites = [];
    folders = [];
    tags = [];
    favoritesLoading = true;
    selectedFolderId = null;
    selectedTagIds = [];
    editingFavorite = null;

    // Analytics
    analytics = null;
    analyticsLoading = true;
    domainStats = [];
    dailyVisits = [];
    hourlyDistribution = [];

    // UI
    activePanel = 'history';
    commandPaletteOpen = false;
    commandPaletteQuery = '';
    focusedIndex = -1;
    confirmModalOpen = false;
    confirmModalMessage = '';
    confirmModalAction = null;
    cleanupModalOpen = false;
    cleanupRange = 'hour';
  },
};
