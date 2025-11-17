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
  }

  // Suggestions are returned via setSuggestions() when native handler responds
}

// Singleton instance
export const omniboxBridge = new OmniboxBridge();

/**
 * Debounced query function for search input
 */
export function debouncedQuery(searchTerm: string): void {
  omniboxStore.clearDebounceTimer();

  const timerId = window.setTimeout(() => {
    omniboxBridge.query(searchTerm);
  }, omniboxStore.config.debounceDelay);

  omniboxStore.setDebounceTimer(timerId);
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
      toggle: () => void;
      open: (mode?: string, query?: string) => void;
      close: () => void;
      findQuery: (query: string) => void;
      setActive: (active: boolean) => void;
    };
  }
}
