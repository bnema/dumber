/**
 * Omnibox Module Initialization
 *
 * Initializes the Svelte 5 omnibox component system
 */

import { mount } from 'svelte';
import { Omnibox } from '../../components/omnibox';
import { ensureShadowMount } from './shadowHost';
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
export async function initializeOmnibox(_config: OmniboxInitConfig = {}): Promise<void> {
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

    // Mount omnibox into the shared global Shadow DOM host
    const mountEl = ensureShadowMount('dumber-omnibox');
    omniboxComponent = mount(Omnibox, {
      target: mountEl as unknown as Element
    });

    // Wait for the component to mount and set up the global API
    await new Promise<void>((resolve) => {
      const checkAPI = () => {
        if (window.__dumber_omnibox && typeof window.__dumber_omnibox === 'object') {
          console.log('✅ Omnibox component system initialized');
          console.log('🔧 Global API is available:', Object.keys(window.__dumber_omnibox));
          isInitialized = true;
          // Notify isolated bundle listeners that omnibox is ready
          try {
            document.dispatchEvent(new CustomEvent('dumber:omnibox-ready'));
          } catch (e) {
            console.warn('Failed to dispatch omnibox-ready event', e);
          }
          resolve();
        } else {
          setTimeout(checkAPI, 10); // Check again in 10ms
        }
      };
      checkAPI();
    });

    // Bridge: listen for page-world events carrying suggestions and forward to API
    const handleSuggestionsEvent = (e: Event) => {
      try {
        const detail = (e as CustomEvent).detail;
        const suggestions = detail?.suggestions ?? detail;
        if (Array.isArray(suggestions) && window.__dumber_omnibox?.setSuggestions) {
          console.log('🔗 [Bridge] Received page-world suggestions event:', suggestions.length);
          window.__dumber_omnibox.setSuggestions(suggestions);
        } else {
          console.warn('🔗 [Bridge] Suggestions event received but API not ready or invalid payload');
        }
      } catch (err) {
        console.warn('🔗 [Bridge] Failed handling suggestions event:', err);
      }
    };

    document.addEventListener(
      'dumber:omnibox-suggestions',
      handleSuggestionsEvent,
      false
    );
    // Unified event name
    document.addEventListener('dumber:omnibox:suggestions', handleSuggestionsEvent, false);

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

  // The shared shadow host remains persistent; just log cleanup

  isInitialized = false;
  console.log('🧹 Omnibox component cleaned up');
}