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
    action: () => void;
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
  const MAX_QUICK_ACCESS_ITEMS = 9;
  type ThemeMode = "light" | "dark";
  const THEME_STORAGE_KEY = "dumber.theme";
  let themeMode = $state<ThemeMode>("dark");
  let themeObserver: MutationObserver | null = null;
  let historyHeaderRef = $state<HTMLElement | null>(null);
  let historyBodyRef = $state<HTMLElement | null>(null);
  let insightsPanelRef = $state<HTMLElement | null>(null);
  let resizeSyncRaf = 0;

  const syncHistoryPanelSize = () => {
    if (!historyBodyRef || !historyHeaderRef || !insightsPanelRef) return;

    if (window.innerWidth <= 960) {
      historyBodyRef.style.maxHeight = "";
      return;
    }

    const insightsHeight = insightsPanelRef.getBoundingClientRect().height;
    if (!insightsHeight || Number.isNaN(insightsHeight)) {
      historyBodyRef.style.maxHeight = "";
      return;
    }

    const headerHeight = historyHeaderRef.getBoundingClientRect().height;
    const bodyStyles = getComputedStyle(historyBodyRef);
    const paddingTop = parseFloat(bodyStyles.paddingTop) || 0;
    const paddingBottom = parseFloat(bodyStyles.paddingBottom) || 0;
    const maxHeight = insightsHeight - headerHeight - paddingTop - paddingBottom;

    if (maxHeight > 0) {
      historyBodyRef.style.maxHeight = `${maxHeight}px`;
    } else {
      historyBodyRef.style.maxHeight = "";
    }
  };

  const scheduleHistoryPanelSync = () => {
    if (resizeSyncRaf) cancelAnimationFrame(resizeSyncRaf);
    resizeSyncRaf = requestAnimationFrame(() => {
      resizeSyncRaf = 0;
      syncHistoryPanelSize();
    });
  };

  // Function for checking if an item is being deleted
  const isDeleting = (id: number) => deletingIds.includes(id);

  // Initialize shortcuts (these are browser shortcuts, not dynamic from API)
  const initializeShortcuts = () => {
    shortcuts = [
      // Browser shortcuts
      {
        key: 'Ctrl+L',
        description: 'Focus address bar',
        action: () => console.log('Ctrl+L triggered')
      },
      {
        key: 'Ctrl+F',
        description: 'Find in page',
        action: () => console.log('Ctrl+F triggered')
      },
      {
        key: 'Ctrl+Shift+C',
        description: 'Copy URL',
        action: () => console.log('Ctrl+Shift+C triggered')
      },
      {
        key: 'Ctrl+R / F5',
        description: 'Reload page',
        action: () => window.location.reload()
      },
      {
        key: 'Ctrl+Shift+R',
        description: 'Hard reload',
        action: () => console.log('Ctrl+Shift+R triggered')
      },
      {
        key: 'F12',
        description: 'DevTools',
        action: () => console.log('F12 triggered')
      },
      {
        key: 'Ctrl+← / Ctrl+→',
        description: 'Navigate back/forward',
        action: () => console.log('Ctrl+Arrow triggered')
      },
      {
        key: 'Ctrl+0',
        description: 'Reset zoom',
        action: () => console.log('Ctrl+0 triggered')
      },
      {
        key: 'Ctrl++ / Ctrl+-',
        description: 'Zoom in/out',
        action: () => console.log('Ctrl+Zoom triggered')
      },
      // Workspace pane mode
      {
        key: 'Ctrl+P',
        description: 'Enter pane mode',
        action: () => console.log('Ctrl+P triggered')
      },
      {
        key: '→ / R (pane mode)',
        description: 'Split pane right',
        action: () => console.log('Split right triggered')
      },
      {
        key: '← / L (pane mode)',
        description: 'Split pane left',
        action: () => console.log('Split left triggered')
      },
      {
        key: '↑ / U (pane mode)',
        description: 'Split pane up',
        action: () => console.log('Split up triggered')
      },
      {
        key: '↓ / D (pane mode)',
        description: 'Split pane down',
        action: () => console.log('Split down triggered')
      },
      {
        key: 'X (pane mode)',
        description: 'Close current pane',
        action: () => console.log('Close pane triggered')
      },
      {
        key: 'Enter (pane mode)',
        description: 'Confirm action',
        action: () => console.log('Confirm triggered')
      },
      {
        key: 'Escape (pane mode)',
        description: 'Exit pane mode',
        action: () => console.log('Exit pane mode triggered')
      },
      // Workspace navigation
      {
        key: 'Alt+Arrow Keys',
        description: 'Navigate between panes',
        action: () => console.log('Navigate panes triggered')
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
      const domain = getDomain(item.url);
      const key = domain || item.url;
      if (seen.has(key)) return;
      seen.add(key);
      picks.push(item);
    };

    for (const item of topVisited ?? []) {
      pushItem(item);
      if (picks.length >= MAX_QUICK_ACCESS_ITEMS) return picks;
    }

    for (const item of historyItems ?? []) {
      pushItem(item);
      if (picks.length >= MAX_QUICK_ACCESS_ITEMS) break;
    }

    return picks;
  };

  let quickAccess = $derived<HistoryItem[]>(buildQuickAccess());
  let quickAccessLoading = $derived(topVisitedLoading || historyLoading);

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

  onMount(() => {
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

    window.addEventListener('resize', scheduleHistoryPanelSync);

    return () => {
      themeObserver?.disconnect();
      themeObserver = null;
      window.removeEventListener('resize', scheduleHistoryPanelSync);
      if (resizeSyncRaf) {
        cancelAnimationFrame(resizeSyncRaf);
        resizeSyncRaf = 0;
      }
    };
  });

  $effect(() => {
    historyItems;
    topVisited;
    stats;
    shortcuts;
    scheduleHistoryPanelSync();
  });

  $effect(() => {
    historyHeaderRef;
    historyBodyRef;
    insightsPanelRef;
    scheduleHistoryPanelSync();
  });

</script>

<svelte:head>
  <title>Dumber Browser - Homepage</title>
  <meta name="description" content="Fast Wayland Browser - Your browsing patterns at a glance" />
</svelte:head>

<div class="homepage-shell">
  <div class="homepage-content">
    <div class="top-bar">
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
              d="M21 12.79A9 9 0 0111.21 3 7 7 0 0012 17a7 7 0 009-4.21z"
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
    </div>

    <section class="hero-panel brutal-panel">
      <div class="hero-heading">
        <h1 class="hero-title">Jump back in</h1>
        <p class="hero-subtitle">Direct access to the sites you visit the most, plus what you just explored.</p>
      </div>
      <div class="quick-access-wrapper">
        {#if quickAccessLoading}
          <div class="loading quick-access-loading">Preparing your shortcuts...</div>
        {:else if quickAccess.length === 0}
          <div class="empty-state quick-access-empty">
            <h3>No quick links yet</h3>
            <p>Browse a few sites and we’ll surface your frequent destinations here.</p>
          </div>
        {:else}
          <div class="quick-access-grid">
            {#each quickAccess as item (item.id)}
              <div
                class="quick-access-item"
                role="button"
                tabindex="0"
                onclick={() => navigateTo(item.url)}
                onkeydown={(e) => e.key === 'Enter' && navigateTo(item.url)}
              >
                <div class="quick-access-favicon">
                  {#if item.favicon_url}
                    <img
                      src={item.favicon_url}
                      alt=""
                      class="quick-access-favicon-img"
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
                    <div class="quick-access-fallback" style="display: none;">
                      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                        <circle cx="12" cy="12" r="10" />
                        <path d="M2 12h20" />
                      </svg>
                    </div>
                  {:else}
                    <div class="quick-access-fallback">
                      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                        <circle cx="12" cy="12" r="10" />
                        <path d="M2 12h20" />
                      </svg>
                    </div>
                  {/if}
                </div>
                <div class="quick-access-meta">
                  <div class="quick-access-title">{getDisplayTitle(item)}</div>
                  <div class="quick-access-domain">{getDomain(item.url)}</div>
                </div>
                <div class="quick-access-visits">{formatVisitLabel(item.visit_count)}</div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </section>

    <div class="main-panels">
      <section class="history-panel brutal-panel">
        <div class="panel-header" bind:this={historyHeaderRef}>
          <h2 class="panel-title">Recent History</h2>
          <p class="panel-subtitle">Scroll to revisit anything from your latest sessions.</p>
        </div>
        <div class="panel-body history-body" bind:this={historyBodyRef}>
          {#if historyLoading}
            <div class="loading">Loading history...</div>
          {:else if historyItems.length === 0}
            <div class="empty-state">
              <h3>No history yet</h3>
              <p>Start browsing to see your recent history here.</p>
            </div>
          {:else}
            <div class="history-list">
              {#each historyItems as item (item.id)}
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
                    <div class="history-item-top">
                      <div class="history-title">{getDisplayTitle(item)}</div>
                      <div class="history-time">{formatTime(item.last_visited)}</div>
                    </div>
                    <div class="history-item-bottom">
                      <div class="history-domain">{getDomain(item.url)}</div>
                      <div class="history-url">{item.url}</div>
                    </div>
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
              {/each}

              {#if hasMoreHistory}
                <div bind:this={historyScrollSentinel} class="scroll-sentinel"></div>
                {#if loadingMoreHistory}
                  <div class="loading-more">Loading more history...</div>
                {/if}
              {/if}
            </div>
          {/if}
        </div>
      </section>

      <section class="insights-panel brutal-panel" bind:this={insightsPanelRef}>
        <div class="panel-header">
          <h2 class="panel-title">Usage Insights</h2>
          <p class="panel-subtitle">Your browsing patterns at a glance.</p>
        </div>
        <div class="panel-body insights-body">
            <div class="insight-block">
              <h3 class="block-title">Top sites</h3>
              {#if topVisitedLoading}
                <div class="loading">Loading usage insights...</div>
              {:else if topVisited.length === 0}
                <div class="empty-state compact">
                  <p>No visit data available</p>
                </div>
              {:else}
                <div class="insights-list">
                  {#each topVisited as item, index (item.id)}
                    <div
                      class="insight-item"
                      role="button"
                      tabindex="0"
                      onclick={() => navigateTo(item.url)}
                      onkeydown={(e) => e.key === 'Enter' && navigateTo(item.url)}
                    >
                      <div class="insight-rank">{index + 1}</div>
                      <div class="insight-body">
                        <div class="insight-title">{getDisplayTitle(item)}</div>
                        <div class="insight-domain">{getDomain(item.url)}</div>
                      </div>
                      <div class="insight-visits">{formatVisitLabel(item.visit_count)}</div>
                    </div>
                  {/each}
                </div>
              {/if}
            </div>

            <div class="insight-block">
              <h3 class="block-title">History snapshot</h3>
              {#if statsLoading}
                <div class="loading">Loading stats...</div>
              {:else if stats}
                <div class="stats-grid">
                  <div class="stat-item">
                    <div class="stat-value">{stats.total_entries}</div>
                    <div class="stat-label">Entries Stored</div>
                  </div>
                  <div class="stat-item">
                    <div class="stat-value">{stats.recent_count}</div>
                    <div class="stat-label">Recent Window</div>
                  </div>
                  {#if stats.newest_entry}
                    <div class="stat-item">
                      <div class="stat-value">{formatTime(stats.newest_entry)}</div>
                      <div class="stat-label">Newest Visit</div>
                    </div>
                  {/if}
                  {#if stats.oldest_entry}
                    <div class="stat-item">
                      <div class="stat-value">{formatCalendarDate(stats.oldest_entry)}</div>
                      <div class="stat-label">Oldest Entry Loaded</div>
                    </div>
                  {/if}
                </div>
              {:else}
                <div class="empty-state compact">
                  <p>No statistics available</p>
                </div>
              {/if}
            </div>
        </div>
      </section>
    </div>

    <section class="shortcuts-panel brutal-panel">
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

  <Footer />
</div>

<style>
/* Homepage Layout Styles */
.homepage-shell {
  min-height: 100vh;
  background-color: var(--dynamic-bg);
  color: var(--dynamic-text);
  display: flex;
  flex-direction: column;
  padding: clamp(1.75rem, 3vw, 3rem);
  font-family: "Fira Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
}

.homepage-shell * {
  box-sizing: border-box;
}

.homepage-content {
  flex: 1;
  width: 100%;
  max-width: 1180px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: clamp(1.75rem, 2.6vw, 2.75rem);
}

.top-bar {
  display: flex;
  justify-content: flex-end;
}

.theme-toggle-button {
  width: 3rem;
  height: 3rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 2px solid var(--dynamic-border);
  border-radius: 0;
  background-color: var(--dynamic-surface);
  color: var(--dynamic-text);
  transition:
    background-color 200ms ease,
    color 200ms ease,
    border-color 200ms ease;
  cursor: pointer;
}

.theme-toggle-button:hover,
.theme-toggle-button:focus-visible {
  border-color: var(--dynamic-accent);
  color: var(--dynamic-accent);
  outline: none;
}

.theme-toggle-button svg {
  width: 1.5rem;
  height: 1.5rem;
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

.brutal-panel {
  border: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-surface);
  border-radius: 0;
  display: flex;
  flex-direction: column;
  gap: 0;
}

.hero-panel {
  gap: clamp(1.25rem, 2vw, 1.75rem);
  padding: clamp(1.75rem, 2.2vw, 2.5rem);
}

.hero-heading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-width: 36rem;
}

.hero-title {
  font-size: clamp(2rem, 3vw, 2.75rem);
  font-weight: 700;
  letter-spacing: -0.01em;
  margin: 0;
}

.hero-subtitle {
  font-size: 1rem;
  line-height: 1.6;
  color: var(--dynamic-muted);
  margin: 0;
}

.quick-access-wrapper {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.quick-access-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
}

.quick-access-item {
  flex: 1 1 min(16rem, 100%);
  min-width: 12rem;
  display: flex;
  align-items: center;
  gap: 1rem;
  border: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-surface-variant);
  padding: 0.9rem 1.1rem;
  cursor: pointer;
  border-radius: 0;
  transition:
    border-color 200ms ease,
    background-color 200ms ease;
}

.quick-access-item:hover,
.quick-access-item:focus-visible {
  border-color: var(--dynamic-accent);
  outline: none;
}

.quick-access-favicon {
  width: 48px;
  height: 48px;
  border: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-surface);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border-radius: 0;
}

.quick-access-favicon-img {
  width: 70%;
  height: 70%;
  object-fit: contain;
}

.quick-access-fallback {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--dynamic-muted);
}

.quick-access-meta {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.quick-access-title {
  font-size: 1rem;
  font-weight: 600;
  color: var(--dynamic-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.quick-access-domain {
  font-size: 0.85rem;
  color: var(--dynamic-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.quick-access-visits {
  font-size: 0.82rem;
  color: var(--dynamic-accent);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  font-weight: 600;
  flex-shrink: 0;
}

.loading {
  padding: 1.25rem;
  text-align: center;
  color: var(--dynamic-muted);
  border: 2px dashed var(--dynamic-border);
  background-color: var(--dynamic-bg);
}

.empty-state {
  border: 2px dashed var(--dynamic-border);
  padding: 1.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  text-align: center;
  align-items: center;
  color: var(--dynamic-muted);
  background-color: var(--dynamic-bg);
}

.empty-state h3 {
  margin: 0;
  color: var(--dynamic-text);
  font-size: 1.1rem;
}

.empty-state p {
  margin: 0;
  color: var(--dynamic-muted);
}

.empty-state.compact {
  padding: 1.25rem;
}

/* Main panels with height matching */
.main-panels {
  display: flex;
  flex-wrap: wrap;
  gap: clamp(1.5rem, 2vw, 2rem);
  align-items: flex-start;
}

.history-panel {
  flex: 1 1 55%;
  min-width: min(32rem, 100%);
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.insights-panel {
  flex: 1 1 35%;
  min-width: min(24rem, 100%);
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.panel-header {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  padding: clamp(1.25rem, 2vw, 1.75rem) clamp(1.5rem, 2.5vw, 2rem);
  border-bottom: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-surface);
}

.panel-title {
  margin: 0;
  font-size: 1.3rem;
  font-weight: 600;
}

.panel-subtitle {
  margin: 0;
  font-size: 0.95rem;
  color: var(--dynamic-muted);
}

.panel-body {
  padding: clamp(1.25rem, 2vw, 1.75rem) clamp(1.5rem, 2.5vw, 2rem);
  display: flex;
  flex-direction: column;
  gap: 1.1rem;
  min-height: 0;
  overflow: hidden;
}

.history-body {
  gap: 1rem;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.history-list {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding-right: 0.5rem;
}

.history-item {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
  border: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-surface-variant);
  padding: 1rem 1.1rem;
  cursor: pointer;
  min-height: 80px;
  border-radius: 0;
  transition:
    border-color 200ms ease,
    background-color 200ms ease;
}

.history-item:hover,
.history-item:focus-visible {
  border-color: var(--dynamic-accent);
  background-color: var(--dynamic-bg);
  outline: none;
}

.history-item.deleting {
  opacity: 0.4;
  pointer-events: none;
  transform: translateX(6px);
}

.history-item-leading {
  width: 44px;
  height: 44px;
  border: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-surface);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border-radius: 0;
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

.history-item-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
  justify-content: center;
}

.history-item-top {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.history-title {
  font-weight: 600;
  color: var(--dynamic-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.history-time {
  font-size: 0.85rem;
  color: var(--dynamic-muted);
  flex-shrink: 0;
}

.history-item-bottom {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  overflow: hidden;
  position: relative;
}

.history-domain {
  font-size: 0.9rem;
  font-weight: 500;
  color: var(--dynamic-text);
  white-space: nowrap;
  flex-shrink: 0;
}

.history-url {
  font-size: 0.8rem;
  color: var(--dynamic-muted);
  white-space: nowrap;
  font-family:
    "JetBrains Mono",
    "SFMono-Regular",
    Menlo,
    monospace;
  overflow: hidden;
  flex: 1;
  position: relative;
  mask-image: linear-gradient(to right, black 70%, transparent 100%);
  -webkit-mask-image: linear-gradient(to right, black 70%, transparent 100%);
}

.history-delete {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  border-left: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-surface);
  color: var(--dynamic-muted);
  font-size: 1.1rem;
  transition: background-color 200ms ease, color 200ms ease;
  flex-shrink: 0;
}

.history-delete:hover,
.history-delete:focus-visible {
  color: var(--dynamic-accent);
  background-color: var(--dynamic-bg);
  outline: none;
}

.scroll-sentinel {
  width: 100%;
  height: 1px;
}

.loading-more {
  padding: 0.75rem;
  text-align: center;
  color: var(--dynamic-muted);
  font-size: 0.85rem;
}

.insights-body {
  gap: 1.5rem;
}

.insight-block {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
}

.block-title {
  font-size: 0.85rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--dynamic-muted);
  margin: 0;
}

.insights-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.insight-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  border: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-surface-variant);
  padding: 0.85rem 1rem;
  cursor: pointer;
  border-radius: 0;
  transition:
    border-color 200ms ease,
    background-color 200ms ease;
}

.insight-item:hover,
.insight-item:focus-visible {
  border-color: var(--dynamic-accent);
  outline: none;
}

.insight-rank {
  width: 2rem;
  height: 2rem;
  border: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-surface);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 600;
  font-size: 0.9rem;
  color: var(--dynamic-accent);
  flex-shrink: 0;
  border-radius: 0;
}

.insight-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.insight-title {
  font-weight: 600;
  color: var(--dynamic-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.insight-domain {
  font-size: 0.85rem;
  color: var(--dynamic-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.insight-visits {
  font-size: 0.78rem;
  color: var(--dynamic-accent);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  font-weight: 600;
  flex-shrink: 0;
}

.stats-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 0.85rem;
}

.stat-item {
  flex: 1 1 160px;
  border: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-surface-variant);
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  border-radius: 0;
}

.stat-value {
  font-size: 1.4rem;
  font-weight: 600;
  color: var(--dynamic-text);
}

.stat-label {
  font-size: 0.8rem;
  color: var(--dynamic-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.shortcuts-panel {
  gap: 0;
}

.shortcuts-list {
  display: flex;
  flex-wrap: wrap;
  gap: 0.85rem;
}

.shortcut-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  border: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-bg);
  padding: 0.75rem 1rem;
  border-radius: 0;
}

.shortcut-key-badge {
  font-family:
    "JetBrains Mono",
    "SFMono-Regular",
    Menlo,
    monospace;
  font-size: 0.85rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  padding: 0.35rem 0.65rem;
  border: 2px solid var(--dynamic-border);
  background-color: var(--dynamic-surface-variant);
  color: var(--dynamic-muted);
  font-weight: 500;
  border-radius: 0;
}

.shortcut-desc {
  font-size: 0.95rem;
  color: var(--dynamic-text);
}

.shortcuts-panel .loading {
  border-style: solid;
}

/* Responsive Design */
@media (max-width: 1200px) {
  .history-panel,
  .insights-panel {
    min-width: min(28rem, 100%);
  }
}

@media (max-width: 960px) {
  .homepage-shell {
    padding: clamp(1.25rem, 5vw, 2rem);
  }

  .main-panels {
    flex-direction: column;
  }

  .history-panel,
  .insights-panel {
    min-width: 100%;
  }

  .quick-access-item {
    flex: 1 1 calc(50% - 1rem);
  }
}

@media (max-width: 640px) {
  .theme-toggle-button {
    width: 2.75rem;
    height: 2.75rem;
  }

  .panel-body,
  .panel-header,
  .hero-panel {
    padding-left: 1.25rem;
    padding-right: 1.25rem;
  }

  .quick-access-item {
    flex: 1 1 100%;
  }

  .history-item {
    flex-direction: column;
    align-items: flex-start;
  }

  .history-delete {
    width: 100%;
    border-left: 0;
    border-top: 2px solid var(--dynamic-border);
    align-self: stretch;
  }

  .history-item-top {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.35rem;
  }

  .history-time {
    order: 3;
  }
}
</style>
