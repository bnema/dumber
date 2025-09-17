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
    background-color: var(--dynamic-surface);
    border: 1px solid var(--dynamic-border);
    border-radius: 0;
    box-shadow: 0 8px 12px rgba(0, 0, 0, 0.08);
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
    color: var(--dynamic-text);
    font-size: 0.875rem;
    font-weight: 600;
    min-width: 9rem;
    padding: 0.75rem 1rem;
    text-align: center;
  }

  .toast-light.toast-info {
    background-color: rgb(239 246 255);
    border-color: rgb(191 219 254);
  }

  .toast-dark.toast-info {
    background-color: rgb(59 130 246 / 0.2);
    border-color: rgb(59 130 246);
  }

  .toast-light.toast-success {
    background-color: rgb(240 253 244);
    border-color: rgb(187 247 208);
  }

  .toast-dark.toast-success {
    background-color: rgb(22 163 74 / 0.2);
    border-color: rgb(22 163 74);
  }

  .toast-light.toast-error {
    background-color: rgb(254 242 242);
    border-color: rgb(254 202 202);
  }

  .toast-dark.toast-error {
    background-color: rgb(220 38 38 / 0.2);
    border-color: rgb(220 38 38);
  }

  .toast-light.toast-info .toast-message {
    color: rgb(59 130 246);
  }

  .toast-dark.toast-info .toast-message {
    color: rgb(191 219 254);
  }

  .toast-light.toast-success .toast-message {
    color: rgb(22 163 74);
  }

  .toast-dark.toast-success .toast-message {
    color: rgb(187 247 208);
  }

  .toast-light.toast-error .toast-message {
    color: rgb(220 38 38);
  }

  .toast-dark.toast-error .toast-message {
    color: rgb(254 202 202);
  }
</style>
