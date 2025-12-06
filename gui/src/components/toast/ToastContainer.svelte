<script lang="ts">
  import Toast from './Toast.svelte';

  interface ToastData {
    id: number;
    message: string;
    duration?: number;
    type?: 'info' | 'success' | 'error';
  }

  // Svelte 5 rune for reactive state
  let toasts = $state<ToastData[]>([]);
  let counter = $state(0);
  let zoomToastId = $state<number | null>(null);
  let zoomDebounceTimer: number | undefined = undefined;

  function addToast(message: string, duration = 2500, type: 'info' | 'success' | 'error' = 'info'): number {
    const id = ++counter;
    const toast: ToastData = { id, message, duration, type };

    toasts.push(toast);
    console.log('📋 Toast:', message);
    return id;
  }

  function dismissToast(id: number) {
    console.log(`[ToastContainer] Dismissing toast with id: ${id}`);
    const index = toasts.findIndex(t => t.id === id);
    if (index === -1) {
      console.log(`[ToastContainer] Toast with id ${id} not found in array`);
      return;
    }

    // Clear zoom toast tracking if this is the zoom toast
    if (zoomToastId === id) {
      console.log(`[ToastContainer] Clearing zoom toast tracking for id: ${id}`);
      zoomToastId = null;
    }

    console.log(`[ToastContainer] Removing toast at index ${index}, remaining: ${toasts.length - 1}`);
    toasts.splice(index, 1);
  }

  function clearAllToasts() {
    toasts = [];
    zoomToastId = null;
    if (zoomDebounceTimer) {
      window.clearTimeout(zoomDebounceTimer);
      zoomDebounceTimer = undefined;
    }
  }
  // Export for external use
  window.__dumber_clearToasts = clearAllToasts;

  function showZoomToast(zoomLevel: number) {
    console.log('[dumber] showZoomToast called with level:', zoomLevel);

    // Debug: Check if toast container exists in DOM
    const container = document.querySelector('.toast-container');
    const browserRoot = document.querySelector('.browser-component-root');
    console.log('[dumber] DOM check - container exists:', !!container, 'browser-root exists:', !!browserRoot, 'toasts array length:', toasts.length);

    // Clear existing debounce timer
    if (zoomDebounceTimer) {
      window.clearTimeout(zoomDebounceTimer);
    }

    // Remove existing zoom toasts immediately (no animation needed for replacement)
    toasts = toasts.filter(toast => {
      const isZoomToast = toast.message.includes('Zoom level:') || toast.id === zoomToastId;
      return !isZoomToast;
    });

    // Show new zoom toast
    const percentage = Math.round(zoomLevel * 100);
    const message = `Zoom level: ${percentage}%`;
    const id = addToast(message, 1500, 'info');
    zoomToastId = id;

    console.log('[dumber] Added zoom toast - id:', id, 'current toasts count:', toasts.length);

    // Debounced cleanup for rapid zoom changes
    zoomDebounceTimer = window.setTimeout(() => {
      zoomToastId = null;
      zoomDebounceTimer = undefined;
    }, 1500);
  }

  // Listen for toast events from main world bridge
  if (typeof document !== 'undefined') {
    // Listen for general toast requests
    document.addEventListener('dumber:toast', ((event: CustomEvent) => {
      const { message, duration, type } = event.detail;
      console.log('[ToastContainer] Received toast event:', { message, duration, type });
      addToast(message, duration, type);
    }) as EventListener);

    // Listen for zoom toast requests
    document.addEventListener('dumber:toast:zoom', ((event: CustomEvent) => {
      const { level } = event.detail;
      console.log('[ToastContainer] Received zoom toast event:', level);
      showZoomToast(level);
    }) as EventListener);

    console.log('✅ Toast event listeners set up for CustomEvents');
  }

  // Effect that runs when toasts array changes
  $effect(() => {
    const container = document.querySelector('.toast-container');
    const browserRoot = document.querySelector('.browser-component-root');
    console.log('[dumber] Toast state changed - container:', !!container, 'browser-root:', !!browserRoot, 'toasts:', toasts.length, 'toasts:', toasts.map(t => t.message));
  });

  // One-time setup effect
  $effect(() => {
    console.log('🔄 ToastContainer initialized, functions exposed');

    // Monitor container periodically for debugging (disabled)
    // const monitorInterval = setInterval(() => {
    //   const container = document.querySelector('.toast-container');
    //   const browserRoot = document.querySelector('.browser-component-root');
    //   console.log('[dumber] Periodic check - container:', !!container, 'browser-root:', !!browserRoot, 'toasts:', toasts.length);
    // }, 5000);

    return () => {
      // clearInterval(monitorInterval);
      console.log('🗑️ ToastContainer cleanup');
    };
  });
</script>

<div class="browser-component-root toast-container">
  {#each toasts as toast (toast.id)}
    <Toast
      id={toast.id}
      message={toast.message}
      duration={toast.duration}
      type={toast.type}
      onDismiss={dismissToast}
    />
  {/each}
</div>

<style>
  .toast-container {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    left: 0.5rem;
    max-width: 20rem;
    pointer-events: none;
    position: fixed;
    top: 0.5rem;
    z-index: 2147483647;
  }
</style>
