//go:build webkit_cgo

package webkit

// getToastScript returns the injected toast notification component script.
// Provides a reusable toast system that can display messages across all pages.
func getToastScript() string {
	return `(() => {
  try {
    if (window.__dumber_toast_loaded) return; // idempotent
    window.__dumber_toast_loaded = true;

    const T = {
      container: null,
      toasts: [], // {id, element, timeout}
      counter: 0,
      zoomToastId: null,
      zoomDebounceTimer: null,

      init() {
        if (T.container) return;
        
        // Create toast container
        const container = document.createElement('div');
        container.id = 'dumber-toast-container';
        container.style.cssText = 'position:fixed;bottom:20px;right:20px;z-index:2147483646;pointer-events:none;';
        document.documentElement.appendChild(container);
        T.container = container;

        // Add CSS styles
        const style = document.createElement('style');
        style.textContent = ` +
		"`" + `
          .dumber-toast {
            background: rgba(18, 18, 18, 0.95);
            color: #eee;
            padding: 12px 16px;
            border-radius: 8px;
            border: 1px solid #444;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
            font-family: system-ui, -apple-system, Segoe UI, Roboto, Ubuntu, "Helvetica Neue", Arial, sans-serif;
            font-size: 14px;
            line-height: 1.4;
            margin-bottom: 8px;
            max-width: 320px;
            word-wrap: break-word;
            backdrop-filter: blur(8px);
            -webkit-backdrop-filter: blur(8px);
            pointer-events: auto;
            cursor: default;
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
            transform: translateX(100%);
            opacity: 0;
          }
          
          .dumber-toast.show {
            transform: translateX(0);
            opacity: 1;
          }
          
          .dumber-toast:hover {
            background: rgba(27, 27, 27, 0.95);
            border-color: #555;
          }
        ` + "`" + `;
        document.documentElement.appendChild(style);
      },

      show(message, duration = 2500) {
        T.init();
        
        const id = ++T.counter;
        const toast = document.createElement('div');
        toast.className = 'dumber-toast';
        toast.textContent = message || 'Notification';
        toast.setAttribute('data-toast-id', id);
        
        // Click to dismiss
        toast.addEventListener('click', () => T.dismiss(id));
        
        T.container.appendChild(toast);
        
        // Trigger show animation
        requestAnimationFrame(() => {
          toast.classList.add('show');
        });
        
        // Auto-dismiss
        const timeout = setTimeout(() => T.dismiss(id), duration);
        
        T.toasts.push({ id, element: toast, timeout });
        
        console.log('📋 Toast:', message);
        return id;
      },

      dismiss(id) {
        const toastIndex = T.toasts.findIndex(t => t.id === id);
        if (toastIndex === -1) return;
        
        const toast = T.toasts[toastIndex];
        clearTimeout(toast.timeout);
        
        // Clear zoom toast tracking if this is the zoom toast
        if (T.zoomToastId === id) {
          T.zoomToastId = null;
        }
        
        // Animate out
        toast.element.style.transform = 'translateX(100%)';
        toast.element.style.opacity = '0';
        
        setTimeout(() => {
          if (toast.element && toast.element.parentNode) {
            toast.element.parentNode.removeChild(toast.element);
          }
          T.toasts.splice(toastIndex, 1);
        }, 300);
      },

      clear() {
        T.toasts.forEach(toast => {
          clearTimeout(toast.timeout);
          if (toast.element && toast.element.parentNode) {
            toast.element.parentNode.removeChild(toast.element);
          }
        });
        T.toasts = [];
        T.zoomToastId = null;
        clearTimeout(T.zoomDebounceTimer);
        T.zoomDebounceTimer = null;
      }
    };

    // Public API for native integration
    window.__dumber_showToast = (message, duration) => T.show(message, duration);
    window.__dumber_dismissToast = (id) => T.dismiss(id);
    window.__dumber_clearToasts = () => T.clear();
    window.__dumber_showZoomToast = (zoomLevel) => {
      console.log('[dumber] showZoomToast called with level:', zoomLevel);
      
      // Clear existing debounce timer
      clearTimeout(T.zoomDebounceTimer);
      
      // Force remove ALL zoom toasts immediately (aggressive cleanup)
      for (let i = T.toasts.length - 1; i >= 0; i--) {
        const toast = T.toasts[i];
        if (toast.element && (toast.element.textContent.includes('Zoom level:') || toast.id === T.zoomToastId)) {
          clearTimeout(toast.timeout);
          if (toast.element.parentNode) {
            toast.element.parentNode.removeChild(toast.element);
          }
          T.toasts.splice(i, 1);
        }
      }
      T.zoomToastId = null;
      
      // Also cleanup any orphaned zoom toast elements in DOM
      const orphanedZoomToasts = document.querySelectorAll('.dumber-toast');
      orphanedZoomToasts.forEach(el => {
        if (el.textContent.includes('Zoom level:')) {
          el.parentNode && el.parentNode.removeChild(el);
        }
      });
      
      // Debounce the toast display
      T.zoomDebounceTimer = setTimeout(() => {
        const percentage = Math.round(zoomLevel * 100);
        const sign = percentage > 100 ? '+' : '';
        const diff = percentage - 100;
        const message = 'Zoom level: ' + sign + diff + '% (' + percentage + '%)';
        console.log('[dumber] Showing zoom toast:', message);
        T.zoomToastId = T.show(message, 2000);
      }, 150);
    };
    
    // Test function for manual debugging
    window.__dumber_testToast = () => {
      console.log('[dumber] Testing toast system...');
      return T.show('Toast system is working!', 2000);
    };
    
    // Emergency cleanup function for stuck toasts
    window.__dumber_forceCleanupToasts = () => {
      console.log('[dumber] Force cleaning up all toasts...');
      T.clear();
      // Also force remove any orphaned toast elements
      const orphans = document.querySelectorAll('.dumber-toast');
      orphans.forEach(el => el.parentNode && el.parentNode.removeChild(el));
      console.log('[dumber] Cleanup complete, removed', orphans.length, 'orphaned toasts');
    };

    console.log('✅ Dumber Browser toast system loaded');
    
  } catch (e) {
    console.error('❌ Failed to initialize Dumber toast system:', e);
  }
})();`
}
