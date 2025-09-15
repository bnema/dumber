type HistoryEntry = {
  id: number;
  url: string;
  title: string;
  visit_count: number;
  last_visited: string | null;
  created_at: string | null;
}

export class AppService {
  private history: HistoryEntry[] = [];
  private shortcuts: Record<string, any> = {};

  constructor() {}

  async initialize(): Promise<void> {
    try {
      await Promise.all([
        this.loadHistory(),
        this.loadShortcuts()
      ]);
    } catch (error) {
      console.error('Failed to initialize app:', error);
    }
  }

  async loadHistory(): Promise<void> {
    try {
      const recentHistory: HistoryEntry[] = await fetch('/api/history/recent?limit=50').then(r => r.json());
      this.history = recentHistory;
    } catch (error) {
      console.error('Failed to load history:', error);
      // Mock history for development/fallback
      this.history = [
        { 
          id: 1, 
          url: 'https://github.com/wailsapp/wails', 
          title: 'Wails Framework', 
          visit_count: 3,
          last_visited: new Date().toISOString(),
          created_at: new Date().toISOString()
        },
        { 
          id: 2, 
          url: 'https://go.dev', 
          title: 'The Go Programming Language', 
          visit_count: 2,
          last_visited: new Date().toISOString(),
          created_at: new Date().toISOString()
        },
        { 
          id: 3, 
          url: 'https://developer.mozilla.org', 
          title: 'MDN Web Docs', 
          visit_count: 1,
          last_visited: new Date().toISOString(),
          created_at: new Date().toISOString()
        }
      ];
    }
  }

  async loadShortcuts(): Promise<void> {
    try {
      const cfg = await fetch('/api/config').then(r => r.json());
      const raw = cfg?.search_shortcuts || {};
      // Normalize field casing from backend (supports URL/Description and url/description)
      const normalized: Record<string, { description: string; url: string }> = {};
      for (const [key, value] of Object.entries(raw)) {
        const v: any = value as any;
        normalized[key] = {
          description: v.description ?? v.Description ?? '',
          url: v.url ?? v.URL ?? ''
        };
      }
      this.shortcuts = normalized;
    } catch (error) {
      console.error('Failed to load shortcuts:', error);
      // Mock shortcuts for development/fallback
      this.shortcuts = {
        'g': { description: 'Google Search', url: 'https://google.com/search?q={query}' },
        'gh': { description: 'GitHub Search', url: 'https://github.com/search?q={query}' },
        'so': { description: 'Stack Overflow', url: 'https://stackoverflow.com/search?q={query}' },
        'w': { description: 'Wikipedia', url: 'https://en.wikipedia.org/wiki/Special:Search?search={query}' }
      };
    }
  }

  getHistory(): HistoryEntry[] {
    return this.history;
  }

  getShortcuts(): Record<string, any> {
    return this.shortcuts;
  }

  async copyToClipboard(text: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      console.log('Copied to clipboard:', text);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  }
}
