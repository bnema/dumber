/**
 * Omnibox Component Types
 *
 * TypeScript interfaces for omnibox/find functionality that match Go structs
 */

export interface Suggestion {
  url: string;
  favicon?: string;
}

export interface SearchShortcut {
  url: string;
  description: string;
}

export interface OmniboxMessage {
  type: "navigate" | "query" | "get_search_shortcuts";
  url?: string;
  q?: string;
  limit?: number;
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

export interface OmniboxConfig {
  maxMatches?: number;
  debounceDelay?: number;
  defaultLimit?: number;
}

export interface OmniboxState {
  visible: boolean;
  mode: OmniboxMode;
  suggestions: Suggestion[];
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
