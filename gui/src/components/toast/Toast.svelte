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
  let timeoutId: number | undefined;

  onMount(() => {
    timeoutId = window.setTimeout(() => {
      dismiss();
    }, duration);
  });

  onDestroy(() => {
    if (timeoutId) {
      window.clearTimeout(timeoutId);
      timeoutId = undefined;
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
</script>

<button
  class="toast-base {type === 'success' ? 'toast-success' : type === 'error' ? 'toast-error' : 'toast-info'} {isExiting ? 'exiting' : ''}"
  onclick={handleClick}
  data-toast-id={id}
  type="button"
  aria-label="Dismiss notification"
  title="Click to dismiss"
>
  <div class="toast-content">
    {message || 'Notification'}
  </div>
  <style>
    .toast-content {
      width: 100%;
      min-width: 9rem; /* ~min-w-36 */
      padding: 1rem 1.25rem; /* px-5 py-4 */
      text-align: center;
      color: #fff;
      font-weight: 600;
      font-size: 0.875rem; /* text-sm */
      border-radius: 0.75rem; /* rounded-xl */
      box-shadow: 0 25px 50px -12px rgba(0,0,0,0.5); /* shadow-2xl */
      backdrop-filter: blur(6px);
      border: 1px solid rgba(255,255,255,0.18);
      transition: all 250ms ease;
    }

    .toast-success .toast-content {
      background: linear-gradient(135deg, rgba(34,197,94,0.9), rgba(21,128,61,0.9));
      border-color: rgba(134,239,172,0.3);
    }
    .toast-error .toast-content {
      background: linear-gradient(135deg, rgba(239,68,68,0.9), rgba(185,28,28,0.9));
      border-color: rgba(252,165,165,0.3);
    }
    .toast-info .toast-content {
      background: linear-gradient(135deg, rgba(59,130,246,0.9), rgba(29,78,216,0.9));
      border-color: rgba(147,197,253,0.3);
    }
  </style>
</button>

<style>
  .toast-base {
    /* Reset button styles */
    border: none;
    background: none;
    padding: 0;
    font: inherit;
    text-align: left;

    /* Toast functionality */
    cursor: pointer;
    user-select: none;
    position: relative;
    overflow: visible;

    /* Layout */
    display: block;
    visibility: visible;
    z-index: 2147483647;
    pointer-events: auto;

    /* Animations */
    animation: slideIn 0.3s ease-out forwards;
    transition: transform 0.2s ease, filter 0.2s ease;
  }

  .toast-base.exiting {
    animation: slideOut 0.25s ease-in forwards;
  }

  @keyframes slideIn {
    from {
      transform: translateX(-100%);
      opacity: 0;
    }
    to {
      transform: translateX(0);
      opacity: 1;
    }
  }

  @keyframes slideOut {
    from {
      transform: translateX(0);
      opacity: 1;
    }
    to {
      transform: translateX(-100%);
      opacity: 0;
    }
  }

  .toast-base:hover {
    filter: brightness(1.1);
    transform: scale(1.02);
  }

  .toast-base:active {
    transform: scale(0.98);
  }

</style>