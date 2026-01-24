/**
 * Global type declarations for Dumber browser WebKit bridge
 */

// ═══════════════════════════════════════════════════════════════════════════════
// WEBKIT BRIDGE TYPES
// ═══════════════════════════════════════════════════════════════════════════════

interface DumberBridge {
  postMessage: (message: unknown) => void;
}

interface DumberWebKit {
  messageHandlers?: {
    dumber?: DumberBridge;
  };
}

// ═══════════════════════════════════════════════════════════════════════════════
// VIEW TRANSITION API (not yet in TypeScript DOM lib)
// ═══════════════════════════════════════════════════════════════════════════════

interface ViewTransition {
  finished: Promise<void>;
  ready: Promise<void>;
  updateCallbackDone: Promise<void>;
}

// ═══════════════════════════════════════════════════════════════════════════════
// THEME TYPES
// ═══════════════════════════════════════════════════════════════════════════════

type ThemeMode = 'light' | 'dark';

interface ColorSchemeManager {
  setUserPreference?: (theme: ThemeMode) => void;
}

// ═══════════════════════════════════════════════════════════════════════════════
// HOMEPAGE MESSAGING TYPES
// ═══════════════════════════════════════════════════════════════════════════════

/** Response from Go backend for homepage requests */
interface HomepageResponse {
  requestId: string;
  success: boolean;
  data?: unknown;
  error?: string;
}

/** Error response format (can be string for legacy or object) */
type HomepageErrorResponse = string | { requestId?: string; error?: string };

/** Callback type for homepage responses */
type HomepageResponseCallback = (response: HomepageResponse) => void;

/** Callback type for legacy data responses (can receive raw data or Response object) */
type HomepageLegacyCallback = (dataOrResponse: unknown) => void;

/** Callback type for error responses */
type HomepageErrorCallback = (errorOrResponse: HomepageErrorResponse) => void;

// ═══════════════════════════════════════════════════════════════════════════════
// GLOBAL DECLARATIONS
// ═══════════════════════════════════════════════════════════════════════════════

declare global {
  // Re-export bridge type for use in other modules
  type DumberBridge = {
    postMessage: (message: unknown) => void;
  };
  interface Window {
    // ─────────────────────────────────────────────────────────────────────────
    // Core WebKit Bridge
    // ─────────────────────────────────────────────────────────────────────────
    /** WebView ID assigned by the browser backend */
    __dumber_webview_id?: number;
    /** WebKit message handlers for native bridge */
    webkit?: DumberWebKit;

    // ─────────────────────────────────────────────────────────────────────────
    // Config Page Callbacks
    // ─────────────────────────────────────────────────────────────────────────
    /** Callback invoked when config save succeeds */
    __dumber_config_saved?: (resp?: unknown) => void;
    /** Callback invoked when config save fails */
    __dumber_config_error?: (msg: unknown) => void;

    // ─────────────────────────────────────────────────────────────────────────
    // Keybindings Callbacks
    // ─────────────────────────────────────────────────────────────────────────
    /** Callback invoked when keybindings are loaded */
    __dumber_keybindings_loaded?: (resp?: unknown) => void;
    /** Callback invoked when keybindings load fails */
    __dumber_keybindings_error?: (msg: unknown) => void;
    /** Callback invoked when a keybinding is set */
    __dumber_keybinding_set?: (resp?: unknown) => void;
    /** Callback invoked when keybinding set fails */
    __dumber_keybinding_set_error?: (msg: unknown) => void;
    /** Callback invoked when a keybinding is reset */
    __dumber_keybinding_reset?: (resp?: unknown) => void;
    /** Callback invoked when keybinding reset fails */
    __dumber_keybinding_reset_error?: (msg: unknown) => void;
    /** Callback invoked when all keybindings are reset */
    __dumber_keybindings_reset_all?: (resp?: unknown) => void;
    /** Callback invoked when reset all keybindings fails */
    __dumber_keybindings_reset_all_error?: (msg: unknown) => void;

    // ─────────────────────────────────────────────────────────────────────────
    // Homepage Callbacks
    // ─────────────────────────────────────────────────────────────────────────
    /** Generic response handler for homepage requests */
    __dumber_homepage_response?: HomepageResponseCallback;
    /** History timeline data callback */
    __dumber_history_timeline?: HomepageLegacyCallback;
    /** Folders data callback */
    __dumber_folders?: HomepageLegacyCallback;
    /** Tags data callback */
    __dumber_tags?: HomepageLegacyCallback;
    /** Favorites data callback */
    __dumber_favorites?: HomepageLegacyCallback;
    /** Analytics data callback */
    __dumber_analytics?: HomepageLegacyCallback;
    /** Domain stats data callback */
    __dumber_domain_stats?: HomepageLegacyCallback;
    /** History search results callback */
    __dumber_history_search_results?: HomepageLegacyCallback;
    /** History deleted callback */
    __dumber_history_deleted?: HomepageLegacyCallback;
    /** History cleared callback */
    __dumber_history_cleared?: HomepageLegacyCallback;
    /** Error callback for homepage requests */
    __dumber_error?: HomepageErrorCallback;

    // ─────────────────────────────────────────────────────────────────────────
    // Theme Management
    // ─────────────────────────────────────────────────────────────────────────
    /** Color scheme manager for theme switching */
    __dumber_color_scheme_manager?: ColorSchemeManager;
    /** Legacy theme setter */
    __dumber_setTheme?: (theme: ThemeMode) => void;
  }

  interface Document {
    /** View Transition API */
    startViewTransition?: (callback: () => void | Promise<void>) => ViewTransition;
  }
}

export {};
