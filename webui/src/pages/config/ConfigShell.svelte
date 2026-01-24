<script lang="ts">
  import { onMount } from 'svelte';
  import { Home } from '@lucide/svelte';

  interface Props {
    children?: import('svelte').Snippet;
  }

  let { children }: Props = $props();

  function navigateWithViewTransition(url: string) {
    if (typeof document.startViewTransition !== 'function') {
      window.location.href = url;
      return;
    }

    const transition = document.startViewTransition(() => {
      document.documentElement.dataset.vt = 'leaving';
    });

    transition.finished.finally(() => {
      window.location.href = url;
    });
  }

  onMount(() => {
    // Avoid default WebKit focus rings.
    document.documentElement.dataset.vt = 'ready';
  });
</script>

<svelte:head>
  <title>dumb://config</title>
  <meta name="description" content="Dumber Browser - Settings" />
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- trusted static styles for shell initialization -->
  {@html `<style>
    html, body { margin: 0; padding: 0; }
    html { background: var(--background, #0a0a0a); }
    body { background: var(--background, #0a0a0a); color: var(--foreground, #e5e5e5); }
    *:focus { outline: none !important; }
    *:focus-visible { outline: none !important; }
    button:focus, input:focus, a:focus { outline: none !important; box-shadow: none !important; }
    * { -webkit-overflow-scrolling: touch; }
  </style>`}
</svelte:head>

<div class="config-shell browser-ui">
  <div class="terminal-frame">
    <header class="terminal-header">
      <div class="header-left">
        <span class="terminal-path">dumb://config</span>
      </div>
      <div class="header-right">
        <button
          class="action-button"
          type="button"
          onclick={() => navigateWithViewTransition('dumb://home')}
          title="Back to Home"
        >
          <Home size={14} strokeWidth={2} />
          <span class="button-text">HOME</span>
        </button>
      </div>
    </header>

    <main class="terminal-body scrollable">
      {#if children}
        {@render children()}
      {/if}
    </main>

    <footer class="terminal-footer">
      <span class="footer-text">Dumber Browser</span>
    </footer>
  </div>
</div>

<style>
  :global(html),
  :global(body) {
    height: 100%;
  }

  :global(body) {
    overflow: hidden;
    overscroll-behavior: contain;
  }

  .config-shell {
    height: 100vh;
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    font-family: ui-monospace, 'Fira Code', 'Cascadia Code', Menlo, Monaco, Consolas, monospace;
    line-height: 1.5;
    color: var(--foreground);
    background: var(--background);
    overflow: hidden;
  }

  .terminal-frame {
    flex: 1;
    display: grid;
    grid-template-rows: auto 1fr auto;
    width: 100%;
    height: 100%;
    background: color-mix(in srgb, var(--card) 60%, var(--background) 40%);
  }

  :global(.dark) .terminal-frame {
    box-shadow: 0 24px 48px -12px rgb(0 0 0 / 0.5);
  }

  .terminal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.75rem 1rem;
    min-height: 3.5rem;
    border-bottom: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 90%, var(--card) 10%);
    position: sticky;
    top: 0;
    z-index: 5;
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 1.5rem;
  }

  .terminal-path {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--primary, #4ade80);
    letter-spacing: 0.05em;
  }

  .header-right {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .action-button {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.75rem;
    font-size: 0.7rem;
    font-weight: 500;
    letter-spacing: 0.08em;
    color: var(--muted-foreground);
    background: transparent;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    cursor: pointer;
    transition: all 120ms ease;
  }

  .action-button:hover,
  .action-button:focus-visible {
    color: var(--foreground);
    border-color: color-mix(in srgb, var(--border) 50%, var(--foreground) 50%);
    background: color-mix(in srgb, var(--card) 30%, transparent);
    outline: none;
  }

  .terminal-body {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
    background: var(--background);
  }

  .terminal-body.scrollable {
    overflow-y: auto;
    -webkit-overflow-scrolling: touch;
    touch-action: pan-y;
  }

  .terminal-footer {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0.75rem 1rem;
    border-top: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 92%, var(--card) 8%);
  }

  .footer-text {
    font-size: 0.7rem;
    letter-spacing: 0.08em;
    color: var(--muted-foreground);
  }

  /* View Transition helpers */
  :global(html[data-vt='leaving']) {
    view-transition-name: config-root;
  }

  :global(::view-transition-old(config-root)) {
    animation: vt-fade-out 180ms ease both;
  }

  :global(::view-transition-new(config-root)) {
    animation: vt-fade-in 220ms ease both;
  }

  @keyframes vt-fade-out {
    to {
      opacity: 0;
      transform: translateY(6px);
    }
  }

  @keyframes vt-fade-in {
    from {
      opacity: 0;
      transform: translateY(-6px);
    }
  }
</style>
