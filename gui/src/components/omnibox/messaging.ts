/**
 * Omnibox Message Bridge
 *
 * TypeScript interface for Go-JavaScript communication
 */

import type { OmniboxMessage, OmniboxMessageBridge, Suggestion } from './types';
import { omniboxStore } from './stores.svelte.ts';

export class OmniboxBridge implements OmniboxMessageBridge {
  /**
   * Send message to Go backend via WebKit message handler
   */
  postMessage(msg: OmniboxMessage): void {
    try {
      console.log('📨 Sending message to Go backend:', msg);
      const messageHandler = window.webkit?.messageHandlers?.dumber;
      if (messageHandler && messageHandler.postMessage) {
        const jsonMsg = JSON.stringify(msg);
        console.log('📨 JSON message:', jsonMsg);
        messageHandler.postMessage(jsonMsg);
      } else {
        console.warn('WebKit message handler not available');
      }
    } catch (error) {
      console.error('Failed to send message to Go backend:', error);
    }
  }

  /**
   * Update suggestions from Go backend
   */
  setSuggestions(suggestions: Suggestion[]): void {
    omniboxStore.updateSuggestions(suggestions);
  }

  /**
   * Handle navigation request
   */
  navigate(url: string): void {
    console.log('🚀 Omnibox navigate called with:', url);
    this.postMessage({
      type: 'navigate',
      url
    });
  }

  /**
   * Handle search query
   */
  query(searchTerm: string, limit?: number): void {
    this.postMessage({
      type: 'query',
      q: searchTerm,
      limit: limit || omniboxStore.config.defaultLimit
    });
  }
}

// Singleton instance
export const omniboxBridge = new OmniboxBridge();

/**
 * Debounced query function for search input
 */
export function debouncedQuery(searchTerm: string): void {
  omniboxStore.clearDebounceTimer();

  const timerId = window.setTimeout(() => {
    omniboxBridge.query(searchTerm);
  }, omniboxStore.config.debounceDelay);

  omniboxStore.setDebounceTimer(timerId);
}

// Extend global window interface for Go bridge compatibility
declare global {
  interface Window {
    webkit?: {
      messageHandlers?: {
        dumber?: {
          postMessage: (message: string) => void;
        };
      };
    };
    // Omnibox API for Go bridge
    __dumber_omnibox?: {
      setSuggestions: (suggestions: Suggestion[]) => void;
      toggle: () => void;
      open: (mode?: string, query?: string) => void;
      close: () => void;
      findQuery: (query: string) => void;
    };
  }
}