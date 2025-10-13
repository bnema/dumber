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

    // Mount the Svelte toast container - this exposes all toast functions to window
    mount(ToastContainer, {
      target: rootElement as unknown as Element,
    });

    // Force immediate effect execution to ensure component functions are exposed
    flushSync();

    toastInitialized = true;
    console.log("✅ Toast system initialized - functions exposed by ToastContainer");
  } catch (error) {
    console.error("❌ Failed to initialize toast system:", error);
    throw error;
  }
}
