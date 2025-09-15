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
  let deletingIds = $state<Set<number>>(new Set());

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

  // Fetch recent history from the API
  const fetchHistory = async () => {
    try {
      const response = await fetch('dumb://homepage/api/history/recent?limit=20');
      if (response.ok) {
        const data = await response.json();
        historyItems = Array.isArray(data) ? data : [];
      } else {
        console.warn('Failed to fetch history:', response.status);
        historyItems = [];
      }
    } catch (error) {
      console.error('Error fetching history:', error);
      historyItems = [];
    } finally {
      historyLoading = false;
    }
  };

  // Navigate to a URL when history item is clicked
  const navigateTo = (url: string) => {
    window.location.href = url;
  };

  // Delete a history entry
  const deleteHistoryEntry = async (id: number, event: Event) => {
    event.stopPropagation(); // Prevent navigation

    // Add to deleting set for animation
    deletingIds = new Set([...deletingIds, id]);

    try {
      const response = await fetch(`dumb://homepage/api/history/delete?id=${id}`);
      if (response.ok) {
        // Wait a bit for the fade animation to complete
        await new Promise(resolve => setTimeout(resolve, 200));

        // Refresh history after successful deletion
        await fetchHistory();
        await fetchStats();
        await fetchTopVisited();
      } else {
        console.error('Failed to delete history entry:', response.status);
        // Remove from deleting set if failed
        deletingIds = new Set([...deletingIds].filter(deletingId => deletingId !== id));
      }
    } catch (error) {
      console.error('Error deleting history entry:', error);
      // Remove from deleting set if failed
      deletingIds = new Set([...deletingIds].filter(deletingId => deletingId !== id));
    }

    // Remove from deleting set after refresh
    deletingIds = new Set([...deletingIds].filter(deletingId => deletingId !== id));
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

  // Fetch history stats from the API
  const fetchStats = async () => {
    try {
      const response = await fetch('dumb://homepage/api/history/stats');
      if (response.ok) {
        const data = await response.json();
        stats = data;
      } else {
        console.warn('Failed to fetch stats:', response.status);
        stats = null;
      }
    } catch (error) {
      console.error('Error fetching stats:', error);
      stats = null;
    } finally {
      statsLoading = false;
    }
  };

  // Fetch top visited sites using search endpoint sorted by visit count
  const fetchTopVisited = async () => {
    try {
      const response = await fetch('dumb://homepage/api/history/search?q=&limit=5');
      if (response.ok) {
        const data = await response.json();
        topVisited = Array.isArray(data) ? data.slice(0, 5) : [];
      } else {
        console.warn('Failed to fetch top visited:', response.status);
        topVisited = [];
      }
    } catch (error) {
      console.error('Error fetching top visited:', error);
      topVisited = [];
    } finally {
      topVisitedLoading = false;
    }
  };

  onMount(() => {
    initializeShortcuts();
    fetchHistory();
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
            class:deleting={deletingIds.has(item.id)}
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