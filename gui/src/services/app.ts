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
        },
        {
          id: 2,
          url: "https://go.dev",
          title: "The Go Programming Language",
          visit_count: 2,
          last_visited: new Date().toISOString(),
          created_at: new Date().toISOString(),
        },
        {
          id: 3,
          url: "https://developer.mozilla.org",
          title: "MDN Web Docs",
          visit_count: 1,
          last_visited: new Date().toISOString(),
          created_at: new Date().toISOString(),
        },
      ];
    }
  }

  async loadShortcuts(): Promise<void> {
    try {
      const cfg = await fetch("/api/config").then((r) => r.json());
      const raw = cfg?.search_shortcuts || {};
      // Normalize field casing from backend (supports URL/Description and url/description)
      const normalized: Record<string, SearchShortcut> = {};
      for (const [key, value] of Object.entries(raw)) {
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
    } catch (error) {
      console.error("Failed to load shortcuts:", error);
      // Mock shortcuts for development/fallback
      this.shortcuts = {
        g: {
          id: 0,
          shortcut: "g",
          url_template: "https://google.com/search?q={query}",
          description: "Google Search",
          created_at: null,
        },
        gh: {
          id: 0,
          shortcut: "gh",
          url_template: "https://github.com/search?q={query}",
          description: "GitHub Search",
          created_at: null,
        },
        so: {
          id: 0,
          shortcut: "so",
          url_template: "https://stackoverflow.com/search?q={query}",
          description: "Stack Overflow",
          created_at: null,
        },
        w: {
          id: 0,
          shortcut: "w",
          url_template:
            "https://en.wikipedia.org/wiki/Special:Search?search={query}",
          description: "Wikipedia",
          created_at: null,
        },
      };
    }
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
