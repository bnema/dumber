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
      }
    };

    // Public API for native integration
    window.__dumber_showToast = (message, duration) => T.show(message, duration);
    window.__dumber_dismissToast = (id) => T.dismiss(id);
    window.__dumber_clearToasts = () => T.clear();

    console.log('✅ Dumber Browser toast system loaded');
    
  } catch (e) {
    console.error('❌ Failed to initialize Dumber toast system:', e);
  }
})();`
}