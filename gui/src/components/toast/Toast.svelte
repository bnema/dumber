<script lang="ts">
  import { onMount, onDestroy } from 'svelte';

  interface ToastProps {
    id: number;
    message: string;
    duration?: number;
    type?: 'info' | 'success' | 'error';
    onDismiss: (id: number) => void;
  }

  let { id, message, duration = 2500, type = 'info', onDismiss }: ToastProps = $props();

  let isExiting = $state(false);
  let isDarkMode = $state(false);
  let themeObserver: MutationObserver | undefined;
  let timeoutId: number | undefined;

  onMount(() => {
    timeoutId = window.setTimeout(() => {
      dismiss();
    }, duration);

    syncTheme();

    if (typeof window !== 'undefined' && 'MutationObserver' in window) {
      themeObserver = new MutationObserver(syncTheme);
      themeObserver.observe(document.documentElement, {
        attributes: true,
        attributeFilter: ['class']
      });
    }
  });

  onDestroy(() => {
    if (timeoutId) {
      window.clearTimeout(timeoutId);
      timeoutId = undefined;
    }

    if (themeObserver) {
      themeObserver.disconnect();
      themeObserver = undefined;
    }
  });

  function dismiss() {
    if (timeoutId) {
      window.clearTimeout(timeoutId);
      timeoutId = undefined;
    }
    isExiting = true;
    // Allow time for exit animation before removing from parent
    setTimeout(() => {
      onDismiss(id);
    }, 250);
  }

  function handleClick() {
    dismiss();
  }

  function syncTheme() {
    if (typeof document === 'undefined') {
      return;
    }

    const prefersDark = document.documentElement.classList.contains('dark');

    if (prefersDark) {
      isDarkMode = true;
      return;
    }

    try {
      isDarkMode = window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false;
    } catch {
      isDarkMode = false;
    }
  }
</script>

<button
  class={`toast ${isDarkMode ? 'toast-dark' : 'toast-light'} toast-${type} ${isExiting ? 'toast-exit' : 'toast-enter'}`}
  onclick={handleClick}
  data-toast-id={id}
  type="button"
  aria-label="Dismiss notification"
  title="Click to dismiss"
>
  <div class="toast-message">
    {message || 'Notification'}
  </div>
</button>

<style>
  .toast {
    --toast-bg: color-mix(in srgb, var(--dynamic-surface) 88%, var(--dynamic-text) 12%);
    --toast-border: color-mix(in srgb, var(--dynamic-border) 70%, var(--dynamic-text) 30%);
    --toast-text: var(--dynamic-text);
    background-color: var(--toast-bg);
    border: 1px solid var(--toast-border);
    border-radius: 0;
    box-shadow: 0 8px 14px rgba(0, 0, 0, 0.08);
    cursor: pointer;
    display: inline-flex;
    margin: 0;
    pointer-events: auto;
    padding: 0;
    transition: transform 0.2s ease, opacity 0.2s ease;
  }

  .toast-enter {
    opacity: 1;
    transform: translateY(0);
  }

  .toast-exit {
    opacity: 0;
    transform: translateY(-16px);
  }

  .toast-message {
    color: var(--toast-text);
    font-size: 0.875rem;
    font-weight: 600;
    min-width: 9rem;
    padding: 0.75rem 1rem;
    text-align: center;
  }

  .toast.toast-info {
    --toast-bg: color-mix(in srgb, var(--dynamic-surface) 82%, var(--dynamic-text) 18%);
    --toast-border: color-mix(in srgb, var(--dynamic-border) 55%, var(--dynamic-text) 45%);
  }

  .toast.toast-success {
    --toast-bg: color-mix(in srgb, var(--dynamic-surface) 78%, var(--dynamic-text) 22%);
    --toast-border: color-mix(in srgb, var(--dynamic-border) 50%, var(--dynamic-text) 50%);
  }

  .toast.toast-error {
    --toast-bg: color-mix(in srgb, var(--dynamic-surface) 74%, var(--dynamic-text) 26%);
    --toast-border: color-mix(in srgb, var(--dynamic-border) 45%, var(--dynamic-text) 55%);
    --toast-text: color-mix(in srgb, var(--dynamic-text) 80%, var(--dynamic-muted) 20%);
  }
</style>
