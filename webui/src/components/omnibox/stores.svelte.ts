/**
 * Omnibox State Management
 *
 * Svelte 5 rune-based state management for omnibox functionality
 */

import type {
  OmniboxMode,
  ViewMode,
  Suggestion,
  Favorite,
  FindMatch,
  HighlightNode,
  OmniboxConfig,
  SearchShortcut,
} from "./types";

// Default configuration
const DEFAULT_CONFIG: Required<OmniboxConfig> = {
  maxMatches: 2000,
  debounceDelay: 120,
  defaultLimit: 10,
};

// Load viewMode from localStorage or default to "history"
function loadViewMode(): ViewMode {
  try {
    const stored = localStorage.getItem("dumber_omnibox_viewMode");
    if (stored === "favorites" || stored === "history") {
      return stored;
    }
  } catch (e) {
    console.warn("Failed to load viewMode from localStorage:", e);
  }
  return "history";
}

// Global state using Svelte 5 runes
let visible = $state(false);
let mode = $state<OmniboxMode>("omnibox");
let viewMode = $state<ViewMode>(loadViewMode());
let suggestions = $state<Suggestion[]>([]);
let favorites = $state<Favorite[]>([]);
let matches = $state<FindMatch[]>([]);
let selectedIndex = $state(-1);
let activeIndex = $state(-1);
let faded = $state(false);
let inputValue = $state("");
let highlightNodes = $state<HighlightNode[]>([]);
let prevOverflow = $state("");
let debounceTimer = $state(0);
let searchShortcuts = $state<Record<string, SearchShortcut>>({});

// Inline suggestion state (fish-style ghost text)
let inlineSuggestion = $state<string | null>(null);
let inlineCompletion = $state<string | null>(null);

// Configuration
let config = $state<Required<OmniboxConfig>>(DEFAULT_CONFIG);

export const omniboxStore = {
  // State getters
  get visible() {
    return visible;
  },
  get mode() {
    return mode;
  },
  get viewMode() {
    return viewMode;
  },
  get suggestions() {
    return suggestions;
  },
  get favorites() {
    return favorites;
  },
  get matches() {
    return matches;
  },
  get selectedIndex() {
    return selectedIndex;
  },
  get activeIndex() {
    return activeIndex;
  },
  get faded() {
    return faded;
  },
  get inputValue() {
    return inputValue;
  },
  get highlightNodes() {
    return highlightNodes;
  },
  get prevOverflow() {
    return prevOverflow;
  },
  get config() {
    return config;
  },
  get searchShortcuts() {
    return searchShortcuts;
  },
  get inlineSuggestion() {
    return inlineSuggestion;
  },
  get inlineCompletion() {
    return inlineCompletion;
  },

  // Computed getters
  get hasContent() {
    if (mode === "find") {
      return matches.length > 0;
    }
    // In omnibox mode, check the current view
    return viewMode === "history" ? suggestions.length > 0 : favorites.length > 0;
  },
  get totalItems() {
    if (mode === "find") {
      return matches.length;
    }
    // In omnibox mode, return count based on current view
    return viewMode === "history" ? suggestions.length : favorites.length;
  },
  get selectedItem() {
    if (selectedIndex < 0) return null;
    if (mode === "find") {
      return matches[selectedIndex];
    }
    // In omnibox mode, return item from current view
    return viewMode === "history"
      ? suggestions[selectedIndex]
      : favorites[selectedIndex];
  },

  // Actions
  setVisible(value: boolean) {
    if (value && !visible) {
      // Store scroll state when opening
      prevOverflow = document.documentElement.style.overflow;
      document.documentElement.style.overflow = "hidden";
    } else if (!value && visible) {
      // Restore scroll state when closing
      document.documentElement.style.overflow = prevOverflow || "";
      prevOverflow = "";

      // Clear find highlights when closing
      if (mode === "find") {
        this.clearHighlights();
      }
    }
    visible = value;
  },

  setMode(newMode: OmniboxMode) {
    // Clear previous mode's state
    if (newMode === "omnibox") {
      // Switching to omnibox mode - clear find state
      this.clearHighlights();
    } else if (newMode === "find") {
      // Switching to find mode - clear suggestions
      this.updateSuggestions([]);
    }

    mode = newMode;
    selectedIndex = -1;
    faded = false;
    inputValue = ""; // Clear input for fresh start
  },

  setInputValue(value: string) {
    inputValue = value;
  },

  setFaded(value: boolean) {
    faded = value;
  },

  setSelectedIndex(index: number) {
    const maxIndex = this.totalItems - 1;
    if (maxIndex < 0) {
      selectedIndex = -1;
    } else {
      selectedIndex = Math.max(0, Math.min(index, maxIndex));
    }
  },

  selectNext() {
    if (this.totalItems === 0) return;
    const newIndex =
      selectedIndex < 0 ? 0 : (selectedIndex + 1) % this.totalItems;
    this.setSelectedIndex(newIndex);
    faded = true;
  },

  selectPrevious() {
    if (this.totalItems === 0) return;
    const newIndex =
      selectedIndex <= 0 ? this.totalItems - 1 : selectedIndex - 1;
    this.setSelectedIndex(newIndex);
    faded = true;
  },

  open(newMode: OmniboxMode = "omnibox", initialQuery?: string) {
    this.setMode(newMode);
    this.setVisible(true);
    if (typeof initialQuery === "string") {
      inputValue = initialQuery;
    }
  },

  close() {
    this.setVisible(false);
    faded = false;
  },

  toggle() {
    if (!visible) {
      // When opening via toggle (Ctrl+L), always open in omnibox mode
      this.setMode("omnibox");
      this.setVisible(true);
    } else {
      this.setVisible(false);
    }
  },

  updateSuggestions(newSuggestions: Suggestion[]) {
    suggestions = Array.isArray(newSuggestions) ? newSuggestions : [];
    selectedIndex = -1;
  },

  updateMatches(newMatches: FindMatch[]) {
    matches = Array.isArray(newMatches) ? newMatches : [];
    selectedIndex = newMatches.length > 0 ? 0 : -1;
  },

  clearHighlights() {
    try {
      highlightNodes.forEach(({ span, text }) => {
        const parent = span.parentNode;
        if (parent) {
          parent.replaceChild(text, span);
          parent.normalize();
        }
      });
    } catch (error) {
      console.warn("Error clearing highlights:", error);
    }
    highlightNodes = [];
    matches = [];
    selectedIndex = -1;
    activeIndex = -1;
  },

  addHighlightNode(node: HighlightNode) {
    highlightNodes = [...highlightNodes, node];
  },

  setActiveIndex(index: number) {
    activeIndex = index;
  },

  setDebounceTimer(timerId: number) {
    if (debounceTimer) {
      clearTimeout(debounceTimer);
    }
    debounceTimer = timerId;
  },

  clearDebounceTimer() {
    if (debounceTimer) {
      clearTimeout(debounceTimer);
      debounceTimer = 0;
    }
  },

  updateConfig(newConfig: Partial<OmniboxConfig>) {
    config = { ...config, ...newConfig };
  },

  updateSearchShortcuts(shortcuts: Record<string, SearchShortcut>) {
    searchShortcuts = shortcuts;
  },

  setViewMode(newViewMode: ViewMode) {
    viewMode = newViewMode;
    selectedIndex = -1; // Reset selection when switching views

    // Persist to localStorage
    try {
      localStorage.setItem("dumber_omnibox_viewMode", newViewMode);
    } catch (e) {
      console.warn("Failed to save viewMode to localStorage:", e);
    }
  },

  updateFavorites(newFavorites: Favorite[]) {
    favorites = Array.isArray(newFavorites) ? newFavorites : [];
    // Reset selection if we're in favorites view
    if (viewMode === "favorites") {
      selectedIndex = -1;
    }
  },

  setInlineSuggestion(url: string | null, currentInput: string) {
    console.log('[INLINE] setInlineSuggestion called:', { url, currentInput });
    inlineSuggestion = url;
    if (!url || !currentInput) {
      inlineCompletion = null;
      console.log('[INLINE] Cleared - missing url or input');
      return;
    }

    // Normalize URL by stripping protocol and www for matching
    let normalizedUrl = url.toLowerCase();
    normalizedUrl = normalizedUrl.replace(/^https?:\/\//, '');
    normalizedUrl = normalizedUrl.replace(/^www\./, '');

    const normalizedInput = currentInput.toLowerCase();

    if (normalizedUrl.startsWith(normalizedInput)) {
      // Show the completion (rest of normalized URL after what user typed)
      inlineCompletion = normalizedUrl.slice(normalizedInput.length);
      console.log('[INLINE] Set completion:', inlineCompletion);
    } else {
      inlineCompletion = null;
      console.log('[INLINE] Cleared - no prefix match after normalization');
    }
  },

  clearInlineSuggestion() {
    inlineSuggestion = null;
    inlineCompletion = null;
  },

  reset() {
    visible = false;
    mode = "omnibox";
    // Keep viewMode persistent - it's user preference
    suggestions = [];
    favorites = [];
    matches = [];
    selectedIndex = -1;
    activeIndex = -1;
    faded = false;
    inputValue = "";
    prevOverflow = "";
    inlineSuggestion = null;
    inlineCompletion = null;
    this.clearHighlights();
    this.clearDebounceTimer();
  },
};
