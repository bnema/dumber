<script lang="ts">
  import { onMount } from 'svelte';
  import Footer from '$components/Footer.svelte';
  // Icons disabled for investigation: removed @lucide/svelte imports

  interface HistoryItem {
    id: number;
    url: string;
    title: string;
    favicon_url?: string;
    visit_count: number;
    last_visited: string;
    created_at: string;
  }

  interface Shortcut {
    key: string;
    description: string;
  }

  interface HistoryStats {
    total_entries: number;
    recent_count: number;
    oldest_entry?: string;
    newest_entry?: string;
  }

  let historyItems = $state<HistoryItem[]>([]);
  let topVisited = $state<HistoryItem[]>([]);
  let shortcuts = $state<Shortcut[]>([]);
  let stats = $state<HistoryStats | null>(null);
  let historyLoading = $state(true);
  let topVisitedLoading = $state(true);
  let shortcutsLoading = $state(true);
  let statsLoading = $state(true);
  let deletingIds = $state<number[]>([]);
  let historyOffset = $state(0);
  let hasMoreHistory = $state(true);
  let loadingMoreHistory = $state(false);
  const MIN_VISIT_COUNT_FOR_QUICK_ACCESS = 2;
  type ThemeMode = "light" | "dark";
  const THEME_STORAGE_KEY = "dumber.theme";
  let themeMode = $state<ThemeMode>("dark");
  let themeObserver: MutationObserver | null = null;
  const INITIAL_HISTORY_DISPLAY = 15;
  let showAllHistory = $state(false);
  const PINNED_SITES_STORAGE_KEY = "dumber.pinnedSites";
  let pinnedSites = $state<Set<string>>(new Set());
  let displayedHistoryItems = $derived(
    showAllHistory ? historyItems : historyItems.slice(0, INITIAL_HISTORY_DISPLAY)
  );

  // Function for checking if an item is being deleted
  const isDeleting = (id: number) => deletingIds.includes(id);

  // Pinned sites management
  const loadPinnedSites = () => {
    try {
      const stored = localStorage.getItem(PINNED_SITES_STORAGE_KEY);
      if (stored) {
        const urls = JSON.parse(stored) as string[];
        pinnedSites = new Set(urls);
      }
    } catch (error) {
      console.warn("[homepage] Failed to load pinned sites", error);
      pinnedSites = new Set();
    }
  };

  const savePinnedSites = () => {
    try {
      const urls = Array.from(pinnedSites);
      localStorage.setItem(PINNED_SITES_STORAGE_KEY, JSON.stringify(urls));
    } catch (error) {
      console.warn("[homepage] Failed to save pinned sites", error);
    }
  };

  const isPinned = (url: string): boolean => pinnedSites.has(url);

  const togglePin = (url: string) => {
    if (pinnedSites.has(url)) {
      pinnedSites.delete(url);
    } else {
      pinnedSites.add(url);
    }
    pinnedSites = new Set(pinnedSites); // Trigger reactivity
    savePinnedSites();
  };

  // Initialize shortcuts (these are browser shortcuts, not dynamic from API)
  const initializeShortcuts = () => {
    shortcuts = [
      // Browser shortcuts
      {
        key: 'Ctrl+L',
        description: 'Focus address bar'
      },
      {
        key: 'Ctrl+F',
        description: 'Find in page'
      },
      {
        key: 'Ctrl+Shift+C',
        description: 'Copy URL'
      },
      {
        key: 'Ctrl+R / F5',
        description: 'Reload page'
      },
      {
        key: 'Ctrl+Shift+R',
        description: 'Hard reload'
      },
      {
        key: 'F12',
        description: 'DevTools'
      },
      {
        key: 'Ctrl+← / Ctrl+→',
        description: 'Navigate back/forward'
      },
      {
        key: 'Ctrl+0',
        description: 'Reset zoom'
      },
      {
        key: 'Ctrl++ / Ctrl+-',
        description: 'Zoom in/out'
      },
      // Workspace pane mode
      {
        key: 'Ctrl+P',
        description: 'Enter pane mode'
      },
      {
        key: '→ / R (pane mode)',
        description: 'Split pane right'
      },
      {
        key: '← / L (pane mode)',
        description: 'Split pane left'
      },
      {
        key: '↑ / U (pane mode)',
        description: 'Split pane up'
      },
      {
        key: '↓ / D (pane mode)',
        description: 'Split pane down'
      },
      {
        key: 'X (pane mode)',
        description: 'Close current pane'
      },
      {
        key: 'Enter (pane mode)',
        description: 'Confirm action'
      },
      {
        key: 'Escape (pane mode)',
        description: 'Exit pane mode'
      },
      // Workspace navigation
      {
        key: 'Alt+Arrow Keys',
        description: 'Navigate between panes'
      }
    ];
    shortcutsLoading = false;
  };

  // Setup message bridge callbacks (once)
  let callbacksInitialized = false;
  const pendingRequests = new Map();

  const initializeCallbacks = () => {
    if (callbacksInitialized) return;

    (window as any).__dumber_history_recent = (data: any[], requestId?: string) => {
      const request = pendingRequests.get(requestId || 'default');
      if (request) {
        request.resolve(data);
        pendingRequests.delete(requestId || 'default');
      }
    };

    (window as any).__dumber_history_error = (error: string, requestId?: string) => {
      const request = pendingRequests.get(requestId || 'default');
      if (request) {
        request.reject(new Error(error));
        pendingRequests.delete(requestId || 'default');
      }
    };

    // Omnibox suggestions callback - forward to injected script via DOM event bridge
    let _pendingSuggestions: any[] | null = null;
    (window as any).__dumber_omnibox_suggestions = (suggestions: any[]) => {
      try {
        console.log('🎯 [HOMEPAGE] Received omnibox suggestions:', suggestions?.length || 0, 'items');
        // Dispatch an event that injected-world listener can consume
        const evt = new CustomEvent('dumber:omnibox-suggestions', {
          detail: { suggestions }
        });
        document.dispatchEvent(evt);
        _pendingSuggestions = null; // clear cache on successful dispatch
      } catch (err) {
        console.warn('⚠️ [HOMEPAGE] Failed to dispatch suggestions event, caching:', err);
        _pendingSuggestions = suggestions;
      }
    };

    // If omnibox gets ready later, re-send any pending suggestions
    document.addEventListener('dumber:omnibox-ready', () => {
      if (_pendingSuggestions && Array.isArray(_pendingSuggestions)) {
        try {
          const evt = new CustomEvent('dumber:omnibox-suggestions', {
            detail: { suggestions: _pendingSuggestions }
          });
          document.dispatchEvent(evt);
          _pendingSuggestions = null;
          console.log('🔄 [HOMEPAGE] Re-dispatched pending suggestions after omnibox-ready');
        } catch (e) {
          console.warn('⚠️ [HOMEPAGE] Failed to re-dispatch pending suggestions:', e);
        }
      }
    });

    callbacksInitialized = true;
  };

  const sendHistoryRequest = (type: string, params: any): Promise<any[]> => {
    initializeCallbacks();

    const requestId = `${type}_${Date.now()}_${Math.random()}`;

    return new Promise((resolve, reject) => {
      pendingRequests.set(requestId, { resolve, reject });

      // Timeout after 10 seconds
      setTimeout(() => {
        if (pendingRequests.has(requestId)) {
          pendingRequests.delete(requestId);
          reject(new Error('Request timeout'));
        }
      }, 10000);

      try {
        const bridge = (window as any).webkit?.messageHandlers?.dumber;
        if (bridge && typeof bridge.postMessage === 'function') {
          bridge.postMessage(JSON.stringify({
            ...params,
            type,
            requestId
          }));
        } else {
          reject(new Error('WebKit message handler not available'));
        }
      } catch (error) {
        pendingRequests.delete(requestId);
        reject(error);
      }
    });
  };

  // Fetch recent history via message bridge
  const fetchHistory = async () => {
    try {
      const data = await sendHistoryRequest('history_recent', {
        limit: 20,
        offset: 0
      });

      historyItems = Array.isArray(data) ? data : [];
      historyOffset = historyItems.length;
      hasMoreHistory = data.length === 20;
    } catch (error) {
      console.error('Error fetching history:', error);
      historyItems = [];
      hasMoreHistory = false;
    } finally {
      historyLoading = false;
    }
  };

  // Load more history items for infinite scroll
  const loadMoreHistory = async () => {
    if (loadingMoreHistory || !hasMoreHistory) return;

    loadingMoreHistory = true;
    try {
      const data = await sendHistoryRequest('history_recent', {
        limit: 20,
        offset: historyOffset
      });

      if (Array.isArray(data) && data.length > 0) {
        historyItems = [...historyItems, ...data];
        historyOffset += data.length;
        hasMoreHistory = data.length === 20; // If we got full batch, there might be more
      } else {
        hasMoreHistory = false;
      }
    } catch (error) {
      console.error('Error fetching more history:', error);
      hasMoreHistory = false;
    } finally {
      loadingMoreHistory = false;
    }
  };

  // Navigate to a URL when history item is clicked
  const navigateTo = (url: string) => {
    window.location.href = url;
  };

  // Delete a history entry
  const deleteHistoryEntry = async (id: number, event: Event) => {
    event.stopPropagation(); // Prevent navigation

    // Add to deleting array for animation
    if (!deletingIds.includes(id)) {
      deletingIds = [...deletingIds, id];
    }

    try {
      // Set up response handlers
      (window as any).__dumber_history_deleted = async () => {
        // Wait a bit for the fade animation to complete
        await new Promise(resolve => setTimeout(resolve, 200));

        // Reset pagination and refresh history after successful deletion
        historyOffset = 0;
        hasMoreHistory = true;
        await fetchHistory();
        await fetchStats();
        await fetchTopVisited();

        // Remove from deleting array after refresh
        deletingIds = deletingIds.filter(i => i !== id);
      };

      (window as any).__dumber_history_error = (error: string) => {
        console.error('Error deleting history entry:', error);
        // Remove from deleting array if failed
        deletingIds = deletingIds.filter(i => i !== id);
      };

      // Send message to Go backend
      const bridge = (window as any).webkit?.messageHandlers?.dumber;
      if (bridge && typeof bridge.postMessage === 'function') {
        bridge.postMessage(JSON.stringify({
          type: 'history_delete',
          historyId: id.toString()
        }));
      } else {
        throw new Error('WebKit message handler not available');
      }
    } catch (error) {
      console.error('Error sending delete request:', error);
      // Remove from deleting array if failed
      deletingIds = deletingIds.filter(i => i !== id);
    }
  };

  // Get domain from URL
  const getDomain = (url: string): string => {
    try {
      return new URL(url).hostname;
    } catch {
      return url;
    }
  };

  const getDisplayTitle = (item: HistoryItem): string => {
    if (!item) return '';
    const trimmed = item.title?.trim();
    if (trimmed) return trimmed;
    const domain = getDomain(item.url);
    return domain || item.url;
  };

  const highlightDomainInUrl = (url: string): string => {
    try {
      const urlObj = new URL(url);
      const domain = urlObj.hostname;
      const domainStart = url.indexOf(domain);

      if (domainStart === -1) return url;

      const beforeDomain = url.slice(0, domainStart);
      const afterDomain = url.slice(domainStart + domain.length);

      return `${beforeDomain}<span class="history-url-domain">${domain}</span>${afterDomain}`;
    } catch {
      return url;
    }
  };

  // Format relative time
  const formatTime = (timestamp: string): string => {
    try {
      const date = new Date(timestamp);
      const now = new Date();
      const diff = now.getTime() - date.getTime();

      const minutes = Math.floor(diff / (1000 * 60));
      const hours = Math.floor(diff / (1000 * 60 * 60));
      const days = Math.floor(diff / (1000 * 60 * 60 * 24));

      if (minutes < 1) return 'Just now';
      if (minutes < 60) return `${minutes}m ago`;
      if (hours < 24) return `${hours}h ago`;
      if (days < 7) return `${days}d ago`;

      return date.toLocaleDateString();
    } catch {
      return '';
    }
  };

  const formatCalendarDate = (timestamp?: string): string => {
    if (!timestamp) return '—';
    try {
      const date = new Date(timestamp);
      return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
      return '—';
    }
  };

  const formatVisitLabel = (count: number): string => {
    if (!count || count <= 0) return 'Open';
    if (count >= 1000) {
      const value = count / 1000;
      const formatted = value >= 10 ? Math.round(value).toString() : value.toFixed(1).replace(/\.0$/, '');
      return `${formatted}k visits`;
    }
    return `${count} ${count === 1 ? 'visit' : 'visits'}`;
  };

  const buildQuickAccess = (): HistoryItem[] => {
    const picks: HistoryItem[] = [];
    const seen = new Set<string>();

    const pushItem = (item: HistoryItem | null | undefined) => {
      if (!item) return;
      if (seen.has(item.url)) return;
      seen.add(item.url);
      picks.push(item);
    };

    // First, add pinned sites (no visit count requirement for pinned)
    for (const item of topVisited ?? []) {
      if (pinnedSites.has(item.url)) {
        pushItem(item);
      }
    }

    for (const item of historyItems ?? []) {
      if (pinnedSites.has(item.url)) {
        pushItem(item);
      }
    }

    // Then add most visited sites (non-pinned, with minimum visit count)
    for (const item of topVisited ?? []) {
      if (!pinnedSites.has(item.url) && item.visit_count >= MIN_VISIT_COUNT_FOR_QUICK_ACCESS) {
        pushItem(item);
      }
    }

    // Finally add recent history (non-pinned, with minimum visit count)
    for (const item of historyItems ?? []) {
      if (!pinnedSites.has(item.url) && item.visit_count >= MIN_VISIT_COUNT_FOR_QUICK_ACCESS) {
        pushItem(item);
      }
    }

    return picks;
  };

  let quickAccess = $derived<HistoryItem[]>((pinnedSites.size, buildQuickAccess()));
  let quickAccessLoading = $derived(topVisitedLoading || historyLoading);
  interface LatestVisitMeta {
    entry: HistoryItem;
    relative: string;
    absolute: string;
    domain: string;
    title: string;
  }

  const latestVisitInfo = $derived<LatestVisitMeta | null>((() => {
    if (!historyItems.length) return null;

    let latest: HistoryItem | null = null;
    let latestEpoch = Number.NEGATIVE_INFINITY;

    for (const entry of historyItems) {
      const epoch = Number(new Date(entry.last_visited));
      if (!Number.isFinite(epoch)) continue;
      if (epoch > latestEpoch) {
        latest = entry;
        latestEpoch = epoch;
      }
    }

    if (!latest) return null;

    return {
      entry: latest,
      relative: formatTime(latest.last_visited),
      absolute: formatCalendarDate(latest.last_visited),
      domain: getDomain(latest.url),
      title: getDisplayTitle(latest)
    };
  })());

  const persistTheme = (mode: ThemeMode) => {
    try {
      localStorage.setItem(THEME_STORAGE_KEY, mode);
    } catch (error) {
      console.warn("[homepage] Failed to persist theme preference", error);
    }
  };

  const applyTheme = (mode: ThemeMode) => {
    themeMode = mode;

    const manager = (window as any).__dumber_color_scheme_manager as
      | { setUserPreference?: (theme: ThemeMode) => void }
      | undefined;

    if (manager?.setUserPreference) {
      manager.setUserPreference(mode);
      return;
    }

    if (window.__dumber_setTheme) {
      window.__dumber_setTheme(mode);
      persistTheme(mode);
      return;
    }

    if (mode === "dark") {
      document.documentElement.classList.add("dark");
    } else {
      document.documentElement.classList.remove("dark");
    }
    persistTheme(mode);
  };

  const toggleTheme = () => {
    const nextMode = themeMode === "dark" ? "light" : "dark";
    applyTheme(nextMode);
  };

  // Fetch history stats via message bridge
  const fetchStats = async () => {
    try {
      // Set up response handler
      (window as any).__dumber_history_stats = (data: any) => {
        stats = data;
        statsLoading = false;
      };

      (window as any).__dumber_history_error = (error: string) => {
        console.error('Error fetching stats:', error);
        stats = null;
        statsLoading = false;
      };

      // Send message to Go backend
      const bridge = (window as any).webkit?.messageHandlers?.dumber;
      if (bridge && typeof bridge.postMessage === 'function') {
        bridge.postMessage(JSON.stringify({
          type: 'history_stats'
        }));
      } else {
        throw new Error('WebKit message handler not available');
      }
    } catch (error) {
      console.error('Error sending stats request:', error);
      stats = null;
      statsLoading = false;
    }
  };

  // Fetch top visited sites via message bridge
  const fetchTopVisited = async () => {
    try {
      // Set up response handler
      (window as any).__dumber_history_search = (data: any[]) => {
        // Sort by visit_count in descending order and take top 5
        const sortedData = Array.isArray(data) ?
          data.sort((a, b) => (b.visit_count || 0) - (a.visit_count || 0)).slice(0, 5) : [];
        topVisited = sortedData;
        topVisitedLoading = false;
      };

      (window as any).__dumber_history_error = (error: string) => {
        console.error('Error fetching top visited:', error);
        topVisited = [];
        topVisitedLoading = false;
      };

      // Send message to Go backend
      const bridge = (window as any).webkit?.messageHandlers?.dumber;
      if (bridge && typeof bridge.postMessage === 'function') {
        bridge.postMessage(JSON.stringify({
          type: 'history_search',
          q: '',
          limit: 50  // Get more results to sort properly
        }));
      } else {
        throw new Error('WebKit message handler not available');
      }
    } catch (error) {
      console.error('Error sending top visited request:', error);
      topVisited = [];
      topVisitedLoading = false;
    }
  };

  // Intersection Observer for infinite scroll
  let historyScrollSentinel = $state<HTMLElement | null>(null);

  const setupInfiniteScroll = () => {
    if (!historyScrollSentinel) return;

    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (entry && entry.isIntersecting && hasMoreHistory && !loadingMoreHistory) {
          loadMoreHistory();
        }
      },
      {
        threshold: 0.1,
        rootMargin: '50px'
      }
    );

    observer.observe(historyScrollSentinel);

    return () => observer.disconnect();
  };

  // Helper to check if text overflows and apply truncation class
  const checkOverflow = (node: HTMLElement) => {
    const check = () => {
      if (node.scrollWidth > node.clientWidth) {
        node.classList.add('truncated');
      } else {
        node.classList.remove('truncated');
      }
    };

    check();

    const resizeObserver = new ResizeObserver(check);
    resizeObserver.observe(node);

    return {
      destroy() {
        resizeObserver.disconnect();
      }
    };
  };

  onMount(() => {
    loadPinnedSites();
    initializeShortcuts();
    fetchHistory().then(() => {
      // Setup infinite scroll after initial history is loaded
      setTimeout(setupInfiniteScroll, 100);
    });
    fetchStats();
    fetchTopVisited();

    // Theme synchronization
    const syncThemeState = () => {
      themeMode = document.documentElement.classList.contains("dark")
        ? "dark"
        : "light";
    };
    syncThemeState();
    themeObserver = new MutationObserver(syncThemeState);
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });

    return () => {
      themeObserver?.disconnect();
      themeObserver = null;
    };
  });


</script>

<svelte:head>
  <title>Dumber Browser - Homepage</title>
  <meta name="description" content="Fast Wayland Browser - Your browsing patterns at a glance" />
</svelte:head>

<div class="homepage-shell">
  <div class="terminal-frame">
    <header class="terminal-header">
      <div class="terminal-heading">
        <span class="terminal-path">dumb://home</span>
        <span class="terminal-meta">
          {statsLoading ? 'syncing…' : `${stats?.recent_count ?? 0} recent`}
          · {historyItems.length} entries
          · {pinnedSites.size} pinned
        </span>
      </div>
      <button
        class="theme-toggle-button"
        type="button"
        aria-pressed={themeMode === "dark"}
        title={themeMode === "dark" ? "Switch to light mode" : "Switch to dark mode"}
        onclick={toggleTheme}
      >
        <span class="sr-only">
          {themeMode === "dark" ? "Switch to light mode" : "Switch to dark mode"}
        </span>
        {#if themeMode === "dark"}
          <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
            <path
              fill="currentColor"
              fill-rule="evenodd"
              clip-rule="evenodd"
              d="M17.293 13.293A8 8 0 0 1 10.707 2.997a8.001 8.001 0 1 0 6.586 10.296z"
            />
          </svg>
        {:else}
          <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
            <circle cx="12" cy="12" r="5" fill="currentColor" />
            <g stroke="currentColor" stroke-width="2" stroke-linecap="round">
              <line x1="12" y1="1" x2="12" y2="4" />
              <line x1="12" y1="20" x2="12" y2="23" />
              <line x1="4.22" y1="4.22" x2="6.34" y2="6.34" />
              <line x1="17.66" y1="17.66" x2="19.78" y2="19.78" />
              <line x1="1" y1="12" x2="4" y2="12" />
              <line x1="20" y1="12" x2="23" y2="12" />
              <line x1="4.22" y1="19.78" x2="6.34" y2="17.66" />
              <line x1="17.66" y1="6.34" x2="19.78" y2="4.22" />
            </g>
          </svg>
        {/if}
      </button>
    </header>

    <div class="terminal-body">
      <div class="terminal-status-row">
        <div class="status-chip">
          <span class="chip-label">history</span>
          <span class="chip-value">{historyItems.length}</span>
        </div>
        <div class="status-chip">
          <span class="chip-label">pinned</span>
          <span class="chip-value">{pinnedSites.size}</span>
        </div>
        <div class="status-chip">
          <span class="chip-label">quick-links</span>
          <span class="chip-value">{quickAccess.length}</span>
        </div>
        {#if stats && stats.total_entries}
          <div class="status-chip">
            <span class="chip-label">stored</span>
            <span class="chip-value">{stats.total_entries}</span>
          </div>
        {/if}
      </div>

      <div
        class="last-visit-banner"
        role="button"
        tabindex={latestVisitInfo ? 0 : -1}
        onclick={() => latestVisitInfo && navigateTo(latestVisitInfo.entry.url)}
        onkeydown={(event) => {
          if (event.key === 'Enter' && latestVisitInfo) {
            navigateTo(latestVisitInfo.entry.url);
          }
        }}
        aria-label={latestVisitInfo ? `Open ${latestVisitInfo.title}` : 'No recorded visits'}
      >
        <span class="banner-label">last-visit</span>
        {#if latestVisitInfo}
          <span class="banner-title" use:checkOverflow>
            {latestVisitInfo.title}
          </span>
          <span class="banner-domain">{latestVisitInfo.domain}</span>
          <span class="banner-time">{latestVisitInfo.relative}</span>
          <span class="banner-absolute">{latestVisitInfo.absolute}</span>
        {:else}
          <span class="banner-empty">none recorded yet</span>
        {/if}
      </div>

      <div class="terminal-grid main-panels">
        <section class="history-panel terminal-pane">
        <div class="panel-header">
          <h2 class="panel-title">Recent History</h2>
          <p class="panel-subtitle">Scroll to revisit anything from your latest sessions.</p>
        </div>
        <div class="panel-body history-body">
          {#if historyLoading}
            <div class="loading">Loading history...</div>
          {:else if historyItems.length === 0}
            <div class="empty-state">
              <h3>No history yet</h3>
              <p>Start browsing to see your recent history here.</p>
            </div>
          {:else}
            <div class="history-list">
              {#each displayedHistoryItems as item (item.id)}
                <div
                  class="history-item"
                  class:deleting={isDeleting(item.id)}
                  role="button"
                  tabindex="0"
                  onclick={() => navigateTo(item.url)}
                  onkeydown={(e) => e.key === 'Enter' && navigateTo(item.url)}
                >
                  <div class="history-item-leading">
                    {#if item.favicon_url}
                      <img
                        src={item.favicon_url}
                        alt=""
                        class="history-favicon-img"
                        onerror={(e) => {
                          const target = e.target as HTMLImageElement;
                          if (target) {
                            target.style.display = 'none';
                            const fallback = target.nextElementSibling as HTMLElement;
                            if (fallback) fallback.style.display = 'flex';
                          }
                        }}
                        onload={(e) => {
                          const target = e.target as HTMLImageElement;
                          if (target) {
                            const fallback = target.nextElementSibling as HTMLElement;
                            if (fallback) fallback.style.display = 'none';
                          }
                        }}
                      />
                      <div class="history-favicon-fallback" style="display: none;">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                          <circle cx="12" cy="12" r="10" />
                          <path d="M2 12h20" />
                        </svg>
                      </div>
                    {:else}
                      <div class="history-favicon-fallback">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                          <circle cx="12" cy="12" r="10" />
                          <path d="M2 12h20" />
                        </svg>
                      </div>
                    {/if}
                  </div>
                  <div class="history-item-content">
                    <div class="history-item-main">
                      <div class="history-title" use:checkOverflow>
                        {getDisplayTitle(item)}
                      </div>
                      <div class="history-time">{formatTime(item.last_visited)}</div>
                    </div>
                    <div class="history-url" use:checkOverflow>
                      {@html highlightDomainInUrl(item.url)}
                    </div>
                  </div>
                  <div class="history-actions">
                    <div
                      class="history-pin"
                      role="button"
                      tabindex="0"
                      onclick={(e) => { e.stopPropagation(); togglePin(item.url); }}
                      onkeydown={(e) => { if (e.key === 'Enter') { e.stopPropagation(); togglePin(item.url); } }}
                      title={isPinned(item.url) ? "Unpin from favorites" : "Pin to favorites"}
                    >
                      {#if isPinned(item.url)}
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                          <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
                        </svg>
                      {:else}
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" stroke-linecap="round" aria-hidden="true">
                          <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
                        </svg>
                      {/if}
                    </div>
                    <div
                      class="history-delete"
                      role="button"
                      tabindex="0"
                      onclick={(e) => deleteHistoryEntry(item.id, e)}
                      onkeydown={(e) => e.key === 'Enter' && deleteHistoryEntry(item.id, e)}
                      title="Delete this entry"
                    >
                      <span aria-hidden="true">×</span>
                    </div>
                  </div>
                </div>
              {/each}

              {#if !showAllHistory && historyItems.length > INITIAL_HISTORY_DISPLAY}
                <button
                  class="show-more-button"
                  onclick={() => showAllHistory = true}
                  type="button"
                >
                  Show {historyItems.length - INITIAL_HISTORY_DISPLAY} more items
                </button>
              {:else if showAllHistory && hasMoreHistory}
                <div bind:this={historyScrollSentinel} class="scroll-sentinel"></div>
                {#if loadingMoreHistory}
                  <div class="loading-more">Loading more history...</div>
                {/if}
              {/if}
            </div>
          {/if}
        </div>
      </section>

        <section class="quick-access-panel terminal-pane">
        <div class="panel-header">
          <h2 class="panel-title">Jump back in</h2>
          <p class="panel-subtitle">Direct access to the sites you like the most.</p>
        </div>
        <div class="panel-body quick-access-body">
          {#if quickAccessLoading}
            <div class="loading">Preparing your shortcuts...</div>
          {:else if quickAccess.length === 0}
            <div class="empty-state compact">
              <h3>No quick links yet</h3>
              <p>Browse a few sites and we'll surface your frequent destinations here.</p>
            </div>
          {:else}
            <div class="quick-access-grid-compact">
              {#each quickAccess as item (item.id)}
                <div
                  class="quick-access-item-compact"
                  role="button"
                  tabindex="0"
                  onclick={() => navigateTo(item.url)}
                  onkeydown={(e) => e.key === 'Enter' && navigateTo(item.url)}
                >
                  <div class="quick-access-favicon-compact">
                    {#if item.favicon_url}
                      <img
                        src={item.favicon_url}
                        alt=""
                        class="quick-access-favicon-img-compact"
                        onerror={(e) => {
                          const target = e.target as HTMLImageElement;
                          if (target) {
                            target.style.display = 'none';
                            const fallback = target.nextElementSibling as HTMLElement;
                            if (fallback) fallback.style.display = 'flex';
                          }
                        }}
                        onload={(e) => {
                          const target = e.target as HTMLImageElement;
                          if (target) {
                            const fallback = target.nextElementSibling as HTMLElement;
                            if (fallback) fallback.style.display = 'none';
                          }
                        }}
                      />
                      <div class="quick-access-fallback-compact" style="display: none;">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                          <circle cx="12" cy="12" r="10" />
                          <path d="M2 12h20" />
                        </svg>
                      </div>
                    {:else}
                      <div class="quick-access-fallback-compact">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                          <circle cx="12" cy="12" r="10" />
                          <path d="M2 12h20" />
                        </svg>
                      </div>
                    {/if}
                  </div>
                  <div class="quick-access-meta-compact">
                    <div class="quick-access-title-compact" use:checkOverflow>
                      {getDisplayTitle(item)}
                    </div>
                    <div class="quick-access-domain-compact">{getDomain(item.url)}</div>
                  </div>
                  <div class="quick-access-actions-compact">
                    <div
                      class="quick-access-pin-compact"
                      role="button"
                      tabindex="0"
                      onclick={(e) => { e.stopPropagation(); togglePin(item.url); }}
                      onkeydown={(e) => { if (e.key === 'Enter') { e.stopPropagation(); togglePin(item.url); } }}
                      title={isPinned(item.url) ? "Unpin from favorites" : "Pin to favorites"}
                    >
                      {#if isPinned(item.url)}
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                          <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
                        </svg>
                      {:else}
                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" stroke-linecap="round" aria-hidden="true">
                          <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
                        </svg>
                      {/if}
                    </div>
                    <div class="quick-access-visits-compact">{formatVisitLabel(item.visit_count)}</div>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </section>
      </div>

      <div class="terminal-grid supporting-panels">
        <section class="stats-panel terminal-pane">
          <div class="panel-header">
            <h2 class="panel-title">Usage Stats</h2>
            <p class="panel-subtitle">Your browsing activity at a glance.</p>
          </div>
          <div class="panel-body">
            {#if statsLoading}
              <div class="loading">Loading stats...</div>
            {:else if stats}
              <div class="stats-grid-horizontal">
                <div class="stat-item-horizontal">
                  <div class="stat-value">{stats.total_entries}</div>
                  <div class="stat-label">Entries Stored</div>
                </div>
                <div class="stat-item-horizontal">
                  <div class="stat-value">{stats.recent_count}</div>
                  <div class="stat-label">Recent Window</div>
                </div>
                {#if stats.newest_entry}
                  <div class="stat-item-horizontal">
                    <div class="stat-value">{formatTime(stats.newest_entry)}</div>
                    <div class="stat-label">Newest Visit</div>
                  </div>
                {/if}
                {#if stats.oldest_entry}
                  <div class="stat-item-horizontal">
                    <div class="stat-value">{formatCalendarDate(stats.oldest_entry)}</div>
                    <div class="stat-label">Oldest Entry</div>
                  </div>
                {/if}
              </div>
            {:else}
              <div class="empty-state compact">
                <p>No statistics available</p>
              </div>
            {/if}
          </div>
        </section>

        <section class="shortcuts-panel terminal-pane">
          <div class="panel-header">
            <h2 class="panel-title">Keyboard Shortcuts</h2>
            <p class="panel-subtitle">Stay quick with the essentials.</p>
          </div>
          <div class="panel-body">
            {#if shortcutsLoading}
              <div class="loading">Loading shortcuts...</div>
            {:else}
              <div class="shortcuts-list">
                {#each shortcuts as shortcut (shortcut.key)}
                  <div class="shortcut-item">
                    <div class="shortcut-key-badge">{shortcut.key}</div>
                    <div class="shortcut-desc">{shortcut.description}</div>
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        </section>
      </div>
    </div>
  </div>

  <Footer />
</div>

<style>
.homepage-shell {
  min-height: 100vh;
  background-color: var(--dynamic-bg);
  color: var(--dynamic-text);
  display: flex;
  flex-direction: column;
  padding: clamp(1.5rem, 4vw, 2.5rem);
  font-family:
    "JetBrains Mono",
    "Fira Code",
    "SFMono-Regular",
    Menlo,
    monospace;
  line-height: 1.45;
}

.homepage-shell * {
  box-sizing: border-box;
}

.terminal-frame {
  flex: 1;
  width: 100%;
  max-width: 1160px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--dynamic-border);
  background: color-mix(in srgb, var(--dynamic-surface) 80%, var(--dynamic-bg) 20%);
  box-shadow:
    inset 0 0 0 1px color-mix(in srgb, var(--dynamic-border) 30%, transparent),
    0 16px 28px -24px rgb(0 0 0 / 0.6);
}

.terminal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.85rem 1.1rem;
  border-bottom: 1px solid var(--dynamic-border);
  background: color-mix(in srgb, var(--dynamic-bg) 92%, var(--dynamic-surface) 8%);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  font-size: 0.78rem;
}

.terminal-heading {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.terminal-path {
  font-weight: 600;
  color: var(--dynamic-text);
}

.terminal-meta {
  color: var(--dynamic-muted);
  font-size: 0.72rem;
  letter-spacing: 0.12em;
}

.theme-toggle-button {
  width: 2.5rem;
  height: 2.5rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--dynamic-border);
  background-color: var(--dynamic-bg);
  color: var(--dynamic-muted);
  transition: color 150ms ease, border-color 150ms ease, background-color 150ms ease;
  border-radius: 0;
  cursor: pointer;
}

.theme-toggle-button:hover,
.theme-toggle-button:focus-visible {
  border-color: color-mix(in srgb, var(--dynamic-border) 45%, var(--dynamic-text) 55%);
  color: var(--dynamic-text);
  background-color: color-mix(in srgb, var(--dynamic-bg) 80%, var(--dynamic-surface) 20%);
  outline: none;
}

.theme-toggle-button svg {
  width: 1.3rem;
  height: 1.3rem;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.terminal-body {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
  padding: 1.25rem 1.5rem 1.5rem;
  background: var(--dynamic-bg);
}

.terminal-status-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
}

.status-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.45rem 0.8rem;
  border: 1px solid var(--dynamic-border);
  background: color-mix(in srgb, var(--dynamic-bg) 92%, var(--dynamic-surface) 8%);
  font-size: 0.68rem;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--dynamic-muted);
}

.status-chip .chip-value {
  color: var(--dynamic-text);
  font-weight: 600;
}

.last-visit-banner {
  margin-top: 0.9rem;
  display: grid;
  grid-template-columns: minmax(0, auto) minmax(0, 1fr) minmax(120px, auto) minmax(120px, auto) minmax(120px, auto);
  gap: 0.75rem;
  align-items: center;
  padding: 0.75rem 1rem;
  border: 1px solid var(--dynamic-border);
  background: color-mix(in srgb, var(--dynamic-bg) 90%, var(--dynamic-surface) 10%);
  color: var(--dynamic-text);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  transition: background-color 150ms ease, border-color 150ms ease;
  cursor: pointer;
}

.last-visit-banner:hover,
.last-visit-banner:focus-visible {
  background: color-mix(in srgb, var(--dynamic-bg) 72%, var(--dynamic-surface) 28%);
  border-color: color-mix(in srgb, var(--dynamic-border) 45%, var(--dynamic-text) 55%);
  outline: none;
}

.last-visit-banner[tabindex="-1"],
.last-visit-banner[tabindex="-1"]:hover {
  cursor: default;
  background: color-mix(in srgb, var(--dynamic-bg) 95%, var(--dynamic-surface) 5%);
}

.banner-label {
  color: var(--dynamic-muted);
  font-size: 0.68rem;
}

.banner-title {
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  letter-spacing: 0.06em;
}

:global(.banner-title.truncated) {
  mask-image: linear-gradient(to right, black 85%, transparent 100%);
  -webkit-mask-image: linear-gradient(to right, black 85%, transparent 100%);
}

.banner-domain {
  color: var(--dynamic-muted);
  font-size: 0.68rem;
  letter-spacing: 0.1em;
}

.banner-time {
  font-size: 0.72rem;
  color: var(--dynamic-text);
  white-space: nowrap;
}

.banner-absolute {
  font-size: 0.68rem;
  color: var(--dynamic-muted);
  white-space: nowrap;
}

.banner-empty {
  grid-column: 2 / -1;
  color: var(--dynamic-muted);
  font-size: 0.72rem;
  letter-spacing: 0.1em;
}

.terminal-grid {
  display: grid;
  gap: 1rem;
}

.main-panels {
  grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
}

.supporting-panels {
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
}

.terminal-pane {
  border: 1px solid var(--dynamic-border);
  background: color-mix(in srgb, var(--dynamic-bg) 88%, var(--dynamic-surface) 12%);
  display: flex;
  flex-direction: column;
  min-height: 0;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--dynamic-border) 18%, transparent);
}

.panel-header {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  padding: 1rem 1.1rem;
  border-bottom: 1px solid var(--dynamic-border);
  background: color-mix(in srgb, var(--dynamic-bg) 75%, transparent);
}

.panel-title {
  margin: 0;
  font-size: 0.95rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--dynamic-text);
}

.panel-subtitle {
  margin: 0;
  font-size: 0.72rem;
  color: var(--dynamic-muted);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.panel-body {
  padding: 1.1rem 1.1rem 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1.1rem;
  min-height: 0;
  overflow: hidden;
}

.loading {
  padding: 0.9rem;
  text-align: center;
  color: var(--dynamic-muted);
  border: 1px dashed var(--dynamic-border);
  background-color: color-mix(in srgb, var(--dynamic-bg) 85%, var(--dynamic-surface) 15%);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  font-size: 0.7rem;
}

.empty-state {
  border: 1px dashed var(--dynamic-border);
  padding: 1.4rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  align-items: center;
  text-align: center;
  color: var(--dynamic-muted);
  background-color: color-mix(in srgb, var(--dynamic-bg) 90%, var(--dynamic-surface) 10%);
  letter-spacing: 0.06em;
}

.empty-state h3 {
  margin: 0;
  color: var(--dynamic-text);
  font-size: 0.9rem;
  text-transform: uppercase;
}

.empty-state p {
  margin: 0;
  font-size: 0.72rem;
}

.empty-state.compact {
  padding: 1rem;
}

.history-body {
  gap: 0.75rem;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.history-list {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  border: 1px solid var(--dynamic-border);
  background: color-mix(in srgb, var(--dynamic-bg) 95%, var(--dynamic-surface) 5%);
}

.history-item {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 0.75rem;
  padding: 0.6rem 0.9rem;
  border-bottom: 1px dashed var(--dynamic-border);
  transition: background-color 150ms ease, color 150ms ease, border-color 150ms ease;
}

.history-item:last-child {
  border-bottom: none;
}

.history-item:hover,
.history-item:focus-visible {
  background: color-mix(in srgb, var(--dynamic-bg) 65%, var(--dynamic-surface) 35%);
  outline: none;
}

.history-item.deleting {
  opacity: 0.35;
  pointer-events: none;
  transform: translateX(4px);
}

.history-item-leading {
  width: 38px;
  height: 38px;
  border: 1px solid var(--dynamic-border);
  background-color: var(--dynamic-bg);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.history-favicon-img {
  width: 60%;
  height: 60%;
  object-fit: contain;
}

.history-favicon-fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  color: var(--dynamic-muted);
}

.history-favicon-fallback svg {
  opacity: 0.55;
}

.history-item-content {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}

.history-item-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.history-title {
  font-weight: 600;
  color: var(--dynamic-text);
  white-space: nowrap;
  overflow: hidden;
}

:global(.history-title.truncated) {
  mask-image: linear-gradient(to right, black 85%, transparent 100%);
  -webkit-mask-image: linear-gradient(to right, black 85%, transparent 100%);
}

.history-title:hover {
  cursor: pointer;
}

.history-time {
  font-size: 0.72rem;
  color: var(--dynamic-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  white-space: nowrap;
}

.history-url {
  font-size: 0.72rem;
  color: var(--dynamic-muted);
  white-space: nowrap;
  overflow: hidden;
}

:global(.history-url.truncated) {
  mask-image: linear-gradient(to right, black 85%, transparent 100%);
  -webkit-mask-image: linear-gradient(to right, black 85%, transparent 100%);
}

.history-url :global(.history-url-domain) {
  color: var(--dynamic-text) !important;
  font-weight: 600;
}

.history-actions {
  display: flex;
  gap: 0.35rem;
}

.history-pin,
.history-delete {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 1px solid var(--dynamic-border);
  color: var(--dynamic-muted);
  background: transparent;
  transition: background-color 120ms ease, color 120ms ease, border-color 120ms ease;
}

.history-pin:hover,
.history-pin:focus-visible,
.history-delete:hover,
.history-delete:focus-visible {
  color: var(--dynamic-text);
  border-color: color-mix(in srgb, var(--dynamic-border) 45%, var(--dynamic-text) 55%);
  background: color-mix(in srgb, var(--dynamic-bg) 80%, var(--dynamic-surface) 20%);
  outline: none;
}

.scroll-sentinel {
  height: 1px;
}

.loading-more {
  padding: 0.75rem;
  font-size: 0.7rem;
  text-align: center;
  color: var(--dynamic-muted);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.quick-access-body {
  gap: 0.9rem;
}

.quick-access-grid-compact {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.quick-access-item-compact {
  border: 1px solid var(--dynamic-border);
  background: color-mix(in srgb, var(--dynamic-bg) 92%, var(--dynamic-surface) 8%);
  padding: 0.75rem 0.9rem;
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 0.75rem;
  align-items: center;
  transition: background-color 150ms ease, border-color 150ms ease;
  cursor: pointer;
}

.quick-access-item-compact:hover,
.quick-access-item-compact:focus-visible {
  border-color: color-mix(in srgb, var(--dynamic-border) 45%, var(--dynamic-text) 55%);
  background: color-mix(in srgb, var(--dynamic-bg) 70%, var(--dynamic-surface) 30%);
  outline: none;
}

.quick-access-favicon-compact {
  width: 36px;
  height: 36px;
  border: 1px solid var(--dynamic-border);
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--dynamic-bg);
}

.quick-access-favicon-img-compact {
  width: 60%;
  height: 60%;
  object-fit: contain;
}

.quick-access-fallback-compact {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  color: var(--dynamic-muted);
}

.quick-access-fallback-compact svg {
  opacity: 0.55;
}

.quick-access-meta-compact {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}

.quick-access-title-compact {
  color: var(--dynamic-text);
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
}

:global(.quick-access-title-compact.truncated) {
  mask-image: linear-gradient(to right, black 85%, transparent 100%);
  -webkit-mask-image: linear-gradient(to right, black 85%, transparent 100%);
}

.quick-access-title-compact:hover {
  cursor: pointer;
}

.quick-access-domain-compact {
  color: var(--dynamic-muted);
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.quick-access-actions-compact {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  align-items: flex-end;
  justify-content: center;
}

.quick-access-pin-compact {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: 1px solid var(--dynamic-border);
  color: var(--dynamic-muted);
  background: transparent;
  transition: background-color 120ms ease, border-color 120ms ease, color 120ms ease;
  cursor: pointer;
}

.quick-access-pin-compact:hover,
.quick-access-pin-compact:focus-visible {
  color: var(--dynamic-text);
  border-color: color-mix(in srgb, var(--dynamic-border) 45%, var(--dynamic-text) 55%);
  background: color-mix(in srgb, var(--dynamic-bg) 80%, var(--dynamic-surface) 20%);
  outline: none;
}

.quick-access-pin-compact svg {
  width: 14px;
  height: 14px;
}

.quick-access-visits-compact {
  font-size: 0.7rem;
  color: var(--dynamic-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.show-more-button {
  width: 100%;
  padding: 0.65rem 0.75rem;
  border: 1px solid var(--dynamic-border);
  background: transparent;
  color: var(--dynamic-muted);
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  cursor: pointer;
  transition: background-color 150ms ease, border-color 150ms ease, color 150ms ease;
  border-radius: 0;
}

.show-more-button:hover,
.show-more-button:focus-visible {
  background: color-mix(in srgb, var(--dynamic-bg) 75%, var(--dynamic-surface) 25%);
  color: var(--dynamic-text);
  border-color: color-mix(in srgb, var(--dynamic-border) 45%, var(--dynamic-text) 55%);
  outline: none;
}

.stats-grid-horizontal {
  display: grid;
  gap: 0.85rem;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
}

.stat-item-horizontal {
  border: 1px solid var(--dynamic-border);
  padding: 0.85rem;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  background: color-mix(in srgb, var(--dynamic-bg) 92%, var(--dynamic-surface) 8%);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.stat-value {
  font-size: 0.95rem;
  color: var(--dynamic-text);
  font-weight: 600;
}

.stat-label {
  font-size: 0.68rem;
  color: var(--dynamic-muted);
}

.shortcuts-list {
  display: grid;
  gap: 0.6rem;
}

.shortcut-item {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 0.6rem;
  align-items: center;
  border: 1px solid var(--dynamic-border);
  padding: 0.6rem 0.75rem;
  background: color-mix(in srgb, var(--dynamic-bg) 92%, var(--dynamic-surface) 8%);
}

.shortcut-key-badge {
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  padding: 0.25rem 0.6rem;
  border: 1px solid var(--dynamic-border);
  color: var(--dynamic-text);
  background: color-mix(in srgb, var(--dynamic-bg) 80%, var(--dynamic-surface) 20%);
}

.shortcut-desc {
  font-size: 0.72rem;
  color: var(--dynamic-muted);
  letter-spacing: 0.06em;
}

@media (max-width: 960px) {
  .terminal-body {
    padding: 1.1rem;
  }

  .terminal-grid {
    gap: 0.75rem;
  }

  .main-panels {
    grid-template-columns: 1fr;
  }

  .last-visit-banner {
    grid-template-columns: minmax(0, auto) minmax(0, 1fr);
    grid-template-rows: repeat(3, auto);
    gap: 0.5rem 0.75rem;
  }

  .banner-time,
  .banner-absolute {
    justify-self: flex-start;
  }
}

@media (max-width: 640px) {
  .homepage-shell {
    padding: 1.25rem;
  }

  .terminal-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .theme-toggle-button {
    align-self: flex-end;
  }

  .terminal-status-row {
    gap: 0.5rem;
  }

  .status-chip {
    padding: 0.35rem 0.65rem;
  }

  .last-visit-banner {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: repeat(4, auto);
  }

  .banner-label,
  .banner-title,
  .banner-domain,
  .banner-time,
  .banner-absolute {
    justify-self: flex-start;
  }
}
</style>
