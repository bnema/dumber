<script lang="ts">
  import { onMount } from 'svelte';

  interface HistoryItem {
    id: number;
    url: string;
    title: string;
    visit_count: number;
    last_visited: string;
    created_at: string;
  }

  interface Shortcut {
    key: string;
    description: string;
    action: () => void;
  }

  let historyItems = $state<HistoryItem[]>([]);
  let shortcuts = $state<Shortcut[]>([]);
  let historyLoading = $state(true);
  let shortcutsLoading = $state(true);

  // Initialize shortcuts (these are browser shortcuts, not dynamic from API)
  const initializeShortcuts = () => {
    shortcuts = [
      {
        key: 'Ctrl+L',
        description: 'Focus address bar',
        action: () => {
          // This will be handled by the browser's keyboard shortcuts
          console.log('Ctrl+L triggered');
        }
      },
      {
        key: 'Ctrl+F',
        description: 'Find in page',
        action: () => {
          // This will be handled by the browser's keyboard shortcuts
          console.log('Ctrl+F triggered');
        }
      },
      {
        key: 'Ctrl+R',
        description: 'Reload page',
        action: () => {
          window.location.reload();
        }
      },
      {
        key: 'Ctrl+0',
        description: 'Reset zoom',
        action: () => {
          // This will be handled by the browser's zoom controls
          console.log('Ctrl+0 triggered');
        }
      },
      {
        key: 'Ctrl++',
        description: 'Zoom in',
        action: () => {
          // This will be handled by the browser's zoom controls
          console.log('Ctrl++ triggered');
        }
      },
      {
        key: 'Ctrl+-',
        description: 'Zoom out',
        action: () => {
          // This will be handled by the browser's zoom controls
          console.log('Ctrl+- triggered');
        }
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

  onMount(() => {
    initializeShortcuts();
    fetchHistory();
  });

  // Combined loading state
  const loading = $derived(historyLoading || shortcutsLoading);
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
            role="button"
            tabindex="0"
            onclick={() => navigateTo(item.url)}
            onkeydown={(e) => e.key === 'Enter' && navigateTo(item.url)}
          >
            <div class="history-line">
              <div class="history-favicon-chip">
                <div class="history-favicon-fallback">🌐</div>
              </div>
              <div class="history-title">{item.title || 'Untitled'}</div>
              <div class="history-sep">•</div>
              <div class="history-domain">{getDomain(item.url)}</div>
              <div class="history-sep">•</div>
              <div class="history-url">{item.url}</div>
              <div class="history-time">{formatTime(item.last_visited)}</div>
            </div>
          </div>
        {/each}
      {/if}
    </div>
  </div>

  <div class="shortcuts-section">
    <h2 class="section-title">Keyboard Shortcuts</h2>
    <div class="shortcuts-grid">
      {#if shortcutsLoading}
        <div class="loading">Loading shortcuts...</div>
      {:else}
        {#each shortcuts as shortcut (shortcut.key)}
          <div
            class="shortcut"
            role="button"
            tabindex="0"
            onclick={shortcut.action}
            onkeydown={(e) => e.key === 'Enter' && shortcut.action()}
          >
            <div class="shortcut-key">{shortcut.key}</div>
            <div class="shortcut-desc">{shortcut.description}</div>
          </div>
        {/each}
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
    flex: 1;
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
    transition: background 0.2s;
    border-left: 3px solid #404040;
    overflow: hidden;
    max-width: 100%;
  }

  .history-item:hover {
    background: #3d3d3d;
    border-left-color: #0066cc;
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

  .shortcuts-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
    max-height: calc(100vh - 8rem);
    overflow-y: auto;
  }

  .shortcut {
    padding: 1rem;
    background: #2d2d2d;
    border: 2px solid #404040;
    border-radius: 8px;
    cursor: pointer;
    transition: all 0.2s;
    text-align: center;
  }

  .shortcut:hover {
    background: #3d3d3d;
    border-color: #0066cc;
    transform: translateY(-2px);
  }

  .shortcut-key {
    font-weight: bold;
    color: #0066cc;
    margin-bottom: 0.5rem;
    font-size: 1.1rem;
  }

  .shortcut-desc {
    color: #888;
    font-size: 0.9rem;
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

    .shortcuts-grid {
      grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    }
  }
</style>