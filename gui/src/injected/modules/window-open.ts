/**
 * Window Open Interceptor Module
 *
 * OBSOLETE: window.open() is now handled directly via page-world bridge bypass.
 * This module is kept for compatibility but no longer performs active interception.
 */

let isInitialized = false;

/**
 * Initialize the window.open interceptor (no-op)
 * Window.open is now handled directly via JavaScript bypass in page-world
 */
export function initializeWindowOpenInterceptor(): void {
  if (isInitialized) {
    return;
  }

  // No-op - window.open handled directly via page-world bridge
  isInitialized = true;
  console.log("[window-open] ✅ Interceptor initialized (bypass mode)");
}

/**
 * Get initialization status
 */
export function isWindowOpenInterceptorInitialized(): boolean {
  return isInitialized;
}
