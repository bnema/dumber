export interface HistoryEntry {
  id: number;
  url: string;
  title: string | null;
  favicon_url: string | null;
  visit_count: number;
  last_visited: string;
  created_at: string;
}

export type History = HistoryEntry;

export interface SearchShortcut {
  url: string;
  description: string;
}

export interface Shortcut {
  url_template: string;
  description?: string;
}
