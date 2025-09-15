/**
 * Omnibox State Management
 *
 * Svelte 5 rune-based state management for omnibox functionality
 */

import type {
  OmniboxMode,
  Suggestion,
  FindMatch,
  HighlightNode,
  OmniboxConfig
} from './types';

// Default configuration
const DEFAULT_CONFIG: Required<OmniboxConfig> = {
  maxMatches: 2000,
  debounceDelay: 120,
  defaultLimit: 10
};

// Global state using Svelte 5 runes
let visible = $state(false);
let mode = $state<OmniboxMode>('omnibox');
let suggestions = $state<Suggestion[]>([]);
let matches = $state<FindMatch[]>([]);
let selectedIndex = $state(-1);
let activeIndex = $state(-1);
let faded = $state(false);
let inputValue = $state('');
let highlightNodes = $state<HighlightNode[]>([]);
let prevOverflow = $state('');
let debounceTimer = $state(0);

// Configuration
let config = $state<Required<OmniboxConfig>>(DEFAULT_CONFIG);

export const omniboxStore = {
  // State getters
  get visible() { return visible; },
  get mode() { return mode; },
  get suggestions() { return suggestions; },
  get matches() { return matches; },
  get selectedIndex() { return selectedIndex; },
  get activeIndex() { return activeIndex; },
  get faded() { return faded; },
  get inputValue() { return inputValue; },
  get highlightNodes() { return highlightNodes; },
  get prevOverflow() { return prevOverflow; },
  get config() { return config; },

  // Computed getters
  get hasContent() {
    return (mode === 'omnibox' && suggestions.length > 0) ||
           (mode === 'find' && matches.length > 0);
  },
  get totalItems() {
    return mode === 'omnibox' ? suggestions.length : matches.length;
  },
  get selectedItem() {
    if (selectedIndex < 0) return null;
    return mode === 'omnibox' ? suggestions[selectedIndex] : matches[selectedIndex];
  },

  // Actions
  setVisible(value: boolean) {
    if (value && !visible) {
      // Store scroll state when opening
      prevOverflow = document.documentElement.style.overflow;
      document.documentElement.style.overflow = 'hidden';
    } else if (!value && visible) {
      // Restore scroll state when closing
      document.documentElement.style.overflow = prevOverflow || '';
      prevOverflow = '';

      // Clear find highlights when closing
      if (mode === 'find') {
        this.clearHighlights();
      }
    }
    visible = value;
  },

  setMode(newMode: OmniboxMode) {
    // Clear previous mode's state
    if (newMode === 'omnibox') {
      // Switching to omnibox mode - clear find state
      this.clearHighlights();
    } else if (newMode === 'find') {
      // Switching to find mode - clear suggestions
      this.updateSuggestions([]);
    }

    mode = newMode;
    selectedIndex = -1;
    faded = false;
    inputValue = ''; // Clear input for fresh start
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
    const newIndex = selectedIndex < 0 ? 0 : (selectedIndex + 1) % this.totalItems;
    this.setSelectedIndex(newIndex);
    faded = true;
  },

  selectPrevious() {
    if (this.totalItems === 0) return;
    const newIndex = selectedIndex <= 0 ? this.totalItems - 1 : selectedIndex - 1;
    this.setSelectedIndex(newIndex);
    faded = true;
  },

  open(newMode: OmniboxMode = 'omnibox', initialQuery?: string) {
    this.setMode(newMode);
    this.setVisible(true);
    if (typeof initialQuery === 'string') {
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
      this.setMode('omnibox');
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
      console.warn('Error clearing highlights:', error);
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

  reset() {
    visible = false;
    mode = 'omnibox';
    suggestions = [];
    matches = [];
    selectedIndex = -1;
    activeIndex = -1;
    faded = false;
    inputValue = '';
    prevOverflow = '';
    this.clearHighlights();
    this.clearDebounceTimer();
  }
};