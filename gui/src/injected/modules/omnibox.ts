/**
 * Omnibox Module Initialization
 *
 * Initializes the Svelte 5 omnibox component system
 */

import { mount } from 'svelte';
import { Omnibox } from '../../components/omnibox';
import type { OmniboxConfig } from '../../components/omnibox/types';

let omniboxComponent: ReturnType<typeof mount> | null = null;
let isInitialized = false;

export interface OmniboxInitConfig extends OmniboxConfig {
  // Additional initialization options if needed
  containerId?: string;
}

/**
 * Initialize the omnibox component system
 */
export async function initializeOmnibox(config: OmniboxInitConfig = {}): Promise<void> {
  if (isInitialized) {
    console.warn('Omnibox already initialized');
    return;
  }

  try {
    // Ensure we're in the main frame (not an iframe)
    if (window.self !== window.top) {
      console.log('🚫 Omnibox skipped in iframe');
      return;
    }

    // Check if DOM is ready
    if (!document.body) {
      console.warn('DOM body not ready for omnibox initialization');
      return;
    }

    // Create container element
    const containerId = config.containerId || 'dumber-omnibox-root';
    let container = document.getElementById(containerId);

    if (!container) {
      container = document.createElement('div');
      container.id = containerId;
      container.style.cssText = 'position:fixed;inset:0;z-index:2147483647;pointer-events:none;';
      document.documentElement.appendChild(container);
    }

    // Mount the Svelte component
    omniboxComponent = mount(Omnibox, {
      target: container
    });

    // Wait for the component to mount and set up the global API
    await new Promise<void>((resolve) => {
      const checkAPI = () => {
        if (window.__dumber_omnibox && typeof window.__dumber_omnibox === 'object') {
          console.log('✅ Omnibox component system initialized');
          console.log('🔧 Global API is available:', Object.keys(window.__dumber_omnibox));
          isInitialized = true;
          resolve();
        } else {
          setTimeout(checkAPI, 10); // Check again in 10ms
        }
      };
      checkAPI();
    });

  } catch (error) {
    console.error('❌ Failed to initialize omnibox component:', error);
    throw error;
  }
}

/**
 * Get the current initialization status
 */
export function isOmniboxInitialized(): boolean {
  return isInitialized;
}

/**
 * Cleanup omnibox component (useful for hot reload during development)
 */
export function cleanupOmnibox(): void {
  if (omniboxComponent) {
    try {
      omniboxComponent.$destroy?.();
    } catch (error) {
      console.warn('Error destroying omnibox component:', error);
    }
    omniboxComponent = null;
  }

  const container = document.getElementById('dumber-omnibox-root');
  if (container) {
    container.remove();
  }

  isInitialized = false;
  console.log('🧹 Omnibox component cleaned up');
}