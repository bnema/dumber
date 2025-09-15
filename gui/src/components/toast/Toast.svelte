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
  <div class="w-full min-w-36 px-5 py-4 text-center text-white font-semibold text-sm rounded-xl shadow-2xl backdrop-blur-md border transition-all duration-250 {type === 'success' ? 'bg-gradient-to-br from-green-500/90 to-green-700/90 border-green-300/30' : type === 'error' ? 'bg-gradient-to-br from-red-500/90 to-red-700/90 border-red-300/30' : 'bg-gradient-to-br from-blue-500/90 to-blue-700/90 border-blue-300/30'}">
    {message || 'Notification'}
  </div>
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