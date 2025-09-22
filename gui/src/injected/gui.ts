/**
 * Unified GUI Bundle Entry Point
 *
 * Single entry point for all WebKit GUI functionality.
 * Loads immediately and provides modular initialization system.
 */

// Import all GUI modules
import { initializeToast, type ToastConfig } from "./modules/toast";
import { initializeOmnibox, type OmniboxInitConfig } from "./modules/omnibox";
import {
  initializeWorkspace,
  type WorkspaceConfigPayload,
  type WorkspaceRuntime,
} from "./modules/workspace";
import { initializeWindowOpenInterceptor } from "./modules/window-open";
import { keyboardService, type KeyboardService } from "$lib/keyboard";
import type { Suggestion } from "../components/omnibox/types";
// Note: color-scheme module is loaded separately at document-start by WebKit

// Global interface for the unified GUI system
interface DumberGUI {
  initializeToast: (config?: ToastConfig) => Promise<void>;
  initializeOmnibox: (config?: OmniboxInitConfig) => Promise<void>;
  keyboard: KeyboardService;
  isReady: boolean;
}

// (legacy suggestions normalization removed; omnibox now fetches suggestions directly via API)

// DOM readiness utility - handles all document ready states
function whenDOMReady(callback: () => void) {
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", callback, { once: true });
  } else if (document.readyState === "interactive" && !document.body) {
    // Interactive but no body yet, wait a bit more
    setTimeout(() => whenDOMReady(callback), 10);
  } else {
    // DOM ready and body exists
    callback();
  }
}

// Install a page-world bridge so Go can call window.__dumber_omnibox_suggestions()
// Page-world bridge is injected by WebKit at document-start; no-op here to avoid duplicates.

// Create global namespace
declare global {
  interface Window {
    __dumber_gui?: DumberGUI;
    __dumber_gui_ready?: boolean;
    __dumber_showToast?: (
      message: string,
      duration?: number,
      type?: "info" | "success" | "error",
    ) => number | void;
    __dumber_showZoomToast?: (level: number) => void;
    __dumber_keyboard?: KeyboardService;
    __dumber_workspace_config?: WorkspaceConfigPayload;
    __dumber_workspace?: WorkspaceRuntime;
    __dumber_pane?: { id: string; active: boolean };
    __dumber_omnibox?: {
      setSuggestions: (suggestions: Suggestion[]) => void;
      toggle: () => void;
      open: (mode?: string, query?: string) => void;
      close: () => void;
      findQuery: (query: string) => void;
      setActive: (active: boolean) => void;
    };
    __dumber_gui_bootstrap?: () => void;
    // Legacy compatibility functions for Go bridge
    __dumber_toggle?: () => void;
    __dumber_find_open?: (query?: string) => void;
    __dumber_find_close?: () => void;
    __dumber_find_query?: (query: string) => void;
  }
}

// Prevent multiple initialization with enhanced logging
if (!window.__dumber_gui_ready) {
  // Skip initialization in iframes - GUI only needed in main frame
  if (window.self !== window.top) {
    console.log("🚫 Dumber GUI skipped in iframe");
  } else {
    window.__dumber_gui_ready = true;

    let workspaceHasFocus = true; // Assume focus initially, will be corrected by focus events
    let currentPaneId = window.__dumber_pane?.id || "unknown";
    let hasReceivedFocusEvent = false; // Track if we've received any workspace focus events

    document.addEventListener("dumber:workspace-focus", (event: Event) => {
      const detail = (event as CustomEvent).detail || {};
      console.log(
        "[workspace focus event]",
        detail,
        "prev focus=",
        workspaceHasFocus,
      );

      hasReceivedFocusEvent = true;

      // Update focus state
      if (typeof detail?.active === "boolean") {
        workspaceHasFocus = detail.active;
      } else {
        workspaceHasFocus = detail !== false;
      }

      // Update pane tracking
      if (detail?.paneId) {
        currentPaneId = detail.paneId;
      }

      // Update global pane state
      if (window.__dumber_pane) {
        window.__dumber_pane.active = workspaceHasFocus;
      }

      console.log(
        "[workspace focus updated] focus=",
        workspaceHasFocus,
        "paneId=",
        currentPaneId,
      );

      // Handle GUI visibility based on focus
      if (window.__dumber_omnibox) {
        if (workspaceHasFocus) {
          window.__dumber_omnibox.setActive(true);
        } else {
          window.__dumber_omnibox.setActive(false);
          window.__dumber_omnibox.close();
        }
      }
    });

    // Initialize Svelte GUI systems immediately
    whenDOMReady(async () => {
      try {
        // Page-world bridge is injected by WebKit; avoid duplicate injection here

        // Initialize window.open interceptor first (must be early)
        initializeWindowOpenInterceptor();

        // Initialize toast system first
        await initializeToast();

        // Verify toast functions are available
        if (typeof window.__dumber_showToast !== "function") {
          throw new Error("Toast functions not exposed after initialization");
        }

        // Initialize omnibox system
        await initializeOmnibox();

        // Initialize workspace controls (pane/tab scaffolding)
        initializeWorkspace(window.__dumber_workspace_config);

        console.log("✅ All GUI systems initialized successfully");
      } catch (e) {
        console.error("❌ Failed to initialize Svelte toast system:", e);

        // Enhanced fallback with better error recovery
        window.__dumber_showToast = (
          message: string,
          duration: number = 2500,
        ) => {
          // Remove existing toasts to prevent overlap
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

            // Animate in
            requestAnimationFrame(() => {
              toast.style.opacity = "1";
              toast.style.transform = "translateX(0)";
            });

            // Animate out and remove
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

        // Expose other required functions
        window.__dumber_dismissToast = () => {
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
    });

    // Expose keyboard service globally for Go bridge
    window.__dumber_keyboard = keyboardService;

    // Initialize global keyboard and mouse event listeners for the keyboard service
    whenDOMReady(() => {
      // Global keyboard event listener (capture phase for high priority)
      document.addEventListener(
        "keydown",
        (event) => {
          keyboardService.handleKeyboardEvent(event);
        },
        true,
      );

      // Global mouse event listener for navigation buttons
      document.addEventListener(
        "mousedown",
        (event) => {
          keyboardService.handleMouseEvent(event);
        },
        true,
      );

      console.log("✅ KeyboardService initialized with global listeners");

      // Listen to bridge keyboard events from page-world and forward to KeyboardService
      document.addEventListener("dumber:key", (e: Event) => {
        const detail = (e as CustomEvent).detail || {};
        if (detail && typeof detail.shortcut === "string") {
          keyboardService.handleNativeShortcut(detail.shortcut);
        }
      });

      document.addEventListener("dumber:ui:shortcut", (e: Event) => {
        const detail = (e as CustomEvent).detail || {};
        const action = detail?.action;
        const eventPaneId = detail?.paneId;
        const source = detail?.source || "unknown";

        if (typeof action !== "string") {
          return;
        }

        // Enhanced focus checking with pane ID validation
        const isForThisPane = !eventPaneId || eventPaneId === currentPaneId;
        // Only apply strict focus checking to omnibox actions, be permissive for others
        const isOmniboxAction = action.startsWith("omnibox-");
        const shouldHandle =
          isForThisPane &&
          (isOmniboxAction
            ? hasReceivedFocusEvent
              ? workspaceHasFocus
              : true
            : true);

        if (!shouldHandle) {
          console.log("[dumber shortcuts] ignored action", {
            action,
            workspaceHasFocus,
            isForThisPane,
            eventPaneId,
            currentPaneId,
            source,
            detail,
            hasReceivedFocusEvent,
          });
          return;
        }

        console.log("[dumber shortcuts] handling action", {
          action,
          source,
          paneId: currentPaneId,
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

      // Suggestions are now fetched via API directly by the omnibox component; no page-bridge handlers needed
    });

    // Legacy compatibility functions for existing Go code
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

    // Find query function that Go uses to set find query
    window.__dumber_find_query = (query: string) => {
      if (window.__dumber_omnibox?.findQuery) {
        window.__dumber_omnibox.findQuery(query);
      }
    };

    // Create the global GUI object
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

    // Bootstrap function for lazy GUI initialization by workspace manager
    window.__dumber_gui_bootstrap = function () {
      console.log("[gui-bootstrap] Initializing GUI for workspace");

      // Pane ID will be updated via focus events

      // Ensure omnibox is available for this pane
      if (!window.__dumber_omnibox) {
        console.log(
          `[gui-bootstrap] Omnibox not yet available for pane ${currentPaneId}, will be loaded when needed`,
        );
      } else {
        console.log(
          `[gui-bootstrap] Omnibox already available for pane ${currentPaneId}`,
        );
        // Ensure omnibox is aware of the current pane
        window.__dumber_omnibox.setActive(workspaceHasFocus);
      }

      console.log(
        `[gui-bootstrap] GUI bootstrap complete for pane ${currentPaneId}`,
      );
    };
  }
}
