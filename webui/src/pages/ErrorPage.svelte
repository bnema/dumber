<script lang="ts">
  import { onMount } from 'svelte';
  import { ModeWatcher } from 'mode-watcher';
  import { Button } from '$lib/components/ui/button';
  import * as Card from '$lib/components/ui/card';

  type ErrorConfig = {
    code: number;
    title: string;
    message: string;
    icon: string;
    actions: string[];
  };

  const defaultConfig: ErrorConfig = {
    code: 404,
    title: 'Page Not Found',
    message: 'The page you are looking for does not exist.',
    icon: 'warning',
    actions: ['back'],
  };

  // Error configurations
  const errorConfigs: Record<string, ErrorConfig> = {
    blocked: { code: 0, title: 'Page Blocked', message: 'Content blocked by filter', icon: 'shield', actions: ['whitelist', 'bypass', 'back'] },
    '404': defaultConfig,
    '500': { code: 500, title: 'Server Error', message: 'Something went wrong on the server.', icon: 'error', actions: ['retry', 'back'] },
    '502': { code: 502, title: 'Bad Gateway', message: 'The server received an invalid response.', icon: 'error', actions: ['retry', 'back'] },
    '503': { code: 503, title: 'Service Unavailable', message: 'The server is temporarily unavailable.', icon: 'error', actions: ['retry', 'back'] },
    offline: { code: 0, title: 'You Are Offline', message: 'Check your internet connection.', icon: 'offline', actions: ['retry', 'back'] },
    ssl: { code: 0, title: 'Connection Not Secure', message: 'The security certificate is invalid.', icon: 'lock', actions: ['bypass', 'back'] },
    timeout: { code: 0, title: 'Connection Timed Out', message: 'The server took too long to respond.', icon: 'warning', actions: ['retry', 'back'] },
  };

  let config = $state<ErrorConfig>(defaultConfig);
  let targetUrl = $state('');
  let targetDomain = $state('');
  let customMessage = $state('');
  let isLoading = $state(false);
  let actionError = $state('');

  onMount(() => {
    const params = new URLSearchParams(window.location.search);
    const typeParam = params.get('type') || params.get('code') || '404';

    const selectedConfig = errorConfigs[typeParam];
    config = selectedConfig ?? {
      code: parseInt(typeParam) || 0,
      title: `Error ${typeParam}`,
      message: 'An unexpected error occurred.',
      icon: 'error',
      actions: ['retry', 'back'],
    };

    targetUrl = params.get('url') || '';
    customMessage = params.get('reason') || params.get('message') || '';

    if (targetUrl) {
      try {
        targetDomain = new URL(targetUrl).hostname;
      } catch {
        targetDomain = targetUrl;
      }
    }
  });

  function sendMessage(type: string, payload: Record<string, unknown>): Promise<unknown> {
    return new Promise((resolve, reject) => {
      const requestId = `error_${Date.now()}_${Math.random().toString(36).slice(2)}`;
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
      if (window.webkit?.messageHandlers?.dumber) {
        window.webkit.messageHandlers.dumber.postMessage(JSON.stringify({ type, requestId, payload }));
      } else {
        reject(new Error('WebKit message handler not available'));
      }
      setTimeout(() => { window.removeEventListener('message', handler); reject(new Error('Timeout')); }, 10000);
    });
  }

  async function addToWhitelist() {
    if (!targetDomain) return;
    isLoading = true;
    actionError = '';
    try {
      await sendMessage('addToWhitelist', { domain: targetDomain });
      window.location.href = targetUrl;
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed';
      isLoading = false;
    }
  }

  async function bypassOnce() {
    if (!targetUrl) return;
    isLoading = true;
    actionError = '';
    try {
      await sendMessage('bypassOnce', { url: targetUrl });
      window.location.href = targetUrl;
    } catch (e) {
      actionError = e instanceof Error ? e.message : 'Failed';
      isLoading = false;
    }
  }

  function retry() {
    if (targetUrl) {
      window.location.href = targetUrl;
    } else {
      window.location.reload();
    }
  }

  function goBack() {
    window.history.back();
  }

  const icons: Record<string, string> = {
    shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="M9 12l2 2 4-4"/>',
    warning: '<path d="M12 9v4"/><path d="M12 17h.01"/><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>',
    error: '<circle cx="12" cy="12" r="10"/><path d="M15 9l-6 6"/><path d="M9 9l6 6"/>',
    offline: '<line x1="1" y1="1" x2="23" y2="23"/><path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55"/><path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39"/><path d="M10.71 5.05A16 16 0 0 1 22.58 9"/><path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88"/><path d="M8.53 16.11a6 6 0 0 1 6.95 0"/><line x1="12" y1="20" x2="12.01" y2="20"/>',
    lock: '<rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
  };

  const iconColors: Record<string, string> = {
    shield: 'text-primary',
    warning: 'text-yellow-500',
    error: 'text-destructive',
    offline: 'text-muted-foreground',
    lock: 'text-yellow-500',
  };
</script>

<svelte:head>
  <title>{config.title}</title>
</svelte:head>

<ModeWatcher />
<div class="min-h-screen flex items-center justify-center p-8 bg-background">
  <Card.Root class="w-full max-w-md">
    <Card.Header class="text-center">
      <div class="flex justify-center mb-4">
        <svg class="w-16 h-16 {iconColors[config.icon] || 'text-muted-foreground'}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <!-- eslint-disable-next-line svelte/no-at-html-tags -- trusted static SVG paths -->
          {@html icons[config.icon] || icons.error}
        </svg>
      </div>
      {#if config.code > 0}
        <div class="text-6xl font-bold text-muted-foreground/30 mb-2">{config.code}</div>
      {/if}
      <Card.Title class="text-xl">{config.title}</Card.Title>
      <Card.Description>{customMessage || config.message}</Card.Description>
    </Card.Header>

    <Card.Content class="space-y-4">
      {#if targetUrl}
        <div class="bg-muted p-3 rounded-md">
          <p class="text-xs text-muted-foreground uppercase tracking-wide mb-1">URL</p>
          <code class="text-sm break-all">{targetUrl}</code>
        </div>
      {/if}

      {#if actionError}
        <div class="bg-destructive/10 border border-destructive/30 text-destructive p-3 rounded-md text-sm">
          {actionError}
        </div>
      {/if}
    </Card.Content>

    <Card.Footer class="flex flex-col gap-3">
      {#if config.actions.includes('whitelist') && targetDomain}
        <Button class="w-full" onclick={addToWhitelist} disabled={isLoading}>
          {isLoading ? 'Adding...' : 'Add to Whitelist'}
        </Button>
      {/if}

      {#if config.actions.includes('bypass') && targetUrl}
        <Button variant="outline" class="w-full" onclick={bypassOnce} disabled={isLoading}>
          Proceed Anyway
        </Button>
      {/if}

      {#if config.actions.includes('retry')}
        <Button class="w-full" onclick={retry} disabled={isLoading}>
          Try Again
        </Button>
      {/if}

      {#if config.actions.includes('back')}
        <Button variant="ghost" class="w-full" onclick={goBack} disabled={isLoading}>
          Go Back
        </Button>
      {/if}

      {#if config.actions.includes('whitelist') && targetDomain}
        <p class="text-xs text-muted-foreground text-center">
          Adding <strong class="text-foreground">{targetDomain}</strong> to whitelist will disable content filtering.
        </p>
      {/if}
    </Card.Footer>
  </Card.Root>
</div>
