<script lang="ts">
  import { homepageState } from '../state.svelte';
  import type { KeyboardHint } from '../keyboard';

  // Toggle visibility with ?
  let showHints = $state(false);
  let hintTimeout: ReturnType<typeof setTimeout> | null = null;

  // Panel-specific hints
  const panelHints = $derived.by((): KeyboardHint[] => {
    switch (homepageState.activePanel) {
      case 'history':
        return [
          { keys: ['d', 'x'], description: 'Delete entry' },
          { keys: ['D h'], description: 'Clear last hour' },
          { keys: ['D d'], description: 'Clear last day' },
          { keys: ['D w'], description: 'Clear last week' },
          { keys: ['D D'], description: 'Clear all history' },
          { keys: ['D @'], description: 'Clear domain' },
          { keys: ['p'], description: 'Pin to favorites' },
        ];
      case 'favorites':
        return [
          { keys: ['e'], description: 'Edit favorite' },
          { keys: ['t'], description: 'Manage tags' },
          { keys: ['m'], description: 'Move to folder' },
          { keys: ['1-9'], description: 'Set shortcut key' },
          { keys: ['d'], description: 'Delete favorite' },
        ];
      case 'analytics':
        return [
          { keys: ['r'], description: 'Refresh stats' },
          { keys: ['D @'], description: 'Clear domain' },
        ];
      default:
        return [];
    }
  });

  // Navigation hints (always shown)
  const navHints: KeyboardHint[] = [
    { keys: ['j', '↓'], description: 'Move down' },
    { keys: ['k', '↑'], description: 'Move up' },
    { keys: ['g g'], description: 'Go to first' },
    { keys: ['G'], description: 'Go to last' },
    { keys: ['Enter', 'o'], description: 'Open item' },
    { keys: ['/'], description: 'Search / Command' },
    { keys: ['Esc'], description: 'Close / Cancel' },
  ];

  // Panel switch hints
  const panelSwitchHints: KeyboardHint[] = [
    { keys: ['g h'], description: 'History panel' },
    { keys: ['g f'], description: 'Favorites panel' },
    { keys: ['g a'], description: 'Analytics panel' },
  ];

  // Quick access hints
  const quickAccessHints: KeyboardHint[] = [
    { keys: ['1-9'], description: 'Quick navigate to shortcut' },
  ];

  // Handle keyboard toggle
  function handleKeydown(e: KeyboardEvent) {
    if (e.key === '?' && !e.ctrlKey && !e.altKey && !e.metaKey) {
      // Don't trigger if in input
      if ((e.target as HTMLElement).tagName === 'INPUT') return;
      // Don't trigger if command palette is open
      if (homepageState.commandPaletteOpen) return;

      e.preventDefault();
      toggleHints();
    }
  }

  function toggleHints() {
    showHints = !showHints;

    // Auto-hide after 10 seconds
    if (showHints) {
      if (hintTimeout) clearTimeout(hintTimeout);
      hintTimeout = setTimeout(() => {
        showHints = false;
      }, 10000);
    } else {
      if (hintTimeout) {
        clearTimeout(hintTimeout);
        hintTimeout = null;
      }
    }
  }

  function closeHints() {
    showHints = false;
    if (hintTimeout) {
      clearTimeout(hintTimeout);
      hintTimeout = null;
    }
  }

  // Cleanup on destroy
  import { onDestroy } from 'svelte';
  onDestroy(() => {
    if (hintTimeout) clearTimeout(hintTimeout);
  });
</script>

<svelte:document onkeydown={handleKeydown} />

{#if showHints}
  <div
    class="hints-overlay"
    onclick={closeHints}
    onkeydown={(e) => { if (e.key === 'Escape' || e.key === 'Enter') closeHints(); }}
    role="button"
    tabindex="0"
    aria-label="Close keyboard hints"
  >
    <div class="hints-panel" onclick={(e) => e.stopPropagation()} role="presentation">
      <div class="hints-header">
        <span class="hints-icon"></span>
        <span class="hints-title">KEYBOARD SHORTCUTS</span>
        <button class="hints-close" onclick={closeHints} type="button">
          <span>×</span>
        </button>
      </div>

      <div class="hints-body">
        <!-- Navigation Section -->
        <section class="hints-section">
          <h3 class="section-title">
            <span class="section-icon"></span>
            Navigation
          </h3>
          <div class="hints-grid">
            {#each navHints as hint (hint.description)}
              <div class="hint-row">
                <div class="hint-keys">
                  {#each hint.keys as key, i}
                    {#if i > 0}<span class="key-sep">/</span>{/if}
                    <kbd>{key}</kbd>
                  {/each}
                </div>
                <span class="hint-desc">{hint.description}</span>
              </div>
            {/each}
          </div>
        </section>

        <!-- Panel Switch Section -->
        <section class="hints-section">
          <h3 class="section-title">
            <span class="section-icon"></span>
            Panels
          </h3>
          <div class="hints-grid">
            {#each panelSwitchHints as hint (hint.description)}
              <div class="hint-row">
                <div class="hint-keys">
                  {#each hint.keys as key, i}
                    {#if i > 0}<span class="key-sep">/</span>{/if}
                    <kbd>{key}</kbd>
                  {/each}
                </div>
                <span class="hint-desc">{hint.description}</span>
              </div>
            {/each}
          </div>
        </section>

        <!-- Current Panel Section -->
        {#if panelHints.length > 0}
          <section class="hints-section panel-specific">
            <h3 class="section-title">
              <span class="section-icon"></span>
              {homepageState.activePanel.charAt(0).toUpperCase() + homepageState.activePanel.slice(1)} Actions
            </h3>
            <div class="hints-grid">
              {#each panelHints as hint (hint.description)}
                <div class="hint-row">
                  <div class="hint-keys">
                    {#each hint.keys as key, i}
                      {#if i > 0}<span class="key-sep">/</span>{/if}
                      <kbd>{key}</kbd>
                    {/each}
                  </div>
                  <span class="hint-desc">{hint.description}</span>
                </div>
              {/each}
            </div>
          </section>
        {/if}

        <!-- Quick Access Section -->
        <section class="hints-section">
          <h3 class="section-title">
            <span class="section-icon"></span>
            Quick Access
          </h3>
          <div class="hints-grid">
            {#each quickAccessHints as hint (hint.description)}
              <div class="hint-row">
                <div class="hint-keys">
                  {#each hint.keys as key, i}
                    {#if i > 0}<span class="key-sep">/</span>{/if}
                    <kbd>{key}</kbd>
                  {/each}
                </div>
                <span class="hint-desc">{hint.description}</span>
              </div>
            {/each}
          </div>
        </section>
      </div>

      <div class="hints-footer">
        <span class="footer-hint">Press <kbd>?</kbd> to toggle hints</span>
      </div>
    </div>
  </div>
{/if}

<style>
  .hints-overlay {
    position: fixed;
    inset: 0;
    z-index: 150;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgb(0 0 0 / 0.6);
    backdrop-filter: blur(4px);
    animation: fade-in 150ms ease;
  }

  @keyframes fade-in {
    from { opacity: 0; }
  }

  .hints-panel {
    width: 100%;
    max-width: 540px;
    max-height: 85vh;
    margin: 1rem;
    display: flex;
    flex-direction: column;
    background: var(--card);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    box-shadow: 0 24px 48px -12px rgb(0 0 0 / 0.6);
    animation: panel-in 200ms cubic-bezier(0.16, 1, 0.3, 1);
    overflow: hidden;
  }

  @keyframes panel-in {
    from {
      opacity: 0;
      transform: scale(0.95);
    }
  }

  /* Header */
  .hints-header {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.85rem 1rem;
    border-bottom: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 80%, transparent);
  }

  .hints-icon {
    font-size: 1rem;
    color: var(--primary, #4ade80);
  }

  .hints-title {
    flex: 1;
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.1em;
    color: var(--foreground);
  }

  .hints-close {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 1.75rem;
    height: 1.75rem;
    font-size: 1.25rem;
    color: var(--muted-foreground);
    background: transparent;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    cursor: pointer;
    transition: all 120ms ease;
  }

  .hints-close:hover {
    color: var(--foreground);
    border-color: color-mix(in srgb, var(--border) 50%, var(--foreground) 50%);
  }

  /* Body */
  .hints-body {
    flex: 1;
    overflow-y: auto;
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }

  .hints-section {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .hints-section.panel-specific {
    padding: 0.75rem;
    background: color-mix(in srgb, var(--primary, #4ade80) 8%, transparent);
    border: 1px solid color-mix(in srgb, var(--primary, #4ade80) 25%, var(--border) 75%);
  }

  .section-title {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin: 0;
    font-size: 0.72rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--foreground);
  }

  .section-icon {
    font-size: 0.85rem;
    color: var(--muted-foreground);
  }

  .panel-specific .section-icon {
    color: var(--primary, #4ade80);
  }

  .hints-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 0.4rem;
  }

  .hint-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.4rem 0.6rem;
    background: color-mix(in srgb, var(--background) 60%, transparent);
    border: 1px solid color-mix(in srgb, var(--border) 50%, transparent);
  }

  .hint-keys {
    display: flex;
    align-items: center;
    gap: 0.2rem;
    flex-shrink: 0;
  }

  .hint-keys kbd {
    padding: 0.2rem 0.4rem;
    font-size: 0.68rem;
    font-family: inherit;
    color: var(--foreground);
    background: var(--background);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    min-width: 1.4rem;
    text-align: center;
  }

  .key-sep {
    font-size: 0.6rem;
    color: var(--muted-foreground);
    opacity: 0.6;
  }

  .hint-desc {
    font-size: 0.72rem;
    color: var(--muted-foreground);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* Footer */
  .hints-footer {
    display: flex;
    justify-content: center;
    padding: 0.65rem 1rem;
    border-top: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 70%, transparent);
  }

  .footer-hint {
    font-size: 0.68rem;
    color: var(--muted-foreground);
    letter-spacing: 0.05em;
  }

  .footer-hint kbd {
    padding: 0.15rem 0.35rem;
    font-size: 0.62rem;
    background: var(--background);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    margin: 0 0.25rem;
  }

  /* Scrollbar */
  .hints-body::-webkit-scrollbar {
    width: 6px;
  }

  .hints-body::-webkit-scrollbar-track {
    background: transparent;
  }

  .hints-body::-webkit-scrollbar-thumb {
    background: var(--border);
  }

  /* Responsive */
  @media (max-width: 480px) {
    .hints-grid {
      grid-template-columns: 1fr;
    }

    .hints-panel {
      max-height: 90vh;
    }
  }
</style>
