/**
 * Omnibox Message Bridge
 *
 * TypeScript interface for Go-JavaScript communication
 */

import type { OmniboxMessage, OmniboxMessageBridge, Suggestion, Favorite, SearchShortcut } from "./types";
import { omniboxStore } from "./stores.svelte.ts";

export class OmniboxBridge implements OmniboxMessageBridge {
  // Forward messages via CustomEvent to main world bridge
  // The main world will forward to webkit.messageHandlers.dumber
  postMessage(msg: OmniboxMessage): void {
    console.log("📤 [DEBUG] Posting message to backend via bridge:", msg);
    try {
      // Dispatch CustomEvent that main-world bridge will listen to
      document.dispatchEvent(new CustomEvent("dumber:isolated-message", {
        detail: { payload: msg }
      }));
      console.log("✅ [DEBUG] Dispatched isolated message event");
    } catch (e) {
      console.error("❌ [DEBUG] Failed to dispatch message:", e);
      // Fallback navigation if dispatch fails
      if (msg.type === "navigate" && typeof msg.url === "string" && msg.url) {
        try {
          window.location.href = msg.url;
        } catch (navError) {
          console.error("Fallback navigation failed:", navError);
        }
      }
    }
  }

  /**
   * Update suggestions from Go backend
   */
  setSuggestions(suggestions: Suggestion[]): void {
    console.log("📝 [DEBUG] Received suggestions from backend:", suggestions);
    omniboxStore.updateSuggestions(suggestions);
  }

  /**
   * Update search shortcuts from Go backend
   */
  setSearchShortcuts(shortcuts: Record<string, SearchShortcut>): void {
    console.log("📝 [DEBUG] Received search shortcuts from backend:", shortcuts);
    omniboxStore.updateSearchShortcuts(shortcuts);
  }

  /**
   * Handle navigation request
   */
  navigate(url: string): void {
    console.log("🚀 Omnibox navigate called with:", url);
    this.postMessage({ type: "navigate", url });
  }

  /**
   * Handle search query
   */
  query(searchTerm: string, limit?: number): void {
    const q = searchTerm ?? "";
    const lim = limit || omniboxStore.config.defaultLimit;
    console.log("🔍 [DEBUG] Sending query to backend:", { q, limit: lim });
    // Send to native handler; Go will compute suggestions and call setSuggestions
    this.postMessage({ type: "query", q, limit: lim });
  }

  /**
   * Fetch initial history for empty omnibox
   */
  fetchInitialHistory(limit?: number): void {
    const lim = limit || omniboxStore.config.defaultLimit;
    console.log("[DEBUG] Fetching initial history:", { limit: lim });
    // Send to native handler; Go will compute suggestions based on config and call setSuggestions
    this.postMessage({ type: "omnibox_initial_history", limit: lim });
  }

  /**
   * Fetch search shortcuts from backend via messaging bridge
   */
  async fetchSearchShortcuts(): Promise<void> {
    return new Promise((resolve, reject) => {
      // Set up one-time response handler
      const originalCallback = (window as any).__dumber_search_shortcuts;
      (window as any).__dumber_search_shortcuts = (data: unknown) => {
        try {
          // Type guard
          if (typeof data !== "object" || data === null) {
            reject(new Error("Invalid shortcuts data"));
            return;
          }
          const dataObj = data as Record<string, unknown>;
          // Normalize to match our SearchShortcut type
          const normalized: Record<string, SearchShortcut> = {};
          for (const [key, value] of Object.entries(dataObj)) {
            const v = value as Record<string, unknown>;
            normalized[key] = {
              url: (v.url ?? v.URL ?? "") as string,
              description: (v.description ?? v.Description ?? "") as string,
            };
          }
          this.setSearchShortcuts(normalized);

          // Restore original callback if it existed
          if (originalCallback) {
            (window as any).__dumber_search_shortcuts = originalCallback;
          }

          resolve();
        } catch (error) {
          console.error("Failed to process search shortcuts:", error);
          reject(error);
        }
      };

      // Send message to Go backend
      const bridge = window.webkit?.messageHandlers?.dumber;
      if (bridge && typeof bridge.postMessage === "function") {
        bridge.postMessage(
          JSON.stringify({
            type: "get_search_shortcuts",
          }),
        );
      } else {
        reject(new Error("WebKit message handler not available"));
      }
    });
  }

  /**
   * Update favorites from Go backend
   */
  setFavorites(favorites: Favorite[]): void {
    console.log("📝 [DEBUG] Received favorites from backend:", favorites);
    omniboxStore.updateFavorites(favorites);
  }

  /**
   * Fetch favorites from backend via messaging bridge
   */
  async fetchFavorites(): Promise<void> {
    return new Promise((resolve, reject) => {
      // Set up one-time event listener for favorites response
      const handleFavorites = ((event: CustomEvent) => {
        try {
          const { favorites } = event.detail;
          if (!Array.isArray(favorites)) {
            reject(new Error("Invalid favorites data"));
            return;
          }
          console.log("📝 [DEBUG] Received favorites via CustomEvent:", favorites);
          this.setFavorites(favorites as Favorite[]);

          // Clean up event listener
          document.removeEventListener("dumber:favorites", handleFavorites as EventListener);

          resolve();
        } catch (error) {
          console.error("Failed to process favorites:", error);
          document.removeEventListener("dumber:favorites", handleFavorites as EventListener);
          reject(error);
        }
      }) as EventListener;

      // Listen for favorites event from main world bridge
      document.addEventListener("dumber:favorites", handleFavorites as EventListener);

      // Send message to Go backend via CustomEvent bridge (like other omnibox messages)
      this.postMessage({ type: "get_favorites" });

      // Timeout after 5 seconds
      setTimeout(() => {
        document.removeEventListener("dumber:favorites", handleFavorites as EventListener);
        reject(new Error("Favorites fetch timeout"));
      }, 5000);
    });
  }

  /**
   * Toggle favorite status for a URL
   */
  toggleFavorite(url: string, title: string, faviconURL: string): void {
    console.log("⭐ [DEBUG] Toggling favorite:", { url, title, faviconURL });
    this.postMessage({ type: "toggle_favorite", url, title, faviconURL });

    // Notify other parts of the app that favorites changed
    // Homepage will listen for this and refresh its favorites list
    setTimeout(() => {
      document.dispatchEvent(new CustomEvent("dumber:favorites-changed"));
    }, 100); // Small delay to let backend complete
  }

  /**
   * Handle inline suggestion from backend (fish-style ghost text)
   */
  setInlineSuggestion(url: string | null): void {
    console.log('[INLINE] bridge.setInlineSuggestion called:', url);
    const inputValue = omniboxStore.inputValue;
    omniboxStore.setInlineSuggestion(url, inputValue);
  }

  /**
   * Query for prefix-matching URL (for inline suggestions)
   */
  prefixQuery(prefix: string): void {
    console.log('[INLINE] prefixQuery called:', prefix);
    if (!prefix || prefix.trim().length < MIN_SEARCH_LENGTH) {
      omniboxStore.clearInlineSuggestion();
      return;
    }
    this.postMessage({ type: "prefix_query", q: prefix });
    console.log('[INLINE] prefix_query message sent');
  }

  // Suggestions are returned via setSuggestions() when native handler responds
}

// Singleton instance
export const omniboxBridge = new OmniboxBridge();

// Minimum characters required before triggering search operations
const MIN_SEARCH_LENGTH = 2;

// Debounce timers for different operations
const debounceTimers: Record<string, number> = {};

/**
 * Creates a debounced search function with minimum length threshold
 * Prevents freezing on first letter and rapid input
 */
function createDebouncedSearch(
  key: string,
  action: (query: string) => void,
  onClear?: () => void
): (query: string) => void {
  return (query: string) => {
    // Clear previous timer
    if (debounceTimers[key]) {
      clearTimeout(debounceTimers[key]);
      debounceTimers[key] = 0;
    }

    const trimmed = (query || "").trim();

    // Clear/reset immediately if query is empty or too short
    if (trimmed.length < MIN_SEARCH_LENGTH) {
      if (onClear) {
        onClear();
      }
      return;
    }

    // Debounce the actual search operation
    debounceTimers[key] = window.setTimeout(() => {
      action(query);
    }, omniboxStore.config.debounceDelay);
  };
}

/**
 * Debounced query function for omnibox search input
 */
export const debouncedQuery = createDebouncedSearch(
  "query",
  (searchTerm) => omniboxBridge.query(searchTerm),
  () => omniboxStore.updateSuggestions([])
);

/**
 * Debounced find function for find-in-page
 */
export function debouncedFind(query: string, findFn: (q: string) => void): void {
  createDebouncedSearch("find", findFn, () => findFn(""))(query);
}

// Debounce for inline suggestions - balanced between responsiveness and smoothness
const PREFIX_DEBOUNCE_MS = 80;
let prefixDebounceTimer = 0;

/**
 * Debounced prefix query for inline suggestions (fish-style ghost text)
 */
export function debouncedPrefixQuery(query: string): void {
  if (prefixDebounceTimer) {
    clearTimeout(prefixDebounceTimer);
    prefixDebounceTimer = 0;
  }

  const trimmed = (query || "").trim();
  if (trimmed.length < MIN_SEARCH_LENGTH) {
    // Clear after a short delay to avoid flicker
    prefixDebounceTimer = window.setTimeout(() => {
      omniboxStore.clearInlineSuggestion();
    }, 50);
    return;
  }

  prefixDebounceTimer = window.setTimeout(() => {
    omniboxBridge.prefixQuery(trimmed);
  }, PREFIX_DEBOUNCE_MS);
}

// Extend global window interface for Go bridge compatibility
declare global {
  interface Window {
    webkit?: {
      messageHandlers?: {
        dumber?: {
          postMessage: (message: string) => void;
        };
      };
    };
    // Omnibox API for Go bridge
    __dumber_omnibox?: {
      setSuggestions: (suggestions: Suggestion[]) => void;
      setInlineSuggestion: (url: string | null) => void;
      toggle: () => void;
      open: (mode?: string, query?: string) => void;
      close: () => void;
      findQuery: (query: string) => void;
      setActive: (active: boolean) => void;
    };
  }
}
