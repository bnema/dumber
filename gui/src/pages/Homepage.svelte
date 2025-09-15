<script lang="ts">
  import { onMount } from 'svelte';
  import { Trash2, Globe } from '@lucide/svelte';

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

  // Function for checking if an item is being deleted
  const isDeleting = (id: number) => deletingIds.includes(id);

  // Initialize shortcuts (these are browser shortcuts, not dynamic from API)
  const initializeShortcuts = () => {
    shortcuts = [
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
        key: 'Ctrl+R',
        description: 'Reload page',
        action: () => window.location.reload()
      },
      {
        key: 'Ctrl+Shift+R',
        description: 'Hard reload',
        action: () => console.log('Ctrl+Shift+R triggered')
      },
      {
        key: 'F5',
        description: 'Reload page',
        action: () => window.location.reload()
      },
      {
        key: 'F12',
        description: 'DevTools',
        action: () => console.log('F12 triggered')
      },
      {
        key: 'Alt+←',
        description: 'Navigate back',
        action: () => console.log('Alt+← triggered')
      },
      {
        key: 'Alt+→',
        description: 'Navigate forward',
        action: () => console.log('Alt+→ triggered')
      },
      {
        key: 'Ctrl+0',
        description: 'Reset zoom',
        action: () => console.log('Ctrl+0 triggered')
      },
      {
        key: 'Ctrl++',
        description: 'Zoom in',
        action: () => console.log('Ctrl++ triggered')
      },
      {
        key: 'Ctrl+-',
        description: 'Zoom out',
        action: () => console.log('Ctrl+- triggered')
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
        topVisited = Array.isArray(data) ? data.slice(0, 5) : [];
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
          limit: 5
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
  });

</script>

<div class="homepage-container">
<div class="container">
  <div class="history-section">
    <h2 class="section-title">Recent History</h2>
    <div class="history-list">
      {#if historyLoading}
        <div class="loading">Loading history...</div>
      {:else if historyItems.length === 0}
        <div class="empty-state">
          <h3>No history yet</h3>
          <p>Start browsing to see your recent history here.</p>
        </div>
      {:else}
        {#each historyItems as item (item.url)}
          <div
            class="history-item"
            class:deleting={isDeleting(item.id)}
            role="button"
            tabindex="0"
            onclick={() => navigateTo(item.url)}
            onkeydown={(e) => e.key === 'Enter' && navigateTo(item.url)}
          >
            <div class="history-line">
              <div class="history-favicon-chip">
                {#if item.favicon_url}
                  <img
                    src={item.favicon_url}
                    alt=""
                    class="history-favicon-img"
                    onerror={(e) => {
                      const target = e.target as HTMLImageElement;
                      if (target) {
                        target.style.display='none';
                        const fallback = target.nextElementSibling as HTMLElement;
                        if (fallback) fallback.style.display='block';
                      }
                    }}
                    onload={(e) => {
                      const target = e.target as HTMLImageElement;
                      if (target) {
                        const fallback = target.nextElementSibling as HTMLElement;
                        if (fallback) fallback.style.display='none';
                      }
                    }}
                  />
                  <div class="history-favicon-fallback" style="display: none;">
                    <Globe size={12} />
                  </div>
                {:else}
                  <div class="history-favicon-fallback">
                    <Globe size={12} />
                  </div>
                {/if}
              </div>
              <div class="history-title">{item.title || 'Untitled'}</div>
              <div class="history-sep">•</div>
              <div class="history-domain">{getDomain(item.url)}</div>
              <div class="history-sep">•</div>
              <div class="history-url">{item.url}</div>
              <div class="history-time">{formatTime(item.last_visited)}</div>
              <div
                class="history-delete"
                role="button"
                tabindex="0"
                onclick={(e) => deleteHistoryEntry(item.id, e)}
                onkeydown={(e) => e.key === 'Enter' && deleteHistoryEntry(item.id, e)}
                title="Delete this entry"
              >
                <Trash2 size={16} />
              </div>
            </div>
          </div>
        {/each}

        <!-- Infinite scroll sentinel and loading indicator -->
        {#if hasMoreHistory}
          <div bind:this={historyScrollSentinel} class="scroll-sentinel"></div>
          {#if loadingMoreHistory}
            <div class="loading-more">Loading more history...</div>
          {/if}
        {/if}
      {/if}
    </div>
  </div>

  <div class="shortcuts-section">
    <h2 class="section-title">Keyboard Shortcuts</h2>
    <div class="shortcuts-container">
      {#if shortcutsLoading}
        <div class="loading">Loading shortcuts...</div>
      {:else}
        <table class="shortcuts-table">
          <tbody>
            {#each shortcuts as shortcut (shortcut.key)}
              <tr class="shortcut-row">
                <td class="shortcut-key">{shortcut.key}</td>
                <td class="shortcut-desc">{shortcut.description}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>

    <h2 class="section-title stats-title">Statistics</h2>
    <div class="stats-container">
      {#if statsLoading}
        <div class="loading">Loading stats...</div>
      {:else if stats}
        <div class="stats-grid">
          <div class="stat-item">
            <div class="stat-value">{stats.total_entries}</div>
            <div class="stat-label">History Entries</div>
          </div>
        </div>
      {:else}
        <div class="empty-state">
          <p>No statistics available</p>
        </div>
      {/if}
    </div>

    <h2 class="section-title top-visited-title">Top 5 Most Visited</h2>
    <div class="top-visited-container">
      {#if topVisitedLoading}
        <div class="loading">Loading top visited...</div>
      {:else if topVisited.length === 0}
        <div class="empty-state">
          <p>No visit data available</p>
        </div>
      {:else}
        <div class="top-visited-list">
          {#each topVisited as item, index (item.url)}
            <div
              class="top-visited-item"
              role="button"
              tabindex="0"
              onclick={() => navigateTo(item.url)}
              onkeydown={(e) => e.key === 'Enter' && navigateTo(item.url)}
            >
              <div class="rank">{index + 1}</div>
              <div class="site-info">
                <div class="site-title">{item.title || 'Untitled'}</div>
                <div class="site-domain">{getDomain(item.url)}</div>
              </div>
              <div class="visit-count">{item.visit_count}</div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>
</div>

<style>
  /* Scoped homepage container to prevent global CSS conflicts */
  .homepage-container {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #1a1a1a;
    color: #ffffff;
    min-height: 100vh;
    overflow-x: hidden;
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    box-sizing: border-box;
  }

  /* Reset styles for children, but scoped to homepage only */
  .homepage-container * {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
  }

  .container {
    flex: 1;
    display: flex;
    max-width: 1200px;
    margin: 0 auto;
    width: 100%;
    padding: 2rem;
    gap: 2rem;
    overflow-x: hidden;
  }

  .history-section {
    flex: 2;
    min-height: 0;
    min-width: 0;
  }

  .shortcuts-section {
    flex: 1;
    min-height: 0;
    min-width: 0;
  }

  .section-title {
    font-size: 1.5rem;
    margin-bottom: 1rem;
    color: #ffffff;
    border-bottom: 2px solid #404040;
    padding-bottom: 0.5rem;
  }

  .history-list {
    overflow-y: auto;
    overflow-x: hidden;
    max-height: calc(100vh - 8rem);
    max-width: 100%;
  }

  .history-item {
    padding: 0.75rem;
    margin-bottom: 0.5rem;
    background: #2d2d2d;
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.2s ease-in-out;
    border-left: 3px solid #404040;
    overflow: hidden;
    max-width: 100%;
    position: relative;
    opacity: 1;
    transform: translateX(0);
  }

  .history-item:hover {
    background: #3d3d3d;
    border-left-color: #0066cc;
  }

  .history-item:hover .history-delete {
    opacity: 1;
    visibility: visible;
  }

  .history-item.deleting {
    opacity: 0;
    transform: translateX(-20px);
    pointer-events: none;
  }

  .history-line {
    display: flex;
    gap: 0.5rem;
    white-space: nowrap;
    align-items: center;
    overflow: hidden;
    width: 100%;
    min-width: 0;
  }

  .history-favicon-chip {
    flex: 0 0 20px;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: #ccc;
    border: 1px solid rgba(0,0,0,.12);
    box-shadow: 0 1px 2px rgba(0,0,0,.12);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .history-favicon-img {
    width: 16px;
    height: 16px;
    filter: brightness(1.06) contrast(1.03);
    image-rendering: -webkit-optimize-contrast;
  }

  .history-favicon-fallback {
    font-size: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: #888;
  }

  .history-title {
    font-size: 0.95rem;
    color: #e6e6e6;
    flex: 0 0 auto;
  }

  .history-domain {
    font-size: 0.9rem;
    color: #a5a5a5;
    flex: 0 0 auto;
  }

  .history-sep {
    color: #666;
    flex: 0 0 auto;
  }

  .history-url {
    font-size: 0.9rem;
    color: #7a7a7a;
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    -webkit-mask-image: linear-gradient(to right, rgba(0,0,0,1) 85%, rgba(0,0,0,0) 100%);
    mask-image: linear-gradient(to right, rgba(0,0,0,1) 85%, rgba(0,0,0,0) 100%);
  }

  .history-time {
    font-size: 0.85rem;
    color: #666;
    flex: 0 0 auto;
    margin-left: auto;
  }

  .history-delete {
    position: absolute;
    right: 0.75rem;
    top: 50%;
    transform: translateY(-50%);
    opacity: 0;
    visibility: hidden;
    transition: opacity 0.2s, visibility 0.2s, transform 0.2s;
    cursor: pointer;
    background: #1a1a1a;
    border-radius: 3px;
    padding: 0.25rem;
    border: 1px solid #404040;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2rem;
    height: 2rem;
    color: #888;
  }

  .history-delete:hover {
    background: #ff4444;
    border-color: #ff6666;
    color: #fff;
    transform: translateY(-50%) scale(1.1);
  }

  .shortcuts-container {
    max-height: calc(100vh - 8rem);
    overflow-y: auto;
  }

  .shortcuts-table {
    width: 100%;
    border-collapse: collapse;
    background: #2d2d2d;
    border-radius: 6px;
    overflow: hidden;
  }

  .shortcut-row {
    border-bottom: 1px solid #404040;
    transition: background 0.2s;
  }

  .shortcut-row:last-child {
    border-bottom: none;
  }

  .shortcut-row:hover {
    background: #3d3d3d;
  }

  .shortcut-key {
    padding: 0.4rem 0.8rem;
    font-weight: bold;
    color: #0066cc;
    font-size: 0.85rem;
    font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
    white-space: nowrap;
    width: 1%;
  }

  .shortcut-desc {
    padding: 0.4rem 0.8rem;
    color: #e6e6e6;
    font-size: 0.85rem;
  }

  .stats-title {
    margin-top: 2rem;
  }

  .stats-container {
    margin-top: 1rem;
  }

  .stats-grid {
    display: flex;
    gap: 1rem;
  }

  .stat-item {
    background: #2d2d2d;
    border-radius: 6px;
    padding: 1rem;
    text-align: center;
    flex: 1;
    border: 1px solid #404040;
  }

  .stat-value {
    font-size: 1.5rem;
    font-weight: bold;
    color: #0066cc;
    margin-bottom: 0.25rem;
  }

  .stat-label {
    font-size: 0.8rem;
    color: #888;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .top-visited-title {
    margin-top: 2rem;
  }

  .top-visited-container {
    margin-top: 1rem;
  }

  .top-visited-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .top-visited-item {
    display: flex;
    align-items: center;
    padding: 0.75rem;
    background: #2d2d2d;
    border-radius: 6px;
    cursor: pointer;
    transition: background 0.2s;
    border: 1px solid #404040;
    gap: 1rem;
  }

  .top-visited-item:hover {
    background: #3d3d3d;
    border-color: #0066cc;
  }

  .rank {
    font-weight: bold;
    color: #0066cc;
    font-size: 1.1rem;
    min-width: 1.5rem;
    text-align: center;
  }

  .site-info {
    flex: 1;
    min-width: 0;
  }

  .site-title {
    font-size: 0.9rem;
    color: #e6e6e6;
    margin-bottom: 0.2rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .site-domain {
    font-size: 0.8rem;
    color: #888;
  }

  .visit-count {
    font-size: 0.85rem;
    color: #0066cc;
    font-weight: bold;
    min-width: 2rem;
    text-align: center;
  }

  .loading {
    padding: 2rem;
    text-align: center;
    color: #888;
  }

  .scroll-sentinel {
    height: 1px;
    width: 100%;
  }

  .loading-more {
    padding: 1rem;
    text-align: center;
    color: #888;
    font-size: 0.9rem;
  }

  .empty-state {
    padding: 3rem 2rem;
    text-align: center;
    color: #666;
  }

  .empty-state h3 {
    margin-bottom: 1rem;
    color: #888;
  }

  @media (max-width: 768px) {
    .container {
      flex-direction: column;
      padding: 1rem;
    }
  }
</style>