/**
 * Main-world bridge script for Dumber Browser
 *
 * This script is injected into the page-world context to provide:
 * - Window.open interceptor with debouncing
 * - Toast notifications bridge
 * - Theme management
 * - DOM zoom functionality
 * - Omnibox suggestions bridge
 */

// Type definitions for window extensions
declare global {
  interface Window {
    __dumber: DumberAPI;
    __dumber_page_bridge_installed?: boolean;
    __dumber_window_open_intercepted?: boolean;
    __dumber_dom_zoom_seed?: number;
    __dumber_dom_zoom_level?: number;
    __dumber_initial_theme?: string;
    __dumber_setTheme?: (theme: 'light' | 'dark') => void;
    __dumber_applyDomZoom?: (level: number) => void;
    __dumber_showToast?: (message: string, duration?: number, type?: 'info' | 'success' | 'error') => number | void;
    __dumber_showZoomToast?: (level: number) => void;
    __dumber_omnibox_suggestions?: (suggestions: Suggestion[]) => void;
    webkit?: {
      messageHandlers?: {
        dumber?: {
          postMessage: (message: string) => void;
        };
      };
    };
  }
}

interface WindowIntent {
  url: string;
  target: string;
  features: string;
  timestamp: number;
  userTriggered: boolean;
}

interface Suggestion {
  // Define proper suggestion types based on your needs
  [key: string]: unknown;
}

interface DumberAPI {
  toast: {
    show: (message: string, duration?: number, type?: 'info' | 'success' | 'error') => void;
    zoom: (level: number) => void;
  };
  omnibox: {
    suggestions: (suggestions: Suggestion[]) => void;
  };
}

(() => {
  try {
    // Prevent multiple installations
    if (window.__dumber_page_bridge_installed) {
      return;
    }
    window.__dumber_page_bridge_installed = true;

    // Initialize zoom level placeholder (will be replaced by Go)
    const initialZoom = 1.0; // __DOM_ZOOM_DEFAULT__ will be replaced by Go
    window.__dumber_dom_zoom_seed = initialZoom;

    // Theme setter function for GTK theme integration
    window.__dumber_setTheme = (theme: 'light' | 'dark') => {
      window.__dumber_initial_theme = theme;
      console.log('[dumber] Setting theme to:', theme);
      if (theme === 'dark') {
        document.documentElement.classList.add('dark');
      } else {
        document.documentElement.classList.remove('dark');
      }
    };

    // Initialize unified API object
    window.__dumber = window.__dumber || {} as DumberAPI;

    // Toast bridge
    window.__dumber.toast = window.__dumber.toast || {
      show: (message: string, duration?: number, type?: string) => {
        try {
          document.dispatchEvent(new CustomEvent('dumber:toast:show', {
            detail: { message, duration, type }
          }));
          // Legacy compatibility
          document.dispatchEvent(new CustomEvent('dumber:showToast', {
            detail: { message, duration, type }
          }));
        } catch {
          // ignore
        }
      },
      zoom: (level: number) => {
        try {
          document.dispatchEvent(new CustomEvent('dumber:toast:zoom', {
            detail: { level }
          }));
          // Legacy compatibility
          document.dispatchEvent(new CustomEvent('dumber:showZoomToast', {
            detail: { level }
          }));
        } catch {
          // ignore
        }
      }
    };

    // Legacy toast helpers
    window.__dumber_showToast = (message: string, duration?: number, type?: 'info' | 'success' | 'error') => {
      window.__dumber.toast.show(message, duration, type);
    };
    window.__dumber_showZoomToast = (level: number) => {
      window.__dumber.toast.zoom(level);
    };

    // DOM zoom functionality
    if (typeof window.__dumber_dom_zoom_level !== 'number') {
      window.__dumber_dom_zoom_level = initialZoom;
    }

    const applyZoomStyles = (node: HTMLElement, level: number): void => {
      if (!node) return;

      if (Math.abs(level - 1.0) < 1e-6) {
        // Reset zoom
        node.style.removeProperty('zoom');
        node.style.removeProperty('transform');
        node.style.removeProperty('transform-origin');
        node.style.removeProperty('width');
        node.style.removeProperty('min-width');
        node.style.removeProperty('height');
        node.style.removeProperty('min-height');
        return;
      }

      const scale = level;
      const inversePercent = 100 / scale;
      const widthValue = `${inversePercent}%`;

      node.style.removeProperty('zoom');
      node.style.transform = `scale(${scale})`;
      node.style.transformOrigin = '0 0';
      node.style.width = widthValue;
      node.style.minWidth = widthValue;
      node.style.minHeight = '100%';
    };

    window.__dumber_applyDomZoom = (level: number) => {
      try {
        window.__dumber_dom_zoom_level = level;
        window.__dumber_dom_zoom_seed = level;
        applyZoomStyles(document.documentElement, level);
        if (document.body) {
          applyZoomStyles(document.body, level);
        }
      } catch (e) {
        console.error('[dumber] DOM zoom error', e);
      }
    };

    // Apply initial zoom
    window.__dumber_applyDomZoom(window.__dumber_dom_zoom_level);

    if (!document.body) {
      document.addEventListener('DOMContentLoaded', () => {
        if (typeof window.__dumber_dom_zoom_level === 'number') {
          window.__dumber_applyDomZoom!(window.__dumber_dom_zoom_level);
        }
      }, { once: true });
    }

    // Omnibox suggestions bridge
    const omniboxQueue: Suggestion[] = [];
    let omniboxReady = false;

    const omniboxDispatch = (suggestions: Suggestion[]) => {
      try {
        document.dispatchEvent(new CustomEvent('dumber:omnibox:suggestions', {
          detail: { suggestions }
        }));
        // Legacy compatibility
        document.dispatchEvent(new CustomEvent('dumber:omnibox-suggestions', {
          detail: { suggestions }
        }));
      } catch (e) {
        console.error('[dumber] Omnibox dispatch error', e);
      }
    };

    window.__dumber_omnibox_suggestions = (suggestions: Suggestion[]) => {
      if (omniboxReady) {
        omniboxDispatch(suggestions);
      } else {
        try {
          omniboxQueue.push(...suggestions);
        } catch (e){
          console.error('[dumber] Omnibox queue error', e);
        }
      }
    };

    document.addEventListener('dumber:omnibox-ready', () => {
      omniboxReady = true;
      if (omniboxQueue && omniboxQueue.length) {
        const items = omniboxQueue.slice();
        omniboxQueue.length = 0;
        items.forEach(s => omniboxDispatch([s]) );
      }
    });

    // Unified omnibox API
    window.__dumber.omnibox = window.__dumber.omnibox || {
      suggestions: (suggestions: Suggestion[]) => {
        window.__dumber_omnibox_suggestions!(suggestions);
      }
    };

    // User interaction tracking and window.open interceptor
    let lastUserInteraction = 0;
    const INTERACTION_TIMEOUT = 5000; // 5 seconds
    const WINDOW_OPEN_DEBOUNCE = 1000; // 1 second between calls
    const recentWindowOpenCalls = new Map<string, number>();

    const trackUserInteraction = () => {
      lastUserInteraction = Date.now();
    };

    const isUserInteraction = (): boolean => {
      const now = Date.now();
      const timeSinceInteraction = now - lastUserInteraction;
      return timeSinceInteraction <= INTERACTION_TIMEOUT;
    };

    const isDuplicateWindowOpen = (url: string): boolean => {
      const now = Date.now();
      const lastCall = recentWindowOpenCalls.get(url);

      if (lastCall && (now - lastCall) < WINDOW_OPEN_DEBOUNCE) {
        return true;
      }

      recentWindowOpenCalls.set(url, now);

      // Clean up old entries
      for (const [key, timestamp] of recentWindowOpenCalls.entries()) {
        if (now - timestamp > WINDOW_OPEN_DEBOUNCE * 2) {
          recentWindowOpenCalls.delete(key);
        }
      }

      return false;
    };

    // Track user interactions for popup validation
    ['click', 'mousedown', 'keydown', 'touchstart'].forEach(eventType => {
      document.addEventListener(eventType, trackUserInteraction, true);
    });

    console.log('[window-open] User interaction tracking enabled');

    // Window.open interceptor - only install once
    if (!window.__dumber_window_open_intercepted) {
      try {

        window.open = function(
          url?: string | URL | null,
          target?: string | null,
          features?: string | null
        ): WindowProxy | null {
          console.log('[window-open] Bridge called with:', url, target, features);
          console.log('[window-open] Call stack:', new Error().stack);

          const urlString = url ? (typeof url === 'string' ? url : url.toString()) : '';

          // Check for duplicate calls to prevent multiple panes
          if (isDuplicateWindowOpen(urlString)) {
            console.log('[window-open] Blocked: duplicate call within debounce window');
            return {
              closed: true,
              location: { href: urlString },
              close: () => {},
              focus: () => {},
              blur: () => {},
              postMessage: () => {}
            } as unknown as WindowProxy;
          }

          // Check for user interaction to prevent popup spam
          if (!isUserInteraction()) {
            console.log('[window-open] Blocked: no recent user interaction detected');
            return {
              closed: true,
              location: { href: '' },
              close: () => {},
              focus: () => {},
              blur: () => {},
              postMessage: () => {}
            } as unknown as WindowProxy;
          }

          const originalFeatures = features || '';
          console.log('[window-open] Using original features:', originalFeatures);

          const intent: WindowIntent = {
            url: urlString,
            target: target || '_blank',
            features: originalFeatures,
            timestamp: Date.now(),
            userTriggered: true
          };

          // Send message to Go to handle popup creation
          try {
            if (window.webkit?.messageHandlers?.dumber) {
              window.webkit.messageHandlers.dumber.postMessage(JSON.stringify({
                type: 'handle-window-open',
                payload: intent
              }));
              console.log('[window-open] Sent window.open request to Go - bypassing WebKit completely');
            }
          } catch (e) {
            console.error('[window-open] Failed to send to Go:', e);
          }

          // Return fake window object - don't call original window.open
          console.log('[window-open] Returning fake window object - bypassing WebKit create signal');
          return {
            closed: false,
            location: { href: urlString },
            close() { (this as { closed: boolean }).closed = true; },
            focus: () => {},
            blur: () => {},
            postMessage: () => {}
          } as unknown as WindowProxy;
        };

        window.__dumber_window_open_intercepted = true;
        console.log('[window-open] ✅ Page-world bridge installed');
      } catch (error) {
        console.error('[window-open] ❌ Failed to install page-world bridge:', error);
      }
    }

    // Console capture functionality
    const setupConsoleCapture = () => {
      const originalConsole = {
        log: console.log,
        warn: console.warn,
        error: console.error,
        info: console.info,
        debug: console.debug
      };

      const sendConsoleMessage = (level: string, args: unknown[]) => {
        try {
          const message = args.map(arg =>
            typeof arg === 'object' ? JSON.stringify(arg) : String(arg)
          ).join(' ');

          const formattedMessage = `${window.location.href} ${message}`;

          // Send to Go via existing message handler
          if (window.webkit?.messageHandlers?.dumber) {
            window.webkit.messageHandlers.dumber.postMessage(JSON.stringify({
              type: 'console-message',
              payload: { level, message: formattedMessage, url: window.location.href }
            }));
          }
        } catch {
          // Silently ignore errors in console capture to avoid infinite loops
        }
      };

      const createConsoleWrapper = (originalMethod: (...args: unknown[]) => void, level: string) => {
        return function(...args: unknown[]) {
          originalMethod.apply(console, args);
          sendConsoleMessage(level, args);
        };
      };

      // Override console methods to capture messages
      console.log = createConsoleWrapper(originalConsole.log, 'LOG');
      console.warn = createConsoleWrapper(originalConsole.warn, 'WARN');
      console.error = createConsoleWrapper(originalConsole.error, 'ERROR');
      console.info = createConsoleWrapper(originalConsole.info, 'INFO');
      console.debug = createConsoleWrapper(originalConsole.debug, 'DEBUG');
    };

    // Initialize console capture
    setupConsoleCapture();

  } catch (e) {
    console.warn('[dumber] unified bridge init failed', e);
  }
})();