// ═══════════════════════════════════════════════════════════════════════════════
// Homepage TypeScript Types
// Mirrors Go models from internal/db/models.go and related query files
// ═══════════════════════════════════════════════════════════════════════════════

// ─────────────────────────────────────────────────────────────────────────────
// History Types
// ─────────────────────────────────────────────────────────────────────────────

export interface HistoryEntry {
  id: number;
  url: string;
  title: string | null;
  favicon_url: string | null;
  visit_count: number;
  last_visited: string;
  created_at: string;
}

export interface TimelineGroup {
  date: string; // YYYY-MM-DD
  label: string; // "Today", "Yesterday", "Dec 5", etc.
  entries: HistoryEntry[];
}

export interface HistoryStats {
  total_entries: number;
  total_visits: number;
  unique_days: number;
}

export interface DomainStat {
  domain: string;
  page_count: number;
  total_visits: number;
  last_visit: string;
}

export interface DailyVisitCount {
  day: string;
  entries: number;
  visits: number;
}

export interface HourlyDistribution {
  hour: number;
  visit_count: number;
}

// ─────────────────────────────────────────────────────────────────────────────
// Favorites Types
// ─────────────────────────────────────────────────────────────────────────────

export interface Favorite {
  id: number;
  url: string;
  title: string | null;
  favicon_url: string | null;
  position: number;
  created_at: string;
  updated_at: string;
  folder_id: number | null;
  shortcut_key: number | null; // 1-9
  tags?: Tag[]; // Populated via join
}

export interface Folder {
  id: number;
  name: string;
  icon: string | null;
  position: number;
  created_at: string;
}

export interface Tag {
  id: number;
  name: string;
  color: string; // Hex color like #6b7280
  created_at: string;
}

export interface TagAssignment {
  favorite_id: number;
  tag_id: number;
}

// ─────────────────────────────────────────────────────────────────────────────
// Analytics Types
// ─────────────────────────────────────────────────────────────────────────────

// Matches Go HistoryAnalytics struct (flat structure)
export interface Analytics {
  total_entries: number;
  total_visits: number;
  unique_days: number;
  top_domains: DomainStat[];
  daily_visits: DailyVisitCount[];
  hourly_distribution: HourlyDistribution[];
}

// ─────────────────────────────────────────────────────────────────────────────
// UI State Types
// ─────────────────────────────────────────────────────────────────────────────

export type PanelType = 'history' | 'favorites' | 'analytics';

export type HistoryCleanupRange = 'hour' | 'day' | 'week' | 'month' | 'all';

export interface CommandPaletteItem {
  id: string;
  label: string;
  description?: string;
  icon?: import('svelte').Component | string;
  action: () => void;
  shortcut?: string;
}

export interface KeyboardShortcut {
  key: string;
  description: string;
  action: () => void;
  context?: 'global' | 'history' | 'favorites' | 'command-palette';
}

// ─────────────────────────────────────────────────────────────────────────────
// Message Types (WebKit Bridge)
// ─────────────────────────────────────────────────────────────────────────────

export type MessageType =
  // Folders
  | 'folder_list'
  | 'folder_create'
  | 'folder_update'
  | 'folder_delete'
  // Tags
  | 'tag_list'
  | 'tag_create'
  | 'tag_update'
  | 'tag_delete'
  | 'tag_assign'
  | 'tag_remove'
  // History
  | 'history_timeline'
  | 'history_search_fts'
  | 'history_analytics'
  | 'history_domain_stats'
  | 'history_delete_entry'
  | 'history_delete_range'
  | 'history_clear_all'
  | 'history_delete_domain'
  // Favorites
  | 'favorite_list'
  | 'favorite_set_shortcut'
  | 'favorite_get_by_shortcut'
  | 'favorite_set_folder';

export interface Message<T = unknown> {
  type: MessageType;
  requestId?: string;
  payload?: T;
}

export interface MessageResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: string;
}

// ─────────────────────────────────────────────────────────────────────────────
// Request/Response Payloads
// ─────────────────────────────────────────────────────────────────────────────

// History
export interface HistoryTimelineRequest {
  limit?: number;
  offset?: number;
}

export interface HistorySearchRequest {
  query: string;
  limit?: number;
}

export interface HistoryDeleteRangeRequest {
  range: HistoryCleanupRange;
}

export interface HistoryDeleteDomainRequest {
  domain: string;
}

// Folders
export interface FolderCreateRequest {
  name: string;
  icon?: string;
}

export interface FolderUpdateRequest {
  id: number;
  name: string;
  icon?: string;
}

// Tags
export interface TagCreateRequest {
  name: string;
  color?: string;
}

export interface TagUpdateRequest {
  id: number;
  name?: string;
  color?: string;
}

export interface TagAssignRequest {
  favorite_id: number;
  tag_id: number;
}

// Favorites
export interface FavoriteShortcutRequest {
  favorite_id: number;
  shortcut_key: number; // 1-9
}

export interface FavoriteFolderRequest {
  favorite_id: number;
  folder_id: number | null;
}
