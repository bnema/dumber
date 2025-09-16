/**
 * Global Shadow DOM Host Utility
 *
 * Ensures a single, shared ShadowRoot is created and reused for all injected UI components
 * (omnibox, toast, etc.), with a minimal CSS reset applied once.
 */

// Import the compiled Tailwind bundle as a string so we can inject it
// directly into the shadow root. The ?inline suffix makes Vite embed the
// processed CSS, ensuring all utility classes are available without relying
// on runtime @import resolution (which fails inside the injected shadow DOM).
import globalStyles from '../../styles/tailwind.css?inline';

// Track shadow-root initialization to avoid duplicate reset injection
const shadowResetApplied = new WeakSet<ShadowRoot>();

const HOST_STYLES = `
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

let cachedHost: HTMLElement | null = null;
let hostObserver: MutationObserver | null = null;
let hostKeepaliveTimer: number | null = null;

function ensureHostPresence(host: HTMLElement): void {
  const docEl = document.documentElement;
  if (docEl && !host.isConnected) {
    docEl.appendChild(host);
  }

  if (typeof MutationObserver !== 'undefined') {
    if (!hostObserver) {
      hostObserver = new MutationObserver(() => {
        if (cachedHost && !cachedHost.isConnected && document.documentElement) {
          document.documentElement.appendChild(cachedHost);
        }
      });
    }

    try {
      hostObserver.disconnect();
      if (docEl) {
        hostObserver.observe(docEl, { childList: true });
      }
    } catch {
      // MutationObserver might not be allowed by the page; fall back to polling in that case.
    }
  }

  if (hostKeepaliveTimer === null && typeof window !== 'undefined') {
    hostKeepaliveTimer = window.setInterval(() => {
      if (cachedHost && !cachedHost.isConnected && document.documentElement) {
        document.documentElement.appendChild(cachedHost);
      }
    }, 2000);
  }
}

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
  }

  host.style.cssText = HOST_STYLES;

  cachedHost = host;
  ensureHostPresence(host);

  const shadowRoot = host.shadowRoot ?? host.attachShadow({ mode: 'open' });

  // Reflect current theme from document root to the shadow host so :host(.dark) works
  try {
    const isDark = document.documentElement.classList.contains('dark');
    host.classList.toggle('dark', isDark);
    // Keep it in sync if the theme toggles later
    const observer = new MutationObserver(() => {
      host.classList.toggle('dark', document.documentElement.classList.contains('dark'));
    });
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
  } catch { /* no-op */ }

  // Inject a minimal reset and base styles into the shadow root once
  if (!shadowResetApplied.has(shadowRoot)) {
    const resetStyle = document.createElement('style');
    resetStyle.textContent = `
      :host { all: initial; }
      *, *::before, *::after { box-sizing: border-box; }
      :host { font-family: system-ui, -apple-system, 'Segoe UI', Roboto, Ubuntu, 'Helvetica Neue', Arial, sans-serif; }
    `;
    shadowRoot.appendChild(resetStyle);

    // Ensure dynamic design tokens exist within the shadow tree. Tailwind v4
    // utilities rely on these CSS variables. We set light defaults and a dark variant on :host.
    const tokensStyle = document.createElement('style');
    tokensStyle.textContent = `
      :host {
        --dynamic-bg: var(--color-browser-bg);
        --dynamic-surface: var(--color-browser-surface);
        --dynamic-text: var(--color-browser-text);
        --dynamic-muted: var(--color-browser-muted);
        --dynamic-accent: var(--color-browser-accent);
        --dynamic-border: var(--color-browser-border);
      }
      :host(.dark) {
        --dynamic-bg: rgb(17 24 39);
        --dynamic-surface: rgb(31 41 55);
        --dynamic-text: rgb(243 244 246);
        --dynamic-muted: rgb(156 163 175);
        --dynamic-accent: rgb(96 165 250);
        --dynamic-border: rgb(55 65 81);
      }
    `;
    shadowRoot.appendChild(tokensStyle);

    // Inject the global GUI stylesheet so components rendered inside the shadow
    // root (e.g., toasts, omnibox) receive their styles
    try {
      // Prefer constructable stylesheets when available
      const hasConstructable = typeof CSSStyleSheet !== 'undefined';
      const supportsAdopted = 'adoptedStyleSheets' in shadowRoot;
      if (supportsAdopted && hasConstructable) {
        const sheet = new CSSStyleSheet();
        sheet.replaceSync(globalStyles);
        const rootWithSheets = shadowRoot as ShadowRoot & { adoptedStyleSheets: CSSStyleSheet[] };
        rootWithSheets.adoptedStyleSheets = [...(rootWithSheets.adoptedStyleSheets ?? []), sheet];
      } else {
        // Fallback: append a <style> element with the global CSS
        const styleTag = document.createElement('style');
        styleTag.textContent = globalStyles;
        shadowRoot.appendChild(styleTag);
      }
    } catch {
      // Final fallback if anything above fails
      const styleTag = document.createElement('style');
      styleTag.textContent = globalStyles;
      shadowRoot.appendChild(styleTag);
    }
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
