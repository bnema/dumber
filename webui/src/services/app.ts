import type { HistoryEntry, SearchShortcut } from "../types/generated.js";

export class AppService {
  private history: HistoryEntry[] = [];
  private shortcuts: Record<string, SearchShortcut> = {};

  constructor() {}

  async initialize(): Promise<void> {
    try {
      await Promise.all([this.loadHistory(), this.loadShortcuts()]);
    } catch (error) {
      console.error("Failed to initialize app:", error);
    }
  }

  async loadHistory(): Promise<void> {
    try {
      // Use message bridge instead of fetch
      return new Promise((resolve, reject) => {
        // Set up response handler
        window.__dumber_history_recent = (data: HistoryEntry[]) => {
          this.history = Array.isArray(data) ? data : [];
          resolve();
        };

        window.__dumber_history_error = (error: string) => {
          console.error("Failed to load history:", error);
          reject(new Error(error));
        };

        // Send message to Go backend
        const bridge = window.webkit?.messageHandlers?.dumber;
        if (bridge && typeof bridge.postMessage === "function") {
          bridge.postMessage(
            JSON.stringify({
              type: "history_recent",
              limit: 50,
              offset: 0,
            }),
          );
        } else {
          reject(new Error("WebKit message handler not available"));
        }
      });
    } catch (error) {
      console.error("Failed to load history:", error);
      // Mock history for development/fallback
      this.history = [
        {
          id: 1,
          url: "https://github.com/wailsapp/wails",
          title: "Wails Framework",
          visit_count: 3,
          last_visited: new Date().toISOString(),
          created_at: new Date().toISOString(),
          favicon_url: null,
        },
        {
          id: 2,
          url: "https://go.dev",
          title: "The Go Programming Language",
          visit_count: 2,
          last_visited: new Date().toISOString(),
          created_at: new Date().toISOString(),
          favicon_url: null,
        },
        {
          id: 3,
          url: "https://developer.mozilla.org",
          title: "MDN Web Docs",
          visit_count: 1,
          last_visited: new Date().toISOString(),
          created_at: new Date().toISOString(),
          favicon_url: null,
        },
      ];
    }
  }

  async loadShortcuts(): Promise<void> {
    return new Promise((resolve, reject) => {
      // Set up response handler
      window.__dumber_search_shortcuts = (data: unknown) => {
        // Type guard
        if (typeof data !== "object" || data === null) {
          reject(new Error("Invalid shortcuts data"));
          return;
        }
        const dataObj = data as Record<string, unknown>;
        // Normalize field casing from backend (supports URL/Description and url/description)
        const normalized: Record<string, SearchShortcut> = {};
        for (const [key, value] of Object.entries(dataObj)) {
          const v = value as Record<string, unknown>;
          normalized[key] = {
            id: 0,
            shortcut: key,
            url_template: (v.url ?? v.URL ?? "") as string,
            description: (v.description ?? v.Description ?? "") as string,
            created_at: null,
          };
        }
        this.shortcuts = normalized;
        resolve();
      };

      window.__dumber_search_shortcuts_error = (error: string) => {
        console.error("Failed to load shortcuts:", error);
        reject(new Error(error));
      };

      // Send message to Go backend
      const bridge = window.webkit?.messageHandlers?.dumber;
      if (bridge && typeof bridge.postMessage === "function") {
        bridge.postMessage({
          type: "get_search_shortcuts",
          webview_id: (window as any).__dumber_webview_id ?? 0,
        });
      } else {
        reject(new Error("WebKit message handler not available"));
      }
    });
  }

  getHistory(): HistoryEntry[] {
    return this.history;
  }

  getShortcuts(): Record<string, SearchShortcut> {
    return this.shortcuts;
  }

  async copyToClipboard(text: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      console.log("Copied to clipboard:", text);
    } catch (err) {
      console.error("Failed to copy:", err);
    }
  }
}
