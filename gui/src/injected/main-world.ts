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
    __dumber_setTheme?: (theme: "light" | "dark") => void;
    __dumber_applyDomZoom?: (level: number) => void;
    __dumber_showToast?: (
      message: string,
      duration?: number,
      type?: "info" | "success" | "error",
    ) => number | void;
    __dumber_showZoomToast?: (level: number) => void;
    __dumber_omnibox_suggestions?: (suggestions: Suggestion[]) => void;
    __dumber_webview_id?: string;
    __dumber_is_active?: boolean;
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
  requestId: string;
  windowType: string; // "tab", "popup", "unknown"
}

interface Suggestion {
  // Define proper suggestion types based on your needs
  [key: string]: unknown;
}

interface DumberAPI {
  toast: {
    show: (
      message: string,
      duration?: number,
      type?: "info" | "success" | "error",
    ) => void;
    zoom: (level: number) => void;
  };
  omnibox: {
    suggestions: (suggestions: Suggestion[]) => void;
  };
}

// Function to detect if window.open call is for a popup based on features
function detectWindowType(features?: string | null): string {
  if (!features) {
    return "tab"; // Default to tab if no features specified
  }

  const featuresStr = features.toLowerCase();

  // Check for typical popup characteristics
  const hasSize = /width=\d+|height=\d+/.test(featuresStr);
  const hasNoToolbar = /toolbar=0|toolbar=no/.test(featuresStr);
  const hasNoMenubar = /menubar=0|menubar=no/.test(featuresStr);
  const hasNoLocation = /location=0|location=no/.test(featuresStr);

  // OAuth/login popups typically have size constraints and disabled UI elements
  const popupIndicators = [hasSize, hasNoToolbar, hasNoMenubar, hasNoLocation].filter(Boolean).length;

  // If 2 or more popup indicators are present, treat as popup
  if (popupIndicators >= 2) {
    return "popup";
  }

  return "tab";
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

    // Initialize WebView ID and active state (will be replaced by Go)
    window.__dumber_webview_id = "__WEBVIEW_ID__";
    window.__dumber_is_active = "__WEBVIEW_ACTIVE__" as unknown as boolean;

    // Theme setter function for GTK theme integration
    window.__dumber_setTheme = (theme: "light" | "dark") => {
      window.__dumber_initial_theme = theme;
      console.log("[dumber] Setting theme to:", theme);
      if (theme === "dark") {
        document.documentElement.classList.add("dark");
      } else {
        document.documentElement.classList.remove("dark");
      }
    };

    // Initialize unified API object
    window.__dumber = window.__dumber || ({} as DumberAPI);

    // Toast bridge
    window.__dumber.toast = window.__dumber.toast || {
      show: (message: string, duration?: number, type?: string) => {
        try {
          document.dispatchEvent(
            new CustomEvent("dumber:toast:show", {
              detail: { message, duration, type },
            }),
          );
          // Legacy compatibility
          document.dispatchEvent(
            new CustomEvent("dumber:showToast", {
              detail: { message, duration, type },
            }),
          );
        } catch {
          // ignore
        }
      },
      zoom: (level: number) => {
        try {
          document.dispatchEvent(
            new CustomEvent("dumber:toast:zoom", {
              detail: { level },
            }),
          );
          // Legacy compatibility
          document.dispatchEvent(
            new CustomEvent("dumber:showZoomToast", {
              detail: { level },
            }),
          );
        } catch {
          // ignore
        }
      },
    };

    // Legacy toast helpers
    window.__dumber_showToast = (
      message: string,
      duration?: number,
      type?: "info" | "success" | "error",
    ) => {
      window.__dumber.toast.show(message, duration, type);
    };
    window.__dumber_showZoomToast = (level: number) => {
      window.__dumber.toast.zoom(level);
    };

    // DOM zoom functionality
    if (typeof window.__dumber_dom_zoom_level !== "number") {
      window.__dumber_dom_zoom_level = initialZoom;
    }

    const applyZoomStyles = (node: HTMLElement, level: number): void => {
      if (!node) return;

      if (Math.abs(level - 1.0) < 1e-6) {
        // Reset zoom
        node.style.removeProperty("zoom");
        node.style.removeProperty("transform");
        node.style.removeProperty("transform-origin");
        node.style.removeProperty("width");
        node.style.removeProperty("min-width");
        node.style.removeProperty("height");
        node.style.removeProperty("min-height");
        return;
      }

      const scale = level;
      const inversePercent = 100 / scale;
      const widthValue = `${inversePercent}%`;

      node.style.removeProperty("zoom");
      node.style.transform = `scale(${scale})`;
      node.style.transformOrigin = "0 0";
      node.style.width = widthValue;
      node.style.minWidth = widthValue;
      node.style.minHeight = "100%";
    };

    window.__dumber_applyDomZoom = (level: number) => {
      try {
        window.__dumber_dom_zoom_level = level;
        window.__dumber_dom_zoom_seed = level;
        applyZoomStyles(document.documentElement, level);
        if (document.body) {
          applyZoomStyles(document.body, level);
        }
      } catch (err) {
        console.error("[dumber] DOM zoom error", err);
      }
    };

    // Apply initial zoom
    window.__dumber_applyDomZoom(window.__dumber_dom_zoom_level);

    if (!document.body) {
      document.addEventListener(
        "DOMContentLoaded",
        () => {
          if (typeof window.__dumber_dom_zoom_level === "number") {
            window.__dumber_applyDomZoom!(window.__dumber_dom_zoom_level);
          }
        },
        { once: true },
      );
    }

    // Omnibox suggestions bridge
    const omniboxQueue: Suggestion[] = [];
    let omniboxReady = false;

    const omniboxDispatch = (suggestions: Suggestion[]) => {
      try {
        document.dispatchEvent(
          new CustomEvent("dumber:omnibox:suggestions", {
            detail: { suggestions },
          }),
        );
        // Legacy compatibility
        document.dispatchEvent(
          new CustomEvent("dumber:omnibox-suggestions", {
            detail: { suggestions },
          }),
        );
      } catch (err) {
        console.error("[dumber] Omnibox dispatch error", err);
      }
    };

    window.__dumber_omnibox_suggestions = (suggestions: Suggestion[]) => {
      if (omniboxReady) {
        omniboxDispatch(suggestions);
      } else {
        try {
          omniboxQueue.push(...suggestions);
        } catch (err) {
          console.error("[dumber] Omnibox queue error", err);
        }
      }
    };

    document.addEventListener("dumber:omnibox-ready", () => {
      omniboxReady = true;
      if (omniboxQueue && omniboxQueue.length) {
        const items = omniboxQueue.slice();
        omniboxQueue.length = 0;
        items.forEach((s) => omniboxDispatch([s]));
      }
    });

    // Unified omnibox API
    window.__dumber.omnibox = window.__dumber.omnibox || {
      suggestions: (suggestions: Suggestion[]) => {
        window.__dumber_omnibox_suggestions!(suggestions);
      },
    };

    // Popup window.opener bridge using shared localStorage
    const setupPopupOpenerBridge = () => {
      try {
        // Check if this is a popup by looking for parent info in localStorage
        const findParentPopupId = () => {
          const keys = Object.keys(localStorage);
          for (const key of keys) {
            if (key.startsWith('popup_') && key.endsWith('_parent_info')) {
              const popupId = key.replace('popup_', '').replace('_parent_info', '');
              const parentInfo = JSON.parse(localStorage.getItem(key) || '{}');

              // Check if this popup info is recent (within last 30 seconds)
              const age = Date.now() - (parentInfo.timestamp || 0);
              if (age < 30000) {
                console.log(`[dumber-popup] Found parent info for popup ID: ${popupId}`, parentInfo);
                return { popupId, parentInfo };
              }
            }
          }
          return null;
        };

        const parentData = findParentPopupId();
        if (parentData && !window.opener) {
          const { popupId, parentInfo } = parentData;

          console.log(`[dumber-popup] Setting up window.opener bridge for popup ID: ${popupId}`);

          // Create window.opener proxy that uses localStorage for communication
          window.opener = {
            postMessage: (data: unknown, targetOrigin?: string) => {
              try {
                localStorage.setItem(`popup_${popupId}_message_to_parent`, JSON.stringify({
                  data,
                  origin: targetOrigin || '*',
                  timestamp: Date.now(),
                  source: 'popup',
                }));
                console.log(`[dumber-popup] Sent message to parent via localStorage:`, data);
              } catch (err) {
                console.warn(`[dumber-popup] Failed to send message to parent:`, err);
              }
            },

            focus: () => {
              try {
                localStorage.setItem(`popup_${popupId}_parent_action`, JSON.stringify({
                  action: 'focus',
                  timestamp: Date.now(),
                }));
              } catch (err) {
                console.warn(`[dumber-popup] Failed to request parent focus:`, err);
              }
            },

            blur: () => {
              try {
                localStorage.setItem(`popup_${popupId}_parent_action`, JSON.stringify({
                  action: 'blur',
                  timestamp: Date.now(),
                }));
              } catch (err) {
                console.warn(`[dumber-popup] Failed to request parent blur:`, err);
              }
            },

            location: {
              href: parentInfo.parentUrl || '',
              origin: new URL(parentInfo.parentUrl || 'about:blank').origin,
            },

            closed: false,

            // Support for custom properties/methods that websites might use
            [Symbol.for('popup.bridge')]: true,
          } as unknown as Window;

          // Set up message polling to receive messages from parent
          const pollForParentMessages = () => {
            try {
              const messageKey = `popup_${popupId}_message_to_popup`;
              const messageData = localStorage.getItem(messageKey);

              if (messageData) {
                const { data, origin, timestamp } = JSON.parse(messageData);

                // Check if message is recent (within last 5 seconds)
                if (Date.now() - timestamp < 5000) {
                  console.log(`[dumber-popup] Received message from parent:`, data);

                  // Dispatch as MessageEvent to window
                  const event = new MessageEvent('message', {
                    data,
                    origin: origin || parentInfo.parentUrl || '',
                    source: window.opener,
                  });
                  window.dispatchEvent(event);
                }

                // Clean up message
                localStorage.removeItem(messageKey);
              }
            } catch (err) {
              console.warn(`[dumber-popup] Failed to poll for parent messages:`, err);
            }
          };

          // Poll for messages every 100ms
          setInterval(pollForParentMessages, 100);

          console.log(`[dumber-popup] window.opener bridge established successfully`);
        }
      } catch (err) {
        console.warn(`[dumber-popup] Failed to setup window.opener bridge:`, err);
      }
    };

    // Setup popup bridge if this appears to be a popup
    setupPopupOpenerBridge();

    // Parent window message polling - only for OAuth scenarios
    let parentPollingInterval: ReturnType<typeof setInterval> | null = null;

    const setupParentMessagePolling = () => {
      // Only start polling if we have popup mappings or OAuth callbacks
      const hasRelevantData = () => {
        try {
          const keys = Object.keys(localStorage);
          return keys.some(key =>
            key.startsWith('popup_mapping_') ||
            key.startsWith('oauth_callback_') ||
            key.includes('message_to_parent')
          );
        } catch {
          return false;
        }
      };

      const pollForPopupMessages = () => {
        try {
          // Stop polling if no relevant data exists
          if (!hasRelevantData()) {
            return;
          }

          const keys = Object.keys(localStorage);

          for (const key of keys) {
            // Handle popup messages to parent
            if (key.includes('message_to_parent')) {
              const messageData = localStorage.getItem(key);
              if (!messageData) continue;

              try {
                const { data, origin, timestamp, source } = JSON.parse(messageData);

                // Check if message is recent (within last 5 seconds) and from a popup
                if (Date.now() - timestamp < 5000 && source === 'popup') {
                  console.log(`[dumber-parent] Received message from popup:`, data);

                  // Dispatch as MessageEvent to parent window
                  const event = new MessageEvent('message', {
                    data,
                    origin: origin || window.location.origin,
                    source: null, // popup reference would be here in real browser
                  });
                  window.dispatchEvent(event);

                  // Clean up message
                  localStorage.removeItem(key);
                }
              } catch (err) {
                console.warn(`[dumber-parent] Failed to parse popup message:`, err);
                localStorage.removeItem(key); // Clean up invalid message
              }
            }

            // Handle OAuth callback detection from popups
            if (key.startsWith('oauth_callback_')) {
              const callbackData = localStorage.getItem(key);
              if (!callbackData) continue;

              try {
                const { url, webviewId, timestamp, isOAuthCallback } = JSON.parse(callbackData);

                // Check if callback is recent (within last 10 seconds) and is an OAuth callback
                if (Date.now() - timestamp < 10000 && isOAuthCallback) {
                  console.log(`[dumber-parent] OAuth callback detected for webview ${webviewId}:`, url);

                  // Send close request to backend for this popup webview
                  const bridge = window.webkit?.messageHandlers?.dumber;
                  if (bridge && typeof bridge.postMessage === 'function') {
                    try {
                      const closeMessage = {
                        type: 'close-popup',
                        webviewId,
                        reason: 'oauth-callback-success',
                        timestamp: Date.now()
                      };

                      console.log(`[dumber-parent] Sending popup close request:`, closeMessage);
                      bridge.postMessage(JSON.stringify(closeMessage));
                    } catch (err) {
                      console.warn(`[dumber-parent] Failed to send popup close request:`, err);
                    }
                  } else {
                    console.warn(`[dumber-parent] No webkit bridge available for popup close request`);
                  }

                  // Clean up OAuth callback data
                  localStorage.removeItem(key);
                }
              } catch (err) {
                console.warn(`[dumber-parent] Failed to parse OAuth callback data:`, err);
                localStorage.removeItem(key); // Clean up invalid data
              }
            }
          }
        } catch (err) {
          console.warn(`[dumber-parent] Failed to poll for popup messages:`, err);
        }
      };

      // Start polling only when needed
      const startPollingForOAuthCallbacks = () => {
        if (!parentPollingInterval && hasRelevantData()) {
          console.log(`[dumber-parent] Starting OAuth/popup message polling`);
          parentPollingInterval = setInterval(pollForPopupMessages, 100);
        }
      };

      // Check periodically if polling should start
      setInterval(() => {
        if (hasRelevantData()) {
          startPollingForOAuthCallbacks();
        }
      }, 1000);
    };

    // Setup conditional parent message polling
    setupParentMessagePolling();

    // OAuth callback detection for auto-close
    const setupOAuthCallbackDetection = () => {
      const detectOAuthCallback = () => {
        const url = window.location.href.toLowerCase();

        // Check for OAuth callback patterns - be more specific to avoid false positives
        const isCallback = (
          // OAuth callback URLs typically have these parameters
          url.includes('code=') ||
          url.includes('access_token=') ||
          url.includes('id_token=') ||
          // OAuth error responses
          url.includes('error=access_denied') ||
          url.includes('error=unauthorized') ||
          // OAuth callback paths (more specific than just "callback" or "redirect")
          url.includes('/oauth/callback') ||
          url.includes('/auth/callback') ||
          url.includes('oauth2callback') ||
          url.includes('googlepopupcallback')
        );

        if (isCallback) {
          try {
            // Determine which webview should be closed
            let targetWebViewId = window.__dumber_webview_id;

            // If this is a parent window detecting OAuth callback, find the associated popup
            const popupMappingKey = `popup_mapping_${window.__dumber_webview_id}`;
            const popupMappingData = localStorage.getItem(popupMappingKey);

            if (popupMappingData) {
              try {
                const mapping = JSON.parse(popupMappingData);
                // Check if mapping is recent (within last 60 seconds)
                const age = Date.now() - (mapping.timestamp || 0);
                if (age < 60000 && mapping.popupId) {
                  targetWebViewId = mapping.popupId;
                  console.log(`[oauth-callback] Parent detected OAuth callback, targeting popup webview: ${targetWebViewId}`);

                  // Clean up the mapping since we're using it
                  localStorage.removeItem(popupMappingKey);
                }
              } catch (err) {
                console.warn(`[oauth-callback] Failed to parse popup mapping:`, err);
              }
            }

            const callbackData = {
              url: window.location.href,
              webviewId: targetWebViewId,
              timestamp: Date.now(),
              isOAuthCallback: true,
            };

            localStorage.setItem(`oauth_callback_${targetWebViewId}`, JSON.stringify(callbackData));
            console.log(`[oauth-callback] Detected OAuth callback, targeting webview ${targetWebViewId}:`, callbackData);

            // Auto-cleanup sensitive OAuth data after 10 seconds
            setTimeout(() => {
              try {
                localStorage.removeItem(`oauth_callback_${targetWebViewId}`);
                console.log(`[oauth-callback] Cleaned up OAuth callback data from localStorage`);
              } catch (err) {
                console.warn(`[oauth-callback] Failed to cleanup OAuth callback data:`, err);
              }
            }, 10000);
          } catch (err) {
            console.warn(`[oauth-callback] Failed to store OAuth callback data:`, err);
          }
        }
      };

      // Check immediately
      detectOAuthCallback();

      // Monitor for URL changes (for SPAs that don't reload)
      let lastUrl = window.location.href;
      setInterval(() => {
        if (window.location.href !== lastUrl) {
          lastUrl = window.location.href;
          detectOAuthCallback();
        }
      }, 500);

      // Also check on navigation events
      window.addEventListener('popstate', detectOAuthCallback);
      window.addEventListener('hashchange', detectOAuthCallback);
    };

    // Setup OAuth callback detection
    setupOAuthCallbackDetection();

    // Enhanced window.open deduplication system
    interface PendingRequest {
      id: string;
      url: string;
      timestamp: number;
      webviewId: string;
      resolved: boolean;
    }

    class WindowOpenDebouncer {
      private pendingRequests = new Map<string, PendingRequest>();
      private readonly DEBOUNCE_WINDOW = 150; // Increased from 100ms
      private readonly CLEANUP_INTERVAL = 1000; // Clean old entries every 1s

      generateRequestId(): string {
        return `${window.__dumber_webview_id}-${Date.now()}-${Math.random().toString(36).substring(2, 11)}`;
      }

      isDuplicate(url: string, webviewId: string): boolean {
        const key = this.createKey(url, webviewId);
        const existing = this.pendingRequests.get(key);

        if (!existing) return false;

        const timeDiff = Date.now() - existing.timestamp;
        return timeDiff < this.DEBOUNCE_WINDOW && !existing.resolved;
      }

      registerRequest(url: string, webviewId: string): string {
        const requestId = this.generateRequestId();
        const key = this.createKey(url, webviewId);

        this.pendingRequests.set(key, {
          id: requestId,
          url,
          timestamp: Date.now(),
          webviewId,
          resolved: false,
        });

        // Schedule cleanup
        setTimeout(() => this.markResolved(key), this.DEBOUNCE_WINDOW);
        return requestId;
      }

      private createKey(url: string, webviewId: string): string {
        return `${webviewId}:${url}`;
      }

      private markResolved(key: string): void {
        const request = this.pendingRequests.get(key);
        if (request) {
          request.resolved = true;
          // Clean up after additional delay
          setTimeout(
            () => this.pendingRequests.delete(key),
            this.CLEANUP_INTERVAL,
          );
        }
      }
    }

    // Global debouncer instance
    const windowOpenDebouncer = new WindowOpenDebouncer();

    // User interaction tracking
    let lastUserInteraction = 0;
    const INTERACTION_TIMEOUT = 5000; // 5 seconds

    const trackUserInteraction = () => {
      lastUserInteraction = Date.now();
    };

    const isUserInteraction = (): boolean => {
      const now = Date.now();
      const timeSinceInteraction = now - lastUserInteraction;
      return timeSinceInteraction <= INTERACTION_TIMEOUT;
    };


    const createFakeWindow = (url: string, popupId?: string): WindowProxy => {
      // Generate unique popup ID if not provided
      const actualPopupId = popupId || `${window.__dumber_webview_id}-${Date.now()}-${Math.random().toString(36).substring(2, 11)}`;

      // Set up communication channel via shared localStorage
      try {
        localStorage.setItem(`popup_${actualPopupId}_parent_info`, JSON.stringify({
          parentUrl: window.location.href,
          parentWebViewId: window.__dumber_webview_id,
          timestamp: Date.now(),
          popupUrl: url,
        }));

        console.log(`[window-open] [WebView ${window.__dumber_webview_id}] Set up shared storage communication for popup ID: ${actualPopupId}`);
      } catch (err) {
        console.warn(`[window-open] [WebView ${window.__dumber_webview_id}] Failed to set up shared storage:`, err);
      }

      return {
        closed: false,
        location: { href: url },
        close() {
          (this as { closed: boolean }).closed = true;
        },
        focus: () => {
          try {
            localStorage.setItem(`popup_${actualPopupId}_parent_action`, JSON.stringify({
              action: 'focus',
              timestamp: Date.now(),
            }));
          } catch (err) {
            console.warn(`[window-open] Failed to store focus action:`, err);
          }
        },
        blur: () => {
          try {
            localStorage.setItem(`popup_${actualPopupId}_parent_action`, JSON.stringify({
              action: 'blur',
              timestamp: Date.now(),
            }));
          } catch (err) {
            console.warn(`[window-open] Failed to store blur action:`, err);
          }
        },
        postMessage: (data: unknown, targetOrigin?: string) => {
          try {
            localStorage.setItem(`popup_${actualPopupId}_message_to_popup`, JSON.stringify({
              data,
              origin: targetOrigin || '*',
              timestamp: Date.now(),
              source: 'parent',
            }));
            console.log(`[window-open] [WebView ${window.__dumber_webview_id}] Stored message for popup ${actualPopupId}:`, data);
          } catch (err) {
            console.warn(`[window-open] Failed to store postMessage:`, err);
          }
        },
      } as unknown as WindowProxy;
    };


    // Track user interactions for popup validation
    ["click", "mousedown", "keydown", "touchstart"].forEach((eventType) => {
      document.addEventListener(eventType, trackUserInteraction, true);
    });

    console.log(
      `[window-open] [WebView ${window.__dumber_webview_id}] User interaction tracking enabled`,
    );


    // Window.open interceptor - only install once
    if (!window.__dumber_window_open_intercepted) {
      try {
        window.open = function (
          url?: string | URL | null,
          target?: string | null,
          features?: string | null,
        ): WindowProxy | null {
          const urlString = url
            ? typeof url === "string"
              ? url
              : url.toString()
            : "";
          const webviewId = window.__dumber_webview_id || "unknown";

          console.log(
            `[window-open] [WebView ${webviewId}] Bridge called with:`,
            urlString,
            target,
            features,
          );


          // Enhanced duplicate check
          if (windowOpenDebouncer.isDuplicate(urlString, webviewId)) {
            console.log(
              `[window-open] [WebView ${webviewId}] BLOCKED: Duplicate request within debounce window`,
            );
            return createFakeWindow(urlString);
          }

          // Active WebView check
          if (!window.__dumber_is_active) {
            console.log(
              `[window-open] [WebView ${webviewId}] BLOCKED: Inactive WebView`,
            );
            return createFakeWindow(urlString);
          }

          // User interaction check
          if (!isUserInteraction()) {
            console.log(
              `[window-open] [WebView ${webviewId}] BLOCKED: No recent user interaction`,
            );
            return createFakeWindow("");
          }

          // Register request and get unique ID
          const requestId = windowOpenDebouncer.registerRequest(
            urlString,
            webviewId,
          );

          const intent: WindowIntent = {
            url: urlString,
            target: target || "_blank",
            features: features || "",
            timestamp: Date.now(),
            userTriggered: true,
            requestId, // NEW: Add unique request ID
            windowType: detectWindowType(features), // Detect popup vs tab
          };

          // Send to Go backend
          try {
            if (window.webkit?.messageHandlers?.dumber) {
              window.webkit.messageHandlers.dumber.postMessage(
                JSON.stringify({
                  type: "handle-window-open",
                  payload: intent,
                }),
              );
              console.log(
                `[window-open] [WebView ${webviewId}] Sent request ${requestId} to Go backend`,
              );
            }
          } catch (err) {
            console.error(
              `[window-open] [WebView ${webviewId}] Failed to send request ${requestId}:`,
              err,
            );
          }

          return createFakeWindow(urlString, requestId);
        };

        window.__dumber_window_open_intercepted = true;
        console.log(
          `[window-open] [WebView ${window.__dumber_webview_id}] ✅ Page-world bridge installed`,
        );
      } catch (error) {
        console.error(
          `[window-open] [WebView ${window.__dumber_webview_id}] ❌ Failed to install page-world bridge:`,
          error,
        );
      }
    }

    // Console capture functionality
    const setupConsoleCapture = () => {
      const originalConsole = {
        log: console.log,
        warn: console.warn,
        error: console.error,
        info: console.info,
        debug: console.debug,
      };

      const sendConsoleMessage = (level: string, args: unknown[]) => {
        try {
          const message = args
            .map((arg) =>
              typeof arg === "object" ? JSON.stringify(arg) : String(arg),
            )
            .join(" ");

          const formattedMessage = `${window.location.href} ${message}`;

          // Send to Go via existing message handler
          if (window.webkit?.messageHandlers?.dumber) {
            window.webkit.messageHandlers.dumber.postMessage(
              JSON.stringify({
                type: "console-message",
                payload: {
                  level,
                  message: formattedMessage,
                  url: window.location.href,
                  webviewId: window.__dumber_webview_id,
                },
              }),
            );
          }
        } catch {
          // Silently ignore errors in console capture to avoid infinite loops
        }
      };

      const createConsoleWrapper = (
        originalMethod: (...args: unknown[]) => void,
        level: string,
      ) => {
        return function (...args: unknown[]) {
          originalMethod.apply(console, args);
          sendConsoleMessage(level, args);
        };
      };

      // Override console methods to capture messages
      console.log = createConsoleWrapper(originalConsole.log, "LOG");
      console.warn = createConsoleWrapper(originalConsole.warn, "WARN");
      console.error = createConsoleWrapper(originalConsole.error, "ERROR");
      console.info = createConsoleWrapper(originalConsole.info, "INFO");
      console.debug = createConsoleWrapper(originalConsole.debug, "DEBUG");
    };

    // Initialize console capture
    setupConsoleCapture();
  } catch (err) {
    console.warn("[dumber] unified bridge init failed", err);
  }
})();
