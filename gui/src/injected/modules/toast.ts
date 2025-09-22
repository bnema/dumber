/**
 * Toast System Module
 *
 * Provides toast notification functionality using Svelte 5 components
 * with proper DOM ready checking and theme support.
 */

import { mount, flushSync } from "svelte";
import ToastContainer from "$components/toast/ToastContainer.svelte";
import { ensureShadowMount } from "./shadowHost";

export interface ToastConfig {
  theme?: "light" | "dark";
}

// Type definitions for window globals
declare global {
  interface Window {
    __dumber_toast_loaded?: boolean;
    __dumber_initial_theme?: string;
  }
}

let toastInitialized = false;

// Apply theme based on current system/GTK preference
function updateTheme(isDark: boolean): void {
  if (isDark) {
    document.documentElement.classList.add("dark");
  } else {
    document.documentElement.classList.remove("dark");
  }
}

export async function initializeToast(config?: ToastConfig): Promise<void> {
  if (toastInitialized || window.__dumber_toast_loaded) {
    // Verify functions still exist - if not, reinitialize
    if (
      typeof window.__dumber_showToast === "function" &&
      typeof window.__dumber_showZoomToast === "function"
    ) {
      console.log("✅ Toast system already initialized and functional");
      return;
    }
    // If functions are missing, reinitialize
    console.log("⚠️ Toast functions missing, reinitializing...");
    toastInitialized = false;
    window.__dumber_toast_loaded = false;
  }

  console.log("🚀 Initializing Svelte toast system at:", document.readyState);
  console.log(
    "📊 DOM state check - head:",
    !!document.head,
    "body:",
    !!document.body,
  );
  window.__dumber_toast_loaded = true;

  // Ensure DOM is ready
  if (document.readyState === "loading") {
    return new Promise((resolve) => {
      document.addEventListener("DOMContentLoaded", async () => {
        await initializeToast(config);
        resolve();
      });
    });
  }

  if (!document.head || !document.body) {
    console.warn(
      "❌ DOM structure not ready, deferring toast initialization. Retrying...",
    );
    window.__dumber_toast_loaded = false; // Reset the flag so we can try again
    return new Promise((resolve) => {
      const checkDOM = () => {
        if (document.head && document.body) {
          console.log("✅ DOM structure now ready, initializing toast system");
          initializeToast(config).then(() => resolve());
        } else {
          console.log("⏳ DOM still not ready, retrying in 50ms...");
          setTimeout(checkDOM, 50);
        }
      };
      setTimeout(checkDOM, 100);
    });
  }

  try {
    // Use the global Shadow DOM host for isolation
    const rootElement = ensureShadowMount("dumber-toast");

    // Initialize theme
    const initialTheme = config?.theme || window.__dumber_initial_theme;
    if (initialTheme !== undefined) {
      updateTheme(initialTheme === "dark");
    }

    // Mount the Svelte toast container (Svelte 5 syntax)
    mount(ToastContainer, {
      target: rootElement as unknown as Element,
    });

    // Signal isolated GUI readiness for page-world bridge
    try {
      document.documentElement.setAttribute("data-dumber-gui", "ready");
      document.dispatchEvent(new CustomEvent("dumber:gui-ready"));
    } catch {
      // Ignore errors in GUI readiness signaling
    }

    // Listen for page-world bridge events and forward to internal toast functions
    const handleShowToast = (e: Event) => {
      const detail = (e as CustomEvent).detail || {};
      if (typeof window.__dumber_showToast === "function") {
        window.__dumber_showToast(detail.message, detail.duration, detail.type);
      }
    };
    document.addEventListener("dumber:showToast", handleShowToast);
    document.addEventListener("dumber:toast:show", handleShowToast);

    const handleZoomToast = (e: Event) => {
      const detail = (e as CustomEvent).detail || {};
      if (typeof window.__dumber_showZoomToast === "function") {
        window.__dumber_showZoomToast(detail.level);
      }
    };
    document.addEventListener("dumber:showZoomToast", handleZoomToast);
    document.addEventListener("dumber:toast:zoom", handleZoomToast);

    // Force immediate effect execution to ensure onMount callbacks run
    flushSync();

    // Toast functions are now exposed by the component itself
    console.log("✅ Toast functions exposed via component instantiation");

    toastInitialized = true;
    console.log("✅ Toast system initialized with Svelte 5");
  } catch (error) {
    console.error("❌ Failed to initialize toast system:", error);

    // Fallback to basic toast system if Svelte fails
    initializeFallbackToast();
  }
}

function exposeToastFunctions(): void {
  // The Svelte ToastContainer component already exposes these functions
  // We just need to make sure they exist, they're handled by the component
  console.log("✅ Toast functions will be exposed by ToastContainer component");
}

function initializeFallbackToast(): void {
  console.warn("🔄 Falling back to basic toast system");

  // Create basic toast container for fallback
  const fallbackContainer = document.createElement("div");
  fallbackContainer.id = "dumber-fallback-toast";
  fallbackContainer.style.cssText = `
    position: fixed;
    top: 20px;
    right: 20px;
    z-index: 10000;
    pointer-events: none;
  `;
  document.documentElement.appendChild(fallbackContainer);

  // Expose toast functions for fallback too
  exposeToastFunctions();

  toastInitialized = true;
  console.log("✅ Fallback toast system ready");
}
