/**
 * Dumber GUI bootstrap routine shared between injected pages and special schemes.
 *
 * Sets up toast, omnibox, keyboard service, and workspace event listeners.
 */

import { initializeToast, type ToastConfig } from "./modules/toast";
import {
  initializeOmnibox,
  type OmniboxInitConfig,
} from "./modules/omnibox";
import "./modules/workspace"; // Auto-initializes pane mode UI notifications
import { keyboardService, type KeyboardService } from "$lib/keyboard";
import type { Suggestion } from "../components/omnibox/types";

declare global {
  interface Window {
    __dumber_gui?: DumberGUI;
    __dumber_gui_ready?: boolean;
    __dumber_gui_ready_for?: Document | null;
    __dumber_showToast?: (
      message: string,
      duration?: number,
      type?: "info" | "success" | "error",
    ) => number | void;
    __dumber_showZoomToast?: (level: number) => void;
    __dumber_keyboard?: KeyboardService;
    __dumber_omnibox?: {
      setSuggestions: (suggestions: Suggestion[]) => void;
      toggle: () => void;
      open: (mode?: string, query?: string) => void;
      close: () => void;
      findQuery: (query: string) => void;
      setActive: (active: boolean) => void;
    };
    __dumber_gui_bootstrap?: () => void;
    __dumber_toggle?: () => void;
    __dumber_find_open?: (query?: string) => void;
    __dumber_find_close?: () => void;
    __dumber_find_query?: (query: string) => void;
    __dumber_dismissToast?: () => void;
    __dumber_clearToasts?: () => void;
  }
}

interface DumberGUI {
  initializeToast: (config?: ToastConfig) => Promise<void>;
  initializeOmnibox: (config?: OmniboxInitConfig) => Promise<void>;
  keyboard: KeyboardService;
  isReady: boolean;
}

function whenDOMReady(callback: () => void) {
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", callback, { once: true });
  } else if (document.readyState === "interactive" && !document.body) {
    setTimeout(() => whenDOMReady(callback), 10);
  } else {
    callback();
  }
}

function normalizeWebviewId(value: unknown): number | null {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed || trimmed === "__WEBVIEW_ID__") {
      return null;
    }
    const parsed = Number.parseInt(trimmed, 10);
    return Number.isNaN(parsed) ? null : parsed;
  }

  return null;
}

export function bootstrapGUI(): void {
  if (window.__dumber_gui_ready_for === document) {
    return;
  }

  if (window.self !== window.top) {
    console.log("🚫 Dumber GUI skipped in iframe");
    return;
  }

  window.__dumber_gui_ready_for = document;
  window.__dumber_gui_ready = true;

  try {
    window.addEventListener("pagehide", () => {
      window.__dumber_gui_ready = false;
      window.__dumber_gui_ready_for = null;
    }, { once: true });
  } catch {
    // Ignore environments without pagehide support
  }

  let workspaceHasFocus = false;
  const initialRawWebViewId = window.__dumber_webview_id;
  let currentWebViewId = normalizeWebviewId(initialRawWebViewId);
  if (currentWebViewId !== null) {
    window.__dumber_webview_id = currentWebViewId;
  }
  let hasReceivedFocusEvent = false;

  if (currentWebViewId === null) {
    console.log("[workspace] WebView ID unknown, requesting from Go backend");
    if (window.webkit?.messageHandlers?.dumber) {
      window.webkit.messageHandlers.dumber.postMessage(
        JSON.stringify({
          type: "request-webview-id",
          payload: { timestamp: Date.now() },
        }),
      );
    }
  }

  document.addEventListener("dumber:webview-id", (event: Event) => {
    const detail = (event as CustomEvent).detail || {};
    const incomingId = normalizeWebviewId(detail.webviewId);
    if (incomingId !== null && incomingId !== currentWebViewId) {
      currentWebViewId = incomingId;
      window.__dumber_webview_id = incomingId;
      console.log("[workspace] Received webview ID from Go:", incomingId);
    }
  });

  document.addEventListener("dumber:workspace-focus", (event: Event) => {
    const detail = (event as CustomEvent).detail || {};
    const eventWebviewId = normalizeWebviewId(detail?.webviewId);

    if (eventWebviewId !== null) {
      if (currentWebViewId === null) {
        currentWebViewId = eventWebviewId;
        window.__dumber_webview_id = eventWebviewId;
        console.log(
          "[workspace] Learned webview ID from focus event:",
          eventWebviewId,
        );
      } else if (eventWebviewId !== currentWebViewId) {
        console.log("[workspace] Ignoring focus event for other webview", {
          eventWebviewId,
          self: currentWebViewId,
          detail,
        });
        return;
      }
    }

    console.log("[workspace focus event]", detail, "prev focus=", workspaceHasFocus);

    hasReceivedFocusEvent = true;

    if (typeof detail?.active === "boolean") {
      workspaceHasFocus = detail.active;
    } else {
      workspaceHasFocus = detail !== false;
    }

    console.log(
      "[workspace focus updated] focus=",
      workspaceHasFocus,
      "webviewId=",
      currentWebViewId,
    );

    if (window.__dumber_omnibox) {
      if (workspaceHasFocus) {
        window.__dumber_omnibox.setActive(true);
      } else {
        window.__dumber_omnibox.setActive(false);
        window.__dumber_omnibox.close();
      }
    }
  });

  whenDOMReady(async () => {
    console.log("[GUI] DOM ready, starting initialization");
    let toastInitialized = false;

    if (typeof window.__dumber_showToast === "function") {
      console.log("[GUI] Toast already initialized, skipping");
      toastInitialized = true;
    } else {
      try {
        console.log("[GUI] Attempting toast initialization");
        await initializeToast();
        if (typeof window.__dumber_showToast !== "function") {
          throw new Error("Toast functions not exposed after initialization");
        }
        toastInitialized = true;
        console.log("[GUI] Toast initialized successfully");
      } catch (e) {
        console.error("❌ Failed to initialize Svelte toast system:", e);
        window.__dumber_showToast = (
          message: string,
          duration: number = 2500,
        ) => {
          document
            .querySelectorAll(".dumber-fallback-toast")
            .forEach((el) => el.remove());

          const toast = document.createElement("div");
          toast.className = "dumber-fallback-toast";
          toast.textContent = message;
          toast.style.cssText = `
            position: fixed;
            bottom: 20px;
            right: 20px;
            background: rgba(0,0,0,0.9);
            color: white;
            padding: 12px 16px;
            border-radius: 6px;
            z-index: 2147483647;
            font-family: system-ui, -apple-system, sans-serif;
            font-size: 14px;
            opacity: 0;
            transform: translateX(100%);
            transition: all 0.3s ease;
            max-width: 300px;
            word-wrap: break-word;
          `;

          if (document.body) {
            document.body.appendChild(toast);

            requestAnimationFrame(() => {
              toast.style.opacity = "1";
              toast.style.transform = "translateX(0)";
            });

            setTimeout(() => {
              toast.style.opacity = "0";
              toast.style.transform = "translateX(100%)";
              setTimeout(() => toast.remove(), 300);
            }, duration);
          }
        };

        window.__dumber_showZoomToast = (level: number) => {
          const percentage = Math.round(level * 100);
          window.__dumber_showToast!(`Zoom: ${percentage}%`, 1500);
        };

        window.__dumber_dismissToast = (_id?: number) => {
          document
            .querySelectorAll(".dumber-fallback-toast")
            .forEach((el) => el.remove());
        };

        window.__dumber_clearToasts = () => {
          document
            .querySelectorAll(".dumber-fallback-toast")
            .forEach((el) => el.remove());
        };
      }
    }

    try {
      await initializeOmnibox();
      console.log("✅ Omnibox system initialized successfully");

      // Auto-open omnibox on about:blank pages
      if (window.location.href === "about:blank" && window.__dumber_omnibox) {
        console.log("[about:blank] Auto-opening omnibox");
        setTimeout(() => {
          window.__dumber_omnibox?.open?.("omnibox", "");
        }, 100);
      }
    } catch (omniboxError) {
      console.error("❌ Failed to initialize omnibox system:", omniboxError);
    }

    console.log("✅ GUI systems initialization complete", {
      toast: toastInitialized ? "Svelte" : "Fallback",
      omnibox: !!window.__dumber_omnibox,
    });
  });

  window.__dumber_keyboard = keyboardService;

  whenDOMReady(() => {
    document.addEventListener(
      "keydown",
      (event) => {
        keyboardService.handleKeyboardEvent(event);
      },
      true,
    );

    document.addEventListener(
      "mousedown",
      (event) => {
        keyboardService.handleMouseEvent(event);
      },
      true,
    );

    console.log("✅ KeyboardService initialized with global listeners");

    document.addEventListener("dumber:key", (e: Event) => {
      const detail = (e as CustomEvent).detail || {};
      if (detail && typeof detail.shortcut === "string") {
        keyboardService.handleNativeShortcut(detail.shortcut);
      }
    });

    document.addEventListener("dumber:ui:shortcut", (e: Event) => {
      const detail = (e as CustomEvent).detail || {};
      const action = detail?.action;
      const eventWebViewId = normalizeWebviewId(detail?.webviewId);
      const source = detail?.source || "unknown";

      if (eventWebViewId !== null && currentWebViewId === null) {
        currentWebViewId = eventWebViewId;
        window.__dumber_webview_id = eventWebViewId;
        console.log(
          "[dumber shortcuts] Learned webview ID from shortcut event:",
          eventWebViewId,
        );
      }

      console.log("[dumber shortcuts] Event received", {
        action,
        eventWebViewId,
        currentWebViewId,
        detail,
      });

      if (typeof action !== "string") {
        console.log("[dumber shortcuts] No action in event, ignoring");
        return;
      }

      const isForThisWebView =
        eventWebViewId === null || eventWebViewId === currentWebViewId;
      const isOmniboxAction = action.startsWith("omnibox-");
      const shouldHandle =
        isForThisWebView &&
        (isOmniboxAction
          ? hasReceivedFocusEvent
            ? workspaceHasFocus
            : true
          : true);

      if (!shouldHandle) {
        console.log("[dumber shortcuts] ignored action", {
          action,
          workspaceHasFocus,
          isForThisWebView,
          eventWebViewId,
          currentWebViewId,
          source,
          detail,
          hasReceivedFocusEvent,
        });
        return;
      }

      console.log("[dumber shortcuts] handling action", {
        action,
        source,
        webviewId: currentWebViewId,
      });

      const omnibox = window.__dumber_omnibox;

      switch (action) {
        case "omnibox-nav-toggle":
          omnibox?.open?.("omnibox", detail?.query);
          break;
        case "omnibox-open":
          omnibox?.open?.("omnibox", detail?.query);
          break;
        case "omnibox-find-toggle":
          omnibox?.open?.("find", detail?.query);
          break;
        case "omnibox-close":
          omnibox?.close?.();
          break;
        default:
          console.warn("[dumber] Unknown UI shortcut action:", action);
      }
    });
  });

  window.__dumber_toggle = () => {
    try {
      console.log("🎯 __dumber_toggle called");
      console.log("🔧 Omnibox API available:", !!window.__dumber_omnibox);
      if (window.__dumber_omnibox?.toggle) {
        console.log("✅ Using Svelte omnibox toggle");
        window.__dumber_omnibox.toggle();
        return;
      }

      throw new Error("Omnibox toggle requested but API is unavailable");
    } catch (error) {
      console.error("❌ Error in __dumber_toggle:", error);
    }
  };

  window.__dumber_find_open = (query?: string) => {
    try {
      if (window.__dumber_omnibox?.open) {
        window.__dumber_omnibox.open("find", query);
        return;
      }

      throw new Error("Omnibox find requested but API is unavailable");
    } catch (error) {
      console.error("❌ Error in __dumber_find_open:", error);
    }
  };

  window.__dumber_find_close = () => {
    if (window.__dumber_omnibox?.close) {
      window.__dumber_omnibox.close();
      return;
    }

    const error = new Error("Omnibox close requested but API is unavailable");
    console.error("❌ Error in __dumber_find_close:", error);
  };

  window.__dumber_find_query = (query: string) => {
    if (window.__dumber_omnibox?.findQuery) {
      window.__dumber_omnibox.findQuery(query);
    }
  };

  window.__dumber_gui = {
    initializeToast: async (config?: ToastConfig) => {
      return await initializeToast(config);
    },
    initializeOmnibox: async (config?: OmniboxInitConfig) => {
      return await initializeOmnibox(config);
    },
    keyboard: keyboardService,
    isReady: true,
  };

  window.__dumber_gui_bootstrap = function () {
    console.log("[gui-bootstrap] Initializing GUI for workspace");

    if (!window.__dumber_omnibox) {
      console.log(
        `[gui-bootstrap] Omnibox not yet available for webview ${currentWebViewId}, will be loaded when needed`,
      );
    } else {
      console.log(
        `[gui-bootstrap] Omnibox already available for webview ${currentWebViewId}`,
      );
      window.__dumber_omnibox.setActive(workspaceHasFocus);
    }

    console.log(
      `[gui-bootstrap] GUI bootstrap complete for webview ${currentWebViewId}`,
    );
  };
}
