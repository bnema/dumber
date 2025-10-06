/**
 * Omnibox Message Bridge
 *
 * TypeScript interface for Go-JavaScript communication
 */

import type { OmniboxMessage, OmniboxMessageBridge, Suggestion, SearchShortcut } from "./types";
import { omniboxStore } from "./stores.svelte.ts";

export class OmniboxBridge implements OmniboxMessageBridge {
  // Forward messages to the native WebKit message handler when available
  // Fallback: for navigate, perform direct location change
  postMessage(msg: OmniboxMessage): void {
    console.log("📤 [DEBUG] Posting message to backend:", msg);
    // NOTE: Do NOT detach postMessage from its receiver. In WKWebView,
    // postMessage must be called on the UserMessageHandler instance.
    const bridge = window.webkit?.messageHandlers?.dumber;
    if (bridge && typeof bridge.postMessage === "function") {
      try {
        console.log("📱 [DEBUG] Using webkit message handler");
        bridge.postMessage(JSON.stringify(msg));
        return;
      } catch (e) {
        console.warn(
          "postMessage to native handler failed, using fallback:",
          e,
        );
      }
    }
    console.log("⚠️ [DEBUG] No webkit bridge available");
    // Fallback navigation if no native bridge is available
    if (msg.type === "navigate" && typeof msg.url === "string" && msg.url) {
      try {
        window.location.href = msg.url;
      } catch (e) {
        console.error("Fallback navigation failed:", e);
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
   * Fetch search shortcuts from backend config
   */
  async fetchSearchShortcuts(): Promise<void> {
    try {
      const cfg = await fetch("/api/config").then((r) => r.json());
      const raw = cfg?.search_shortcuts || {};
      // Normalize to match our SearchShortcut type
      const normalized: Record<string, SearchShortcut> = {};
      for (const [key, value] of Object.entries(raw)) {
        const v = value as Record<string, unknown>;
        normalized[key] = {
          url: (v.url ?? v.URL ?? "") as string,
          description: (v.description ?? v.Description ?? "") as string,
        };
      }
      this.setSearchShortcuts(normalized);
    } catch (error) {
      console.error("Failed to fetch search shortcuts:", error);
    }
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
