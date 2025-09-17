import type { HistoryEntry, SearchShortcut } from '../types/generated.js';

// Global window API types for browser integration
declare global {
  interface Window {
    // Toast system API
    __dumber_toast_loaded?: boolean;
    __dumber_showToast?: (message: string, duration?: number, type?: 'info' | 'success' | 'error') => number | void;
    __dumber_dismissToast?: (id: number) => void;
    __dumber_clearToasts?: () => void;
    __dumber_showZoomToast?: (zoomLevel: number) => void;

    // Theme integration
    __dumber_initial_theme?: string;
    __dumber_setTheme?: (theme: 'light' | 'dark') => void;

    // History callbacks
    __dumber_history_recent?: (data: HistoryEntry[]) => void;
    __dumber_history_error?: (error: string) => void;

    // Shortcuts callbacks
    __dumber_shortcuts_loaded?: (data: Record<string, SearchShortcut>) => void;
    __dumber_shortcuts_error?: (error: string) => void;

    // WebKit message handler
    webkit?: {
      messageHandlers?: {
        dumber?: {
          postMessage: (message: string) => void;
        };
      };
    };
  }
}

export interface ToastAPI {
  show: (message: string, duration?: number, type?: 'info' | 'success' | 'error') => number;
  dismiss: (id: number) => void;
  clear: () => void;
  showZoom: (zoomLevel: number) => void;
}

export interface ThemeAPI {
  setTheme: (theme: 'light' | 'dark') => void;
  getCurrentTheme: () => 'light' | 'dark';
}

// Message types for WebKit communication
export interface BrowserMessage {
  action: string;
  data?: unknown;
}

export {}; // Make this file a module