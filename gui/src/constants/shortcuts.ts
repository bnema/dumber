/**
 * Search shortcuts configuration
 *
 * @deprecated This file is only used as a fallback. The real source of truth
 * is the backend Go config (internal/config/defaults.go). Shortcuts should be
 * fetched from the backend via /api/config or the messaging bridge.
 *
 * Only kept for development fallback when backend is unavailable.
 */

export interface SearchShortcutDefinition {
  shortcut: string;
  url_template: string;
  description: string;
}

/**
 * Default search shortcuts
 * Alphabetically ordered by shortcut key
 */
export const DEFAULT_SHORTCUTS: Record<string, SearchShortcutDefinition> = {
  ddg: {
    shortcut: "ddg",
    url_template: "https://duckduckgo.com/?q={query}",
    description: "DuckDuckGo search",
  },
  g: {
    shortcut: "g",
    url_template: "https://www.google.com/search?q={query}",
    description: "Google search",
  },
  gh: {
    shortcut: "gh",
    url_template: "https://github.com/search?q={query}",
    description: "GitHub search",
  },
  go: {
    shortcut: "go",
    url_template: "https://pkg.go.dev/search?q={query}",
    description: "Go package search",
  },
  mdn: {
    shortcut: "mdn",
    url_template: "https://developer.mozilla.org/en-US/search?q={query}",
    description: "MDN Web Docs search",
  },
  npm: {
    shortcut: "npm",
    url_template: "https://www.npmjs.com/search?q={query}",
    description: "npm package search",
  },
  r: {
    shortcut: "r",
    url_template: "https://www.reddit.com/search?q={query}",
    description: "Reddit search",
  },
  so: {
    shortcut: "so",
    url_template: "https://stackoverflow.com/search?q={query}",
    description: "Stack Overflow search",
  },
  w: {
    shortcut: "w",
    url_template: "https://en.wikipedia.org/wiki/{query}",
    description: "Wikipedia search",
  },
  yt: {
    shortcut: "yt",
    url_template: "https://www.youtube.com/results?search_query={query}",
    description: "YouTube search",
  },
};

/**
 * Get array of shortcut keys for validation
 */
export const SHORTCUT_KEYS = Object.keys(DEFAULT_SHORTCUTS);

/**
 * Check if a string is a valid shortcut
 */
export function isValidShortcut(key: string): boolean {
  return key in DEFAULT_SHORTCUTS;
}

/**
 * Get shortcut description by key
 */
export function getShortcutDescription(key: string): string | null {
  return DEFAULT_SHORTCUTS[key]?.description ?? null;
}
