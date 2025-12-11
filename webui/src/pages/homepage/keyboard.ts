// ═══════════════════════════════════════════════════════════════════════════════
// Homepage Keyboard Navigation (Telescope-style)
// Vim-inspired navigation with fuzzy filtering and quick shortcuts
// ═══════════════════════════════════════════════════════════════════════════════

import { homepageState } from './state.svelte';
import {
  navigateTo,
  deleteHistoryEntry,
  deleteHistoryByRange,
  clearAllHistory,
  deleteHistoryByDomain,
  getFavoriteByShortcut,
  searchHistoryFTS,
} from './messaging';
import type { HistoryEntry, Favorite, HistoryCleanupRange } from './types';

// ═══════════════════════════════════════════════════════════════════════════════
// TYPES
// ═══════════════════════════════════════════════════════════════════════════════

interface KeyBinding {
  key: string;
  ctrl?: boolean;
  shift?: boolean;
  alt?: boolean;
  meta?: boolean;
  description: string;
  action: () => void | Promise<void>;
  context?: 'global' | 'history' | 'favorites' | 'command-palette' | 'cleanup';
}

// ═══════════════════════════════════════════════════════════════════════════════
// STATE
// ═══════════════════════════════════════════════════════════════════════════════

let pendingPrefix: string | null = null;
let prefixTimeout: ReturnType<typeof setTimeout> | null = null;
const PREFIX_TIMEOUT_MS = 1000;

// ═══════════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════════

function getDomain(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return url;
  }
}

function getCurrentItem(): HistoryEntry | Favorite | null {
  const list = homepageState.currentList;
  const idx = homepageState.focusedIndex;
  return list[idx] ?? null;
}

function clearPendingPrefix(): void {
  pendingPrefix = null;
  homepageState.setPendingKeyPrefix(null);
  if (prefixTimeout) {
    clearTimeout(prefixTimeout);
    prefixTimeout = null;
  }
}

function setPendingPrefix(prefix: string): void {
  pendingPrefix = prefix;
  homepageState.setPendingKeyPrefix(prefix);
  if (prefixTimeout) {
    clearTimeout(prefixTimeout);
  }
  prefixTimeout = setTimeout(clearPendingPrefix, PREFIX_TIMEOUT_MS);
}

// ═══════════════════════════════════════════════════════════════════════════════
// ACTIONS
// ═══════════════════════════════════════════════════════════════════════════════

async function openCurrentItem(): Promise<void> {
  const item = getCurrentItem();
  if (item && 'url' in item) {
    navigateTo(item.url);
  }
}

async function deleteCurrentHistoryEntry(): Promise<void> {
  if (homepageState.activePanel !== 'history') return;

  const item = getCurrentItem() as HistoryEntry | null;
  if (!item) return;

  await deleteHistoryEntry(item.id);

  // Adjust focus if needed
  const newLength = homepageState.currentListLength;
  if (homepageState.focusedIndex >= newLength) {
    homepageState.setFocusedIndex(Math.max(0, newLength - 1));
  }
}

async function handleCleanupShortcut(range: HistoryCleanupRange): Promise<void> {
  if (homepageState.activePanel !== 'history') return;

  const messages: Record<HistoryCleanupRange, string> = {
    hour: 'Clear history from the last hour?',
    day: 'Clear history from the last day?',
    week: 'Clear history from the last week?',
    month: 'Clear history from the last month?',
    all: 'Clear ALL history? This cannot be undone.',
  };

  homepageState.showConfirm(messages[range], async () => {
    if (range === 'all') {
      await clearAllHistory();
    } else {
      await deleteHistoryByRange(range);
    }
  });
}

async function clearCurrentDomain(): Promise<void> {
  if (homepageState.activePanel !== 'history') return;

  const item = getCurrentItem() as HistoryEntry | null;
  if (!item) return;

  const domain = getDomain(item.url);
  homepageState.showConfirm(
    `Clear all history for ${domain}?`,
    async () => {
      await deleteHistoryByDomain(domain);
    }
  );
}

async function handleQuickShortcut(key: number): Promise<void> {
  const favorite = await getFavoriteByShortcut(key);
  if (favorite) {
    navigateTo(favorite.url);
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// KEY BINDINGS
// ═══════════════════════════════════════════════════════════════════════════════

const globalBindings: KeyBinding[] = [
  // Command palette
  {
    key: 'p',
    ctrl: true,
    description: 'Open command palette',
    action: () => homepageState.openCommandPalette(),
  },
  // Note: "/" may require Shift on some keyboard layouts (e.g., AZERTY)
  // We handle it specially in handleKeyDown to ignore Shift state
  {
    key: '/',
    description: 'Open command palette / search',
    action: () => homepageState.openCommandPalette(),
  },

  // Quick navigation (1-9)
  ...Array.from({ length: 9 }, (_, i) => ({
    key: String(i + 1),
    description: `Navigate to favorite shortcut ${i + 1}`,
    action: () => handleQuickShortcut(i + 1),
  })),

  // Panel navigation (g prefix)
  {
    key: 'h',
    description: 'Go to History panel',
    action: () => {
      if (pendingPrefix === 'g') {
        homepageState.setActivePanel('history');
        clearPendingPrefix();
      }
    },
    context: 'global',
  },
  {
    key: 'f',
    description: 'Go to Favorites panel',
    action: () => {
      if (pendingPrefix === 'g') {
        homepageState.setActivePanel('favorites');
        clearPendingPrefix();
      }
    },
    context: 'global',
  },
  {
    key: 'a',
    description: 'Go to Analytics panel',
    action: () => {
      if (pendingPrefix === 'g') {
        homepageState.setActivePanel('analytics');
        clearPendingPrefix();
      }
    },
    context: 'global',
  },

  // Escape
  {
    key: 'Escape',
    description: 'Close modals / clear search',
    action: () => {
      clearPendingPrefix();
      if (homepageState.commandPaletteOpen) {
        homepageState.closeCommandPalette();
      } else if (homepageState.confirmModalOpen) {
        homepageState.cancelConfirm();
      } else if (homepageState.cleanupModalOpen) {
        homepageState.closeCleanupModal();
      } else if (homepageState.historySearchQuery) {
        homepageState.setHistorySearchQuery('');
      }
    },
  },

  // Tab - cycle through panels
  {
    key: 'Tab',
    description: 'Next panel',
    action: () => {
      const panels = ['history', 'favorites', 'analytics'] as const;
      const currentIdx = panels.indexOf(homepageState.activePanel);
      const nextIdx = (currentIdx + 1) % panels.length;
      const nextPanel = panels[nextIdx];
      if (nextPanel) {
        homepageState.setActivePanel(nextPanel);
      }
    },
  },
  {
    key: 'Tab',
    shift: true,
    description: 'Previous panel',
    action: () => {
      const panels = ['history', 'favorites', 'analytics'] as const;
      const currentIdx = panels.indexOf(homepageState.activePanel);
      const prevIdx = (currentIdx - 1 + panels.length) % panels.length;
      const prevPanel = panels[prevIdx];
      if (prevPanel) {
        homepageState.setActivePanel(prevPanel);
      }
    },
  },
];

const listBindings: KeyBinding[] = [
  // Navigation
  {
    key: 'j',
    description: 'Move down',
    action: () => homepageState.focusNext(),
  },
  {
    key: 'ArrowDown',
    description: 'Move down',
    action: () => homepageState.focusNext(),
  },
  {
    key: 'k',
    description: 'Move up',
    action: () => homepageState.focusPrev(),
  },
  {
    key: 'ArrowUp',
    description: 'Move up',
    action: () => homepageState.focusPrev(),
  },
  {
    key: 'n',
    ctrl: true,
    description: 'Move down',
    action: () => homepageState.focusNext(),
  },
  {
    key: 'p',
    ctrl: true,
    description: 'Move up',
    action: () => homepageState.focusPrev(),
  },
  {
    key: 'g',
    description: 'Go to first / prefix for panel nav',
    action: () => {
      if (pendingPrefix === 'g') {
        homepageState.focusFirst();
        clearPendingPrefix();
      } else {
        setPendingPrefix('g');
      }
    },
  },
  {
    key: 'G',
    shift: true,
    description: 'Go to last',
    action: () => homepageState.focusLast(),
  },

  // Selection
  {
    key: 'Enter',
    description: 'Open selected item',
    action: openCurrentItem,
  },
  {
    key: 'o',
    description: 'Open selected item',
    action: openCurrentItem,
  },
];

const historyBindings: KeyBinding[] = [
  // Delete
  {
    key: 'd',
    description: 'Delete entry / prefix for cleanup',
    action: () => {
      if (pendingPrefix === 'D') {
        // D d - clear last day
        handleCleanupShortcut('day');
        clearPendingPrefix();
      } else {
        deleteCurrentHistoryEntry();
      }
    },
    context: 'history',
  },
  {
    key: 'x',
    description: 'Delete entry',
    action: deleteCurrentHistoryEntry,
    context: 'history',
  },

  // Cleanup shortcuts (D prefix)
  {
    key: 'D',
    shift: true,
    description: 'Cleanup prefix / Clear ALL',
    action: () => {
      if (pendingPrefix === 'D') {
        // D D - clear all
        handleCleanupShortcut('all');
        clearPendingPrefix();
      } else {
        setPendingPrefix('D');
      }
    },
    context: 'history',
  },
  {
    key: 'h',
    description: 'Clear last hour',
    action: () => {
      if (pendingPrefix === 'D') {
        handleCleanupShortcut('hour');
        clearPendingPrefix();
      }
    },
    context: 'history',
  },
  {
    key: 'w',
    description: 'Clear last week',
    action: () => {
      if (pendingPrefix === 'D') {
        handleCleanupShortcut('week');
        clearPendingPrefix();
      }
    },
    context: 'history',
  },
  {
    key: 'm',
    description: 'Clear last month',
    action: () => {
      if (pendingPrefix === 'D') {
        handleCleanupShortcut('month');
        clearPendingPrefix();
      }
    },
    context: 'history',
  },
  {
    key: '@',
    shift: true,
    description: 'Clear current domain',
    action: () => {
      if (pendingPrefix === 'D') {
        clearCurrentDomain();
        clearPendingPrefix();
      }
    },
    context: 'history',
  },
];

const commandPaletteBindings: KeyBinding[] = [
  {
    key: 'j',
    ctrl: true,
    description: 'Move down',
    action: () => homepageState.focusNext(),
    context: 'command-palette',
  },
  {
    key: 'n',
    ctrl: true,
    description: 'Move down',
    action: () => homepageState.focusNext(),
    context: 'command-palette',
  },
  {
    key: 'ArrowDown',
    description: 'Move down',
    action: () => homepageState.focusNext(),
    context: 'command-palette',
  },
  {
    key: 'k',
    ctrl: true,
    description: 'Move up',
    action: () => homepageState.focusPrev(),
    context: 'command-palette',
  },
  {
    key: 'p',
    ctrl: true,
    description: 'Move up',
    action: () => homepageState.focusPrev(),
    context: 'command-palette',
  },
  {
    key: 'ArrowUp',
    description: 'Move up',
    action: () => homepageState.focusPrev(),
    context: 'command-palette',
  },
  {
    key: 'Enter',
    description: 'Execute selected command',
    action: () => {
      // This will be handled by the command palette component
    },
    context: 'command-palette',
  },
];

// ═══════════════════════════════════════════════════════════════════════════════
// KEY HANDLER
// ═══════════════════════════════════════════════════════════════════════════════

function matchBinding(e: KeyboardEvent, binding: KeyBinding): boolean {
  if (binding.key !== e.key) return false;
  if (!!binding.ctrl !== e.ctrlKey) return false;
  if (!!binding.shift !== e.shiftKey) return false;
  if (!!binding.alt !== e.altKey) return false;
  if (!!binding.meta !== e.metaKey) return false;
  return true;
}

function getActiveBindings(): KeyBinding[] {
  const bindings: KeyBinding[] = [...globalBindings];

  if (homepageState.commandPaletteOpen) {
    bindings.push(...commandPaletteBindings);
  } else {
    bindings.push(...listBindings);

    switch (homepageState.activePanel) {
      case 'history':
        bindings.push(...historyBindings);
        break;
      case 'favorites':
        // Add favorites-specific bindings here
        break;
    }
  }

  return bindings;
}

export function handleKeyDown(e: KeyboardEvent): void {
  // Skip if focused on input/textarea (except for navigation keys)
  const target = e.target as HTMLElement;
  const isInput = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA';

  // Special handling for "/" - opens command palette from anywhere
  // Ignore modifier keys since "/" may require Shift on some layouts (AZERTY)
  if (e.key === '/') {
    e.preventDefault();
    e.stopPropagation();
    if (isInput) {
      (target as HTMLInputElement).blur();
    }
    homepageState.openCommandPalette();
    return;
  }

  if (isInput) {
    // Allow specific keys when in input
    const allowedInInput = ['Escape', 'ArrowUp', 'ArrowDown', 'Enter'];
    const ctrlAllowed = ['j', 'k', 'n', 'p'];

    if (!allowedInInput.includes(e.key) && !(e.ctrlKey && ctrlAllowed.includes(e.key))) {
      return;
    }
  }

  const bindings = getActiveBindings();

  for (const binding of bindings) {
    if (matchBinding(e, binding)) {
      // Check context
      if (binding.context) {
        switch (binding.context) {
          case 'history':
            if (homepageState.activePanel !== 'history') continue;
            break;
          case 'favorites':
            if (homepageState.activePanel !== 'favorites') continue;
            break;
          case 'command-palette':
            if (!homepageState.commandPaletteOpen) continue;
            break;
        }
      }

      e.preventDefault();
      e.stopPropagation();
      binding.action();
      return;
    }
  }

  // Handle 'g' prefix standalone
  if (e.key === 'g' && !e.ctrlKey && !e.shiftKey && !e.altKey && !e.metaKey && !isInput) {
    if (!pendingPrefix) {
      e.preventDefault();
      setPendingPrefix('g');
    }
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// TIERED SEARCH (Client-side first, then FTS)
// ═══════════════════════════════════════════════════════════════════════════════

let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;
const SEARCH_DEBOUNCE_MS = 400;
const MIN_FTS_QUERY_LENGTH = 3;

/**
 * Fuzzy match score calculator.
 * Supports words in any order (fzf-style).
 * Returns score > 0 if match, 0 if no match. Higher = better.
 */
function fuzzyScore(text: string, query: string): number {
  const lowerText = text.toLowerCase();
  const lowerQuery = query.toLowerCase();

  // Split query into words for "any order" matching
  const queryWords = lowerQuery.split(/\s+/).filter(w => w.length > 0);
  if (queryWords.length === 0) return 0;

  let totalScore = 0;

  for (const word of queryWords) {
    // Exact substring match (highest score)
    const exactIdx = lowerText.indexOf(word);
    if (exactIdx !== -1) {
      // Bonus for word boundary match (start of text or after space/punctuation)
      const atBoundary = exactIdx === 0 || /[\s\-_./]/.test(lowerText[exactIdx - 1] || '');
      totalScore += 100 + (atBoundary ? 50 : 0) + (word.length * 10);
      continue;
    }

    // Fuzzy: chars in sequence (lower score)
    let qi = 0;
    let lastMatchIdx = -1;
    let consecutiveBonus = 0;

    for (let i = 0; i < lowerText.length && qi < word.length; i++) {
      if (lowerText[i] === word[qi]) {
        // Bonus for consecutive matches
        if (lastMatchIdx === i - 1) consecutiveBonus += 5;
        lastMatchIdx = i;
        qi++;
      }
    }

    if (qi === word.length) {
      // All chars found in order
      totalScore += 30 + consecutiveBonus;
    } else {
      // This query word not found - no match
      return 0;
    }
  }

  return totalScore;
}

interface ScoredEntry {
  entry: HistoryEntry;
  score: number;
}

/**
 * Filter history entries client-side using fuzzy matching.
 * Combines title + URL so words can match across both fields.
 * E.g. "google search" matches title "Google" + URL containing "search"
 */
function filterHistoryLocal(query: string): HistoryEntry[] {
  const q = query.trim();
  if (!q) return [];

  const scored: ScoredEntry[] = [];

  for (const entry of homepageState.history) {
    const title = entry.title || '';
    const url = entry.url;

    // Combine title and URL for cross-field word matching
    // Title gets priority (appears first), URL stripped of protocol
    const urlClean = url.replace(/^https?:\/\//, '').replace(/\/$/, '');
    const combined = `${title} ${urlClean}`;

    const score = fuzzyScore(combined, q);

    // Boost score if title matches directly (more relevant)
    const titleBonus = fuzzyScore(title, q) > 0 ? 50 : 0;

    if (score > 0) {
      scored.push({ entry, score: score + titleBonus });
    }
  }

  // Sort by score descending, limit results
  scored.sort((a, b) => b.score - a.score);
  return scored.slice(0, 50).map(s => s.entry);
}

export function handleSearchInput(query: string): void {
  homepageState.setHistorySearchQuery(query);

  if (searchDebounceTimer) {
    clearTimeout(searchDebounceTimer);
    searchDebounceTimer = null;
  }

  const trimmed = query.trim();
  if (!trimmed) {
    homepageState.setHistorySearchResults([]);
    return;
  }

  // Tier 1: Instant client-side fuzzy filter on cached history
  const localResults = filterHistoryLocal(trimmed);
  homepageState.setHistorySearchResults(localResults);

  // Tier 2: If local returns 0 results AND query is long enough, try FTS
  // Also try FTS after debounce if query is substantial (deeper search)
  if (trimmed.length >= MIN_FTS_QUERY_LENGTH) {
    searchDebounceTimer = setTimeout(() => {
      // Only call FTS if local results are sparse
      if (localResults.length < 5) {
        searchHistoryFTS(trimmed);
      }
    }, SEARCH_DEBOUNCE_MS);
  }
}

// ═══════════════════════════════════════════════════════════════════════════════
// KEYBOARD HINTS FOR UI
// ═══════════════════════════════════════════════════════════════════════════════

export interface KeyboardHint {
  keys: string[];
  description: string;
}

export function getContextualHints(): KeyboardHint[] {
  const hints: KeyboardHint[] = [];

  if (homepageState.commandPaletteOpen) {
    hints.push(
      { keys: ['Ctrl+j', '↓'], description: 'Move down' },
      { keys: ['Ctrl+k', '↑'], description: 'Move up' },
      { keys: ['Enter'], description: 'Execute' },
      { keys: ['Escape'], description: 'Close' }
    );
  } else {
    hints.push(
      { keys: ['j', '↓'], description: 'Move down' },
      { keys: ['k', '↑'], description: 'Move up' },
      { keys: ['Enter', 'o'], description: 'Open' },
      { keys: ['/'], description: 'Search' }
    );

    if (homepageState.activePanel === 'history') {
      hints.push(
        { keys: ['d', 'x'], description: 'Delete' },
        { keys: ['D h'], description: 'Clear hour' },
        { keys: ['D d'], description: 'Clear day' },
        { keys: ['D D'], description: 'Clear all' }
      );
    }

    hints.push(
      { keys: ['g h'], description: 'History' },
      { keys: ['g f'], description: 'Favorites' },
      { keys: ['g a'], description: 'Analytics' },
      { keys: ['1-9'], description: 'Quick nav' }
    );
  }

  return hints;
}

// ═══════════════════════════════════════════════════════════════════════════════
// INITIALIZATION
// ═══════════════════════════════════════════════════════════════════════════════

export function initializeKeyboard(): () => void {
  document.addEventListener('keydown', handleKeyDown);

  return () => {
    document.removeEventListener('keydown', handleKeyDown);
    clearPendingPrefix();
    if (searchDebounceTimer) {
      clearTimeout(searchDebounceTimer);
    }
  };
}
