/**
 * Omnibox Component Types
 *
 * TypeScript interfaces for omnibox/find functionality that match Go structs
 */

export interface Suggestion {
  url: string;
  favicon?: string;
}

export interface Favorite {
  id: number;
  url: string;
  title: string;
  favicon_url: string;
  position: number;
}

export interface SearchShortcut {
  url: string;
  description: string;
}

export interface OmniboxMessage {
  type: "navigate" | "query" | "omnibox_initial_history" | "get_search_shortcuts" | "get_favorites" | "toggle_favorite" | "is_favorite" | "prefix_query";
  url?: string;
  q?: string;
  limit?: number;
  title?: string;
  faviconURL?: string;
}

export interface FindMatch {
  element: HTMLElement;
  context: string;
}

export interface HighlightNode {
  span: HTMLElement;
  text: Text;
}

export type OmniboxMode = "omnibox" | "find";
export type ViewMode = "history" | "favorites";

export interface OmniboxConfig {
  maxMatches?: number;
  debounceDelay?: number;
  defaultLimit?: number;
}

export interface OmniboxState {
  visible: boolean;
  mode: OmniboxMode;
  viewMode: ViewMode;
  suggestions: Suggestion[];
  favorites: Favorite[];
  matches: FindMatch[];
  selectedIndex: number;
  activeIndex: number;
  faded: boolean;
  inputValue: string;
  highlightNodes: HighlightNode[];
  prevOverflow: string;
  searchShortcuts: Record<string, SearchShortcut>;
}

// Message bridge interface for Go communication
export interface OmniboxMessageBridge {
  postMessage(msg: OmniboxMessage): void;
  setSuggestions(suggestions: Suggestion[]): void;
}

// Global API for Go bridge
export interface OmniboxAPI {
  setSuggestions: (suggestions: Suggestion[]) => void;
  toggle: () => void;
  open: (mode?: OmniboxMode, query?: string) => void;
  close: () => void;
  findQuery: (query: string) => void;
}
