<script lang="ts">
  import { onMount } from 'svelte';

  let blockedUrl = $state('');
  let blockedDomain = $state('');
  let reason = $state('Content blocked by filter');
  let isLoading = $state(false);
  let error = $state('');

  onMount(() => {
    // Parse URL parameters
    const params = new URLSearchParams(window.location.search);
    blockedUrl = params.get('url') || '';
    reason = params.get('reason') || 'Content blocked by filter';

    // Extract domain from URL
    try {
      if (blockedUrl) {
        const url = new URL(blockedUrl);
        blockedDomain = url.hostname;
      }
    } catch {
      blockedDomain = blockedUrl;
    }
  });

  function sendMessage(type: string, payload: Record<string, unknown>): Promise<unknown> {
    return new Promise((resolve, reject) => {
      const requestId = `blocked_${Date.now()}_${Math.random().toString(36).slice(2)}`;

      const handler = (event: MessageEvent) => {
        if (event.data?.requestId === requestId) {
          window.removeEventListener('message', handler);
          if (event.data.success) {
            resolve(event.data.data);
          } else {
            reject(new Error(event.data.error || 'Unknown error'));
          }
        }
      };

      window.addEventListener('message', handler);

      // Send to WebKit message handler
      if (window.webkit?.messageHandlers?.dumber) {
        window.webkit.messageHandlers.dumber.postMessage(
          JSON.stringify({ type, requestId, payload })
        );
      } else {
        reject(new Error('WebKit message handler not available'));
      }

      // Timeout after 10 seconds
      setTimeout(() => {
        window.removeEventListener('message', handler);
        reject(new Error('Request timeout'));
      }, 10000);
    });
  }

  async function addToWhitelist() {
    if (!blockedDomain) return;

    isLoading = true;
    error = '';

    try {
      await sendMessage('addToWhitelist', { domain: blockedDomain });
      // Reload the original URL after whitelisting
      window.location.href = blockedUrl;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to add to whitelist';
      isLoading = false;
    }
  }

  async function bypassOnce() {
    if (!blockedUrl) return;

    isLoading = true;
    error = '';

    try {
      await sendMessage('bypassOnce', { url: blockedUrl });
      // Reload the original URL after bypass
      window.location.href = blockedUrl;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to bypass';
      isLoading = false;
    }
  }

  function goBack() {
    window.history.back();
  }
</script>

<svelte:head>
  <title>Page Blocked</title>
  {@html `<style>
    html, body {
      background: var(--background, #0a0a0a);
      margin: 0;
      padding: 0;
      min-height: 100vh;
    }
  </style>`}
</svelte:head>

<div class="blocked-container">
  <div class="blocked-card">
    <div class="icon-container">
      <svg class="shield-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
        <path d="M9 12l2 2 4-4"/>
      </svg>
    </div>

    <h1 class="title">Page Blocked</h1>

    <p class="reason">{reason}</p>

    {#if blockedUrl}
      <div class="url-display">
        <span class="url-label">Blocked URL:</span>
        <code class="url-value">{blockedUrl}</code>
      </div>
    {/if}

    {#if error}
      <div class="error-message">{error}</div>
    {/if}

    <div class="actions">
      <button
        class="btn btn-primary"
        onclick={addToWhitelist}
        disabled={isLoading || !blockedDomain}
      >
        {#if isLoading}
          <span class="loading-spinner"></span>
        {:else}
          Add to Whitelist
        {/if}
      </button>

      <button
        class="btn btn-secondary"
        onclick={bypassOnce}
        disabled={isLoading || !blockedUrl}
      >
        Go Anyway (Once)
      </button>

      <button
        class="btn btn-ghost"
        onclick={goBack}
        disabled={isLoading}
      >
        Go Back
      </button>
    </div>

    {#if blockedDomain}
      <p class="domain-note">
        Adding <strong>{blockedDomain}</strong> to whitelist will disable content filtering for this site.
      </p>
    {/if}
  </div>
</div>

<style>
  .blocked-container {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2rem;
    background: var(--background, #0a0a0a);
    color: var(--foreground, #e5e5e5);
    font-family: 'Inter', system-ui, -apple-system, sans-serif;
  }

  .blocked-card {
    max-width: 480px;
    width: 100%;
    padding: 2.5rem 2rem;
    background: var(--card, #141414);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border, #262626);
    text-align: center;
  }

  .icon-container {
    margin-bottom: 1.5rem;
  }

  .shield-icon {
    width: 64px;
    height: 64px;
    color: var(--primary, #3b82f6);
    opacity: 0.9;
  }

  .title {
    font-size: 1.5rem;
    font-weight: 600;
    margin: 0 0 0.75rem 0;
    color: var(--foreground, #e5e5e5);
  }

  .reason {
    font-size: 0.9375rem;
    color: var(--muted-foreground, #737373);
    margin: 0 0 1.5rem 0;
  }

  .url-display {
    background: var(--background, #0a0a0a);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border, #262626);
    padding: 0.75rem 1rem;
    margin-bottom: 1.5rem;
    text-align: left;
  }

  .url-label {
    display: block;
    font-size: 0.75rem;
    color: var(--muted-foreground, #737373);
    margin-bottom: 0.25rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .url-value {
    font-family: 'JetBrains Mono NF', monospace;
    font-size: 0.8125rem;
    color: var(--foreground, #e5e5e5);
    word-break: break-all;
    display: block;
  }

  .error-message {
    background: rgba(239, 68, 68, 0.1);
    border: 1px solid rgba(239, 68, 68, 0.3);
    color: #ef4444;
    padding: 0.75rem 1rem;
    margin-bottom: 1.5rem;
    font-size: 0.875rem;
  }

  .actions {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-bottom: 1.5rem;
  }

  .btn {
    padding: 0.75rem 1.5rem;
    font-size: 0.875rem;
    font-weight: 500;
    border: 1px solid transparent;
    cursor: pointer;
    transition: all 150ms ease;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
  }

  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn-primary {
    background: var(--primary, #3b82f6);
    color: white;
    border-color: var(--primary, #3b82f6);
  }

  .btn-primary:hover:not(:disabled) {
    filter: brightness(1.1);
  }

  .btn-secondary {
    background: transparent;
    color: var(--foreground, #e5e5e5);
    border-color: var(--border, #262626);
  }

  .btn-secondary:hover:not(:disabled) {
    background: var(--secondary, #1a1a1a);
    border-color: var(--muted-foreground, #737373);
  }

  .btn-ghost {
    background: transparent;
    color: var(--muted-foreground, #737373);
    border-color: transparent;
  }

  .btn-ghost:hover:not(:disabled) {
    color: var(--foreground, #e5e5e5);
  }

  .loading-spinner {
    width: 1rem;
    height: 1rem;
    border: 2px solid transparent;
    border-top-color: currentColor;
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .domain-note {
    font-size: 0.75rem;
    color: var(--muted-foreground, #737373);
    margin: 0;
  }

  .domain-note strong {
    color: var(--foreground, #e5e5e5);
    font-weight: 500;
  }
</style>
