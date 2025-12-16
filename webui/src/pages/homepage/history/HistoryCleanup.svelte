<script lang="ts">
  import type { HistoryCleanupRange } from '../types';

  interface Props {
    onCleanup: (range: HistoryCleanupRange) => void;
    onClose: () => void;
  }

  let { onCleanup, onClose }: Props = $props();

  type CleanupOption = {
    range: HistoryCleanupRange;
    label: string;
    shortcut: string;
    description: string;
    destructive?: boolean;
  };

  const options: CleanupOption[] = [
    { range: 'hour', label: 'LAST HOUR', shortcut: 'D h', description: 'Delete entries from the past 60 minutes' },
    { range: 'day', label: 'LAST DAY', shortcut: 'D d', description: 'Delete entries from the past 24 hours' },
    { range: 'week', label: 'LAST WEEK', shortcut: 'D w', description: 'Delete entries from the past 7 days' },
    { range: 'month', label: 'LAST MONTH', shortcut: 'D m', description: 'Delete entries from the past 30 days' },
    { range: 'all', label: 'CLEAR ALL', shortcut: 'D D', description: 'Permanently delete all browsing history', destructive: true },
  ];

  let selectedIndex = $state(0);
  let confirming = $state(false);

  const handleKeyDown = (e: KeyboardEvent) => {
    switch (e.key) {
      case 'Escape':
        e.preventDefault();
        if (confirming) {
          confirming = false;
        } else {
          onClose();
        }
        break;
      case 'ArrowDown':
      case 'j':
        e.preventDefault();
        selectedIndex = Math.min(selectedIndex + 1, options.length - 1);
        break;
      case 'ArrowUp':
      case 'k':
        e.preventDefault();
        selectedIndex = Math.max(selectedIndex - 1, 0);
        break;
      case 'Enter':
        e.preventDefault();
        handleSelect(options[selectedIndex]!);
        break;
    }
  };

  const handleSelect = (option: CleanupOption) => {
    if (option.destructive && !confirming) {
      confirming = true;
      return;
    }
    onCleanup(option.range);
    onClose();
  };

  const handleOverlayClick = (e: Event) => {
    if (e.target === e.currentTarget) {
      onClose();
    }
  };
</script>

<svelte:window onkeydown={handleKeyDown} />

<div
  class="cleanup-overlay"
  onclick={handleOverlayClick}
  onkeydown={(e) => { if (e.key === 'Escape') onClose(); }}
  role="button"
  tabindex="0"
  aria-label="Close cleanup modal"
>
  <div class="cleanup-modal" role="presentation">
    <div class="modal-header">
      <span class="modal-icon"></span>
      <span class="modal-title">CLEAR HISTORY</span>
      <button class="close-btn" type="button" onclick={onClose}>
        <kbd>Esc</kbd>
      </button>
    </div>

    {#if confirming}
      <div class="confirm-panel">
        <div class="confirm-icon"></div>
        <p class="confirm-message">
          This will permanently delete <strong>ALL</strong> browsing history.
          This action cannot be undone.
        </p>
        <div class="confirm-actions">
          <button
            class="confirm-btn cancel"
            type="button"
            onclick={() => confirming = false}
          >
            CANCEL
            <kbd>Esc</kbd>
          </button>
          <button
            class="confirm-btn destructive"
            type="button"
            onclick={() => { onCleanup('all'); onClose(); }}
          >
            DELETE ALL
            <kbd>Enter</kbd>
          </button>
        </div>
      </div>
    {:else}
      <div class="cleanup-options">
        {#each options as option, i (option.range)}
          <button
            class="cleanup-option"
            class:selected={selectedIndex === i}
            class:destructive={option.destructive}
            type="button"
            onclick={() => handleSelect(option)}
            onmouseenter={() => selectedIndex = i}
          >
            <div class="option-main">
              <span class="option-label">{option.label}</span>
              <span class="option-desc">{option.description}</span>
            </div>
            <kbd class="option-shortcut">{option.shortcut}</kbd>
          </button>
        {/each}
      </div>
    {/if}

    <div class="modal-footer">
      <span class="hint"><kbd>j</kbd><kbd>k</kbd> navigate</span>
      <span class="hint"><kbd>Enter</kbd> select</span>
    </div>
  </div>
</div>

<style>
  .cleanup-overlay {
    position: fixed;
    inset: 0;
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgb(0 0 0 / 0.75);
    backdrop-filter: blur(4px);
    animation: fade-in 150ms ease;
  }

  @keyframes fade-in {
    from { opacity: 0; }
  }

  .cleanup-modal {
    width: 100%;
    max-width: 420px;
    margin: 1rem;
    background: var(--card);
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    box-shadow: 0 24px 48px -12px rgb(0 0 0 / 0.6);
    animation: modal-in 150ms ease;
  }

  @keyframes modal-in {
    from {
      opacity: 0;
      transform: scale(0.96) translateY(-8px);
    }
  }

  .modal-header {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 80%, transparent);
  }

  .modal-icon::before {
    content: '';
    font-size: 0.9rem;
    color: #fbbf24;
  }

  .modal-title {
    flex: 1;
    font-size: 0.72rem;
    font-weight: 600;
    letter-spacing: 0.1em;
    color: var(--foreground);
  }

  .close-btn {
    padding: 0.25rem 0.5rem;
    background: transparent;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    cursor: pointer;
  }

  .close-btn kbd {
    font-size: 0.6rem;
    color: var(--muted-foreground);
  }

  .close-btn:hover {
    border-color: var(--foreground);
  }

  .close-btn:hover kbd {
    color: var(--foreground);
  }

  .cleanup-options {
    display: flex;
    flex-direction: column;
  }

  .cleanup-option {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.85rem 1rem;
    text-align: left;
    background: transparent;
    border: none;
    border-bottom: 1px solid color-mix(in srgb, var(--border) 50%, transparent);
    cursor: pointer;
    transition: background-color 100ms ease;
  }

  .cleanup-option:last-child {
    border-bottom: none;
  }

  .cleanup-option:hover,
  .cleanup-option.selected {
    background: color-mix(in srgb, var(--card) 80%, var(--background) 20%);
  }

  .cleanup-option.selected {
    outline: 1px solid var(--primary, #4ade80);
    outline-offset: -1px;
  }

  .cleanup-option.destructive .option-label {
    color: #ef4444;
  }

  .option-main {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .option-label {
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    color: var(--foreground);
  }

  .option-desc {
    font-size: 0.68rem;
    color: var(--muted-foreground);
    letter-spacing: 0.04em;
  }

  .option-shortcut {
    padding: 0.2rem 0.45rem;
    font-size: 0.58rem;
    letter-spacing: 0.05em;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: var(--background);
    color: var(--muted-foreground);
  }

  .confirm-panel {
    padding: 1.5rem 1rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1rem;
    text-align: center;
  }

  .confirm-icon::before {
    content: '';
    font-size: 2rem;
    color: #ef4444;
  }

  .confirm-message {
    margin: 0;
    font-size: 0.8rem;
    line-height: 1.5;
    color: var(--foreground);
  }

  .confirm-message strong {
    color: #ef4444;
  }

  .confirm-actions {
    display: flex;
    gap: 0.5rem;
    width: 100%;
    margin-top: 0.5rem;
  }

  .confirm-btn {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 0.6rem 1rem;
    font-size: 0.7rem;
    font-weight: 500;
    letter-spacing: 0.08em;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    cursor: pointer;
    transition: all 100ms ease;
  }

  .confirm-btn kbd {
    font-size: 0.55rem;
    padding: 0.1rem 0.3rem;
    border: 1px solid;
    opacity: 0.7;
  }

  .confirm-btn.cancel {
    background: transparent;
    color: var(--muted-foreground);
  }

  .confirm-btn.cancel:hover {
    color: var(--foreground);
    border-color: var(--foreground);
  }

  .confirm-btn.destructive {
    background: #b91c1c;
    color: #fef2f2;
    border-color: #991b1b;
  }

  .confirm-btn.destructive:hover {
    background: #dc2626;
  }

  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 1rem;
    padding: 0.6rem 1rem;
    border-top: 1px solid var(--border);
    background: color-mix(in srgb, var(--background) 60%, transparent);
  }

  .hint {
    font-size: 0.6rem;
    color: var(--muted-foreground);
    letter-spacing: 0.05em;
  }

  .hint kbd {
    font-size: 0.55rem;
    padding: 0.1rem 0.25rem;
    margin-right: 0.25rem;
    border-width: 1px;
    border-style: solid;
    border-color: var(--border);
    background: var(--background);
  }
</style>
