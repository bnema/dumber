/**
 * Global Shadow DOM Host Utility
 *
 * Ensures a single, shared ShadowRoot is created and reused for all injected UI components
 * (omnibox, toast, etc.), with a minimal CSS reset applied once.
 */

// Track shadow-root initialization to avoid duplicate reset injection
const shadowResetApplied = new WeakSet<ShadowRoot>();

export const GLOBAL_SHADOW_HOST_ID = 'dumber-ui-root';

/**
 * Ensure and return the global ShadowRoot used by injected UI.
 */
export function getGlobalShadowRoot(): ShadowRoot {
  // Create or find the host container
  let host = document.getElementById(GLOBAL_SHADOW_HOST_ID) as HTMLElement | null;
  if (!host) {
    host = document.createElement('div');
    host.id = GLOBAL_SHADOW_HOST_ID;
    host.style.cssText = `
      position: fixed !important;
      top: 0 !important;
      left: 0 !important;
      right: 0 !important;
      bottom: 0 !important;
      z-index: 2147483647 !important;
      pointer-events: none !important;
      margin: 0 !important;
      padding: 0 !important;
      border: none !important;
      background: none !important;
      isolation: isolate !important;
      contain: layout style !important;
    `;
    document.documentElement.appendChild(host);
  }

  const shadowRoot = host.shadowRoot ?? host.attachShadow({ mode: 'open' });

  // Inject a minimal reset and base styles into the shadow root once
  if (!shadowResetApplied.has(shadowRoot)) {
    const resetStyle = document.createElement('style');
    resetStyle.textContent = `
      :host { all: initial; }
      *, *::before, *::after { box-sizing: border-box; }
      :host { font-family: system-ui, -apple-system, 'Segoe UI', Roboto, Ubuntu, 'Helvetica Neue', Arial, sans-serif; }
    `;
    shadowRoot.appendChild(resetStyle);
    shadowResetApplied.add(shadowRoot);
  }

  return shadowRoot;
}

/**
 * Ensure and return a dedicated mount element inside the global ShadowRoot.
 * Use a unique id per feature (e.g., "dumber-omnibox", "dumber-toast").
 */
export function ensureShadowMount(mountId: string): HTMLElement {
  const root = getGlobalShadowRoot();
  let mount = root.getElementById?.(mountId) as HTMLElement | null;
  if (!mount) {
    mount = document.createElement('div');
    mount.id = mountId;
    mount.style.cssText = `
      position: relative;
      pointer-events: none; /* components inside control their own pointer events */
    `;
    root.appendChild(mount);
  }
  return mount;
}
