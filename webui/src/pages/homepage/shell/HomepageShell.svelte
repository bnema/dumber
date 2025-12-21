<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { homepageState } from '../state.svelte';
  import { initializeHomepage } from '../messaging';
  import { initializeKeyboard } from '../keyboard';
  import CommandPalette from './CommandPalette.svelte';
  import StatusBar from './StatusBar.svelte';
  import KeyboardHints from './KeyboardHints.svelte';
  import { History, Star, BarChart3, Search, Moon, Sun, Settings } from '@lucide/svelte';

  // Props for slot content
  interface Props {
    children?: import('svelte').Snippet;
  }
  let { children }: Props = $props();

  // Theme state
  type ThemeMode = 'light' | 'dark';
  let themeMode = $state<ThemeMode>('dark');
  let themeObserver: MutationObserver | null = null;

  // Keyboard cleanup
  let cleanupKeyboard: (() => void) | null = null;

  // Panel tabs config
  const panelTabs = [
    { id: 'history' as const, label: 'HIST', icon: History },
    { id: 'favorites' as const, label: 'FAV', icon: Star },
    { id: 'analytics' as const, label: 'STAT', icon: BarChart3 },
  ];

  // Theme management
  const syncThemeState = () => {
    themeMode = document.documentElement.classList.contains('dark') ? 'dark' : 'light';
  };

  function navigateWithViewTransition(url: string) {
    const doc = document as any;

    if (typeof doc?.startViewTransition !== 'function') {
      window.location.href = url;
      return;
    }

    const transition = doc.startViewTransition(() => {
      document.documentElement.dataset.vt = 'leaving';
    });

    transition.finished.finally(() => {
      window.location.href = url;
    });
  }

  const toggleTheme = () => {
    const nextMode = themeMode === 'dark' ? 'light' : 'dark';
    const manager = (window as any).__dumber_color_scheme_manager as
      | { setUserPreference?: (theme: ThemeMode) => void }
      | undefined;

    if (manager?.setUserPreference) {
      manager.setUserPreference(nextMode);
      return;
    }

    if ((window as any).__dumber_setTheme) {
      (window as any).__dumber_setTheme(nextMode);
      localStorage.setItem('dumber.theme', nextMode);
      return;
    }

    switch (nextMode) {
      case 'light':
        document.documentElement.classList.add('light');
        document.documentElement.classList.remove('dark');
        break;
      case 'dark':
        document.documentElement.classList.remove('light');
        document.documentElement.classList.add('dark');
        break;
    }
    localStorage.setItem('dumber.theme', nextMode);
    themeMode = nextMode;
  };

  onMount(async () => {
    // Initialize keyboard navigation
    cleanupKeyboard = initializeKeyboard();

    // Initialize data fetching
    await initializeHomepage();

    // Theme synchronization
    syncThemeState();
    themeObserver = new MutationObserver(syncThemeState);
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    });
  });

  onDestroy(() => {
    cleanupKeyboard?.();
    themeObserver?.disconnect();
    themeObserver = null;
    homepageState.reset();
  });
</script>

<svelte:head>
  <title>dumb://home</title>
  <meta name="description" content="Dumber Browser - Homepage" />
  {@html `<style>
    html, body { margin: 0; padding: 0; }
    html { background: var(--background, #0a0a0a); }
    body { background: var(--background, #0a0a0a); color: var(--foreground, #e5e5e5); }
    /* Disable GTK/WebKit default focus rings - we handle focus styling ourselves */
    *:focus { outline: none !important; }
    *:focus-visible { outline: none !important; }
    button:focus, input:focus, a:focus { outline: none !important; box-shadow: none !important; }
    /* Enable touch scrolling for WebKit */
    * { -webkit-overflow-scrolling: touch; }
  </style>`}
</svelte:head>

<div class="homepage-shell">
  <!-- Terminal Frame -->
  <div class="terminal-frame">
    <!-- Header -->
    <header class="terminal-header">
      <div class="header-left">
        <span class="terminal-path">dumb://home</span>
        <div class="header-tabs" role="tablist" aria-label="Homepage panels">
          {#each panelTabs as tab, index (tab.id)}
            <button
              class="tab-button"
              class:active={homepageState.activePanel === tab.id}
              onclick={() => homepageState.setActivePanel(tab.id)}
              onkeydown={(e) => {
                switch (e.key) {
                  case 'Enter':
                  case ' ':
                    e.preventDefault();
                    homepageState.setActivePanel(tab.id);
                    break;
                  case 'ArrowLeft':
                  case 'ArrowUp':
                    e.preventDefault();
                    {
                      const prevIndex = index === 0 ? panelTabs.length - 1 : index - 1;
                      const prevTab = panelTabs[prevIndex];
                      if (prevTab) {
                        homepageState.setActivePanel(prevTab.id);
                        (e.currentTarget.parentElement?.children[prevIndex] as HTMLElement)?.focus();
                      }
                    }
                    break;
                  case 'ArrowRight':
                  case 'ArrowDown':
                    e.preventDefault();
                    {
                      const nextIndex = index === panelTabs.length - 1 ? 0 : index + 1;
                      const nextTab = panelTabs[nextIndex];
                      if (nextTab) {
                        homepageState.setActivePanel(nextTab.id);
                        (e.currentTarget.parentElement?.children[nextIndex] as HTMLElement)?.focus();
                      }
                    }
                    break;
                }
              }}
              type="button"
              role="tab"
              aria-selected={homepageState.activePanel === tab.id}
              tabindex={homepageState.activePanel === tab.id ? 0 : -1}
            >
              <tab.icon size={14} strokeWidth={2} class="tab-icon" />
              <span class="tab-label">{tab.label}</span>
            </button>
          {/each}
        </div>
      </div>
      <div class="header-right">
        <button
          class="action-button"
          type="button"
          onclick={() => homepageState.openCommandPalette()}
          title="Command Palette (Ctrl+P or /)"
        >
          <Search size={14} strokeWidth={2} />
          <span class="button-text">CMD</span>
          <kbd class="kbd-hint">/</kbd>
        </button>
        <button
          class="action-button"
          type="button"
          onclick={() => navigateWithViewTransition('dumb://config')}
          title="Settings"
        >
          <Settings size={14} strokeWidth={2} />
          <span class="button-text">CFG</span>
        </button>
        <button
          class="theme-toggle"
          type="button"
          onclick={toggleTheme}
          title={themeMode === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
          aria-pressed={themeMode === 'dark'}
        >
          {#if themeMode === 'dark'}
            <Moon size={18} strokeWidth={2} />
          {:else}
            <Sun size={18} strokeWidth={2} />
          {/if}
        </button>
      </div>
    </header>

    <!-- Main Content Area -->
    <main class="terminal-body scrollable">
      {#if children}
        {@render children()}
      {:else}
        <div class="empty-shell">
          <span class="shell-prompt">$</span>
          <span class="shell-cursor">_</span>
        </div>
      {/if}
    </main>

    <!-- Status Bar -->
    <StatusBar />
  </div>

  <!-- Keyboard Hints Overlay -->
  <KeyboardHints />

  <!-- Command Palette Modal -->
  {#if homepageState.commandPaletteOpen}
    <CommandPalette />
  {/if}

  <!-- Confirmation Modal -->
  {#if homepageState.confirmModalOpen}
    <div
      class="modal-overlay"
      onclick={() => homepageState.cancelConfirm()}
      onkeydown={(e) => { if (e.key === 'Escape') homepageState.cancelConfirm(); }}
      role="button"
      tabindex="0"
      aria-label="Close confirmation modal"
    >
      <div class="confirm-modal" onclick={(e) => e.stopPropagation()} role="presentation">
        <div class="modal-header">
          <span class="modal-icon"></span>
          <span class="modal-title">CONFIRM</span>
        </div>
        <p class="modal-message">{homepageState.confirmModalMessage}</p>
        <div class="modal-actions">
          <button
            class="modal-btn modal-btn-cancel"
            type="button"
            onclick={() => homepageState.cancelConfirm()}
          >
            CANCEL
            <kbd>Esc</kbd>
          </button>
          <button
            class="modal-btn modal-btn-confirm"
            type="button"
            onclick={() => homepageState.confirmAction()}
          >
            CONFIRM
            <kbd>Enter</kbd>
          </button>
        </div>
      </div>
    </div>
  {/if}
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

  .homepage-shell {
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

  /* Dark mode: subtle shadow */
  :global(.dark) .terminal-frame {
    box-shadow: 0 24px 48px -12px rgb(0 0 0 / 0.5);
  }

  /* Header */
  .terminal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.75rem 1rem;
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

  .header-tabs {
    display: flex;
    gap: 0.25rem;
  }

  .tab-button {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.4rem 0.75rem;
    font-size: 0.7rem;
    font-weight: 500;
    letter-spacing: 0.1em;
    color: var(--muted-foreground);
    background: transparent;
    border: 1px solid transparent;
    cursor: pointer;
    transition: all 120ms ease;
  }

  .tab-button:hover {
    color: var(--foreground);
    background: color-mix(in srgb, var(--card) 50%, transparent);
  }

  .tab-button:focus-visible {
    color: var(--foreground);
    border-color: var(--primary, #4ade80);
    background: color-mix(in srgb, var(--primary, #4ade80) 15%, transparent);
  }

  .tab-button.active {
    color: var(--foreground);
    background: var(--card);
    border-color: var(--border);
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

  .kbd-hint {
    padding: 0.15rem 0.35rem;
    font-size: 0.65rem;
    color: var(--muted-foreground);
    background: var(--background);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    margin-left: 0.25rem;
  }

  .theme-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2.25rem;
    height: 2.25rem;
    color: var(--muted-foreground);
    background: transparent;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    cursor: pointer;
    transition: all 120ms ease;
  }

  .theme-toggle:hover,
  .theme-toggle:focus-visible {
    color: var(--foreground);
    border-color: color-mix(in srgb, var(--border) 50%, var(--foreground) 50%);
    outline: none;
  }

  .theme-toggle :global(svg) {
    flex-shrink: 0;
  }

  /* Main Content */
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

  .empty-shell {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    color: var(--muted-foreground);
    font-size: 1.25rem;
  }

  .shell-prompt {
    color: var(--primary, #4ade80);
  }

  .shell-cursor {
    animation: blink 1s step-end infinite;
  }

  @keyframes blink {
    50% { opacity: 0; }
  }

  /* Modal Overlay */
  .modal-overlay {
    position: fixed;
    inset: 0;
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgb(0 0 0 / 0.7);
    backdrop-filter: blur(4px);
    animation: fade-in 150ms ease;
  }

  @keyframes fade-in {
    from { opacity: 0; }
  }

  .confirm-modal {
    width: 100%;
    max-width: 420px;
    margin: 1rem;
    background: var(--card);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    box-shadow: 0 24px 48px -12px rgb(0 0 0 / 0.6);
    animation: modal-in 200ms ease;
  }

  @keyframes modal-in {
    from {
      opacity: 0;
      transform: scale(0.95) translateY(-8px);
    }
  }

  .modal-header {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.85rem 1rem;
    border-bottom-width: 1px;
    border-bottom-style: solid;
    border-bottom-color: var(--border);
    background: color-mix(in srgb, var(--background) 80%, transparent);
  }

  .modal-icon {
    font-size: 1rem;
    color: var(--warning);
  }

  .modal-title {
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.1em;
    color: var(--foreground);
  }

  .modal-message {
    margin: 0;
    padding: 1.25rem 1rem;
    font-size: 0.85rem;
    color: var(--foreground);
    line-height: 1.6;
  }

  .modal-actions {
    display: flex;
    gap: 0.5rem;
    padding: 0.85rem 1rem;
    border-top-width: 1px;
    border-top-style: solid;
    border-top-color: var(--border);
    background: color-mix(in srgb, var(--background) 50%, transparent);
  }

  .modal-btn {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.6rem;
    padding: 0.6rem 1rem;
    font-size: 0.72rem;
    font-weight: 500;
    letter-spacing: 0.08em;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    cursor: pointer;
    transition: all 120ms ease;
  }

  .modal-btn kbd {
    padding: 0.1rem 0.3rem;
    font-size: 0.6rem;
    background: var(--background);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    opacity: 0.7;
  }

  .modal-btn-cancel {
    color: var(--muted-foreground);
    background: transparent;
  }

  .modal-btn-cancel:hover {
    color: var(--foreground);
    background: color-mix(in srgb, var(--card) 40%, transparent);
  }

  .modal-btn-confirm {
    color: var(--destructive-foreground);
    background: var(--destructive);
    border-color: color-mix(in srgb, var(--destructive) 80%, black 20%);
  }

  .modal-btn-confirm:hover {
    background: color-mix(in srgb, var(--destructive) 85%, white 15%);
  }

  /* Responsive */
  @media (max-width: 768px) {
    .terminal-header {
      flex-wrap: wrap;
      gap: 0.75rem;
    }

    .header-left {
      flex-wrap: wrap;
      gap: 0.75rem;
    }

    .header-tabs {
      order: 10;
      width: 100%;
    }

    .tab-button {
      flex: 1;
      justify-content: center;
    }

    .action-button .button-text {
      display: none;
    }
  }
</style>
